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
	FeedURL           string         `json:"feed_url,omitempty"`
	EpisodeCount      int            `json:"episode_count"`
	NewestEpisodeDate time.Time      `json:"newest_episode_date"`
	CreatedAt         time.Time      `json:"created_at"`
	AddedDate         time.Time      `json:"added_date,omitempty"`
	IsSubscribed      bool           `json:"is_subscribed"`
	IsDead            bool           `json:"is_dead"`
	MyRate            int            `json:"my_rate,omitempty"`
	Notes             string         `json:"notes,omitempty"`
	DataSource        string         `json:"data_source,omitempty"`

	// 🆕 PodcastIndex 新增字段（可选，使用 omitempty 保持向后兼容）
	Link                    string     `json:"link,omitempty"`                              // 播客网站链接
	NewestEnclosureURL      string     `json:"newest_enclosure_url,omitempty"`              // 最新单集音频URL
	NewestEnclosureDuration int        `json:"newest_enclosure_duration,omitempty"`         // 最新单集时长（秒）
	LastUpdate              *time.Time `json:"last_update,omitempty"`                       // Feed最后更新时间
	OldestEpisodeDate       *time.Time `json:"oldest_episode_date,omitempty"`               // 最旧单集发布日期
	PopularityScore         int        `json:"popularity_score,omitempty"`                  // 受欢迎程度 (0-10)
	Priority                int        `json:"priority,omitempty"`                           // 抓取优先级 (0-10, -1=暂停)
	UpdateFrequency         int        `json:"update_frequency,omitempty"`                  // 更新频率 (0-10)

	Tags              []TagResponse  `json:"tags,omitempty"`
}

// List 获取播客节目列表
// @Summary 获取播客节目列表
// @Description 获取播客节目列表，支持按标签筛选（支持多选AND逻辑）、搜索、排序和分页
// @Tags Podcasts
// @Accept json
// @Produce json
// @Param tag_id query []int false "标签ID列表（多个标签使用AND逻辑）"
// @Param search query string false "搜索关键词"
// @Param sort_by query string false "排序方式: recent_update(最近更新), newest_added(最新添加), episode_count(单集数量), title(名称)"
// @Param page query int false "页码（默认1）"
// @Param page_size query int false "每页数量（默认15）"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/podcasts [get]
func (h *PodcastHandler) List(c *gin.Context) {
	db := database.GetDB()

	// 构建查询
	query := db.Model(&models.Podcast{}).Preload("Tags")

	// 获取查询参数
	tagIDStrs := c.QueryArray("tag_id")
	searchKeyword := c.Query("search")
	sortBy := c.DefaultQuery("sort_by", "recent_update") // 默认按最近更新排序

	// 分页参数
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "15"))

	// 验证分页参数
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 15
	}

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

	// 排序逻辑（默认：综合时间倒序）
	switch sortBy {
	case "recent_update":
		// 综合排序：优先使用最新单集时间，否则用创建时间
		query = query.Order("CASE WHEN newest_episode_date IS NOT NULL THEN newest_episode_date ELSE created_at END DESC")
	case "newest_added":
		// 按添加时间倒序
		query = query.Order("added_date DESC")
	case "episode_count":
		// 按单集数量倒序
		query = query.Order("episode_count DESC")
	case "title":
		// 按名称正序
		query = query.Order("title COLLATE NOCASE ASC")
	default:
		// 默认按最近更新排序
		query = query.Order("CASE WHEN newest_episode_date IS NOT NULL THEN newest_episode_date ELSE created_at END DESC")
	}

	// 获取总数
	var total int64
	query.Count(&total)

	// 分页查询
	var podcasts []models.Podcast
	offset := (page - 1) * pageSize
	if err := query.Offset(offset).Limit(pageSize).Find(&podcasts).Error; err != nil {
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

	// 计算总页数
	totalPages := int(total) / pageSize
	if int(total)%pageSize > 0 {
		totalPages++
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
		"pagination": gin.H{
			"page":        page,
			"page_size":   pageSize,
			"total":       total,
			"total_pages": totalPages,
		},
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
			ID:    tag.ID,
			Name:  tag.Name,
			Color: tag.Color,
		}
	}

	return PodcastResponse{
		ID:                podcast.ID,
		XYZID:             podcast.XYZID,
		Title:             podcast.Title,
		Description:       podcast.Description,
		Author:            podcast.Author,
		CoverURL:          podcast.CoverURL,
		FeedURL:           podcast.FeedURL,
		EpisodeCount:      podcast.EpisodeCount,
		NewestEpisodeDate: podcast.NewestEpisodeDate,
		CreatedAt:         podcast.CreatedAt,
		AddedDate:         podcast.AddedDate,
		IsSubscribed:      podcast.IsSubscribed,
		IsDead:            podcast.IsDead,
		MyRate:            podcast.MyRate,
		Notes:             podcast.Notes,
		DataSource:        podcast.DataSource,

		// 🆕 PodcastIndex 新增字段
		Link:                    podcast.Link,
		NewestEnclosureURL:      podcast.NewestEnclosureURL,
		NewestEnclosureDuration: podcast.NewestEnclosureDuration,
		LastUpdate:              podcast.LastUpdate,
		OldestEpisodeDate:       podcast.OldestEpisodeDate,
		PopularityScore:         podcast.PopularityScore,
		Priority:                podcast.Priority,
		UpdateFrequency:         podcast.UpdateFrequency,

		Tags: tags,
	}
}
