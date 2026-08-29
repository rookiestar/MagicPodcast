package processing

import "errors"

var (
	ErrEpisodeNotFound            = errors.New("episode not found")
	ErrEpisodeNotFocused          = errors.New("episode is not in Focus")
	ErrProcessingInputUnavailable = errors.New("processing input is unavailable")
	ErrRunNotFound                = errors.New("processing run not found")
	ErrRunBusy                    = errors.New("processing run is already executing")
	ErrInvalidStart               = errors.New("invalid processing start request")
	ErrRetryUnsafe                = errors.New("processing run cannot be retried safely")
	ErrRetryNotReady              = errors.New("processing retry is not ready")
	ErrArtifactNotFound           = errors.New("artifact set not found")
	ErrArtifactExists             = errors.New("artifact set already exists")
	ErrInvalidArtifact            = errors.New("invalid artifact set")
	ErrArtifactAudioUnavailable   = errors.New("artifact audio is unavailable")
	ErrArtifactAudioMismatch      = errors.New("artifact audio digest does not match")
)

// AdapterError is the provider-neutral error contract used by every
// processing seam.
type AdapterError struct {
	ErrorCode     string
	SafeMessage   string
	CanRetry      bool
	ResultUnknown bool
}

func (e *AdapterError) Error() string {
	if e == nil {
		return ""
	}
	return e.SafeMessage
}

func NewAdapterError(code, safeMessage string, retryable bool) error {
	return &AdapterError{
		ErrorCode:   code,
		SafeMessage: safeMessage,
		CanRetry:    retryable,
	}
}

func NewUnknownExternalResultError(code, safeMessage string) error {
	return &AdapterError{
		ErrorCode:     code,
		SafeMessage:   safeMessage,
		ResultUnknown: true,
	}
}

type classifiedError struct {
	code          string
	message       string
	retryable     bool
	resultUnknown bool
}

func classifyAdapterError(err error) classifiedError {
	if err == nil {
		return classifiedError{}
	}
	var adapterErr *AdapterError
	if errors.As(err, &adapterErr) {
		return classifiedError{
			code:          adapterErr.ErrorCode,
			message:       adapterErr.SafeMessage,
			retryable:     adapterErr.CanRetry,
			resultUnknown: adapterErr.ResultUnknown,
		}
	}
	return classifiedError{
		code:    "adapter_error",
		message: "processing adapter failed",
	}
}
