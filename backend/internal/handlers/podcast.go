package handlers

import (
	"fmt"
	"html"
	"strings"
	"time"

	"magicpodcast/internal/cache"
	"magicpodcast/internal/database"
	"magicpodcast/internal/middleware"
	"magicpodcast/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// PodcastHandler Podcast 处理器
type PodcastHandler struct{}

// NewPodcastHandler 创建 Podcast 处理器
func NewPodcastHandler() *PodcastHandler {
	return &PodcastHandler{}
}

// PodcastResponse Podcast 响应结构
type PodcastResponse struct {
	ID                uint      `json:"id"`
	XYZID             string    `json:"xyz_id"`
	Title             string    `json:"title"`
	Description       string    `json:"description"`
	Author            string    `json:"author"`
	CoverURL          string    `json:"cover_url"`
	CustomCoverURL    string    `json:"custom_cover_url,omitempty"` // 自定义封面URL（优先使用）
	FeedURL           string    `json:"feed_url,omitempty"`
	EpisodeCount      int       `json:"episode_count"`
	NewestEpisodeDate time.Time `json:"newest_episode_date"`
	CreatedAt         time.Time `json:"created_at"`
	AddedDate         time.Time `json:"added_date,omitempty"`
	IsSubscribed      bool      `json:"is_subscribed"`
	IsDead            bool      `json:"is_dead"`
	MyRate            int       `json:"my_rate,omitempty"`
	Notes             string    `json:"notes,omitempty"`
	DataSource        string    `json:"data_source,omitempty"`

	// 🆕 PodcastIndex 新增字段（可选，使用 omitempty 保持向后兼容）
	Link                    string     `json:"link,omitempty"`                      // 播客网站链接
	NewestEnclosureURL      string     `json:"newest_enclosure_url,omitempty"`      // 最新单集音频URL
	NewestEnclosureDuration int        `json:"newest_enclosure_duration,omitempty"` // 最新单集时长（秒）
	LastUpdate              *time.Time `json:"last_update,omitempty"`               // Feed最后更新时间
	OldestEpisodeDate       *time.Time `json:"oldest_episode_date,omitempty"`       // 最旧单集发布日期
	PopularityScore         int        `json:"popularity_score,omitempty"`          // 受欢迎程度 (0-10)
	Priority                int        `json:"priority,omitempty"`                  // 抓取优先级 (0-10, -1=暂停)
	UpdateFrequency         int        `json:"update_frequency,omitempty"`          // 更新频率 (0-10)

	Tags []TagResponse `json:"tags,omitempty"`
}

type PodcastSummaryResponse struct {
	ID                uint          `json:"id"`
	Title             string        `json:"title"`
	Description       string        `json:"description"`
	Author            string        `json:"author"`
	CoverURL          string        `json:"cover_url"`
	CustomCoverURL    string        `json:"custom_cover_url,omitempty"`
	EpisodeCount      int           `json:"episode_count"`
	NewestEpisodeDate time.Time     `json:"newest_episode_date"`
	AddedDate         time.Time     `json:"added_date,omitempty"`
	IsSubscribed      bool          `json:"is_subscribed"`
	IsDead            bool          `json:"is_dead"`
	Tags              []TagResponse `json:"tags,omitempty"`
}

const podcastSummaryView = "summary"
const podcastListDescriptionDBLimit = 640
const podcastListDescriptionLimit = 160

var podcastSummarySelectColumns = []string{
	"id",
	"title",
	fmt.Sprintf("substr(description, 1, %d) AS description", podcastListDescriptionDBLimit),
	"author",
	"cover_url",
	"custom_cover_url",
	"episode_count",
	"newest_episode_date",
	"added_date",
	"is_subscribed",
	"is_dead",
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

	// 获取查询参数
	searchKeyword := c.Query("search")
	sortBy := c.DefaultQuery("sort_by", "recent_update") // 默认按最近更新排序
	view := c.DefaultQuery("view", "")
	summaryView := view == podcastSummaryView

	// 分页参数（使用辅助函数）
	pagination := ParsePaginationParams(c, 15)
	page := pagination.Page
	pageSize := pagination.PageSize

	// 解析标签ID（使用辅助函数）
	tagIDs := ParseUintSliceQueryParam(c, "tag_id")

	// 尝试从缓存获取（仅对无搜索关键词的请求缓存）
	memCache := cache.GetCache()
	cacheKey := ""
	if searchKeyword == "" {
		cacheKey = cache.NewKeyBuilder().PodcastList(page, pageSize, sortBy, tagIDs, "", view)
		if cached, ok := memCache.Get(cacheKey); ok {
			cache.RecordHit()
			cachedResp := copyGinH(cached.(gin.H))
			cachedResp["cached"] = true
			setPrivateCache(c, 60)
			c.JSON(200, cachedResp)
			return
		}
		cache.RecordMiss()
	}

	// 构建查询
	query := db.Model(&models.Podcast{}).Preload("Tags", func(db *gorm.DB) *gorm.DB {
		return db.Select("tags.id", "tags.name", "tags.color")
	})

	// 按标签筛选（AND逻辑：必须同时拥有所有选中的标签）
	if len(tagIDs) > 0 {
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
	if summaryView {
		query = query.Select(podcastSummarySelectColumns)
	}
	if err := query.Offset(offset).Limit(pageSize).Find(&podcasts).Error; err != nil {
		middleware.InternalErrorResponseWithCode(c, "DATABASE_ERROR", "Failed to fetch podcasts")
		return
	}

	// 转换为响应格式
	var data interface{}
	if summaryView {
		data = h.modelsToSummaryResponses(podcasts)
	} else {
		response := make([]PodcastResponse, len(podcasts))
		for i, podcast := range podcasts {
			response[i] = h.modelToResponse(&podcast)
		}
		data = response
	}

	// 计算总页数
	totalPages := int(total) / pageSize
	if int(total)%pageSize > 0 {
		totalPages++
	}

	// 构建响应
	resp := gin.H{
		"success": true,
		"data":    data,
		"pagination": gin.H{
			"page":        page,
			"page_size":   pageSize,
			"total":       total,
			"total_pages": totalPages,
		},
	}

	// 缓存结果（仅缓存无搜索关键词的请求）
	if cacheKey != "" {
		memCache.Set(cacheKey, resp)
	}

	setPrivateCache(c, 60)

	c.JSON(200, resp)
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
		middleware.NotFoundResponse(c, "NOT_FOUND", "Podcast not found")
		return
	}

	// 设置浏览器缓存头（播客详情缓存5分钟）
	c.Header("Cache-Control", "private, max-age=300")

	middleware.SuccessResponse(c, h.modelToResponse(&podcast))
}

// @Summary 批量获取播客
// @Description 根据播客ID列表批量获取播客详情
// @Tags Podcast
// @Accept json
// @Produce json
// @Param ids body []int true "播客ID列表"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Router /api/v1/podcasts/batch [post]
func (h *PodcastHandler) BatchGet(c *gin.Context) {
	db := database.GetDB()

	// 解析请求体
	var request struct {
		IDs  []uint `json:"ids" binding:"required,min=1,max=100"`
		View string `json:"view"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		middleware.BadRequestResponse(c, "INVALID_REQUEST", "Invalid request body: "+err.Error())
		return
	}

	// 查询播客
	var podcasts []models.Podcast
	query := db.Preload("Tags", func(db *gorm.DB) *gorm.DB {
		return db.Select("tags.id", "tags.name", "tags.color")
	})
	if request.View == podcastSummaryView {
		query = query.Select(podcastSummarySelectColumns)
	}
	if err := query.Where("id IN ?", request.IDs).Find(&podcasts).Error; err != nil {
		middleware.InternalErrorResponseWithCode(c, "DATABASE_ERROR", "Failed to fetch podcasts")
		return
	}

	// 转换为响应格式
	if request.View == podcastSummaryView {
		middleware.SuccessResponse(c, h.modelsToSummaryResponses(podcasts))
		return
	}

	responses := make([]PodcastResponse, len(podcasts))
	for i, podcast := range podcasts {
		responses[i] = h.modelToResponse(&podcast)
	}
	middleware.SuccessResponse(c, responses)
}

func truncatePodcastDescription(description string) string {
	description = strings.TrimSpace(description)
	description = htmlTagPattern.ReplaceAllString(description, " ")
	description = html.UnescapeString(description)
	description = strings.Join(strings.Fields(description), " ")

	runes := []rune(description)
	if len(runes) <= podcastListDescriptionLimit {
		return description
	}
	return strings.TrimSpace(string(runes[:podcastListDescriptionLimit])) + "..."
}

func (h *PodcastHandler) modelsToSummaryResponses(podcasts []models.Podcast) []PodcastSummaryResponse {
	response := make([]PodcastSummaryResponse, len(podcasts))
	for i := range podcasts {
		response[i] = h.modelToSummaryResponse(&podcasts[i])
	}
	return response
}

func (h *PodcastHandler) modelToSummaryResponse(podcast *models.Podcast) PodcastSummaryResponse {
	tags := make([]TagResponse, len(podcast.Tags))
	for i, tag := range podcast.Tags {
		tags[i] = TagResponse{
			ID:    tag.ID,
			Name:  tag.Name,
			Color: tag.Color,
		}
	}

	return PodcastSummaryResponse{
		ID:                podcast.ID,
		Title:             podcast.Title,
		Description:       truncatePodcastDescription(podcast.Description),
		Author:            podcast.Author,
		CoverURL:          podcast.CoverURL,
		CustomCoverURL:    podcast.CustomCoverURL,
		EpisodeCount:      podcast.EpisodeCount,
		NewestEpisodeDate: podcast.NewestEpisodeDate,
		AddedDate:         podcast.AddedDate,
		IsSubscribed:      podcast.IsSubscribed,
		IsDead:            podcast.IsDead,
		Tags:              tags,
	}
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
		CustomCoverURL:    podcast.CustomCoverURL,
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

// UpdateCustomCoverRequest 更新自定义封面请求
type UpdateCustomCoverRequest struct {
	CustomCoverURL   string `json:"custom_cover_url" binding:"max=512"`
	ConfirmationText string `json:"confirmation_text,omitempty"`
}

// UpdateCustomCover 更新播客自定义封面
// @Summary 更新播客自定义封面
// @Description 更新指定播客的自定义封面URL（不会被同步覆盖）
// @Tags Podcasts
// @Accept json
// @Produce json
// @Param id path int true "播客 ID"
// @Param request body UpdateCustomCoverRequest true "自定义封面URL"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Router /api/v1/podcasts/{id}/custom-cover [put]
func (h *PodcastHandler) UpdateCustomCover(c *gin.Context) {
	podcastID, ok := ParseUintParam(c, "id")
	if !ok {
		return
	}

	var req UpdateCustomCoverRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.BindJSONError(c, err)
		return
	}

	db := database.GetDB()

	// 检查播客是否存在
	var podcast models.Podcast
	if err := db.First(&podcast, podcastID).Error; err != nil {
		middleware.NotFoundResponse(c, "NOT_FOUND", "播客不存在")
		return
	}
	if !middleware.RequireConfirmationText(
		c,
		req.ConfirmationText,
		fmt.Sprintf("OVERWRITE COVER %d", podcastID),
		fmt.Sprintf("覆盖播客 %q 的自定义封面地址", podcast.Title),
	) {
		return
	}

	// 更新自定义封面
	if err := db.Model(&podcast).Update("custom_cover_url", req.CustomCoverURL).Error; err != nil {
		middleware.InternalErrorResponseWithCode(c, "INTERNAL_ERROR", "更新自定义封面失败: "+err.Error())
		return
	}

	// 使播客详情缓存失效
	cache.InvalidatePodcastDetail(podcastID)

	// 重新获取更新后的播客
	db.First(&podcast, podcastID)

	middleware.SuccessResponse(c, gin.H{
		"id":               podcast.ID,
		"custom_cover_url": podcast.CustomCoverURL,
	})
}
