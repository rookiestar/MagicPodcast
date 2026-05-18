package handlers

import (
	"fmt"
	"net/http"
	"time"

	"magicpodcast/internal/cache"
	"magicpodcast/internal/database"
	"magicpodcast/internal/handlers/dto"
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

// 类型别名，保持向后兼容
type WorkflowResponse = dto.WorkflowResponse
type WorkflowStats = dto.WorkflowStats
type JobResponse = dto.JobResponse
type JobExecutionResponse = dto.JobExecutionResponse

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

	cacheKey := fmt.Sprintf("%s:p%d:s%d:%s", cache.NewKeyBuilder().WorkflowList(), page, pageSize, sortBy)
	memCache := cache.GetCache()
	if cached, ok := memCache.Get(cacheKey); ok {
		cache.RecordHit()
		cachedResp := copyGinH(cached.(gin.H))
		cachedResp["cached"] = true
		setPrivateCache(c, 60)
		c.JSON(http.StatusOK, cachedResp)
		return
	}
	cache.RecordMiss()

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

	resp := gin.H{
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
	}
	memCache.SetWithTTL(cacheKey, resp, 2*time.Minute)
	setPrivateCache(c, 60)
	c.JSON(http.StatusOK, resp)
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
	cache.InvalidateWorkflowList()

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
	cache.InvalidateWorkflowDetail(workflow.ID)

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
	cache.InvalidateWorkflowDetail(workflow.ID)

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
	cache.InvalidateWorkflowDetail(workflow.ID)

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

	// 批量查询所有 Job 的 Report（优化 N+1 查询）
	jobIDs := make([]uint, len(jobs))
	for i, job := range jobs {
		jobIDs[i] = job.ID
	}
	reportMap := h.getBatchReports(jobIDs)

	// 转换为响应格式
	response := make([]JobResponse, len(jobs))
	for i, job := range jobs {
		response[i] = h.toJobResponseWithReport(&job, reportMap[job.ID])
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
			"llm_summary":      report.LLMSummary,
			"llm_model_used":   report.LLMModelUsed,
			"llm_tokens_used":  report.LLMTokensUsed,
			"llm_error":        report.LLMError,
		},
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
