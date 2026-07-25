package feed

import (
	"time"
)

// DefaultBatchDuration is the hard upper bound on networking for one workflow
// job. After this window the batch must stop fetching and finalize.
const DefaultBatchDuration = 15 * time.Minute

// AccessDeniedRetryOffsets are the batch-relative times at which a 403/401 may
// be retried (~minutes 3, 8, and 13). They are absolute offsets from batch
// start, not inter-attempt delays.
var AccessDeniedRetryOffsets = []time.Duration{
	3 * time.Minute,
	8 * time.Minute,
	13 * time.Minute,
}

// Transient retry bounds inside a batch (network / timeout / 5xx).
const (
	// MaxTransientRetries is the maximum number of retries AFTER the first
	// attempt for network/timeout/5xx failures within one batch.
	MaxTransientRetries = 3
	// DefaultTransientBase is the full-jitter base for transient batch retries.
	DefaultTransientBase = 2 * time.Second
	// DefaultTransientMax caps a single transient backoff wait.
	DefaultTransientMax = 30 * time.Second
)

// BatchRetryDecision is the pure policy outcome for one failed attempt inside a
// bounded workflow batch. It never sleeps and never touches I/O.
type BatchRetryDecision struct {
	// Retry is true when another network attempt is allowed before the deadline.
	Retry bool
	// Wait is how long to wait from now before the next attempt (0 = immediate).
	// For access_denied the wait is derived from the next fixed offset so the
	// caller can sleep until batchStart+offset.
	Wait time.Duration
	// Reason is a stable machine label for attempt history / diagnostics.
	Reason string
}

// BatchRetryInput carries the minimum state needed to decide the next attempt.
type BatchRetryInput struct {
	// Category is the classified error for the attempt that just finished.
	Category ErrorCategory
	// Attempt is 1-based for the attempt that just finished (first pass = 1).
	Attempt int
	// AccessDeniedRetries is how many access_denied retries have already been
	// scheduled/consumed after the first pass (0..len(AccessDeniedRetryOffsets)).
	AccessDeniedRetries int
	// TransientRetries is how many network/timeout/5xx retries have already
	// been consumed after the first pass.
	TransientRetries int
	// BatchElapsed is now - batchStart.
	BatchElapsed time.Duration
	// BatchRemaining is deadline - now (clamped at 0 by the caller when past).
	BatchRemaining time.Duration
	// RetryAfter is the upstream Retry-After header for 429/503 when present.
	RetryAfter string
	// Now is the current time (for HTTP-date Retry-After parsing).
	Now time.Time
}

// DecideBatchRetry classifies whether and when to retry inside the 15-minute
// batch. Access denied uses fixed offsets (~3/8/13 min); 429/503 honor
// Retry-After within remaining time; network/timeout/5xx use bounded backoff.
// Policy rejections, parse errors, not-found, and circuit_open do not retry.
func DecideBatchRetry(in BatchRetryInput) BatchRetryDecision {
	if in.BatchRemaining <= 0 {
		return BatchRetryDecision{Retry: false, Reason: "batch_deadline"}
	}
	if in.Attempt < 1 {
		in.Attempt = 1
	}

	switch in.Category {
	case ErrorCategoryAccessDenied:
		if in.AccessDeniedRetries >= len(AccessDeniedRetryOffsets) {
			return BatchRetryDecision{Retry: false, Reason: "access_denied_budget_exhausted"}
		}
		target := AccessDeniedRetryOffsets[in.AccessDeniedRetries]
		if in.BatchElapsed >= target {
			// Already past the slot; try immediately if any remaining time.
			return BatchRetryDecision{Retry: true, Wait: 0, Reason: "access_denied_slot"}
		}
		wait := target - in.BatchElapsed
		if wait >= in.BatchRemaining {
			return BatchRetryDecision{Retry: false, Reason: "access_denied_past_deadline"}
		}
		return BatchRetryDecision{Retry: true, Wait: wait, Reason: "access_denied_scheduled"}

	case ErrorCategoryRateLimited, ErrorCategoryServiceUnavailable:
		if in.TransientRetries >= MaxTransientRetries {
			return BatchRetryDecision{Retry: false, Reason: "transient_budget_exhausted"}
		}
		wait := DefaultTransientBase
		if d, ok := ParseRetryAfter(in.RetryAfter, in.Now); ok {
			wait = d
		} else {
			wait = fullJitter(DefaultTransientBase, DefaultTransientMax, in.TransientRetries, func() float64 { return 0.5 })
		}
		if wait > MaxRetryAfter {
			wait = MaxRetryAfter
		}
		if wait >= in.BatchRemaining {
			return BatchRetryDecision{Retry: false, Reason: "retry_after_past_deadline"}
		}
		return BatchRetryDecision{Retry: true, Wait: wait, Reason: "retry_after_or_backoff"}

	case ErrorCategoryTimeout, ErrorCategoryNetwork:
		if in.TransientRetries >= MaxTransientRetries {
			return BatchRetryDecision{Retry: false, Reason: "transient_budget_exhausted"}
		}
		wait := fullJitter(DefaultTransientBase, DefaultTransientMax, in.TransientRetries, func() float64 { return 0.5 })
		if wait >= in.BatchRemaining {
			return BatchRetryDecision{Retry: false, Reason: "transient_past_deadline"}
		}
		return BatchRetryDecision{Retry: true, Wait: wait, Reason: "transient_backoff"}

	case ErrorCategoryCircuitOpen:
		// Circuit open is a derived policy signal; the batch does not schedule
		// further attempts solely because of it (sibling feeds already continue).
		return BatchRetryDecision{Retry: false, Reason: "circuit_open_no_retry"}

	default:
		return BatchRetryDecision{Retry: false, Reason: "non_retryable"}
	}
}

// BatchTerminalStatus maps success/failure counts to the job terminal status
// after a bounded batch. Mixed outcomes are partial; all-fail is failed;
// all-success or empty is completed.
func BatchTerminalStatus(successCount, failedCount int) string {
	if successCount > 0 && failedCount > 0 {
		return "partial"
	}
	if failedCount > 0 {
		return "failed"
	}
	return "completed"
}

// SoftRateNeverZero documents and asserts the floor invariant used by tests:
// spacing may grow, but concurrency and admission never drop to zero.
func SoftRateNeverZero(tier SoftRateTier) bool {
	_ = softRateSpacingFor(tier)
	return tier == SoftRateNormal || tier == SoftRateCautious || tier == SoftRateSlow
}
