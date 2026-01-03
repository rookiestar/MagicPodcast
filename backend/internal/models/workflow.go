package models

import (
	"database/sql/driver"
	"encoding/json"
	"time"
)

// Workflow 工作流模型
type Workflow struct {
	BaseModel

	Title       string `gorm:"size:255;not null" json:"title"`       // 工作流标题
	Description string `gorm:"type:text" json:"description"`         // 工作流描述
	Conditions  string `gorm:"type:text" json:"conditions"`          // 筛选条件（JSON 格式）
	IsEnabled   bool   `gorm:"default:true" json:"is_enabled"`       // 是否启用

	// 关联关系
	Jobs []Job `gorm:"foreignKey:WorkflowID;constraint:OnDelete:CASCADE" json:"jobs,omitempty"`
}

// TableName 指定表名
func (Workflow) TableName() string {
	return "workflows"
}

// Job 定时任务模型
type Job struct {
	BaseModel

	WorkflowID uint   `gorm:"not null;index" json:"workflow_id"` // 所属工作流 ID
	Workflow   Workflow `gorm:"foreignKey:WorkflowID" json:"workflow,omitempty"` // 所属工作流
	Status     string `gorm:"size:32;default:'pending'" json:"status"` // 状态: pending, running, completed, failed

	// 调度配置
	CronExpression string `gorm:"size:128" json:"cron_expression"` // Cron 表达式

	// 执行记录
	LastExecutionTime *time.Time `json:"last_execution_time"` // 最后执行时间
	NextExecutionTime *time.Time `json:"next_execution_time"` // 下次执行时间

	// 关联关系
	Executions []JobExecution `gorm:"foreignKey:JobID;constraint:OnDelete:CASCADE" json:"executions,omitempty"`
}

// TableName 指定表名
func (Job) TableName() string {
	return "jobs"
}

// JobExecution 任务执行记录模型
type JobExecution struct {
	BaseModel

	JobID uint `gorm:"not null;index" json:"job_id"` // 所属任务 ID
	Job   Job  `gorm:"foreignKey:JobID" json:"job,omitempty"` // 所属任务

	// 执行信息
	StartTime time.Time `json:"start_time"`             // 开始时间
	EndTime   *time.Time `json:"end_time,omitempty"`   // 结束时间
	Status    string      `gorm:"size:32" json:"status"` // 状态: running, completed, failed

	// 日志信息（JSON 格式）
	LogInfo string `gorm:"type:text" json:"log_info"` // 执行日志（JSON 格式）

	// 关联关系
	Reports []Report `gorm:"foreignKey:ExecutionID;constraint:OnDelete:CASCADE" json:"reports,omitempty"`
}

// TableName 指定表名
func (JobExecution) TableName() string {
	return "job_executions"
}

// Report 抓取报告模型
type Report struct {
	BaseModel

	ExecutionID uint `gorm:"not null;index" json:"execution_id"` // 所属执行记录 ID
	Execution   JobExecution `gorm:"foreignKey:ExecutionID" json:"execution,omitempty"` // 所属执行记录

	// 报告内容
	ReportBody string `gorm:"type:text" json:"report_body"` // 报告内容（JSON 格式）
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
