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
	"magicpodcast/internal/logger"
)

const XiaoyuzhouFeedDomain = "feed.xyzfm.space"

const (
	defaultCircuitCooldown     = 10 * time.Minute
	defaultRetryBackoffInitial = 30 * time.Second
	defaultRetryBackoffMax     = 10 * time.Minute
	maxRetryAfter              = 24 * time.Hour

	// HalfOpen recovery tuning. After an OPEN circuit's cooldown elapses it
	// transitions to probe (half-open): a bounded number of probe requests are
	// admitted, and SuccessesToClose consecutive successes close the circuit.
	defaultHalfOpenMaxRequests = 1
	defaultSuccessesToClose    = 2
	// Evidence-gated domain health: 5xx/timeout/network failures only OPEN a
	// domain circuit once at least this many distinct feeds have failed within
	// the evidence window. A single flaky feed therefore cannot block an
	// otherwise-healthy domain unless the policy opts into immediate behavior.
	defaultDomainEvidenceMinDistinctFeeds = 1
	defaultEvidenceWindow                 = 5 * time.Minute
)

var ErrFeedCircuitOpen = errors.New("feed circuit is open")

// ErrFeedNotModified signals a 304 Not Modified response. It is not a failure:
// the conditional check confirmed the persisted snapshot is still current, so
// the Coordinator recovers the Feed from that snapshot instead of treating the
// empty 304 body as a parse error. It never escapes to callers.
var ErrFeedNotModified = errors.New("feed not modified")

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

	// HalfOpenMaxRequests is the number of probe requests admitted while the
	// circuit is half-open (<=0 uses defaultHalfOpenMaxRequests).
	HalfOpenMaxRequests int
	// SuccessesToClose is the number of consecutive probe successes required to
	// close a recovering circuit (<=0 uses defaultSuccessesToClose).
	SuccessesToClose int
	// ImmediateCircuitOnAccessDenied trips the circuit immediately on the first
	// 403/401 for this domain (Xiaoyuzhou-style). Other domains do not open a
	// domain circuit on a single access-denied response.
	ImmediateCircuitOnAccessDenied bool
	// DomainEvidenceMinDistinctFeeds gates 5xx/timeout/network failures: the
	// circuit only opens once this many distinct feeds fail within
	// EvidenceWindow (<=0 uses defaultDomainEvidenceMinDistinctFeeds).
	DomainEvidenceMinDistinctFeeds int
	// EvidenceWindow is how long a feed's evidence failure counts toward the
	// distinct-feed threshold (<=0 uses defaultEvidenceWindow).
	EvidenceWindow time.Duration
}

type CoordinatorConfig struct {
	DomainPolicies map[string]DomainPolicy
	LastGoodStore  FeedStateStore
	MaxStaleAge    time.Duration
}

type Coordinator struct {
	mu           sync.Mutex
	policies     map[string]DomainPolicy
	inFlight     map[string]*inFlightFetch
	sharedResult map[string]cachedFetch
	semaphores   map[string]chan struct{}
	lastGood     FeedStateStore
	maxStaleAge  time.Duration
	circuits     map[string]*domainCircuit
	// metrics aggregates fetch/circuit/conditional-get/last-good counters for
	// the protected admin diagnostics view. It defaults to the process-wide
	// registry; tests inject an isolated instance via SetMetrics before any
	// fetch so they can assert deterministically.
	metrics *FeedMetrics
	// jitter returns a float in [0,1) used to decorrelate bounded backoff so
	// simultaneous recoveries do not synchronize. Tests override it for
	// deterministic boundary checks.
	jitter func() float64
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
	state            CircuitState
	openUntil        time.Time
	cooldownAttempt  int
	halfOpenInFlight int
	halfOpenSuccess  int
	// evidence maps a canonical feed URL to the time of its most recent
	// 5xx/timeout/network failure within the evidence window. Only distinct
	// feed keys count toward DomainEvidenceMinDistinctFeeds.
	evidence map[string]time.Time
}

// DefaultCoordinatorConfig applies the initial conservative policy only to
// Xiaoyuzhou Feed traffic. Other domains retain their existing parallelism.
// Xiaoyuzhou is the only domain that opens its circuit on a first 403; it
// tolerates a single 5xx/timeout (evidence threshold 1) because it is the
// known-fragile target this coordinator exists to protect.
func DefaultCoordinatorConfig() CoordinatorConfig {
	return CoordinatorConfig{
		DomainPolicies: map[string]DomainPolicy{
			XiaoyuzhouFeedDomain: {
				MaxConcurrency:                  1,
				MinRefreshInterval:              5 * time.Minute,
				MaxJitter:                       2 * time.Second,
				CircuitCooldown:                 defaultCircuitCooldown,
				RetryBackoffInitial:             defaultRetryBackoffInitial,
				RetryBackoffMax:                 defaultRetryBackoffMax,
				HalfOpenMaxRequests:             defaultHalfOpenMaxRequests,
				SuccessesToClose:                defaultSuccessesToClose,
				ImmediateCircuitOnAccessDenied:  true,
				DomainEvidenceMinDistinctFeeds:  1,
				EvidenceWindow:                  defaultEvidenceWindow,
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
		metrics:      SharedFeedMetrics(),
		jitter:       defaultJitter,
	}
}

// SetMetrics injects an isolated counter registry. It is intended for tests
// only and must be called before any fetch; it is not safe to swap the registry
// concurrently with active fetches.
func (c *Coordinator) SetMetrics(metrics *FeedMetrics) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.metrics = metrics
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
// positive concurrency policy. The fetch callback receives the conditional-GET
// validators the Coordinator loaded from the persisted snapshot (empty when no
// validated snapshot exists), so the Fetcher can send If-None-Match /
// If-Modified-Since and let the Coordinator recover a 304.
func (c *Coordinator) Do(ctx context.Context, rawURL string, fetch func(context.Context, RequestValidators) (*FetchResult, error)) (result *FetchResult, err error) {
	if c == nil {
		return fetch(ctx, RequestValidators{})
	}
	if ctx == nil {
		ctx = context.Background()
	}
	// Record exactly one access outcome per Do call — shared-cache hits, fresh
	// local results, coalesced in-flight waits, and live fetches are all
	// distinguishable by their Access.SourceType label. result is nil on the
	// cancelled in-flight path, which records nothing.
	defer func() {
		if result != nil {
			c.metrics.RecordFetch(result.Access)
		}
	}()

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

	result, err = c.run(ctx, rawURL, policy, fetch)
	c.mu.Lock()
	sharedResult := cloneFetchResult(result)
	if err == nil && result != nil && result.Feed != nil && policy.MinRefreshInterval > 0 {
		c.sharedResult[key] = cachedFetch{storedAt: time.Now(), result: sharedResult}
	}
	if err == nil && result != nil && result.Feed != nil {
		// A recovered 304 means the persisted body is unchanged and still
		// current: advance only validated_at rather than rewriting the body,
		// so its retrieval time and fingerprint stay authoritative.
		if result.Access.CacheStatus == CacheStatusNotModified {
			if touchErr := c.touchValidatedAt(key); touchErr != nil {
				logger.Warnf("feed last-good validated_at touch failed for %s: %v", key, touchErr)
			}
		} else {
			c.saveLastGood(key, result)
		}
	}
	delete(c.inFlight, key)
	call.result = sharedResult
	call.err = err
	close(call.done)
	c.mu.Unlock()
	return result, err
}

func circuitPolicyEnabled(policy DomainPolicy) bool {
	return policy.CircuitCooldown > 0 || policy.RetryBackoffInitial > 0 || policy.RetryBackoffMax > 0 ||
		policy.ImmediateCircuitOnAccessDenied || policy.HalfOpenMaxRequests > 0 || policy.SuccessesToClose > 0 ||
		policy.DomainEvidenceMinDistinctFeeds > 0 || policy.EvidenceWindow > 0
}

func (c *Coordinator) reserveCircuitLocked(domain string, policy DomainPolicy) (probe bool, circuitState CircuitState, openUntil time.Time, blocked bool) {
	state := c.circuits[domain]
	if state == nil {
		state = &domainCircuit{state: CircuitStateNotUsed}
		c.circuits[domain] = state
	}
	now := time.Now()
	switch state.state {
	case CircuitStateProbe:
		// Half-open: admit a bounded number of probes, block the rest as OPEN
		// so queued work does not stampede a recovering domain.
		if state.halfOpenInFlight < halfOpenMax(policy) {
			state.halfOpenInFlight++
			c.metrics.RecordRetry(domain)
			return true, CircuitStateProbe, state.openUntil, false
		}
		return false, CircuitStateOpen, state.openUntil, true
	case CircuitStateOpen:
		if state.openUntil.After(now) {
			return false, CircuitStateOpen, state.openUntil, true
		}
		// Cooldown elapsed: enter half-open with a single probe.
		state.state = CircuitStateProbe
		state.halfOpenInFlight = 1
		state.halfOpenSuccess = 0
		c.recordCircuitTransitionLocked(domain, CircuitStateOpen, CircuitStateProbe)
		c.metrics.RecordRetry(domain)
		return true, CircuitStateProbe, state.openUntil, false
	default:
		// NotUsed or Closed: normal admission, not a probe.
		return false, state.state, time.Time{}, false
	}
}

// recordCircuitTransitionLocked counts a domain circuit state transition. It
// must be called while holding c.mu (the metrics registry has its own lock).
func (c *Coordinator) recordCircuitTransitionLocked(domain string, from, to CircuitState) {
	c.metrics.RecordCircuitTransition(domain, from, to)
}

func (c *Coordinator) completeCircuitLocked(domain string, policy DomainPolicy, probe bool, feedKey string, result *FetchResult, err error) {
	state := c.circuits[domain]
	if state == nil {
		return
	}
	succeeded := err == nil && result != nil && result.Feed != nil
	if probe {
		if state.halfOpenInFlight > 0 {
			state.halfOpenInFlight--
		}
		if succeeded {
			state.halfOpenSuccess++
			if state.halfOpenSuccess >= successesToClose(policy) {
				c.recordCircuitTransitionLocked(domain, CircuitStateProbe, CircuitStateClosed)
				c.closeCircuitLocked(state)
			}
			return
		}
		// A probe failure re-opens the circuit with an escalated cooldown so a
		// still-failing domain backs off rather than being re-probed tightly.
		state.cooldownAttempt++
		state.openUntil = time.Now().Add(c.circuitWait(policy, state.cooldownAttempt, result))
		state.state = CircuitStateOpen
		state.halfOpenSuccess = 0
		c.recordCircuitTransitionLocked(domain, CircuitStateProbe, CircuitStateOpen)
		if result != nil {
			result.Access.CircuitState = CircuitStateOpen
		}
		return
	}
	if succeeded {
		return
	}
	if isImmediateCircuitFailure(policy, result) {
		from := state.state
		c.openCircuitLocked(state, policy, 1, result)
		c.recordCircuitTransitionLocked(domain, from, CircuitStateOpen)
		return
	}
	if isEvidenceCircuitFailure(result) {
		if state.evidence == nil {
			state.evidence = make(map[string]time.Time)
		}
		state.evidence[feedKey] = time.Now()
		pruneEvidenceLocked(state, policy)
		if distinctEvidenceLocked(state, policy) >= evidenceMin(policy) {
			from := state.state
			c.openCircuitLocked(state, policy, 1, result)
			c.recordCircuitTransitionLocked(domain, from, CircuitStateOpen)
		}
	}
}

func (c *Coordinator) openCircuitLocked(state *domainCircuit, policy DomainPolicy, attempt int, result *FetchResult) {
	state.cooldownAttempt = attempt
	state.openUntil = time.Now().Add(c.circuitWait(policy, attempt, result))
	state.state = CircuitStateOpen
	state.halfOpenSuccess = 0
	if result != nil {
		result.Access.CircuitState = CircuitStateOpen
	}
}

func (c *Coordinator) closeCircuitLocked(state *domainCircuit) {
	state.state = CircuitStateClosed
	state.openUntil = time.Time{}
	state.cooldownAttempt = 0
	state.halfOpenSuccess = 0
	state.halfOpenInFlight = 0
	state.evidence = nil
}

// isImmediateCircuitFailure reports failures that OPEN the circuit without any
// evidence threshold: rate limits always, and access-denied only for domains
// that opted into ImmediateCircuitOnAccessDenied (Xiaoyuzhou).
func isImmediateCircuitFailure(policy DomainPolicy, result *FetchResult) bool {
	if result == nil || result.Access.HTTPStatus == nil {
		return false
	}
	status := *result.Access.HTTPStatus
	if status == http.StatusTooManyRequests {
		return true
	}
	if status == http.StatusForbidden && policy.ImmediateCircuitOnAccessDenied {
		return true
	}
	return false
}

// isEvidenceCircuitFailure reports 5xx/timeout/network failures that only OPEN
// the circuit once enough distinct feeds have failed (domain-health signal).
func isEvidenceCircuitFailure(result *FetchResult) bool {
	if result == nil {
		return false
	}
	switch result.Access.ErrorCategory {
	case ErrorCategoryServiceUnavailable, ErrorCategoryTimeout, ErrorCategoryNetwork:
		return true
	}
	if result.Access.HTTPStatus != nil && *result.Access.HTTPStatus >= 500 {
		return true
	}
	return false
}

func halfOpenMax(policy DomainPolicy) int {
	if policy.HalfOpenMaxRequests > 0 {
		return policy.HalfOpenMaxRequests
	}
	return defaultHalfOpenMaxRequests
}

func successesToClose(policy DomainPolicy) int {
	if policy.SuccessesToClose > 0 {
		return policy.SuccessesToClose
	}
	return defaultSuccessesToClose
}

func evidenceMin(policy DomainPolicy) int {
	if policy.DomainEvidenceMinDistinctFeeds > 0 {
		return policy.DomainEvidenceMinDistinctFeeds
	}
	return defaultDomainEvidenceMinDistinctFeeds
}

func evidenceWindow(policy DomainPolicy) time.Duration {
	if policy.EvidenceWindow > 0 {
		return policy.EvidenceWindow
	}
	return defaultEvidenceWindow
}

func pruneEvidenceLocked(state *domainCircuit, policy DomainPolicy) {
	cutoff := time.Now().Add(-evidenceWindow(policy))
	for feedKey, at := range state.evidence {
		if at.Before(cutoff) {
			delete(state.evidence, feedKey)
		}
	}
}

func distinctEvidenceLocked(state *domainCircuit, policy DomainPolicy) int {
	if state.evidence == nil {
		return 0
	}
	cutoff := time.Now().Add(-evidenceWindow(policy))
	count := 0
	for _, at := range state.evidence {
		if !at.Before(cutoff) {
			count++
		}
	}
	return count
}

// circuitWait computes the OPEN cooldown for the next attempt. Forbidden
// responses use the fixed policy cooldown; rate limits honor Retry-After
// (capped); everything else uses a bounded exponential with equal jitter so
// simultaneous recoveries decorrelate without ever producing a zero cooldown.
func (c *Coordinator) circuitWait(policy DomainPolicy, failureCount int, result *FetchResult) time.Duration {
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
	return boundedBackoff(initial, maximum, failureCount, c.jitter)
}

// boundedBackoff returns an equal-jitter cooldown in [det/2, det], where det is
// the exponential backoff capped at maximum. Equal jitter (half deterministic,
// half random) decorrelates simultaneous recoveries while guaranteeing a
// non-trivial cooldown floor.
func boundedBackoff(initial, maximum time.Duration, attempt int, jitter func() float64) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	det := initial
	for i := 1; i < attempt && det < maximum; i++ {
		det *= 2
		if det >= maximum {
			det = maximum
			break
		}
	}
	if det > maximum {
		det = maximum
	}
	if jitter == nil {
		return det
	}
	random := jitter()
	if random < 0 {
		random = 0
	} else if random >= 1 {
		return det
	}
	return det/2 + time.Duration(random*float64(det)/2)
}

func defaultJitter() float64 {
	return rand.Float64()
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
	snapshot, ok, err := c.lastGood.Load(key)
	if err != nil {
		// A persistence/corruption error must never be disguised as a clean
		// miss: log it but fall back honestly rather than serving a broken or
		// unvalidated snapshot as fresh.
		logger.Warnf("feed last-good load failed for %s: %v", key, err)
		return nil, false
	}
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

	if c.metrics != nil {
		c.metrics.RecordLastGoodHit()
	}
	return &FetchResult{
		Feed:       parsed,
		RawContent: append([]byte(nil), snapshot.RawContent...),
		Access:     access,
	}, true
}

func (c *Coordinator) run(ctx context.Context, rawURL string, policy DomainPolicy, fetch func(context.Context, RequestValidators) (*FetchResult, error)) (result *FetchResult, err error) {
	domain := TargetDomain(rawURL)
	feedKey := CanonicalizeURL(rawURL)
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
			c.completeCircuitLocked(domain, policy, probe, feedKey, result, err)
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
		openUntil, blocked := c.circuitBlockedLocked(domain, policy)
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
		probe, circuitState, openUntil, blocked = c.reserveCircuitLocked(domain, policy)
		if blocked {
			c.mu.Unlock()
			return circuitOpenResult(rawURL, openUntil), ErrFeedCircuitOpen
		}
		reservedCircuit = true
		c.mu.Unlock()
	}

	validators := c.loadConditionalValidators(feedKey)
	result, err = fetch(ctx, validators)
	if result != nil && result.Access.CacheStatus == CacheStatusNotModified {
		// 304 Not Modified is a successful conditional check, not a failure:
		// recover the Feed from the same persisted snapshot we sent validators
		// for. If the snapshot vanished between Load and the 304 (eviction or
		// restart race), perform one unconditional GET in the same budget so we
		// never return a nil Feed or an empty increment.
		if recovered, ok := c.recoverNotModified(rawURL, feedKey, result); ok {
			result = recovered
			err = nil
		} else {
			logger.Warnf("feed 304 unrecoverable for %s: snapshot missing/corrupt, falling back to one unconditional GET", feedKey)
			result, err = fetch(ctx, RequestValidators{})
		}
	}
	// Record the conditional-GET outcome BEFORE the CacheStatus=Miss stamp
	// below, so a recovered 304 still classifies as "304" rather than "miss".
	c.recordConditionalGet(validators, result)
	if result != nil && err == nil && policy.MinRefreshInterval > 0 && result.Access.CacheStatus != CacheStatusNotModified {
		result.Access.CacheStatus = CacheStatusMiss
	}
	return result, err
}

// recordConditionalGet classifies the conditional-GET outcome for metrics:
// "miss" when no validated snapshot existed (request was unconditional), "304"
// when validators were sent and the server confirmed Not Modified, "200" when
// validators were sent but the content changed and was returned in full.
func (c *Coordinator) recordConditionalGet(validators RequestValidators, result *FetchResult) {
	if c.metrics == nil {
		return
	}
	if !validators.Present() {
		c.metrics.RecordConditionalGet(conditionalGetResultMiss)
		return
	}
	if result != nil && result.Access.CacheStatus == CacheStatusNotModified {
		c.metrics.RecordConditionalGet(conditionalGetResult304)
		return
	}
	c.metrics.RecordConditionalGet(conditionalGetResult200)
}

func (c *Coordinator) circuitBlockedLocked(domain string, policy DomainPolicy) (time.Time, bool) {
	state := c.circuits[domain]
	if state == nil {
		return time.Time{}, false
	}
	now := time.Now()
	if state.state == CircuitStateOpen && state.openUntil.After(now) {
		return state.openUntil, true
	}
	if state.state == CircuitStateProbe && state.halfOpenInFlight >= halfOpenMax(policy) {
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
		FeedURL:      key,
		RawContent:   result.RawContent,
		ETag:         result.Access.ETag,
		LastModified: result.Access.LastModified,
	}
	if result.Access.RetrievedAt != nil {
		snapshot.RetrievedAt = *result.Access.RetrievedAt
	}
	if snapshot.RetrievedAt.IsZero() {
		snapshot.RetrievedAt = time.Now()
	}
	if err := c.lastGood.Save(snapshot); err != nil {
		// Durability failure: the snapshot is unavailable for future restart
		// recovery but the live fetch still succeeded, so log and continue.
		if errors.Is(err, ErrSnapshotNotPersisted) {
			logger.Warnf("feed last-good not persisted for %s: %v", key, err)
		} else {
			logger.Warnf("feed last-good save failed for %s: %v", key, err)
		}
	}
}

// loadConditionalValidators returns the ETag/Last-Modified validators for the
// canonical feed URL, but only when the persisted snapshot passes fingerprint
// validation (Load already checks this) AND its body parses. That guarantees a
// later 304 can always be recovered from the same snapshot row.
func (c *Coordinator) loadConditionalValidators(key string) RequestValidators {
	if c.lastGood == nil {
		return RequestValidators{}
	}
	snapshot, ok, err := c.lastGood.Load(key)
	if err != nil || !ok || snapshot == nil {
		return RequestValidators{}
	}
	if _, parseErr := parseSnapshot(snapshot); parseErr != nil {
		return RequestValidators{}
	}
	return RequestValidators{
		IfNoneMatch:     snapshot.ETag,
		IfModifiedSince: snapshot.LastModified,
	}
}

// recoverNotModified resolves a 304 by serving the persisted snapshot captured
// with the validators we sent. The recovered outcome keeps HTTPStatus=304,
// marks CacheStatus=not_modified, preserves the snapshot's original
// RetrievedAt (the body did not change), and carries no failure, so it counts
// as a success for circuit and retry accounting. Returns (nil, false) when the
// snapshot is missing, corrupt, or unparseable so the caller falls back to a
// single unconditional GET.
func (c *Coordinator) recoverNotModified(rawURL, key string, probed *FetchResult) (*FetchResult, bool) {
	if c.lastGood == nil {
		return nil, false
	}
	snapshot, ok, err := c.lastGood.Load(key)
	if err != nil || !ok || snapshot == nil {
		return nil, false
	}
	parsed, err := parseSnapshot(snapshot)
	if err != nil {
		return nil, false
	}
	access := probed.Access
	access.SourceType = AccessSourcePrimary
	access.CacheStatus = CacheStatusNotModified
	access.Freshness = FreshnessFresh
	access.ErrorCategory = ErrorCategoryNone
	access.ResponseBytes = 0
	// Preserve the body's original retrieval time: a 304 confirms the content
	// is unchanged, so RetrievedAt must not advance (that would fake freshness).
	access.RetrievedAt = snapshotTime(snapshot)
	return &FetchResult{
		Feed:       parsed,
		RawContent: append([]byte(nil), snapshot.RawContent...),
		Access:     access,
	}, true
}

func (c *Coordinator) touchValidatedAt(key string) error {
	if c.lastGood == nil {
		return nil
	}
	return c.lastGood.TouchValidatedAt(key)
}

// UsePersistentLastGood upgrades the in-process last-good store to a tiered
// (memory L1 + durable L2) store. It is intended to be called once during
// startup, before any Feed fetch, when the application database is available.
// Snapshots already cached in the prior store are best-effort: at startup the
// process store is empty, so there is nothing to drain.
func (c *Coordinator) UsePersistentLastGood(store FeedStateStore) {
	if c == nil || store == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastGood = NewTieredSnapshotStore(store, LastGoodStoreConfig{})
}

// CircuitSnapshot returns the live state of every domain circuit that has been
// used (open/probe/closed). Unused circuits are omitted. It is read under the
// coordinator lock and surfaces only domain + state + (when OPEN) the cooldown
// end instant — no feed URLs, request counts, or bodies.
func (c *Coordinator) CircuitSnapshot() []CircuitStateRow {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	rows := make([]CircuitStateRow, 0, len(c.circuits))
	now := time.Now()
	for domain, state := range c.circuits {
		if state == nil || state.state == CircuitStateNotUsed {
			continue
		}
		row := CircuitStateRow{Domain: domain, State: string(state.state)}
		if state.state == CircuitStateOpen && state.openUntil.After(now) {
			openUntil := state.openUntil.UTC().Format(time.RFC3339)
			row.OpenUntil = &openUntil
		}
		rows = append(rows, row)
	}
	return rows
}

// LastGoodStats returns capacity/durability counters for the last-good store:
// entry count, total bytes, evictions, and write failures. It surfaces no feed
// URLs, bodies, cookies, or credentials.
func (c *Coordinator) LastGoodStats() (SnapshotStoreStats, error) {
	if c == nil {
		return SnapshotStoreStats{}, nil
	}
	c.mu.Lock()
	store := c.lastGood
	c.mu.Unlock()
	if store == nil {
		return SnapshotStoreStats{}, nil
	}
	return store.Stats()
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
	snapshot, ok, err := c.lastGood.Load(key)
	if err != nil {
		logger.Warnf("feed last-good load failed for %s: %v", key, err)
		return nil, false
	}
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
