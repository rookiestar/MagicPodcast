package workflow

import (
	"context"
	"testing"
	"time"
)

// TestExecutionTracker_TryStart 测试启动工作流执行
func TestExecutionTracker_TryStart(t *testing.T) {
	tracker := NewExecutionTracker()

	// 第一次启动应该成功
	ctx, started := tracker.TryStart(1, 5*time.Minute)
	if !started {
		t.Error("第一次启动应该成功")
	}
	if ctx == nil {
		t.Error("context不应该为nil")
	}

	// 第二次启动应该失败（已有任务在运行）
	ctx2, started2 := tracker.TryStart(1, 5*time.Minute)
	if started2 {
		t.Error("第二次启动应该失败（任务已在运行）")
	}
	if ctx2 != nil {
		t.Error("第二次启动context应该为nil")
	}

	// 清理
	tracker.Complete(1)
}

// TestExecutionTracker_Complete 测试完成工作流执行
func TestExecutionTracker_Complete(t *testing.T) {
	tracker := NewExecutionTracker()

	// 启动任务
	ctx, started := tracker.TryStart(1, 5*time.Minute)
	if !started {
		t.Fatal("启动任务应该成功")
	}
	if ctx == nil {
		t.Fatal("context不应该为nil")
	}

	// 完成任务
	tracker.Complete(1)

	// 再次启动应该成功（任务已完成）
	ctx2, started2 := tracker.TryStart(1, 5*time.Minute)
	if !started2 {
		t.Error("任务完成后再次启动应该成功")
	}
	if ctx2 == nil {
		t.Error("context不应该为nil")
	}

	// 清理
	tracker.Complete(1)
}

// TestExecutionTracker_IsRunning 测试检查任务是否在运行
func TestExecutionTracker_IsRunning(t *testing.T) {
	tracker := NewExecutionTracker()

	// 初始状态：未运行
	if tracker.IsRunning(1) {
		t.Error("初始状态应该是未运行")
	}

	// 启动任务
	tracker.TryStart(1, 5*time.Minute)

	// 运行中
	if !tracker.IsRunning(1) {
		t.Error("启动后应该是运行中")
	}

	// 完成任务
	tracker.Complete(1)

	// 完成后：未运行
	if tracker.IsRunning(1) {
		t.Error("完成后应该是未运行")
	}
}

// TestExecutionTracker_GetRunningCount 测试获取运行任务数量
func TestExecutionTracker_GetRunningCount(t *testing.T) {
	tracker := NewExecutionTracker()

	// 初始数量为0
	if count := tracker.GetRunningCount(); count != 0 {
		t.Errorf("初始数量应该为0，实际为%d", count)
	}

	// 启动3个任务
	tracker.TryStart(1, 5*time.Minute)
	tracker.TryStart(2, 5*time.Minute)
	tracker.TryStart(3, 5*time.Minute)

	if count := tracker.GetRunningCount(); count != 3 {
		t.Errorf("运行数量应该为3，实际为%d", count)
	}

	// 完成1个任务
	tracker.Complete(1)

	if count := tracker.GetRunningCount(); count != 2 {
		t.Errorf("完成1个后数量应该为2，实际为%d", count)
	}

	// 清理
	tracker.Complete(2)
	tracker.Complete(3)
}

// TestExecutionTracker_Cancel 测试取消任务
func TestExecutionTracker_Cancel(t *testing.T) {
	tracker := NewExecutionTracker()

	// 启动任务
	ctx, started := tracker.TryStart(1, 5*time.Minute)
	if !started {
		t.Fatal("启动任务应该成功")
	}

	// 取消任务
	cancelled := tracker.Cancel(1)
	if !cancelled {
		t.Error("取消任务应该成功")
	}

	// 检查context是否已取消
	select {
	case <-ctx.Done():
		// context已取消，符合预期
	default:
		t.Error("context应该已被取消")
	}

	// 再次取消应该失败（任务已不存在）
	cancelled2 := tracker.Cancel(1)
	if cancelled2 {
		t.Error("取消不存在的任务应该失败")
	}
}

// TestExecutionTracker_CancelAll 测试取消所有任务
func TestExecutionTracker_CancelAll(t *testing.T) {
	tracker := NewExecutionTracker()

	// 启动3个任务
	ctx1, _ := tracker.TryStart(1, 5*time.Minute)
	ctx2, _ := tracker.TryStart(2, 5*time.Minute)
	ctx3, _ := tracker.TryStart(3, 5*time.Minute)

	// 取消所有任务
	count := tracker.CancelAll()
	if count != 3 {
		t.Errorf("应该取消3个任务，实际取消%d个", count)
	}

	// 检查所有context是否已取消
	contexts := []context.Context{ctx1, ctx2, ctx3}
	for i, ctx := range contexts {
		select {
		case <-ctx.Done():
			// context已取消，符合预期
		default:
			t.Errorf("context[%d]应该已被取消", i)
		}
	}

	// 运行数量应该为0
	if count := tracker.GetRunningCount(); count != 0 {
		t.Errorf("取消所有后运行数量应该为0，实际为%d", count)
	}
}

// TestExecutionTracker_ConcurrentAccess 测试并发访问
func TestExecutionTracker_ConcurrentAccess(t *testing.T) {
	tracker := NewExecutionTracker()

	// 并发启动多个任务
	done := make(chan bool)
	for i := 1; i <= 10; i++ {
		go func(id int) {
			ctx, started := tracker.TryStart(uint(id), 5*time.Minute)
			if started && ctx != nil {
				time.Sleep(10 * time.Millisecond)
				tracker.Complete(uint(id))
			}
			done <- true
		}(i)
	}

	// 等待所有goroutine完成
	for i := 0; i < 10; i++ {
		<-done
	}

	// 最终应该没有任务在运行
	if count := tracker.GetRunningCount(); count != 0 {
		t.Errorf("并发测试后运行数量应该为0，实际为%d", count)
	}
}

// TestExecutionTracker_ContextCancellation 测试context取消
func TestExecutionTracker_ContextCancellation(t *testing.T) {
	tracker := NewExecutionTracker()

	// 启动一个短超时的任务
	ctx, started := tracker.TryStart(1, 10*time.Millisecond)
	if !started {
		t.Fatal("启动任务应该成功")
	}
	if ctx == nil {
		t.Fatal("context不应该为nil")
	}

	// 等待超时
	time.Sleep(20 * time.Millisecond)

	// 检查context是否已超时
	select {
	case <-ctx.Done():
		if ctx.Err() != context.DeadlineExceeded {
			t.Errorf("context应该因超时而取消，实际错误: %v", ctx.Err())
		}
	default:
		t.Error("context应该已超时")
	}

	// 清理
	tracker.Complete(1)
}
