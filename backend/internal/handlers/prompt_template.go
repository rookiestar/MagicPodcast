package handlers

import (
	"time"

	"github.com/gin-gonic/gin"
	"magicpodcast/internal/llm"
	"magicpodcast/internal/middleware"
)

// PromptTemplateHandler Prompt模板处理器
type PromptTemplateHandler struct {
	promptManager *llm.PromptManager
}

// NewPromptTemplateHandler 创建Prompt模板处理器
func NewPromptTemplateHandler(promptManager *llm.PromptManager) *PromptTemplateHandler {
	return &PromptTemplateHandler{
		promptManager: promptManager,
	}
}

// PromptTemplateRequest Prompt模板请求
type PromptTemplateRequest struct {
	Name        string `json:"name" binding:"required"`
	Content     string `json:"content" binding:"required"`
	Description string `json:"description"`
}

// PromptTemplateResponse Prompt模板响应
type PromptTemplateResponse struct {
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Content     string    `json:"content"`
	IsDefault   bool      `json:"is_default"`
	ModifiedAt  time.Time `json:"modified_at"`
}

// ListTemplates 获取所有Prompt模板
// @Summary 获取所有Prompt模板列表
// @Description 列出所有可用的Prompt模板
// @Tags PromptTemplates
// @Produce json
// @Router /api/v1/prompt-templates [get]
func (h *PromptTemplateHandler) ListTemplates(c *gin.Context) {
	templates, err := h.promptManager.ListTemplates()
	if err != nil {
		middleware.InternalErrorResponseWithCode(c, "LIST_FAILED", "获取模板列表失败")
		return
	}

	// 转换为响应格式
	response := make([]PromptTemplateResponse, len(templates))
	for i, tpl := range templates {
		response[i] = PromptTemplateResponse{
			Name:        tpl.Name,
			Description: tpl.Description,
			Content:     tpl.Content,
			IsDefault:   tpl.IsDefault,
			ModifiedAt:  tpl.ModifiedAt,
		}
	}

	middleware.SuccessResponse(c, response)
}

// GetTemplate 获取特定Prompt模板
// @Summary 获取Prompt模板详情
// @Description 获取指定名称的Prompt模板内容
// @Tags PromptTemplates
// @Produce json
// @Param name path string true "模板名称"
// @Router /api/v1/prompt-templates/{name} [get]
func (h *PromptTemplateHandler) GetTemplate(c *gin.Context) {
	name := c.Param("name")

	// 获取模板信息
	templates, err := h.promptManager.ListTemplates()
	if err != nil {
		middleware.InternalErrorResponseWithCode(c, "LIST_FAILED", "获取模板列表失败")
		return
	}

	// 查找目标模板
	var targetTemplate *llm.PromptFileInfo
	for _, tpl := range templates {
		if tpl.Name == name {
			targetTemplate = &tpl
			break
		}
	}

	if targetTemplate == nil {
		middleware.NotFoundResponse(c, "NOT_FOUND", "模板不存在")
		return
	}

	middleware.SuccessResponse(c, PromptTemplateResponse{
		Name:        targetTemplate.Name,
		Description: targetTemplate.Description,
		Content:     targetTemplate.Content,
		IsDefault:   targetTemplate.IsDefault,
		ModifiedAt:  targetTemplate.ModifiedAt,
	})
}

// CreateTemplate 创建新的Prompt模板
// @Summary 创建Prompt模板
// @Description 创建新的Prompt模板文件
// @Tags PromptTemplates
// @Accept json
// @Produce json
// @Param template body PromptTemplateRequest true "模板信息"
// @Router /api/v1/prompt-templates [post]
func (h *PromptTemplateHandler) CreateTemplate(c *gin.Context) {
	var req PromptTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.BadRequestResponse(c, "INVALID_PARAM", err.Error())
		return
	}

	// 检查名称是否已存在
	templates, err := h.promptManager.ListTemplates()
	if err != nil {
		middleware.InternalErrorResponseWithCode(c, "LIST_FAILED", "检查模板名称失败")
		return
	}

	for _, tpl := range templates {
		if tpl.Name == req.Name {
			middleware.ConflictResponse(c, "ALREADY_EXISTS", "模板名称已存在")
			return
		}
	}

	// 添加描述注释（如果用户提供了描述）
	content := req.Content
	if req.Description != "" {
		content = "# " + req.Description + "\n" + content
	}

	// 保存模板
	if err := h.promptManager.SaveTemplate(req.Name, content); err != nil {
		middleware.InternalErrorResponseWithCode(c, "SAVE_FAILED", err.Error())
		return
	}

	c.JSON(201, gin.H{
		"success": true,
		"data": gin.H{
			"name": req.Name,
		},
		"message": "模板创建成功",
	})
}

// UpdateTemplate 更新Prompt模板
// @Summary 更新Prompt模板
// @Description 更新现有的Prompt模板
// @Tags PromptTemplates
// @Accept json
// @Produce json
// @Param name path string true "模板名称"
// @Param template body PromptTemplateRequest true "模板信息"
// @Router /api/v1/prompt-templates/{name} [put]
func (h *PromptTemplateHandler) UpdateTemplate(c *gin.Context) {
	name := c.Param("name")

	var req PromptTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.BadRequestResponse(c, "INVALID_PARAM", err.Error())
		return
	}

	// 添加描述注释（如果用户提供了描述）
	content := req.Content
	if req.Description != "" {
		content = "# " + req.Description + "\n" + content
	}

	// 保存模板（会覆盖已存在的文件）
	if err := h.promptManager.SaveTemplate(name, content); err != nil {
		middleware.InternalErrorResponseWithCode(c, "SAVE_FAILED", err.Error())
		return
	}

	middleware.SuccessResponseWithMessage(c, "模板更新成功", nil)
}

// DeleteTemplate 删除Prompt模板
// @Summary 删除Prompt模板
// @Description 删除指定的Prompt模板（默认模板不可删除）
// @Tags PromptTemplates
// @Produce json
// @Param name path string true "模板名称"
// @Router /api/v1/prompt-templates/{name} [delete]
func (h *PromptTemplateHandler) DeleteTemplate(c *gin.Context) {
	name := c.Param("name")

	if err := h.promptManager.DeleteTemplate(name); err != nil {
		if err.Error() == "不能删除默认模板" {
			middleware.ForbiddenResponse(c, "FORBIDDEN", "默认模板不可删除")
			return
		}

		middleware.InternalErrorResponseWithCode(c, "DELETE_FAILED", err.Error())
		return
	}

	middleware.SuccessResponseWithMessage(c, "模板删除成功", nil)
}

// ResetTemplate 重置为默认模板
// @Summary 重置Prompt模板为默认值
// @Description 将模板重置为内置的默认内容
// @Tags PromptTemplates
// @Produce json
// @Param name path string true "模板名称"
// @Router /api/v1/prompt-templates/{name}/reset [post]
func (h *PromptTemplateHandler) ResetTemplate(c *gin.Context) {
	name := c.Param("name")

	if err := h.promptManager.ResetToDefault(name); err != nil {
		middleware.InternalErrorResponseWithCode(c, "RESET_FAILED", err.Error())
		return
	}

	middleware.SuccessResponseWithMessage(c, "模板已重置为默认值", nil)
}
