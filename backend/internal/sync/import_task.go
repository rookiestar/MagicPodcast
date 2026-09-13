package sync

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"magicpodcast/internal/logger"
	"magicpodcast/internal/models"
	"magicpodcast/internal/opml"

	"gorm.io/gorm"
)

// ActiveImportTasks 记录本进程正在执行的导入任务 ID。进程重启后注册表为
// 空：仍处于 running 的任务记录由读取方判定为 interrupted，不自动重新
// 执行任何写入（#398 R4）。
var ActiveImportTasks sync.Map

// CreateImportTask 建立任务记录并登记为活动任务。
func CreateImportTask(db *gorm.DB, fileName string, total int) (*models.ImportTask, error) {
	task := &models.ImportTask{
		Status:    models.ImportTaskStatusRunning,
		FileName:  fileName,
		Total:     total,
		StartedAt: time.Now(),
	}
	if err := db.Create(task).Error; err != nil {
		return nil, err
	}
	ActiveImportTasks.Store(task.ID, struct{}{})
	return task, nil
}

// FinalizeImportTask 先可靠保存终态与完整逐条结果，再由调用方发送最终
// 通知；传输层错误不得覆盖已保存的业务结果。
func FinalizeImportTask(db *gorm.DB, task *models.ImportTask, result *SyncResult, runErr error) error {
	if task == nil {
		return nil
	}
	ActiveImportTasks.Delete(task.ID)

	task.Status = models.ImportTaskStatusCompleted
	if runErr != nil {
		task.Status = models.ImportTaskStatusFailed
		task.ErrorMessage = runErr.Error()
	}
	if result != nil {
		task.Total = result.TotalPodcasts
		task.SuccessCount = result.SuccessPodcasts
		task.PendingCount = result.StubPodcasts
		task.ConflictCount = result.ConflictPodcasts
		task.MergedCount = result.MergedPodcasts
		task.UnchangedCount = result.UnchangedPodcasts
		task.SkippedCount = result.SkippedPodcasts
		task.FailedCount = result.FailedPodcasts
		if entries, err := json.Marshal(result.Entries); err == nil {
			task.ResultJSON = string(entries)
		} else {
			logger.Warnf("序列化导入逐条结果失败: %v", err)
		}
		task.Processed = len(result.Entries)
	}
	now := time.Now()
	task.FinishedAt = &now
	return db.Model(&models.ImportTask{}).Where("id = ?", task.ID).Updates(map[string]interface{}{
		"status":          task.Status,
		"error_message":   task.ErrorMessage,
		"total":           task.Total,
		"processed":       task.Processed,
		"success_count":   task.SuccessCount,
		"pending_count":   task.PendingCount,
		"conflict_count":  task.ConflictCount,
		"merged_count":    task.MergedCount,
		"unchanged_count": task.UnchangedCount,
		"skipped_count":   task.SkippedCount,
		"failed_count":    task.FailedCount,
		"result_json":     task.ResultJSON,
		"finished_at":     task.FinishedAt,
	}).Error
}

// GetImportTask 读取任务并推导中断状态：running 但不在本进程注册表中的
// 任务说明进程已重启，明确标记为 interrupted/待继续，不自动重跑。
func GetImportTask(db *gorm.DB, id uint) (*models.ImportTask, []ImportEntryResult, error) {
	var task models.ImportTask
	if err := db.First(&task, id).Error; err != nil {
		return nil, nil, err
	}
	applyDerivedStatus(db, &task)
	entries := decodeImportEntries(task.ResultJSON)
	return &task, entries, nil
}

// GetLatestImportTask 返回最近一次导入任务，用于页面刷新/断线后恢复。
func GetLatestImportTask(db *gorm.DB) (*models.ImportTask, []ImportEntryResult, error) {
	var task models.ImportTask
	if err := db.Order("id DESC").First(&task).Error; err != nil {
		return nil, nil, err
	}
	applyDerivedStatus(db, &task)
	entries := decodeImportEntries(task.ResultJSON)
	return &task, entries, nil
}

func applyDerivedStatus(db *gorm.DB, task *models.ImportTask) {
	if task.Status != models.ImportTaskStatusRunning {
		return
	}
	if _, active := ActiveImportTasks.Load(task.ID); active {
		return
	}
	task.Status = models.ImportTaskStatusInterrupted
	// 中断状态落库，避免每次读取重复推导。
	_ = db.Model(&models.ImportTask{}).Where("id = ?", task.ID).
		Update("status", models.ImportTaskStatusInterrupted).Error
}

func decodeImportEntries(raw string) []ImportEntryResult {
	if raw == "" {
		return []ImportEntryResult{}
	}
	var entries []ImportEntryResult
	if err := json.Unmarshal([]byte(raw), &entries); err != nil {
		return []ImportEntryResult{}
	}
	return entries
}

// taskProgressReporter 把导入进度持久化到任务记录：页面断线后仍可查询
// 同一任务的已提交条目。持久化按时间节流，避免高频写放大。
type taskProgressReporter struct {
	inner       ProgressReporter
	db          *gorm.DB
	taskID      uint
	mu          sync.Mutex
	lastPersist time.Time
}

// NewTaskProgressReporter 用任务记录包装进度报告器。
func NewTaskProgressReporter(inner ProgressReporter, db *gorm.DB, taskID uint) ProgressReporter {
	if db == nil || taskID == 0 {
		return inner
	}
	return &taskProgressReporter{inner: inner, db: db, taskID: taskID}
}

func (r *taskProgressReporter) persistProgress(current, total int) {
	r.mu.Lock()
	if time.Since(r.lastPersist) < time.Second {
		r.mu.Unlock()
		return
	}
	r.lastPersist = time.Now()
	r.mu.Unlock()
	_ = r.db.Model(&models.ImportTask{}).Where("id = ?", r.taskID).
		Updates(map[string]interface{}{"processed": current, "total": total}).Error
}

func (r *taskProgressReporter) Report(message string) {
	r.inner.Report(message)
}

func (r *taskProgressReporter) ReportSuccess(message string) {
	r.inner.ReportSuccess(message)
}

func (r *taskProgressReporter) ReportError(message string) {
	r.inner.ReportError(message)
}

func (r *taskProgressReporter) ReportProgress(current, total int, message string) {
	r.persistProgress(current, total)
	r.inner.ReportProgress(current, total, message)
}

func (r *taskProgressReporter) ReportSkip(reason SkipReason, message string) {
	r.inner.ReportSkip(reason, message)
}

func (r *taskProgressReporter) ReportSummary(summary *SyncSummary) {
	r.inner.ReportSummary(summary)
}

func (r *taskProgressReporter) Close() {
	r.inner.Close()
}

// RunImportTask 是「创建任务 → 执行导入 → 保存终态」的标准编排，供两个
// 导入入口复用。业务持久化与 SSE 写出彼此独立：连接失败不影响任务结果。
func (s *Service) RunImportTask(db *gorm.DB, fileName string, outlines []opml.Outline, reporter ProgressReporter, config ImportConfig, decisions map[string]string) (*models.ImportTask, *SyncResult, error) {
	task, err := CreateImportTask(db, fileName, len(outlines))
	if err != nil {
		return nil, nil, fmt.Errorf("创建导入任务失败: %w", err)
	}
	wrapped := NewTaskProgressReporter(reporter, db, task.ID)
	result, runErr := s.ImportOPMLOutlines(outlines, wrapped, config, decisions)
	if finErr := FinalizeImportTask(db, task, result, runErr); finErr != nil {
		logger.Errorf("保存导入任务终态失败: task=%d err=%v", task.ID, finErr)
	}
	return task, result, runErr
}
