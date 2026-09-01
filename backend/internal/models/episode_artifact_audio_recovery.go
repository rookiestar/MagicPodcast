package models

import "time"

const (
	EpisodeArtifactAudioRecoveryStatusQueued      = "queued"
	EpisodeArtifactAudioRecoveryStatusDownloading = "downloading"
	EpisodeArtifactAudioRecoveryStatusCompleted   = "completed"
	EpisodeArtifactAudioRecoveryStatusFailed      = "failed"
)

// EpisodeArtifactAudioRecovery is the durable, user-triggered repair state
// for one immutable artifact set. Sensitive identity and claim fields stay
// backend-only; the API exposes a separate safe summary.
type EpisodeArtifactAudioRecovery struct {
	ID              uint               `gorm:"primaryKey" json:"-"`
	ArtifactSetID   uint               `gorm:"not null" json:"-"`
	ArtifactSet     EpisodeArtifactSet `gorm:"foreignKey:ArtifactSetID;constraint:OnDelete:CASCADE" json:"-"`
	EpisodeID       uint               `gorm:"not null" json:"-"`
	Episode         Episode            `gorm:"foreignKey:EpisodeID;constraint:OnDelete:CASCADE" json:"-"`
	AudioAssetID    uint               `gorm:"not null" json:"-"`
	AudioAsset      EpisodeAudioAsset  `gorm:"foreignKey:AudioAssetID;constraint:OnDelete:RESTRICT" json:"-"`
	AudioSHA256     string             `gorm:"column:audio_sha256;size:64;not null" json:"-"`
	Status          string             `gorm:"size:16;not null;check:chk_episode_artifact_audio_recovery_status,status IN ('queued','downloading','completed','failed')" json:"-"`
	AttemptCount    int                `gorm:"not null;default:0" json:"-"`
	MaxAttempts     int                `gorm:"not null;default:3" json:"-"`
	RetryDeadlineAt time.Time          `gorm:"not null" json:"-"`
	NextAttemptAt   *time.Time         `json:"-"`
	ClaimToken      string             `gorm:"size:64;not null;default:''" json:"-"`
	ClaimExpiresAt  *time.Time         `json:"-"`
	ErrorCode       string             `gorm:"size:80;not null;default:''" json:"-"`
	ErrorMessage    string             `gorm:"size:300;not null;default:''" json:"-"`
	ErrorRetryable  bool               `gorm:"not null;default:false" json:"-"`
	QueuedAt        time.Time          `gorm:"not null" json:"-"`
	DownloadingAt   *time.Time         `json:"-"`
	CompletedAt     *time.Time         `json:"-"`
	FailedAt        *time.Time         `json:"-"`
	CreatedAt       time.Time          `gorm:"not null" json:"-"`
	UpdatedAt       time.Time          `gorm:"not null" json:"-"`
}

func (EpisodeArtifactAudioRecovery) TableName() string {
	return "episode_artifact_audio_recoveries"
}

const EpisodeArtifactAudioRecoveryCreateTableSQL = `
CREATE TABLE IF NOT EXISTS episode_artifact_audio_recoveries (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	artifact_set_id INTEGER NOT NULL,
	episode_id INTEGER NOT NULL,
	audio_asset_id INTEGER NOT NULL,
	audio_sha256 TEXT NOT NULL,
	status TEXT NOT NULL,
	attempt_count INTEGER NOT NULL DEFAULT 0,
	max_attempts INTEGER NOT NULL DEFAULT 3,
	retry_deadline_at DATETIME NOT NULL,
	next_attempt_at DATETIME,
	claim_token TEXT NOT NULL DEFAULT '',
	claim_expires_at DATETIME,
	error_code TEXT NOT NULL DEFAULT '',
	error_message TEXT NOT NULL DEFAULT '',
	error_retryable NUMERIC NOT NULL DEFAULT false,
	queued_at DATETIME NOT NULL,
	downloading_at DATETIME,
	completed_at DATETIME,
	failed_at DATETIME,
	created_at DATETIME NOT NULL,
	updated_at DATETIME NOT NULL,
	CONSTRAINT fk_episode_artifact_audio_recoveries_artifact_set
		FOREIGN KEY (artifact_set_id) REFERENCES episode_artifact_sets(id) ON DELETE CASCADE,
	CONSTRAINT fk_episode_artifact_audio_recoveries_episode
		FOREIGN KEY (episode_id) REFERENCES episodes(id) ON DELETE CASCADE,
	CONSTRAINT fk_episode_artifact_audio_recoveries_audio_asset
		FOREIGN KEY (audio_asset_id) REFERENCES episode_audio_assets(id) ON DELETE RESTRICT,
	CONSTRAINT chk_episode_artifact_audio_recovery_status
		CHECK (status IN ('queued','downloading','completed','failed'))
)`

const EpisodeArtifactAudioRecoveryUniqueIndexSQL = `
CREATE UNIQUE INDEX IF NOT EXISTS idx_episode_artifact_audio_recoveries_artifact_set_id
ON episode_artifact_audio_recoveries(artifact_set_id)`

const EpisodeArtifactAudioRecoveryClaimIndexSQL = `
CREATE INDEX IF NOT EXISTS idx_episode_artifact_audio_recoveries_claim
ON episode_artifact_audio_recoveries(status, claim_expires_at)`

const EpisodeArtifactAudioRecoveryNextAttemptIndexSQL = `
CREATE INDEX IF NOT EXISTS idx_episode_artifact_audio_recoveries_next_attempt
ON episode_artifact_audio_recoveries(status, next_attempt_at)`

// ArtifactAudioRecovery is a short alias for callers that do not need the
// episode-prefixed persistence name.
type ArtifactAudioRecovery = EpisodeArtifactAudioRecovery
