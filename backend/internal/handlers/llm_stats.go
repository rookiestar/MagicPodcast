package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// LLMStatsHandler LLM统计处理器
type LLMStatsHandler struct {
	// TODO: 可以在这里添加依赖，如果需要访问全局LLM客户端
}

// NewLLMStatsHandler 创建LLM统计处理器
func NewLLMStatsHandler() *LLMStatsHandler {
	return &LLMStatsHandler{}
}

// LLMStatsResponse LLM统计响应
type LLMStatsResponse struct {
	TotalRequests  int64   `json:"total_requests"`
	TotalTokens    int64   `json:"total_tokens"`
	DailyTokens    int64   `json:"daily_tokens"`
	DailyRequests  int64   `json:"daily_requests"`
	DailyCostCents float64 `json:"daily_cost_cents"`
	LastResetDate  string  `json:"last_reset_date"`
	Enabled        bool    `json:"enabled"`
}

// GetGlobalLLMStats 获取全局LLM使用统计
// @Summary 获取全局LLM使用统计
// @Description 获取LLM客户端的使用统计信息
// @Tags LLM
// @Produce json
// @Router /api/v1/llm/stats [get]
func (h *LLMStatsHandler) GetGlobalLLMStats(c *gin.Context) {
	// TODO: 这里需要从全局LLM客户端获取统计信息
	// 由于LLM客户端目前在router中初始化，我们需要一个全局访问方式
	// 或者将stats存储在数据库/缓存中

	// 临时返回一个示例响应
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": LLMStatsResponse{
			TotalRequests:  0,
			TotalTokens:    0,
			DailyTokens:    0,
			DailyRequests:  0,
			DailyCostCents: 0.0,
			LastResetDate:  "",
			Enabled:        true,
		},
		"message": "LLM统计功能已集成，但需要实现全局LLM客户端访问（待完善）",
	})
}
