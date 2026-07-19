package dto

import (
	"time"

	"magicpodcast/internal/models"
)

// WorkflowResponse Workflow 响应结构
type WorkflowResponse struct {
	ID          uint                     `json:"id"`
	Name        string                   `json:"name"`
	Description string                   `json:"description"`
	Schedule    string                   `json:"schedule"`
	ScopeType   models.WorkflowScopeType `json:"scope_type"`
	ScopeConfig models.ScopeConfig       `json:"scope_config"`
	RulesConfig models.RulesConfig       `json:"rules_config"`
	IsEnabled   bool                     `json:"is_enabled"`
	CreatedAt   time.Time                `json:"created_at"`
	UpdatedAt   time.Time                `json:"updated_at"`
	LastJob     *JobResponse             `json:"last_job,omitempty"`
	Stats       *WorkflowStats           `json:"stats,omitempty"`
}

// WorkflowStats 工作流统计信息
type WorkflowStats struct {
	TotalJobs     int64      `json:"total_jobs"`
	SuccessJobs   int64      `json:"success_jobs"`
	FailedJobs    int64      `json:"failed_jobs"`
	TotalEpisodes float64    `json:"total_episodes"` // 平均每次执行匹配的单集数
	PodcastCount  int64      `json:"podcast_count"`
	LastExecution *time.Time `json:"last_execution,omitempty"`
	NextExecution *time.Time `json:"next_execution,omitempty"`
}

// JobResponse Job 响应结构
type JobResponse struct {
	ID                uint                   `json:"id"`
	WorkflowID        uint                   `json:"workflow_id"`
	Status            models.JobStatus       `json:"status"`
	StartTime         *time.Time             `json:"start_time,omitempty"`
	EndTime           *time.Time             `json:"end_time,omitempty"`
	PodcastsProcessed int                    `json:"podcasts_processed"`
	EpisodesFound     int                    `json:"episodes_found"`
	EpisodesCreated   int                    `json:"episodes_created"`
	EpisodesMatched   int                    `json:"episodes_matched"`
	ErrorCount        int                    `json:"error_count"`
	TriggeredBy       string                 `json:"triggered_by"`
	CreatedAt         time.Time              `json:"created_at"`
	Duration          *int64                 `json:"duration,omitempty"` // 执行时长（毫秒）
	Executions        []JobExecutionResponse `json:"executions,omitempty"`

	// LLM相关字段
	LLMSummary    *string `json:"llm_summary,omitempty"`     // LLM生成的摘要
	LLMModelUsed  *string `json:"llm_model_used,omitempty"`  // 使用的模型名称
	LLMTokensUsed *int    `json:"llm_tokens_used,omitempty"` // 使用的token数量
	LLMError      *string `json:"llm_error,omitempty"`       // LLM错误信息
}

// JobExecutionResponse JobExecution 响应结构
type JobExecutionResponse struct {
	ID              uint                   `json:"id"`
	JobID           uint                   `json:"job_id"`
	PodcastID       *uint                  `json:"podcast_id,omitempty"`
	PodcastTitle    string                 `json:"podcast_title,omitempty"`
	PodcastFeedURL  string                 `json:"podcast_feed_url,omitempty"`
	Status          models.ExecutionStatus `json:"status"`
	EpisodesFound   int                    `json:"episodes_found"`
	EpisodesCreated int                    `json:"episodes_created"`
	EpisodesMatched int                    `json:"episodes_matched"`
	ErrorMessage    string                 `json:"error_message,omitempty"`
	LogInfo         string                 `json:"log_info,omitempty"`
	ProcessingTime  int                    `json:"processing_time"` // 毫秒
	CreatedAt       time.Time              `json:"created_at"`

	FeedHTTPStatus          *int       `json:"feed_http_status"`
	FeedErrorCategory       string     `json:"feed_error_category"`
	FeedTargetDomain        string     `json:"feed_target_domain"`
	FeedResponseTimeMs      int        `json:"feed_response_time_ms"`
	FeedRetryAfter          string     `json:"feed_retry_after,omitempty"`
	FeedETag                string     `json:"feed_etag,omitempty"`
	FeedLastModified        string     `json:"feed_last_modified,omitempty"`
	FeedCacheControl        string     `json:"feed_cache_control,omitempty"`
	FeedExpires             string     `json:"feed_expires,omitempty"`
	FeedAge                 string     `json:"feed_age,omitempty"`
	FeedResponseBytes       int64      `json:"feed_response_bytes"`
	FeedSourceType          string     `json:"feed_source_type"`
	FeedCacheStatus         string     `json:"feed_cache_status"`
	FeedFreshness           string     `json:"feed_freshness"`
	FeedEgressID            string     `json:"feed_egress_id"`
	FeedSnapshotRetrievedAt *time.Time `json:"feed_snapshot_retrieved_at,omitempty"`
	FeedCircuitState        string     `json:"feed_circuit_state"`
}

// BatchWorkflowStats 批量工作流统计
type BatchWorkflowStats struct {
	TotalJobs   int64
	SuccessJobs int64
	FailedJobs  int64
	AvgEpisodes float64
}
