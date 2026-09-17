package personidentity

import (
	"context"
	"errors"

	"magicpodcast/internal/codexruntime"
)

// Stable failure classifications shared by SSE payloads and run summaries.
// A classification is derived only from typed errors and provider-confirmed
// codes; error text is never mined for a diagnosis.
const (
	FailureSourcesChanged     = "sources_changed"
	FailureDeadline           = "deadline"
	FailureCancelled          = "cancelled"
	FailureRuntimeUnavailable = "runtime_unavailable"
	FailureRuntimeProtocol    = "runtime_protocol"
	FailureExecutionFailed    = "execution_failed"
	FailureProfileUnavailable = "profile_unavailable"
	FailureCapabilityDenied   = "capability_denied"
	FailureInvalidResult      = "invalid_result"
	FailureSaveFailed         = "save_failed"
	FailureInvalidRequest     = "invalid_request"
	FailureUnknown            = "unknown"
	FailureAuthentication     = "authentication_failed"
	FailureQuota              = "quota_exceeded"
	FailureConnection         = "upstream_connection_failed"
)

// SaveError marks a persistence failure after the Runtime produced a result.
// It keeps the original error wrappable so ErrSourcesChanged keeps its
// distinct meaning.
type SaveError struct{ Err error }

func (e *SaveError) Error() string { return e.Err.Error() }

// Unwrap keeps errors.Is/As working for the wrapped database failure.
func (e *SaveError) Unwrap() error { return e.Err }

// InvalidResultError marks an output that failed schema or evidence
// validation after a completed Runtime turn.
type InvalidResultError struct{ Err error }

func (e *InvalidResultError) Error() string { return e.Err.Error() }

// Unwrap keeps the underlying validation cause inspectable.
func (e *InvalidResultError) Unwrap() error { return e.Err }

// FailureClass is the stable classification and retryability of one failure.
type FailureClass struct {
	Code      string
	Retryable bool
}

// ClassifyFailure maps an error onto its stable classification. Distinct user
// actions depend on this mapping: retry makes sense for retryable connection
// failures, reconciliation for lost completion events, source refresh for
// source changes — never one generic message for all of them.
func ClassifyFailure(err error) FailureClass {
	switch {
	case err == nil:
		return FailureClass{Code: FailureUnknown}
	case errors.Is(err, ErrSourcesChanged):
		return FailureClass{Code: FailureSourcesChanged}
	case errors.Is(err, ErrTranscriptRequired), errors.Is(err, ErrEpisodeNotFound):
		return FailureClass{Code: FailureInvalidRequest}
	case errors.Is(err, context.DeadlineExceeded):
		return FailureClass{Code: FailureDeadline, Retryable: true}
	case errors.Is(err, context.Canceled):
		return FailureClass{Code: FailureCancelled}
	}
	var saveErr *SaveError
	if errors.As(err, &saveErr) {
		return FailureClass{Code: FailureSaveFailed}
	}
	var invalidErr *InvalidResultError
	if errors.As(err, &invalidErr) {
		return FailureClass{Code: FailureInvalidResult}
	}
	var runtimeErr *codexruntime.RuntimeError
	if errors.As(err, &runtimeErr) {
		switch runtimeErr.Code {
		case codexruntime.ErrorAuthentication:
			return FailureClass{Code: FailureAuthentication}
		case codexruntime.ErrorQuota:
			return FailureClass{Code: FailureQuota, Retryable: true}
		case codexruntime.ErrorConnection:
			return FailureClass{Code: FailureConnection, Retryable: true}
		case codexruntime.ErrorRuntimeUnavailable:
			return FailureClass{Code: FailureRuntimeUnavailable, Retryable: true}
		case codexruntime.ErrorProtocol, codexruntime.ErrorHostClosed:
			return FailureClass{Code: FailureRuntimeProtocol}
		case codexruntime.ErrorProfileUnavailable:
			return FailureClass{Code: FailureProfileUnavailable}
		case codexruntime.ErrorCapabilityDenied:
			return FailureClass{Code: FailureCapabilityDenied}
		case codexruntime.ErrorInvalidRequest:
			return FailureClass{Code: FailureInvalidRequest}
		default:
			return FailureClass{Code: FailureExecutionFailed, Retryable: runtimeErr.Retryable}
		}
	}
	return FailureClass{Code: FailureUnknown}
}
