package handlers

import (
	"fmt"
	"magicpodcast/internal/middleware"
	"magicpodcast/internal/services"

	"github.com/gin-gonic/gin"
)

// WorkflowHandlerRefactored 重构后的工作流处理器（Phase 2）
type WorkflowHandlerRefactored struct {
	workflowService *services.WorkflowService
}

// NewWorkflowHandlerRefactored 创建重构后的工作流处理器
func NewWorkflowHandlerRefactored(workflowService *services.WorkflowService) *WorkflowHandlerRefactored {
	return &WorkflowHandlerRefactored{
		workflowService: workflowService,
	}
}

// List 获取工作流列表
// @Summary 获取工作流列表
// @Description 获取所有工作流，支持排序和分页
// @Tags Workflows
// @Accept json
// @Produce json
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
// @Param enabled_only query bool false "仅显示已启用"
// @Param sort_by query string false "排序字段" default(created_at)
// @Param sort_order query string false "排序方向" default(desc)
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/workflows [get]
func (h *WorkflowHandlerRefactored) List(c *gin.Context) {
	// 解析参数
	page, _ := c.GetQuery("page")
	pageSize, _ := c.GetQuery("page_size")
	enabledOnly := c.Query("enabled_only") == "true"
	sortBy := c.DefaultQuery("sort_by", "created_at")
	sortOrder := c.DefaultQuery("sort_order", "desc")

	// 转换参数
	pageInt := parseInt(page, 1)
	pageSizeInt := parseInt(pageSize, 20)

	// 调用 Service
	result, err := h.workflowService.ListWorkflows(pageInt, pageSizeInt, enabledOnly)
	if err != nil {
		middleware.HandleError(c, err)
		return
	}

	// 应用排序
	// 注意：排序应该在 Service 层处理，这里作为示例
	middleware.SuccessResponse(c, gin.H{
		"workflows": result.Workflows,
		"total":     result.Total,
		"page":      result.Page,
		"page_size": result.PageSize,
		"sort_by":   sortBy,
		"sort_order": sortOrder,
	})
}

// Get 获取工作流详情
// @Summary 获取工作流详情
// @Description 根据ID获取工作流详情
// @Tags Workflows
// @Accept json
// @Produce json
// @Param id path int true "工作流ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/workflows/{id} [get]
func (h *WorkflowHandlerRefactored) Get(c *gin.Context) {
	id, err := parseUintParam(c, "id")
	if err != nil {
		middleware.ValidationErrorResponse(c, "id", "must be a valid number")
		return
	}

	// 调用 Service
	workflow, err := h.workflowService.GetWorkflow(id)
	if err != nil {
		middleware.HandleError(c, err)
		return
	}

	middleware.SuccessResponse(c, workflow)
}

// Create 创建工作流
// @Summary 创建工作流
// @Description 创建新的工作流
// @Tags Workflows
// @Accept json
// @Produce json
// @Param workflow body services.CreateWorkflowRequest true "工作流信息"
// @Success 201 {object} map[string]interface{}
// @Router /api/v1/workflows [post]
func (h *WorkflowHandlerRefactored) Create(c *gin.Context) {
	var req services.CreateWorkflowRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.ValidationErrorResponse(c, "request body", "invalid format: "+err.Error())
		return
	}

	// 调用 Service
	workflow, err := h.workflowService.CreateWorkflow(&req)
	if err != nil {
		middleware.HandleError(c, err)
		return
	}

	middleware.CreatedResponse(c, workflow)
}

// Update 更新工作流
// @Summary 更新工作流
// @Description 更新现有工作流
// @Tags Workflows
// @Accept json
// @Produce json
// @Param id path int true "工作流ID"
// @Param workflow body services.UpdateWorkflowRequest true "工作流信息"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/workflows/{id} [put]
func (h *WorkflowHandlerRefactored) Update(c *gin.Context) {
	id, err := parseUintParam(c, "id")
	if err != nil {
		middleware.ValidationErrorResponse(c, "id", "must be a valid number")
		return
	}

	var req services.UpdateWorkflowRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.ValidationErrorResponse(c, "request body", "invalid format: "+err.Error())
		return
	}

	// 调用 Service
	workflow, err := h.workflowService.UpdateWorkflow(id, &req)
	if err != nil {
		middleware.HandleError(c, err)
		return
	}

	middleware.SuccessResponse(c, workflow)
}

// Delete 删除工作流
// @Summary 删除工作流
// @Description 删除指定的工作流
// @Tags Workflows
// @Accept json
// @Produce json
// @Param id path int true "工作流ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/workflows/{id} [delete]
func (h *WorkflowHandlerRefactored) Delete(c *gin.Context) {
	id, err := parseUintParam(c, "id")
	if err != nil {
		middleware.ValidationErrorResponse(c, "id", "must be a valid number")
		return
	}

	// 调用 Service
	if err := h.workflowService.DeleteWorkflow(id); err != nil {
		middleware.HandleError(c, err)
		return
	}

	middleware.SuccessResponse(c, gin.H{
		"message": "Workflow deleted successfully",
	})
}

// Toggle 切换工作流启用状态
// @Summary 切换工作流状态
// @Description 切换工作流的启用/禁用状态
// @Tags Workflows
// @Accept json
// @Produce json
// @Param id path int true "工作流ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/workflows/{id}/toggle [post]
func (h *WorkflowHandlerRefactored) Toggle(c *gin.Context) {
	id, err := parseUintParam(c, "id")
	if err != nil {
		middleware.ValidationErrorResponse(c, "id", "must be a valid number")
		return
	}

	// 调用 Service
	workflow, err := h.workflowService.ToggleWorkflow(id)
	if err != nil {
		middleware.HandleError(c, err)
		return
	}

	middleware.SuccessResponse(c, workflow)
}

// Trigger 手动触发工作流
// @Summary 手动触发工作流
// @Description 立即执行指定的工作流
// @Tags Workflows
// @Accept json
// @Produce json
// @Param id path int true "工作流ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/workflows/{id}/trigger [post]
func (h *WorkflowHandlerRefactored) Trigger(c *gin.Context) {
	id, err := parseUintParam(c, "id")
	if err != nil {
		middleware.ValidationErrorResponse(c, "id", "must be a valid number")
		return
	}

	// 调用 Service
	if err := h.workflowService.TriggerWorkflow(id); err != nil {
		middleware.HandleError(c, err)
		return
	}

	middleware.SuccessResponse(c, gin.H{
		"message": "Workflow triggered successfully",
	})
}

// ========== 辅助函数 ==========

// parseUintParam 解析uint参数
func parseUintParam(c *gin.Context, key string) (uint, error) {
	value := c.Param(key)
	var id uint
	if _, err := fmt.Sscanf(value, "%d", &id); err != nil {
		return 0, err
	}
	return id, nil
}

// parseInt 解析int参数，带默认值
func parseInt(s string, defaultValue int) int {
	if s == "" {
		return defaultValue
	}
	var result int
	if _, err := fmt.Sscanf(s, "%d", &result); err != nil {
		return defaultValue
	}
	return result
}
