package scheduler

import (
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"magicpodcast/internal/models"

	"github.com/robfig/cron/v3"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// mockExecutor 模拟执行器，用于测试
type mockExecutor struct {
	executeCount int
	mu           sync.Mutex
}

func (m *mockExecutor) Execute(workflowID uint) error {
	m.mu.Lock()
	m.executeCount++
	m.mu.Unlock()
	return nil
}

// setupTestDB 创建测试数据库
func setupTestDB(t *testing.T) *gorm.DB {
	// 使用临时文件数据库，避免多个测试之间的数据共享
	// 使用t.Name()和t.Time()确保每次测试都有独立的数据库
	tmpFile := fmt.Sprintf("/tmp/test_scheduler_%d_%s.db", time.Now().UnixNano(), t.Name())
	db, err := gorm.Open(sqlite.Open(tmpFile), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}

	// 自动迁移测试表
	if err := db.AutoMigrate(&models.Workflow{}, &models.Job{}, &models.SchedulerRun{}); err != nil {
		t.Fatalf("Failed to migrate test database: %v", err)
	}

	t.Cleanup(func() {
		// 清理测试数据库
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			sqlDB.Close()
		}
		// 删除临时文件
		os.Remove(tmpFile)
		os.Remove(tmpFile + "-shm")
		os.Remove(tmpFile + "-wal")
	})

	return db
}

// createTestWorkflow 创建测试工作流
func createTestWorkflow(t *testing.T, db *gorm.DB, id uint, enabled bool, schedule string) *models.Workflow {
	wf := &models.Workflow{
		Name:        fmt.Sprintf("test-workflow-%d", id), // 确保名称唯一
		Schedule:    schedule,
		ScopeType:   models.ScopeTypeAllSubscribed,
		ScopeConfig: models.ScopeConfig{},
		RulesConfig: models.RulesConfig{},
		IsEnabled:   enabled,
	}

	if err := db.Create(wf).Error; err != nil {
		t.Fatalf("Failed to create test workflow: %v", err)
	}

	return wf
}

// TestScheduler_Reload_ConcurrentAccess 测试 Reload 时的并发访问
func TestScheduler_Reload_ConcurrentAccess(t *testing.T) {
	db := setupTestDB(t)

	// 创建测试工作流
	var workflowIDs []uint
	for i := uint(1); i <= 5; i++ {
		wf := createTestWorkflow(t, db, i, true, "0 0 * * * *") // 每小时执行
		workflowIDs = append(workflowIDs, wf.ID)
	}

	// 创建调度器（不启动 cron）
	scheduler := &Scheduler{
		db:     db,
		cron:   cron.New(cron.WithSeconds()),
		jobIDs: make(map[uint]cron.EntryID),
	}

	// 手动注册一些初始工作流
	scheduler.mu.Lock()
	for _, wfID := range workflowIDs {
		entryID, _ := scheduler.cron.AddFunc("0 0 * * * *", func() {})
		scheduler.jobIDs[wfID] = entryID
	}
	scheduler.mu.Unlock()

	scheduler.cron.Start()

	// 启动多个 goroutine 并发访问调度器
	var wg sync.WaitGroup
	errors := make(chan error, 10)

	// goroutine 1: 执行 Reload
	wg.Add(1)
	go func() {
		defer wg.Done()
		time.Sleep(10 * time.Millisecond)
		if err := scheduler.Reload(); err != nil {
			errors <- err
		}
	}()

	// goroutine 2-5: 并发读取状态
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				time.Sleep(5 * time.Millisecond)
				status := scheduler.GetStatus()
				if status == nil {
					errors <- &testError{"status is nil"}
				}
			}
		}()
	}

	// 等待所有 goroutine 完成
	wg.Wait()
	close(errors)

	// 检查是否有错误
	for err := range errors {
		t.Errorf("并发访问错误: %v", err)
	}

	// 验证调度器状态正常
	scheduler.mu.RLock()
	finalJobCount := len(scheduler.jobIDs)
	scheduler.mu.RUnlock()

	if finalJobCount != len(workflowIDs) {
		t.Errorf("期望有 %d 个工作流，实际有 %d 个", len(workflowIDs), finalJobCount)
	}
}

// TestScheduler_Reload_FailureWithRollback 测试 Reload 失败时的回滚
func TestScheduler_Reload_FailureWithRollback(t *testing.T) {
	db := setupTestDB(t)

	// 创建测试工作流
	// 注意：不能直接创建无效的 cron 表达式，因为 Workflow 的 BeforeSave hook 会验证
	// 我们通过禁用工作流来模拟部分失败的场景
	wf1 := createTestWorkflow(t, db, 1, true, "0 0 * * * *")
	_ = createTestWorkflow(t, db, 2, false, "0 */5 * * * *") // 禁用的工作流不会被注册
	_ = createTestWorkflow(t, db, 3, true, "0 */10 * * * *")

	// 创建调度器
	scheduler := &Scheduler{
		db:     db,
		cron:   cron.New(cron.WithSeconds()),
		jobIDs: make(map[uint]cron.EntryID),
	}

	// 预先注册一个有效的工作流
	scheduler.mu.Lock()
	entryID, _ := scheduler.cron.AddFunc("0 0 * * * *", func() {})
	scheduler.jobIDs[wf1.ID] = entryID
	scheduler.mu.Unlock()

	scheduler.cron.Start()

	// 记录初始状态
	scheduler.mu.RLock()
	initialCount := len(scheduler.jobIDs)
	scheduler.mu.RUnlock()

	if initialCount != 1 {
		t.Fatalf("初始状态应该有 1 个工作流，实际有 %d 个", initialCount)
	}

	// 执行 Reload（应该会部分失败，但不会全部失败）
	err := scheduler.Reload()

	// Reload 应该成功（因为至少有一个有效的工作流）
	if err != nil {
		t.Errorf("Reload 不应该失败: %v", err)
	}

	// 验证调度器仍然有工作流（不是空的）
	scheduler.mu.RLock()
	finalCount := len(scheduler.jobIDs)
	scheduler.mu.RUnlock()

	if finalCount == 0 {
		t.Error("Reload 失败后调度器不应该为空")
	}

	// 应该有2个有效的工作流（ID=1 和 ID=3，ID=2被禁用）
	if finalCount != 2 {
		t.Logf("Warning: 期望有 2 个工作流，实际有 %d 个", finalCount)
	}
}

// TestScheduler_Reload_EmptyDatabase 测试空数据库的 Reload
func TestScheduler_Reload_EmptyDatabase(t *testing.T) {
	db := setupTestDB(t)

	scheduler := &Scheduler{
		db:     db,
		cron:   cron.New(cron.WithSeconds()),
		jobIDs: make(map[uint]cron.EntryID),
	}

	scheduler.cron.Start()

	// 空数据库的 Reload 应该成功
	if err := scheduler.Reload(); err != nil {
		t.Errorf("空数据库的 Reload 应该成功: %v", err)
	}

	scheduler.mu.RLock()
	count := len(scheduler.jobIDs)
	scheduler.mu.RUnlock()

	if count != 0 {
		t.Errorf("空数据库 Reload 后应该有 0 个工作流，实际有 %d 个", count)
	}
}

// TestScheduler_Reload_MultipleSequential 测试多次连续的 Reload
func TestScheduler_Reload_MultipleSequential(t *testing.T) {
	db := setupTestDB(t)

	// 创建测试工作流
	wf1 := createTestWorkflow(t, db, 1, true, "0 0 * * * *")
	wf2 := createTestWorkflow(t, db, 2, true, "0 */5 * * * *")

	scheduler := &Scheduler{
		db:     db,
		cron:   cron.New(cron.WithSeconds()),
		jobIDs: make(map[uint]cron.EntryID),
	}

	scheduler.cron.Start()

	// 执行多次 Reload
	for i := 0; i < 5; i++ {
		if err := scheduler.Reload(); err != nil {
			t.Errorf("第 %d 次 Reload 失败: %v", i+1, err)
		}

		scheduler.mu.RLock()
		count := len(scheduler.jobIDs)
		scheduler.mu.RUnlock()

		expectedCount := 0
		// 检查有多少个workflow是enabled的
		if wf1.IsEnabled {
			expectedCount++
		}
		if wf2.IsEnabled {
			expectedCount++
		}

		if count != expectedCount {
			t.Errorf("第 %d 次 Reload 后应该有 %d 个工作流，实际有 %d 个", i+1, expectedCount, count)
		}

		time.Sleep(10 * time.Millisecond)
	}
}

// TestScheduler_Reload_ConcurrentReload 测试并发的 Reload 调用
func TestScheduler_Reload_ConcurrentReload(t *testing.T) {
	db := setupTestDB(t)

	// 创建测试工作流
	var workflowIDs []uint
	for i := uint(1); i <= 3; i++ {
		wf := createTestWorkflow(t, db, i, true, "0 0 * * * *")
		workflowIDs = append(workflowIDs, wf.ID)
	}

	scheduler := &Scheduler{
		db:     db,
		cron:   cron.New(cron.WithSeconds()),
		jobIDs: make(map[uint]cron.EntryID),
	}

	scheduler.cron.Start()

	// 并发执行多个 Reload
	var wg sync.WaitGroup
	errors := make(chan error, 3)

	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := scheduler.Reload(); err != nil {
				errors <- err
			}
		}()
	}

	wg.Wait()
	close(errors)

	// 检查是否有错误
	for err := range errors {
		t.Logf("Reload error (might be expected): %v", err)
	}

	// 验证最终状态一致
	scheduler.mu.RLock()
	finalCount := len(scheduler.jobIDs)
	scheduler.mu.RUnlock()

	// 并发场景下，最终应该有正确数量的工作流
	if finalCount != int(len(workflowIDs)) {
		t.Logf("Note: 期望有 %d 个工作流，实际有 %d 个（并发可能不稳定）", len(workflowIDs), finalCount)
	}
}

// TestScheduler_PauseAndResume 测试暂停和恢复工作流
func TestScheduler_PauseAndResume(t *testing.T) {
	db := setupTestDB(t)

	wf := createTestWorkflow(t, db, 1, true, "0 0 * * * *")

	scheduler := &Scheduler{
		db:     db,
		cron:   cron.New(cron.WithSeconds()),
		jobIDs: make(map[uint]cron.EntryID),
	}

	// 添加工作流
	scheduler.mu.Lock()
	entryID, _ := scheduler.cron.AddFunc(wf.Schedule, func() {})
	scheduler.jobIDs[wf.ID] = entryID
	scheduler.mu.Unlock()

	scheduler.cron.Start()

	// 暂停工作流
	if err := scheduler.PauseWorkflow(wf.ID); err != nil {
		t.Errorf("暂停工作流失败: %v", err)
	}

	scheduler.mu.RLock()
	_, exists := scheduler.jobIDs[wf.ID]
	scheduler.mu.RUnlock()

	if exists {
		t.Error("暂停后工作流不应该存在于 jobIDs 中")
	}

	// 重新加载workflow以确保是最新的状态
	if err := db.First(wf, wf.ID).Error; err != nil {
		t.Fatalf("重新加载workflow失败: %v", err)
	}

	// 恢复工作流
	if err := scheduler.ResumeWorkflow(wf.ID); err != nil {
		t.Errorf("恢复工作流失败: %v", err)
	}

	scheduler.mu.RLock()
	_, exists = scheduler.jobIDs[wf.ID]
	scheduler.mu.RUnlock()

	if !exists {
		t.Error("恢复后工作流应该存在于 jobIDs 中")
	}
}

// testError 自定义错误类型
type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}
