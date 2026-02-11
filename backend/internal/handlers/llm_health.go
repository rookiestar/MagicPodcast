package handlers

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"magicpodcast/internal/config"
	"magicpodcast/internal/llm"
	"magicpodcast/internal/logger"
)

// LLMHealthHandler LLM健康检查处理器
type LLMHealthHandler struct {
	llmClient *llm.Client
}

// LLMHealthResponse LLM健康状态响应
type LLMHealthResponse struct {
	Status         string         `json:"status"`          // healthy | degraded | unhealthy
	Model          string         `json:"model"`
	APIKeyValid   bool           `json:"api_key_valid"`
	APIKeySet     bool           `json:"api_key_set"`
	LastCheck     string         `json:"last_check"`
	ResponseTime  int64          `json:"response_time_ms"`
	ErrorMessage  string         `json:"error_message,omitempty"`
}

// NewLLMHealthHandler 创建LLM健康检查处理器
func NewLLMHealthHandler(llmClient *llm.Client) *LLMHealthHandler {
	return &LLMHealthHandler{llmClient: llmClient}
}

// GetHealth 检查LLM服务健康状态
// @Summary 检查LLM服务健康状态
// @Description 检查LLM API连通性和配置有效性
// @Tags LLM
// @Produce json
// @Router /api/v1/llm/health [get]
func (h *LLMHealthHandler) GetHealth(c *gin.Context) {
	cfg := config.Get()
	startTime := time.Now()

	response := LLMHealthResponse{
		Status:       "unknown",
		Model:        cfg.LLM.DefaultModel,
		APIKeyValid:  false,
		APIKeySet:    cfg.LLM.APIKey != "",
		LastCheck:    startTime.Format("2006-01-02T15:04:05Z"),
		ErrorMessage: "",
	}

	// 检查是否启用LLM
	if !cfg.LLM.Enabled {
		response.Status = "disabled"
		response.ErrorMessage = "LLM功能未启用"
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data":    response,
			"message": "LLM功能未启用",
		})
		return
	}

	// 检查API Key是否配置
	if cfg.LLM.APIKey == "" || cfg.LLM.APIKey == "YOUR_ZHIPUAI_API_KEY_HERE" {
		response.Status = "unhealthy"
		response.ErrorMessage = "LLM API Key未配置或使用占位符"
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"data":    response,
			"message": "LLM API Key未配置",
		})
		return
	}

	// 尝试调用LLM API验证（使用简单测试提示）
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 使用一个简单的测试请求
	testPrompt := "测试"
	_, err := h.llmClient.GenerateSummary(ctx, "", testPrompt, llm.SummaryOptions{
		Model:       cfg.LLM.DefaultModel,
		Temperature: 0.1,
		MaxTokens:   10,
	})

	responseTime := time.Since(startTime).Milliseconds()

	if err != nil {
		response.Status = "unhealthy"
		response.ResponseTime = responseTime
		response.APIKeyValid = false
		response.ErrorMessage = fmt.Sprintf("LLM API调用失败: %v", err)
		logger.Errorf("[LLM Health] API check failed: %v", err)

		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"data":    response,
			"message": fmt.Sprintf("LLM服务不可用: %v", err),
		})
		return
	}

	// API调用成功
	response.Status = "healthy"
	response.APIKeyValid = true
	response.ResponseTime = responseTime

	logger.Infof("[LLM Health] API check successful (model: %s, time: %dms)", cfg.LLM.DefaultModel, responseTime)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
		"message": fmt.Sprintf("LLM服务正常 (响应时间: %dms)", responseTime),
	})
}
