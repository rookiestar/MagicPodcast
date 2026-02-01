package feed

import (
	"errors"
	"fmt"
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
)

// FeedError Feed错误
type FeedError struct {
	Type     FeedErrorType
	FeedURL  string
	Original error
	Message  string
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
		return fe.Type == ErrorTypeNetworkError || fe.Type == ErrorTypeTimeout
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

	// 检查是否为EOF错误（连接被服务器过早关闭）
	if strings.Contains(errMsg, "EOF") || err.Error() == "EOF" {
		return &FeedError{
			Type:     ErrorTypeNetworkError,
			FeedURL:  feedURL,
			Original: err,
			Message:  fmt.Sprintf("连接被服务器关闭（EOF）: %s", feedURL),
		}
	}

	// 检查是否为402付费错误
	if strings.Contains(errMsg, "402") || strings.Contains(errMsg, "Payment Required") {
		return &FeedError{
			Type:     ErrorTypePaymentRequired,
			FeedURL:  feedURL,
			Original: err,
			Message:  fmt.Sprintf("付费播客（需要订阅）: %s", feedURL),
		}
	}

	// 检查是否为403/401访问拒绝
	if strings.Contains(errMsg, "403") || strings.Contains(errMsg, "Forbidden") ||
		strings.Contains(errMsg, "401") || strings.Contains(errMsg, "Unauthorized") {
		return &FeedError{
			Type:     ErrorTypeAccessDenied,
			FeedURL:  feedURL,
			Original: err,
			Message:  fmt.Sprintf("访问被拒绝: %s", feedURL),
		}
	}

	// 检查是否为证书过期错误
	if strings.Contains(errMsg, "certificate") && (strings.Contains(errMsg, "expired") || strings.Contains(errMsg, "not yet valid")) {
		return &FeedError{
			Type:     ErrorTypeCertificateExpired,
			FeedURL:  feedURL,
			Original: err,
			Message:  fmt.Sprintf("证书已过期: %s", feedURL),
		}
	}

	// 检查是否为超时错误
	if strings.Contains(errMsg, "timeout") || strings.Contains(errMsg, "deadline exceeded") {
		return &FeedError{
			Type:     ErrorTypeTimeout,
			FeedURL:  feedURL,
			Original: err,
			Message:  fmt.Sprintf("连接超时: %s", feedURL),
		}
	}

	// 检查是否为404错误
	if strings.Contains(errMsg, "404") || strings.Contains(errMsg, "Not Found") {
		return &FeedError{
			Type:     ErrorTypeNotFound,
			FeedURL:  feedURL,
			Original: err,
			Message:  fmt.Sprintf("Feed不存在: %s", feedURL),
		}
	}

	// 检查是否为网络连接错误
	if strings.Contains(errMsg, "connection refused") ||
		strings.Contains(errMsg, "no such host") ||
		strings.Contains(errMsg, "network is unreachable") ||
		strings.Contains(errMsg, "connection reset") ||
		strings.Contains(errMsg, "broken pipe") {
		return &FeedError{
			Type:     ErrorTypeNetworkError,
			FeedURL:  feedURL,
			Original: err,
			Message:  fmt.Sprintf("网络连接失败: %s", feedURL),
		}
	}

	// 默认为未知错误
	return &FeedError{
		Type:     ErrorTypeUnknown,
		FeedURL:  feedURL,
		Original: err,
		Message:  fmt.Sprintf("未知错误: %s - %v", feedURL, err),
	}
}

// WrapHTTPError 包装HTTP错误
func WrapHTTPError(feedURL string, err error) error {
	if err == nil {
		return nil
	}

	// 检查是否为HTTP状态码错误
	var httpErr interface{}
	if errors.As(err, &httpErr) {
		// 尝试提取HTTP状态码
		errMsg := err.Error()
		if strings.Contains(errMsg, "status code") {
			// gofeed库的错误格式通常包含 "status code"
			return ClassifyError(feedURL, err)
		}
	}

	return ClassifyError(feedURL, err)
}
