package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"magicpodcast/internal/config"
	"magicpodcast/internal/database"
	"magicpodcast/internal/logger"
	"magicpodcast/internal/middleware"
	"magicpodcast/internal/models"
	"magicpodcast/internal/opml"
	"magicpodcast/internal/sync"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// SyncHandler 同步处理器
type SyncHandler struct {
	syncService *sync.Service
	db          *gorm.DB
}

// NewSyncHandler 创建同步处理器
func NewSyncHandler() (*SyncHandler, error) {
	db := database.GetDB()

	// 从配置读取PodcastIndex数据库路径
	cfg := config.Get()
	podcastIndexPath := cfg.PodcastIndex.Path

	// 创建同步服务
	syncService, err := sync.NewService(db, podcastIndexPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create sync service: %w", err)
	}

	return &SyncHandler{
		syncService: syncService,
		db:          db,
	}, nil
}

// ImportOPMLRequest OPML导入请求
type ImportOPMLRequest struct {
	FilePath string `json:"file_path"` // 用于已上传的文件路径
}

// SyncStatusResponse 同步状态响应
type SyncStatusResponse struct {
	LastSyncTime   *time.Time     `json:"last_sync_time"`
	TotalPodcasts  int            `json:"total_podcasts"`
	PodcastSources map[string]int `json:"podcast_sources"` // 数据来源统计
}

// parseImportDecisions 解析用户对需确认条目的显式决定（xmlUrl → confirm）。
func parseImportDecisions(raw string) (map[string]string, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var decisions map[string]string
	if err := json.Unmarshal([]byte(raw), &decisions); err != nil {
		return nil, fmt.Errorf("decisions 字段必须是 JSON 对象")
	}
	return decisions, nil
}

// PreviewOPMLImport OPML 导入预览：只做差异核对，不抓取、不写库。
// POST /api/v1/sync/import/preview
func (h *SyncHandler) PreviewOPMLImport(c *gin.Context) {
	file, err := c.FormFile("opml_file")
	if err != nil {
		if middleware.RequestBodyLimitExceeded(c) {
			middleware.RequestTooLargeResponse(c, middleware.DefaultUploadRequestLimitBytes)
			return
		}
		middleware.BadRequestResponse(c, "INVALID_FILE", "OPML文件上传失败，请确保使用multipart/form-data格式")
		return
	}
	if validation := validateOPMLUpload(file); validation != nil {
		validation.respond(c)
		return
	}

	tempFilePath, cleanup, ok := saveOPMLUpload(c, file)
	if !ok {
		return
	}
	defer cleanup()

	outlines, err := opml.NewParser().ParseFile(tempFilePath)
	if err != nil {
		respondOPMLParseError(c, err)
		return
	}

	preview, err := h.syncService.PreviewImportOPML(outlines)
	if err != nil {
		middleware.InternalErrorResponseWithCode(c, "PREVIEW_ERROR", fmt.Sprintf("预览失败: %v", err))
		return
	}

	c.JSON(200, gin.H{
		"success": true,
		"message": opmlResultMessage(preview.Total, preview.NewCount, 0, 0),
		"preview": preview,
	})
}

// ImportOPML 导入OPML文件
// POST /api/v1/sync/import
func (h *SyncHandler) ImportOPML(c *gin.Context) {
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
	if validation := validateOPMLUpload(file); validation != nil {
		validation.respond(c)
		return
	}

	decisions, err := parseImportDecisions(c.PostForm("decisions"))
	if err != nil {
		middleware.BadRequestResponse(c, "INVALID_DECISIONS", err.Error())
		return
	}

	logger.Infof("收到OPML文件: %s (%d bytes)", file.Filename, file.Size)

	tempFilePath, cleanup, ok := saveOPMLUpload(c, file)
	if !ok {
		return
	}
	defer cleanup()

	// 解析在业务写入前完成，解析失败与流式入口共用同一错误契约。
	outlines, err := opml.NewParser().ParseFile(tempFilePath)
	if err != nil {
		respondOPMLParseError(c, err)
		return
	}

	task, wrapped := h.startImportTask(file.Filename, len(outlines), sync.NewLogProgressReporter())
	result, runErr := h.runImport(outlines, wrapped, decisions, task)
	if runErr != nil {
		logger.Infof("导入失败: %v", runErr)
		middleware.InternalErrorResponseWithCode(c, "IMPORT_ERROR", fmt.Sprintf("导入失败: %v", runErr))
		return
	}

	logger.Infof("导入成功: %d/%d", result.SuccessPodcasts, result.TotalPodcasts)

	var taskID interface{}
	if task != nil {
		taskID = task.ID
	}
	c.JSON(200, gin.H{
		"success":            true,
		"task_id":            taskID,
		"message":            opmlResultMessage(result.TotalPodcasts, result.SuccessPodcasts, result.StubPodcasts, result.FailedPodcasts),
		"total_podcasts":     result.TotalPodcasts,
		"success_count":      result.SuccessPodcasts,
		"failed_count":       result.FailedPodcasts,
		"stub_podcasts":      result.StubPodcasts,
		"skipped_podcasts":   result.SkippedPodcasts,
		"merged_podcasts":    result.MergedPodcasts,
		"conflict_podcasts":  result.ConflictPodcasts,
		"unchanged_podcasts": result.UnchangedPodcasts,
		"entries":            result.Entries,
		"errors":             result.Errors,
	})
}

// startImportTask creates the required recovery record; nil prevents import writes.
func (h *SyncHandler) startImportTask(fileName string, total int, reporter sync.ProgressReporter) (*models.ImportTask, sync.ProgressReporter) {
	if h.db == nil {
		return nil, reporter
	}
	task, err := sync.CreateImportTask(h.db, fileName, total)
	if err != nil {
		logger.Warnf("创建导入任务失败，拒绝执行导入: %v", err)
		return nil, reporter
	}
	return task, sync.NewTaskProgressReporter(reporter, h.db, task.ID)
}

// startChildImportTask 创建持久化父链的重试任务记录（#417/#418）；创建失败
// 与普通入口一致地拒绝执行导入。
func (h *SyncHandler) startChildImportTask(parentTaskID uint, fileName string, total int, reporter sync.ProgressReporter) (*models.ImportTask, sync.ProgressReporter, error) {
	if h.db == nil {
		return nil, reporter, fmt.Errorf("导入任务存储不可用")
	}
	task, err := sync.CreateChildImportTask(h.db, parentTaskID, fileName, total)
	if err != nil {
		logger.Warnf("创建重试导入任务失败，拒绝执行导入: %v", err)
		return nil, reporter, err
	}
	return task, sync.NewTaskProgressReporter(reporter, h.db, task.ID), nil
}

// runImport 执行导入并保存任务终态；终态与逐条结果先落库再返回响应。
func (h *SyncHandler) runImport(outlines []opml.Outline, reporter sync.ProgressReporter, decisions map[string]string, task *models.ImportTask) (*sync.SyncResult, error) {
	if task == nil {
		return nil, fmt.Errorf("导入任务未能保存，未执行导入")
	}
	if err := sync.InitializeImportResults(reporter, outlines); err != nil {
		// Initialization happens before any subscription write. Persist the
		// failed terminal state as well, otherwise a background task would look
		// like a restart interruption on the next status read.
		if finErr := sync.FinalizeImportTask(h.db, task, nil, err); finErr != nil {
			logger.Errorf("保存导入初始化失败终态失败: task=%d err=%v", task.ID, finErr)
		}
		return nil, fmt.Errorf("保存导入范围失败: %w", err)
	}
	result, runErr := h.syncService.ImportOPMLOutlines(outlines, reporter, sync.DefaultImportConfig, decisions)
	if task != nil {
		if finErr := sync.FinalizeImportTask(h.db, task, result, runErr); finErr != nil {
			logger.Errorf("保存导入任务终态失败: task=%d err=%v", task.ID, finErr)
			return result, finErr
		}
	}
	if runErr == nil {
		sync.PublishImportSummary(reporter)
	}
	return result, runErr
}

// runImportInBackground executes a persisted import task after the HTTP
// request has returned. It deliberately does not use the request context:
// losing the browser connection must not stop the durable import task.
func (h *SyncHandler) runImportInBackground(outlines []opml.Outline, reporter sync.ProgressReporter, decisions map[string]string, task *models.ImportTask) {
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				runErr := fmt.Errorf("导入任务发生内部错误: %v", recovered)
				var current models.ImportTask
				alreadyFinalized := h.db != nil && h.db.First(&current, task.ID).Error == nil && current.Status != models.ImportTaskStatusRunning
				if !alreadyFinalized {
					if finErr := sync.FinalizeImportTask(h.db, task, nil, runErr); finErr != nil {
						logger.Errorf("保存导入 panic 终态失败: task=%d err=%v", task.ID, finErr)
					}
				}
				logger.Errorf("后台导入任务 panic: task=%d err=%v", task.ID, runErr)
			}
		}()
		if _, err := h.runImport(outlines, reporter, decisions, task); err != nil {
			logger.Warnf("后台导入任务失败: task=%d err=%v", task.ID, err)
		}
	}()
}

// GetImportTaskStatus 查询导入任务状态与逐条结果。
// GET /api/v1/sync/import/tasks/:id
func (h *SyncHandler) GetImportTaskStatus(c *gin.Context) {
	taskID, ok := ParseUintParam(c, "id")
	if !ok {
		return
	}
	if h.db == nil {
		middleware.NotFoundResponse(c, "IMPORT_TASK_NOT_FOUND", "导入任务不存在")
		return
	}
	task, entries, err := sync.GetImportTask(h.db, taskID)
	if err != nil {
		middleware.NotFoundResponse(c, "IMPORT_TASK_NOT_FOUND", "导入任务不存在")
		return
	}
	c.JSON(200, gin.H{
		"success": true,
		"task":    task,
		"entries": entries,
	})
}

// GetLatestImportTask 返回最近一次导入任务，用于页面刷新/断线后恢复。
// “没有历史任务”（record not found）与“读取失败”是两种状态：前者返回
// task:null，后者返回错误响应，前端据此区分空态与读取失败并保留已知结果。
// GET /api/v1/sync/import/tasks/latest
func (h *SyncHandler) GetLatestImportTask(c *gin.Context) {
	if h.db == nil {
		c.JSON(200, gin.H{"success": true, "task": nil, "entries": []gin.H{}})
		return
	}
	task, entries, err := sync.GetLatestImportTask(h.db)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(200, gin.H{"success": true, "task": nil, "entries": []gin.H{}})
			return
		}
		middleware.InternalErrorResponseWithCode(c, "IMPORT_TASK_QUERY_FAILED", "最近导入任务读取失败，请重试")
		return
	}
	c.JSON(200, gin.H{
		"success": true,
		"task":    task,
		"entries": entries,
	})
}

// opmlResultMessage 生成两个导入入口共用的结果文案；空 OPML 明确显示
// 0 条而不是宣称新增成功（#398 R11）。
func opmlResultMessage(total, success, stub, failed int) string {
	if total == 0 {
		return "OPML文件中没有订阅条目（0 条），未新增节目"
	}
	return fmt.Sprintf("导入完成：成功 %d，待同步 %d，失败 %d", success, stub, failed)
}

// SyncSubscriptions 同步所有订阅（定时任务手动触发）
// POST /api/v1/sync/subscriptions
func (h *SyncHandler) SyncSubscriptions(c *gin.Context) {
	if !middleware.RequireConfirmationText(
		c,
		middleware.ConfirmationTextFromHeaderOrForm(c),
		"SYNC SUBSCRIPTIONS",
		"同步全部订阅并写入播客与单集数据",
	) {
		return
	}

	logger.Info("开始同步所有订阅...")

	result, err := h.syncService.SyncAllPodcasts()
	if err != nil {
		logger.Infof("同步失败: %v", err)
		middleware.InternalErrorResponseWithCode(c, "SYNC_ERROR", fmt.Sprintf("同步失败: %v", err))
		return
	}

	logger.Infof("同步完成: %d 个新单集", result.NewEpisodes)

	c.JSON(200, gin.H{
		"success":        true,
		"message":        fmt.Sprintf("同步完成，获取 %d 个新单集", result.NewEpisodes),
		"total_podcasts": result.TotalPodcasts,
		"success_count":  result.SuccessPodcasts,
		"failed_count":   result.FailedPodcasts,
		"new_episodes":   result.NewEpisodes,
		"errors":         result.Errors,
	})
}

// GetSyncStatus 获取同步状态
// GET /api/v1/sync/status
func (h *SyncHandler) GetSyncStatus(c *gin.Context) {
	db := database.GetDB()

	// 统计播客总数
	var totalPodcasts int64
	db.Model(&models.Podcast{}).Where("is_subscribed = ?", true).Count(&totalPodcasts)

	// 统计数据来源
	var sources []struct {
		DataSource string
		Count      int64
	}
	db.Model(&models.Podcast{}).
		Select("data_source, COUNT(*) as count").
		Where("is_subscribed = ?", true).
		Group("data_source").
		Scan(&sources)

	podcastSources := make(map[string]int)
	for _, s := range sources {
		podcastSources[s.DataSource] = int(s.Count)
	}

	// 获取最近一次同步时间（从sync_configs表）
	var lastSync time.Time
	err := db.Model(&models.SyncConfig{}).
		Where("config_key = ?", "last_sync_time").
		Pluck("config_value", &lastSync).Error

	var lastSyncPtr *time.Time
	if err == nil {
		lastSyncPtr = &lastSync
	}

	c.JSON(200, gin.H{
		"success":         true,
		"total_podcasts":  totalPodcasts,
		"podcast_sources": podcastSources,
		"last_sync_time":  lastSyncPtr,
	})
}

// Close 关闭handler，释放资源
func (h *SyncHandler) Close() error {
	if h.syncService != nil {
		return h.syncService.Close()
	}
	return nil
}

// SyncPodcastEpisodesRequest 同步单个podcast的episodes请求
type SyncPodcastEpisodesRequest struct {
	Mode             string `json:"mode"`   // 同步模式: incremental, full, smart
	Update           bool   `json:"update"` // 是否更新已存在的episode
	ConfirmationText string `json:"confirmation_text,omitempty"`
}

// SyncPodcastEpisodes 同步指定podcast的episodes
// POST /api/v1/podcasts/:id/episodes/sync
func (h *SyncHandler) SyncPodcastEpisodes(c *gin.Context) {
	// 获取podcast ID（使用辅助函数）
	podcastID, ok := ParseUintParam(c, "id")
	if !ok {
		return
	}

	// 解析请求参数
	var req SyncPodcastEpisodesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// 使用默认配置
		req = SyncPodcastEpisodesRequest{
			Mode:   "smart",
			Update: true,
		}
	}
	if !middleware.RequireConfirmationText(
		c,
		req.ConfirmationText,
		fmt.Sprintf("SYNC EPISODES %d", podcastID),
		fmt.Sprintf("同步播客 %d 的全部单集并可能覆盖已有元数据", podcastID),
	) {
		return
	}

	// 构建同步配置
	config := sync.DefaultEpisodeSyncConfig
	config.Mode = sync.ParseEpisodeSyncMode(req.Mode)
	config.UpdateExisting = req.Update

	// 使用SSE流式报告进度
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	reporter := sync.NewSSEProgressReporter(c.Writer)
	defer reporter.Close() // ✅ 确保 reporter 被关闭

	// 执行同步
	result, err := h.syncService.SyncPodcastEpisodes(uint(podcastID), reporter, config)
	if err != nil {
		reporter.ReportError(fmt.Sprintf("同步失败: %v", err))
		return
	}

	// 发送最终结果
	c.JSON(200, gin.H{
		"success": true,
		"message": fmt.Sprintf("同步完成: 新增 %d, 更新 %d, 跳过 %d",
			result.Created, result.Updated, result.Skipped),
		"result": result,
	})
}

// SyncAllEpisodesRequest 同步所有episodes请求
type SyncAllEpisodesRequest struct {
	Mode             string `json:"mode"` // 同步模式: incremental, full, smart
	ConfirmationText string `json:"confirmation_text,omitempty"`
}

// SyncAllEpisodes 同步所有podcast的episodes（SSE流式）
// POST /api/v1/sync/episodes
func (h *SyncHandler) SyncAllEpisodes(c *gin.Context) {
	// 解析请求参数
	var req SyncAllEpisodesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// 使用默认配置
		req = SyncAllEpisodesRequest{
			Mode: "smart",
		}
	}
	if !middleware.RequireConfirmationText(c, req.ConfirmationText, "SYNC ALL EPISODES", "同步全部播客的单集并写入数据库") {
		return
	}

	// 构建同步配置
	config := sync.DefaultEpisodeSyncConfig
	config.Mode = sync.ParseEpisodeSyncMode(req.Mode)

	logger.Infof("🚀 开始同步所有podcast的episodes (模式: %s)", req.Mode)

	// 使用SSE流式报告进度
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	reporter := sync.NewSSEProgressReporter(c.Writer)
	defer reporter.Close() // ✅ 确保 reporter 被关闭

	// 执行同步
	if err := h.syncService.SyncAllPodcastEpisodes(reporter, config); err != nil {
		reporter.ReportError(fmt.Sprintf("同步失败: %v", err))
		return
	}

	logger.Info("✅ 所有podcast的episodes同步完成")
}

// SyncAllEpisodesNonStreaming 同步所有podcast的episodes（非流式，用于定时任务）
// POST /api/v1/sync/episodes/sync
func (h *SyncHandler) SyncAllEpisodesNonStreaming(c *gin.Context) {
	// 解析请求参数
	var req SyncAllEpisodesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// 使用默认配置
		req = SyncAllEpisodesRequest{
			Mode: "smart",
		}
	}
	if !middleware.RequireConfirmationText(c, req.ConfirmationText, "SYNC ALL EPISODES", "同步全部播客的单集并写入数据库") {
		return
	}

	// 构建同步配置
	config := sync.DefaultEpisodeSyncConfig
	config.Mode = sync.ParseEpisodeSyncMode(req.Mode)

	logger.Infof("🚀 开始同步所有podcast的episodes (非流式, 模式: %s)", req.Mode)

	// 使用日志报告器（非流式）
	reporter := sync.NewLogProgressReporter()

	// 执行同步
	if err := h.syncService.SyncAllPodcastEpisodes(reporter, config); err != nil {
		logger.Infof("❌ 同步失败: %v", err)
		middleware.InternalErrorResponseWithCode(c, "SYNC_ERROR", fmt.Sprintf("同步失败: %v", err))
		return
	}

	logger.Info("✅ 所有podcast的episodes同步完成")

	middleware.SuccessResponseWithMessage(c, "所有podcast的episodes同步完成", nil)
}
