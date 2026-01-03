package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"magicpodcast/internal/database"
	"magicpodcast/internal/models"

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
	ID                uint           `json:"id"`
	XYZID             string         `json:"xyz_id"`
	Title             string         `json:"title"`
	Description       string         `json:"description"`
	Author            string         `json:"author"`
	CoverURL          string         `json:"cover_url"`
	EpisodeCount      int            `json:"episode_count"`
	NewestEpisodeDate time.Time      `json:"newest_episode_date"`
	CreatedAt         time.Time      `json:"created_at"`
	Tags              []TagResponse  `json:"tags,omitempty"`
}

// List 获取播客节目列表
// @Summary 获取播客节目列表
// @Description 获取播客节目列表，支持按标签筛选（支持多选AND逻辑）和搜索
// @Tags Podcasts
// @Accept json
// @Produce json
// @Param tag_id query []int false "标签ID列表（多个标签使用AND逻辑）"
// @Param search query string false "搜索关键词"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/podcasts [get]
func (h *PodcastHandler) List(c *gin.Context) {
	db := database.GetDB()

	// 构建查询
	query := db.Model(&models.Podcast{}).Preload("Tags")

	// 获取查询参数
	tagIDStrs := c.QueryArray("tag_id")
	searchKeyword := c.Query("search")

	// 按标签筛选（AND逻辑：必须同时拥有所有选中的标签）
	if len(tagIDStrs) > 0 {
		var tagIDs []uint
		for _, tagIDStr := range tagIDStrs {
			tagID, err := strconv.ParseUint(tagIDStr, 10, 32)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{
					"success": false,
					"error": gin.H{
						"code":    "INVALID_PARAM",
						"message": "Invalid tag_id parameter",
					},
				})
				return
			}
			tagIDs = append(tagIDs, uint(tagID))
		}

		// 使用AND逻辑：为每个标签添加JOIN条件
		// 必须同时拥有所有选中的标签
		for i, tagID := range tagIDs {
			alias := fmt.Sprintf("pt%d", i)
			query = query.Joins(
				fmt.Sprintf("INNER JOIN podcasts_tags %s ON %s.podcast_id = podcasts.id", alias, alias),
			).Where(fmt.Sprintf("%s.tag_id = ?", alias), tagID)
		}

		// GROUP BY 确保结果不重复
		query = query.Group("podcasts.id")
	}

	// 搜索功能
	if searchKeyword != "" {
		searchPattern := fmt.Sprintf("%%%s%%", searchKeyword)
		query = query.Where("title LIKE ? OR description LIKE ? OR author LIKE ?",
			searchPattern, searchPattern, searchPattern)
	}

	// 执行查询
	var podcasts []models.Podcast
	if err := query.Find(&podcasts).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "DATABASE_ERROR",
				"message": "Failed to fetch podcasts",
			},
		})
		return
	}

	// 转换为响应格式
	response := make([]PodcastResponse, len(podcasts))
	for i, podcast := range podcasts {
		response[i] = h.modelToResponse(&podcast)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
	})
}

// Get 获取单个播客节目详情
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
	db := database.GetDB()
	id := c.Param("id")

	var podcast models.Podcast
	if err := db.Preload("Tags").First(&podcast, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "NOT_FOUND",
				"message": "Podcast not found",
			},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    h.modelToResponse(&podcast),
	})
}

// modelToResponse 将模型转换为响应格式
func (h *PodcastHandler) modelToResponse(podcast *models.Podcast) PodcastResponse {
	// 转换标签
	tags := make([]TagResponse, len(podcast.Tags))
	for i, tag := range podcast.Tags {
		tags[i] = TagResponse{
			ID:          tag.ID,
			Name:        tag.Name,
			Description: tag.Description,
			Color:       tag.Color,
		}
	}

	return PodcastResponse{
		ID:                podcast.ID,
		XYZID:             podcast.XYZID,
		Title:             podcast.Title,
		Description:       podcast.Description,
		Author:            podcast.Author,
		CoverURL:          podcast.CoverURL,
		EpisodeCount:      podcast.EpisodeCount,
		NewestEpisodeDate: podcast.NewestEpisodeDate,
		CreatedAt:         podcast.CreatedAt,
		Tags:              tags,
	}
}
