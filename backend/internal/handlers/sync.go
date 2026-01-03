package handlers

import (
	"fmt"
	"log"
	"net/http"
	"path/filepath"
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
