package llm

import (
	"errors"
	"fmt"
	"strings"
)

var (
	// ErrEmptyCompletion is a retryable empty or whitespace-only assistant body.
	ErrEmptyCompletion = errors.New("LLM返回空摘要")
	// ErrIncompleteCompletion is a truncated completion; do not retry with the same parameters.
	ErrIncompleteCompletion = errors.New("LLM摘要不完整")
	// ErrInvalidFinishReason is a missing or abnormal finish reason; not treated as success.
	ErrInvalidFinishReason = errors.New("LLM结束原因异常")
)

func isSuccessfulFinishReason(reason string) bool {
	switch strings.ToLower(strings.TrimSpace(reason)) {
	case "stop", "end_turn", "stop_sequence":
		return true
	default:
		return false
	}
}

func isTruncatedFinishReason(reason string) bool {
	switch strings.ToLower(strings.TrimSpace(reason)) {
	case "length", "max_tokens", "max_output_tokens":
		return true
	default:
		return false
	}
}

func evaluateCompletion(content, finishReason string) (formatted string, incomplete bool, retryable bool, err error) {
	formatted = formatSummary(content)
	reason := strings.TrimSpace(finishReason)

	if isTruncatedFinishReason(reason) {
		return formatted, true, false, fmt.Errorf("%w: finish_reason=%s", ErrIncompleteCompletion, reason)
	}
	if !isSuccessfulFinishReason(reason) {
		return formatted, false, false, fmt.Errorf("%w: finish_reason=%q", ErrInvalidFinishReason, reason)
	}
	if strings.TrimSpace(formatted) == "" {
		return formatted, false, true, ErrEmptyCompletion
	}
	return formatted, false, false, nil
}
