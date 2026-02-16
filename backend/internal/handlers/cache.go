package handlers

import (
	"magicpodcast/internal/cache"
	"magicpodcast/internal/middleware"

	"github.com/gin-gonic/gin"
)

// CacheHandler 缓存处理器
type CacheHandler struct{}

// NewCacheHandler 创建缓存处理器
func NewCacheHandler() *CacheHandler {
	return &CacheHandler{}
}

// CacheStatsResponse 缓存统计响应
type CacheStatsResponse struct {
	TotalItems int   `json:"total_items"`
	HitCount   int64 `json:"hit_count"`
	MissCount  int64 `json:"miss_count"`
	HitRate    float64 `json:"hit_rate"`
}

// GetStats 获取缓存统计信息
// @Summary 获取缓存统计
// @Description 获取内存缓存的统计信息
// @Tags Cache
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/cache/stats [get]
func (h *CacheHandler) GetStats(c *gin.Context) {
	stats := cache.GetStats()

	hitRate := float64(0)
	total := stats.HitCount + stats.MissCount
	if total > 0 {
		hitRate = float64(stats.HitCount) / float64(total) * 100
	}

	middleware.SuccessResponse(c, CacheStatsResponse{
		TotalItems: stats.TotalItems,
		HitCount:   stats.HitCount,
		MissCount:  stats.MissCount,
		HitRate:    hitRate,
	})
}

// ClearCache 清空缓存
// @Summary 清空缓存
// @Description 清空所有内存缓存
// @Tags Cache
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/cache/clear [post]
func (h *CacheHandler) ClearCache(c *gin.Context) {
	cache.GetCache().Clear()

	middleware.SuccessResponseWithMessage(c, "Cache cleared successfully", gin.H{
		"cleared": true,
	})
}
