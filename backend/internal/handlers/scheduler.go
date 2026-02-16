package handlers

import (
	"magicpodcast/internal/middleware"
	"magicpodcast/internal/scheduler"

	"github.com/gin-gonic/gin"
)

// SchedulerHandler 调度器HTTP处理器
type SchedulerHandler struct {
	scheduler *scheduler.Scheduler
}

// NewSchedulerHandler 创建调度器处理器
func NewSchedulerHandler(scheduler *scheduler.Scheduler) *SchedulerHandler {
	return &SchedulerHandler{
		scheduler: scheduler,
	}
}

// Reload 重新加载调度器
func (h *SchedulerHandler) Reload(c *gin.Context) {
	if err := h.scheduler.Reload(); err != nil {
		middleware.InternalErrorResponseWithCode(c, "SCHEDULER_RELOAD_FAILED", "重新加载调度器失败")
		return
	}

	middleware.SuccessResponseWithMessage(c, "调度器已重新加载", nil)
}

// GetStatus 获取调度器状态
func (h *SchedulerHandler) GetStatus(c *gin.Context) {
	status := h.scheduler.GetStatus()

	middleware.SuccessResponse(c, status)
}

// PauseWorkflow 暂停工作流调度
func (h *SchedulerHandler) PauseWorkflow(c *gin.Context) {
	workflowID, ok := ParseUintParam(c, "id")
	if !ok {
		return
	}

	if err := h.scheduler.PauseWorkflow(workflowID); err != nil {
		middleware.BadRequestResponse(c, "PAUSE_FAILED", err.Error())
		return
	}

	middleware.SuccessResponseWithMessage(c, "工作流调度已暂停", nil)
}

// ResumeWorkflow 恢复工作流调度
func (h *SchedulerHandler) ResumeWorkflow(c *gin.Context) {
	workflowID, ok := ParseUintParam(c, "id")
	if !ok {
		return
	}

	if err := h.scheduler.ResumeWorkflow(workflowID); err != nil {
		middleware.BadRequestResponse(c, "RESUME_FAILED", err.Error())
		return
	}

	middleware.SuccessResponseWithMessage(c, "工作流调度已恢复", nil)
}
