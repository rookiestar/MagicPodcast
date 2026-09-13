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

var importTaskRegistrationMu sync.Mutex

// CreateImportTask 建立任务记录并登记为活动任务。
func CreateImportTask(db *gorm.DB, fileName string, total int) (*models.ImportTask, error) {
	importTaskRegistrationMu.Lock()
	defer importTaskRegistrationMu.Unlock()
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
	importTaskRegistrationMu.Lock()
	defer importTaskRegistrationMu.Unlock()
	defer ActiveImportTasks.Delete(task.ID)

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
	importTaskRegistrationMu.Lock()
	defer importTaskRegistrationMu.Unlock()
	if task.Status != models.ImportTaskStatusRunning {
		return
	}
	if _, active := ActiveImportTasks.Load(task.ID); active {
		return
	}
	task.Status = models.ImportTaskStatusInterrupted
	// 中断状态落库，避免每次读取重复推导。
	_ = db.Model(&models.ImportTask{}).Where("id = ? AND status = ?", task.ID, models.ImportTaskStatusRunning).
		Update("status", models.ImportTaskStatusInterrupted).Error
	// Finalization may have completed after the caller's initial read.
	_ = db.First(task, task.ID).Error
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
	entries     []ImportEntryResult
	summary     *SyncSummary
}

// InitializeImportResults saves the submitted scope before any subscription writes.
func InitializeImportResults(reporter ProgressReporter, outlines []opml.Outline) error {
	r, ok := reporter.(*taskProgressReporter)
	if !ok {
		return nil
	}
	for _, outline := range dedupeImportOutlines(outlines) {
		r.entries = append(r.entries, ImportEntryResult{Title: outline.GetTitle(), FeedURL: outline.XMLURL, Outcome: "unprocessed", Detail: "尚无已完成结果，重试前将重新核对本地记录"})
	}
	return r.persistEntries(0)
}

func (r *taskProgressReporter) persistEntries(processed int) error {
	raw, err := json.Marshal(r.entries)
	if err != nil {
		return err
	}
	counts := map[string]interface{}{"result_json": string(raw), "processed": processed, "total": len(r.entries)}
	for _, key := range []string{"success_count", "pending_count", "conflict_count", "merged_count", "unchanged_count", "skipped_count", "failed_count"} {
		counts[key] = 0
	}
	increment := func(key string) { counts[key] = counts[key].(int) + 1 }
	for _, entry := range r.entries {
		switch entry.Outcome {
		case ImportOutcomeNew, ImportOutcomeUpdated:
			increment("success_count")
		case ImportOutcomeMerged:
			increment("success_count")
			increment("merged_count")
		case ImportOutcomePending:
			increment("pending_count")
		case ImportOutcomeConflict, ImportOutcomeDeleted:
			increment("conflict_count")
		case ImportOutcomeUnchanged:
			increment("unchanged_count")
		case ImportOutcomeSkipped:
			increment("skipped_count")
		case ImportOutcomeFailed:
			increment("failed_count")
		}
	}
	return r.db.Model(&models.ImportTask{}).Where("id = ?", r.taskID).Updates(counts).Error
}

func (r *taskProgressReporter) RecordImportResult(entry ImportEntryResult, processed int) error {
	for i := range r.entries {
		if r.entries[i].FeedURL == entry.FeedURL {
			r.entries[i] = entry
			break
		}
	}
	return r.persistEntries(processed)
}

// PublishImportSummary is called only after successful terminal persistence.
func PublishImportSummary(reporter ProgressReporter) {
	if r, ok := reporter.(*taskProgressReporter); ok && r.summary != nil {
		r.inner.ReportSummary(r.summary)
	}
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
	r.summary = summary
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
	if err := InitializeImportResults(wrapped, outlines); err != nil {
		ActiveImportTasks.Delete(task.ID)
		return task, nil, err
	}
	result, runErr := s.ImportOPMLOutlines(outlines, wrapped, config, decisions)
	if finErr := FinalizeImportTask(db, task, result, runErr); finErr != nil {
		logger.Errorf("保存导入任务终态失败: task=%d err=%v", task.ID, finErr)
		return task, result, finErr
	}
	if runErr == nil {
		PublishImportSummary(wrapped)
	}
	return task, result, runErr
}
