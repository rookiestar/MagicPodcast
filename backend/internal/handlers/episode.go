package handlers

import (
	"net/http"
	"strconv"

	"magicpodcast/internal/database"
	"magicpodcast/internal/models"

	"github.com/gin-gonic/gin"
)

// EpisodeHandler 单集处理器
type EpisodeHandler struct{}

// NewEpisodeHandler 创建单集处理器
func NewEpisodeHandler() *EpisodeHandler {
	return &EpisodeHandler{}
}

// EpisodeResponse 单集响应结构
type EpisodeResponse struct {
	ID               uint   `json:"id"`
	GUID             string `json:"guid"`               // 替代xyz_id，使用GUID作为唯一标识
	PodcastID        uint   `json:"podcast_id"`
	EpisodeNo        string `json:"episode_no"`
	Title            string `json:"title"`
	MediumURL        string `json:"medium_url"`
	ShowNotes        string `json:"show_notes"`
	PublishedDate    string `json:"published_date"`
	Duration         int    `json:"duration"`            // 音频时长（秒）
	Link             string `json:"link"`                // 单集网页链接
	ImageURL         string `json:"image_url"`           // 单集封面图URL
	EnclosureType    string `json:"enclosure_type"`      // 音频MIME类型
	EnclosureLength  int64  `json:"enclosure_length"`    // 音频文件大小（字节）
	MyRate           int    `json:"my_rate"`
	Notes            string `json:"notes"`
}

// ListByPodcast 获取指定播客的单集列表
// @Summary 获取播客的单集列表
// @Description 根据 Podcast ID 获取该播客的所有单集
// @Tags Episodes
// @Accept json
// @Produce json
// @Param id path int true "Podcast ID"
// @Success 200 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Router /api/v1/podcasts/{id}/episodes [get]
func (h *EpisodeHandler) ListByPodcast(c *gin.Context) {
	db := database.GetDB()
	podcastIDStr := c.Param("id")

	podcastID, err := strconv.ParseUint(podcastIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "INVALID_PARAM",
				"message": "Invalid id parameter",
			},
		})
		return
	}

	// 检查播客是否存在
	var podcast models.Podcast
	if err := db.First(&podcast, podcastID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "NOT_FOUND",
				"message": "Podcast not found",
			},
		})
		return
	}

	// 获取该播客的所有单集，按发布日期倒序
	var episodes []models.Episode
	if err := db.Where("podcast_id = ?", podcastID).
		Order("published_date DESC").
		Find(&episodes).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "DATABASE_ERROR",
				"message": "Failed to fetch episodes",
			},
		})
		return
	}

	// 转换为响应格式
	response := make([]EpisodeResponse, len(episodes))
	for i, episode := range episodes {
		response[i] = EpisodeResponse{
			ID:              episode.ID,
			GUID:            episode.GUID,
			PodcastID:       episode.PodcastID,
			EpisodeNo:       episode.EpisodeNo,
			Title:           episode.Title,
			MediumURL:       episode.MediumURL,
			ShowNotes:       episode.ShowNotes,
			PublishedDate:   episode.PublishedDate.Format("2006-01-02T15:04:05Z07:00"),
			Duration:        episode.Duration,
			Link:            episode.Link,
			ImageURL:        episode.ImageURL,
			EnclosureType:   episode.EnclosureType,
			EnclosureLength: episode.EnclosureLength,
			MyRate:          episode.MyRate,
			Notes:           episode.Notes,
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
	})
}
