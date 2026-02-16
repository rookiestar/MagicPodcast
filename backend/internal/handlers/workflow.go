package handlers

import (
	"fmt"
	"net/http"
	"time"

	"magicpodcast/internal/database"
	"magicpodcast/internal/llm"
	"magicpodcast/internal/logger"
	"magicpodcast/internal/middleware"
	"magicpodcast/internal/models"
	"magicpodcast/internal/scheduler"
	"magicpodcast/internal/workflow"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// WorkflowHandler Workflow 处理器
type WorkflowHandler struct {
	executor   *workflow.Executor
	scheduler  *scheduler.Scheduler
	tracker    *workflow.ExecutionTracker
	summarizer workflow.SummarizerInterface
}

// NewWorkflowHandler 创建 Workflow 处理器
func NewWorkflowHandler(executor *workflow.Executor, scheduler *scheduler.Scheduler, summarizer workflow.SummarizerInterface) *WorkflowHandler {
	return &WorkflowHandler{
		executor:   executor,
		scheduler:  scheduler,
		tracker:    workflow.NewExecutionTracker(),
		summarizer: summarizer,
	}
}

// WorkflowResponse Workflow 响应结构
type WorkflowResponse struct {
	ID          uint                     `json:"id"`
	Name        string                   `json:"name"`
	Description string                   `json:"description"`
	Schedule    string                   `json:"schedule"`
	ScopeType   models.WorkflowScopeType `json:"scope_type"`
	ScopeConfig models.ScopeConfig       `json:"scope_config"`
	RulesConfig models.RulesConfig       `json:"rules_config"`
	IsEnabled   bool                     `json:"is_enabled"`
	CreatedAt   time.Time                `json:"created_at"`
	UpdatedAt   time.Time                `json:"updated_at"`
	LastJob     *JobResponse             `json:"last_job,omitempty"`
	Stats       *WorkflowStats           `json:"stats,omitempty"`
}

// WorkflowStats 工作流统计信息
type WorkflowStats struct {
	TotalJobs     int64      `json:"total_jobs"`
	SuccessJobs   int64      `json:"success_jobs"`
	FailedJobs    int64      `json:"failed_jobs"`
	TotalEpisodes float64    `json:"total_episodes"` // 平均每次执行匹配的单集数
	PodcastCount  int64      `json:"podcast_count"`
	LastExecution *time.Time `json:"last_execution,omitempty"`
	NextExecution *time.Time `json:"next_execution,omitempty"`
}

// JobResponse Job 响应结构
type JobResponse struct {
	ID                uint                   `json:"id"`
	WorkflowID        uint                   `json:"workflow_id"`
	Status            models.JobStatus       `json:"status"`
	StartTime         *time.Time             `json:"start_time,omitempty"`
	EndTime           *time.Time             `json:"end_time,omitempty"`
	PodcastsProcessed int                    `json:"podcasts_processed"`
	EpisodesFound     int                    `json:"episodes_found"`
	EpisodesCreated   int                    `json:"episodes_created"`
	EpisodesMatched   int                    `json:"episodes_matched"`
	ErrorCount        int                    `json:"error_count"`
	TriggeredBy       string                 `json:"triggered_by"`
	CreatedAt         time.Time              `json:"created_at"`
	Duration          *int64                 `json:"duration,omitempty"` // 执行时长（毫秒）
	Executions        []JobExecutionResponse `json:"executions,omitempty"`

	// LLM相关字段
	LLMSummary     *string `json:"llm_summary,omitempty"`      // LLM生成的摘要
	LLMModelUsed   *string `json:"llm_model_used,omitempty"`   // 使用的模型名称
	LLMTokensUsed  *int    `json:"llm_tokens_used,omitempty"`  // 使用的token数量
	LLMError       *string `json:"llm_error,omitempty"`        // LLM错误信息
}

// JobExecutionResponse JobExecution 响应结构
type JobExecutionResponse struct {
	ID              uint                   `json:"id"`
	JobID           uint                   `json:"job_id"`
	PodcastID       *uint                  `json:"podcast_id,omitempty"`
	PodcastTitle    string                 `json:"podcast_title,omitempty"`
	PodcastFeedURL  string                 `json:"podcast_feed_url,omitempty"`
	Status          models.ExecutionStatus `json:"status"`
	EpisodesFound   int                    `json:"episodes_found"`
	EpisodesCreated int                    `json:"episodes_created"`
	EpisodesMatched int                    `json:"episodes_matched"`
	ErrorMessage    string                 `json:"error_message,omitempty"`
	LogInfo         string                 `json:"log_info,omitempty"`
	ProcessingTime  int                    `json:"processing_time"` // 毫秒
	CreatedAt       time.Time              `json:"created_at"`
}

// List 获取工作流列表
// @Summary 获取工作流列表
// @Description 获取所有工作流，支持排序和分页
// @Tags Workflows
// @Accept json
// @Produce json
// @Param sort_by query string false "排序方式: updated(最近更新，默认), execution(下次执行时间)"
// @Param page query int false "页码（默认1）"
// @Param page_size query int false "每页数量（默认20）"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/workflows [get]
func (h *WorkflowHandler) List(c *gin.Context) {
	db := database.GetDB()

	// 分页参数（使用辅助函数）
	pagination := ParsePaginationParams(c, 20)
	page := pagination.Page
	pageSize := pagination.PageSize

	// 获取排序参数（默认：updated）
	sortBy := c.DefaultQuery("sort_by", "updated")

	// 查询总数
	var total int64
	db.Model(&models.Workflow{}).Count(&total)

	// 查询工作流列表（一次性查询所有数据，避免N+1问题）
	var workflows []models.Workflow
	offset := (page - 1) * pageSize

	// 调试日志
	logger.Infof("[Workflow] 查询工作流列表: page=%d, pageSize=%d, sortBy=%s, offset=%d", page, pageSize, sortBy, offset)

	// 分步查询以避免N+1问题
	// 1. 查询workflows
	orderClause := h.buildSortOrderClause(sortBy)
	if err := db.Order(orderClause).Limit(pageSize).Offset(offset).Find(&workflows).Error; err != nil {
		logger.Infof("[Workflow] 查询失败: %v", err)
		middleware.InternalErrorResponseWithCode(c, "INTERNAL_ERROR", "Failed to fetch workflows")
		return
	}

	// 2. 收集所有需要查询的Job IDs
	var jobIDs []uint
	for _, wf := range workflows {
		if wf.LastJobID != nil {
			jobIDs = append(jobIDs, *wf.LastJobID)
		}
	}

	// 3. 一次性查询所有需要的Jobs（从N次查询减少到1次查询）
	var jobs []models.Job
	if len(jobIDs) > 0 {
		if err := db.Where("id IN ?", jobIDs).Find(&jobs).Error; err != nil {
			logger.Infof("[Workflow] 查询Jobs失败: %v", err)
			// 不中断流程，继续返回workflows
		}
	}

	// 4. 建立Job ID到Job的映射
	jobMap := make(map[uint]*models.Job)
	for i := range jobs {
		jobMap[jobs[i].ID] = &jobs[i]
	}

	// 5. 为每个workflow设置LastJob
	for i := range workflows {
		if workflows[i].LastJobID != nil {
			if job, ok := jobMap[*workflows[i].LastJobID]; ok {
				workflows[i].LastJob = job
			}
		}
	}

	// 6. 批量查询所有workflow的统计数据（优化N+1查询）
	workflowIDs := make([]uint, len(workflows))
	for i, wf := range workflows {
		workflowIDs[i] = wf.ID
	}
	statsMap := h.getBatchWorkflowStats(workflowIDs)

	// 获取订阅节目数（用于ScopeTypeAllSubscribed）
	var subscribedPodcastCount int64
	db.Model(&models.Podcast{}).Where("is_subscribed = ?", true).Count(&subscribedPodcastCount)

	// 转换为响应格式
	response := make([]WorkflowResponse, len(workflows))
	for i, wf := range workflows {
		stats := statsMap[wf.ID]
		response[i] = h.toWorkflowResponseWithStats(&wf, stats, subscribedPodcastCount)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"workflows": response,
			"pagination": gin.H{
				"page":        page,
				"page_size":   pageSize,
				"total":       total,
				"total_pages": (total + int64(pageSize) - 1) / int64(pageSize),
			},
		},
	})
}

// buildSortOrderClause 根据排序参数构建ORDER BY子句
func (h *WorkflowHandler) buildSortOrderClause(sortBy string) string {
	switch sortBy {
	case "execution":
		// 按下一次执行时间升序（即将执行的在前），NULL值（无下次执行计划）排在最后
		// 使用 datetime() 函数将所有时区的时间转换为 UTC 进行排序，避免时区不一致导致的排序错误
		return "next_run_at IS NOT NULL, datetime(next_run_at) ASC"
	case "updated":
		// 按更新时间倒序（最近更新的在前）
		return "updated_at DESC"
	default:
		// 默认按更新时间倒序
		return "updated_at DESC"
	}
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
		middleware.NotFoundResponse(c, "NOT_FOUND", "Workflow not found")
		return
	}

	// 手动加载LastJob
	if workflow.LastJobID != nil {
		var job models.Job
		if err := db.Where("id = ?", *workflow.LastJobID).First(&job).Error; err == nil {
			workflow.LastJob = &job
		}
	}

	middleware.SuccessResponse(c, h.toWorkflowResponse(&workflow))
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
		middleware.BadRequestResponse(c, "INVALID_PARAM", err.Error())
		return
	}

	// 验证cron表达式
	if err := models.ValidateCron(req.Schedule); err != nil {
		middleware.BadRequestResponse(c, "INVALID_CRON", err.Error())
		return
	}

	// 验证范围配置
	if err := validateScopeConfig(req.ScopeType, req.ScopeConfig); err != nil {
		middleware.BadRequestResponse(c, "INVALID_SCOPE", err.Error())
		return
	}

	// 验证规则配置（包括LLM参数）
	logger.Infof("[Create] Received LLM config: enabled=%v, max_episodes=%d, model=%s",
		req.RulesConfig.LLMEnabled,
		req.RulesConfig.LLMMaxEpisodes,
		req.RulesConfig.LLMModel)

	if err := validateRulesConfig(req.RulesConfig); err != nil {
		middleware.BadRequestResponse(c, "INVALID_RULES", err.Error())
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

	// 计算并设置下次执行时间
	if workflow.IsEnabled && workflow.Schedule != "" {
		nextRun, err := workflow.GetNextRunTime()
		if err == nil {
			workflow.NextRunAt = &nextRun
			logger.Infof("📅 新建工作流下次执行时间 [ID=%d]: %s", workflow.ID, nextRun.Format("2006-01-02 15:04:05"))
		} else {
			logger.Infof("⚠️  计算下次执行时间失败 [ID=%d]: %v", workflow.ID, err)
		}
	}

	if err := db.Omit("LastJob", "Jobs").Create(&workflow).Error; err != nil {
		middleware.InternalErrorResponseWithCode(c, "INTERNAL_ERROR", "Failed to create workflow")
		return
	}

	// 如果工作流启用且配置了 schedule,注册到调度器
	if workflow.IsEnabled && workflow.Schedule != "" {
		if err := h.scheduler.AddWorkflow(&workflow); err != nil {
			logger.Infof("⚠️  注册工作流到调度器失败 [ID=%d]: %v", workflow.ID, err)
			// 不返回错误,因为工作流已经创建成功,只是调度注册失败
		} else {
			logger.Infof("✅ 工作流已注册到调度器 [ID=%d, Schedule=%s]", workflow.ID, workflow.Schedule)
		}
	}

	middleware.CreatedResponse(c, h.toWorkflowResponse(&workflow))
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
		middleware.NotFoundResponse(c, "NOT_FOUND", "Workflow not found")
		return
	}

	var req WorkflowRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.BadRequestResponse(c, "INVALID_PARAM", err.Error())
		return
	}

	// 验证cron表达式
	if err := models.ValidateCron(req.Schedule); err != nil {
		middleware.BadRequestResponse(c, "INVALID_CRON", err.Error())
		return
	}

	// 验证范围配置
	logger.Infof("[Update] Workflow ID=%d, scope_type=%s, scope_config=%+v",
		workflow.ID, req.ScopeType, req.ScopeConfig)

	if req.ScopeType == "specific_podcasts" {
		oldCount := len(workflow.ScopeConfig.PodcastIDs)
		newCount := len(req.ScopeConfig.PodcastIDs)
		logger.Infof("[Update] specific_podcasts: 旧=%d个播客, 新=%d个播客", oldCount, newCount)
		if newCount == 0 {
			logger.Infof("⚠️  [Update] 警告: podcast_ids为空，将覆盖原有数据")
		} else if newCount < oldCount/2 {
			logger.Infof("⚠️  [Update] 警告: podcast_ids大幅减少 (%d -> %d)，可能是数据丢失", oldCount, newCount)
		}
	}

	if err := validateScopeConfig(req.ScopeType, req.ScopeConfig); err != nil {
		middleware.BadRequestResponse(c, "INVALID_SCOPE", err.Error())
		return
	}

	// 验证规则配置（包括LLM参数）
	logger.Infof("[Update] Received LLM config: enabled=%v, max_episodes=%d, model=%s",
		req.RulesConfig.LLMEnabled,
		req.RulesConfig.LLMMaxEpisodes,
		req.RulesConfig.LLMModel)

	if err := validateRulesConfig(req.RulesConfig); err != nil {
		middleware.BadRequestResponse(c, "INVALID_RULES", err.Error())
		return
	}

	workflow.Name = req.Name
	workflow.Description = req.Description
	workflow.Schedule = req.Schedule
	workflow.ScopeType = req.ScopeType
	workflow.ScopeConfig = req.ScopeConfig

	// 打印调试信息
	logger.Infof("[Update] Original RulesConfig from DB: %+v", workflow.RulesConfig)
	logger.Infof("[Update] New RulesConfig from request: %+v", req.RulesConfig)

	workflow.RulesConfig = req.RulesConfig

	logger.Infof("[Update] Workflow RulesConfig after assignment: %+v", workflow.RulesConfig)

	workflow.IsEnabled = req.IsEnabled

	// 如果工作流启用且配置了schedule，计算并更新下次执行时间
	if workflow.IsEnabled && workflow.Schedule != "" {
		nextRun, err := workflow.GetNextRunTime()
		if err == nil {
			workflow.NextRunAt = &nextRun
			logger.Infof("📅 更新工作流下次执行时间 [ID=%d]: %s", workflow.ID, nextRun.Format("2006-01-02 15:04:05"))
		} else {
			logger.Infof("⚠️  计算下次执行时间失败 [ID=%d]: %v", workflow.ID, err)
		}
	}

	// 使用Updates而不是Save，明确指定要更新的字段
	updates := map[string]interface{}{
		"name":         workflow.Name,
		"description":  workflow.Description,
		"schedule":     workflow.Schedule,
		"scope_type":   workflow.ScopeType,
		"scope_config": workflow.ScopeConfig,
		"rules_config": workflow.RulesConfig,
		"is_enabled":   workflow.IsEnabled,
	}

	if workflow.NextRunAt != nil {
		updates["next_run_at"] = workflow.NextRunAt
	}

	if err := db.Model(&models.Workflow{}).Where("id = ?", workflow.ID).Updates(updates).Error; err != nil {
		middleware.InternalErrorResponseWithCode(c, "INTERNAL_ERROR", "Failed to update workflow")
		return
	}

	// 重新加载调度器以应用更新
	if err := h.scheduler.Reload(); err != nil {
		logger.Infof("⚠️  重新加载调度器失败 [ID=%d]: %v", workflow.ID, err)
		// 不返回错误，因为工作流已经更新成功
	}

	middleware.SuccessResponse(c, h.toWorkflowResponse(&workflow))
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
		middleware.NotFoundResponse(c, "NOT_FOUND", "Workflow not found")
		return
	}

	if err := db.Delete(&workflow).Error; err != nil {
		middleware.InternalErrorResponseWithCode(c, "INTERNAL_ERROR", "Failed to delete workflow")
		return
	}

	middleware.SuccessResponse(c, gin.H{"message": "Workflow deleted successfully"})
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
		middleware.NotFoundResponse(c, "NOT_FOUND", "Workflow not found")
		return
	}

	workflow.IsEnabled = !workflow.IsEnabled

	// 如果启用工作流且配置了schedule，计算并更新下次执行时间
	if workflow.IsEnabled && workflow.Schedule != "" {
		nextRun, err := workflow.GetNextRunTime()
		if err == nil {
			workflow.NextRunAt = &nextRun
			logger.Infof("📅 更新工作流下次执行时间 [ID=%d]: %s", workflow.ID, nextRun.Format("2006-01-02 15:04:05"))
		} else {
			logger.Infof("⚠️  计算下次执行时间失败 [ID=%d]: %v", workflow.ID, err)
		}
	}

	if err := db.Save(&workflow).Error; err != nil {
		middleware.InternalErrorResponseWithCode(c, "INTERNAL_ERROR", "Failed to toggle workflow")
		return
	}

	// 重新加载调度器以应用更新
	if err := h.scheduler.Reload(); err != nil {
		logger.Infof("⚠️  重新加载调度器失败 [ID=%d]: %v", workflow.ID, err)
		// 不返回错误，因为工作流已经更新成功
	}

	middleware.SuccessResponse(c, h.toWorkflowResponse(&workflow))
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
		middleware.NotFoundResponse(c, "NOT_FOUND", "Workflow not found")
		return
	}

	// 分页参数（使用辅助函数）
	pagination := ParsePaginationParams(c, 20)
	page := pagination.Page
	pageSize := pagination.PageSize

	// 查询总数
	var total int64
	db.Model(&models.Job{}).Where("workflow_id = ?", id).Count(&total)

	// 查询任务列表
	var jobs []models.Job
	offset := (page - 1) * pageSize
	if err := db.Where("workflow_id = ?", id).Order("created_at DESC").Limit(pageSize).Offset(offset).Find(&jobs).Error; err != nil {
		middleware.InternalErrorResponseWithCode(c, "INTERNAL_ERROR", "Failed to fetch jobs")
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
				"page":        page,
				"page_size":   pageSize,
				"total":       total,
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
		middleware.NotFoundResponse(c, "NOT_FOUND", "Job not found")
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

	middleware.SuccessResponse(c, response)
}

// GetJobReport 获取Job的报告
// @Summary 获取Job报告
// @Description 获取指定Job的执行报告（Markdown格式）
// @Tags Workflows
// @Accept json
// @Produce json
// @Param id path int true "Job ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/jobs/{id}/report [get]
func (h *WorkflowHandler) GetJobReport(c *gin.Context) {
	db := database.GetDB()
	id := c.Param("id")

	// 查找报告
	var report models.Report
	if err := db.Where("job_id = ?", id).First(&report).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			middleware.NotFoundResponse(c, "REPORT_NOT_FOUND", "报告不存在或尚未生成")
			return
		}

		middleware.InternalErrorResponseWithCode(c, "INTERNAL_ERROR", "Failed to fetch report")
		return
	}

	// 返回报告
	middleware.SuccessResponse(c, gin.H{
		"id":               report.ID,
		"job_id":           report.JobID,
		"title":            report.Title,
		"content":          report.Content,
		"summary":          report.Summary,
		"episodes_count":   report.EpisodesCount,
		"podcasts_count":   report.PodcastsCount,
		"matched_count":    report.MatchedCount,
		"time_range_start": report.TimeRangeStart,
		"time_range_end":   report.TimeRangeEnd,
		"time_range_mode":  report.TimeRangeMode,
		"generated_at":     report.GeneratedAt,
		"format":           report.Format,
		"file_size":        report.FileSize,
		// LLM相关字段
		"llm_summary":     report.LLMSummary,
		"llm_model_used":  report.LLMModelUsed,
		"llm_tokens_used": report.LLMTokensUsed,
		"llm_error":       report.LLMError,
	})
}

// RegenerateLLMSummary 重新生成LLM摘要
// @Summary 重新生成LLM摘要
// @Description 为报告重新生成AI智能摘要
// @Tags Workflows
// @Accept json
// @Produce json
// @Param id path int true "Job ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/jobs/{id}/regenerate-llm [post]
func (h *WorkflowHandler) RegenerateLLMSummary(c *gin.Context) {
	db := database.GetDB()
	jobID := c.Param("id")

	// 检查summarizer是否可用
	if h.summarizer == nil {
		middleware.ServiceUnavailableResponse(c, "LLM_NOT_CONFIGURED", "LLM服务未配置，请先配置LLM相关设置")
		return
	}

	// 1. 获取Job
	var job models.Job
	if err := db.Preload("Workflow").First(&job, jobID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			middleware.NotFoundResponse(c, "JOB_NOT_FOUND", "任务不存在")
			return
		}
		middleware.InternalErrorResponseWithCode(c, "INTERNAL_ERROR", "Failed to fetch job")
		return
	}

	// 2. 获取Report
	var report models.Report
	if err := db.Where("job_id = ?", jobID).First(&report).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			middleware.NotFoundResponse(c, "REPORT_NOT_FOUND", "报告不存在，请先执行工作流生成报告")
			return
		}
		middleware.InternalErrorResponseWithCode(c, "INTERNAL_ERROR", "Failed to fetch report")
		return
	}

	// 3. 获取JobExecutions以重建EpisodeReportData
	var executions []models.JobExecution
	if err := db.Where("job_id = ?", jobID).Find(&executions).Error; err != nil {
		logger.Errorf("Failed to fetch job executions: %v", err)
		middleware.InternalErrorResponseWithCode(c, "INTERNAL_ERROR", "Failed to fetch job executions")
		return
	}

	// 4. 构建EpisodeReportData（从executions重建）
	reportData := make([]workflow.EpisodeReportData, 0, len(executions))
	for _, exec := range executions {
		// 只处理成功的execution
		if exec.Status != models.ExecutionStatusSuccess {
			continue
		}

		// 获取该execution的episodes
		var episodes []models.Episode
		if err := db.Where("podcast_id = ? AND published_date >= ? AND published_date <= ?",
			exec.PodcastID,
			report.TimeRangeStart,
			report.TimeRangeEnd,
		).Find(&episodes).Error; err != nil {
			logger.Errorf("Failed to fetch episodes for podcast %d: %v", exec.PodcastID, err)
			continue
		}

		// 转换为workflow.EpisodeDetail格式
		episodeDetails := make([]workflow.EpisodeDetail, 0, len(episodes))
		for _, ep := range episodes {
			detail := workflow.EpisodeDetail{
				Title:         ep.Title,
				ShowNotes:     ep.ShowNotes,
				PublishedDate: ep.PublishedDate,
				UpdatedDate:   &ep.UpdatedAt,
				EpisodeNo:     ep.EpisodeNo,
				Link:          ep.Link,
				XYZID:         "", // Episode模型没有XYZID字段
				QRCode:        "", // 不需要二维码信息
				QRCodeError:   false,
			}
			episodeDetails = append(episodeDetails, detail)
		}

		reportData = append(reportData, workflow.EpisodeReportData{
			PodcastID:      *exec.PodcastID,
			PodcastTitle:   exec.PodcastTitle,
			PodcastFeedURL: exec.PodcastFeedURL,
			Episodes:       episodeDetails,
		})
	}

	if len(reportData) == 0 {
		middleware.BadRequestResponse(c, "NO_DATA", "没有可用于生成摘要的单集数据")
		return
	}

	// 5. 调用summarizer重新生成摘要
	workflowConfig := job.Workflow
	options := llm.SummaryOptions{
		MaxEpisodes: workflowConfig.RulesConfig.LLMMaxEpisodes,
		Temperature: workflowConfig.RulesConfig.LLMTemperature,
		MaxTokens:   workflowConfig.RulesConfig.LLMMaxTokens,
		Model:       workflowConfig.RulesConfig.LLMModel,
	}

	// 转换 []workflow.EpisodeReportData 到 []llm.EpisodeReportData
	llmReportData := make([]llm.EpisodeReportData, len(reportData))
	for i, d := range reportData {
		llmEpisodeDetails := make([]llm.EpisodeDetail, len(d.Episodes))
		for j, ep := range d.Episodes {
			llmEpisodeDetails[j] = llm.EpisodeDetail{
				Title:         ep.Title,
				ShowNotes:     ep.ShowNotes,
				PublishedDate: ep.PublishedDate,
				UpdatedDate:   ep.UpdatedDate,
				EpisodeNo:     ep.EpisodeNo,
				Link:          ep.Link,
				XYZID:         ep.XYZID,
				QRCode:        ep.QRCode,
				QRCodeError:   ep.QRCodeError,
			}
		}
		llmReportData[i] = llm.EpisodeReportData{
			PodcastID:      d.PodcastID,
			PodcastTitle:   d.PodcastTitle,
			PodcastFeedURL: d.PodcastFeedURL,
			Episodes:       llmEpisodeDetails,
		}
	}

	result, err := h.summarizer.GenerateForReport(
		llmReportData,
		workflowConfig.Name,
		workflowConfig.RulesConfig.LLMUserPrompt,
		options,
	)

	if err != nil {
		logger.Errorf("Failed to regenerate LLM summary [JobID=%s]: %v", jobID, err)

		// 更新报告的错误信息
		report.LLMError = err.Error()
		if err := db.Save(&report).Error; err != nil {
			logger.Errorf("Failed to update report error: %v", err)
		}

		middleware.InternalErrorResponseWithCode(c, "LLM_ERROR", fmt.Sprintf("LLM摘要生成失败: %v", err))
		return
	}

	// 6. 更新报告的LLM相关字段
	report.LLMSummary = result.Summary
	report.LLMModelUsed = result.ModelUsed
	report.LLMTokensUsed = result.TokensUsed
	report.LLMError = ""

	// 重新插入LLM摘要到markdown
	newContent := insertLLMSummary(report.Content, result.Summary)
	report.Content = newContent

	if err := db.Save(&report).Error; err != nil {
		logger.Errorf("Failed to update report: %v", err)
		middleware.InternalErrorResponseWithCode(c, "INTERNAL_ERROR", "Failed to update report")
		return
	}

	logger.Infof("✅ LLM摘要重新生成成功 [JobID=%s, Tokens=%d]", jobID, result.TokensUsed)

	// 7. 返回更新后的报告
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "LLM摘要重新生成成功",
		"data": gin.H{
			"id":               report.ID,
			"job_id":           report.JobID,
			"title":            report.Title,
			"content":          report.Content,
			"summary":          report.Summary,
			"episodes_count":   report.EpisodesCount,
			"podcasts_count":   report.PodcastsCount,
			"matched_count":    report.MatchedCount,
			"time_range_start": report.TimeRangeStart,
			"time_range_end":   report.TimeRangeEnd,
			"time_range_mode":  report.TimeRangeMode,
			"generated_at":     report.GeneratedAt,
			"format":           report.Format,
			"file_size":        report.FileSize,
			"llm_summary":     report.LLMSummary,
			"llm_model_used":  report.LLMModelUsed,
			"llm_tokens_used": report.LLMTokensUsed,
			"llm_error":       report.LLMError,
		},
	})
}

// insertLLMSummary 将LLM摘要插入到标题之后、元数据卡片之前
func insertLLMSummary(markdown, llmSummary string) string {
	lines := make([]string, 0)

	for i, line := range splitLines(markdown) {
		if i == 0 {
			// 第一行是标题，在标题后插入AI摘要
			lines = append(lines, line)
			lines = append(lines, "")
			lines = append(lines, "## 🤖 AI智能摘要")
			lines = append(lines, "")
			lines = append(lines, llmSummary)
			lines = append(lines, "")
			lines = append(lines, "---")
			lines = append(lines, "")
		} else {
			lines = append(lines, line)
		}
	}

	return joinLines(lines)
}

// splitLines 辅助函数：分割行
func splitLines(s string) []string {
	lines := make([]string, 0)
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

// joinLines 辅助函数：连接行
func joinLines(lines []string) string {
	result := ""
	for i, line := range lines {
		if i > 0 {
			result += "\n"
		}
		result += line
	}
	return result
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
		logger.Infof("🔍 [DEBUG Handler] Failed to load workflow %s: %v", id, err)
		middleware.NotFoundResponse(c, "NOT_FOUND", "工作流不存在")
		return
	}

	// 检查工作流是否启用
	if !workflow.IsEnabled {
		middleware.BadRequestResponse(c, "WORKFLOW_DISABLED", "工作流未启用，请先启用后再执行")
		return
	}

	// 使用ExecutionTracker检查是否已有任务在运行
	// 超时时间设置为30分钟
	ctx, started := h.tracker.TryStart(workflow.ID, 30*time.Minute)
	if !started {
		logger.Infof("⚠️  工作流已在执行中 [WorkflowID=%d]", workflow.ID)
		middleware.ConflictResponse(c, "JOB_RUNNING", "该工作流正在执行中，请等待当前任务完成")
		return
	}

	// 异步执行工作流（避免阻塞HTTP请求）
	go func() {
		// 确保执行完成后清理跟踪器
		defer h.tracker.Complete(workflow.ID)

		job, err := h.executor.Execute(ctx, &workflow, "manual")
		if err != nil {
			logger.Infof("❌ 工作流执行失败 [WorkflowID=%d]: %v", workflow.ID, err)
		} else {
			logger.Infof("✅ 工作流执行完成 [WorkflowID=%d, JobID=%d]", workflow.ID, job.ID)
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
	Name        string                   `json:"name" binding:"required,min=1,max=200"`
	Description string                   `json:"description"`
	Schedule    string                   `json:"schedule" binding:"required"`
	ScopeType   models.WorkflowScopeType `json:"scope_type" binding:"required"`
	ScopeConfig models.ScopeConfig       `json:"scope_config"`
	RulesConfig models.RulesConfig       `json:"rules_config"`
	IsEnabled   bool                     `json:"is_enabled"`
}

// BatchWorkflowStats 批量查询的工作流统计信息
type BatchWorkflowStats struct {
	TotalJobs     int64
	SuccessJobs   int64
	FailedJobs    int64
	AvgEpisodes   float64
	LastExecution *time.Time
	NextExecution *time.Time
}

// getBatchWorkflowStats 批量获取工作流统计信息（优化N+1查询）
func (h *WorkflowHandler) getBatchWorkflowStats(workflowIDs []uint) map[uint]*BatchWorkflowStats {
	db := database.GetDB()
	result := make(map[uint]*BatchWorkflowStats)

	if len(workflowIDs) == 0 {
		return result
	}

	// 单次查询获取所有工作流的任务统计
	type JobStatusCount struct {
		WorkflowID uint
		Status     models.JobStatus
		Count      int64
	}

	var statusCounts []JobStatusCount
	db.Model(&models.Job{}).
		Select("workflow_id, status, count(*) as count").
		Where("workflow_id IN ?", workflowIDs).
		Group("workflow_id, status").
		Find(&statusCounts)

	// 聚合统计数据
	for _, sc := range statusCounts {
		if result[sc.WorkflowID] == nil {
			result[sc.WorkflowID] = &BatchWorkflowStats{}
		}
		result[sc.WorkflowID].TotalJobs += sc.Count
		if sc.Status == models.JobStatusCompleted {
			result[sc.WorkflowID].SuccessJobs = sc.Count
		}
		if sc.Status == models.JobStatusFailed {
			result[sc.WorkflowID].FailedJobs = sc.Count
		}
	}

	// 查询平均单集数
	type AvgEpisodes struct {
		WorkflowID  uint
		AvgEpisodes float64
	}
	var avgEpisodes []AvgEpisodes
	db.Model(&models.Job{}).
		Select("workflow_id, COALESCE(CAST(AVG(episodes_matched) AS FLOAT), 0) as avg_episodes").
		Where("workflow_id IN ? AND status = ?", workflowIDs, models.JobStatusCompleted).
		Group("workflow_id").
		Find(&avgEpisodes)

	for _, ae := range avgEpisodes {
		if result[ae.WorkflowID] == nil {
			result[ae.WorkflowID] = &BatchWorkflowStats{}
		}
		result[ae.WorkflowID].AvgEpisodes = ae.AvgEpisodes
	}

	// 初始化空的统计数据（用于没有job的workflow）
	for _, id := range workflowIDs {
		if result[id] == nil {
			result[id] = &BatchWorkflowStats{}
		}
	}

	return result
}

// toWorkflowResponseWithStats 使用预加载统计数据转换为响应格式（优化N+1查询）
func (h *WorkflowHandler) toWorkflowResponseWithStats(workflow *models.Workflow, stats *BatchWorkflowStats, subscribedPodcastCount int64) WorkflowResponse {
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

	// 使用预加载的统计数据
	workflowStats := WorkflowStats{
		TotalJobs:     stats.TotalJobs,
		SuccessJobs:   stats.SuccessJobs,
		FailedJobs:    stats.FailedJobs,
		TotalEpisodes: stats.AvgEpisodes,
		LastExecution: workflow.LastExecutionAt,
		NextExecution: workflow.NextRunAt,
	}

	// 计算关联的节目数
	switch workflow.ScopeType {
	case models.ScopeTypeSpecificPodcasts:
		workflowStats.PodcastCount = int64(len(workflow.ScopeConfig.PodcastIDs))
	case models.ScopeTypeAllSubscribed:
		workflowStats.PodcastCount = subscribedPodcastCount
	case models.ScopeTypeCustomSources:
		workflowStats.PodcastCount = int64(len(workflow.ScopeConfig.CustomURLs))
	}

	resp.Stats = &workflowStats

	return resp
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

	// 统计平均每次执行匹配的单集数（只统计成功完成的执行，包括匹配到0个的情况）
	db.Model(&models.Job{}).Where("workflow_id = ? AND status = ?", workflow.ID, models.JobStatusCompleted).Select("COALESCE(CAST(AVG(episodes_matched) AS FLOAT), 0)").Scan(&stats.TotalEpisodes)

	// 计算关联的节目数
	switch workflow.ScopeType {
	case models.ScopeTypeSpecificPodcasts:
		stats.PodcastCount = int64(len(workflow.ScopeConfig.PodcastIDs))
	case models.ScopeTypeAllSubscribed:
		// 统计已订阅的节目数
		db.Model(&models.Podcast{}).Where("is_subscribed = ?", true).Count(&stats.PodcastCount)
	case models.ScopeTypeCustomSources:
		stats.PodcastCount = int64(len(workflow.ScopeConfig.CustomURLs))
	}

	// 获取最后一次执行时间和下次执行时间（使用持久化字段）
	stats.LastExecution = workflow.LastExecutionAt
	stats.NextExecution = workflow.NextRunAt

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
		EpisodesMatched:   job.EpisodesMatched,
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

	// 添加LLM统计信息（从关联的Report中获取）
	db := database.GetDB()
	var report models.Report
	if err := db.Where("job_id = ?", job.ID).First(&report).Error; err == nil {
		// 找到报告，添加LLM字段
		if report.LLMSummary != "" {
			resp.LLMSummary = &report.LLMSummary
		}
		if report.LLMModelUsed != "" {
			resp.LLMModelUsed = &report.LLMModelUsed
		}
		if report.LLMTokensUsed > 0 {
			resp.LLMTokensUsed = &report.LLMTokensUsed
		}
		if report.LLMError != "" {
			resp.LLMError = &report.LLMError
		}
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
		EpisodesMatched: exec.EpisodesMatched,
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

// validateRulesConfig 验证规则配置（包括LLM参数）
func validateRulesConfig(config models.RulesConfig) error {
	// 如果启用LLM，验证相关参数
	if config.LLMEnabled {
		// 验证temperature范围
		if config.LLMTemperature < 0 || config.LLMTemperature > 1.0 {
			return fmt.Errorf("llm_temperature必须在0.0-1.0之间")
		}

		// 验证max_tokens
		if config.LLMMaxTokens < 100 || config.LLMMaxTokens > 4000 {
			return fmt.Errorf("llm_max_tokens必须在100-4000之间")
		}

		// 验证max_episodes
		if config.LLMMaxEpisodes < 1 || config.LLMMaxEpisodes > 100 {
			return fmt.Errorf("llm_max_episodes必须在1-100之间")
		}
	}

	return nil
}
