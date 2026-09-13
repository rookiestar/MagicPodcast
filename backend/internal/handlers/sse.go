package handlers

import (
	"encoding/json"
	"fmt"
	"magicpodcast/internal/logger"
	"net/http"
	"sync"
	"time"

	"magicpodcast/internal/middleware"
	"magicpodcast/internal/opml"
	syncpkg "magicpodcast/internal/sync"

	"github.com/gin-gonic/gin"
)

// SSEProgressMessage SSE进度消息
type SSEProgressMessage struct {
	Type      string `json:"type"`      // "info", "success", "error", "progress", "complete"
	Current   int    `json:"current"`   // 当前进度（仅type为progress时）
	Total     int    `json:"total"`     // 总数（仅type为progress时）
	Message   string `json:"message"`   // 消息内容
	Timestamp string `json:"timestamp"` // 时间戳
}

// SSEProgressReporter SSE进度报告器
type SSEProgressReporter struct {
	mu      sync.Mutex // 保护并发写入
	flusher http.Flusher
	writer  http.ResponseWriter
	// closed 只表示写路径停止发送业务消息（写出失败或正常关闭）；
	// released 表示 keepalive ticker/goroutine 已释放。两者分离后，
	// 写出失败不会让 Close 提前返回而泄漏保活循环（#398 R13）。
	closed        bool
	released      bool
	keepalive     *time.Ticker
	stopKeepalive chan struct{}
}

func NewSSEProgressReporter(c *gin.Context) *SSEProgressReporter {
	// 设置SSE响应头
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("Access-Control-Allow-Origin", "*")

	reporter := &SSEProgressReporter{
		flusher:       c.Writer.(http.Flusher),
		writer:        c.Writer,
		closed:        false,
		stopKeepalive: make(chan struct{}),
	}

	// 启动keepalive goroutine，每10秒发送一次注释消息
	reporter.startKeepalive()

	return reporter
}

// startKeepalive 启动keepalive机制
func (r *SSEProgressReporter) startKeepalive() {
	// 使用更频繁的keepalive（10秒），防止被60秒超时断开
	r.keepalive = time.NewTicker(10 * time.Second)

	go func() {
		for {
			select {
			case <-r.keepalive.C:
				r.sendKeepalive()
			case <-r.stopKeepalive:
				return
			}
		}
	}()
}

func (r *SSEProgressReporter) sendKeepalive() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return
	}

	// 发送SSE注释（客户端会忽略，但保持连接活跃）
	// 使用注释格式：: comment\n\n
	if _, err := fmt.Fprintf(r.writer, ": ping\n\n"); err != nil {
		logger.Warnf("[SSE] Keepalive write error: %v", err)
		r.closed = true
		return
	}
	r.flusher.Flush()
	logger.Debugf("[SSE] 发送keepalive ping")
}

func (r *SSEProgressReporter) send(msgType string, message string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		logger.Debugf("[SSE] Skip sending (closed): %s - %s", msgType, message)
		return
	}

	logger.Debugf("[SSE] Sending: %s - %s", msgType, message)

	msg := SSEProgressMessage{
		Type:      msgType,
		Message:   message,
		Timestamp: time.Now().Format("15:04:05"),
	}

	data, _ := json.Marshal(msg)
	if _, err := fmt.Fprintf(r.writer, "data: %s\n\n", data); err != nil {
		logger.Warnf("[SSE] Write error: %v", err)
		r.closed = true
		return
	}
	r.flusher.Flush()
}

func (r *SSEProgressReporter) Report(message string) {
	r.send("info", message)
}

func (r *SSEProgressReporter) ReportSuccess(message string) {
	r.send("success", message)
}

func (r *SSEProgressReporter) ReportError(message string) {
	r.send("error", message)
}

func (r *SSEProgressReporter) ReportProgress(current, total int, message string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		logger.Debugf("[SSE] Skip ReportProgress (closed): [%d/%d] %s", current, total, message)
		return
	}

	logger.Debugf("[SSE] ReportProgress: [%d/%d] %s", current, total, message)

	msg := SSEProgressMessage{
		Type:      "progress",
		Current:   current,
		Total:     total,
		Message:   message,
		Timestamp: time.Now().Format("15:04:05"),
	}

	data, _ := json.Marshal(msg)
	if _, err := fmt.Fprintf(r.writer, "data: %s\n\n", data); err != nil {
		logger.Warnf("[SSE] Write error in ReportProgress: %v", err)
		r.closed = true
		return
	}
	r.flusher.Flush()
}

func (r *SSEProgressReporter) ReportSkip(reason syncpkg.SkipReason, message string) {
	// 根据跳过原因决定消息类型
	var msgType string
	switch reason {
	case syncpkg.SkipReasonPaid:
		msgType = "skip_paid"
	case syncpkg.SkipReasonCertificate:
		msgType = "skip_cert"
	case syncpkg.SkipReasonNotFound:
		msgType = "skip_not_found"
	case syncpkg.SkipReasonAccessDenied:
		msgType = "skip_access_denied"
	case syncpkg.SkipReasonGeoBlocked:
		msgType = "skip_geo_blocked"
	case syncpkg.SkipReasonDuplicate:
		msgType = "skip_duplicate"
	case syncpkg.SkipReasonInvalidFormat:
		msgType = "skip_invalid"
	case syncpkg.SkipReasonNoUpdate:
		msgType = "skip_no_update"
	default:
		msgType = "skip_other"
	}

	// 添加reason到消息中
	r.sendWithType(msgType, message, reason)
}

func (r *SSEProgressReporter) sendWithType(msgType string, message string, reason syncpkg.SkipReason) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return
	}

	msg := SSEProgressMessage{
		Type:      msgType,
		Message:   message,
		Timestamp: time.Now().Format("15:04:05"),
	}

	// 添加reason字段
	data, _ := json.Marshal(map[string]interface{}{
		"type":      msgType,
		"message":   message,
		"timestamp": msg.Timestamp,
		"reason":    string(reason),
	})

	if _, err := fmt.Fprintf(r.writer, "data: %s\n\n", data); err != nil {
		logger.Warnf("[SSE] Write error in sendWithType: %v", err)
		r.closed = true
		return
	}
	r.flusher.Flush()
}

func (r *SSEProgressReporter) ReportSummary(summary *syncpkg.SyncSummary) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		logger.Debugf("[SSE] Skip ReportSummary (closed)")
		return
	}

	doneLabel := "同步完成"
	if summary.Operation == "import" {
		doneLabel = "导入完成"
	}

	// 构建汇总消息
	summaryMsg := map[string]interface{}{
		"type": "summary",
		"message": fmt.Sprintf("%s！成功: %d, 失败: %d, 跳过: %d",
			doneLabel,
			summary.SuccessPodcasts,
			summary.FailedPodcasts,
			summary.SkippedPodcasts),
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
		"duration":           summary.Duration.String(),
		"timestamp":          time.Now().Format("15:04:05"),
	}

	data, _ := json.Marshal(summaryMsg)
	if _, err := fmt.Fprintf(r.writer, "data: %s\n\n", data); err != nil {
		logger.Warnf("[SSE] Write error in ReportSummary: %v", err)
		r.closed = true
		return
	}
	r.flusher.Flush()

	logger.Debugf("[SSE] ReportSummary: 总=%d 成功=%d 失败=%d 跳过=%d 无更新=%d",
		summary.TotalPodcasts, summary.SuccessPodcasts, summary.FailedPodcasts,
		summary.SkippedPodcasts, summary.NoUpdatePodcasts)
}

func (r *SSEProgressReporter) ReportComplete(message string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return
	}

	msg := SSEProgressMessage{
		Type:      "complete",
		Message:   message,
		Timestamp: time.Now().Format("15:04:05"),
	}
	data, _ := json.Marshal(msg)
	if _, err := fmt.Fprintf(r.writer, "data: %s\n\n", data); err != nil {
		logger.Warnf("[SSE] Write error in ReportComplete: %v", err)
		r.closed = true
		return
	}
	r.flusher.Flush()
}

func (r *SSEProgressReporter) ReportDone() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return
	}

	if _, err := fmt.Fprintf(r.writer, "data: [DONE]\n\n"); err != nil {
		logger.Warnf("[SSE] Write error in ReportDone: %v", err)
		r.closed = true
		return
	}
	r.flusher.Flush()
}

// ReportTaskID 在流刚开始时发送任务标识，客户端据此恢复/查询同一任务。
func (r *SSEProgressReporter) ReportTaskID(taskID uint) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return
	}

	data, _ := json.Marshal(map[string]interface{}{
		"type":    "task",
		"task_id": taskID,
		"message": fmt.Sprintf("导入任务 #%d 已开始，断开后可按任务查询", taskID),
	})
	if _, err := fmt.Fprintf(r.writer, "data: %s\n\n", data); err != nil {
		logger.Warnf("[SSE] Write error in ReportTaskID: %v", err)
		r.closed = true
		return
	}
	r.flusher.Flush()
}

// Close 停止业务写路径并释放 keepalive 资源。无论此前是否已因写出失败
// 置为 closed，ticker/goroutine 都必须被释放且只释放一次（#398 R13）。
func (r *SSEProgressReporter) Close() {
	r.mu.Lock()
	r.closed = true
	if r.released {
		r.mu.Unlock()
		return
	}
	r.released = true
	keepalive := r.keepalive
	stopKeepalive := r.stopKeepalive
	r.mu.Unlock()

	if keepalive != nil {
		keepalive.Stop()
	}
	if stopKeepalive != nil {
		close(stopKeepalive)
	}
	logger.Debugf("[SSE] 停止keepalive")
}

// ImportOPMLSSE 导入OPML文件（SSE流式响应）
// POST /api/v1/sync/import-sse
func (h *SyncHandler) ImportOPMLSSE(c *gin.Context) {
	if !middleware.RequireConfirmationText(
		c,
		middleware.ConfirmationTextFromHeaderOrForm(c),
		"IMPORT OPML",
		"导入所选 OPML 文件并写入播客订阅数据",
	) {
		return
	}

	// 获取上传的文件
	file, err := c.FormFile("opml_file")
	if err != nil {
		if middleware.RequestBodyLimitExceeded(c) {
			middleware.RequestTooLargeResponse(c, middleware.DefaultUploadRequestLimitBytes)
			return
		}
		middleware.BadRequestResponse(c, "INVALID_FILE", "OPML文件上传失败，请确保使用multipart/form-data格式")
		return
	}
	// 校验失败在发送 SSE 响应头前返回与普通入口一致的 JSON 错误结构。
	if validation := validateOPMLUpload(file); validation != nil {
		validation.respond(c)
		return
	}

	tempFilePath, cleanup, ok := saveOPMLUpload(c, file)
	if !ok {
		return
	}
	defer cleanup()

	decisions, err := parseImportDecisions(c.PostForm("decisions"))
	if err != nil {
		middleware.BadRequestResponse(c, "INVALID_DECISIONS", err.Error())
		return
	}

	// 解析在任何 SSE 响应头之前完成：解析失败与普通入口返回同一 JSON
	// 错误结构，不留下半开的流（#398 R11）。
	outlines, err := (opml.NewParser()).ParseFile(tempFilePath)
	if err != nil {
		logger.Warnf("[SSE] OPML解析失败: %v", err)
		respondOPMLParseError(c, err)
		return
	}

	// 创建SSE reporter
	reporter := NewSSEProgressReporter(c)
	defer reporter.Close()

	logger.Infof("[SSE] 开始导入OPML（本地匹配 + 在线同步）: %s", file.Filename)
	// 任务记录先建立：流首条消息携带任务 ID，页面断开后可按任务恢复。
	task, wrapped := h.startImportTask(file.Filename, len(outlines), reporter)
	if task != nil {
		reporter.ReportTaskID(task.ID)
	}
	result, err := h.runImport(outlines, wrapped, decisions, task)
	if err != nil {
		logger.Warnf("[SSE] 导入失败: %v", err)
		reporter.ReportError("导入失败: " + err.Error())
		return
	}

	if result.TotalPodcasts == 0 {
		reporter.Report(opmlResultMessage(0, 0, 0, 0))
	}
	logger.Infof("[SSE] 导入完成，summary 已发送: 成功=%d 失败=%d",
		result.SuccessPodcasts, result.FailedPodcasts)
}

// SyncPodcastsMetadataSSE 同步所有播客的元数据（SSE流式响应）
// POST /api/v1/sync/podcasts/metadata-sse
func (h *SyncHandler) SyncPodcastsMetadataSSE(c *gin.Context) {
	var confirmation middleware.ConfirmationRequest
	_ = c.ShouldBindJSON(&confirmation)
	if !middleware.RequireConfirmationText(
		c,
		confirmation.ConfirmationText,
		"SYNC ALL",
		"刷新全部订阅播客的资料，并按各节目同步范围写入单集（可能新增或更新单集内容），可能耗时较长",
	) {
		return
	}

	// 添加panic恢复机制
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("元数据同步发生panic: %v", r)
			// 尝试发送错误消息到客户端
			errorMsg := SSEProgressMessage{
				Type:      "error",
				Message:   "同步过程中发生内部错误",
				Timestamp: time.Now().Format("15:04:05"),
			}
			errorData, _ := json.Marshal(errorMsg)
			fmt.Fprintf(c.Writer, "data: %s\n\n", errorData)
			c.Writer.(http.Flusher).Flush()
		}
	}()

	// 创建SSE reporter
	reporter := NewSSEProgressReporter(c)
	defer reporter.Close()

	logger.Infof("[SSE] 开始同步所有播客元数据")

	// 执行同步元数据任务
	err := h.syncService.SyncPodcastsMetadataSSE(reporter)
	if err != nil {
		logger.Warnf("[SSE] 同步元数据失败: %v", err)
		reporter.ReportError("同步元数据失败: " + err.Error())
		return
	}

	logger.Infof("[SSE] 同步元数据成功")

	// 发送结束标记
	reporter.ReportDone()

	logger.Debugf("[SSE] 已完成元数据同步")
}
