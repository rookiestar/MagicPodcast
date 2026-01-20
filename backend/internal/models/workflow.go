package models

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
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
	TimeRange     int    `json:"time_range,omitempty"`      // 时间范围（天），0表示不限制，-1表示"自上次更新"
	TimeRangeMode string `json:"time_range_mode,omitempty"` // "days" | "since_last_update" | "all_time"
	MinDuration   int    `json:"min_duration,omitempty"`    // 最小时长（秒），0表示不限制
	MaxResults    int    `json:"max_results,omitempty"`     // 最大结果数，0表示不限制
	Keywords      string `json:"keywords,omitempty"`        // 关键词过滤（逗号分隔）
	ExcludeWords  string `json:"exclude_words,omitempty"`   // 排除词（逗号分隔）
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
	LastJob     *Job             `gorm:"-" json:"last_job,omitempty"` // 不自动迁移，通过关联查询加载
	Jobs        []Job            `gorm:"foreignKey:WorkflowID" json:"jobs,omitempty"`

	// 调度状态持久化字段（新增）
	LastExecutionAt *time.Time `gorm:"index" json:"last_execution_at,omitempty"` // 上次执行时间
	NextRunAt       *time.Time `gorm:"index" json:"next_run_at,omitempty"`       // 下次执行时间
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
	EpisodesMatched   int            `gorm:"default:0" json:"episodes_matched"`
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

	JobID           uint            `gorm:"index;not null" json:"job_id"`
	Job             Job             `gorm:"-" json:"job,omitempty"` // 不持久化到数据库
	PodcastID       *uint           `gorm:"index" json:"podcast_id,omitempty"`
	Podcast         Podcast         `gorm:"-" json:"podcast,omitempty"` // 不持久化到数据库
	PodcastTitle    string          `gorm:"size:500" json:"podcast_title,omitempty"`
	PodcastFeedURL  string          `gorm:"size:1000" json:"podcast_feed_url,omitempty"`
	Status          ExecutionStatus `gorm:"size:20;not null" json:"status"`
	EpisodesFound   int             `gorm:"default:0" json:"episodes_found"`
	EpisodesCreated int             `gorm:"default:0" json:"episodes_created"`
	EpisodesMatched int             `gorm:"default:0" json:"episodes_matched"`
	ErrorMessage    string          `gorm:"type:text" json:"error_message,omitempty"`
	LogInfo         string          `gorm:"type:text" json:"log_info,omitempty"`
	ProcessingTime  int             `gorm:"default:0" json:"processing_time"` // 毫秒
}

// TableName 指定表名
func (JobExecution) TableName() string {
	return "job_executions"
}

// Report 工作流执行报告
type Report struct {
	BaseModel

	JobID         uint      `gorm:"not null;uniqueIndex" json:"job_id"`    // 关联的Job（一对一）
	Job           Job       `gorm:"-" json:"job,omitempty"`              // 关联的Job（不持久化）

	Title         string    `gorm:"size:255;not null" json:"title"`          // 报告标题
	Content       string    `gorm:"type:text;not null" json:"content"`       // Markdown内容
	Summary       string    `gorm:"type:text" json:"summary"`                // 简要摘要

	// 统计字段
	EpisodesCount int       `gorm:"default:0" json:"episodes_count"`  // 包含的episode数
	PodcastsCount int       `gorm:"default:0" json:"podcasts_count"`  // 包含的podcast数
	MatchedCount   int       `gorm:"default:0" json:"matched_count"`    // 匹配的单集数

	// 时间范围信息
	TimeRangeStart time.Time `json:"time_range_start"` // 扫描时间范围起始时间
	TimeRangeEnd   time.Time `json:"time_range_end"`   // 扫描时间范围结束时间
	TimeRangeMode  string    `gorm:"size:20" json:"time_range_mode"` // daily | manual

	GeneratedAt   time.Time `gorm:"not null" json:"generated_at"`       // 生成时间
	Format        string    `gorm:"size:20;default:'markdown'" json:"format"`    // 报告格式
	FileSize      int       `gorm:"default:0" json:"file_size"`         // 内容大小（字节）
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

// ValidateCron 验证Cron表达式是否有效
func ValidateCron(schedule string) error {
	if schedule == "" {
		return fmt.Errorf("cron表达式不能为空")
	}

	// 使用robfig/cron解析器验证
	// 支持6位表达式（秒 分 时 日 月 周）
	parser := cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	_, err := parser.Parse(schedule)
	if err != nil {
		return fmt.Errorf("无效的cron表达式: %w", err)
	}

	return nil
}

// BeforeSave GORM hook - 保存前验证
func (w *Workflow) BeforeSave(tx *gorm.DB) error {
	// 如果有schedule，验证cron表达式
	if w.Schedule != "" {
		if err := ValidateCron(w.Schedule); err != nil {
			return err
		}
	}
	return nil
}

// GetNextRunTime 获取下次执行时间
func (w *Workflow) GetNextRunTime() (time.Time, error) {
	if w.Schedule == "" {
		return time.Time{}, fmt.Errorf("未配置schedule")
	}

	parser := cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	schedule, err := parser.Parse(w.Schedule)
	if err != nil {
		return time.Time{}, err
	}

	return schedule.Next(time.Now()), nil
}

// GetScheduleDescription 获取Cron表达式的可读描述
func (w *Workflow) GetScheduleDescription() string {
	if w.Schedule == "" {
		return "未配置"
	}

	parts := strings.Fields(w.Schedule)
	if len(parts) != 6 {
		return "格式错误"
	}

	// 简化版描述生成
	sec := parts[0]
	min := parts[1]
	hour := parts[2]
	day := parts[3]
	month := parts[4]
	dow := parts[5]

	var desc strings.Builder

	// 处理常见模式
	if sec == "0" && min == "0" && hour != "*" {
		desc.WriteString(fmt.Sprintf("每天 %s 点", hour))
		if min == "0" && hour == "*" {
			desc.WriteString("每小时")
		}
	} else if sec == "0" && min == "0" && hour == "*" && day == "*" && month == "*" && dow == "*" {
		desc.WriteString("每小时")
	} else if sec == "0" && min == "0" && hour == "*" && day == "*" && month == "*" && dow != "*" {
		desc.WriteString(fmt.Sprintf("每周 %s", dow))
	} else {
		desc.WriteString(w.Schedule)
	}

	return desc.String()
}
