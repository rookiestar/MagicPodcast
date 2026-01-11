package models

import (
	"database/sql/driver"
	"encoding/json"
	"time"

	"gorm.io/gorm"
)

// WorkflowScopeType 工作流范围类型
type WorkflowScopeType string

const (
	ScopeTypeSpecificPodcasts WorkflowScopeType = "specific_podcasts" // 指定节目
	ScopeTypeAllSubscribed    WorkflowScopeType = "all_subscribed"    // 全部订阅
	ScopeTypeCustomSources    WorkflowScopeType = "custom_sources"    // 自定义源
)

// WorkflowStatus 工作流状态
type WorkflowStatus string

const (
	WorkflowStatusEnabled  WorkflowStatus = "enabled"
	WorkflowStatusDisabled WorkflowStatus = "disabled"
)

// ScopeConfig 范围配置
type ScopeConfig struct {
	PodcastIDs []int    `json:"podcast_ids,omitempty"` // 指定节目的ID列表
	CustomURLs []string `json:"custom_urls,omitempty"` // 自定义RSS源URL列表
}

// Scan 实现 sql.Scanner 接口
func (s *ScopeConfig) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return nil
	}
	return json.Unmarshal(bytes, s)
}

// Value 实现 driver.Valuer 接口
func (s ScopeConfig) Value() (driver.Value, error) {
	if len(s.PodcastIDs) == 0 && len(s.CustomURLs) == 0 {
		return nil, nil
	}
	return json.Marshal(s)
}

// RulesConfig 规则配置
type RulesConfig struct {
	TimeRange    int    `json:"time_range,omitempty"`    // 时间范围（天），0表示不限制
	MinDuration  int    `json:"min_duration,omitempty"`  // 最小时长（秒），0表示不限制
	MaxResults   int    `json:"max_results,omitempty"`   // 最大结果数，0表示不限制
	Keywords     string `json:"keywords,omitempty"`      // 关键词过滤（逗号分隔）
	ExcludeWords string `json:"exclude_words,omitempty"` // 排除词（逗号分隔）
}

// Scan 实现 sql.Scanner 接口
func (r *RulesConfig) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return nil
	}
	return json.Unmarshal(bytes, r)
}

// Value 实现 driver.Valuer 接口
func (r RulesConfig) Value() (driver.Value, error) {
	return json.Marshal(r)
}

// Workflow 工作流模型
type Workflow struct {
	BaseModel

	Name        string           `gorm:"size:200;not null" json:"name"`
	Description string           `gorm:"size:1000" json:"description"`
	Schedule    string           `gorm:"size:100;not null" json:"schedule"` // cron表达式
	ScopeType   WorkflowScopeType `gorm:"size:50;not null;index" json:"scope_type"`
	ScopeConfig ScopeConfig      `gorm:"type:json" json:"scope_config"`
	RulesConfig RulesConfig      `gorm:"type:json" json:"rules_config"`
	IsEnabled   bool             `gorm:"index;not null" json:"is_enabled"`
	LastJobID   *uint            `gorm:"index" json:"last_job_id,omitempty"`
	LastJob     *Job             `gorm:"-" json:"last_job,omitempty"` // 不自动迁移，手动加载
	Jobs        []Job            `gorm:"foreignKey:WorkflowID" json:"jobs,omitempty"`
}

// TableName 指定表名
func (Workflow) TableName() string {
	return "workflows"
}

// JobStatus 任务状态
type JobStatus string

const (
	JobStatusPending   JobStatus = "pending"
	JobStatusRunning   JobStatus = "running"
	JobStatusCompleted JobStatus = "completed"
	JobStatusFailed    JobStatus = "failed"
	JobStatusCancelled JobStatus = "cancelled"
)

// Job 任务模型（工作流的单次执行）
type Job struct {
	BaseModel
	gorm.DeletedAt `gorm:"index" json:"-"`

	WorkflowID        uint           `gorm:"index;not null" json:"workflow_id"`
	Workflow          Workflow       `gorm:"foreignKey:WorkflowID" json:"workflow,omitempty"`
	Status            JobStatus      `gorm:"size:20;index;not null" json:"status"`
	StartTime         *time.Time     `json:"start_time,omitempty"`
	EndTime           *time.Time     `json:"end_time,omitempty"`
	PodcastsProcessed int            `gorm:"default:0" json:"podcasts_processed"`
	EpisodesFound     int            `gorm:"default:0" json:"episodes_found"`
	EpisodesCreated   int            `gorm:"default:0" json:"episodes_created"`
	ErrorCount        int            `gorm:"default:0" json:"error_count"`
	TriggeredBy       string         `gorm:"size:50;default:cron" json:"triggered_by"` // cron/manual
	Executions        []JobExecution `gorm:"foreignKey:JobID" json:"executions,omitempty"`
}

// TableName 指定表名
func (Job) TableName() string {
	return "jobs"
}

// ExecutionStatus 执行状态
type ExecutionStatus string

const (
	ExecutionStatusPending ExecutionStatus = "pending"
	ExecutionStatusRunning ExecutionStatus = "running"
	ExecutionStatusSuccess ExecutionStatus = "success"
	ExecutionStatusFailed  ExecutionStatus = "failed"
	ExecutionStatusSkipped ExecutionStatus = "skipped"
)

// JobExecution 任务执行详情（单个节目的处理结果）
type JobExecution struct {
	BaseModel
	gorm.DeletedAt `gorm:"index" json:"-"`

	ID              uint            `gorm:"primarykey" json:"id"`
	JobID           uint            `gorm:"index;not null" json:"job_id"`
	Job             Job             `gorm:"foreignKey:JobID" json:"job,omitempty"`
	PodcastID       *uint           `gorm:"index" json:"podcast_id,omitempty"`
	Podcast         Podcast         `gorm:"foreignKey:PodcastID" json:"podcast,omitempty"`
	PodcastTitle    string          `gorm:"size:500" json:"podcast_title,omitempty"`
	PodcastFeedURL  string          `gorm:"size:1000" json:"podcast_feed_url,omitempty"`
	Status          ExecutionStatus `gorm:"size:20;not null" json:"status"`
	EpisodesFound   int             `gorm:"default:0" json:"episodes_found"`
	EpisodesCreated int             `gorm:"default:0" json:"episodes_created"`
	ErrorMessage    string          `gorm:"type:text" json:"error_message,omitempty"`
	LogInfo         string          `gorm:"type:text" json:"log_info,omitempty"`
	ProcessingTime  int             `gorm:"default:0" json:"processing_time"` // 毫秒
}

// TableName 指定表名
func (JobExecution) TableName() string {
	return "job_executions"
}

// Report 抓取报告模型（保留以兼容，暂时不使用）
type Report struct {
	BaseModel

	ExecutionID uint         `gorm:"not null;index" json:"execution_id"`
	Execution   JobExecution `gorm:"foreignKey:ExecutionID" json:"execution,omitempty"`
	ReportBody  string       `gorm:"type:text" json:"report_body"`
}

// TableName 指定表名
func (Report) TableName() string {
	return "reports"
}

// JSONMap 用于存储 JSON 数据的 map 类型
type JSONMap map[string]interface{}

// Scan 实现 sql.Scanner 接口
func (j *JSONMap) Scan(value interface{}) error {
	if value == nil {
		*j = make(JSONMap)
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return nil
	}
	return json.Unmarshal(bytes, j)
}

// Value 实现 driver.Valuer 接口
func (j JSONMap) Value() (driver.Value, error) {
	if len(j) == 0 {
		return nil, nil
	}
	return json.Marshal(j)
}
