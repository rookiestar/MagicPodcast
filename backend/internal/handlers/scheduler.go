package handlers

import (
	"net/http"
	"strconv"

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
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "SCHEDULER_RELOAD_FAILED",
				"message": "重新加载调度器失败",
			},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "调度器已重新加载",
	})
}

// GetStatus 获取调度器状态
func (h *SchedulerHandler) GetStatus(c *gin.Context) {
	status := h.scheduler.GetStatus()

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    status,
	})
}

// PauseWorkflow 暂停工作流调度
func (h *SchedulerHandler) PauseWorkflow(c *gin.Context) {
	idStr := c.Param("id")
	workflowID, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "INVALID_WORKFLOW_ID",
				"message": "无效的工作流ID",
			},
		})
		return
	}

	if err := h.scheduler.PauseWorkflow(uint(workflowID)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "PAUSE_FAILED",
				"message": err.Error(),
			},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "工作流调度已暂停",
	})
}

// ResumeWorkflow 恢复工作流调度
func (h *SchedulerHandler) ResumeWorkflow(c *gin.Context) {
	idStr := c.Param("id")
	workflowID, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "INVALID_WORKFLOW_ID",
				"message": "无效的工作流ID",
			},
		})
		return
	}

	if err := h.scheduler.ResumeWorkflow(uint(workflowID)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "RESUME_FAILED",
				"message": err.Error(),
			},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "工作流调度已恢复",
	})
}
