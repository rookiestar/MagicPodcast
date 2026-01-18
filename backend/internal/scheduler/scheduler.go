package scheduler

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"magicpodcast/internal/models"
	"magicpodcast/internal/workflow"

	"github.com/robfig/cron/v3"
	"gorm.io/gorm"
)

// Scheduler 工作流调度器
type Scheduler struct {
	db       *gorm.DB
	executor *workflow.Executor
	cron     *cron.Cron
	jobIDs   map[uint]cron.EntryID // workflowID -> cron.EntryID
	mu       sync.RWMutex
}

// NewScheduler 创建调度器
func NewScheduler(db *gorm.DB, executor *workflow.Executor) *Scheduler {
	return &Scheduler{
		db:       db,
		executor: executor,
		cron:     cron.New(cron.WithSeconds()),
		jobIDs:   make(map[uint]cron.EntryID),
	}
}

// Start 启动调度器
func (s *Scheduler) Start() error {
	log.Println("🕐 启动工作流调度器")

	// 加载所有已启用的工作流
	var workflows []models.Workflow
	if err := s.db.Where("is_enabled = ? AND schedule != ?", true, "").
		Find(&workflows).Error; err != nil {
		return fmt.Errorf("加载工作流失败: %w", err)
	}

	// 注册每个工作流的cron任务
	for _, wf := range workflows {
		if err := s.registerWorkflow(&wf); err != nil {
			log.Printf("⚠️  注册工作流失败 [ID=%d]: %v", wf.ID, err)
			continue
		}
		log.Printf("✅ 已注册工作流 [ID=%d, Schedule=%s]", wf.ID, wf.Schedule)
	}

	s.cron.Start()
	log.Printf("🚀 调度器已启动，共注册 %d 个工作流", len(workflows))
	return nil
}

// Stop 停止调度器
func (s *Scheduler) Stop() {
	log.Println("🛑 停止工作流调度器")
	ctx := s.cron.Stop()
	<-ctx.Done() // 等待所有正在运行的job完成
	s.mu.Lock()
	s.jobIDs = make(map[uint]cron.EntryID)
	s.mu.Unlock()
}

// registerWorkflow 注册单个工作流
func (s *Scheduler) registerWorkflow(workflow *models.Workflow) error {
	schedule := workflow.Schedule
	if schedule == "" {
		return fmt.Errorf("schedule为空")
	}

	// 封装执行逻辑
	jobFunc := func() {
		s.executeWorkflow(workflow.ID)
	}

	// 添加到cron
	entryID, err := s.cron.AddFunc(schedule, jobFunc)
	if err != nil {
		return fmt.Errorf("添加cron任务失败: %w", err)
	}

	// 记录映射
	s.mu.Lock()
	s.jobIDs[workflow.ID] = entryID
	s.mu.Unlock()

	return nil
}

// executeWorkflow 执行工作流（带重复执行检查和调度历史记录）
func (s *Scheduler) executeWorkflow(workflowID uint) {
	log.Printf("⏰ [调度] 触发工作流 [ID=%d]", workflowID)

	scheduledTime := time.Now()
	startTime := time.Now()

	// 创建调度运行记录
	schedulerRun := &models.SchedulerRun{
		WorkflowID:  workflowID,
		Status:      models.SchedulerRunStatusSuccess,
		ScheduledAt: scheduledTime,
		StartedAt:   &startTime,
	}

	// 重新加载工作流（确保获取最新状态）
	var wf models.Workflow
	if err := s.db.First(&wf, workflowID).Error; err != nil {
		log.Printf("❌ [调度] 工作流不存在 [ID=%d]: %v", workflowID, err)
		schedulerRun.Status = models.SchedulerRunStatusFailed
		schedulerRun.Reason = fmt.Sprintf("工作流不存在: %v", err)
		s.db.Create(schedulerRun)
		return
	}

	// 检查是否启用
	if !wf.IsEnabled {
		log.Printf("⏭️  [调度] 工作流已禁用，跳过执行 [ID=%d]", workflowID)
		schedulerRun.Status = models.SchedulerRunStatusSkipped
		schedulerRun.Reason = "工作流已禁用"
		completedAt := time.Now()
		schedulerRun.CompletedAt = &completedAt
		s.db.Create(schedulerRun)
		return
	}

	// 检查是否有正在运行的任务
	if wf.LastJobID != nil {
		var lastJob models.Job
		if err := s.db.Where("id = ?", *wf.LastJobID).First(&lastJob).Error; err == nil {
			if lastJob.Status == models.JobStatusRunning {
				log.Printf("⏭️  [调度] 工作流正在运行，跳过本次执行 [ID=%d, JobID=%d]",
					workflowID, lastJob.ID)
				schedulerRun.Status = models.SchedulerRunStatusSkipped
				schedulerRun.Reason = fmt.Sprintf("上次任务仍在运行 (JobID=%d)", lastJob.ID)
				completedAt := time.Now()
				schedulerRun.CompletedAt = &completedAt
				s.db.Create(schedulerRun)
				return
			}
		}
	}

	// 执行工作流
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	job, err := s.executor.Execute(ctx, &wf, "cron")
	if err != nil {
		log.Printf("❌ [调度] 工作流执行失败 [ID=%d]: %v", workflowID, err)
		schedulerRun.Status = models.SchedulerRunStatusFailed
		schedulerRun.Reason = fmt.Sprintf("执行失败: %v", err)
	} else {
		log.Printf("✅ [调度] 工作流执行完成 [ID=%d, JobID=%d, 状态=%s]",
			workflowID, job.ID, job.Status)
		schedulerRun.Status = models.SchedulerRunStatusSuccess
		schedulerRun.JobID = &job.ID
	}

	// 完成调度记录
	completedAt := time.Now()
	schedulerRun.CompletedAt = &completedAt
	duration := int(completedAt.Sub(startTime).Milliseconds())
	schedulerRun.Duration = &duration

	if err := s.db.Create(schedulerRun).Error; err != nil {
		log.Printf("⚠️  [调度] 保存调度记录失败: %v", err)
	}

	// 检查连续失败次数并告警
	s.checkAndAlertFailures(workflowID)
}

// checkAndAlertFailures 检查连续失败并记录告警
func (s *Scheduler) checkAndAlertFailures(workflowID uint) {
	const failureThreshold = 3 // 连续失败3次告警

	// 查询最近的调度记录
	var recentRuns []models.SchedulerRun
	if err := s.db.Where("workflow_id = ?", workflowID).
		Order("created_at DESC").
		Limit(failureThreshold).
		Find(&recentRuns).Error; err != nil {
		log.Printf("⚠️  [调度] 查询调度记录失败: %v", err)
		return
	}

	// 检查是否连续失败
	allFailed := true
	for _, run := range recentRuns {
		if run.Status != models.SchedulerRunStatusFailed {
			allFailed = false
			break
		}
	}

	if allFailed && len(recentRuns) >= failureThreshold {
		log.Printf("🚨 [调度] 工作流 [ID=%d] 连续失败 %d 次，需要关注！", workflowID, failureThreshold)
		// TODO: 发送通知（邮件、webhook等）
	}
}

// Reload 重新加载所有工作流
func (s *Scheduler) Reload() error {
	log.Println("🔄 重新加载调度器")

	// 保存旧的 jobIDs 映射用于回滚
	s.mu.Lock()
	oldJobIDs := make(map[uint]cron.EntryID)
	for k, v := range s.jobIDs {
		oldJobIDs[k] = v
	}

	// 移除所有现有的 cron 任务
	for wfID, entryID := range oldJobIDs {
		s.cron.Remove(entryID)
		log.Printf("🗑️  移除工作流 [ID=%d]", wfID)
	}

	// 清空映射
	s.jobIDs = make(map[uint]cron.EntryID)
	// 注意：这里保持锁，避免在清空和重新加载之间有其他操作

	// 加载所有已启用的工作流
	var workflows []models.Workflow
	if err := s.db.Where("is_enabled = ? AND schedule != ?", true, "").
		Find(&workflows).Error; err != nil {
		// 数据库查询失败，回滚：恢复旧的 jobIDs
		log.Printf("❌ 加载工作流失败，尝试回滚: %v", err)
		s.rollbackReload(oldJobIDs)
		s.mu.Unlock()
		return fmt.Errorf("加载工作流失败: %w", err)
	}

	// 注册每个工作流的 cron 任务
	registeredCount := 0
	var firstErr error
	for _, wf := range workflows {
		if err := s.registerWorkflowLocked(&wf); err != nil {
			log.Printf("⚠️  注册工作流失败 [ID=%d]: %v", wf.ID, err)
			if firstErr == nil {
				firstErr = err
			}
			// 继续尝试注册其他工作流
		} else {
			registeredCount++
			log.Printf("✅ 已注册工作流 [ID=%d, Schedule=%s]", wf.ID, wf.Schedule)
		}
	}

	// 如果没有成功注册任何工作流，回滚
	if registeredCount == 0 && len(workflows) > 0 {
		log.Printf("❌ 所有工作流注册失败，尝试回滚: %v", firstErr)
		s.rollbackReload(oldJobIDs)
		s.mu.Unlock()
		return fmt.Errorf("所有工作流注册失败: %w", firstErr)
	}

	s.mu.Unlock()
	log.Printf("🚀 调度器重新加载完成，共注册 %d 个工作流", registeredCount)
	return nil
}

// rollbackReload 回滚到之前的 jobIDs 状态
func (s *Scheduler) rollbackReload(oldJobIDs map[uint]cron.EntryID) {
	log.Printf("🔄 回滚调度器到之前的状态，恢复 %d 个工作流", len(oldJobIDs))

	// 清空当前（可能部分注册的）jobIDs
	s.jobIDs = make(map[uint]cron.EntryID)

	// 恢复旧的 jobIDs 映射
	for wfID := range oldJobIDs {
		// 重新注册到 cron（因为之前被 Remove 了）
		// 注意：这里需要重新获取工作流信息来注册
		var wf models.Workflow
		if err := s.db.First(&wf, wfID).Error; err != nil {
			log.Printf("⚠️  回滚时找不到工作流 [ID=%d]: %v", wfID, err)
			continue
		}

		// 重新注册
		jobFunc := func() {
			s.executeWorkflow(wf.ID)
		}
		if newEntryID, err := s.cron.AddFunc(wf.Schedule, jobFunc); err == nil {
			s.jobIDs[wf.ID] = newEntryID
			log.Printf("✅ 回滚：已恢复工作流 [ID=%d]", wfID)
		} else {
			log.Printf("❌ 回滚：恢复工作流失败 [ID=%d]: %v", wfID, err)
		}
	}
}

// registerWorkflowLocked 注册单个工作流（调用者必须持有锁）
func (s *Scheduler) registerWorkflowLocked(workflow *models.Workflow) error {
	schedule := workflow.Schedule
	if schedule == "" {
		return fmt.Errorf("schedule为空")
	}

	// 封装执行逻辑
	jobFunc := func() {
		s.executeWorkflow(workflow.ID)
	}

	// 添加到cron
	entryID, err := s.cron.AddFunc(schedule, jobFunc)
	if err != nil {
		return fmt.Errorf("添加cron任务失败: %w", err)
	}

	// 记录映射（不需要加锁，因为调用者已经持有锁）
	s.jobIDs[workflow.ID] = entryID

	return nil
}

// PauseWorkflow 暂停指定工作流的调度
func (s *Scheduler) PauseWorkflow(workflowID uint) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	entryID, exists := s.jobIDs[workflowID]
	if !exists {
		return fmt.Errorf("工作流未注册调度")
	}

	s.cron.Remove(entryID)
	delete(s.jobIDs, workflowID)

	log.Printf("⏸️  已暂停工作流调度 [ID=%d]", workflowID)
	return nil
}

// ResumeWorkflow 恢复指定工作流的调度
func (s *Scheduler) ResumeWorkflow(workflowID uint) error {
	var wf models.Workflow
	if err := s.db.First(&wf, workflowID).Error; err != nil {
		return fmt.Errorf("工作流不存在: %w", err)
	}

	if !wf.IsEnabled {
		return fmt.Errorf("工作流未启用")
	}

	if wf.Schedule == "" {
		return fmt.Errorf("工作流未配置schedule")
	}

	return s.registerWorkflow(&wf)
}

// GetStatus 获取调度器状态
func (s *Scheduler) GetStatus() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entries := s.cron.Entries()

	// 构建工作流列表（包含下次执行时间）
	type WorkflowInfo struct {
		EntryID    int       `json:"entry_id"`
		NextRun    time.Time `json:"next_run,omitempty"`
		PrevRun    time.Time `json:"prev_run,omitempty"`
	}

	workflows := make(map[uint]WorkflowInfo)
	for wfID, entryID := range s.jobIDs {
		info := WorkflowInfo{EntryID: int(entryID)}

		// 查找对应的entry获取下次执行时间
		for _, entry := range entries {
			if entry.ID == entryID {
				info.NextRun = entry.Next
				info.PrevRun = entry.Prev
				break
			}
		}

		workflows[wfID] = info
	}

	return map[string]interface{}{
		"is_running": true,
		"total_jobs": len(entries),
		"workflows":  workflows,
	}
}

// GetWorkflowNextRunTime 获取指定工作流的下次执行时间
func (s *Scheduler) GetWorkflowNextRunTime(workflowID uint) (time.Time, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entryID, exists := s.jobIDs[workflowID]
	if !exists {
		return time.Time{}, fmt.Errorf("工作流未注册调度")
	}

	entries := s.cron.Entries()
	for _, entry := range entries {
		if entry.ID == entryID {
			return entry.Next, nil
		}
	}

	return time.Time{}, fmt.Errorf("未找到调度条目")
}

// AddWorkflow 添加工作流到调度器（公开方法）
func (s *Scheduler) AddWorkflow(workflow *models.Workflow) error {
	if !workflow.IsEnabled {
		return fmt.Errorf("工作流未启用")
	}

	if workflow.Schedule == "" {
		return fmt.Errorf("工作流未配置schedule")
	}

	return s.registerWorkflow(workflow)
}
