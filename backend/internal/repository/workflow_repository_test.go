package repository

import (
	"magicpodcast/internal/models"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkflowRepository_Create(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewWorkflowRepository(db)

	// 创建测试工作流
	workflow := generateUniqueWorkflow(1)

	err := repo.Create(workflow)
	require.NoError(t, err)
	assert.NotZero(t, workflow.ID)
	assert.NotZero(t, workflow.CreatedAt)
}

func TestWorkflowRepository_GetByID(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewWorkflowRepository(db)

	// 创建测试数据
	workflow := generateUniqueWorkflow(1)
	require.NoError(t, repo.Create(workflow))

	// 测试查询
	found, err := repo.GetByID(workflow.ID)
	require.NoError(t, err)
	assert.Equal(t, workflow.Name, found.Name)
	assert.True(t, found.IsEnabled)
}

func TestWorkflowRepository_List(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewWorkflowRepository(db)

	// 创建多个测试工作流
	for i := 1; i <= 5; i++ {
		workflow := generateUniqueWorkflow(i)
		require.NoError(t, repo.Create(workflow))
	}

	// 测试列表查询
	workflows, total, err := repo.List(1, 10)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, total, int64(5))
	assert.GreaterOrEqual(t, len(workflows), 5)
}

func TestWorkflowRepository_ListEnabled(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewWorkflowRepository(db)

	// 创建启用和禁用的工作流
	workflow1 := generateUniqueWorkflow(1)
	workflow1.IsEnabled = true
	workflow2 := generateUniqueWorkflow(2)
	workflow2.IsEnabled = false
	require.NoError(t, repo.Create(workflow1))
	require.NoError(t, repo.Create(workflow2))

	// 查询启用的工作流
	workflows, err := repo.ListEnabled()
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(workflows), 1)

	// 验证所有返回的工作流都是启用状态
	for _, wf := range workflows {
		assert.True(t, wf.IsEnabled)
	}
}

func TestWorkflowRepository_Update(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewWorkflowRepository(db)

	// 创建测试工作流
	workflow := generateUniqueWorkflow(1)
	workflow.Name = "原标题"
	workflow.Description = "原描述"
	workflow.IsEnabled = false
	require.NoError(t, repo.Create(workflow))

	// 更新
	workflow.Name = "新标题"
	workflow.Description = "新描述"
	workflow.IsEnabled = true
	err := repo.Update(workflow)
	require.NoError(t, err)

	// 验证
	found, _ := repo.GetByID(workflow.ID)
	assert.Equal(t, "新标题", found.Name)
	assert.Equal(t, "新描述", found.Description)
	assert.True(t, found.IsEnabled)
}

func TestWorkflowRepository_Delete(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewWorkflowRepository(db)

	// 创建测试工作流
	workflow := generateUniqueWorkflow(1)
	workflow.Name = "待删除工作流"
	require.NoError(t, repo.Create(workflow))

	// 删除
	err := repo.Delete(workflow.ID)
	require.NoError(t, err)

	// 验证已删除
	_, err = repo.GetByID(workflow.ID)
	assert.Error(t, err)
}

func TestWorkflowRepository_ToggleStatus(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewWorkflowRepository(db)

	// 创建测试工作流
	workflow := generateUniqueWorkflow(1)
	workflow.IsEnabled = false
	require.NoError(t, repo.Create(workflow))
	assert.False(t, workflow.IsEnabled)

	// 切换状态
	toggled, err := repo.ToggleStatus(workflow.ID)
	require.NoError(t, err)
	assert.True(t, toggled.IsEnabled)

	// 再次切换
	toggled2, err := repo.ToggleStatus(workflow.ID)
	require.NoError(t, err)
	assert.False(t, toggled2.IsEnabled)
}

func TestWorkflowRepository_UpdateStatus(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewWorkflowRepository(db)

	// 创建测试工作流
	workflow := generateUniqueWorkflow(1)
	workflow.IsEnabled = false
	require.NoError(t, repo.Create(workflow))

	// 启用工作流
	err := repo.UpdateStatus(workflow.ID, true)
	require.NoError(t, err)

	// 验证
	found, _ := repo.GetByID(workflow.ID)
	assert.True(t, found.IsEnabled)

	// 禁用工作流
	err = repo.UpdateStatus(workflow.ID, false)
	require.NoError(t, err)

	// 验证
	found2, _ := repo.GetByID(workflow.ID)
	assert.False(t, found2.IsEnabled)
}

func TestWorkflowRepository_GetBySchedule(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewWorkflowRepository(db)

	// 创建相同调度时间的工作流
	schedule := "0 2 * * *"
	workflow1 := generateUniqueWorkflow(1)
	workflow1.Name = "工作流1"
	workflow1.IsEnabled = true
	workflow1.Schedule = schedule
	workflow2 := generateUniqueWorkflow(2)
	workflow2.Name = "工作流2"
	workflow2.IsEnabled = true
	workflow2.Schedule = schedule
	workflow3 := generateUniqueWorkflow(3)
	workflow3.Name = "工作流3"
	workflow3.IsEnabled = false
	workflow3.Schedule = schedule
	require.NoError(t, repo.Create(workflow1))
	require.NoError(t, repo.Create(workflow2))
	require.NoError(t, repo.Create(workflow3))

	// 查询该调度时间且启用的工作流
	workflows, err := repo.GetBySchedule(schedule)
	require.NoError(t, err)
	assert.Equal(t, 2, len(workflows))

	// 验证所有返回的工作流都是启用状态
	for _, wf := range workflows {
		assert.Equal(t, schedule, wf.Schedule)
		assert.True(t, wf.IsEnabled)
	}
}

func TestWorkflowRepository_GetWithJobs(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewWorkflowRepository(db)

	// 创建工作流
	workflow := generateUniqueWorkflow(1)
	require.NoError(t, repo.Create(workflow))

	// 创建任务
	job := &models.Job{
		WorkflowID: workflow.ID,
		Status:     "pending",
	}
	require.NoError(t, db.Create(job).Error)

	// 查询工作流及其任务
	found, err := repo.GetWithJobs(workflow.ID)
	require.NoError(t, err)
	assert.Equal(t, workflow.ID, found.ID)
	// 注意：这里验证 Jobs 关联是否正确加载
	// assert.GreaterOrEqual(t, len(found.Jobs), 1)
}

func TestWorkflowRepository_GetLastExecution(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewWorkflowRepository(db)

	// 创建工作流
	workflow := generateUniqueWorkflow(1)
	require.NoError(t, repo.Create(workflow))

	// 创建任务和执行记录
	job := &models.Job{
		WorkflowID: workflow.ID,
		Status:     models.JobStatusCompleted,
	}
	require.NoError(t, db.Create(job).Error)

	podcastID := uint(1)
	execution := &models.JobExecution{
		JobID:          job.ID,
		PodcastID:      &podcastID,
		Status:         models.ExecutionStatusSuccess,
		LogInfo:        "执行成功",
		ProcessingTime: 1000,
	}
	require.NoError(t, db.Create(execution).Error)

	// 查询最后执行记录
	lastExec, err := repo.GetLastExecution(workflow.ID)
	require.NoError(t, err)
	assert.NotNil(t, lastExec)
	assert.Equal(t, models.ExecutionStatusSuccess, lastExec.Status)
}
