package codexruntime

import "errors"

const (
	ErrorRuntimeUnavailable = "runtime_unavailable"
	ErrorInvalidRequest     = "invalid_request"
	ErrorExecutionNotFound  = "execution_not_found"
	ErrorExecutionFailed    = "execution_failed"
	ErrorCapabilityDenied   = "capability_denied"
	ErrorProtocol           = "runtime_protocol_error"
	ErrorHostClosed         = "runtime_host_closed"
	// ErrorProfileUnavailable is reported when the current account or fixed
	// runtime does not support a requested model profile. It must surface to
	// the user; the runtime never substitutes another profile.
	ErrorProfileUnavailable = "profile_unavailable"
)

type RuntimeError struct {
	Code        string
	SafeMessage string
	Retryable   bool
}

func (e *RuntimeError) Error() string {
	if e == nil {
		return ""
	}
	return e.SafeMessage
}

func newRuntimeError(code, message string, retryable bool) error {
	return &RuntimeError{
		Code:        code,
		SafeMessage: message,
		Retryable:   retryable,
	}
}

func ErrorCode(err error) string {
	var runtimeErr *RuntimeError
	if errors.As(err, &runtimeErr) {
		return runtimeErr.Code
	}
	return ""
}
