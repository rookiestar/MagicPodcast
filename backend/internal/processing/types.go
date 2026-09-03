package processing

import (
	"context"
	"encoding/json"
	"time"

	"magicpodcast/internal/models"
)

const (
	NativeMinutesPipelineVersion = "focus-processing-v2"

	StepAudioPreparation  = "audio_prepare"
	StepTranscription     = "transcription"
	StepMinutesEnrichment = "minutes_enrichment"
	StepEpisodeNotes      = "episode_notes"
	StepArtifactPublish   = "artifact_publish"

	ExternalProgressWaiting   = "waiting"
	ExternalProgressCompleted = "completed"
	ExternalProgressUnknown   = "unknown"
)

type StartRequest struct {
	EpisodeID         uint
	TriggerSource     string
	Force             bool
	RequireReadyAudio bool
	ScheduleRunID     *uint
	// ScheduleQueuePosition is supplied only by the scheduler so a newly
	// created scheduled run can register its item in the same transaction.
	ScheduleQueuePosition *int64
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
	// ReusedTerminal is scheduler-internal. A scheduled trigger must not turn
	// a failed or cancelled run of the same immutable input back into an
	// unbounded automatic retry; a user can still use the explicit retry flow.
	ReusedTerminal bool                      `json:"-"`
	AudioAsset     *models.EpisodeAudioAsset `json:"audio_asset,omitempty"`
	PreparingAudio bool                      `json:"preparing_audio"`
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
	Status            string
	CurrentStep       string
	Checkpoint        json.RawMessage
	MinutesSummary    string
	Transcript        string
	Segments          []TranscriptSegment
	MinutesEnrichment MinutesEnrichment
	WhiteboardPreview []byte
	RawArtifacts      map[string][]byte
	SourceRefs        map[string]string
	SkillVersions     map[string]string
}

type TranscriptSegment struct {
	Order   int    `json:"order"`
	Speaker string `json:"speaker"`
	StartMS int64  `json:"start_ms"`
	Text    string `json:"text"`
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

// TranscriptionCancellationDisposition describes what remains observable after
// local cancellation. It deliberately contains no checkpoint or remote
// identity, because those values must not leave the adapter boundary.
type TranscriptionCancellationDisposition struct {
	RemoteMayContinue bool
	Message           string
}

// TranscriptionCancellationReporter is optional. Adapters that can determine
// whether an already-started external operation may outlive local cancellation
// use it to surface a bounded, user-safe warning.
type TranscriptionCancellationReporter interface {
	CancellationDisposition(json.RawMessage) (TranscriptionCancellationDisposition, error)
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
	NativeMinutes        bool
	MinutesSummary       string
	Transcript           string
	TranscriptSegments   []TranscriptSegment
	MinutesEnrichment    MinutesEnrichment
	WhiteboardPreview    []byte
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
	RootPath                 string
	ManifestPath             string
	ManifestSHA256           string
	AudioSHA256              string
	MinutesSummarySHA256     string
	TranscriptSHA256         string
	TranscriptTimelineSHA256 string
	NotesSHA256              string
}

type ArtifactStore interface {
	Publish(context.Context, ArtifactPublishRequest) (ArtifactPublishResult, error)
	Discard(context.Context, ArtifactPublishResult) error
}

type ArtifactContent struct {
	Kind           string                `json:"kind"`
	Content        string                `json:"content"`
	SHA256         string                `json:"sha256"`
	Segments       []TranscriptSegment   `json:"segments,omitempty"`
	TimelineSHA256 string                `json:"timeline_sha256,omitempty"`
	MediaAvailable bool                  `json:"media_available"`
	AudioRecovery  *AudioRecoverySummary `json:"audio_recovery,omitempty"`
	Chapters       []MinutesChapter      `json:"chapters,omitempty"`
	Keywords       []string              `json:"keywords,omitempty"`
	Decisions      []string              `json:"decisions,omitempty"`
	Quotes         []MinutesQuote        `json:"quotes,omitempty"`
	Links          []MinutesLink         `json:"links,omitempty"`
	Whiteboard     *MinutesWhiteboard    `json:"whiteboard,omitempty"`
}

type ArtifactMedia struct {
	MediaID   string
	MediaType string
	SHA256    string
	Width     int
	Height    int
	Body      []byte
}

type ArtifactMediaReader interface {
	ReadMedia(context.Context, models.EpisodeArtifactSet, string) (ArtifactMedia, error)
}

// ArtifactReader exposes only normalized public documents from an already
// recorded artifact set. Implementations must validate the recorded root and
// expected digest instead of accepting arbitrary paths from callers.
type ArtifactReader interface {
	ReadText(context.Context, models.EpisodeArtifactSet, string) (ArtifactContent, error)
}

type KnowledgePackage struct {
	RunID                    uint
	EpisodeID                uint
	EpisodeTitle             string
	PodcastTitle             string
	PublishedAt              time.Time
	SourceURL                string
	ShowNotes                string
	PipelineVersion          string
	ArtifactGeneratedAt      time.Time
	ManifestSHA256           string
	MinutesSummarySHA256     string
	TranscriptSHA256         string
	TranscriptTimelineSHA256 string
	EpisodeNotesSHA256       string
	MinutesSummary           string
	Transcript               string
	EpisodeNotes             string
	Sources                  map[string]string
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
