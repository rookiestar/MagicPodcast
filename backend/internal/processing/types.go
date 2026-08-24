package processing

import (
	"context"
	"encoding/json"
	"time"

	"magicpodcast/internal/models"
)

const (
	StepTranscription   = "transcription"
	StepEpisodeNotes    = "episode_notes"
	StepArtifactPublish = "artifact_publish"

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
}

type RunCanceler interface {
	Cancel(context.Context, uint) (models.EpisodeProcessingRun, error)
}

type StartResult struct {
	Run              models.EpisodeProcessingRun `json:"run"`
	ReusedActive     bool                        `json:"reused_active"`
	ReusedSuccessful bool                        `json:"reused_successful"`
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

type KnowledgePackage struct {
	RunID               uint
	EpisodeID           uint
	EpisodeTitle        string
	PodcastTitle        string
	PublishedAt         time.Time
	SourceURL           string
	ShowNotes           string
	PipelineVersion     string
	ArtifactGeneratedAt time.Time
	ManifestSHA256      string
	TranscriptSHA256    string
	EpisodeNotesSHA256  string
	Transcript          string
	EpisodeNotes        string
	Sources             map[string]string
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
	// Status is optional for compatibility. Existing automatic bridges leave
	// it empty and are recorded as delivered. Manual bridges return pending.
	Status string
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
