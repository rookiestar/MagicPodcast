package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"sync"
	"time"

	syncpkg "magicpodcast/internal/sync"

	"github.com/gin-gonic/gin"
)

// SSEProgressMessage SSE进度消息
type SSEProgressMessage struct {
	Type      string `json:"type"`       // "info", "success", "error", "progress", "complete"
	Current   int    `json:"current"`    // 当前进度（仅type为progress时）
	Total     int    `json:"total"`      // 总数（仅type为progress时）
	Message   string `json:"message"`    // 消息内容
	Timestamp string `json:"timestamp"`  // 时间戳
}

// SSEProgressReporter SSE进度报告器
type SSEProgressReporter struct {
	mu         sync.Mutex      // 保护并发写入
	flusher    http.Flusher
	writer     http.ResponseWriter
	closed     bool
	keepalive  *time.Ticker
	stopKeepalive chan struct{}
}

func NewSSEProgressReporter(c *gin.Context) *SSEProgressReporter {
	// 设置SSE响应头
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("Access-Control-Allow-Origin", "*")

	reporter := &SSEProgressReporter{
		flusher: c.Writer.(http.Flusher),
		writer:  c.Writer,
		closed:  false,
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
				if r.closed {
					return
				}
				// 发送SSE注释（客户端会忽略，但保持连接活跃）
				// 使用注释格式：: comment\n\n
				fmt.Fprintf(r.writer, ": ping\n\n")
				r.flusher.Flush()
				log.Printf("[SSE] 发送keepalive ping")
			case <-r.stopKeepalive:
				return
			}
		}
	}()
}

func (r *SSEProgressReporter) send(msgType string, message string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		log.Printf("[SSE] Skip sending (closed): %s - %s", msgType, message)
		return
	}

	log.Printf("[SSE] Sending: %s - %s", msgType, message)

	msg := SSEProgressMessage{
		Type:      msgType,
		Message:   message,
		Timestamp: time.Now().Format("15:04:05"),
	}

	data, _ := json.Marshal(msg)
	fmt.Fprintf(r.writer, "data: %s\n\n", data)
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
		log.Printf("[SSE] Skip ReportProgress (closed): [%d/%d] %s", current, total, message)
		return
	}

	log.Printf("[SSE] ReportProgress: [%d/%d] %s", current, total, message)

	msg := SSEProgressMessage{
		Type:      "progress",
		Current:   current,
		Total:     total,
		Message:   message,
		Timestamp: time.Now().Format("15:04:05"),
	}

	data, _ := json.Marshal(msg)
	fmt.Fprintf(r.writer, "data: %s\n\n", data)
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

	fmt.Fprintf(r.writer, "data: %s\n\n", data)
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
		log.Printf("[SSE] 停止keepalive")
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
	tempFilePath := filepath.Join(tempDir, file.Filename)

	if err := c.SaveUploadedFile(file, tempFilePath); err != nil {
		c.SSEvent("", "error")
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "保存文件失败",
		})
		return
	}

	// 创建SSE reporter
	reporter := NewSSEProgressReporter(c)
	defer reporter.Close()

	// 在goroutine中执行导入，避免阻塞
	// 但由于SSE需要保持连接，我们在这里同步执行
	log.Printf("[SSE] 开始导入OPML: %s", file.Filename)
	result, err := h.syncService.ImportOPMLWithProgressAndConfig(tempFilePath, reporter, syncpkg.DefaultImportConfig)
	if err != nil {
		log.Printf("[SSE] 导入失败: %v", err)
		reporter.ReportError("导入失败: " + err.Error())
		return
	}

	log.Printf("[SSE] 导入成功，发送完成消息: 成功=%d 失败=%d",
		result.SuccessPodcasts, result.FailedPodcasts)

	// 发送完成消息
	reporter.ReportSuccess(fmt.Sprintf("导入完成！成功: %d, 失败: %d",
		result.SuccessPodcasts, result.FailedPodcasts))

	// 发送最终结果
	resultMsg := SSEProgressMessage{
		Type:      "complete",
		Message:   "导入完成",
		Timestamp: time.Now().Format("15:04:05"),
	}
	resultData, _ := json.Marshal(resultMsg)
	fmt.Fprintf(c.Writer, "data: %s\n\n", resultData)
	c.Writer.(http.Flusher).Flush()

	log.Printf("[SSE] 已发送complete消息")
}
