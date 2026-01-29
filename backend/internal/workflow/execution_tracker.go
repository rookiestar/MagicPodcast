package workflow

import (
	"context"
	"magicpodcast/internal/logger"
	"sync"
	"time"
)

// ExecutionTracker 工作流执行跟踪器
// 用于跟踪正在运行的工作流执行，防止重复执行和Goroutine泄漏
type ExecutionTracker struct {
	mu      sync.RWMutex
	running map[uint]context.CancelFunc // workflowID -> cancel function
}

// NewExecutionTracker 创建新的执行跟踪器
func NewExecutionTracker() *ExecutionTracker {
	return &ExecutionTracker{
		running: make(map[uint]context.CancelFunc),
	}
}

// TryStart 尝试启动工作流执行
// 如果工作流已在运行，返回false和现有任务的cancel函数
// 如果可以启动，返回true和新创建的context
func (et *ExecutionTracker) TryStart(workflowID uint, timeout time.Duration) (context.Context, bool) {
	et.mu.Lock()
	defer et.mu.Unlock()

	// 检查是否已有任务在运行
	if _, exists := et.running[workflowID]; exists {
		return nil, false
	}

	// 创建可取消的context
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	et.running[workflowID] = cancel

	logger.Infof("✅ 工作流执行已注册 [WorkflowID=%d]", workflowID)
	return ctx, true
}

// Complete 标记工作流执行完成
func (et *ExecutionTracker) Complete(workflowID uint) {
	et.mu.Lock()
	defer et.mu.Unlock()

	if cancel, exists := et.running[workflowID]; exists {
		cancel() // 取消context
		delete(et.running, workflowID)
		logger.Infof("✅ 工作流执行已清理 [WorkflowID=%d]", workflowID)
	}
}

// IsRunning 检查工作流是否正在运行
func (et *ExecutionTracker) IsRunning(workflowID uint) bool {
	et.mu.RLock()
	defer et.mu.RUnlock()

	_, exists := et.running[workflowID]
	return exists
}

// GetRunningCount 获取正在运行的工作流数量
func (et *ExecutionTracker) GetRunningCount() int {
	et.mu.RLock()
	defer et.mu.RUnlock()

	return len(et.running)
}

// Cancel 取消指定工作流的执行
func (et *ExecutionTracker) Cancel(workflowID uint) bool {
	et.mu.Lock()
	defer et.mu.Unlock()

	cancel, exists := et.running[workflowID]
	if !exists {
		return false
	}

	cancel()
	delete(et.running, workflowID)
	logger.Infof("⚠️  工作流执行已取消 [WorkflowID=%d]", workflowID)
	return true
}

// CancelAll 取消所有正在运行的工作流
func (et *ExecutionTracker) CancelAll() int {
	et.mu.Lock()
	defer et.mu.Unlock()

	count := 0
	for workflowID, cancel := range et.running {
		cancel()
		delete(et.running, workflowID)
		logger.Infof("⚠️  工作流执行已取消 [WorkflowID=%d]", workflowID)
		count++
	}

	return count
}
