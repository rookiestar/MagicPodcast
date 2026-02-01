package repository

import (
	"magicpodcast/internal/models"

	"gorm.io/gorm"
)

// WorkflowRepository 工作流数据访问接口
type WorkflowRepository interface {
	Repository

	// Create 创建工作流
	Create(workflow *models.Workflow) error

	// GetByID 根据ID获取工作流
	GetByID(id uint) (*models.Workflow, error)

	// List 获取工作流列表
	List(page, pageSize int) ([]*models.Workflow, int64, error)

	// ListEnabled 获取启用的工作流列表
	ListEnabled() ([]*models.Workflow, error)

	// Update 更新工作流
	Update(workflow *models.Workflow) error

	// Delete 删除工作流
	Delete(id uint) error

	// ToggleStatus 切换启用状态
	ToggleStatus(id uint) (*models.Workflow, error)

	// UpdateStatus 更新状态
	UpdateStatus(id uint, isEnabled bool) error

	// GetBySchedule 根据调度表达式获取工作流
	GetBySchedule(schedule string) ([]*models.Workflow, error)

	// GetWithJobs 获取工作流及其任务
	GetWithJobs(id uint) (*models.Workflow, error)

	// GetLastExecution 获取最后执行记录
	GetLastExecution(workflowID uint) (*models.JobExecution, error)
}

// workflowRepository 工作流数据访问实现
type workflowRepository struct {
	*BaseRepository
}

// NewWorkflowRepository 创建工作流Repository
func NewWorkflowRepository(db *gorm.DB) WorkflowRepository {
	return &workflowRepository{
		BaseRepository: NewBaseRepository(db),
	}
}

// Create 创建工作流
func (r *workflowRepository) Create(workflow *models.Workflow) error {
	return r.DB().Create(workflow).Error
}

// GetByID 根据ID获取工作流
func (r *workflowRepository) GetByID(id uint) (*models.Workflow, error) {
	var workflow models.Workflow
	err := r.DB().First(&workflow, id).Error
	if err != nil {
		return nil, err
	}
	return &workflow, nil
}

// List 获取工作流列表
func (r *workflowRepository) List(page, pageSize int) ([]*models.Workflow, int64, error) {
	var workflows []*models.Workflow
	var total int64

	query := r.DB().Model(&models.Workflow{})

	// 计算总数
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 分页并预加载最后任务
	offset := (page - 1) * pageSize
	err := query.Preload("LastJob").
		Order("created_at DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&workflows).Error

	return workflows, total, err
}

// ListEnabled 获取启用的工作流列表
func (r *workflowRepository) ListEnabled() ([]*models.Workflow, error) {
	var workflows []*models.Workflow
	err := r.DB().Where("is_enabled = ?", true).
		Order("created_at ASC").
		Find(&workflows).Error
	return workflows, err
}

// Update 更新工作流
func (r *workflowRepository) Update(workflow *models.Workflow) error {
	return r.DB().Save(workflow).Error
}

// Delete 删除工作流
func (r *workflowRepository) Delete(id uint) error {
	return r.DB().Delete(&models.Workflow{}, id).Error
}

// ToggleStatus 切换启用状态
func (r *workflowRepository) ToggleStatus(id uint) (*models.Workflow, error) {
	var workflow models.Workflow
	if err := r.DB().First(&workflow, id).Error; err != nil {
		return nil, err
	}

	workflow.IsEnabled = !workflow.IsEnabled
	if err := r.DB().Save(&workflow).Error; err != nil {
		return nil, err
	}

	return &workflow, nil
}

// UpdateStatus 更新状态
func (r *workflowRepository) UpdateStatus(id uint, isEnabled bool) error {
	return r.DB().Model(&models.Workflow{}).
		Where("id = ?", id).
		Update("is_enabled", isEnabled).Error
}

// GetBySchedule 根据调度表达式获取工作流
func (r *workflowRepository) GetBySchedule(schedule string) ([]*models.Workflow, error) {
	var workflows []*models.Workflow
	err := r.DB().Where("schedule = ? AND is_enabled = ?", schedule, true).
		Find(&workflows).Error
	return workflows, err
}

// GetWithJobs 获取工作流及其任务
func (r *workflowRepository) GetWithJobs(id uint) (*models.Workflow, error) {
	var workflow models.Workflow
	err := r.DB().Preload("Jobs").First(&workflow, id).Error
	if err != nil {
		return nil, err
	}
	return &workflow, nil
}

// GetLastExecution 获取最后执行记录
func (r *workflowRepository) GetLastExecution(workflowID uint) (*models.JobExecution, error) {
	var execution models.JobExecution
	err := r.DB().Where("workflow_id = ?", workflowID).
		Order("start_time DESC").
		First(&execution).Error
	if err != nil {
		return nil, err
	}
	return &execution, nil
}
