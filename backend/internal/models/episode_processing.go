package models

import "time"

const (
	ProcessingRunStatusQueued          = "queued"
	ProcessingRunStatusRunning         = "running"
	ProcessingRunStatusWaitingExternal = "waiting_external"
	ProcessingRunStatusCompleted       = "completed"
	ProcessingRunStatusFailed          = "failed"
	ProcessingRunStatusCancelled       = "cancelled"

	ProcessingTriggerManual    = "manual"
	ProcessingTriggerScheduled = "scheduled"

	DeliveryStatusPending    = "pending"
	DeliveryStatusDelivering = "delivering"
	DeliveryStatusDelivered  = "delivered"
	DeliveryStatusFailed     = "failed"
	DeliveryStatusCancelled  = "cancelled"
)

var ProcessingRunTerminalStatuses = []string{
	ProcessingRunStatusCompleted,
	ProcessingRunStatusFailed,
	ProcessingRunStatusCancelled,
}

var ProcessingRunActiveStatuses = []string{
	ProcessingRunStatusQueued,
	ProcessingRunStatusRunning,
	ProcessingRunStatusWaitingExternal,
}

func IsProcessingRunTerminal(status string) bool {
	switch status {
	case ProcessingRunStatusCompleted, ProcessingRunStatusFailed, ProcessingRunStatusCancelled:
		return true
	default:
		return false
	}
}

// EpisodeProcessingRun is the durable authority for one episode-processing
// request. It deliberately does not reuse Workflow Job or consumption state.
type EpisodeProcessingRun struct {
	ID              uint                  `gorm:"primaryKey" json:"id"`
	EpisodeID       uint                  `gorm:"not null;index" json:"episode_id"`
	Episode         Episode               `gorm:"foreignKey:EpisodeID;constraint:OnDelete:CASCADE" json:"-"`
	ProcessingKey   string                `gorm:"size:64;not null;index" json:"processing_key"`
	AudioDigest     string                `gorm:"size:64;not null" json:"audio_digest"`
	PipelineVersion string                `gorm:"size:100;not null" json:"pipeline_version"`
	TriggerSource   string                `gorm:"size:20;not null;check:chk_processing_run_trigger,trigger_source IN ('manual','scheduled')" json:"trigger_source"`
	Status          string                `gorm:"size:24;not null;index;check:chk_processing_run_status,status IN ('queued','running','waiting_external','completed','failed','cancelled')" json:"status"`
	CurrentStep     string                `gorm:"size:64;not null;default:''" json:"current_step"`
	PreviousRunID   *uint                 `gorm:"index" json:"previous_run_id,omitempty"`
	PreviousRun     *EpisodeProcessingRun `gorm:"foreignKey:PreviousRunID;constraint:OnDelete:SET NULL" json:"-"`
	AttemptCount    int                   `gorm:"not null;default:0" json:"attempt_count"`
	MaxAttempts     int                   `gorm:"not null;default:3" json:"max_attempts"`
	RetryDeadlineAt time.Time             `gorm:"not null" json:"retry_deadline_at"`
	NextAttemptAt   *time.Time            `gorm:"index" json:"next_attempt_at,omitempty"`
	StartedAt       *time.Time            `json:"started_at,omitempty"`
	FinishedAt      *time.Time            `gorm:"index" json:"finished_at,omitempty"`
	CancelledAt     *time.Time            `json:"cancelled_at,omitempty"`
	ErrorCode       string                `gorm:"size:80;not null;default:''" json:"error_code,omitempty"`
	ErrorMessage    string                `gorm:"size:500;not null;default:''" json:"error_message,omitempty"`
	ErrorRetryable  bool                  `gorm:"not null;default:false" json:"error_retryable"`
	CreatedAt       time.Time             `gorm:"not null;index" json:"created_at"`
	UpdatedAt       time.Time             `gorm:"not null;index" json:"updated_at"`
}

func (EpisodeProcessingRun) TableName() string { return "episode_processing_runs" }

// ProcessingCheckpoint stores opaque adapter state for restart-safe polling.
// StateJSON is intentionally excluded from API serialization.
type ProcessingCheckpoint struct {
	ID             uint                 `gorm:"primaryKey" json:"id"`
	RunID          uint                 `gorm:"not null;uniqueIndex:idx_processing_checkpoint_run_step" json:"run_id"`
	Run            EpisodeProcessingRun `gorm:"foreignKey:RunID;constraint:OnDelete:CASCADE" json:"-"`
	Step           string               `gorm:"size:64;not null;uniqueIndex:idx_processing_checkpoint_run_step" json:"step"`
	Adapter        string               `gorm:"size:64;not null" json:"adapter"`
	AdapterVersion string               `gorm:"size:100;not null" json:"adapter_version"`
	Status         string               `gorm:"size:32;not null;check:chk_processing_checkpoint_status,status IN ('waiting','completed')" json:"status"`
	StateJSON      string               `gorm:"type:text;not null" json:"-"`
	StateHash      string               `gorm:"size:64;not null" json:"state_hash"`
	CreatedAt      time.Time            `gorm:"not null" json:"created_at"`
	UpdatedAt      time.Time            `gorm:"not null" json:"updated_at"`
}

func (ProcessingCheckpoint) TableName() string { return "processing_checkpoints" }

// EpisodeArtifactSet identifies one immutable, atomically published local
// artifact directory. RootPath never leaves the backend API.
type EpisodeArtifactSet struct {
	ID               uint                 `gorm:"primaryKey" json:"id"`
	RunID            uint                 `gorm:"not null;uniqueIndex" json:"run_id"`
	Run              EpisodeProcessingRun `gorm:"foreignKey:RunID;constraint:OnDelete:CASCADE" json:"-"`
	EpisodeID        uint                 `gorm:"not null;index" json:"episode_id"`
	Episode          Episode              `gorm:"foreignKey:EpisodeID;constraint:OnDelete:CASCADE" json:"-"`
	PipelineVersion  string               `gorm:"size:100;not null" json:"pipeline_version"`
	RootPath         string               `gorm:"type:text;not null" json:"-"`
	ManifestPath     string               `gorm:"size:255;not null" json:"manifest_path"`
	ManifestSHA256   string               `gorm:"size:64;not null" json:"manifest_sha256"`
	TranscriptSHA256 string               `gorm:"size:64;not null" json:"transcript_sha256"`
	NotesSHA256      string               `gorm:"size:64;not null" json:"notes_sha256"`
	IsCurrent        bool                 `gorm:"not null;default:false;index" json:"is_current"`
	CreatedAt        time.Time            `gorm:"not null;index" json:"created_at"`
}

func (EpisodeArtifactSet) TableName() string { return "episode_artifact_sets" }

// KnowledgeDelivery is independent from both processing and consumption.
type KnowledgeDelivery struct {
	ID             uint               `gorm:"primaryKey" json:"id"`
	ArtifactSetID  uint               `gorm:"not null;index" json:"artifact_set_id"`
	ArtifactSet    EpisodeArtifactSet `gorm:"foreignKey:ArtifactSetID;constraint:OnDelete:CASCADE" json:"-"`
	Target         string             `gorm:"size:64;not null" json:"target"`
	Destination    string             `gorm:"size:255;not null" json:"destination"`
	AdapterVersion string             `gorm:"size:100;not null" json:"adapter_version"`
	DeliveryKey    string             `gorm:"size:64;not null;uniqueIndex" json:"delivery_key"`
	Status         string             `gorm:"size:24;not null;index;check:chk_knowledge_delivery_status,status IN ('pending','delivering','delivered','failed','cancelled')" json:"status"`
	AttemptCount   int                `gorm:"not null;default:0" json:"attempt_count"`
	RemoteRef      string             `gorm:"size:255;not null;default:''" json:"remote_ref,omitempty"`
	PublicURL      string             `gorm:"size:1000;not null;default:''" json:"public_url,omitempty"`
	ErrorCode      string             `gorm:"size:80;not null;default:''" json:"error_code,omitempty"`
	ErrorMessage   string             `gorm:"size:500;not null;default:''" json:"error_message,omitempty"`
	ErrorRetryable bool               `gorm:"not null;default:false" json:"error_retryable"`
	DeliveredAt    *time.Time         `json:"delivered_at,omitempty"`
	CreatedAt      time.Time          `gorm:"not null;index" json:"created_at"`
	UpdatedAt      time.Time          `gorm:"not null;index" json:"updated_at"`
}

func (KnowledgeDelivery) TableName() string { return "knowledge_deliveries" }

const ActiveProcessingRunUniqueIndexSQL = `
CREATE UNIQUE INDEX IF NOT EXISTS idx_episode_processing_runs_one_active
ON episode_processing_runs(episode_id)
WHERE status IN ('queued', 'running', 'waiting_external')`

const CurrentEpisodeArtifactSetUniqueIndexSQL = `
CREATE UNIQUE INDEX IF NOT EXISTS idx_episode_artifact_sets_one_current
ON episode_artifact_sets(episode_id)
WHERE is_current = 1`

const ProcessingRunTerminalStatusTriggerSQL = `
CREATE TRIGGER IF NOT EXISTS trg_episode_processing_runs_terminal_status
BEFORE UPDATE OF status ON episode_processing_runs
WHEN OLD.status IN ('completed', 'failed', 'cancelled')
 AND NEW.status <> OLD.status
BEGIN
	SELECT RAISE(ABORT, 'terminal processing run status is immutable');
END`
