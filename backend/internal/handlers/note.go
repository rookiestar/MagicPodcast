package handlers

import (
	"magicpodcast/internal/cache"
	"magicpodcast/internal/database"
	"magicpodcast/internal/middleware"
	"magicpodcast/internal/models"

	"github.com/gin-gonic/gin"
)

// NoteHandler 备注处理器
type NoteHandler struct{}

// NewNoteHandler 创建备注处理器
func NewNoteHandler() *NoteHandler {
	return &NoteHandler{}
}

// UpdateNoteRequest 更新备注请求
type UpdateNoteRequest struct {
	Notes string `json:"notes" binding:"max=2000"`
}

// UpdatePodcastNotes 更新播客备注
// @Summary 更新播客备注
// @Description 更新指定播客的备注信息
// @Tags Notes
// @Accept json
// @Produce json
// @Param id path int true "播客 ID"
// @Param request body UpdateNoteRequest true "备注内容"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Router /api/v1/podcasts/{id}/notes [put]
func (h *NoteHandler) UpdatePodcastNotes(c *gin.Context) {
	podcastID, ok := ParseUintParam(c, "id")
	if !ok {
		return
	}

	var req UpdateNoteRequest
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

	// 更新备注
	if err := db.Model(&podcast).Update("notes", req.Notes).Error; err != nil {
		middleware.InternalErrorResponseWithCode(c, "INTERNAL_ERROR", "更新备注失败: "+err.Error())
		return
	}

	// 使播客详情缓存失效
	cache.InvalidatePodcastDetail(podcastID)

	// 重新获取更新后的播客
	db.First(&podcast, podcastID)

	middleware.SuccessResponse(c, gin.H{
		"id":    podcast.ID,
		"notes": podcast.Notes,
	})
}

// GetPodcastNotes 获取播客备注
// @Summary 获取播客备注
// @Description 获取指定播客的备注信息
// @Tags Notes
// @Accept json
// @Produce json
// @Param id path int true "播客 ID"
// @Success 200 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Router /api/v1/podcasts/{id}/notes [get]
func (h *NoteHandler) GetPodcastNotes(c *gin.Context) {
	podcastID, ok := ParseUintParam(c, "id")
	if !ok {
		return
	}

	db := database.GetDB()

	var podcast models.Podcast
	if err := db.Select("id, notes").First(&podcast, podcastID).Error; err != nil {
		middleware.NotFoundResponse(c, "NOT_FOUND", "播客不存在")
		return
	}

	middleware.SuccessResponse(c, gin.H{
		"id":    podcast.ID,
		"notes": podcast.Notes,
	})
}

// UpdateEpisodeNotes 更新单集备注
// @Summary 更新单集备注
// @Description 更新指定单集的备注信息
// @Tags Notes
// @Accept json
// @Produce json
// @Param id path int true "单集 ID"
// @Param request body UpdateNoteRequest true "备注内容"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Router /api/v1/episodes/{id}/notes [put]
func (h *NoteHandler) UpdateEpisodeNotes(c *gin.Context) {
	episodeID, ok := ParseUintParam(c, "id")
	if !ok {
		return
	}

	var req UpdateNoteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.BindJSONError(c, err)
		return
	}

	db := database.GetDB()

	// 检查单集是否存在
	var episode models.Episode
	if err := db.First(&episode, episodeID).Error; err != nil {
		middleware.NotFoundResponse(c, "NOT_FOUND", "单集不存在")
		return
	}

	// 更新备注
	if err := db.Model(&episode).Update("notes", req.Notes).Error; err != nil {
		middleware.InternalErrorResponseWithCode(c, "INTERNAL_ERROR", "更新备注失败: "+err.Error())
		return
	}

	// 使单集所属播客的详情缓存失效
	cache.InvalidatePodcastDetail(episode.PodcastID)

	// 重新获取更新后的单集
	db.First(&episode, episodeID)

	middleware.SuccessResponse(c, gin.H{
		"id":    episode.ID,
		"notes": episode.Notes,
	})
}

// GetEpisodeNotes 获取单集备注
// @Summary 获取单集备注
// @Description 获取指定单集的备注信息
// @Tags Notes
// @Accept json
// @Produce json
// @Param id path int true "单集 ID"
// @Success 200 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Router /api/v1/episodes/{id}/notes [get]
func (h *NoteHandler) GetEpisodeNotes(c *gin.Context) {
	episodeID, ok := ParseUintParam(c, "id")
	if !ok {
		return
	}

	db := database.GetDB()

	var episode models.Episode
	if err := db.Select("id, notes").First(&episode, episodeID).Error; err != nil {
		middleware.NotFoundResponse(c, "NOT_FOUND", "单集不存在")
		return
	}

	middleware.SuccessResponse(c, gin.H{
		"id":    episode.ID,
		"notes": episode.Notes,
	})
}
