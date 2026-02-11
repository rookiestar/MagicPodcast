package handlers

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"magicpodcast/internal/config"
	"magicpodcast/internal/database"
	"magicpodcast/internal/models"
)

// LLMStatsHandler LLM统计处理器
type LLMStatsHandler struct {
	db *gorm.DB
}

// LLMStatsResponse LLM统计响应
type LLMStatsResponse struct {
	Period          Period            `json:"period"`
	Usage           Usage             `json:"usage"`
	Errors          Errors            `json:"errors"`
	Workflows       LLMWorkflowStats  `json:"workflows"`
	TotalRequests   int64             `json:"total_requests"`
	TotalTokens     int64             `json:"total_tokens"`
	DailyTokens     int64             `json:"daily_tokens"`
	DailyRequests   int64             `json:"daily_requests"`
	DailyCostCents float64           `json:"daily_cost_cents"`
	LastResetDate  string            `json:"last_reset_date"`
	Enabled        bool              `json:"enabled"`
}

// Period 时间段
type Period struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

// Usage 使用情况
type Usage struct {
	TotalTokens        int64   `json:"total_tokens"`
	TotalRequests      int64   `json:"total_requests"`
	AvgTokensPerReq    float64 `json:"avg_tokens_per_request"`
	TotalCostEstimate  string  `json:"total_cost_estimate"`
}

// Errors 错误统计
type Errors struct {
	TotalFailures int64        `json:"total_failures"`
	FailureRate  float64      `json:"failure_rate"`
	TopErrors    []ErrorInfo  `json:"top_errors"`
}

// ErrorInfo 错误信息
type ErrorInfo struct {
	Error        string `json:"error"`
	Count        int64  `json:"count"`
	LastOccurrence string `json:"last_occurrence"`
}

// LLMWorkflowStats LLM工作流统计
type LLMWorkflowStats struct {
	TotalWithLLM int `json:"total_with_llm"`
	Successful    int `json:"successful"`
	Failed       int `json:"failed"`
}

// NewLLMStatsHandler 创建LLM统计处理器
func NewLLMStatsHandler() *LLMStatsHandler {
	db := database.GetDB()
	return &LLMStatsHandler{db: db}
}

// GetGlobalLLMStats 获取全局LLM使用统计
// @Summary 获取全局LLM使用统计
// @Tags LLM
// @Produce json
// @Router /api/v1/llm/stats [get]
func (h *LLMStatsHandler) GetGlobalLLMStats(c *gin.Context) {
	cfg := config.Get()

	// 获取查询参数（默认7天）
	days := 7
	if d := c.Query("days"); d != "" {
		fmt.Sscanf(d, "%d", &days)
	}

	// 计算时间范围
	now := time.Now()
	startTime := now.AddDate(0, 0, -days)
	endTime := now

	// 构建查询
	var stats []struct {
		TotalTokens   int64
		TotalRequests int64
	}

	// 成功的LLM调用
	h.db.Model(&models.Report{}).
		Where("created_at >= ? AND created_at <= ?", startTime, endTime).
		Where("llm_error IS NULL OR llm_error = ''").
		Select("COALESCE(SUM(llm_tokens_used), 0) as total_tokens, COUNT(*) as total_requests").
		Scan(&stats)

	// 成功统计
	successTokens := int64(0)
	successRequests := int64(0)
	if len(stats) > 0 {
		successTokens = stats[0].TotalTokens
		successRequests = stats[0].TotalRequests
	}

	// 失败的LLM调用
	var failStats []struct {
		Count int64
	}
	h.db.Model(&models.Report{}).
		Where("created_at >= ? AND created_at <= ?", startTime, endTime).
		Where("llm_error IS NOT NULL AND llm_error != ''").
		Select("COUNT(*) as count").
		Scan(&failStats)

	failCount := int64(0)
	if len(failStats) > 0 {
		failCount = failStats[0].Count
	}

	totalRequests := successRequests + failCount
	totalTokens := successTokens
	failureRate := 0.0
	if totalRequests > 0 {
		failureRate = float64(failCount) / float64(totalRequests)
	}

	// 平均token数
	avgTokens := 0.0
	if successRequests > 0 {
		avgTokens = float64(successTokens) / float64(successRequests)
	}

	// 成本估算（智谱AI GLM-4.5-air: ¥0.5/1M tokens）
	totalCost := float64(successTokens) * 0.5 / 1000000.0

	// Top错误统计
	var topErrors []ErrorInfo
	h.db.Model(&models.Report{}).
		Where("created_at >= ? AND created_at <= ?", startTime, endTime).
		Where("llm_error IS NOT NULL AND llm_error != ''").
		Select("llm_error as error, COUNT(*) as count, MAX(created_at) as last_occurrence").
		Group("llm_error").
		Order("count DESC").
		Limit(5).
		Scan(&topErrors)

	// 工作流统计
	var workflowStats []struct {
		Total    int
		Success  int
		Failed   int
	}
	h.db.Raw(`
			SELECT
				COUNT(*) as total,
				SUM(CASE WHEN llm_error IS NULL OR llm_error = '' THEN 1 ELSE 0 END) as success,
				SUM(CASE WHEN llm_error IS NOT NULL AND llm_error != '' THEN 1 ELSE 0 END) as failed
			FROM reports
			WHERE created_at >= ? AND created_at <= ?
		`, startTime, endTime).Scan(&workflowStats)

	totalWorkflows := 0
	successfulWorkflows := 0
	failedWorkflows := 0
	if len(workflowStats) > 0 {
		totalWorkflows = workflowStats[0].Total
		successfulWorkflows = workflowStats[0].Success
		failedWorkflows = workflowStats[0].Failed
	}

	// 今日统计
	todayStart := time.Now().Truncate(24 * time.Hour)
	var dailyStats []struct {
		Tokens   int64
		Requests int64
	}
	h.db.Model(&models.Report{}).
		Where("created_at >= ?", todayStart).
		Where("llm_error IS NULL OR llm_error = ''").
		Select("COALESCE(SUM(llm_tokens_used), 0) as tokens, COUNT(*) as requests").
		Scan(&dailyStats)

	dailyTokens := int64(0)
	dailyRequests := int64(0)
	if len(dailyStats) > 0 {
		dailyTokens = dailyStats[0].Tokens
		dailyRequests = dailyStats[0].Requests
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": LLMStatsResponse{
			Period: Period{
				Start: startTime.Format("2006-01-02T15:04:05Z"),
				End:   endTime.Format("2006-01-02T15:04:05Z"),
			},
			Usage: Usage{
				TotalTokens:       totalTokens,
				TotalRequests:     totalRequests,
				AvgTokensPerReq:  avgTokens,
				TotalCostEstimate: fmt.Sprintf("¥%.2f", totalCost),
			},
			Errors: Errors{
				TotalFailures: failCount,
				FailureRate:  failureRate,
				TopErrors:    topErrors,
			},
			Workflows: LLMWorkflowStats{
				TotalWithLLM: totalWorkflows,
				Successful:    successfulWorkflows,
				Failed:       failedWorkflows,
			},
			TotalRequests:  totalRequests,
			TotalTokens:    totalTokens,
			DailyTokens:    dailyTokens,
			DailyRequests:  dailyRequests,
			DailyCostCents: totalCost * 100, // 转换为分
			LastResetDate:  todayStart.Format("2006-01-02T15:04:05Z"),
			Enabled:        cfg.LLM.Enabled,
		},
		"message": fmt.Sprintf("最近%d天的LLM使用统计", days),
	})
}
