package feed

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// FeedErrorType Feed错误类型
type FeedErrorType int

const (
	// ErrorTypeUnknown 未知错误
	ErrorTypeUnknown FeedErrorType = iota
	// ErrorTypePaymentRequired 付费播客 (402)
	ErrorTypePaymentRequired
	// ErrorTypeCertificateExpired 证书过期
	ErrorTypeCertificateExpired
	// ErrorTypeNetworkError 网络错误
	ErrorTypeNetworkError
	// ErrorTypeTimeout 超时
	ErrorTypeTimeout
	// ErrorTypeNotFound 404错误
	ErrorTypeNotFound
	// ErrorTypeInvalidFeed 无效的feed格式
	ErrorTypeInvalidFeed
	// ErrorTypeAccessDenied 访问拒绝 (403, 401)
	ErrorTypeAccessDenied
	// ErrorTypeGeoBlocked 地区限制
	ErrorTypeGeoBlocked
	// ErrorTypeRateLimited 请求过于频繁 (429)
	ErrorTypeRateLimited
	// ErrorTypeServiceUnavailable 上游服务暂时不可用 (503/5xx)
	ErrorTypeServiceUnavailable
	// ErrorTypeInvalidRequest 客户端安全决策拒绝（如重定向协议/跳数越界），
	// 既非网络故障也非上游可重试错误
	ErrorTypeInvalidRequest
	// ErrorTypePolicyRejected robots.txt 通用 Disallow 规则禁止了该 Feed 路径，
	// 属于抓取前的本地准入决策：既不是网络故障，也不是上游 HTTP 403，AccessOutcome
	// 用独立的 policy_rejected 类别区分，且不可重试（重试也不会改变规则）。
	ErrorTypePolicyRejected
)

// String renders the error category as a short stable label so structured retry
// logs and diagnostics can print it directly. Unknown values fall back to the
// numeric form rather than a misleading name.
func (t FeedErrorType) String() string {
	switch t {
	case ErrorTypeUnknown:
		return "unknown"
	case ErrorTypePaymentRequired:
		return "payment_required"
	case ErrorTypeCertificateExpired:
		return "certificate_expired"
	case ErrorTypeNetworkError:
		return "network"
	case ErrorTypeTimeout:
		return "timeout"
	case ErrorTypeNotFound:
		return "not_found"
	case ErrorTypeInvalidFeed:
		return "invalid_feed"
	case ErrorTypeAccessDenied:
		return "access_denied"
	case ErrorTypeGeoBlocked:
		return "geo_blocked"
	case ErrorTypeRateLimited:
		return "rate_limited"
	case ErrorTypeServiceUnavailable:
		return "service_unavailable"
	case ErrorTypeInvalidRequest:
		return "invalid_request"
	case ErrorTypePolicyRejected:
		return "policy_rejected"
	default:
		return fmt.Sprintf("feed_error_type_%d", int(t))
	}
}

type HTTPStatusError struct {
	StatusCode int
}

func (e *HTTPStatusError) Error() string {
	return fmt.Sprintf("HTTP status code %d", e.StatusCode)
}

func newHTTPStatusError(status int) error {
	return &HTTPStatusError{StatusCode: status}
}

// FeedError Feed错误
type FeedError struct {
	Type     FeedErrorType
	FeedURL  string
	Original error
	Message  string
	// RetryAfter carries the upstream Retry-After header value (verbatim) for
	// retryable HTTP errors (429/503). It is the single channel through which
	// the outer retry loop learns the upstream-requested wait without needing
	// the AccessOutcome. It is never logged verbatim if it could carry a
	// sensitive value — Retry-After is server-controlled and bounded, and the
	// retry layer parses+caps it before sleeping.
	RetryAfter string
}

func (e *FeedError) Error() string {
	return e.Message
}

func (e *FeedError) Unwrap() error {
	return e.Original
}

// GetSkipReason 获取跳过原因（如果适用）
func (e *FeedError) GetSkipReason() (shouldSkip bool, reason string, description string) {
	switch e.Type {
	case ErrorTypePaymentRequired:
		return true, "paid", "付费播客（需要订阅）"
	case ErrorTypeCertificateExpired:
		return true, "certificate", "SSL/TLS证书已过期"
	case ErrorTypeNotFound:
		return true, "not_found", "Feed不存在 (404)"
	case ErrorTypeAccessDenied:
		return true, "access_denied", "访问被拒绝"
	case ErrorTypeGeoBlocked:
		return true, "geo_blocked", "地区限制，无法访问"
	default:
		return false, "", ""
	}
}

// IsPaymentRequired 是否为付费播客错误
func IsPaymentRequired(err error) bool {
	var fe *FeedError
	if errors.As(err, &fe) {
		return fe.Type == ErrorTypePaymentRequired
	}
	return false
}

// IsCertificateExpired 是否为证书过期错误
func IsCertificateExpired(err error) bool {
	var fe *FeedError
	if errors.As(err, &fe) {
		return fe.Type == ErrorTypeCertificateExpired
	}
	return false
}

// IsNetworkError 是否为网络错误
func IsNetworkError(err error) bool {
	var fe *FeedError
	if errors.As(err, &fe) {
		return fe.Type == ErrorTypeNetworkError
	}
	return false
}

// IsRetryable 是否可重试
func IsRetryable(err error) bool {
	var fe *FeedError
	if errors.As(err, &fe) {
		return fe.Type == ErrorTypeNetworkError || fe.Type == ErrorTypeTimeout ||
			fe.Type == ErrorTypeRateLimited || fe.Type == ErrorTypeServiceUnavailable
	}
	return false
}

// GetSkipReasonFromError 获取错误的跳过原因
func GetSkipReasonFromError(err error) (shouldSkip bool, reason string, description string) {
	var fe *FeedError
	if errors.As(err, &fe) {
		return fe.GetSkipReason()
	}
	return false, "", ""
}

// ClassifyError 分类错误
func ClassifyError(feedURL string, err error) *FeedError {
	if err == nil {
		return nil
	}

	errMsg := err.Error()
	safeURL := SanitizeFeedURL(feedURL)
	// A redirect rejected by policy (non-HTTP(S) scheme or hop-limit) carries
	// no target in ErrFeedUnsafeRedirect itself, BUT net/http wraps it in a
	// *url.Error whose URL IS the rejected redirect target. Never interpolate
	// the original error into the message here, or that target — which may
	// carry credentials or an internal address — leaks into logs and
	// user-facing summaries. Keep Original so errors.Is keeps working.
	if errors.Is(err, ErrFeedUnsafeRedirect) {
		return &FeedError{Type: ErrorTypeInvalidRequest, FeedURL: safeURL, Original: err, Message: fmt.Sprintf("Feed重定向被拒绝（协议或跳数限制）: %s", safeURL)}
	}
	var statusErr *HTTPStatusError
	if errors.As(err, &statusErr) {
		status := statusErr.StatusCode
		switch {
		case status == http.StatusUnauthorized || status == http.StatusForbidden:
			return &FeedError{Type: ErrorTypeAccessDenied, FeedURL: safeURL, Original: err, Message: fmt.Sprintf("访问被拒绝: %s", safeURL)}
		case status == http.StatusTooManyRequests:
			return &FeedError{Type: ErrorTypeRateLimited, FeedURL: safeURL, Original: err, Message: fmt.Sprintf("请求受到限速: %s", safeURL)}
		case status >= http.StatusInternalServerError:
			// All 5xx are transient upstream failures (502/503/504 gateways, 500
			// internal errors). The ErrorTypeServiceUnavailable doc promises 5xx
			// coverage and the outer retry policy treats this type as retryable,
			// so mapping the whole range here is what makes a generic 500/502/504
			// eligible for the bounded retry budget.
			return &FeedError{Type: ErrorTypeServiceUnavailable, FeedURL: safeURL, Original: err, Message: fmt.Sprintf("上游服务暂时不可用: %s", safeURL)}
		default:
			return &FeedError{Type: ErrorTypeUnknown, FeedURL: safeURL, Original: err, Message: fmt.Sprintf("HTTP请求失败: %s", safeURL)}
		}
	}

	if strings.Contains(errMsg, "EOF") || err.Error() == "EOF" {
		return &FeedError{Type: ErrorTypeNetworkError, FeedURL: safeURL, Original: err, Message: fmt.Sprintf("连接被服务器关闭（EOF）: %s", safeURL)}
	}
	if strings.Contains(errMsg, "402") || strings.Contains(errMsg, "Payment Required") {
		return &FeedError{Type: ErrorTypePaymentRequired, FeedURL: safeURL, Original: err, Message: fmt.Sprintf("付费播客（需要订阅）: %s", safeURL)}
	}
	if strings.Contains(errMsg, "403") || strings.Contains(errMsg, "Forbidden") || strings.Contains(errMsg, "401") || strings.Contains(errMsg, "Unauthorized") {
		return &FeedError{Type: ErrorTypeAccessDenied, FeedURL: safeURL, Original: err, Message: fmt.Sprintf("访问被拒绝: %s", safeURL)}
	}
	if strings.Contains(errMsg, "certificate") && (strings.Contains(errMsg, "expired") || strings.Contains(errMsg, "not yet valid")) {
		return &FeedError{Type: ErrorTypeCertificateExpired, FeedURL: safeURL, Original: err, Message: fmt.Sprintf("证书已过期: %s", safeURL)}
	}
	if strings.Contains(errMsg, "timeout") || strings.Contains(errMsg, "deadline exceeded") {
		return &FeedError{Type: ErrorTypeTimeout, FeedURL: safeURL, Original: err, Message: fmt.Sprintf("连接超时: %s", safeURL)}
	}
	if strings.Contains(errMsg, "404") || strings.Contains(errMsg, "Not Found") {
		return &FeedError{Type: ErrorTypeNotFound, FeedURL: safeURL, Original: err, Message: fmt.Sprintf("Feed不存在: %s", safeURL)}
	}
	if strings.Contains(errMsg, "connection refused") || strings.Contains(errMsg, "no such host") || strings.Contains(errMsg, "network is unreachable") || strings.Contains(errMsg, "connection reset") || strings.Contains(errMsg, "broken pipe") {
		return &FeedError{Type: ErrorTypeNetworkError, FeedURL: safeURL, Original: err, Message: fmt.Sprintf("网络连接失败: %s", safeURL)}
	}
	return &FeedError{Type: ErrorTypeUnknown, FeedURL: safeURL, Original: err, Message: fmt.Sprintf("未知错误: %s - %v", safeURL, err)}
}

// WrapFeedParseError classifies a successful HTTP response whose body is not
// parseable as a Feed without exposing the response body.
func WrapFeedParseError(feedURL string, err error) error {
	if err == nil {
		return nil
	}
	safeURL := SanitizeFeedURL(feedURL)
	return &FeedError{Type: ErrorTypeInvalidFeed, FeedURL: safeURL, Original: err, Message: fmt.Sprintf("Feed解析失败: %s", safeURL)}
}

// WrapHTTPError 包装HTTP错误
func WrapHTTPError(feedURL string, err error) error {
	if err == nil {
		return nil
	}
	return ClassifyError(feedURL, err)
}
