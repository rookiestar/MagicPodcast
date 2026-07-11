package handlers

import (
	"fmt"

	"magicpodcast/internal/database"
	"magicpodcast/internal/handlers/dto"
	"magicpodcast/internal/models"

	"github.com/gin-gonic/gin"
)

// ========== 响应转换函数 ==========

// getBatchWorkflowStats 批量获取工作流统计信息（优化N+1查询）
func (h *WorkflowHandler) getBatchWorkflowStats(workflowIDs []uint) map[uint]*dto.BatchWorkflowStats {
	db := database.GetDB()
	result := make(map[uint]*dto.BatchWorkflowStats)

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
			result[sc.WorkflowID] = &dto.BatchWorkflowStats{}
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
			result[ae.WorkflowID] = &dto.BatchWorkflowStats{}
		}
		result[ae.WorkflowID].AvgEpisodes = ae.AvgEpisodes
	}

	// 初始化空的统计数据（用于没有job的workflow）
	for _, id := range workflowIDs {
		if result[id] == nil {
			result[id] = &dto.BatchWorkflowStats{}
		}
	}

	return result
}

// toWorkflowResponseWithStats 使用预加载统计数据转换为响应格式（优化N+1查询）
func (h *WorkflowHandler) toWorkflowResponseWithStats(workflow *models.Workflow, stats *dto.BatchWorkflowStats, subscribedPodcastCount int64) dto.WorkflowResponse {
	resp := dto.WorkflowResponse{
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
	workflowStats := dto.WorkflowStats{
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

func workflowListRulesConfigSummary(config models.RulesConfig) models.RulesConfig {
	return models.RulesConfig{
		TimeRange:     config.TimeRange,
		TimeRangeMode: config.TimeRangeMode,
	}
}

// toWorkflowResponse 转换为响应格式
func (h *WorkflowHandler) toWorkflowResponse(workflow *models.Workflow) dto.WorkflowResponse {
	resp := dto.WorkflowResponse{
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
	var stats dto.WorkflowStats

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
func (h *WorkflowHandler) toJobResponse(job *models.Job) dto.JobResponse {
	resp := dto.JobResponse{
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
		executions := make([]dto.JobExecutionResponse, len(job.Executions))
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
func (h *WorkflowHandler) toJobExecutionResponse(exec *models.JobExecution) dto.JobExecutionResponse {
	return dto.JobExecutionResponse{
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

func workflowReportResponse(report *models.Report) gin.H {
	return gin.H{
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
	}
}

// ========== 批量查询优化函数 ==========

// getBatchReports 批量获取 Job 的 Report（优化 N+1 查询）
func (h *WorkflowHandler) getBatchReports(jobIDs []uint, includeLongFields bool) map[uint]*models.Report {
	db := database.GetDB()
	result := make(map[uint]*models.Report)

	if len(jobIDs) == 0 {
		return result
	}

	var reports []models.Report
	query := db.Where("job_id IN ?", jobIDs)
	if !includeLongFields {
		query = query.Select("job_id", "llm_model_used", "llm_tokens_used")
	}
	if err := query.Find(&reports).Error; err != nil {
		return result
	}

	for i := range reports {
		result[reports[i].JobID] = &reports[i]
	}

	return result
}

// toJobResponseWithReport 使用预加载的 Report 转换为响应格式（优化 N+1 查询）
func (h *WorkflowHandler) toJobResponseWithReport(job *models.Job, report *models.Report) dto.JobResponse {
	resp := dto.JobResponse{
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
		executions := make([]dto.JobExecutionResponse, len(job.Executions))
		for i, exec := range job.Executions {
			executions[i] = h.toJobExecutionResponse(&exec)
		}
		resp.Executions = executions
	}

	// 使用预加载的 Report 数据
	if report != nil {
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

// ========== 验证函数 ==========

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

// ========== 排序辅助函数 ==========

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

// ========== LLM辅助函数 ==========

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
