package feed

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDecideBatchRetryAccessDeniedUsesFixedOffsets(t *testing.T) {
	// First 403 after first-pass (attempt=1, no access-denied retries yet):
	// wait until minute 3.
	dec := DecideBatchRetry(BatchRetryInput{
		Category:            ErrorCategoryAccessDenied,
		Attempt:             1,
		AccessDeniedRetries: 0,
		BatchElapsed:        10 * time.Second,
		BatchRemaining:      14*time.Minute + 50*time.Second,
	})
	require.True(t, dec.Retry)
	require.Equal(t, 3*time.Minute-10*time.Second, dec.Wait)
	require.Equal(t, "access_denied_scheduled", dec.Reason)

	// After first scheduled retry consumed, next slot is minute 8.
	dec = DecideBatchRetry(BatchRetryInput{
		Category:            ErrorCategoryAccessDenied,
		Attempt:             2,
		AccessDeniedRetries: 1,
		BatchElapsed:        3*time.Minute + 5*time.Second,
		BatchRemaining:      11*time.Minute + 55*time.Second,
	})
	require.True(t, dec.Retry)
	require.Equal(t, 8*time.Minute-(3*time.Minute+5*time.Second), dec.Wait)

	// Third slot at minute 13.
	dec = DecideBatchRetry(BatchRetryInput{
		Category:            ErrorCategoryAccessDenied,
		Attempt:             3,
		AccessDeniedRetries: 2,
		BatchElapsed:        8 * time.Minute,
		BatchRemaining:      7 * time.Minute,
	})
	require.True(t, dec.Retry)
	require.Equal(t, 5*time.Minute, dec.Wait)

	// Budget exhausted after three scheduled retries.
	dec = DecideBatchRetry(BatchRetryInput{
		Category:            ErrorCategoryAccessDenied,
		Attempt:             4,
		AccessDeniedRetries: 3,
		BatchElapsed:        13 * time.Minute,
		BatchRemaining:      2 * time.Minute,
	})
	require.False(t, dec.Retry)
	require.Equal(t, "access_denied_budget_exhausted", dec.Reason)
}

func TestDecideBatchRetryHonorsRetryAfterWithinDeadline(t *testing.T) {
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	dec := DecideBatchRetry(BatchRetryInput{
		Category:         ErrorCategoryRateLimited,
		Attempt:          1,
		TransientRetries: 0,
		BatchElapsed:     time.Minute,
		BatchRemaining:   10 * time.Minute,
		RetryAfter:       "30",
		Now:              now,
	})
	require.True(t, dec.Retry)
	require.Equal(t, 30*time.Second, dec.Wait)

	// Retry-After longer than remaining time → stop.
	dec = DecideBatchRetry(BatchRetryInput{
		Category:         ErrorCategoryServiceUnavailable,
		Attempt:          1,
		TransientRetries: 0,
		BatchElapsed:     14 * time.Minute,
		BatchRemaining:   20 * time.Second,
		RetryAfter:       "60",
		Now:              now,
	})
	require.False(t, dec.Retry)
	require.Equal(t, "retry_after_past_deadline", dec.Reason)
}

func TestDecideBatchRetryTransientBudgetAndDeadline(t *testing.T) {
	dec := DecideBatchRetry(BatchRetryInput{
		Category:         ErrorCategoryTimeout,
		Attempt:          1,
		TransientRetries: 0,
		BatchElapsed:     time.Minute,
		BatchRemaining:   5 * time.Minute,
	})
	require.True(t, dec.Retry)

	dec = DecideBatchRetry(BatchRetryInput{
		Category:         ErrorCategoryNetwork,
		Attempt:          4,
		TransientRetries: MaxTransientRetries,
		BatchElapsed:     time.Minute,
		BatchRemaining:   5 * time.Minute,
	})
	require.False(t, dec.Retry)
	require.Equal(t, "transient_budget_exhausted", dec.Reason)

	dec = DecideBatchRetry(BatchRetryInput{
		Category:         ErrorCategoryHTTP,
		Attempt:          1,
		TransientRetries: 0,
		BatchElapsed:     14*time.Minute + 59*time.Second,
		BatchRemaining:   time.Second,
	})
	// fullJitter with 0.5 yields base/2 for attempt 0 which may exceed remaining
	// when remaining is 1s and base is 2s → past deadline.
	require.False(t, dec.Retry)
}

func TestDecideBatchRetryNonRetryable(t *testing.T) {
	for _, cat := range []ErrorCategory{
		ErrorCategoryHTTP,
		ErrorCategoryParse,
		ErrorCategoryPolicyRejected,
		ErrorCategoryCircuitOpen,
		ErrorCategoryInvalidRequest,
	} {
		dec := DecideBatchRetry(BatchRetryInput{
			Category:       cat,
			Attempt:        1,
			BatchElapsed:   time.Minute,
			BatchRemaining: 10 * time.Minute,
		})
		require.False(t, dec.Retry, "category %s should not retry", cat)
	}
}

func TestBatchTerminalStatus(t *testing.T) {
	require.Equal(t, "completed", BatchTerminalStatus(3, 0))
	require.Equal(t, "partial", BatchTerminalStatus(2, 1))
	require.Equal(t, "failed", BatchTerminalStatus(0, 3))
	require.Equal(t, "completed", BatchTerminalStatus(0, 0))
}

func TestSoftRateNeverZero(t *testing.T) {
	for _, tier := range []SoftRateTier{SoftRateNormal, SoftRateCautious, SoftRateSlow} {
		require.True(t, SoftRateNeverZero(tier))
		require.GreaterOrEqual(t, softRateSpacingFor(tier), time.Duration(0))
	}
}
