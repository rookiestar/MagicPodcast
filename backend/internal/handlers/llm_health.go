package handlers

import (
	"time"

	"github.com/gin-gonic/gin"
	"magicpodcast/internal/config"
	"magicpodcast/internal/llm"
	"magicpodcast/internal/middleware"
)

// LLMHealthHandler LLM健康检查处理器
type LLMHealthHandler struct {
	llmClient *llm.Client
}

// LLMHealthResponse LLM健康状态响应
type LLMHealthResponse struct {
	Status         string `json:"status"` // healthy | degraded | unhealthy
	Model          string `json:"model"`
	APIKeyValid    bool   `json:"api_key_valid"`
	APIKeySet      bool   `json:"api_key_set"`
	LastCheck      string `json:"last_check"`
	ResponseTime   int64  `json:"response_time_ms"`
	ProbePerformed bool   `json:"probe_performed"`
	Source         string `json:"source"`
	ErrorMessage   string `json:"error_message,omitempty"`
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
		Status:         "unknown",
		Model:          cfg.LLM.DefaultModel,
		APIKeyValid:    false,
		APIKeySet:      cfg.LLM.APIKey != "",
		LastCheck:      startTime.Format("2006-01-02T15:04:05Z"),
		ErrorMessage:   "",
		ProbePerformed: false,
		Source:         "passive",
	}

	// 检查是否启用LLM
	if !cfg.LLM.Enabled {
		response.Status = "disabled"
		response.ErrorMessage = "LLM功能未启用"
		middleware.SuccessResponseWithMessage(c, "LLM功能未启用", response)
		return
	}

	// 检查API Key是否配置
	if cfg.LLM.APIKey == "" || cfg.LLM.APIKey == "YOUR_ZHIPUAI_API_KEY_HERE" {
		response.Status = "unhealthy"
		response.ErrorMessage = "LLM API Key未配置或使用占位符"
		middleware.SuccessResponseWithMessage(c, "LLM API Key未配置", response)
		return
	}

	// 健康读取必须是被动的：不能因为页面轮询而触发真实生成或费用。
	// 真正的可用性由工作流/显式验证调用后的结果体现，这里只报告配置状态。
	response.Status = "unknown"
	response.ErrorMessage = "已配置但未主动探测；健康读取不会调用付费生成"
	middleware.SuccessResponseWithMessage(c, "LLM状态为被动状态，未执行付费检查", response)
}
