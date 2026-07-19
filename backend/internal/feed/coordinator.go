package feed

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mmcdole/gofeed"
)

const XiaoyuzhouFeedDomain = "feed.xyzfm.space"

const (
	defaultCircuitCooldown     = 10 * time.Minute
	defaultRetryBackoffInitial = 30 * time.Second
	defaultRetryBackoffMax     = 10 * time.Minute
	maxRetryAfter              = 24 * time.Hour
)

var ErrFeedCircuitOpen = errors.New("feed circuit is open")

// DomainPolicy controls only load-shaping behavior for one target domain.
// MaxConcurrency <= 0 means unlimited; duplicate-request coalescing remains
// active for every domain as a shared correctness rule.
type DomainPolicy struct {
	MaxConcurrency      int
	MinRefreshInterval  time.Duration
	MaxJitter           time.Duration
	CircuitCooldown     time.Duration
	RetryBackoffInitial time.Duration
	RetryBackoffMax     time.Duration
}

type CoordinatorConfig struct {
	DomainPolicies map[string]DomainPolicy
	LastGoodStore  SnapshotStore
	MaxStaleAge    time.Duration
}

type Coordinator struct {
	mu           sync.Mutex
	policies     map[string]DomainPolicy
	inFlight     map[string]*inFlightFetch
	sharedResult map[string]cachedFetch
	semaphores   map[string]chan struct{}
	lastGood     SnapshotStore
	maxStaleAge  time.Duration
	circuits     map[string]*domainCircuit
}

type inFlightFetch struct {
	done   chan struct{}
	result *FetchResult
	err    error
}

type cachedFetch struct {
	storedAt time.Time
	result   *FetchResult
}

type domainCircuit struct {
	openUntil     time.Time
	failureCount  int
	probeInFlight bool
	state         CircuitState
}

// DefaultCoordinatorConfig applies the initial conservative policy only to
// Xiaoyuzhou Feed traffic. Other domains retain their existing parallelism.
func DefaultCoordinatorConfig() CoordinatorConfig {
	return CoordinatorConfig{
		DomainPolicies: map[string]DomainPolicy{
			XiaoyuzhouFeedDomain: {
				MaxConcurrency:      1,
				MinRefreshInterval:  5 * time.Minute,
				MaxJitter:           2 * time.Second,
				CircuitCooldown:     defaultCircuitCooldown,
				RetryBackoffInitial: defaultRetryBackoffInitial,
				RetryBackoffMax:     defaultRetryBackoffMax,
			},
		},
	}
}

func NewCoordinator(config CoordinatorConfig) *Coordinator {
	policies := make(map[string]DomainPolicy, len(config.DomainPolicies))
	for domain, policy := range config.DomainPolicies {
		policies[strings.ToLower(strings.TrimSpace(domain))] = policy
	}
	lastGoodStore := config.LastGoodStore
	if lastGoodStore == nil {
		lastGoodStore = NewMemorySnapshotStore(LastGoodStoreConfig{})
	}
	maxStaleAge := config.MaxStaleAge
	if maxStaleAge == 0 {
		maxStaleAge = defaultLastGoodMaxStaleAge
	}
	return &Coordinator{
		policies:     policies,
		inFlight:     make(map[string]*inFlightFetch),
		sharedResult: make(map[string]cachedFetch),
		semaphores:   make(map[string]chan struct{}),
		lastGood:     lastGoodStore,
		maxStaleAge:  maxStaleAge,
		circuits:     make(map[string]*domainCircuit),
	}
}

var processCoordinator = NewCoordinator(DefaultCoordinatorConfig())

// SharedCoordinator is the process-wide coordination boundary used by normal
// workflow Feed fetchers.
func SharedCoordinator() *Coordinator {
	return processCoordinator
}

// CanonicalizeURL normalizes only representation details that cannot identify
// a different Feed. Paths and query values remain distinct.
func CanonicalizeURL(rawURL string) string {
	parsed, err := parseURL(rawURL)
	if err != nil {
		return rawURL
	}
	parsed.Fragment = ""
	return parsed.String()
}

func parseURL(rawURL string) (*url.URL, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)
	if (parsed.Scheme == "http" && parsed.Port() == "80") || (parsed.Scheme == "https" && parsed.Port() == "443") {
		parsed.Host = parsed.Hostname()
	}
	return parsed, nil
}

// Do coalesces identical in-flight requests, reuses a successful shared result
// during the configured minimum interval, and serializes only domains with a
// positive concurrency policy.
func (c *Coordinator) Do(ctx context.Context, rawURL string, fetch func(context.Context) (*FetchResult, error)) (*FetchResult, error) {
	if c == nil {
		return fetch(ctx)
	}
	if ctx == nil {
		ctx = context.Background()
	}

	key := CanonicalizeURL(rawURL)
	domain := TargetDomain(rawURL)
	policy := c.policyFor(domain)

	c.mu.Lock()
	if cached, ok := c.sharedResult[key]; ok && policy.MinRefreshInterval > 0 && time.Since(cached.storedAt) < policy.MinRefreshInterval {
		result := cloneFetchResult(cached.result)
		c.mu.Unlock()
		return markSharedResult(result), nil
	}
	c.mu.Unlock()

	if policy.MinRefreshInterval > 0 {
		if result, ok := c.freshLocalResult(ctx, rawURL, key, policy.MinRefreshInterval); ok {
			return result, nil
		}
	}

	c.mu.Lock()
	if call, ok := c.inFlight[key]; ok {
		c.mu.Unlock()
		select {
		case <-call.done:
			result := cloneFetchResult(call.result)
			if call.err != nil {
				return result, call.err
			}
			return markSharedResult(result), nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	call := &inFlightFetch{done: make(chan struct{})}
	c.inFlight[key] = call
	c.mu.Unlock()

	result, err := c.run(ctx, rawURL, policy, fetch)
	c.mu.Lock()
	sharedResult := cloneFetchResult(result)
	if err == nil && result != nil && result.Feed != nil && policy.MinRefreshInterval > 0 {
		c.sharedResult[key] = cachedFetch{storedAt: time.Now(), result: sharedResult}
	}
	if err == nil && result != nil && result.Feed != nil {
		c.saveLastGood(key, result)
	}
	delete(c.inFlight, key)
	call.result = sharedResult
	call.err = err
	close(call.done)
	c.mu.Unlock()
	return result, err
}

func circuitPolicyEnabled(policy DomainPolicy) bool {
	return policy.CircuitCooldown > 0 || policy.RetryBackoffInitial > 0 || policy.RetryBackoffMax > 0
}

func (c *Coordinator) reserveCircuitLocked(domain string) (probe bool, circuitState CircuitState, openUntil time.Time, blocked bool) {
	state := c.circuits[domain]
	if state == nil {
		state = &domainCircuit{state: CircuitStateNotUsed}
		c.circuits[domain] = state
	}
	now := time.Now()
	if state.probeInFlight {
		return false, CircuitStateOpen, state.openUntil, true
	}
	if state.openUntil.After(now) {
		return false, CircuitStateOpen, state.openUntil, true
	}
	if !state.openUntil.IsZero() {
		state.probeInFlight = true
		state.state = CircuitStateProbe
		return true, CircuitStateProbe, state.openUntil, false
	}
	return false, state.state, time.Time{}, false
}

func (c *Coordinator) completeCircuitLocked(domain string, policy DomainPolicy, probe bool, result *FetchResult, err error) {
	state := c.circuits[domain]
	if state == nil {
		return
	}
	state.probeInFlight = false
	if err == nil && result != nil && result.Feed != nil {
		if probe {
			state.openUntil = time.Time{}
			state.failureCount = 0
			state.state = CircuitStateClosed
		}
		return
	}
	if probe {
		state.failureCount++
		state.openUntil = time.Now().Add(circuitWait(policy, state.failureCount, result))
		state.state = CircuitStateOpen
		return
	}
	if !isCircuitFailure(result) {
		return
	}

	state.failureCount++
	state.openUntil = time.Now().Add(circuitWait(policy, state.failureCount, result))
	state.state = CircuitStateOpen
	if result != nil {
		result.Access.CircuitState = CircuitStateOpen
	}
}

func isCircuitFailure(result *FetchResult) bool {
	if result == nil || result.Access.HTTPStatus == nil {
		return false
	}
	switch *result.Access.HTTPStatus {
	case http.StatusForbidden, http.StatusTooManyRequests, http.StatusServiceUnavailable:
		return true
	default:
		return false
	}
}

func circuitWait(policy DomainPolicy, failureCount int, result *FetchResult) time.Duration {
	if result != nil && result.Access.HTTPStatus != nil && *result.Access.HTTPStatus == http.StatusForbidden {
		if policy.CircuitCooldown > 0 {
			return policy.CircuitCooldown
		}
		return defaultCircuitCooldown
	}
	if result != nil {
		if wait, ok := parseRetryAfter(result.Access.RetryAfter, time.Now()); ok {
			if wait > maxRetryAfter {
				return maxRetryAfter
			}
			return wait
		}
	}
	initial := policy.RetryBackoffInitial
	if initial <= 0 {
		initial = defaultRetryBackoffInitial
	}
	maximum := policy.RetryBackoffMax
	if maximum <= 0 {
		maximum = defaultRetryBackoffMax
	}
	wait := initial
	for i := 1; i < failureCount && wait < maximum; i++ {
		if wait > maximum/2 {
			wait = maximum
			break
		}
		wait *= 2
	}
	if wait > maximum {
		return maximum
	}
	return wait
}

func parseRetryAfter(value string, now time.Time) (time.Duration, bool) {
	if seconds, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64); err == nil && seconds >= 0 {
		if seconds > int64(maxRetryAfter/time.Second) {
			return maxRetryAfter, true
		}
		return time.Duration(seconds) * time.Second, true
	}
	when, err := http.ParseTime(strings.TrimSpace(value))
	if err != nil {
		return 0, false
	}
	wait := when.Sub(now)
	if wait < 0 {
		return 0, true
	}
	return wait, true
}

func circuitOpenResult(rawURL string, openUntil time.Time) *FetchResult {
	access := newPrimaryAccessOutcome(rawURL)
	access.ErrorCategory = ErrorCategoryCircuitOpen
	access.CircuitState = CircuitStateOpen
	access.EgressID = EgressUnknown
	if remaining := time.Until(openUntil); remaining > 0 {
		seconds := int64(remaining / time.Second)
		if remaining%time.Second != 0 {
			seconds++
		}
		access.RetryAfter = strconv.FormatInt(seconds, 10)
	}
	return &FetchResult{Error: ErrFeedCircuitOpen, Access: access}
}

// LastGood restores a bounded, verified snapshot after the caller has
// observed a live-fetch failure. Keeping this explicit preserves the source
// order needed for a future verified alternative Feed.
func (c *Coordinator) LastGood(ctx context.Context, rawURL string, failure *FetchResult) (*FetchResult, bool) {
	if c == nil || c.lastGood == nil {
		return nil, false
	}
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		return nil, false
	default:
	}

	key := CanonicalizeURL(rawURL)
	snapshot, ok := c.lastGood.Load(key)
	if !ok || snapshot == nil || !snapshotUsable(snapshot, key, c.maxStaleAge) {
		return nil, false
	}
	parsed, err := parseSnapshot(snapshot)
	if err != nil {
		return nil, false
	}

	access := newPrimaryAccessOutcome(rawURL)
	if failure != nil {
		access = failure.Access
		if failure.Access.HTTPStatus != nil {
			status := *failure.Access.HTTPStatus
			access.HTTPStatus = &status
		}
	}
	access.SourceType = AccessSourceLastGood
	access.CacheStatus = CacheStatusHit
	access.Freshness = snapshotFreshness(snapshot, time.Duration(0))
	access.ResponseTimeMs = 0
	access.ResponseBytes = 0
	access.RetrievedAt = snapshotTime(snapshot)

	return &FetchResult{
		Feed:       parsed,
		RawContent: append([]byte(nil), snapshot.RawContent...),
		Access:     access,
	}, true
}

func (c *Coordinator) run(ctx context.Context, rawURL string, policy DomainPolicy, fetch func(context.Context) (*FetchResult, error)) (result *FetchResult, err error) {
	domain := TargetDomain(rawURL)
	circuitEnabled := circuitPolicyEnabled(policy)
	probe := false
	reservedCircuit := false
	circuitState := CircuitStateNotUsed
	defer func() {
		if recovered := recover(); recovered != nil {
			result = nil
			err = fmt.Errorf("feed access coordinator panic: %v", recovered)
		}
		if circuitEnabled && reservedCircuit {
			if result != nil {
				if probe {
					result.Access.CircuitState = CircuitStateProbe
				} else if circuitState == CircuitStateClosed {
					result.Access.CircuitState = CircuitStateClosed
				}
			}
			c.mu.Lock()
			c.completeCircuitLocked(domain, policy, probe, result, err)
			c.mu.Unlock()
		}
	}()

	if policy.MaxJitter > 0 {
		jitter := time.Duration(rand.Int63n(int64(policy.MaxJitter) + 1))
		timer := time.NewTimer(jitter)
		select {
		case <-timer.C:
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return nil, ctx.Err()
		}
	}

	if circuitEnabled {
		c.mu.Lock()
		openUntil, blocked := c.circuitBlockedLocked(domain)
		c.mu.Unlock()
		if blocked {
			return circuitOpenResult(rawURL, openUntil), ErrFeedCircuitOpen
		}
	}

	var release func()
	if policy.MaxConcurrency > 0 {
		state := c.domainSemaphore(TargetDomain(rawURL), policy.MaxConcurrency)
		select {
		case state <- struct{}{}:
			release = func() { <-state }
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if release != nil {
		defer release()
	}

	if circuitEnabled {
		c.mu.Lock()
		var openUntil time.Time
		var blocked bool
		probe, circuitState, openUntil, blocked = c.reserveCircuitLocked(domain)
		if blocked {
			c.mu.Unlock()
			return circuitOpenResult(rawURL, openUntil), ErrFeedCircuitOpen
		}
		reservedCircuit = true
		c.mu.Unlock()
	}

	result, err = fetch(ctx)
	if result != nil && err == nil && policy.MinRefreshInterval > 0 {
		result.Access.CacheStatus = CacheStatusMiss
	}
	return result, err
}

func (c *Coordinator) circuitBlockedLocked(domain string) (time.Time, bool) {
	state := c.circuits[domain]
	if state == nil {
		return time.Time{}, false
	}
	if state.probeInFlight || state.openUntil.After(time.Now()) {
		return state.openUntil, true
	}
	return time.Time{}, false
}

func (c *Coordinator) domainSemaphore(domain string, maxConcurrency int) chan struct{} {
	key := fmt.Sprintf("%s:%d", domain, maxConcurrency)
	c.mu.Lock()
	defer c.mu.Unlock()
	if existing, ok := c.semaphores[key]; ok {
		return existing
	}
	created := make(chan struct{}, maxConcurrency)
	c.semaphores[key] = created
	return created
}

func (c *Coordinator) policyFor(domain string) DomainPolicy {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.policies[strings.ToLower(domain)]
}

func (c *Coordinator) saveLastGood(key string, result *FetchResult) {
	if c.lastGood == nil || len(result.RawContent) == 0 {
		return
	}
	snapshot := FeedSnapshot{
		FeedURL:    key,
		RawContent: result.RawContent,
	}
	if result.Access.RetrievedAt != nil {
		snapshot.RetrievedAt = *result.Access.RetrievedAt
	}
	if snapshot.RetrievedAt.IsZero() {
		snapshot.RetrievedAt = time.Now()
	}
	_ = c.lastGood.Save(snapshot)
}

func (c *Coordinator) freshLocalResult(ctx context.Context, rawURL, key string, freshFor time.Duration) (*FetchResult, bool) {
	if c.lastGood == nil {
		return nil, false
	}
	select {
	case <-ctx.Done():
		return nil, false
	default:
	}
	snapshot, ok := c.lastGood.Load(key)
	if !ok || !snapshotUsable(snapshot, key, freshFor) {
		return nil, false
	}
	parsed, err := parseSnapshot(snapshot)
	if err != nil {
		return nil, false
	}
	access := newPrimaryAccessOutcome(rawURL)
	access.SourceType = AccessSourceLocalCache
	access.CacheStatus = CacheStatusHit
	access.Freshness = FreshnessFresh
	access.ResponseTimeMs = 0
	access.ResponseBytes = 0
	access.EgressID = "none"
	access.RetrievedAt = snapshotTime(snapshot)
	return &FetchResult{
		Feed:       parsed,
		RawContent: append([]byte(nil), snapshot.RawContent...),
		Access:     access,
	}, true
}

func snapshotUsable(snapshot *FeedSnapshot, key string, maxAge time.Duration) bool {
	if snapshot == nil || CanonicalizeURL(snapshot.FeedURL) != key || snapshot.RetrievedAt.IsZero() {
		return false
	}
	if err := validateSnapshot(snapshot); err != nil {
		return false
	}
	age := time.Since(snapshot.RetrievedAt)
	if age < 0 {
		return true
	}
	return maxAge < 0 || age <= maxAge
}

func snapshotFreshness(snapshot *FeedSnapshot, freshFor time.Duration) Freshness {
	if snapshot == nil {
		return FreshnessUnknown
	}
	if freshFor > 0 && time.Since(snapshot.RetrievedAt) <= freshFor {
		return FreshnessFresh
	}
	return FreshnessStale
}

func snapshotTime(snapshot *FeedSnapshot) *time.Time {
	if snapshot == nil || snapshot.RetrievedAt.IsZero() {
		return nil
	}
	retrievedAt := snapshot.RetrievedAt
	return &retrievedAt
}

func parseSnapshot(snapshot *FeedSnapshot) (*gofeed.Feed, error) {
	if err := validateSnapshot(snapshot); err != nil {
		return nil, err
	}
	parsed, err := gofeed.NewParser().Parse(bytes.NewReader(snapshot.RawContent))
	if err != nil || parsed == nil {
		if err == nil {
			err = fmt.Errorf("parsed feed is nil")
		}
		return nil, err
	}
	return parsed, nil
}

func cloneFetchResult(result *FetchResult) *FetchResult {
	if result == nil {
		return nil
	}
	clone := *result
	if result.Access.HTTPStatus != nil {
		status := *result.Access.HTTPStatus
		clone.Access.HTTPStatus = &status
	}
	if result.NewItems != nil {
		clone.NewItems = append([]*gofeed.Item(nil), result.NewItems...)
	}
	if result.RawContent != nil {
		clone.RawContent = append([]byte(nil), result.RawContent...)
	}
	if result.Access.RetrievedAt != nil {
		retrievedAt := *result.Access.RetrievedAt
		clone.Access.RetrievedAt = &retrievedAt
	}
	return &clone
}

func markSharedResult(result *FetchResult) *FetchResult {
	if result == nil {
		return nil
	}
	result.Access.SourceType = AccessSourceSharedCache
	result.Access.CacheStatus = CacheStatusHit
	result.Access.Freshness = FreshnessFresh
	result.Access.ResponseTimeMs = 0
	result.Access.ResponseBytes = 0
	result.Access.EgressID = "none"
	return result
}
