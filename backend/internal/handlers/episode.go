package handlers

import (
	"errors"
	"html"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"magicpodcast/internal/cache"
	"magicpodcast/internal/database"
	episodelabel "magicpodcast/internal/episode"
	"magicpodcast/internal/middleware"
	"magicpodcast/internal/models"
	"magicpodcast/internal/utils"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// EpisodeHandler 单集处理器
type EpisodeHandler struct{}

// NewEpisodeHandler 创建单集处理器
func NewEpisodeHandler() *EpisodeHandler {
	return &EpisodeHandler{}
}

// EpisodeResponse 单集响应结构
type EpisodeResponse struct {
	ID                uint   `json:"id"`
	GUID              string `json:"guid"` // RSS item GUID，用于单集去重
	PodcastID         uint   `json:"podcast_id"`
	EpisodeNo         string `json:"episode_no"`
	Title             string `json:"title"`
	MediumURL         string `json:"medium_url"`
	ShowNotes         string `json:"show_notes"`
	PublishedDate     string `json:"published_date"`
	Duration          int    `json:"duration"`         // 音频时长（秒）
	Link              string `json:"link"`             // 单集网页链接
	ImageURL          string `json:"image_url"`        // 单集封面图URL
	EnclosureType     string `json:"enclosure_type"`   // 音频MIME类型
	EnclosureLength   int64  `json:"enclosure_length"` // 音频文件大小（字节）
	MyRate            int    `json:"my_rate"`
	Notes             string `json:"notes"`
	VideoAvailability string `json:"video_availability"`
}

// EpisodeShowNotesResponse exposes only the public display document and the
// identity needed by clients to reject a stale response.
type EpisodeShowNotesResponse struct {
	EpisodeID         uint                    `json:"episode_id"`
	ShowNotesDocument utils.ShowNotesDocument `json:"show_notes_document"`
}

const (
	episodeListSummaryView                = "summary"
	episodeShowNotesDBPreviewLimit        = 1600
	episodeShowNotesPreviewLimit          = 600
	episodeSummaryShowNotesDBPreviewLimit = 900
	episodeSummaryShowNotesPreviewLimit   = 320
)

var htmlTagPattern = regexp.MustCompile(`<[^>]+>`)

const episodeListSelectColumns = `
	id,
	guid,
	podcast_id,
	episode_no,
	title,
	medium_url,
	substr(show_notes, 1, ?) AS show_notes,
	published_date,
	duration,
	link,
	image_url,
	enclosure_type,
	enclosure_length,
	my_rate,
	video_availability
`

func episodeShowNotesPreview(showNotes string, previewLimit int) string {
	if showNotes == "" {
		return ""
	}

	preview := htmlTagPattern.ReplaceAllString(showNotes, " ")
	preview = html.UnescapeString(preview)
	preview = strings.Join(strings.Fields(preview), " ")

	if utf8.RuneCountInString(preview) <= previewLimit {
		return preview
	}

	runes := []rune(preview)
	return string(runes[:previewLimit]) + "..."
}

func episodeToResponse(episode models.Episode, previewLimit int) EpisodeResponse {
	return EpisodeResponse{
		ID:                episode.ID,
		GUID:              episode.GUID,
		PodcastID:         episode.PodcastID,
		EpisodeNo:         episodelabel.Normalize(episode.Title, episode.EpisodeNo),
		Title:             episode.Title,
		MediumURL:         episode.MediumURL,
		ShowNotes:         episodeShowNotesPreview(episode.ShowNotes, previewLimit),
		PublishedDate:     episode.PublishedDate.Format("2006-01-02T15:04:05Z07:00"),
		Duration:          episode.Duration,
		Link:              episode.Link,
		ImageURL:          episode.ImageURL,
		EnclosureType:     episode.EnclosureType,
		EnclosureLength:   episode.EnclosureLength,
		MyRate:            episode.MyRate,
		Notes:             "",
		VideoAvailability: models.NormalizeVideoAvailability(episode.VideoAvailability),
	}
}

func episodesToResponse(episodes []models.Episode, previewLimit int) []EpisodeResponse {
	response := make([]EpisodeResponse, len(episodes))
	for i, episode := range episodes {
		response[i] = episodeToResponse(episode, previewLimit)
	}
	return response
}

func countPodcastEpisodes(db *gorm.DB, podcastID uint) (int64, error) {
	var total int64
	err := db.Model(&models.Episode{}).Where("podcast_id = ?", podcastID).Count(&total).Error
	return total, err
}

func listPodcastEpisodes(db *gorm.DB, podcastID uint, page, pageSize, dbPreviewLimit int) ([]models.Episode, error) {
	offset := (page - 1) * pageSize
	var episodes []models.Episode

	err := db.Select(episodeListSelectColumns, dbPreviewLimit).
		Where("podcast_id = ?", podcastID).
		Order("published_date DESC, id DESC").
		Limit(pageSize).
		Offset(offset).
		Find(&episodes).Error

	return episodes, err
}

func episodeListPreviewLimits(summaryView bool) (int, int) {
	if summaryView {
		return episodeSummaryShowNotesDBPreviewLimit, episodeSummaryShowNotesPreviewLimit
	}
	return episodeShowNotesDBPreviewLimit, episodeShowNotesPreviewLimit
}

func episodePaginationPayload(page, pageSize int, total int64) gin.H {
	return gin.H{
		"page":        page,
		"page_size":   pageSize,
		"total":       total,
		"total_pages": (total + int64(pageSize) - 1) / int64(pageSize),
		"has_more":    int64(page*pageSize) < total,
	}
}

// ListByPodcast 获取指定播客的单集列表（支持分页）
// @Summary 获取播客的单集列表
// @Description 根据 Podcast ID 获取该播客的单集列表（支持分页，用于无限滚动）
// @Tags Episodes
// @Accept json
// @Produce json
// @Param id path int true "Podcast ID"
// @Param page query int false "页码（默认1）"
// @Param page_size query int false "每页数量（默认20，最大100）"
// @Success 200 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Router /api/v1/podcasts/{id}/episodes [get]
func (h *EpisodeHandler) ListByPodcast(c *gin.Context) {
	db := database.GetDB()

	podcastID, ok := ParseUintParam(c, "id")
	if !ok {
		return
	}

	// 检查播客是否存在
	var podcast models.Podcast
	if err := db.Select("id").First(&podcast, podcastID).Error; err != nil {
		middleware.NotFoundResponse(c, "NOT_FOUND", "Podcast not found")
		return
	}

	// 分页参数（使用辅助函数）
	pagination := ParsePaginationParams(c, 20)
	page := pagination.Page
	pageSize := pagination.PageSize
	summaryView := strings.EqualFold(c.Query("view"), episodeListSummaryView)
	cacheView := "full"
	if summaryView {
		cacheView = episodeListSummaryView
	}
	dbPreviewLimit, responsePreviewLimit := episodeListPreviewLimits(summaryView)

	cacheKey := cache.NewKeyBuilder().EpisodeList(podcastID, page, pageSize, cacheView)
	memCache := cache.GetCache()
	if cached, ok := memCache.Get(cacheKey); ok {
		cache.RecordHit()
		cachedResp := copyGinH(cached.(gin.H))
		cachedResp["cached"] = true
		setPrivateCache(c, 60)
		c.JSON(200, cachedResp)
		return
	}
	cache.RecordMiss()

	// 获取总数
	total, err := countPodcastEpisodes(db, podcastID)
	if err != nil {
		middleware.InternalErrorResponseWithCode(c, "DATABASE_ERROR", "Failed to count episodes")
		return
	}

	// 获取该播客的单集，按发布日期倒序，支持分页
	episodes, err := listPodcastEpisodes(db, podcastID, page, pageSize, dbPreviewLimit)
	if err != nil {
		middleware.InternalErrorResponseWithCode(c, "DATABASE_ERROR", "Failed to fetch episodes")
		return
	}

	// 转换为响应格式
	resp := gin.H{
		"success":    true,
		"data":       episodesToResponse(episodes, responsePreviewLimit),
		"pagination": episodePaginationPayload(page, pageSize, total),
	}

	memCache.SetWithTTL(cacheKey, resp, 2*time.Minute)
	setPrivateCache(c, 60)
	c.JSON(200, resp)
}

// GetShowNotes returns the complete read-time display document for one episode.
// Stored source text remains unchanged; all format decisions stay centralized
// in BuildShowNotesDocument.
func (h *EpisodeHandler) GetShowNotes(c *gin.Context) {
	db := database.GetDB()

	episodeID, ok := ParseUintParam(c, "id")
	if !ok {
		return
	}

	var episode models.Episode
	if err := db.Select("id", "show_notes").First(&episode, episodeID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			middleware.NotFoundResponse(c, "NOT_FOUND", "Episode not found")
			return
		}
		middleware.InternalErrorResponseWithCode(c, "DATABASE_ERROR", "Failed to fetch episode Show Notes")
		return
	}

	setPrivateCache(c, 60)
	c.JSON(200, gin.H{
		"success": true,
		"data": EpisodeShowNotesResponse{
			EpisodeID:         episode.ID,
			ShowNotesDocument: utils.BuildShowNotesDocument(episode.ShowNotes),
		},
	})
}
