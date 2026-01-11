package sync

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

// LogMessageType 日志消息类型
type LogMessageType string

const (
	LogTypeUnknown    LogMessageType = "unknown"     // 未知类型
	LogTypeInfo       LogMessageType = "info"        // 一般信息
	LogTypeSuccess    LogMessageType = "success"     // 成功
	LogTypeError      LogMessageType = "error"       // 错误
	LogTypeProgress   LogMessageType = "progress"    // 进度
	LogTypeSkipPaid   LogMessageType = "skip_paid"   // 跳过付费播客
	LogTypeSkipCert   LogMessageType = "skip_cert"   // 跳过证书过期
	LogTypeSkipNoUpd  LogMessageType = "skip_noupd"  // 跳过无更新
	LogTypeSkipOther  LogMessageType = "skip_other"  // 跳过其他原因
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
	SkipReasonNoUpdate         SkipReason = "no_update"         // 无内容更新
	SkipReasonOther            SkipReason = "other"             // 其他原因
)

// ProgressReporter 进度报告接口
type ProgressReporter interface {
	Report(message string)
	ReportSuccess(message string)
	ReportError(message string)
	ReportProgress(current, total int, message string)
	ReportSkip(reason SkipReason, message string)
	ReportSummary(summary *SyncSummary)
	Close()
}

// SyncSummary 同步汇总信息
type SyncSummary struct {
	TotalPodcasts      int           `json:"total_podcasts"`       // 总播客数
	SuccessPodcasts    int           `json:"success_podcasts"`     // 成功同步的播客数
	FailedPodcasts     int           `json:"failed_podcasts"`      // 失败的播客数
	SkippedPodcasts    int           `json:"skipped_podcasts"`     // 跳过的播客数
	NoUpdatePodcasts   int           `json:"no_update_podcasts"`   // 无更新的播客数
	TotalEpisodes      int           `json:"total_episodes"`       // 同步的总单集数
	NewEpisodes        int           `json:"new_episodes"`         // 新增的单集数
	UpdatedEpisodes    int           `json:"updated_episodes"`     // 更新的单集数
	Duration           time.Duration `json:"duration"`             // 总耗时
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
	case SkipReasonNoUpdate:
		icon = "✓"
	default:
		icon = "⏭️"
	}
	fmt.Printf("[SKIP] %s %s\n", icon, message)
}

func (r *LogProgressReporter) Close() {
	// Nothing to close for log reporter
}

func (r *LogProgressReporter) ReportSummary(summary *SyncSummary) {
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("📊 同步完成汇总")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("总播客数: %d\n", summary.TotalPodcasts)
	fmt.Printf("✅ 成功: %d\n", summary.SuccessPodcasts)
	fmt.Printf("❌ 失败: %d\n", summary.FailedPodcasts)
	fmt.Printf("⏭️  跳过: %d\n", summary.SkippedPodcasts)
	if summary.NoUpdatePodcasts > 0 {
		fmt.Printf("  └─ 无更新: %d\n", summary.NoUpdatePodcasts)
	}
	if summary.TotalEpisodes > 0 || summary.NewEpisodes > 0 || summary.UpdatedEpisodes > 0 {
		fmt.Println("\n📝 单集统计:")
		fmt.Printf("  总处理: %d\n", summary.TotalEpisodes)
		fmt.Printf("  新增: %d\n", summary.NewEpisodes)
		fmt.Printf("  更新: %d\n", summary.UpdatedEpisodes)
	}
	fmt.Printf("⏱️  总耗时: %s\n", formatDuration(summary.Duration))
	fmt.Println(strings.Repeat("=", 60))
}

// formatDuration 格式化时间间隔为人类可读格式
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.0f秒", d.Seconds())
	} else if d < time.Hour {
		minutes := int(d.Minutes())
		seconds := int(d.Seconds()) - minutes*60
		return fmt.Sprintf("%d分%d秒", minutes, seconds)
	} else {
		hours := int(d.Hours())
		minutes := int(d.Minutes()) - hours*60
		return fmt.Sprintf("%d小时%d分", hours, minutes)
	}
}

// SSEMessage SSE消息格式
type SSEMessage struct {
	Type    LogMessageType `json:"type"`
	Message string         `json:"message"`
	Current int            `json:"current,omitempty"`
	Total   int            `json:"total,omitempty"`
	Reason  string         `json:"reason,omitempty"` // 跳过原因
	Data    map[string]interface{} `json:"data,omitempty"` // 用于summary等复杂消息的额外数据
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
	case SkipReasonNoUpdate:
		msgType = LogTypeSkipNoUpd
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

func (r *SSEProgressReporter) ReportSummary(summary *SyncSummary) {
	// 发送汇总信息，包含详细的统计数据
	r.send(SSEMessage{
		Type:    "summary",
		Message: fmt.Sprintf("同步完成！成功: %d, 失败: %d, 跳过: %d, 耗时: %s",
			summary.SuccessPodcasts,
			summary.FailedPodcasts,
			summary.SkippedPodcasts,
			formatDuration(summary.Duration),
		),
		Data: map[string]interface{}{
			"total_podcasts":     summary.TotalPodcasts,
			"success_podcasts":   summary.SuccessPodcasts,
			"failed_podcasts":    summary.FailedPodcasts,
			"skipped_podcasts":   summary.SkippedPodcasts,
			"no_update_podcasts": summary.NoUpdatePodcasts,
			"total_episodes":     summary.TotalEpisodes,
			"new_episodes":       summary.NewEpisodes,
			"updated_episodes":   summary.UpdatedEpisodes,
			"duration":           formatDuration(summary.Duration),
		},
	})
}
