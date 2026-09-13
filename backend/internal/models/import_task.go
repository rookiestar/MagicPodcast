package models

import "time"

// 导入任务状态。
const (
	ImportTaskStatusRunning     = "running"
	ImportTaskStatusCompleted   = "completed"
	ImportTaskStatusFailed      = "failed"
	ImportTaskStatusInterrupted = "interrupted"
)

// ImportTask 一次 OPML 导入的可恢复任务记录。它是最小任务元数据与逐条
// 业务结果账本：任务终态与结果先可靠保存，再发送最终通知；进程重启后
// 处于 running 的记录由读取方按注册表判定为 interrupted（#398 R4/R12）。
type ImportTask struct {
	BaseModel

	Status         string `gorm:"size:20;not null;default:'running'" json:"status"`
	FileName       string `gorm:"size:255" json:"file_name"`
	Total          int    `json:"total"`
	Processed      int    `json:"processed"`
	SuccessCount   int    `json:"success_count"`
	PendingCount   int    `json:"pending_count"`
	ConflictCount  int    `json:"conflict_count"`
	MergedCount    int    `json:"merged_count"`
	UnchangedCount int    `json:"unchanged_count"`
	SkippedCount   int    `json:"skipped_count"`
	FailedCount    int    `json:"failed_count"`
	// ResultJSON 保存完整逐条导入结果，不受日志截断影响。
	ResultJSON   string `gorm:"type:text" json:"-"`
	ErrorMessage string `json:"error_message"`
	StartedAt    time.Time
	FinishedAt   *time.Time
}

// TableName 指定表名。
func (ImportTask) TableName() string {
	return "import_tasks"
}
