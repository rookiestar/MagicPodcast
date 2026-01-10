package sync

import (
	"encoding/json"
	"fmt"
	"io"
)

// LogMessageType 日志消息类型
type LogMessageType string

const (
	LogTypeUnknown   LogMessageType = "unknown"   // 未知类型
	LogTypeInfo      LogMessageType = "info"      // 一般信息
	LogTypeSuccess   LogMessageType = "success"   // 成功
	LogTypeError     LogMessageType = "error"     // 错误
	LogTypeProgress  LogMessageType = "progress"  // 进度
	LogTypeSkipPaid  LogMessageType = "skip_paid" // 跳过付费播客
	LogTypeSkipCert  LogMessageType = "skip_cert" // 跳过证书过期
	LogTypeSkipOther LogMessageType = "skip_other"// 跳过其他原因
)

// SkipReason 跳过原因
type SkipReason string

const (
	SkipReasonPaid             SkipReason = "paid"              // 付费播客
	SkipReasonCertificate      SkipReason = "certificate"       // 证书过期
	SkipReasonNotFound         SkipReason = "not_found"         // 404不存在
	SkipReasonInvalidFormat    SkipReason = "invalid_format"    // 格式无效
	SkipReasonDuplicate        SkipReason = "duplicate"         // 重复
	SkipReasonAccessDenied     SkipReason = "access_denied"     // 访问拒绝
	SkipReasonGeoBlocked       SkipReason = "geo_blocked"       // 地区限制
	SkipReasonOther            SkipReason = "other"             // 其他原因
)

// ProgressReporter 进度报告接口
type ProgressReporter interface {
	Report(message string)
	ReportSuccess(message string)
	ReportError(message string)
	ReportProgress(current, total int, message string)
	ReportSkip(reason SkipReason, message string)
	Close()
}

// LogProgressReporter 使用log的进度报告器
type LogProgressReporter struct{}

func NewLogProgressReporter() *LogProgressReporter {
	return &LogProgressReporter{}
}

func (r *LogProgressReporter) Report(message string) {
	fmt.Printf("[INFO] %s\n", message)
}

func (r *LogProgressReporter) ReportSuccess(message string) {
	fmt.Printf("[SUCCESS] ✅ %s\n", message)
}

func (r *LogProgressReporter) ReportError(message string) {
	fmt.Printf("[ERROR] ❌ %s\n", message)
}

func (r *LogProgressReporter) ReportProgress(current, total int, message string) {
	fmt.Printf("[%d/%d] %s\n", current, total, message)
}

func (r *LogProgressReporter) ReportSkip(reason SkipReason, message string) {
	var icon string
	switch reason {
	case SkipReasonPaid:
		icon = "💰"
	case SkipReasonCertificate:
		icon = "🔐"
	case SkipReasonNotFound:
		icon = "🔍"
	case SkipReasonInvalidFormat:
		icon = "📄"
	case SkipReasonDuplicate:
		icon = "🔄"
	case SkipReasonAccessDenied:
		icon = "🚫"
	case SkipReasonGeoBlocked:
		icon = "🌍"
	default:
		icon = "⏭️"
	}
	fmt.Printf("[SKIP] %s %s\n", icon, message)
}

func (r *LogProgressReporter) Close() {
	// Nothing to close for log reporter
}

// SSEMessage SSE消息格式
type SSEMessage struct {
	Type    LogMessageType `json:"type"`
	Message string         `json:"message"`
	Current int            `json:"current,omitempty"`
	Total   int            `json:"total,omitempty"`
	Reason  string         `json:"reason,omitempty"` // 跳过原因
}

// SSEProgressReporter 使用Server-Sent Events的进度报告器
type SSEProgressReporter struct {
	writer io.Writer
}

func NewSSEProgressReporter(writer io.Writer) *SSEProgressReporter {
	return &SSEProgressReporter{
		writer: writer,
	}
}

func (r *SSEProgressReporter) Report(message string) {
	r.send(SSEMessage{
		Type:    LogTypeInfo,
		Message: message,
	})
}

func (r *SSEProgressReporter) ReportSuccess(message string) {
	r.send(SSEMessage{
		Type:    LogTypeSuccess,
		Message: message,
	})
}

func (r *SSEProgressReporter) ReportError(message string) {
	r.send(SSEMessage{
		Type:    LogTypeError,
		Message: message,
	})
}

func (r *SSEProgressReporter) ReportProgress(current, total int, message string) {
	r.send(SSEMessage{
		Type:    LogTypeProgress,
		Message: message,
		Current: current,
		Total:   total,
	})
}

func (r *SSEProgressReporter) ReportSkip(reason SkipReason, message string) {
	// 将SkipReason转换为字符串
	reasonStr := string(reason)

	// 根据reason类型选择LogMessageType
	var msgType LogMessageType
	switch reason {
	case SkipReasonPaid:
		msgType = LogTypeSkipPaid
	case SkipReasonCertificate:
		msgType = LogTypeSkipCert
	default:
		msgType = LogTypeSkipOther
	}

	r.send(SSEMessage{
		Type:    msgType,
		Message: message,
		Reason:  reasonStr,
	})
}

func (r *SSEProgressReporter) send(msg SSEMessage) {
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}

	// SSE格式: "data: <json>\n\n"
	fmt.Fprintf(r.writer, "data: %s\n\n", string(data))
}

func (r *SSEProgressReporter) Close() {
	// 发送结束标记
	fmt.Fprintf(r.writer, "data: [DONE]\n\n")
}
