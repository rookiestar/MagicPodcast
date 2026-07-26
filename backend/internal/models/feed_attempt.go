package models

import "time"

// JobFeedAttempt is one network attempt for a podcast inside a Job.
// Safe metadata only — never Feed body, Cookie, Token, credentials, or
// arbitrary response headers (#35/#39).
type JobFeedAttempt struct {
	BaseModel

	JobID      uint   `gorm:"index:idx_feed_attempt_job;not null" json:"job_id"`
	PodcastID  *uint  `gorm:"index;not null" json:"podcast_id"`
	AttemptNo  int    `gorm:"not null;default:1" json:"attempt_no"`
	SourceType string `gorm:"size:32;not null;default:primary" json:"source_type"`
	// AttemptedAt is when the Feed outcome was observed. Synthetic unattempted
	// rows use the batch deadline and have AttemptNo=-1.
	AttemptedAt time.Time `gorm:"not null" json:"attempted_at"`

	HTTPStatus           *int   `gorm:"column:http_status" json:"http_status"`
	ErrorCategory        string `gorm:"size:40;not null;default:not_observed" json:"error_category"`
	FailurePhase         string `gorm:"size:40;not null;default:''" json:"failure_phase"`
	RetryDecision        string `gorm:"size:64;not null;default:''" json:"retry_decision"`
	IdentityVerification string `gorm:"size:80;not null;default:not_checked" json:"identity_verification"`
	TargetDomain         string `gorm:"size:255;not null;default:''" json:"target_domain"`
	// SourceURL is sanitized (no credentials/query secrets).
	SourceURL     string `gorm:"size:1000;not null;default:''" json:"source_url"`
	IsFinalResult bool   `gorm:"not null;default:false" json:"is_final_result"`
	// DerivedPolicy marks outcomes like circuit_open that must not be
	// double-counted as independent upstream root causes.
	DerivedPolicy bool `gorm:"not null;default:false" json:"derived_policy"`
	// User-Agent recovery metadata is safe bounded state only; no raw UA,
	// response headers, cookies or Feed bodies are persisted here.
	UserAgentGateState   string     `gorm:"column:feed_user_agent_gate_state;size:32;not null;default:''" json:"user_agent_gate_state,omitempty"`
	UserAgentProbeResult string     `gorm:"column:feed_user_agent_probe_result;size:40;not null;default:''" json:"user_agent_probe_result,omitempty"`
	UserAgentApprovedBy  string     `gorm:"column:feed_user_agent_approved_by;size:128;not null;default:''" json:"user_agent_approved_by,omitempty"`
	UserAgentApprovedAt  *time.Time `gorm:"column:feed_user_agent_approved_at" json:"user_agent_approved_at,omitempty"`
	UserAgentLastProbeAt *time.Time `gorm:"column:feed_user_agent_last_probe_at" json:"user_agent_last_probe_at,omitempty"`
}

func (JobFeedAttempt) TableName() string { return "job_feed_attempts" }
