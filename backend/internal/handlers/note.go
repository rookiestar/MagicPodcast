package handlers

import (
	"net/http"

	"magicpodcast/internal/database"
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

	// 更新备注
	if err := db.Model(&podcast).Update("notes", req.Notes).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "INTERNAL_ERROR",
				"message": "更新备注失败: " + err.Error(),
			},
		})
		return
	}

	// 重新获取更新后的播客
	db.First(&podcast, podcastID)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"id":    podcast.ID,
			"notes": podcast.Notes,
		},
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
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "NOT_FOUND",
				"message": "播客不存在",
			},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"id":    podcast.ID,
			"notes": podcast.Notes,
		},
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

	// 更新备注
	if err := db.Model(&episode).Update("notes", req.Notes).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "INTERNAL_ERROR",
				"message": "更新备注失败: " + err.Error(),
			},
		})
		return
	}

	// 重新获取更新后的单集
	db.First(&episode, episodeID)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"id":    episode.ID,
			"notes": episode.Notes,
		},
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
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "NOT_FOUND",
				"message": "单集不存在",
			},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"id":    episode.ID,
			"notes": episode.Notes,
		},
	})
}
