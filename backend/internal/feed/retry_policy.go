package feed

import (
	"context"
	"errors"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DefaultRetryBudget is the outer-retry budget (max retries after the first
// attempt) used when FeedConfig.Retry.Budget is unset, preserving the
// previously validated behavior (the legacy DefaultRetryConfig.MaxRetries = 3).
const DefaultRetryBudget = 3

// MaxRetryAfter caps any Retry-After wait so a hostile or misconfigured
// upstream cannot stall the workflow indefinitely. It also caps the full-jitter
// backoff ceiling so no single retry sleeps longer than this.
const MaxRetryAfter = 60 * time.Second

// DefaultRetryBase and DefaultRetryMax are the full-jitter backoff parameters
// mirroring the previously validated legacy retry tuning (2s base, 8s cap).
const (
	DefaultRetryBase = 2 * time.Second
	DefaultRetryMax  = 8 * time.Second
	// DefaultRetryConcurrentPerDomain keeps retry pressure bounded even when
	// the caller has many workers and the domain has no ordinary fetch policy.
	DefaultRetryConcurrentPerDomain = 1
)

// RetryAdmission limits active retry requests independently per target domain.
// It is deliberately separate from the normal Feed concurrency semaphore:
// ordinary fetch policy may be unset for a domain, while retry pressure must
// still have a finite bound. The admission is held only around the retrying
// Fetch call, never across the backoff sleep.
type RetryAdmission struct {
	mu    sync.Mutex
	max   int
	slots map[string]chan struct{}
}

// NewRetryAdmission constructs a per-domain retry admission. Non-positive
// values use the conservative default rather than disabling the bound.
func NewRetryAdmission(maxPerDomain int) *RetryAdmission {
	if maxPerDomain <= 0 {
		maxPerDomain = DefaultRetryConcurrentPerDomain
	}
	return &RetryAdmission{max: maxPerDomain, slots: make(map[string]chan struct{})}
}

// Acquire reserves one retry slot for domain. The caller must invoke the
// returned release function after the retry request completes. A cancelled
// context does not consume a slot and returns admitted=false.
func (a *RetryAdmission) Acquire(ctx context.Context, domain string) (release func(), admitted bool) {
	if a == nil {
		return func() {}, true
	}
	if ctx == nil {
		ctx = context.Background()
	}
	domain = strings.ToLower(strings.TrimSpace(domain))
	if domain == "" {
		domain = "<unknown>"
	}

	a.mu.Lock()
	slot, ok := a.slots[domain]
	if !ok {
		slot = make(chan struct{}, a.max)
		a.slots[domain] = slot
	}
	a.mu.Unlock()

	select {
	case slot <- struct{}{}:
		return func() { <-slot }, true
	case <-ctx.Done():
		return nil, false
	}
}

// Sleeper abstracts the wait between retry attempts so tests run deterministically
// without real sleeping. Production code uses realSleeper.
type Sleeper interface {
	Sleep(d time.Duration)
}

// realSleeper sleeps via time.Sleep and ignores non-positive durations.
type realSleeper struct{}

func (realSleeper) Sleep(d time.Duration) {
	if d > 0 {
		time.Sleep(d)
	}
}

// FakeSleeper is a test double that records every sleep it was asked to perform
// without actually waiting. It is safe for concurrent use.
type FakeSleeper struct {
	mu     sync.Mutex
	delays []time.Duration
}

// Sleep records the requested delay.
func (f *FakeSleeper) Sleep(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.delays = append(f.delays, d)
}

// Delays returns a copy of the recorded sleep sequence.
func (f *FakeSleeper) Delays() []time.Duration {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]time.Duration, len(f.delays))
	copy(out, f.delays)
	return out
}

// RetryPolicy is the SINGLE outer-retry policy layered above the Coordinator.
// Both sync retry loops build it via SharedRetryPolicy (or an explicit value
// for tests) and must not reimplement classification, Retry-After parsing, or
// backoff. Retries always re-enter through the Fetcher/Coordinator, so circuit,
// per-domain concurrency, dedup, and fallback semantics are never bypassed.
//
// There is intentionally no infinite-retry path: Budget is a hard, finite
// ceiling (additionally bounded by maxRetryBudget at config-validation time),
// and every wait is capped.
type RetryPolicy struct {
	// Budget is the max number of retries after the first attempt (>=0).
	Budget int
	// Base is the exponential backoff base; the full-jitter floor is 0.
	Base time.Duration
	// Max caps both the full-jitter backoff ceiling and any Retry-After wait.
	Max time.Duration
	// Now returns the current time; nil => time.Now. Used for HTTP-date parsing.
	Now func() time.Time
	// Rand returns a float in [0,1); nil => a process-wide locked math/rand
	// source. Jitter does not need cryptographic strength; it only spreads
	// concurrent retries to avoid a self-inflicted thundering herd.
	Rand func() float64
	// Sleeper abstracts the wait; nil => realSleeper.
	Sleeper Sleeper
	// Admission bounds active retry requests per target domain. A nil value
	// falls back to the process-wide conservative admission.
	Admission *RetryAdmission
}

// SharedRetryPolicy returns the policy derived from the startup-loaded
// FeedConfig. Budget defaults to DefaultRetryBudget when unset (<=0); a
// configured value is already hard-capped at maxRetryBudget by FeedConfig
// validation. Base mirrors the configured jitter (falling back to
// DefaultRetryBase). There is no hot reload.
func SharedRetryPolicy() RetryPolicy {
	rc := SharedRetryConfig()
	budget := rc.Budget
	if budget <= 0 {
		budget = DefaultRetryBudget
	}
	base := DefaultRetryBase
	if rc.Jitter > 0 {
		base = rc.Jitter
	}
	return RetryPolicy{
		Budget:    budget,
		Base:      base,
		Max:       DefaultRetryMax,
		Admission: SharedRetryAdmission(),
	}
}

var sharedRetryAdmission = NewRetryAdmission(DefaultRetryConcurrentPerDomain)

// SharedRetryAdmission is the process-wide admission used by normal sync
// services. It lets separately constructed services still share the same
// per-domain retry bound.
func SharedRetryAdmission() *RetryAdmission { return sharedRetryAdmission }

// WithSleeper returns a copy with the sleeper injected (for deterministic tests).
func (p RetryPolicy) WithSleeper(s Sleeper) RetryPolicy { p.Sleeper = s; return p }

// WithRand returns a copy with a fixed random source injected (for deterministic
// full-jitter tests).
func (p RetryPolicy) WithRand(r func() float64) RetryPolicy { p.Rand = r; return p }

// WithNow returns a copy with a fixed clock injected (for HTTP-date tests).
func (p RetryPolicy) WithNow(n func() time.Time) RetryPolicy { p.Now = n; return p }

// WithAdmission returns a copy with an isolated retry admission, primarily for
// deterministic tests or a separately scoped workflow service.
func (p RetryPolicy) WithAdmission(admission *RetryAdmission) RetryPolicy {
	p.Admission = admission
	return p
}

// AcquireRetry reserves the per-domain retry slot for one retrying Fetch call.
func (p RetryPolicy) AcquireRetry(ctx context.Context, domain string) (func(), bool) {
	admission := p.Admission
	if admission == nil {
		admission = SharedRetryAdmission()
	}
	if ctx == nil {
		ctx = context.Background()
	}
	// The two legacy sync loops do not carry a request context. Give their
	// admission wait the same finite upper bound as Retry-After so a saturated
	// domain can never turn a bounded retry budget into an unbounded worker wait.
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		waitCtx, cancel := context.WithTimeout(ctx, MaxRetryAfter)
		defer cancel()
		return admission.Acquire(waitCtx, domain)
	}
	return admission.Acquire(ctx, domain)
}

func (p RetryPolicy) sleeper() Sleeper {
	if p.Sleeper != nil {
		return p.Sleeper
	}
	return realSleeper{}
}

func (p RetryPolicy) now() time.Time {
	if p.Now != nil {
		return p.Now()
	}
	return time.Now()
}

func (p RetryPolicy) rand() float64 {
	if p.Rand != nil {
		return p.Rand()
	}
	return defaultRetryRand()
}

// Sleep waits via the policy's sleeper (real or fake). No-op for d<=0 so a
// zero-delay retry (e.g. Retry-After: 0) does not park the worker.
func (p RetryPolicy) Sleep(d time.Duration) { p.sleeper().Sleep(d) }

// ShouldRetry reports whether err is a retryable Feed error (network, timeout,
// 429, 5xx). Access-denied (403/401), not-found (404), payment-required (402),
// parse failures, and redirect-policy rejections never retry. It is the single
// classification the outer loops use.
func (p RetryPolicy) ShouldRetry(err error) bool { return IsRetryable(err) }

// CategoryOf returns the FeedErrorType for an error (ErrorTypeUnknown when it is
// not a FeedError), so structured retry logs can report the category without
// re-deriving classification.
func CategoryOf(err error) FeedErrorType {
	var fe *FeedError
	if errors.As(err, &fe) {
		return fe.Type
	}
	return ErrorTypeUnknown
}

// RetryAfterOf returns the upstream Retry-After string carried by a FeedError
// (empty when absent or when the error is not a FeedError).
func RetryAfterOf(err error) string {
	var fe *FeedError
	if errors.As(err, &fe) {
		return fe.RetryAfter
	}
	return ""
}

// NextDelay returns the wait before the next attempt for a retryable error and
// whether the error is retryable at all.
//
// When the error is rate-limited or service-unavailable AND carries a parseable
// Retry-After (delta-seconds OR HTTP-date), that value wins. Otherwise the wait
// is a full-jitter exponential backoff that grows with the attempt index. Both
// paths are capped at p.Max and at MaxRetryAfter. attempt is the 0-based index
// of the attempt that just failed, so the first retry (attempt 0) uses the base
// ceiling.
func (p RetryPolicy) NextDelay(err error, attempt int) (time.Duration, bool) {
	if !p.ShouldRetry(err) {
		return 0, false
	}
	var fe *FeedError
	if errors.As(err, &fe) && (fe.Type == ErrorTypeRateLimited || fe.Type == ErrorTypeServiceUnavailable) {
		if d, ok := ParseRetryAfter(fe.RetryAfter, p.now()); ok {
			return capDelay(d), true
		}
	}
	return capDelay(fullJitter(p.Base, p.Max, attempt, p.rand)), true
}

// fullJitter returns a delay in [0, ceiling] where ceiling = min(Base * 2^attempt, Max).
// Full jitter (scaling the whole exponential by a uniform random) is the AWS
// recommended variant: it both bounds the wait and spreads concurrent retries.
func fullJitter(base, max time.Duration, attempt int, rnd func() float64) time.Duration {
	if base <= 0 {
		return 0
	}
	ceiling := base
	for i := 0; i < attempt && ceiling < max; i++ {
		ceiling *= 2
		if ceiling > max || ceiling <= 0 {
			ceiling = max
			break
		}
	}
	if ceiling > max {
		ceiling = max
	}
	if ceiling <= 0 {
		return 0
	}
	return time.Duration(float64(ceiling) * rnd())
}

// capDelay clamps a retry wait to [0, MaxRetryAfter]. p.Max is enforced inside
// NextDelay via the backoff ceiling and the Retry-After parse; this global cap
// is the final safety net against any path that could exceed it.
func capDelay(d time.Duration) time.Duration {
	if d < 0 {
		return 0
	}
	if d > MaxRetryAfter {
		return MaxRetryAfter
	}
	return d
}

// ParseRetryAfter parses a Retry-After header value as either delta-seconds or
// an HTTP-date, relative to now. ok is false when the value is absent, negative,
// unparseable, or an HTTP-date already in the past.
func ParseRetryAfter(raw string, now time.Time) (time.Duration, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false
	}
	// delta-seconds (RFC 7231): a non-negative integer.
	if secs, err := strconv.Atoi(raw); err == nil {
		if secs < 0 {
			return 0, false
		}
		return time.Duration(secs) * time.Second, true
	}
	// HTTP-date: the three forms net/http accepts.
	for _, layout := range []string{
		"Mon, 02 Jan 2006 15:04:05 GMT",
		"Monday, 02-Jan-06 15:04:05 GMT",
		"Mon Jan _2 15:04:05 2006",
	} {
		if t, err := time.Parse(layout, raw); err == nil {
			d := t.Sub(now)
			if d < 0 {
				return 0, false
			}
			return d, true
		}
	}
	return 0, false
}

// defaultRetryRand is a process-wide math/rand source guarded by a mutex. The
// fixed seed is intentional: jitter exists to spread concurrent retries, and
// that spreading comes from many interleaved draws across feeds, not from seed
// entropy. Tests inject Rand explicitly and never touch this source.
var (
	defaultRetryRandMu  sync.Mutex
	defaultRetryRandSrc = rand.New(rand.NewSource(1))
)

func defaultRetryRand() float64 {
	defaultRetryRandMu.Lock()
	defer defaultRetryRandMu.Unlock()
	return defaultRetryRandSrc.Float64()
}
