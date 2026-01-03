package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// PodcastHandler Podcast 处理器
type PodcastHandler struct{}

// NewPodcastHandler 创建 Podcast 处理器
func NewPodcastHandler() *PodcastHandler {
	return &PodcastHandler{}
}

// PodcastResponse Podcast 响应结构
type PodcastResponse struct {
	ID              uint      `json:"id"`
	XYZID           string    `json:"xyz_id"`
	Title           string    `json:"title"`
	Description     string    `json:"description"`
	Author          string    `json:"author"`
	CoverURL        string    `json:"cover_url"`
	EpisodeCount    int       `json:"episode_count"`
	NewestEpisodeDate time.Time `json:"newest_episode_date"`
	CreatedAt       time.Time `json:"created_at"`
}

// List 获取播客节目列表（假数据）
// @Summary 获取播客节目列表
// @Description 获取播客节目列表
// @Tags Podcasts
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/podcasts [get]
func (h *PodcastHandler) List(c *gin.Context) {
	// 假数据
	fakePodcasts := []PodcastResponse{
		{
			ID:              1,
			XYZID:           "xyz001",
			Title:           "科技杂谈",
			Description:     "探讨最新科技趋势",
			Author:          "张三",
			CoverURL:        "https://example.com/cover1.jpg",
			EpisodeCount:    50,
			NewestEpisodeDate: time.Now().AddDate(0, 0, -1),
			CreatedAt:       time.Now().AddDate(0, -1, 0),
		},
		{
			ID:              2,
			XYZID:           "xyz002",
			Title:           "商业洞察",
			Description:     "深度商业分析",
			Author:          "李四",
			CoverURL:        "https://example.com/cover2.jpg",
			EpisodeCount:    30,
			NewestEpisodeDate: time.Now().AddDate(0, 0, -2),
			CreatedAt:       time.Now().AddDate(0, -2, 0),
		},
		{
			ID:              3,
			XYZID:           "xyz003",
			Title:           "健康生活",
			Description:     "健康生活方式分享",
			Author:          "王五",
			CoverURL:        "https://example.com/cover3.jpg",
			EpisodeCount:    100,
			NewestEpisodeDate: time.Now().AddDate(0, 0, -3),
			CreatedAt:       time.Now().AddDate(0, -3, 0),
		},
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    fakePodcasts,
	})
}

// Get 获取单个播客节目详情（假数据）
// @Summary 获取播客节目详情
// @Description 根据 ID 获取播客节目详情
// @Tags Podcasts
// @Accept json
// @Produce json
// @Param id path int true "Podcast ID"
// @Success 200 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Router /api/v1/podcasts/{id} [get]
func (h *PodcastHandler) Get(c *gin.Context) {
	id := c.Param("id")

	// 简单的假数据逻辑
	if id == "1" || id == "2" || id == "3" {
		podcast := PodcastResponse{
			ID:              1,
			XYZID:           "xyz001",
			Title:           "科技杂谈",
			Description:     "探讨最新科技趋势，每周更新",
			Author:          "张三",
			CoverURL:        "https://example.com/cover1.jpg",
			EpisodeCount:    50,
			NewestEpisodeDate: time.Now().AddDate(0, 0, -1),
			CreatedAt:       time.Now().AddDate(0, -1, 0),
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data":    podcast,
		})
		return
	}

	c.JSON(http.StatusNotFound, gin.H{
		"success": false,
		"error": gin.H{
			"code":    "NOT_FOUND",
			"message": "Podcast not found",
		},
	})
}
