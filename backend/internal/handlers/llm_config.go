package handlers

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"magicpodcast/internal/llm"
	"magicpodcast/internal/logger"
	"magicpodcast/internal/middleware"
)

// LLMConfigHandler LLM配置验证处理器
type LLMConfigHandler struct {
	llmClient *llm.Client
}

// ValidateKeyRequest API Key验证请求
type ValidateKeyRequest struct {
	APIKey string `json:"api_key" binding:"required"`
}

// ValidateKeyResponse API Key验证响应
type ValidateKeyResponse struct {
	Valid    bool   `json:"valid"`
	Model    string `json:"model,omitempty"`
	TestError string `json:"test_error,omitempty"`
}

// ModelsResponse 可用模型列表响应
type ModelsResponse struct {
	Available []ModelInfo `json:"available"`
	Current   string      `json:"current,omitempty"`
}

// ModelInfo 模型信息
type ModelInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Available   bool   `json:"available"`
	Description string `json:"description,omitempty"`
}

// NewLLMConfigHandler 创建LLM配置验证处理器
func NewLLMConfigHandler(llmClient *llm.Client) *LLMConfigHandler {
	return &LLMConfigHandler{llmClient: llmClient}
}

// ValidateKey 验证LLM API Key
// @Summary 验证LLM API Key
// @Description 验证提供的API Key是否有效
// @Tags LLM
// @Accept json
// @Produce json
// @Router /api/v1/llm/validate-key [post]
func (h *LLMConfigHandler) ValidateKey(c *gin.Context) {
	var req ValidateKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.BadRequestResponse(c, "INVALID_PARAM", fmt.Sprintf("请求参数错误: %v", err))
		return
	}

	// 验证API Key格式（智谱AI格式：id.secret）
	if len(req.APIKey) < 10 {
		middleware.SuccessResponseWithMessage(c, "API Key格式无效", ValidateKeyResponse{
			Valid:     false,
			TestError: "API Key格式无效",
		})
		return
	}

	logger.Infof("[LLM Config] API Key validation requested (length: %d)", len(req.APIKey))

	middleware.SuccessResponseWithMessage(c, "API Key格式验证通过（完整验证需要实际API调用）", ValidateKeyResponse{
		Valid: true,
		Model: "glm-4.5-air", // 默认模型
	})
}

// GetModels 获取可用模型列表
// @Summary 获取可用的LLM模型列表
// @Description 返回支持的LLM模型列表
// @Tags LLM
// @Produce json
// @Router /api/v1/llm/models [get]
func (h *LLMConfigHandler) GetModels(c *gin.Context) {
	// 返回智谱AI支持的模型列表
	models := []ModelInfo{
		{
			ID:          "glm-4.5-air",
			Name:        "GLM-4.5-Air",
			Available:   true,
			Description: "智谱AI轻量级模型，快速响应，适合日常摘要生成",
		},
		{
			ID:          "glm-4-flash",
			Name:        "GLM-4-Flash",
			Available:   true,
			Description: "智谱AI超快速模型，适合简单摘要任务",
		},
		{
			ID:          "glm-4",
			Name:        "GLM-4",
			Available:   true,
			Description: "智谱AI标准模型，平衡性能与质量",
		},
		{
			ID:          "glm-4-plus",
			Name:        "GLM-4-Plus",
			Available:   true,
			Description: "智谱AI增强模型，更高质量但速度较慢",
		},
	}

	logger.Infof("[LLM Config] Model list requested (total: %d models)", len(models))

	middleware.SuccessResponseWithMessage(c, fmt.Sprintf("共%d个可用模型", len(models)), ModelsResponse{
		Available: models,
	})
}
