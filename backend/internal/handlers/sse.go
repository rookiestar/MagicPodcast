package handlers

import (
	"encoding/json"
	"fmt"
	"magicpodcast/internal/logger"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

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
	mu            sync.Mutex // 保护并发写入
	flusher       http.Flusher
	writer        http.ResponseWriter
	closed        bool
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

	// 启动keepalive goroutine，每15秒发送一次注释消息
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

	// 构建汇总消息
	summaryMsg := map[string]interface{}{
		"type": "summary",
		"message": fmt.Sprintf("同步完成！成功: %d, 失败: %d, 跳过: %d",
			summary.SuccessPodcasts,
			summary.FailedPodcasts,
			summary.SkippedPodcasts),
		"total_podcasts":     summary.TotalPodcasts,
		"success_podcasts":   summary.SuccessPodcasts,
		"failed_podcasts":    summary.FailedPodcasts,
		"skipped_podcasts":   summary.SkippedPodcasts,
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

func (r *SSEProgressReporter) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return
	}

	r.closed = true

	// 停止keepalive
	if r.keepalive != nil {
		r.keepalive.Stop()
		close(r.stopKeepalive)
		logger.Debugf("[SSE] 停止keepalive")
	}
}

// ImportOPMLSSE 导入OPML文件（SSE流式响应）
// POST /api/v1/sync/import-sse
func (h *SyncHandler) ImportOPMLSSE(c *gin.Context) {
	// 获取上传的文件
	file, err := c.FormFile("opml_file")
	if err != nil {
		c.SSEvent("", "error")
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "OPML文件上传失败，请确保使用multipart/form-data格式",
		})
		return
	}

	// 验证文件扩展名
	ext := filepath.Ext(file.Filename)
	if ext != ".opml" && ext != ".xml" {
		c.SSEvent("", "error")
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "OPML文件格式不正确，请上传.opml或.xml文件",
		})
		return
	}

	// 保存到临时文件
	tempDir := filepath.Join(".", "data", "temp")
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		c.SSEvent("", "error")
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "创建临时目录失败",
		})
		return
	}

	tempFileName := fmt.Sprintf("%d_%s", time.Now().UnixNano(), filepath.Base(file.Filename))
	tempFilePath := filepath.Join(tempDir, tempFileName)

	if err := c.SaveUploadedFile(file, tempFilePath); err != nil {
		c.SSEvent("", "error")
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "保存文件失败",
		})
		return
	}
	defer func() {
		if err := os.Remove(tempFilePath); err != nil {
			logger.Infof("⚠️  清理临时OPML文件失败: %v", err)
		}
	}()

	// 创建SSE reporter
	reporter := NewSSEProgressReporter(c)
	defer reporter.Close()

	// 在goroutine中执行导入，避免阻塞
	// 但由于SSE需要保持连接，我们在这里同步执行
	logger.Infof("[SSE] 开始导入OPML（仅本地数据库）: %s", file.Filename)
	result, err := h.syncService.ImportOPMLFromPodcastIndexOnly(tempFilePath, reporter)
	if err != nil {
		logger.Warnf("[SSE] 导入失败: %v", err)
		reporter.ReportError("导入失败: " + err.Error())
		return
	}

	logger.Infof("[SSE] 导入成功，发送完成消息: 成功=%d 失败=%d",
		result.SuccessPodcasts, result.FailedPodcasts)

	// 发送完成消息
	reporter.ReportSuccess(fmt.Sprintf("导入完成！成功: %d, 失败: %d",
		result.SuccessPodcasts, result.FailedPodcasts))

	reporter.ReportComplete("导入完成")

	logger.Debugf("[SSE] 已发送complete消息")
}

// SyncPodcastsMetadataSSE 同步所有播客的元数据（SSE流式响应）
// POST /api/v1/sync/podcasts/metadata-sse
func (h *SyncHandler) SyncPodcastsMetadataSSE(c *gin.Context) {
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
