package feed

import (
	"context"
	"fmt"
	"math/rand"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/mmcdole/gofeed"
)

const XiaoyuzhouFeedDomain = "feed.xyzfm.space"

// DomainPolicy controls only load-shaping behavior for one target domain.
// MaxConcurrency <= 0 means unlimited; duplicate-request coalescing remains
// active for every domain as a shared correctness rule.
type DomainPolicy struct {
	MaxConcurrency     int
	MinRefreshInterval time.Duration
	MaxJitter          time.Duration
}

type CoordinatorConfig struct {
	DomainPolicies map[string]DomainPolicy
}

type Coordinator struct {
	mu           sync.Mutex
	policies     map[string]DomainPolicy
	inFlight     map[string]*inFlightFetch
	sharedResult map[string]cachedFetch
	semaphores   map[string]chan struct{}
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

// DefaultCoordinatorConfig applies the initial conservative policy only to
// Xiaoyuzhou Feed traffic. Other domains retain their existing parallelism.
func DefaultCoordinatorConfig() CoordinatorConfig {
	return CoordinatorConfig{
		DomainPolicies: map[string]DomainPolicy{
			XiaoyuzhouFeedDomain: {
				MaxConcurrency:     1,
				MinRefreshInterval: 5 * time.Minute,
				MaxJitter:          2 * time.Second,
			},
		},
	}
}

func NewCoordinator(config CoordinatorConfig) *Coordinator {
	policies := make(map[string]DomainPolicy, len(config.DomainPolicies))
	for domain, policy := range config.DomainPolicies {
		policies[strings.ToLower(strings.TrimSpace(domain))] = policy
	}
	return &Coordinator{
		policies:     policies,
		inFlight:     make(map[string]*inFlightFetch),
		sharedResult: make(map[string]cachedFetch),
		semaphores:   make(map[string]chan struct{}),
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
	policy := c.policyFor(TargetDomain(rawURL))

	c.mu.Lock()
	if cached, ok := c.sharedResult[key]; ok && policy.MinRefreshInterval > 0 && time.Since(cached.storedAt) < policy.MinRefreshInterval {
		result := cloneFetchResult(cached.result)
		c.mu.Unlock()
		return markSharedResult(result), nil
	}
	if call, ok := c.inFlight[key]; ok {
		c.mu.Unlock()
		select {
		case <-call.done:
			if call.err != nil {
				return nil, call.err
			}
			return markSharedResult(cloneFetchResult(call.result)), nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	call := &inFlightFetch{done: make(chan struct{})}
	c.inFlight[key] = call
	c.mu.Unlock()

	result, err := c.run(ctx, rawURL, policy, fetch)
	c.mu.Lock()
	if err == nil && result != nil && result.Feed != nil && policy.MinRefreshInterval > 0 {
		c.sharedResult[key] = cachedFetch{storedAt: time.Now(), result: cloneFetchResult(result)}
	}
	delete(c.inFlight, key)
	call.result = result
	call.err = err
	close(call.done)
	c.mu.Unlock()
	return result, err
}

func (c *Coordinator) run(ctx context.Context, rawURL string, policy DomainPolicy, fetch func(context.Context) (*FetchResult, error)) (result *FetchResult, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			result = nil
			err = fmt.Errorf("feed access coordinator panic: %v", recovered)
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

	result, err = fetch(ctx)
	if result != nil && err == nil && policy.MinRefreshInterval > 0 {
		result.Access.CacheStatus = CacheStatusMiss
	}
	return result, err
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
