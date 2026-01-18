package handlers

import (
	"net/http"
	"strconv"

	"magicpodcast/internal/database"
	"magicpodcast/internal/models"

	"github.com/gin-gonic/gin"
)

// TagHandler 标签处理器
type TagHandler struct{}

// NewTagHandler 创建标签处理器
func NewTagHandler() *TagHandler {
	return &TagHandler{}
}

// TagResponse 标签响应结构
type TagResponse struct {
	ID           uint   `json:"id"`
	Name         string `json:"name"`
	Color        string `json:"color"`
	PodcastCount int    `json:"podcast_count"`
}

// CreateTagRequest 创建标签请求
type CreateTagRequest struct {
	Name  string `json:"name" binding:"required,max=64"`
	Color string `json:"color" binding:"omitempty,len=7"`
}

// UpdateTagRequest 更新标签请求
type UpdateTagRequest struct {
	Color string `json:"color" binding:"omitempty,len=7"`
}

// Create 创建标签
// @Summary 创建标签
// @Description 创建新的标签
// @Tags Tags
// @Accept json
// @Produce json
// @Param request body CreateTagRequest true "标签信息"
// @Success 201 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Router /api/v1/tags [post]
func (h *TagHandler) Create(c *gin.Context) {
	var req CreateTagRequest
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

	// 检查标签名称是否已存在
	var existingTag models.Tag
	if err := db.Where("name = ?", req.Name).First(&existingTag).Error; err == nil {
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "TAG_EXISTS",
				"message": "标签名称已存在",
			},
		})
		return
	}

	// 创建标签
	tag := models.Tag{
		Name:  req.Name,
		Color: req.Color,
	}

	if err := db.Create(&tag).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "INTERNAL_ERROR",
				"message": "创建标签失败: " + err.Error(),
			},
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data":    TagResponse{ID: tag.ID, Name: tag.Name, Color: tag.Color, PodcastCount: 0},
	})
}

// List 获取标签列表
// @Summary 获取标签列表
// @Description 获取所有标签，按关联播客数量降序排序
// @Tags Tags
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/tags [get]
func (h *TagHandler) List(c *gin.Context) {
	db := database.GetDB()

	// 使用原生SQL查询每个标签的播客数量，并按数量降序排序
	type TagWithCount struct {
		ID           uint   `json:"id"`
		Name         string `json:"name"`
		Color        string `json:"color"`
		PodcastCount int    `json:"podcast_count"`
	}

	var tagsWithCount []TagWithCount
	if err := db.Raw(`
		SELECT tags.id, tags.name, tags.color, COUNT(podcasts_tags.podcast_id) as podcast_count
		FROM tags
		LEFT JOIN podcasts_tags ON tags.id = podcasts_tags.tag_id
		GROUP BY tags.id
		ORDER BY podcast_count DESC, tags.id ASC
	`).Scan(&tagsWithCount).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "INTERNAL_ERROR",
				"message": "获取标签列表失败: " + err.Error(),
			},
		})
		return
	}

	// 转换为响应格式
	response := make([]TagResponse, len(tagsWithCount))
	for i, tag := range tagsWithCount {
		response[i] = TagResponse{
			ID:           tag.ID,
			Name:         tag.Name,
			Color:        tag.Color,
			PodcastCount: tag.PodcastCount,
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
	})
}

// Get 获取单个标签详情
// @Summary 获取标签详情
// @Description 根据 ID 获取标签详情
// @Tags Tags
// @Accept json
// @Produce json
// @Param id path int true "标签 ID"
// @Success 200 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Router /api/v1/tags/{id} [get]
func (h *TagHandler) Get(c *gin.Context) {
	id := c.Param("id")

	db := database.GetDB()

	// 查询标签及关联的播客数量
	var count int64
	db.Table("podcasts_tags").Where("tag_id = ?", id).Count(&count)

	var tag models.Tag
	if err := db.First(&tag, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "NOT_FOUND",
				"message": "标签不存在",
			},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": TagResponse{
			ID:           tag.ID,
			Name:         tag.Name,
			Color:        tag.Color,
			PodcastCount: int(count),
		},
	})
}

// Update 更新标签
// @Summary 更新标签
// @Description 更新标签信息
// @Tags Tags
// @Accept json
// @Produce json
// @Param id path int true "标签 ID"
// @Param request body UpdateTagRequest true "更新信息"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Router /api/v1/tags/{id} [put]
func (h *TagHandler) Update(c *gin.Context) {
	id := c.Param("id")
	tagID, err := strconv.ParseUint(id, 10, 32)
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

	var req UpdateTagRequest
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

	// 检查标签是否存在
	var tag models.Tag
	if err := db.First(&tag, uint(tagID)).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "NOT_FOUND",
				"message": "标签不存在",
			},
		})
		return
	}

	// 更新标签
	updates := map[string]interface{}{}
	if req.Color != "" {
		updates["color"] = req.Color
	}

	if err := db.Model(&tag).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "INTERNAL_ERROR",
				"message": "更新标签失败: " + err.Error(),
			},
		})
		return
	}

	// 重新获取更新后的标签及其播客数量
	db.First(&tag, tagID)

	var count int64
	db.Table("podcasts_tags").Where("tag_id = ?", tagID).Count(&count)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": TagResponse{
			ID:           tag.ID,
			Name:         tag.Name,
			Color:        tag.Color,
			PodcastCount: int(count),
		},
	})
}

// Delete 删除标签
// @Summary 删除标签
// @Description 删除指定标签
// @Tags Tags
// @Accept json
// @Produce json
// @Param id path int true "标签 ID"
// @Success 200 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Router /api/v1/tags/{id} [delete]
func (h *TagHandler) Delete(c *gin.Context) {
	id := c.Param("id")

	db := database.GetDB()

	// 检查标签是否存在
	var tag models.Tag
	if err := db.First(&tag, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "NOT_FOUND",
				"message": "标签不存在",
			},
		})
		return
	}

	// 删除标签（GORM会自动删除关联关系）
	if err := db.Delete(&tag).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "INTERNAL_ERROR",
				"message": "删除标签失败: " + err.Error(),
			},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"message": "标签已删除",
		},
	})
}
