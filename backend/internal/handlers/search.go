package handlers

import (
	"magicpodcast/internal/middleware"
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
		middleware.BadRequestResponse(c, "VALIDATION_ERROR", v.Error())
		return
	}

	// 获取搜索类型
	searchType := c.DefaultQuery("type", "all")
	v.ValidateEnum("type", searchType, []string{"all", "podcasts", "episodes"})

	// 获取标签筛选（使用辅助函数）
	tagIDs := ParseUintSliceQueryParam(c, "tag_id")

	// 分页参数（使用辅助函数）
	podcastPagination := ParsePaginationParams(c, 20)
	episodePagination := ParsePaginationParams(c, 20)

	// 如果指定了 episode_page/episode_page_size，使用它们
	episodePage := episodePagination.Page
	episodePageSize := episodePagination.PageSize
	if ep := c.Query("episode_page"); ep != "" {
		episodePagination := ParsePaginationParams(c, 20)
		episodePage = episodePagination.Page
	}
	if eps := c.Query("episode_page_size"); eps != "" {
		episodePagination := ParsePaginationParams(c, 20)
		episodePageSize = episodePagination.PageSize
	}

	// 构建请求
	req := services.SearchRequest{
		Query:           query,
		Type:            searchType,
		TagIDs:          tagIDs,
		Page:            podcastPagination.Page,
		PageSize:        podcastPagination.PageSize,
		EpisodePage:     episodePage,
		EpisodePageSize: episodePageSize,
	}

	// 执行搜索
	result, err := h.searchService.Search(req)
	if err != nil {
		middleware.InternalErrorResponseWithCode(c, "SEARCH_ERROR", err.Error())
		return
	}

	middleware.SuccessResponse(c, result)
}
