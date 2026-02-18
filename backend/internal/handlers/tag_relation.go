package handlers

import (
	"magicpodcast/internal/cache"
	"magicpodcast/internal/middleware"
	"magicpodcast/internal/services"

	"github.com/gin-gonic/gin"
)

// TagRelationHandlerRefactored 重构后的标签关联处理器
type TagRelationHandlerRefactored struct {
	tagRelationService *services.TagRelationService
}

// NewTagRelationHandlerRefactored 创建重构后的标签关联处理器
func NewTagRelationHandlerRefactored(tagRelationService *services.TagRelationService) *TagRelationHandlerRefactored {
	return &TagRelationHandlerRefactored{
		tagRelationService: tagRelationService,
	}
}

// AddTagRequest 添加标签请求
type AddTagRequest struct {
	TagID uint `json:"tag_id" binding:"required"`
}

// AddTagToPodcast 为播客添加标签
// @Summary 为播客添加标签
// @Description 为指定播客添加标签
// @Tags TagRelations
// @Accept json
// @Produce json
// @Param id path int true "播客 ID"
// @Param request body AddTagRequest true "标签 ID"
// @Success 201 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Router /api/v1/podcasts/{id}/tags [post]
func (h *TagRelationHandlerRefactored) AddTagToPodcast(c *gin.Context) {
	podcastID, ok := ParseUintParam(c, "id")
	if !ok {
		return
	}

	var req AddTagRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.ValidationErrorResponse(c, "request body", "invalid format: "+err.Error())
		return
	}

	// 调用 Service
	result, err := h.tagRelationService.AddTag(services.TargetTypePodcast, podcastID, req.TagID)
	if err != nil {
		middleware.HandleError(c, err)
		return
	}

	// 使相关缓存失效
	cache.InvalidatePodcastDetail(podcastID)
	cache.InvalidateTagList()

	middleware.CreatedResponse(c, gin.H{
		"message":    result.Message,
		"podcast_id": result.TargetID,
		"tag_id":     result.TagID,
		"tag_name":   result.TagName,
	})
}

// RemoveTagFromPodcast 移除播客标签
// @Summary 移除播客标签
// @Description 移除指定播客的标签
// @Tags TagRelations
// @Accept json
// @Produce json
// @Param id path int true "播客 ID"
// @Param tagId path int true "标签 ID"
// @Success 200 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Router /api/v1/podcasts/{id}/tags/{tagId} [delete]
func (h *TagRelationHandlerRefactored) RemoveTagFromPodcast(c *gin.Context) {
	podcastID, ok := ParseUintParam(c, "id")
	if !ok {
		return
	}

	tagID, ok := ParseUintParam(c, "tagId")
	if !ok {
		return
	}

	// 调用 Service
	if err := h.tagRelationService.RemoveTagFromPodcast(podcastID, tagID); err != nil {
		middleware.HandleError(c, err)
		return
	}

	// 使相关缓存失效
	cache.InvalidatePodcastDetail(podcastID)
	cache.InvalidateTagList()

	middleware.SuccessResponse(c, gin.H{
		"message": "标签已移除",
	})
}

// GetPodcastTags 获取播客的所有标签
// @Summary 获取播客标签
// @Description 获取指定播客的所有标签
// @Tags TagRelations
// @Accept json
// @Produce json
// @Param id path int true "播客 ID"
// @Success 200 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Router /api/v1/podcasts/{id}/tags [get]
func (h *TagRelationHandlerRefactored) GetPodcastTags(c *gin.Context) {
	podcastID, ok := ParseUintParam(c, "id")
	if !ok {
		return
	}

	// 调用 Service
	tags, err := h.tagRelationService.GetTags(services.TargetTypePodcast, podcastID)
	if err != nil {
		middleware.HandleError(c, err)
		return
	}

	middleware.SuccessResponse(c, gin.H{
		"tags": tags,
	})
}

// AddTagToEpisode 为单集添加标签
// @Summary 为单集添加标签
// @Description 为指定单集添加标签
// @Tags TagRelations
// @Accept json
// @Produce json
// @Param id path int true "单集 ID"
// @Param request body AddTagRequest true "标签 ID"
// @Success 201 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Router /api/v1/episodes/{id}/tags [post]
func (h *TagRelationHandlerRefactored) AddTagToEpisode(c *gin.Context) {
	episodeID, ok := ParseUintParam(c, "id")
	if !ok {
		return
	}

	var req AddTagRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.ValidationErrorResponse(c, "request body", "invalid format: "+err.Error())
		return
	}

	// 调用 Service
	result, err := h.tagRelationService.AddTag(services.TargetTypeEpisode, episodeID, req.TagID)
	if err != nil {
		middleware.HandleError(c, err)
		return
	}

	// 使标签列表缓存失效（单集标签变化影响标签统计）
	cache.InvalidateTagList()

	middleware.CreatedResponse(c, gin.H{
		"message":    result.Message,
		"episode_id": result.TargetID,
		"tag_id":     result.TagID,
		"tag_name":   result.TagName,
	})
}

// RemoveTagFromEpisode 移除单集标签
// @Summary 移除单集标签
// @Description 移除指定单集的标签
// @Tags TagRelations
// @Accept json
// @Produce json
// @Param id path int true "单集 ID"
// @Param tagId path int true "标签 ID"
// @Success 200 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Router /api/v1/episodes/{id}/tags/{tagId} [delete]
func (h *TagRelationHandlerRefactored) RemoveTagFromEpisode(c *gin.Context) {
	episodeID, ok := ParseUintParam(c, "id")
	if !ok {
		return
	}

	tagID, ok := ParseUintParam(c, "tagId")
	if !ok {
		return
	}

	// 调用 Service
	if err := h.tagRelationService.RemoveTagFromEpisode(episodeID, tagID); err != nil {
		middleware.HandleError(c, err)
		return
	}

	// 使标签列表缓存失效
	cache.InvalidateTagList()

	middleware.SuccessResponse(c, gin.H{
		"message": "标签已移除",
	})
}

// GetEpisodeTags 获取单集的所有标签
// @Summary 获取单集标签
// @Description 获取指定单集的所有标签
// @Tags TagRelations
// @Accept json
// @Produce json
// @Param id path int true "单集 ID"
// @Success 200 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Router /api/v1/episodes/{id}/tags [get]
func (h *TagRelationHandlerRefactored) GetEpisodeTags(c *gin.Context) {
	episodeID, ok := ParseUintParam(c, "id")
	if !ok {
		return
	}

	// 调用 Service
	tags, err := h.tagRelationService.GetTags(services.TargetTypeEpisode, episodeID)
	if err != nil {
		middleware.HandleError(c, err)
		return
	}

	middleware.SuccessResponse(c, gin.H{
		"tags": tags,
	})
}
