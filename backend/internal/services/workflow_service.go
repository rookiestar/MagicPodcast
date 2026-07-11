package services

import (
	apperrors "magicpodcast/internal/errors"
	"magicpodcast/internal/models"
	"magicpodcast/internal/repository"

	"gorm.io/gorm"
)

// WorkflowService 工作流服务层
type WorkflowService struct {
	repos *repository.Repositories
}

// NewWorkflowService 创建工作流服务
func NewWorkflowService(repos *repository.Repositories) *WorkflowService {
	return &WorkflowService{
		repos: repos,
	}
}

// ========== 请求和响应DTO ==========

// CreateWorkflowRequest 创建工作流请求（匹配实际模型）
type CreateWorkflowRequest struct {
	Name        string                   `json:"name" binding:"required"`
	Description string                   `json:"description"`
	Schedule    string                   `json:"schedule" binding:"required"` // cron表达式
	ScopeType   models.WorkflowScopeType `json:"scope_type" binding:"required"`
	ScopeConfig models.ScopeConfig       `json:"scope_config"`
	RulesConfig models.RulesConfig       `json:"rules_config"`
	IsEnabled   bool                     `json:"is_enabled"`
}

// UpdateWorkflowRequest 更新工作流请求
type UpdateWorkflowRequest struct {
	Name        *string                   `json:"name"`
	Description *string                   `json:"description"`
	Schedule    *string                   `json:"schedule"`
	ScopeType   *models.WorkflowScopeType `json:"scope_type"`
	ScopeConfig *models.ScopeConfig       `json:"scope_config"`
	RulesConfig *models.RulesConfig       `json:"rules_config"`
	IsEnabled   *bool                     `json:"is_enabled"`
}

// WorkflowResponse 工作流响应（匹配实际模型）
type WorkflowResponse struct {
	ID            uint                     `json:"id"`
	Name          string                   `json:"name"`
	Description   string                   `json:"description"`
	Schedule      string                   `json:"schedule"`
	ScopeType     models.WorkflowScopeType `json:"scope_type"`
	ScopeConfig   models.ScopeConfig       `json:"scope_config"`
	RulesConfig   models.RulesConfig       `json:"rules_config"`
	IsEnabled     bool                     `json:"is_enabled"`
	CreatedAt     string                   `json:"created_at"`
	UpdatedAt     string                   `json:"updated_at"`
	LastExecution *string                  `json:"last_execution_at,omitempty"`
	NextExecution *string                  `json:"next_run_at,omitempty"`
	JobCount      int                      `json:"job_count"`
}

// WorkflowListResponse 工作流列表响应
type WorkflowListResponse struct {
	Workflows []WorkflowResponse `json:"workflows"`
	Total     int64              `json:"total"`
	Page      int                `json:"page"`
	PageSize  int                `json:"page_size"`
}

// ========== CRUD 操作 ==========

// CreateWorkflow 创建工作流
func (s *WorkflowService) CreateWorkflow(req *CreateWorkflowRequest) (*WorkflowResponse, error) {
	// 验证配置
	if err := s.validateWorkflowConfig(req.Schedule, &req.ScopeConfig, &req.RulesConfig); err != nil {
		return nil, err
	}

	// 创建工作流
	workflow := &models.Workflow{
		Name:        req.Name,
		Description: req.Description,
		Schedule:    req.Schedule,
		ScopeType:   req.ScopeType,
		ScopeConfig: req.ScopeConfig,
		RulesConfig: req.RulesConfig,
		IsEnabled:   req.IsEnabled,
	}

	if err := s.repos.Workflow.Create(workflow); err != nil {
		return nil, apperrors.InternalErrorWithErr(err, "Failed to create workflow")
	}

	return s.toWorkflowResponse(workflow), nil
}

// GetWorkflow 获取工作流详情
func (s *WorkflowService) GetWorkflow(id uint) (*WorkflowResponse, error) {
	workflow, err := s.repos.Workflow.GetByID(id)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.NotFoundError("workflow", id)
		}
		return nil, apperrors.InternalErrorWithErr(err, "Failed to fetch workflow")
	}

	return s.toWorkflowResponse(workflow), nil
}

// UpdateWorkflow 更新工作流
func (s *WorkflowService) UpdateWorkflow(id uint, req *UpdateWorkflowRequest) (*WorkflowResponse, error) {
	// 获取现有工作流
	workflow, err := s.repos.Workflow.GetByID(id)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.NotFoundError("workflow", id)
		}
		return nil, apperrors.InternalErrorWithErr(err, "Failed to fetch workflow")
	}

	// 更新字段
	if req.Name != nil {
		workflow.Name = *req.Name
	}
	if req.Description != nil {
		workflow.Description = *req.Description
	}
	if req.IsEnabled != nil {
		workflow.IsEnabled = *req.IsEnabled
	}
	if req.Schedule != nil {
		// 验证cron表达式
		if err := models.ValidateCron(*req.Schedule); err != nil {
			return nil, apperrors.InvalidCronExpressionError(*req.Schedule)
		}
		workflow.Schedule = *req.Schedule
	}
	if req.ScopeType != nil {
		workflow.ScopeType = *req.ScopeType
	}
	if req.ScopeConfig != nil {
		// 验证范围配置
		if err := s.validateScopeConfig(req.ScopeConfig); err != nil {
			return nil, err
		}
		workflow.ScopeConfig = *req.ScopeConfig
	}
	if req.RulesConfig != nil {
		workflow.RulesConfig = *req.RulesConfig
	}

	// 执行更新
	if err := s.repos.Workflow.Update(workflow); err != nil {
		return nil, apperrors.InternalErrorWithErr(err, "Failed to update workflow")
	}

	return s.toWorkflowResponse(workflow), nil
}

// DeleteWorkflow 删除工作流（使用事务确保原子性）
func (s *WorkflowService) DeleteWorkflow(id uint) error {
	// 检查是否存在
	_, err := s.repos.Workflow.GetByID(id)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return apperrors.NotFoundError("workflow", id)
		}
		return apperrors.InternalErrorWithErr(err, "Failed to fetch workflow")
	}

	// 使用事务删除工作流及其关联数据，确保原子性
	err = s.repos.DB().Transaction(func(tx *gorm.DB) error {
		// 删除相关的 Job（级联删除 JobExecutions 和 Reports 由外键约束处理）
		if err := tx.Where("workflow_id = ?", id).Delete(&models.Job{}).Error; err != nil {
			return err
		}

		// 删除工作流
		if err := tx.Delete(&models.Workflow{}, id).Error; err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return apperrors.InternalErrorWithErr(err, "Failed to delete workflow")
	}

	return nil
}

// ListWorkflows 获取工作流列表
func (s *WorkflowService) ListWorkflows(page, pageSize int, enabledOnly bool) (*WorkflowListResponse, error) {
	var workflows []*models.Workflow
	var total int64
	var err error

	if enabledOnly {
		workflows, err = s.repos.Workflow.ListEnabled()
		total = int64(len(workflows))
	} else {
		workflows, total, err = s.repos.Workflow.List(page, pageSize)
	}

	if err != nil {
		return nil, apperrors.InternalErrorWithErr(err, "Failed to fetch workflows")
	}

	// 转换为响应格式
	responses := make([]WorkflowResponse, len(workflows))
	for i, wf := range workflows {
		responses[i] = *s.toWorkflowResponse(wf)
	}

	return &WorkflowListResponse{
		Workflows: responses,
		Total:     total,
		Page:      page,
		PageSize:  pageSize,
	}, nil
}

// ToggleWorkflow 切换工作流启用状态
func (s *WorkflowService) ToggleWorkflow(id uint) (*WorkflowResponse, error) {
	// 使用 Repository 的 ToggleStatus 方法
	toggled, err := s.repos.Workflow.ToggleStatus(id)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.NotFoundError("workflow", id)
		}
		return nil, apperrors.InternalErrorWithErr(err, "Failed to toggle workflow")
	}

	return s.toWorkflowResponse(toggled), nil
}

// ========== 辅助方法 ==========

// validateWorkflowConfig 验证工作流配置
func (s *WorkflowService) validateWorkflowConfig(
	schedule string,
	scope *models.ScopeConfig,
	rules *models.RulesConfig,
) error {
	if err := s.validateScheduleConfig(schedule); err != nil {
		return err
	}

	if err := s.validateScopeConfig(scope); err != nil {
		return err
	}

	if err := s.validateRulesConfig(rules); err != nil {
		return err
	}

	return nil
}

// validateScheduleConfig 验证调度配置
func (s *WorkflowService) validateScheduleConfig(schedule string) error {
	if schedule == "" {
		return apperrors.InvalidWorkflowConfigError("schedule is required")
	}

	// 验证cron表达式
	if err := models.ValidateCron(schedule); err != nil {
		return apperrors.InvalidCronExpressionError(schedule)
	}

	return nil
}

// validateScopeConfig 验证范围配置
func (s *WorkflowService) validateScopeConfig(config *models.ScopeConfig) error {
	if config == nil {
		return apperrors.InvalidWorkflowConfigError("scope_config is required")
	}

	// 注意：实际验证需要结合 ScopeType
	// 如果是 ScopeTypeSpecificPodcasts，需要 podcast_ids 或 custom_urls
	// 如果是 ScopeTypeAllSubscribed 或 ScopeTypeCustomSources，可以为空
	// 这里我们简化处理，只要配置对象存在就通过

	return nil
}

// validateRulesConfig 验证规则配置
func (s *WorkflowService) validateRulesConfig(config *models.RulesConfig) error {
	if config == nil {
		return apperrors.InvalidWorkflowConfigError("rules_config is required")
	}

	// 验证时间范围模式
	if config.TimeRangeMode != "" && config.TimeRangeMode != "days" && config.TimeRangeMode != "since_last_update" && config.TimeRangeMode != "all_time" {
		return apperrors.InvalidWorkflowConfigError("invalid time_range_mode, must be one of: days, since_last_update, all_time")
	}

	// 验证LLM配置
	if config.LLMEnabled {
		if config.LLMMaxEpisodes <= 0 {
			return apperrors.InvalidWorkflowConfigError("llm_max_episodes must be positive when LLM is enabled")
		}
	}

	return nil
}

// toWorkflowResponse 转换为响应格式
func (s *WorkflowService) toWorkflowResponse(workflow *models.Workflow) *WorkflowResponse {
	response := &WorkflowResponse{
		ID:          workflow.ID,
		Name:        workflow.Name,
		Description: workflow.Description,
		Schedule:    workflow.Schedule,
		ScopeType:   workflow.ScopeType,
		ScopeConfig: workflow.ScopeConfig,
		RulesConfig: workflow.RulesConfig,
		IsEnabled:   workflow.IsEnabled,
		CreatedAt:   workflow.CreatedAt.Format("2006-01-02 15:04:05"),
		UpdatedAt:   workflow.UpdatedAt.Format("2006-01-02 15:04:05"),
	}

	// 加载调度时间信息
	if workflow.LastExecutionAt != nil {
		formatted := workflow.LastExecutionAt.Format("2006-01-02 15:04:05")
		response.LastExecution = &formatted
	}
	if workflow.NextRunAt != nil {
		formatted := workflow.NextRunAt.Format("2006-01-02 15:04:05")
		response.NextExecution = &formatted
	}

	if jobCount, err := s.repos.Workflow.CountJobs(workflow.ID); err == nil {
		response.JobCount = int(jobCount)
	}

	return response
}
