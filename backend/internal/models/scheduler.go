package models

import (
	"time"
)

// SchedulerRunStatus 调度器运行状态
type SchedulerRunStatus string

const (
	SchedulerRunStatusSuccess SchedulerRunStatus = "success"
	SchedulerRunStatusFailed  SchedulerRunStatus = "failed"
	SchedulerRunStatusSkipped SchedulerRunStatus = "skipped"
)

// SchedulerRun 调度器运行记录
type SchedulerRun struct {
	ID          uint               `gorm:"primarykey" json:"id"`
	CreatedAt   time.Time          `json:"created_at"`
	WorkflowID  uint               `gorm:"not null;index" json:"workflow_id"`
	Workflow    Workflow           `gorm:"-" json:"workflow,omitempty"`
	Status      SchedulerRunStatus `gorm:"not null;index" json:"status"`
	ScheduledAt time.Time          `gorm:"not null;index" json:"scheduled_at"` // 计划执行时间
	StartedAt   *time.Time         `json:"started_at,omitempty"`               // 实际开始时间
	CompletedAt *time.Time         `json:"completed_at,omitempty"`             // 完成时间
	Duration    *int               `json:"duration,omitempty"`                 // 执行时长（毫秒）
	Reason      string             `gorm:"size:500" json:"reason,omitempty"`   // 状态原因（如：跳过原因、失败原因）
	JobID       *uint              `gorm:"index" json:"job_id,omitempty"`      // 关联的Job ID
}

// TableName 指定表名
func (SchedulerRun) TableName() string {
	return "scheduler_runs"
}
