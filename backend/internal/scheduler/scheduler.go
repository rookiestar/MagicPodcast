package scheduler

import (
	"context"
	"fmt"
	"magicpodcast/internal/logger"
	"strings"
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
	// 使用本地时区创建cron调度器
	// 这样schedule表达式（如 "0 35 8 * * *"）会按本地时间早上8:35执行
	localLoc := time.Local
	return &Scheduler{
		db:       db,
		executor: executor,
		cron:     cron.New(cron.WithSeconds(), cron.WithLocation(localLoc)),
		jobIDs:   make(map[uint]cron.EntryID),
	}
}

// Start 启动调度器
func (s *Scheduler) Start() error {
	logger.Info("🕐 启动工作流调度器")

	// 加载所有已启用的工作流
	var workflows []models.Workflow
	if err := s.db.Where("is_enabled = ? AND schedule != ?", true, "").
		Find(&workflows).Error; err != nil {
		return fmt.Errorf("加载工作流失败: %w", err)
	}

	if len(workflows) == 0 {
		logger.Info("📭 没有已启用的工作流需要调度")
		s.cron.Start()
		return nil
	}

	// 1. 注册每个工作流的cron任务
	registeredCount := 0
	for _, wf := range workflows {
		if err := s.registerWorkflow(&wf); err != nil {
			logger.Infof("⚠️  注册工作流失败 [ID=%d]: %v", wf.ID, err)
			continue
		}
		registeredCount++
		logger.Infof("✅ 已注册工作流 [ID=%d, Schedule=%s]", wf.ID, wf.Schedule)
	}

	// 2. 启动cron调度器
	s.cron.Start()
	logger.Infof("🚀 调度器已启动，共注册 %d 个工作流", registeredCount)

	// 3. 检查并补偿错过的任务执行
	s.checkAndExecuteMissedWorkflows(&workflows)

	return nil
}

// checkAndExecuteMissedWorkflows 检查并执行错过的任务
func (s *Scheduler) checkAndExecuteMissedWorkflows(workflows *[]models.Workflow) {
	logger.Info("🔍 检查是否有错过的任务需要补偿执行...")

	now := time.Now()
	missedCount := 0

	for _, wf := range *workflows {
		// 检查是否设置了下次执行时间
		if wf.NextRunAt == nil {
			// 首次运行或未设置，初始化下次执行时间
			nextRun, err := wf.GetNextRunTime()
			if err != nil {
				logger.Infof("⚠️  计算下次执行时间失败 [ID=%d]: %v", wf.ID, err)
				continue
			}

			// 更新到数据库 (使用 UTC)
			s.db.Model(&wf).Update("next_run_at", nextRun.UTC())
			logger.Infof("📅 初始化下次执行时间 [ID=%d, NextRun=%s]", wf.ID, nextRun.UTC().Format("2006-01-02 15:04:05"))
			continue
		}

		// 检查是否错过了执行时间
		// 统一使用 UTC 时间进行比较,避免时区混淆
		nowUTC := now.UTC()
		nextRunUTC := wf.NextRunAt.UTC()

		if nextRunUTC.Before(nowUTC) {
			missedCount++
			logger.Infof("⚠️  发现错过的执行 [ID=%d, 计划时间=%s, 当前时间=%s]",
				wf.ID,
				wf.NextRunAt.Format("2006-01-02 15:04:05"),
				now.Format("2006-01-02 15:04:05"))

			// 异步补偿执行（避免阻塞调度器启动）
			go s.executeMissedWorkflow(&wf)
		} else {
			logger.Infof("✅ 调度正常 [ID=%d, NextRun=%s]",
				wf.ID,
				wf.NextRunAt.Format("2006-01-02 15:04:05"))
		}
	}

	if missedCount == 0 {
		logger.Info("✅ 没有错过的任务")
	} else {
		logger.Infof("🔧 发现 %d 个错过的任务，正在异步补偿执行...", missedCount)
	}
}

// executeMissedWorkflow 执行错过的任务
func (s *Scheduler) executeMissedWorkflow(workflow *models.Workflow) {
	// 添加短暂的延迟，避免与其他启动任务冲突
	time.Sleep(2 * time.Second)

	logger.Infof("🔄 开始补偿执行 [ID=%d, Name=%s]", workflow.ID, workflow.Name)

	// 检查是否有正在运行的任务
	if workflow.LastJobID != nil {
		var lastJob models.Job
		if err := s.db.Where("id = ?", *workflow.LastJobID).First(&lastJob).Error; err == nil {
			if lastJob.Status == models.JobStatusRunning {
				logger.Infof("⏭️  补偿执行跳过：上次任务仍在运行 [ID=%d, JobID=%d]",
					workflow.ID, lastJob.ID)
				return
			}
		}
	}

	// 执行工作流（标记为补偿执行）
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	job, err := s.executor.Execute(ctx, workflow, "cron-catchup")
	if err != nil {
		logger.Infof("❌ 补偿执行失败 [ID=%d]: %v", workflow.ID, err)
	} else {
		logger.Infof("✅ 补偿执行成功 [ID=%d, JobID=%d]", workflow.ID, job.ID)
	}

	// 计算并更新下次执行时间
	nextRun, err := workflow.GetNextRunTime()
	if err == nil {
		s.db.Model(workflow).Updates(map[string]interface{}{
			"last_execution_at": time.Now().UTC(),
			"next_run_at":       nextRun.UTC(),
		})
		logger.Infof("📅 更新下次执行时间 [ID=%d, NextRun=%s]", workflow.ID, nextRun.UTC().Format("2006-01-02 15:04:05"))
	}
}

// Stop 停止调度器
func (s *Scheduler) Stop() {
	logger.Info("🛑 停止工作流调度器")
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

	// 自动兼容5位和6位表达式
	// 统一转换为6位格式（秒 分 时 日 月 周）
	parts := strings.Fields(schedule)
	var originalSchedule string
	if len(parts) == 5 {
		// 5位表达式：分 时 日 月 周 -> 自动添加秒位
		originalSchedule = schedule
		schedule = "0 " + schedule
		logger.Infof("📝 自动转换5位表达式为6位 [ID=%d]: %s -> %s",
			workflow.ID, originalSchedule, schedule)
	} else if len(parts) != 6 {
		return fmt.Errorf("不支持的cron表达式格式: %s (期望5位或6位)", schedule)
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
	logger.Infof("⏰ [调度] 触发工作流 [ID=%d]", workflowID)

	// 重新加载工作流（确保获取最新状态）
	var wf models.Workflow
	if err := s.db.First(&wf, workflowID).Error; err != nil {
		logger.Infof("❌ [调度] 工作流不存在 [ID=%d]: %v", workflowID, err)
		return
	}

	// 检查是否启用
	if !wf.IsEnabled {
		logger.Infof("⏭️  [调度] 工作流已禁用，跳过执行 [ID=%d]", workflowID)
		return
	}

	// 检查是否有正在运行的任务
	if wf.LastJobID != nil {
		var lastJob models.Job
		if err := s.db.Where("id = ?", *wf.LastJobID).First(&lastJob).Error; err == nil {
			if lastJob.Status == models.JobStatusRunning {
				logger.Infof("⏭️  [调度] 工作流正在运行，跳过本次执行 [ID=%d, JobID=%d]",
					workflowID, lastJob.ID)
				return
			}
		}
	}

	// 执行工作流
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	job, err := s.executor.Execute(ctx, &wf, "cron")
	if err != nil {
		logger.Infof("❌ [调度] 工作流执行失败 [ID=%d]: %v", workflowID, err)
		s.checkAndAlertFailures(workflowID)
		return
	}

	logger.Infof("✅ [调度] 工作流执行完成 [ID=%d, JobID=%d, 状态=%s]",
		workflowID, job.ID, job.Status)

	// 更新调度状态（持久化到数据库）
	now := time.Now()

	// 关键修复：重新加载工作流以获取最新的schedule配置
	// 因为用户可能在执行过程中修改了schedule
	var updatedWf models.Workflow
	if err := s.db.First(&updatedWf, workflowID).Error; err == nil {
		wf = updatedWf
	}

	// 使用当前时间计算下次执行时间（而不是执行完成时间）
	// 这样可以确保即使执行有延迟，下次执行时间仍然是正确的
	nextRun, err := wf.GetNextRunTime()
	if err != nil {
		logger.Infof("⚠️  [调度] 计算下次执行时间失败 [ID=%d]: %v", workflowID, err)
		nextRun = now.AddDate(0, 0, 1) // 默认明天
	}

	// 统一使用 UTC 时间存储,避免时区混乱
	updates := map[string]interface{}{
		"last_execution_at": now.UTC(),
		"next_run_at":       nextRun.UTC(),
	}

	if err := s.db.Model(&wf).Updates(updates).Error; err != nil {
		logger.Infof("⚠️  [调度] 更新调度状态失败 [ID=%d]: %v", workflowID, err)
	} else {
		logger.Infof("📅 [调度] 已更新调度状态 [ID=%d, LastExecution=%s, NextRun=%s]",
			workflowID,
			now.Local().Format("2006-01-02 15:04:05"),
			nextRun.Local().Format("2006-01-02 15:04:05"))
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
		logger.Infof("⚠️  [调度] 查询调度记录失败: %v", err)
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
		logger.Infof("🚨 [调度] 工作流 [ID=%d] 连续失败 %d 次，需要关注！", workflowID, failureThreshold)
		// 当前仅记录告警日志；邮件或 webhook 通知需要先确认真实使用策略。
	}
}

// Reload 重新加载所有工作流
func (s *Scheduler) Reload() error {
	logger.Info("🔄 重新加载调度器")

	// 保存旧的 jobIDs 映射用于回滚
	s.mu.Lock()
	oldJobIDs := make(map[uint]cron.EntryID)
	for k, v := range s.jobIDs {
		oldJobIDs[k] = v
	}

	// 移除所有现有的 cron 任务
	for wfID, entryID := range oldJobIDs {
		s.cron.Remove(entryID)
		logger.Infof("🗑️  移除工作流 [ID=%d]", wfID)
	}

	// 清空映射
	s.jobIDs = make(map[uint]cron.EntryID)
	// 注意：这里保持锁，避免在清空和重新加载之间有其他操作

	// 加载所有已启用的工作流
	var workflows []models.Workflow
	if err := s.db.Where("is_enabled = ? AND schedule != ?", true, "").
		Find(&workflows).Error; err != nil {
		// 数据库查询失败，回滚：恢复旧的 jobIDs
		logger.Infof("❌ 加载工作流失败，尝试回滚: %v", err)
		s.rollbackReload(oldJobIDs)
		s.mu.Unlock()
		return fmt.Errorf("加载工作流失败: %w", err)
	}

	// 注册每个工作流的 cron 任务
	registeredCount := 0
	var firstErr error
	for _, wf := range workflows {
		if err := s.registerWorkflowLocked(&wf); err != nil {
			logger.Infof("⚠️  注册工作流失败 [ID=%d]: %v", wf.ID, err)
			if firstErr == nil {
				firstErr = err
			}
			// 继续尝试注册其他工作流
		} else {
			registeredCount++
			logger.Infof("✅ 已注册工作流 [ID=%d, Schedule=%s]", wf.ID, wf.Schedule)
		}
	}

	// 如果没有成功注册任何工作流，回滚
	if registeredCount == 0 && len(workflows) > 0 {
		logger.Infof("❌ 所有工作流注册失败，尝试回滚: %v", firstErr)
		s.rollbackReload(oldJobIDs)
		s.mu.Unlock()
		return fmt.Errorf("所有工作流注册失败: %w", firstErr)
	}

	s.mu.Unlock()
	logger.Infof("🚀 调度器重新加载完成，共注册 %d 个工作流", registeredCount)
	return nil
}

// rollbackReload 回滚到之前的 jobIDs 状态
func (s *Scheduler) rollbackReload(oldJobIDs map[uint]cron.EntryID) {
	logger.Infof("🔄 回滚调度器到之前的状态，恢复 %d 个工作流", len(oldJobIDs))

	// 清空当前（可能部分注册的）jobIDs
	s.jobIDs = make(map[uint]cron.EntryID)

	// 恢复旧的 jobIDs 映射
	for wfID := range oldJobIDs {
		// 重新注册到 cron（因为之前被 Remove 了）
		// 注意：这里需要重新获取工作流信息来注册
		var wf models.Workflow
		if err := s.db.First(&wf, wfID).Error; err != nil {
			logger.Infof("⚠️  回滚时找不到工作流 [ID=%d]: %v", wfID, err)
			continue
		}

		// 重新注册
		jobFunc := func() {
			s.executeWorkflow(wf.ID)
		}
		if newEntryID, err := s.cron.AddFunc(wf.Schedule, jobFunc); err == nil {
			s.jobIDs[wf.ID] = newEntryID
			logger.Infof("✅ 回滚：已恢复工作流 [ID=%d]", wfID)
		} else {
			logger.Infof("❌ 回滚：恢复工作流失败 [ID=%d]: %v", wfID, err)
		}
	}
}

// registerWorkflowLocked 注册单个工作流（调用者必须持有锁）
func (s *Scheduler) registerWorkflowLocked(workflow *models.Workflow) error {
	schedule := workflow.Schedule
	if schedule == "" {
		return fmt.Errorf("schedule为空")
	}

	// 自动兼容5位和6位表达式
	// 统一转换为6位格式（秒 分 时 日 月 周）
	parts := strings.Fields(schedule)
	var originalSchedule string
	if len(parts) == 5 {
		// 5位表达式：分 时 日 月 周 -> 自动添加秒位
		originalSchedule = schedule
		schedule = "0 " + schedule
		logger.Infof("📝 自动转换5位表达式为6位 [ID=%d]: %s -> %s",
			workflow.ID, originalSchedule, schedule)
	} else if len(parts) != 6 {
		return fmt.Errorf("不支持的cron表达式格式: %s (期望5位或6位)", schedule)
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

	logger.Infof("⏸️  已暂停工作流调度 [ID=%d]", workflowID)
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
		EntryID int       `json:"entry_id"`
		NextRun time.Time `json:"next_run,omitempty"`
		PrevRun time.Time `json:"prev_run,omitempty"`
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
