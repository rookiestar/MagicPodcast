package handlers

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"magicpodcast/internal/database"
	"magicpodcast/internal/models"
	"magicpodcast/internal/scheduler"
	"magicpodcast/internal/workflow"

	"github.com/gin-gonic/gin"
)

// WorkflowHandler Workflow 处理器
type WorkflowHandler struct {
	executor  *workflow.Executor
	scheduler *scheduler.Scheduler
}

// NewWorkflowHandler 创建 Workflow 处理器
func NewWorkflowHandler(executor *workflow.Executor, scheduler *scheduler.Scheduler) *WorkflowHandler {
	return &WorkflowHandler{
		executor:  executor,
		scheduler: scheduler,
	}
}

// WorkflowResponse Workflow 响应结构
type WorkflowResponse struct {
	ID          uint                  `json:"id"`
	Name        string                `json:"name"`
	Description string                `json:"description"`
	Schedule    string                `json:"schedule"`
	ScopeType   models.WorkflowScopeType `json:"scope_type"`
	ScopeConfig models.ScopeConfig    `json:"scope_config"`
	RulesConfig models.RulesConfig    `json:"rules_config"`
	IsEnabled   bool                  `json:"is_enabled"`
	CreatedAt   time.Time             `json:"created_at"`
	UpdatedAt   time.Time             `json:"updated_at"`
	LastJob     *JobResponse          `json:"last_job,omitempty"`
	Stats       *WorkflowStats        `json:"stats,omitempty"`
}

// WorkflowStats 工作流统计信息
type WorkflowStats struct {
	TotalJobs       int64       `json:"total_jobs"`
	SuccessJobs     int64       `json:"success_jobs"`
	FailedJobs      int64       `json:"failed_jobs"`
	SuccessRate     float64     `json:"success_rate"`
	TotalEpisodes   int64       `json:"total_episodes"`
	LastExecution   *time.Time  `json:"last_execution,omitempty"`
	NextExecution   *time.Time  `json:"next_execution,omitempty"`
}

// JobResponse Job 响应结构
type JobResponse struct {
	ID                uint              `json:"id"`
	WorkflowID        uint              `json:"workflow_id"`
	Status            models.JobStatus  `json:"status"`
	StartTime         *time.Time        `json:"start_time,omitempty"`
	EndTime           *time.Time        `json:"end_time,omitempty"`
	PodcastsProcessed int               `json:"podcasts_processed"`
	EpisodesFound     int               `json:"episodes_found"`
	EpisodesCreated   int               `json:"episodes_created"`
	ErrorCount        int               `json:"error_count"`
	TriggeredBy       string            `json:"triggered_by"`
	CreatedAt         time.Time         `json:"created_at"`
	Duration          *int64            `json:"duration,omitempty"` // 执行时长（毫秒）
	Executions        []JobExecutionResponse `json:"executions,omitempty"`
}

// JobExecutionResponse JobExecution 响应结构
type JobExecutionResponse struct {
	ID              uint                      `json:"id"`
	JobID           uint                      `json:"job_id"`
	PodcastID       *uint                     `json:"podcast_id,omitempty"`
	PodcastTitle    string                    `json:"podcast_title,omitempty"`
	PodcastFeedURL  string                    `json:"podcast_feed_url,omitempty"`
	Status          models.ExecutionStatus    `json:"status"`
	EpisodesFound   int                       `json:"episodes_found"`
	EpisodesCreated int                       `json:"episodes_created"`
	ErrorMessage    string                    `json:"error_message,omitempty"`
	LogInfo         string                    `json:"log_info,omitempty"`
	ProcessingTime  int                       `json:"processing_time"` // 毫秒
	CreatedAt       time.Time                 `json:"created_at"`
}

// List 获取工作流列表
// @Summary 获取工作流列表
// @Description 获取所有工作流，支持分页
// @Tags Workflows
// @Accept json
// @Produce json
// @Param page query int false "页码（默认1）"
// @Param page_size query int false "每页数量（默认20）"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/workflows [get]
func (h *WorkflowHandler) List(c *gin.Context) {
	db := database.GetDB()

	// 分页参数
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	// 查询总数
	var total int64
	db.Model(&models.Workflow{}).Count(&total)

	// 查询工作流列表（不使用Preload，手动加载LastJob）
	var workflows []models.Workflow
	offset := (page - 1) * pageSize
	query := db.Order("created_at DESC").Limit(pageSize).Offset(offset)

	// 调试日志
	log.Printf("[Workflow] 查询工作流列表: page=%d, pageSize=%d, offset=%d", page, pageSize, offset)

	if err := query.Find(&workflows).Error; err != nil {
		log.Printf("[Workflow] 查询失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "INTERNAL_ERROR",
				"message": "Failed to fetch workflows",
			},
		})
		return
	}

	// 手动加载每个工作流的LastJob
	for i := range workflows {
		if workflows[i].LastJobID != nil {
			var job models.Job
			if err := db.Where("id = ?", *workflows[i].LastJobID).First(&job).Error; err == nil {
				workflows[i].LastJob = &job
			}
		}
	}

	// 转换为响应格式
	response := make([]WorkflowResponse, len(workflows))
	for i, wf := range workflows {
		response[i] = h.toWorkflowResponse(&wf)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"workflows": response,
			"pagination": gin.H{
				"page":       page,
				"page_size":  pageSize,
				"total":      total,
				"total_pages": (total + int64(pageSize) - 1) / int64(pageSize),
			},
		},
	})
}

// Get 获取单个工作流详情
// @Summary 获取工作流详情
// @Description 根据ID获取工作流详情
// @Tags Workflows
// @Accept json
// @Produce json
// @Param id path int true "工作流ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/workflows/{id} [get]
func (h *WorkflowHandler) Get(c *gin.Context) {
	db := database.GetDB()
	id := c.Param("id")

	var workflow models.Workflow
	if err := db.First(&workflow, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "NOT_FOUND",
				"message": "Workflow not found",
			},
		})
		return
	}

	// 手动加载LastJob
	if workflow.LastJobID != nil {
		var job models.Job
		if err := db.Where("id = ?", *workflow.LastJobID).First(&job).Error; err == nil {
			workflow.LastJob = &job
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    h.toWorkflowResponse(&workflow),
	})
}

// Create 创建工作流
// @Summary 创建工作流
// @Description 创建新的工作流
// @Tags Workflows
// @Accept json
// @Produce json
// @Param workflow body WorkflowRequest true "工作流信息"
// @Success 201 {object} map[string]interface{}
// @Router /api/v1/workflows [post]
func (h *WorkflowHandler) Create(c *gin.Context) {
	db := database.GetDB()

	var req WorkflowRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "INVALID_PARAM",
				"message": err.Error(),
			},
		})
		return
	}

	// 验证cron表达式
	if err := models.ValidateCron(req.Schedule); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "INVALID_CRON",
				"message": err.Error(),
			},
		})
		return
	}

	// 验证范围配置
	if err := validateScopeConfig(req.ScopeType, req.ScopeConfig); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "INVALID_SCOPE",
				"message": err.Error(),
			},
		})
		return
	}

	workflow := models.Workflow{
		Name:        req.Name,
		Description: req.Description,
		Schedule:    req.Schedule,
		ScopeType:   req.ScopeType,
		ScopeConfig: req.ScopeConfig,
		RulesConfig: req.RulesConfig,
		IsEnabled:   req.IsEnabled,
	}

	// 使用 Omit 避免 GORM 的 RETURNING 问题，并手动设置时间
	now := time.Now()
	workflow.CreatedAt = now
	workflow.UpdatedAt = now

	if err := db.Omit("LastJob", "Jobs").Create(&workflow).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "INTERNAL_ERROR",
				"message": "Failed to create workflow",
			},
		})
		return
	}

	// 如果工作流启用且配置了 schedule,注册到调度器
	if workflow.IsEnabled && workflow.Schedule != "" {
		if err := h.scheduler.AddWorkflow(&workflow); err != nil {
			log.Printf("⚠️  注册工作流到调度器失败 [ID=%d]: %v", workflow.ID, err)
			// 不返回错误,因为工作流已经创建成功,只是调度注册失败
		} else {
			log.Printf("✅ 工作流已注册到调度器 [ID=%d, Schedule=%s]", workflow.ID, workflow.Schedule)
		}
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data":    h.toWorkflowResponse(&workflow),
	})
}

// Update 更新工作流
// @Summary 更新工作流
// @Description 更新工作流信息
// @Tags Workflows
// @Accept json
// @Produce json
// @Param id path int true "工作流ID"
// @Param workflow body WorkflowRequest true "工作流信息"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/workflows/{id} [put]
func (h *WorkflowHandler) Update(c *gin.Context) {
	db := database.GetDB()
	id := c.Param("id")

	var workflow models.Workflow
	if err := db.First(&workflow, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "NOT_FOUND",
				"message": "Workflow not found",
			},
		})
		return
	}

	var req WorkflowRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "INVALID_PARAM",
				"message": err.Error(),
			},
		})
		return
	}

	// 验证cron表达式
	if err := models.ValidateCron(req.Schedule); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "INVALID_CRON",
				"message": err.Error(),
			},
		})
		return
	}

	// 验证范围配置
	if err := validateScopeConfig(req.ScopeType, req.ScopeConfig); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "INVALID_SCOPE",
				"message": err.Error(),
			},
		})
		return
	}

	workflow.Name = req.Name
	workflow.Description = req.Description
	workflow.Schedule = req.Schedule
	workflow.ScopeType = req.ScopeType
	workflow.ScopeConfig = req.ScopeConfig
	workflow.RulesConfig = req.RulesConfig
	workflow.IsEnabled = req.IsEnabled

	if err := db.Save(&workflow).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "INTERNAL_ERROR",
				"message": "Failed to update workflow",
			},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    h.toWorkflowResponse(&workflow),
	})
}

// Delete 删除工作流
// @Summary 删除工作流
// @Description 删除工作流（软删除）
// @Tags Workflows
// @Accept json
// @Produce json
// @Param id path int true "工作流ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/workflows/{id} [delete]
func (h *WorkflowHandler) Delete(c *gin.Context) {
	db := database.GetDB()
	id := c.Param("id")

	var workflow models.Workflow
	if err := db.First(&workflow, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "NOT_FOUND",
				"message": "Workflow not found",
			},
		})
		return
	}

	if err := db.Delete(&workflow).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "INTERNAL_ERROR",
				"message": "Failed to delete workflow",
			},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    gin.H{
			"message": "Workflow deleted successfully",
		},
	})
}

// Toggle 启用/禁用工作流
// @Summary 启用/禁用工作流
// @Description 切换工作流的启用状态
// @Tags Workflows
// @Accept json
// @Produce json
// @Param id path int true "工作流ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/workflows/{id}/toggle [post]
func (h *WorkflowHandler) Toggle(c *gin.Context) {
	db := database.GetDB()
	id := c.Param("id")

	var workflow models.Workflow
	if err := db.First(&workflow, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "NOT_FOUND",
				"message": "Workflow not found",
			},
		})
		return
	}

	workflow.IsEnabled = !workflow.IsEnabled
	if err := db.Save(&workflow).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "INTERNAL_ERROR",
				"message": "Failed to toggle workflow",
			},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    h.toWorkflowResponse(&workflow),
	})
}

// ListJobs 获取工作流的执行历史
// @Summary 获取工作流执行历史
// @Description 获取指定工作流的所有执行记录
// @Tags Workflows
// @Accept json
// @Produce json
// @Param id path int true "工作流ID"
// @Param page query int false "页码（默认1）"
// @Param page_size query int false "每页数量（默认20）"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/workflows/{id}/jobs [get]
func (h *WorkflowHandler) ListJobs(c *gin.Context) {
	db := database.GetDB()
	id := c.Param("id")

	// 验证工作流是否存在
	var workflow models.Workflow
	if err := db.First(&workflow, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "NOT_FOUND",
				"message": "Workflow not found",
			},
		})
		return
	}

	// 分页参数
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	// 查询总数
	var total int64
	db.Model(&models.Job{}).Where("workflow_id = ?", id).Count(&total)

	// 查询任务列表
	var jobs []models.Job
	offset := (page - 1) * pageSize
	if err := db.Where("workflow_id = ?", id).Order("created_at DESC").Limit(pageSize).Offset(offset).Find(&jobs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "INTERNAL_ERROR",
				"message": "Failed to fetch jobs",
			},
		})
		return
	}

	// 转换为响应格式
	response := make([]JobResponse, len(jobs))
	for i, job := range jobs {
		response[i] = h.toJobResponse(&job)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"jobs": response,
			"pagination": gin.H{
				"page":       page,
				"page_size":  pageSize,
				"total":      total,
				"total_pages": (total + int64(pageSize) - 1) / int64(pageSize),
			},
		},
	})
}

// GetJob 获取单个任务详情
// @Summary 获取任务详情
// @Description 根据ID获取任务详情
// @Tags Workflows
// @Accept json
// @Produce json
// @Param id path int true "任务ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/jobs/{id} [get]
func (h *WorkflowHandler) GetJob(c *gin.Context) {
	db := database.GetDB()
	id := c.Param("id")

	var job models.Job
	if err := db.Preload("Executions").First(&job, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "NOT_FOUND",
				"message": "Job not found",
			},
		})
		return
	}

	response := h.toJobResponse(&job)

	// 添加执行详情
	if len(job.Executions) > 0 {
		executions := make([]JobExecutionResponse, len(job.Executions))
		for i, exec := range job.Executions {
			executions[i] = h.toJobExecutionResponse(&exec)
		}
		response.Executions = executions
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
	})
}

// Trigger 手动触发工作流
// @Summary 手动触发工作流
// @Description 立即执行工作流
// @Tags Workflows
// @Accept json
// @Produce json
// @Param id path int true "工作流ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/workflows/{id}/trigger [post]
func (h *WorkflowHandler) Trigger(c *gin.Context) {
	db := database.GetDB()
	id := c.Param("id")

	var workflow models.Workflow
	if err := db.First(&workflow, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "NOT_FOUND",
				"message": "工作流不存在",
			},
		})
		return
	}

	// 检查工作流是否启用
	if !workflow.IsEnabled {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "WORKFLOW_DISABLED",
				"message": "工作流未启用，请先启用后再执行",
			},
		})
		return
	}

	// 检查是否有正在运行的Job
	if workflow.LastJobID != nil {
		var lastJob models.Job
		if err := db.Where("id = ?", *workflow.LastJobID).First(&lastJob).Error; err == nil {
			if lastJob.Status == models.JobStatusRunning {
				c.JSON(http.StatusConflict, gin.H{
					"success": false,
					"error": gin.H{
						"code":    "JOB_RUNNING",
						"message": "该工作流正在执行中，请等待当前任务完成",
					},
				})
				return
			}
		}
	}

	// 异步执行工作流（避免阻塞HTTP请求）
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()

		job, err := h.executor.Execute(ctx, &workflow, "manual")
		if err != nil {
			log.Printf("❌ 工作流执行失败 [WorkflowID=%d]: %v", workflow.ID, err)
		} else {
			log.Printf("✅ 工作流执行完成 [WorkflowID=%d, JobID=%d]", workflow.ID, job.ID)
		}
	}()

	// 立即返回202 Accepted，告知用户已开始执行
	c.JSON(http.StatusAccepted, gin.H{
		"success": true,
		"message": "工作流已开始执行，请在执行历史中查看进度",
		"data": gin.H{
			"workflow_id":   workflow.ID,
			"workflow_name": workflow.Name,
			"triggered_by":  "manual",
			"hint":          "使用 GET /api/v1/workflows/:id/jobs 查看执行进度",
		},
	})
}

// WorkflowRequest 创建/更新工作流请求结构
type WorkflowRequest struct {
	Name        string                    `json:"name" binding:"required,min=1,max=200"`
	Description string                    `json:"description"`
	Schedule    string                    `json:"schedule" binding:"required"`
	ScopeType   models.WorkflowScopeType  `json:"scope_type" binding:"required"`
	ScopeConfig models.ScopeConfig        `json:"scope_config"`
	RulesConfig models.RulesConfig        `json:"rules_config"`
	IsEnabled   bool                      `json:"is_enabled"`
}

// toWorkflowResponse 转换为响应格式
func (h *WorkflowHandler) toWorkflowResponse(workflow *models.Workflow) WorkflowResponse {
	resp := WorkflowResponse{
		ID:          workflow.ID,
		Name:        workflow.Name,
		Description: workflow.Description,
		Schedule:    workflow.Schedule,
		ScopeType:   workflow.ScopeType,
		ScopeConfig: workflow.ScopeConfig,
		RulesConfig: workflow.RulesConfig,
		IsEnabled:   workflow.IsEnabled,
		CreatedAt:   workflow.CreatedAt,
		UpdatedAt:   workflow.UpdatedAt,
	}

	// 添加最后一次执行任务信息
	if workflow.LastJob != nil {
		jobResp := h.toJobResponse(workflow.LastJob)
		resp.LastJob = &jobResp
	}

	// 添加统计信息
	db := database.GetDB()
	var stats WorkflowStats

	// 统计任务总数
	db.Model(&models.Job{}).Where("workflow_id = ?", workflow.ID).Count(&stats.TotalJobs)

	// 统计成功和失败的任务数
	db.Model(&models.Job{}).Where("workflow_id = ? AND status = ?", workflow.ID, models.JobStatusCompleted).Count(&stats.SuccessJobs)
	db.Model(&models.Job{}).Where("workflow_id = ? AND status = ?", workflow.ID, models.JobStatusFailed).Count(&stats.FailedJobs)

	// 计算成功率
	if stats.TotalJobs > 0 {
		stats.SuccessRate = float64(stats.SuccessJobs) / float64(stats.TotalJobs) * 100
	}

	// 统计总共创建的单集数
	db.Model(&models.Job{}).Where("workflow_id = ?", workflow.ID).Select("COALESCE(SUM(episodes_created), 0)").Scan(&stats.TotalEpisodes)

	// 获取最后一次执行时间
	var lastJob models.Job
	if err := db.Where("workflow_id = ?", workflow.ID).Order("created_at DESC").First(&lastJob).Error; err == nil {
		stats.LastExecution = &lastJob.CreatedAt
	}

	// 获取下次执行时间（如果工作流已启用且有scheduler）
	if workflow.IsEnabled && workflow.Schedule != "" && h.scheduler != nil {
		if nextRun, err := h.scheduler.GetWorkflowNextRunTime(workflow.ID); err == nil {
			stats.NextExecution = &nextRun
		}
	}

	resp.Stats = &stats

	return resp
}

// toJobResponse 转换为响应格式
func (h *WorkflowHandler) toJobResponse(job *models.Job) JobResponse {
	resp := JobResponse{
		ID:                job.ID,
		WorkflowID:        job.WorkflowID,
		Status:            job.Status,
		StartTime:         job.StartTime,
		EndTime:           job.EndTime,
		PodcastsProcessed: job.PodcastsProcessed,
		EpisodesFound:     job.EpisodesFound,
		EpisodesCreated:   job.EpisodesCreated,
		ErrorCount:        job.ErrorCount,
		TriggeredBy:       job.TriggeredBy,
		CreatedAt:         job.CreatedAt,
	}

	// 计算执行时长
	if job.StartTime != nil && job.EndTime != nil {
		duration := job.EndTime.Sub(*job.StartTime).Milliseconds()
		resp.Duration = &duration
	}

	// 添加执行详情
	if len(job.Executions) > 0 {
		executions := make([]JobExecutionResponse, len(job.Executions))
		for i, exec := range job.Executions {
			executions[i] = h.toJobExecutionResponse(&exec)
		}
		resp.Executions = executions
	}

	return resp
}

// toJobExecutionResponse 转换为响应格式
func (h *WorkflowHandler) toJobExecutionResponse(exec *models.JobExecution) JobExecutionResponse {
	return JobExecutionResponse{
		ID:              exec.ID,
		JobID:           exec.JobID,
		PodcastID:       exec.PodcastID,
		PodcastTitle:    exec.PodcastTitle,
		PodcastFeedURL:  exec.PodcastFeedURL,
		Status:          exec.Status,
		EpisodesFound:   exec.EpisodesFound,
		EpisodesCreated: exec.EpisodesCreated,
		ErrorMessage:    exec.ErrorMessage,
		LogInfo:         exec.LogInfo,
		ProcessingTime:  exec.ProcessingTime,
		CreatedAt:       exec.CreatedAt,
	}
}

// validateScopeConfig 验证范围配置
func validateScopeConfig(scopeType models.WorkflowScopeType, config models.ScopeConfig) error {
	switch scopeType {
	case models.ScopeTypeSpecificPodcasts:
		if len(config.PodcastIDs) == 0 {
			return fmt.Errorf("podcast_ids is required when scope_type is specific_podcasts")
		}
	case models.ScopeTypeCustomSources:
		if len(config.CustomURLs) == 0 {
			return fmt.Errorf("custom_urls is required when scope_type is custom_sources")
		}
	case models.ScopeTypeAllSubscribed:
		// 不需要额外验证
	}
	return nil
}
