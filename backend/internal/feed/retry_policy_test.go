package feed

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// fixedNow is the deterministic clock used by HTTP-date Retry-After tests.
// 2026-07-21 12:00:00 UTC.
var fixedNow = time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)

func TestParseRetryAfterDeltaSeconds(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want time.Duration
		ok   bool
	}{
		{"plain seconds", "30", 30 * time.Second, true},
		{"zero is valid", "0", 0, true},
		{"large seconds", "600", 600 * time.Second, true},
		{"negative rejected", "-5", 0, false},
		{"empty rejected", "", 0, false},
		{"whitespace only", "   ", 0, false},
		{"garbage rejected", "soon", 0, false},
		{"float rejected", "1.5", 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, ok := ParseRetryAfter(tc.raw, fixedNow)
			assert.Equal(t, tc.ok, ok)
			assert.Equal(t, tc.want, d)
		})
	}
}

func TestParseRetryAfterHTTPDate(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want time.Duration
		ok   bool
	}{
		// 30s in the future, RFC 1123 (the form net/http emits).
		{"future IMF-fixdate", "Mon, 21 Jul 2026 12:00:30 GMT", 30 * time.Second, true},
		// 1h in the future.
		{"future one hour", "Mon, 21 Jul 2026 13:00:00 GMT", time.Hour, true},
		// Already in the past: rejected.
		{"past date rejected", "Mon, 21 Jul 2026 11:59:59 GMT", 0, false},
		// RFC 850 variant.
		{"future RFC850", "Monday, 21-Jul-26 12:01:00 GMT", time.Minute, true},
		// asctime variant.
		{"future asctime", "Tue Jul 22 12:00:00 2026", 24 * time.Hour, true},
		// Unparseable date falls through to the seconds branch (fails) then the
		// date layouts (fails) → ok=false.
		{"garbage date rejected", "Someday, soon GMT", 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, ok := ParseRetryAfter(tc.raw, fixedNow)
			assert.Equal(t, tc.ok, ok, "raw=%q", tc.raw)
			if ok {
				assert.InDelta(t, tc.want, d, float64(2*time.Second))
			}
		})
	}
}

func TestFullJitterBoundsAndCeiling(t *testing.T) {
	base := 2 * time.Second
	max := 8 * time.Second

	// rand=0 → always zero delay (full jitter floor).
	for attempt := 0; attempt < 6; attempt++ {
		d := fullJitter(base, max, attempt, func() float64 { return 0 })
		assert.Equal(t, time.Duration(0), d, "attempt %d rand=0", attempt)
	}

	// rand=1 → the full ceiling: base*2^attempt, capped at max.
	expected := []time.Duration{2 * time.Second, 4 * time.Second, 8 * time.Second, 8 * time.Second, 8 * time.Second}
	for attempt, want := range expected {
		d := fullJitter(base, max, attempt, func() float64 { return 1 })
		assert.Equal(t, want, d, "attempt %d rand=1", attempt)
	}

	// rand=0.5 → half the ceiling at attempt 0.
	d := fullJitter(base, max, 0, func() float64 { return 0.5 })
	assert.Equal(t, time.Second, d)

	// base<=0 short-circuits to zero regardless of randomness.
	assert.Equal(t, time.Duration(0), fullJitter(0, max, 3, func() float64 { return 1 }))
}

func TestNextDelayRetryAfterWinsForRateLimited(t *testing.T) {
	policy := RetryPolicy{Budget: 3, Base: 2 * time.Second, Max: 8 * time.Second, Now: func() time.Time { return fixedNow }}

	err := &FeedError{Type: ErrorTypeRateLimited, RetryAfter: "30"}
	d, ok := policy.NextDelay(err, 0)
	assert.True(t, ok)
	assert.Equal(t, 30*time.Second, d)
}

func TestNextDelayRetryAfterWinsForServiceUnavailable(t *testing.T) {
	policy := RetryPolicy{Budget: 3, Base: 2 * time.Second, Max: 8 * time.Second, Now: func() time.Time { return fixedNow }}

	err := &FeedError{Type: ErrorTypeServiceUnavailable, RetryAfter: "10"}
	d, ok := policy.NextDelay(err, 0)
	assert.True(t, ok)
	assert.Equal(t, 10*time.Second, d)
}

func TestNextDelayRetryAfterIgnoredForNetworkError(t *testing.T) {
	// A network error carries no Retry-After semantics even if the field is set;
	// the wait must come from full-jitter backoff, not the header.
	policy := RetryPolicy{Budget: 3, Base: 2 * time.Second, Max: 8 * time.Second, Rand: func() float64 { return 1 }}

	err := &FeedError{Type: ErrorTypeNetworkError, RetryAfter: "30"}
	d, ok := policy.NextDelay(err, 0)
	assert.True(t, ok)
	// rand=1, attempt 0 → full base ceiling (2s), not the Retry-After 30s.
	assert.Equal(t, 2*time.Second, d)
}

func TestNextDelayRetryAfterCappedAtMaxRetryAfter(t *testing.T) {
	policy := RetryPolicy{Budget: 3, Base: 2 * time.Second, Max: 8 * time.Second, Now: func() time.Time { return fixedNow }}

	// Hostile upstream asks for 9999s; the cap (MaxRetryAfter=60s) must win.
	err := &FeedError{Type: ErrorTypeRateLimited, RetryAfter: "9999"}
	d, ok := policy.NextDelay(err, 0)
	assert.True(t, ok)
	assert.Equal(t, MaxRetryAfter, d)
}

func TestNextDelayUnparseableRetryAfterFallsBackToJitter(t *testing.T) {
	policy := RetryPolicy{Budget: 3, Base: 2 * time.Second, Max: 8 * time.Second, Rand: func() float64 { return 0.5 }}

	err := &FeedError{Type: ErrorTypeRateLimited, RetryAfter: "not-a-date"}
	d, ok := policy.NextDelay(err, 2)
	assert.True(t, ok)
	// attempt 2 ceiling = min(2s*2^2, 8s) = 8s; rand=0.5 → 4s.
	assert.Equal(t, 4*time.Second, d)
}

func TestNextDelayNonRetryableReturnsFalse(t *testing.T) {
	policy := RetryPolicy{Budget: 3, Base: 2 * time.Second, Max: 8 * time.Second}

	for _, et := range []FeedErrorType{
		ErrorTypePaymentRequired,
		ErrorTypeCertificateExpired,
		ErrorTypeNotFound,
		ErrorTypeAccessDenied,
		ErrorTypeGeoBlocked,
		ErrorTypeInvalidFeed,
		ErrorTypeInvalidRequest,
	} {
		d, ok := policy.NextDelay(&FeedError{Type: et}, 0)
		assert.False(t, ok, "type %d should not retry", et)
		assert.Equal(t, time.Duration(0), d)
	}
}

func TestShouldRetryClassification(t *testing.T) {
	policy := RetryPolicy{}
	retryable := []FeedErrorType{ErrorTypeNetworkError, ErrorTypeTimeout, ErrorTypeRateLimited, ErrorTypeServiceUnavailable}
	notRetryable := []FeedErrorType{
		ErrorTypePaymentRequired, ErrorTypeCertificateExpired, ErrorTypeNotFound,
		ErrorTypeAccessDenied, ErrorTypeGeoBlocked, ErrorTypeInvalidFeed, ErrorTypeInvalidRequest, ErrorTypeUnknown,
	}
	for _, et := range retryable {
		assert.True(t, policy.ShouldRetry(&FeedError{Type: et}), "type %d should retry", et)
	}
	for _, et := range notRetryable {
		assert.False(t, policy.ShouldRetry(&FeedError{Type: et}), "type %d should not retry", et)
	}
	// A plain non-FeedError never retries.
	assert.False(t, policy.ShouldRetry(errors.New("boom")))
}

func TestCategoryOfAndRetryAfterOf(t *testing.T) {
	fe := &FeedError{Type: ErrorTypeRateLimited, RetryAfter: "12"}
	assert.Equal(t, ErrorTypeRateLimited, CategoryOf(fe))
	assert.Equal(t, "12", RetryAfterOf(fe))

	// Wrapped FeedError is still recognized.
	wrapped := fmtErrorWrap(fe)
	assert.Equal(t, ErrorTypeRateLimited, CategoryOf(wrapped))
	assert.Equal(t, "12", RetryAfterOf(wrapped))

	// Non-FeedError defaults.
	plain := errors.New("nope")
	assert.Equal(t, ErrorTypeUnknown, CategoryOf(plain))
	assert.Equal(t, "", RetryAfterOf(plain))
}

func TestCapDelay(t *testing.T) {
	assert.Equal(t, time.Duration(0), capDelay(-time.Second))
	assert.Equal(t, time.Duration(0), capDelay(0))
	assert.Equal(t, 5*time.Second, capDelay(5*time.Second))
	assert.Equal(t, MaxRetryAfter, capDelay(MaxRetryAfter))
	assert.Equal(t, MaxRetryAfter, capDelay(MaxRetryAfter+time.Hour))
}

func TestSharedRetryPolicyDefaultsBudget(t *testing.T) {
	// SharedRetryPolicy derives from the startup-loaded FeedConfig. With the
	// default (unset) retry budget, Budget must fall back to DefaultRetryBudget
	// so the validated legacy MaxRetries=3 behavior is preserved.
	policy := SharedRetryPolicy()
	assert.Equal(t, DefaultRetryBudget, policy.Budget)
	assert.Equal(t, DefaultRetryMax, policy.Max)
	// Sleeper is intentionally nil in the shared policy; Sleep() falls back to
	// realSleeper so production retries actually wait. Verify that fallback is
	// realSleeper (not some no-op) and that Sleep is a no-op for d<=0.
	_, isReal := policy.sleeper().(realSleeper)
	assert.True(t, isReal)
	policy.Sleep(0) // must not block
}

func TestRetryPolicySleepUsesFakeSleeper(t *testing.T) {
	fake := &FakeSleeper{}
	policy := RetryPolicy{Budget: 1, Base: 2 * time.Second, Max: 8 * time.Second, Sleeper: fake, Rand: func() float64 { return 1 }}

	// Simulate two retry waits the way the outer loop requests them.
	d1, _ := policy.NextDelay(&FeedError{Type: ErrorTypeNetworkError}, 0)
	d2, _ := policy.NextDelay(&FeedError{Type: ErrorTypeNetworkError}, 1)
	policy.Sleep(d1)
	policy.Sleep(d2)

	delays := fake.Delays()
	assert.Len(t, delays, 2)
	assert.Equal(t, 2*time.Second, delays[0]) // attempt 0 ceiling at rand=1
	assert.Equal(t, 4*time.Second, delays[1]) // attempt 1 ceiling at rand=1
}

func TestRetryAdmissionBoundsConcurrentRetriesPerDomain(t *testing.T) {
	admission := NewRetryAdmission(1)
	releaseFirst, admitted := admission.Acquire(context.Background(), "EXAMPLE.com")
	assert.True(t, admitted)

	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	result := make(chan bool, 1)
	go func() {
		close(started)
		secondRelease, secondAdmitted := admission.Acquire(ctx, "example.com")
		if secondRelease != nil {
			secondRelease()
		}
		result <- secondAdmitted
	}()
	<-started
	cancel()
	assert.False(t, <-result, "same-domain retry must wait for the active retry slot")

	otherRelease, otherAdmitted := admission.Acquire(context.Background(), "other.example.com")
	assert.True(t, otherAdmitted, "different domains must not share a retry slot")
	otherRelease()

	releaseFirst()
	releaseNext, admittedNext := admission.Acquire(context.Background(), "example.com")
	assert.True(t, admittedNext, "slot must be reusable after the retry completes")
	releaseNext()
}

// fmtErrorWrap wraps a FeedError in a sentinel wrapper so CategoryOf/RetryAfterOf
// must use errors.As rather than a naive type assertion.
func fmtErrorWrap(err error) error {
	return wrappedError{err: err}
}

type wrappedError struct{ err error }

func (w wrappedError) Error() string { return w.err.Error() }
func (w wrappedError) Unwrap() error { return w.err }
