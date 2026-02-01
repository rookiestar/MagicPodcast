package handlers

import (
	"net/http"
	"strconv"

	"magicpodcast/internal/database"
	"magicpodcast/internal/models"

	"github.com/gin-gonic/gin"
)

// TagRelationHandler 标签关联处理器
type TagRelationHandler struct{}

// NewTagRelationHandler 创建标签关联处理器
func NewTagRelationHandler() *TagRelationHandler {
	return &TagRelationHandler{}
}

// AddTagToPodcastRequest 为播客添加标签请求
type AddTagToPodcastRequest struct {
	TagID uint `json:"tag_id" binding:"required"`
}

// AddTagToPodcast 为播客添加标签
// @Summary 为播客添加标签
// @Description 为指定播客添加标签
// @Tags TagRelations
// @Accept json
// @Produce json
// @Param id path int true "播客 ID"
// @Param request body AddTagToPodcastRequest true "标签 ID"
// @Success 201 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Router /api/v1/podcasts/{id}/tags [post]
func (h *TagRelationHandler) AddTagToPodcast(c *gin.Context) {
	id := c.Param("id")
	podcastID, err := strconv.ParseUint(id, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "INVALID_ID",
				"message": "无效的播客 ID",
			},
		})
		return
	}

	var req AddTagToPodcastRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "INVALID_REQUEST",
				"message": "请求参数错误: " + err.Error(),
			},
		})
		return
	}

	db := database.GetDB()

	// 检查播客是否存在
	var podcast models.Podcast
	if err := db.First(&podcast, podcastID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "NOT_FOUND",
				"message": "播客不存在",
			},
		})
		return
	}

	// 检查标签是否存在
	var tag models.Tag
	if err := db.First(&tag, req.TagID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "NOT_FOUND",
				"message": "标签不存在",
			},
		})
		return
	}

	// 检查关联是否已存在
	var count int64
	db.Model(&podcast).Where("tag_id = ?", req.TagID).Count(&count)
	if count > 0 {
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "RELATION_EXISTS",
				"message": "该播客已有此标签",
			},
		})
		return
	}

	// 添加关联
	if err := db.Model(&podcast).Association("Tags").Append(&tag); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "INTERNAL_ERROR",
				"message": "添加标签失败: " + err.Error(),
			},
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data": gin.H{
			"message":    "标签已添加",
			"podcast_id": podcastID,
			"tag_id":     req.TagID,
			"tag_name":   tag.Name,
		},
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
func (h *TagRelationHandler) RemoveTagFromPodcast(c *gin.Context) {
	id := c.Param("id")
	podcastID, err := strconv.ParseUint(id, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "INVALID_ID",
				"message": "无效的播客 ID",
			},
		})
		return
	}

	tagId := c.Param("tagId")
	tagID, err := strconv.ParseUint(tagId, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "INVALID_ID",
				"message": "无效的标签 ID",
			},
		})
		return
	}

	db := database.GetDB()

	// 检查播客是否存在
	var podcast models.Podcast
	if err := db.First(&podcast, podcastID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "NOT_FOUND",
				"message": "播客不存在",
			},
		})
		return
	}

	// 检查标签是否存在
	var tag models.Tag
	if err := db.First(&tag, tagID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "NOT_FOUND",
				"message": "标签不存在",
			},
		})
		return
	}

	// 移除关联
	if err := db.Model(&podcast).Association("Tags").Delete(&tag); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "INTERNAL_ERROR",
				"message": "移除标签失败: " + err.Error(),
			},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"message": "标签已移除",
		},
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
func (h *TagRelationHandler) GetPodcastTags(c *gin.Context) {
	id := c.Param("id")
	podcastID, err := strconv.ParseUint(id, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "INVALID_ID",
				"message": "无效的播客 ID",
			},
		})
		return
	}

	db := database.GetDB()

	// 检查播客是否存在
	var podcast models.Podcast
	if err := db.Preload("Tags").First(&podcast, podcastID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "NOT_FOUND",
				"message": "播客不存在",
			},
		})
		return
	}

	// 转换为响应格式，并获取每个标签的播客数量
	type TagWithCount struct {
		ID           uint   `json:"id"`
		Name         string `json:"name"`
		Color        string `json:"color"`
		PodcastCount int    `json:"podcast_count"`
	}

	var tagsWithCount []TagWithCount
	for _, tag := range podcast.Tags {
		// 查询每个标签的播客数量
		var count int64
		db.Table("podcasts_tags").Where("tag_id = ?", tag.ID).Count(&count)

		tagsWithCount = append(tagsWithCount, TagWithCount{
			ID:           tag.ID,
			Name:         tag.Name,
			Color:        tag.Color,
			PodcastCount: int(count),
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    tagsWithCount,
	})
}

// AddTagToEpisode 为单集添加标签
// @Summary 为单集添加标签
// @Description 为指定单集添加标签
// @Tags TagRelations
// @Accept json
// @Produce json
// @Param id path int true "单集 ID"
// @Param request body AddTagToPodcastRequest true "标签 ID"
// @Success 201 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Router /api/v1/episodes/{id}/tags [post]
func (h *TagRelationHandler) AddTagToEpisode(c *gin.Context) {
	id := c.Param("id")
	episodeID, err := strconv.ParseUint(id, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "INVALID_ID",
				"message": "无效的单集 ID",
			},
		})
		return
	}

	var req AddTagToPodcastRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "INVALID_REQUEST",
				"message": "请求参数错误: " + err.Error(),
			},
		})
		return
	}

	db := database.GetDB()

	// 检查单集是否存在
	var episode models.Episode
	if err := db.First(&episode, episodeID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "NOT_FOUND",
				"message": "单集不存在",
			},
		})
		return
	}

	// 检查标签是否存在
	var tag models.Tag
	if err := db.First(&tag, req.TagID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "NOT_FOUND",
				"message": "标签不存在",
			},
		})
		return
	}

	// 检查关联是否已存在
	var count int64
	db.Model(&episode).Where("tag_id = ?", req.TagID).Count(&count)
	if count > 0 {
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "RELATION_EXISTS",
				"message": "该单集已有此标签",
			},
		})
		return
	}

	// 添加关联
	if err := db.Model(&episode).Association("Tags").Append(&tag); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "INTERNAL_ERROR",
				"message": "添加标签失败: " + err.Error(),
			},
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data": gin.H{
			"message":    "标签已添加",
			"episode_id": episodeID,
			"tag_id":     req.TagID,
			"tag_name":   tag.Name,
		},
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
func (h *TagRelationHandler) RemoveTagFromEpisode(c *gin.Context) {
	id := c.Param("id")
	episodeID, err := strconv.ParseUint(id, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "INVALID_ID",
				"message": "无效的单集 ID",
			},
		})
		return
	}

	tagId := c.Param("tagId")
	tagID, err := strconv.ParseUint(tagId, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "INVALID_ID",
				"message": "无效的标签 ID",
			},
		})
		return
	}

	db := database.GetDB()

	// 检查单集是否存在
	var episode models.Episode
	if err := db.First(&episode, episodeID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "NOT_FOUND",
				"message": "单集不存在",
			},
		})
		return
	}

	// 检查标签是否存在
	var tag models.Tag
	if err := db.First(&tag, tagID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "NOT_FOUND",
				"message": "标签不存在",
			},
		})
		return
	}

	// 移除关联
	if err := db.Model(&episode).Association("Tags").Delete(&tag); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "INTERNAL_ERROR",
				"message": "移除标签失败: " + err.Error(),
			},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"message": "标签已移除",
		},
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
func (h *TagRelationHandler) GetEpisodeTags(c *gin.Context) {
	id := c.Param("id")
	episodeID, err := strconv.ParseUint(id, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "INVALID_ID",
				"message": "无效的单集 ID",
			},
		})
		return
	}

	db := database.GetDB()

	// 检查单集是否存在
	var episode models.Episode
	if err := db.Preload("Tags").First(&episode, episodeID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "NOT_FOUND",
				"message": "单集不存在",
			},
		})
		return
	}

	// 转换为响应格式，并获取每个标签的播客数量
	type TagWithCount struct {
		ID           uint   `json:"id"`
		Name         string `json:"name"`
		Color        string `json:"color"`
		PodcastCount int    `json:"podcast_count"`
	}

	var tagsWithCount []TagWithCount
	for _, tag := range episode.Tags {
		// 查询每个标签的播客数量
		var count int64
		db.Table("podcasts_tags").Where("tag_id = ?", tag.ID).Count(&count)

		tagsWithCount = append(tagsWithCount, TagWithCount{
			ID:           tag.ID,
			Name:         tag.Name,
			Color:        tag.Color,
			PodcastCount: int(count),
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    tagsWithCount,
	})
}
