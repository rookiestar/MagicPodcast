package handlers

import (
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"strconv"
	"time"

	"magicpodcast/internal/config"
	"magicpodcast/internal/database"
	"magicpodcast/internal/models"
	"magicpodcast/internal/sync"

	"github.com/gin-gonic/gin"
)

// SyncHandler 同步处理器
type SyncHandler struct {
	syncService *sync.Service
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
	}, nil
}

// ImportOPMLRequest OPML导入请求
type ImportOPMLRequest struct {
	FilePath string `json:"file_path"` // 用于已上传的文件路径
}

// SyncStatusResponse 同步状态响应
type SyncStatusResponse struct {
	LastSyncTime   *time.Time `json:"last_sync_time"`
	TotalPodcasts  int        `json:"total_podcasts"`
	PodcastSources map[string]int `json:"podcast_sources"` // 数据来源统计
}

// ImportOPML 导入OPML文件
// POST /api/v1/sync/import
func (h *SyncHandler) ImportOPML(c *gin.Context) {
	// 获取上传的文件
	file, err := c.FormFile("opml_file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "OPML文件上传失败，请确保使用multipart/form-data格式",
		})
		return
	}

	// 验证文件扩展名
	ext := filepath.Ext(file.Filename)
	if ext != ".opml" && ext != ".xml" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "OPML文件格式不正确，请上传.opml或.xml文件",
		})
		return
	}

	log.Printf("收到OPML文件: %s (%d bytes)", file.Filename, file.Size)

	// 保存到临时文件
	tempDir := filepath.Join(".", "data", "temp")
	if err := c.SaveUploadedFile(file, filepath.Join(tempDir, file.Filename)); err != nil {
		log.Printf("保存文件失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "保存文件失败",
		})
		return
	}

	tempFilePath := filepath.Join(tempDir, file.Filename)

	log.Printf("文件已保存到: %s", tempFilePath)

	// 导入OPML
	result, err := h.syncService.ImportOPML(tempFilePath)
	if err != nil {
		log.Printf("导入失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   fmt.Sprintf("导入失败: %v", err),
		})
		return
	}

	log.Printf("导入成功: %d/%d", result.SuccessPodcasts, result.TotalPodcasts)

	c.JSON(http.StatusOK, gin.H{
		"success":         true,
		"message":         fmt.Sprintf("成功导入 %d 个播客", result.SuccessPodcasts),
		"total_podcasts":  result.TotalPodcasts,
		"success_count":   result.SuccessPodcasts,
		"failed_count":    result.FailedPodcasts,
		"errors":          result.Errors,
	})
}

// SyncSubscriptions 同步所有订阅（定时任务手动触发）
// POST /api/v1/sync/subscriptions
func (h *SyncHandler) SyncSubscriptions(c *gin.Context) {
	log.Println("开始同步所有订阅...")

	result, err := h.syncService.SyncAllPodcasts()
	if err != nil {
		log.Printf("同步失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   fmt.Sprintf("同步失败: %v", err),
		})
		return
	}

	log.Printf("同步完成: %d 个新单集", result.NewEpisodes)

	c.JSON(http.StatusOK, gin.H{
		"success":         true,
		"message":         fmt.Sprintf("同步完成，获取 %d 个新单集", result.NewEpisodes),
		"total_podcasts":  result.TotalPodcasts,
		"success_count":   result.SuccessPodcasts,
		"failed_count":    result.FailedPodcasts,
		"new_episodes":    result.NewEpisodes,
		"errors":          result.Errors,
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

	c.JSON(http.StatusOK, gin.H{
		"success":          true,
		"total_podcasts":   totalPodcasts,
		"podcast_sources":  podcastSources,
		"last_sync_time":   lastSyncPtr,
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
	Mode   string `json:"mode"`   // 同步模式: incremental, full, smart
	Update bool  `json:"update"`  // 是否更新已存在的episode
}

// SyncPodcastEpisodes 同步指定podcast的episodes
// POST /api/v1/podcasts/:id/episodes/sync
func (h *SyncHandler) SyncPodcastEpisodes(c *gin.Context) {
	// 获取podcast ID
	idStr := c.Param("id")
	podcastID, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "无效的podcast ID",
		})
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

	// 构建同步配置
	config := sync.DefaultEpisodeSyncConfig

	// 解析同步模式
	switch req.Mode {
	case "incremental":
		config.Mode = sync.SyncModeIncremental
	case "full":
		config.Mode = sync.SyncModeFull
	case "smart":
		config.Mode = sync.SyncModeSmart
	default:
		config.Mode = sync.SyncModeSmart
	}

	config.UpdateExisting = req.Update

	// 使用SSE流式报告进度
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	reporter := sync.NewSSEProgressReporter(c.Writer)

	// 执行同步
	result, err := h.syncService.SyncPodcastEpisodes(uint(podcastID), reporter, config)
	if err != nil {
		reporter.ReportError(fmt.Sprintf("同步失败: %v", err))
		return
	}

	// 发送最终结果
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": fmt.Sprintf("同步完成: 新增 %d, 更新 %d, 跳过 %d",
			result.Created, result.Updated, result.Skipped),
		"result": result,
	})
}

// SyncAllEpisodesRequest 同步所有episodes请求
type SyncAllEpisodesRequest struct {
	Mode string `json:"mode"` // 同步模式: incremental, full, smart
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

	// 构建同步配置
	config := sync.DefaultEpisodeSyncConfig

	// 解析同步模式
	switch req.Mode {
	case "incremental":
		config.Mode = sync.SyncModeIncremental
	case "full":
		config.Mode = sync.SyncModeFull
	case "smart":
		config.Mode = sync.SyncModeSmart
	default:
		config.Mode = sync.SyncModeSmart
	}

	log.Printf("🚀 开始同步所有podcast的episodes (模式: %s)", req.Mode)

	// 使用SSE流式报告进度
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	reporter := sync.NewSSEProgressReporter(c.Writer)

	// 执行同步
	if err := h.syncService.SyncAllPodcastEpisodes(reporter, config); err != nil {
		reporter.ReportError(fmt.Sprintf("同步失败: %v", err))
		return
	}

	log.Println("✅ 所有podcast的episodes同步完成")
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

	// 构建同步配置
	config := sync.DefaultEpisodeSyncConfig

	// 解析同步模式
	switch req.Mode {
	case "incremental":
		config.Mode = sync.SyncModeIncremental
	case "full":
		config.Mode = sync.SyncModeFull
	case "smart":
		config.Mode = sync.SyncModeSmart
	default:
		config.Mode = sync.SyncModeSmart
	}

	log.Printf("🚀 开始同步所有podcast的episodes (非流式, 模式: %s)", req.Mode)

	// 使用日志报告器（非流式）
	reporter := sync.NewLogProgressReporter()

	// 执行同步
	if err := h.syncService.SyncAllPodcastEpisodes(reporter, config); err != nil {
		log.Printf("❌ 同步失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   fmt.Sprintf("同步失败: %v", err),
		})
		return
	}

	log.Println("✅ 所有podcast的episodes同步完成")

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "所有podcast的episodes同步完成",
	})
}

// SyncPodcastsFull 完整同步：元数据 + 单集（SSE流式）
// POST /api/v1/sync/podcasts/full
func (h *SyncHandler) SyncPodcastsFull(c *gin.Context) {
	// 添加panic恢复机制
	defer func() {
		if r := recover(); r != nil {
			log.Printf("❌ 完整同步发生panic: %v", r)
			// 尝试发送错误消息到客户端（如果可能的话）
			c.Header("Content-Type", "text/event-stream")
			fmt.Fprintf(c.Writer, "data: {\"type\":\"error\",\"message\":\"同步过程中发生内部错误\"}\n\n")
			fmt.Fprintf(c.Writer, "data: [DONE]\n\n")
		}
	}()

	// 解析请求参数
	var req struct {
		MetadataMode string `json:"metadata_mode"` // 元数据同步模式: incremental, full
		EpisodeMode  string `json:"episode_mode"`  // 单集同步模式: incremental, full, smart
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		// 使用默认配置
		req.MetadataMode = "incremental"
		req.EpisodeMode = "smart"
	}

	log.Printf("🚀 开始完整同步 (元数据模式: %s, 单集模式: %s)", req.MetadataMode, req.EpisodeMode)

	// 使用SSE流式报告进度
	// 设置关键的SSE响应头，确保连接不会过早断开
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no") // 禁用nginx缓冲
	// 注意：不要手动调用WriteHeader，让Gin框架自动处理

	reporter := sync.NewSSEProgressReporter(c.Writer)

	// 构建同步配置
	config := sync.FullSyncConfig{
		MetadataSyncMode: req.MetadataMode,
		EpisodeSyncMode:  req.EpisodeMode,
	}

	// 执行同步
	if err := h.syncService.SyncPodcastsFull(reporter, config); err != nil {
		reporter.ReportError(fmt.Sprintf("完整同步失败: %v", err))
		reporter.Close() // 确保关闭连接
		return
	}

	// 发送完成标记
	reporter.Close()
	log.Println("✅ 完整同步流程完成")
}
