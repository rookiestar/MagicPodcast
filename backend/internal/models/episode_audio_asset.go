package models

import "time"

const (
	EpisodeAudioAssetStatusQueued      = "queued"
	EpisodeAudioAssetStatusDownloading = "downloading"
	EpisodeAudioAssetStatusReady       = "ready"
	EpisodeAudioAssetStatusFailed      = "failed"
)

// EpisodeAudioAsset is the durable preparation state for one episode's
// managed local audio. SourceDigest identifies Episode.MediumURL without
// persisting it. RelativePath and ClaimToken must never leave the backend API.
type EpisodeAudioAsset struct {
	ID              uint       `gorm:"primaryKey" json:"id"`
	EpisodeID       uint       `gorm:"not null;index" json:"episode_id"`
	Episode         Episode    `gorm:"foreignKey:EpisodeID;constraint:OnDelete:CASCADE" json:"-"`
	SourceDigest    string     `gorm:"size:64;not null;index" json:"-"`
	Status          string     `gorm:"size:16;not null;index;check:chk_episode_audio_asset_status,status IN ('queued','downloading','ready','failed')" json:"status"`
	RelativePath    string     `gorm:"size:500;not null;default:''" json:"-"`
	SHA256          string     `gorm:"size:64;not null;default:''" json:"sha256,omitempty"`
	SizeBytes       int64      `gorm:"not null;default:0" json:"size_bytes"`
	DurationSeconds int        `gorm:"not null" json:"duration_seconds"`
	MediaType       string     `gorm:"size:100;not null;default:''" json:"media_type,omitempty"`
	Extension       string     `gorm:"size:12;not null;default:''" json:"extension,omitempty"`
	ErrorCode       string     `gorm:"size:80;not null;default:''" json:"error_code,omitempty"`
	ErrorMessage    string     `gorm:"size:300;not null;default:''" json:"error_message,omitempty"`
	ClaimToken      string     `gorm:"size:64;not null;default:''" json:"-"`
	ClaimExpiresAt  *time.Time `gorm:"index" json:"-"`
	QueuedAt        time.Time  `gorm:"not null" json:"queued_at"`
	DownloadingAt   *time.Time `json:"downloading_at,omitempty"`
	ReadyAt         *time.Time `gorm:"index" json:"ready_at,omitempty"`
	FailedAt        *time.Time `gorm:"index" json:"failed_at,omitempty"`
	CreatedAt       time.Time  `gorm:"not null;index" json:"created_at"`
	UpdatedAt       time.Time  `gorm:"not null;index" json:"updated_at"`
}

func (EpisodeAudioAsset) TableName() string { return "episode_audio_assets" }

const ActiveEpisodeAudioAssetUniqueIndexSQL = `
CREATE UNIQUE INDEX IF NOT EXISTS idx_episode_audio_assets_one_active
ON episode_audio_assets(episode_id)
WHERE status IN ('queued', 'downloading')`

const ReadyEpisodeAudioAssetUniqueIndexSQL = `
CREATE UNIQUE INDEX IF NOT EXISTS idx_episode_audio_assets_ready_source
ON episode_audio_assets(episode_id, source_digest)
WHERE status = 'ready'`
