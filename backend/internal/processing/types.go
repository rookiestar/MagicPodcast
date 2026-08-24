package processing

import (
	"context"
	"encoding/json"
	"time"

	"magicpodcast/internal/models"
)

const (
	StepAudioPreparation = "audio_prepare"
	StepTranscription    = "transcription"
	StepEpisodeNotes     = "episode_notes"
	StepArtifactPublish  = "artifact_publish"

	ExternalProgressWaiting   = "waiting"
	ExternalProgressCompleted = "completed"
	ExternalProgressUnknown   = "unknown"
)

type StartRequest struct {
	EpisodeID     uint
	TriggerSource string
	Force         bool
}

type ProcessingInput struct {
	AudioDigest     string
	PipelineVersion string
}

// ProcessingInputResolver keeps content identity and pipeline version under
// backend control. HTTP and scheduler callers only select an episode and
// trigger source; they cannot forge an idempotency key.
type ProcessingInputResolver interface {
	ResolveProcessingInput(context.Context, uint) (ProcessingInput, error)
	PipelineVersion() string
}

type RunCanceler interface {
	Cancel(context.Context, uint) (models.EpisodeProcessingRun, error)
}

type StartResult struct {
	Run              models.EpisodeProcessingRun `json:"run"`
	ReusedActive     bool                        `json:"reused_active"`
	ReusedSuccessful bool                        `json:"reused_successful"`
	AudioAsset       *models.EpisodeAudioAsset   `json:"audio_asset,omitempty"`
	PreparingAudio   bool                        `json:"preparing_audio"`
}

type RetryPolicy struct {
	MaxAttempts int
	MaxElapsed  time.Duration
	BaseDelay   time.Duration
}

func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxAttempts: 3,
		MaxElapsed:  24 * time.Hour,
		BaseDelay:   5 * time.Second,
	}
}

type TranscriptionRequest struct {
	RunID           uint
	EpisodeID       uint
	AudioDigest     string
	PipelineVersion string
	// PersistCheckpoint lets an adapter durably record an external-write
	// intent before the write and the returned remote identity immediately
	// afterwards. A write adapter must not perform a second external write in
	// the same Begin/Resume call.
	PersistCheckpoint func(context.Context, string, json.RawMessage) error
}

type TranscriptionProgress struct {
	Status        string
	Checkpoint    json.RawMessage
	Transcript    string
	RawArtifacts  map[string][]byte
	SourceRefs    map[string]string
	SkillVersions map[string]string
}

type TranscriptionAdapter interface {
	Name() string
	Version() string
	// A completed result must include a valid checkpoint that lets Resume
	// reproduce the completed transcript and raw metadata after a retry or
	// restart. The engine rejects non-recoverable completed results.
	Begin(context.Context, TranscriptionRequest) (TranscriptionProgress, error)
	Resume(context.Context, TranscriptionRequest, json.RawMessage) (TranscriptionProgress, error)
	Cancel(context.Context, uint, json.RawMessage) error
}

type RuntimeRequest struct {
	RunID           uint
	EpisodeID       uint
	PipelineVersion string
	Transcript      string
}

type RuntimeResult struct {
	EpisodeNotes   string
	RuntimeVersion string
	PromptVersion  string
	SkillVersions  map[string]string
}

type RuntimeAdapter interface {
	Name() string
	Execute(context.Context, RuntimeRequest) (RuntimeResult, error)
	Cancel(context.Context, uint) error
}

type ArtifactPublishRequest struct {
	RunID                uint
	EpisodeID            uint
	AudioDigest          string
	PipelineVersion      string
	Transcript           string
	EpisodeNotes         string
	TranscriptionAdapter string
	TranscriptionVersion string
	RuntimeAdapter       string
	RuntimeVersion       string
	PromptVersion        string
	SkillVersions        map[string]string
	Sources              map[string]string
	RawArtifacts         map[string][]byte
	GeneratedAt          time.Time
}

type ArtifactPublishResult struct {
	RootPath         string
	ManifestPath     string
	ManifestSHA256   string
	TranscriptSHA256 string
	NotesSHA256      string
}

type ArtifactStore interface {
	Publish(context.Context, ArtifactPublishRequest) (ArtifactPublishResult, error)
	Discard(context.Context, ArtifactPublishResult) error
}

type ArtifactContent struct {
	Kind    string `json:"kind"`
	Content string `json:"content"`
	SHA256  string `json:"sha256"`
}

// ArtifactReader exposes only the two normalized Markdown documents from an
// already recorded artifact set. Implementations must validate the recorded
// root and expected digest instead of accepting arbitrary paths from callers.
type ArtifactReader interface {
	ReadText(context.Context, models.EpisodeArtifactSet, string) (ArtifactContent, error)
}

type KnowledgePackage struct {
	EpisodeID       uint
	PipelineVersion string
	ManifestSHA256  string
	Transcript      string
	EpisodeNotes    string
	Sources         map[string]string
}

type DeliveryRequest struct {
	ArtifactSetID uint
	DeliveryKey   string
	Destination   string
	Package       KnowledgePackage
}

type DeliveryReceipt struct {
	RemoteRef string
	PublicURL string
}

type KnowledgeBridge interface {
	Target() string
	AdapterVersion() string
	Deliver(context.Context, DeliveryRequest) (DeliveryReceipt, error)
	Cancel(context.Context, uint) error
}

type BridgeBinding struct {
	Destination string
	Adapter     KnowledgeBridge
}
