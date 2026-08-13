package sync

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"magicpodcast/internal/logger"
)

// LogMessageType 日志消息类型
type LogMessageType string

const (
	LogTypeUnknown   LogMessageType = "unknown"        // 未知类型
	LogTypeInfo      LogMessageType = "info"           // 一般信息
	LogTypeSuccess   LogMessageType = "success"        // 成功
	LogTypeError     LogMessageType = "error"          // 错误
	LogTypeProgress  LogMessageType = "progress"       // 进度
	LogTypeSkipPaid  LogMessageType = "skip_paid"      // 跳过付费播客
	LogTypeSkipCert  LogMessageType = "skip_cert"      // 跳过证书过期
	LogTypeSkipNoUpd LogMessageType = "skip_no_update" // 跳过无更新
	LogTypeSkipOther LogMessageType = "skip_other"     // 跳过其他原因
)

// SkipReason 跳过原因
type SkipReason string

const (
	SkipReasonPaid          SkipReason = "paid"           // 付费播客
	SkipReasonCertificate   SkipReason = "certificate"    // 证书过期
	SkipReasonNotFound      SkipReason = "not_found"      // 404不存在
	SkipReasonInvalidFormat SkipReason = "invalid_format" // 格式无效
	SkipReasonDuplicate     SkipReason = "duplicate"      // 重复
	SkipReasonAccessDenied  SkipReason = "access_denied"  // 访问拒绝
	SkipReasonGeoBlocked    SkipReason = "geo_blocked"    // 地区限制
	SkipReasonNoUpdate      SkipReason = "no_update"      // 无内容更新
	SkipReasonOther         SkipReason = "other"          // 其他原因
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
	Operation        string        `json:"operation,omitempty"`     // import 或 sync
	TotalPodcasts    int           `json:"total_podcasts"`          // 总播客数
	SuccessPodcasts  int           `json:"success_podcasts"`        // 成功同步的播客数
	FailedPodcasts   int           `json:"failed_podcasts"`         // 失败的播客数
	SkippedPodcasts  int           `json:"skipped_podcasts"`        // 跳过的播客数
	StubPodcasts     int           `json:"stub_podcasts,omitempty"` // 导入时因临时错误创建的待同步记录
	NoUpdatePodcasts int           `json:"no_update_podcasts"`      // 无更新的播客数
	TotalEpisodes    int           `json:"total_episodes"`          // 同步的总单集数
	NewEpisodes      int           `json:"new_episodes"`            // 新增的单集数
	UpdatedEpisodes  int           `json:"updated_episodes"`        // 更新的单集数
	Duration         time.Duration `json:"duration"`                // 总耗时
}

// LogProgressReporter 使用log的进度报告器
type LogProgressReporter struct{}

func NewLogProgressReporter() *LogProgressReporter {
	return &LogProgressReporter{}
}

func (r *LogProgressReporter) Report(message string) {
	logger.Infof("[INFO] %s", message)
}

func (r *LogProgressReporter) ReportSuccess(message string) {
	logger.Infof("[SUCCESS] ✅ %s", message)
}

func (r *LogProgressReporter) ReportError(message string) {
	logger.Errorf("[ERROR] ❌ %s", message)
}

func (r *LogProgressReporter) ReportProgress(current, total int, message string) {
	logger.Infof("[%d/%d] %s", current, total, message)
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
	logger.Infof("[SKIP] %s %s", icon, message)
}

func (r *LogProgressReporter) Close() {
	// Nothing to close for log reporter
}

func (r *LogProgressReporter) ReportSummary(summary *SyncSummary) {
	logger.Info("\n" + strings.Repeat("=", 60))
	logger.Info("📊 同步完成汇总")
	logger.Info(strings.Repeat("=", 60))
	logger.Infof("总播客数: %d", summary.TotalPodcasts)
	logger.Infof("✅ 成功: %d", summary.SuccessPodcasts)
	logger.Infof("❌ 失败: %d", summary.FailedPodcasts)
	logger.Infof("⏭️  跳过: %d", summary.SkippedPodcasts)
	if summary.NoUpdatePodcasts > 0 {
		logger.Infof("  └─ 无更新: %d", summary.NoUpdatePodcasts)
	}
	if summary.TotalEpisodes > 0 || summary.NewEpisodes > 0 || summary.UpdatedEpisodes > 0 {
		logger.Info("\n📝 单集统计:")
		logger.Infof("  总处理: %d", summary.TotalEpisodes)
		logger.Infof("  新增: %d", summary.NewEpisodes)
		logger.Infof("  更新: %d", summary.UpdatedEpisodes)
	}
	logger.Infof("⏱️  总耗时: %s", formatDuration(summary.Duration))
	logger.Info(strings.Repeat("=", 60))
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

func summaryCompletionLabel(operation string) string {
	if operation == "import" {
		return "导入完成"
	}
	return "同步完成"
}

func truncateMessage(message string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}

	runes := []rune(message)
	if len(runes) <= maxRunes {
		return message
	}

	if maxRunes <= 3 {
		return string(runes[:maxRunes])
	}

	return string(runes[:maxRunes-3]) + "..."
}

// SSEMessage SSE消息格式
type SSEMessage struct {
	Type    LogMessageType         `json:"type"`
	Message string                 `json:"message"`
	Current int                    `json:"current,omitempty"`
	Total   int                    `json:"total,omitempty"`
	Reason  string                 `json:"reason,omitempty"` // 跳过原因
	Data    map[string]interface{} `json:"data,omitempty"`   // 用于summary等复杂消息的额外数据
}

// SSEProgressReporter 使用Server-Sent Events的进度报告器
type SSEProgressReporter struct {
	writer         io.Writer
	flusher        http.Flusher
	mu             sync.Mutex
	closed         bool
	lastSendTime   time.Time
	messageCounter int // 消息计数器，用于节流
}

func NewSSEProgressReporter(writer io.Writer) *SSEProgressReporter {
	reporter := &SSEProgressReporter{
		writer:       writer,
		lastSendTime: time.Now(),
	}

	// 尝试获取Flusher接口（用于立即发送数据）
	if flusher, ok := writer.(http.Flusher); ok {
		reporter.flusher = flusher
	}

	return reporter
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
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return
	}

	// 先截断过长的消息以防止SSE传输问题
	const maxMessageLength = 500
	msg.Message = truncateMessage(msg.Message, maxMessageLength)

	data, err := json.Marshal(msg)
	if err != nil {
		// JSON序列化失败时，尝试发送简单的错误消息
		fmt.Fprintf(r.writer, "data: {\"type\":\"error\",\"message\":\"Failed to marshal progress message\"}\n\n")
		if r.flusher != nil {
			r.flusher.Flush()
		}
		return
	}

	// 检查序列化后的总大小（包括"data: "前缀和"\n\n"后缀）
	const maxTotalSize = 4000
	const prefixAndSuffix = len("data: ") + len("\n\n")

	if len(data)+prefixAndSuffix > maxTotalSize {
		// 如果JSON太长，首先尝试移除Data字段
		if msg.Data != nil {
			msg.Data = nil
			data, _ = json.Marshal(msg)
		}

		// 再次检查
		if len(data)+prefixAndSuffix > maxTotalSize {
			// 如果还是太长，尝试只保留消息类型和截断的消息
			truncatedMsg := msg.Message
			truncatedMsg = truncateMessage(truncatedMsg, 100)
			simpleMsg := SSEMessage{
				Type:    msg.Type,
				Message: truncatedMsg,
			}
			data, _ = json.Marshal(simpleMsg)
		}

		// 最后检查
		if len(data)+prefixAndSuffix > maxTotalSize {
			// 如果还是太长，只保留类型
			minimalMsg := SSEMessage{
				Type:    msg.Type,
				Message: "...",
			}
			data, _ = json.Marshal(minimalMsg)
		}
	}

	// 增加消息计数器
	r.messageCounter++

	// 节流：对于某些高频消息，跳过部分发送
	// 更激进的节流策略
	if msg.Type == LogTypeSuccess && msg.Message != "" {
		// 检查是否是"新增: 0, 更新: X"这种消息
		if strings.Contains(msg.Message, "新增: 0, 更新:") && !strings.Contains(msg.Message, "完成") {
			// 这种消息太多，每10条只发1条（之前是5）
			if r.messageCounter%10 != 0 {
				return
			}
		}
	}

	// 对于skip消息也进行节流（每3条发1条）
	if (msg.Type == LogTypeSkipNoUpd || msg.Type == LogTypeSkipOther) && r.messageCounter%3 != 0 {
		return
	}

	// SSE格式: "data: <json>\n\n"
	// 使用安全的写入方式，检查写入错误
	_, writeErr := fmt.Fprintf(r.writer, "data: %s\n\n", string(data))
	if writeErr != nil {
		// 写入失败时静默返回，不记录日志（避免日志爆炸）
		return
	}

	// 立即刷新缓冲区，确保数据发送到客户端
	if r.flusher != nil {
		r.flusher.Flush()
	}

	// 更新最后发送时间
	r.lastSendTime = time.Now()

	// 如果发送速度过快，添加延迟以避免压垮客户端
	// 每发送20条消息，暂停10ms
	if r.messageCounter%20 == 0 {
		time.Sleep(10 * time.Millisecond)
	}
}

func (r *SSEProgressReporter) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return
	}

	r.closed = true

	// 发送结束标记
	if _, err := fmt.Fprintf(r.writer, "data: [DONE]\n\n"); err != nil {
		logger.Errorf("[ERROR] Failed to send SSE close message: %v", err)
	}

	// 立即刷新
	if r.flusher != nil {
		r.flusher.Flush()
	}
}

func (r *SSEProgressReporter) ReportSummary(summary *SyncSummary) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return
	}

	doneLabel := summaryCompletionLabel(summary.Operation)
	// 创建基础消息
	msg := SSEMessage{
		Type: "summary",
		Message: fmt.Sprintf("%s！成功: %d, 失败: %d, 跳过: %d, 耗时: %s",
			doneLabel,
			summary.SuccessPodcasts,
			summary.FailedPodcasts,
			summary.SkippedPodcasts,
			formatDuration(summary.Duration),
		),
		Data: map[string]interface{}{
			"operation":          summary.Operation,
			"total_podcasts":     summary.TotalPodcasts,
			"success_podcasts":   summary.SuccessPodcasts,
			"failed_podcasts":    summary.FailedPodcasts,
			"skipped_podcasts":   summary.SkippedPodcasts,
			"stub_podcasts":      summary.StubPodcasts,
			"no_update_podcasts": summary.NoUpdatePodcasts,
			"total_episodes":     summary.TotalEpisodes,
			"new_episodes":       summary.NewEpisodes,
			"updated_episodes":   summary.UpdatedEpisodes,
			"duration":           formatDuration(summary.Duration),
		},
	}

	// 先截断消息文本
	const maxMessageLength = 200
	msg.Message = truncateMessage(msg.Message, maxMessageLength)

	// 尝试序列化并检查大小
	data, err := json.Marshal(msg)
	if err != nil {
		// JSON序列化失败，发送简单错误
		fmt.Fprintf(r.writer, "data: {\"type\":\"error\",\"message\":\"Failed to marshal summary\"}\n\n")
		return
	}

	// 检查序列化后的总大小（包括"data: "前缀和"\n\n"后缀）
	const maxTotalSize = 4000
	const prefixAndSuffix = len("data: ") + len("\n\n")

	if len(data)+prefixAndSuffix > maxTotalSize {
		// 如果太大，移除Data字段，只保留基本统计信息
		msg.Data = nil
		msg.Message = fmt.Sprintf("同步完成！成功: %d, 失败: %d, 跳过: %d",
			summary.SuccessPodcasts,
			summary.FailedPodcasts,
			summary.SkippedPodcasts,
		)

		data, _ = json.Marshal(msg)

		// 再次检查
		if len(data)+prefixAndSuffix > maxTotalSize {
			// 如果还是太大，进一步截断消息
			msg.Message = "同步完成"
			data, _ = json.Marshal(msg)
		}
	}

	// 安全写入
	if _, writeErr := fmt.Fprintf(r.writer, "data: %s\n\n", string(data)); writeErr != nil {
		logger.Errorf("[ERROR] Failed to write SSE summary: %v", writeErr)
	}

	// 立即刷新
	if r.flusher != nil {
		r.flusher.Flush()
	}

	// 更新最后发送时间
	r.lastSendTime = time.Now()
}

// SilentProgressReporter 静默的进度报告器（用于单集同步，不输出详细日志）
type SilentProgressReporter struct {
	parentReporter ProgressReporter // 父reporter，用于汇总报告
}

func NewSilentProgressReporter(parent ProgressReporter) *SilentProgressReporter {
	return &SilentProgressReporter{
		parentReporter: parent,
	}
}

func (r *SilentProgressReporter) Report(message string) {
	// 静默，不输出
}

func (r *SilentProgressReporter) ReportSuccess(message string) {
	// 静默，不输出
}

func (r *SilentProgressReporter) ReportError(message string) {
	// 静默，不输出
}

func (r *SilentProgressReporter) ReportProgress(current, total int, message string) {
	// 静默，不输出
}

func (r *SilentProgressReporter) ReportSkip(reason SkipReason, message string) {
	// 静默，不输出
}

func (r *SilentProgressReporter) ReportSummary(summary *SyncSummary) {
	// 将汇总报告传递给父reporter
	if r.parentReporter != nil {
		r.parentReporter.ReportSummary(summary)
	}
}

func (r *SilentProgressReporter) Close() {
	// Nothing to close
}
