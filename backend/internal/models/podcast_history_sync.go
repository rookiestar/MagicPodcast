package models

import "time"

// 节目历史同步任务状态。
// pending/queued/running 为活动态；completed/partial/failed 为终态。
// partial 表示本次来源范围未全部处理完成（可继续/重试），completed 表示
// 来源当前可获取范围已全部成功入库，不承诺源站未暴露的历史。
const (
	HistorySyncStatusPending   = "pending"
	HistorySyncStatusQueued    = "queued"
	HistorySyncStatusRunning   = "running"
	HistorySyncStatusCompleted = "completed"
	HistorySyncStatusPartial   = "partial"
	HistorySyncStatusFailed    = "failed"
)

// 节目历史同步任务触发来源。
const (
	HistorySyncTriggerWorkflow = "workflow" // 工作流覆盖入口（追加/新建/编辑/启用/导入/周期补偿/自定义源落地）
	HistorySyncTriggerManual   = "manual"   // 用户手动同步/重试
)

// HistorySyncActiveStatuses 返回活动态任务状态集合。
func HistorySyncActiveStatuses() []string {
	return []string{HistorySyncStatusPending, HistorySyncStatusQueued, HistorySyncStatusRunning}
}

// IsHistorySyncActiveStatus 判断任务状态是否为活动态。
func IsHistorySyncActiveStatus(status string) bool {
	switch status {
	case HistorySyncStatusPending, HistorySyncStatusQueued, HistorySyncStatusRunning:
		return true
	}
	return false
}

// PodcastHistorySyncTask 节目级持久历史同步任务。
// 一个节目同一时刻最多存在一个活动态任务（部分唯一索引保证）；终态任务
// 保留作为历史完成记录，最新一条代表节目当前的历史同步状态。
type PodcastHistorySyncTask struct {
	BaseModel

	PodcastID uint   `gorm:"not null;index:idx_history_sync_tasks_podcast" json:"podcast_id"`
	Trigger   string `gorm:"size:20;not null" json:"trigger"`
	Status    string `gorm:"size:20;not null;default:'pending'" json:"status"`

	Attempts   int        `json:"attempts"` // 已开始的执行次数
	NextRetryAt *time.Time `json:"next_retry_at"`

	// TotalKnown 为来源可提供的条目总数；nil 表示总量未知，此时只展示已处理数量。
	TotalKnown *int `json:"total_known"`

	ProcessedCount int `json:"processed_count"`
	CreatedCount   int `json:"created_count"`
	UpdatedCount   int `json:"updated_count"`
	FailedCount    int `json:"failed_count"`

	ErrorMessage string `gorm:"type:text" json:"error_message"`
	// SourceNote 记录来源覆盖说明：合法空源、来源历史完整性未知或部分完成等信息。
	SourceNote string `gorm:"type:text" json:"source_note"`

	StartedAt  *time.Time `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at"`
}

// TableName 指定表名。
func (PodcastHistorySyncTask) TableName() string {
	return "podcast_history_sync_tasks"
}
