package handlers

import (
	"net/http"
	"strconv"

	"magicpodcast/internal/services"
	"magicpodcast/internal/validation"

	"github.com/gin-gonic/gin"
)

// SearchHandler 搜索处理器
type SearchHandler struct {
	searchService *services.SearchService
}

// NewSearchHandler 创建搜索处理器
func NewSearchHandler() *SearchHandler {
	return &SearchHandler{
		searchService: services.NewSearchService(),
	}
}

// Search 全局搜索
// @Summary 全局搜索
// @Description 搜索播客和单集，支持关键词检索、标签筛选和分页
// @Tags Search
// @Accept json
// @Produce json
// @Param q query string true "搜索关键词"
// @Param type query string false "搜索类型" Enums(all, podcasts, episodes)
// @Param tag_id query []int false "标签ID列表"
// @Param page query int false "播客页码（默认1）"
// @Param page_size query int false "播客每页数量（默认20）"
// @Param episode_page query int false "单集页码（默认1）"
// @Param episode_page_size query int false "单集每页数量（默认20）"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/search [get]
func (h *SearchHandler) Search(c *gin.Context) {
	// 获取搜索关键词
	query := c.Query("q")

	// 输入验证
	v := validation.New()
	v.ValidateRequired("q", query).
		ValidateStringLength("q", query, 1, 200)

	if v.HasErrors() {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "VALIDATION_ERROR",
				"message": v.Error(),
			},
		})
		return
	}

	// 获取搜索类型
	searchType := c.DefaultQuery("type", "all")
	v.ValidateEnum("type", searchType, []string{"all", "podcasts", "episodes"})

	// 获取标签筛选
	tagIDStrs := c.QueryArray("tag_id")
	var tagIDs []uint
	for _, tagIDStr := range tagIDStrs {
		tagID, err := strconv.ParseUint(tagIDStr, 10, 32)
		if err == nil {
			tagIDs = append(tagIDs, uint(tagID))
		}
	}

	// 分页参数
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	episodePage, _ := strconv.Atoi(c.DefaultQuery("episode_page", "1"))
	episodePageSize, _ := strconv.Atoi(c.DefaultQuery("episode_page_size", "20"))

	// 验证分页参数范围
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	if episodePage < 1 {
		episodePage = 1
	}
	if episodePageSize < 1 || episodePageSize > 100 {
		episodePageSize = 20
	}

	// 构建请求
	req := services.SearchRequest{
		Query:           query,
		Type:            searchType,
		TagIDs:          tagIDs,
		Page:            page,
		PageSize:        pageSize,
		EpisodePage:     episodePage,
		EpisodePageSize: episodePageSize,
	}

	// 执行搜索
	result, err := h.searchService.Search(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "SEARCH_ERROR",
				"message": err.Error(),
			},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}
