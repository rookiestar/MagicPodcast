package episodecopilot

import (
	"context"
	"errors"
)

var (
	ErrEpisodeNotFound    = errors.New("episode not found")
	ErrInvalidQuestion    = errors.New("invalid episode copilot question")
	ErrContextUnavailable = errors.New("episode copilot context is unavailable")
	// ErrUnsupportedProfile is returned before any runtime execution when a
	// question carries a profile ID the runtime catalog does not define.
	ErrUnsupportedProfile  = errors.New("unsupported episode copilot profile")
	ErrTargetPersonInvalid = errors.New("target person is not a confirmed participant of this episode")
	ErrPersonPending       = errors.New("target person identity is still pending confirmation")
	ErrTranscriptRequired  = errors.New("persona questions require a current transcript")
)

type SelectionSource string

const (
	SelectionSourceShowNotes  SelectionSource = "show_notes"
	SelectionSourceTranscript SelectionSource = "transcript"
)

type QuestionRequest struct {
	EpisodeID          uint
	Question           string
	Selection          string
	SelectionSource    SelectionSource
	IncludePrivateNote bool
	// ProfileID is the stable runtime profile ID for this question. Empty
	// means the runtime default profile.
	ProfileID string
	// TargetPersonID is a structured episode participant id. Zero keeps the
	// ordinary current-episode question path. A free-text @ name is ignored.
	TargetPersonID uint
}

// ProfileDescriptor exposes the technical meaning of one runtime profile. The
// mapping itself stays inside the runtime module; callers only render it.
type ProfileDescriptor struct {
	ID              string `json:"id"`
	Model           string `json:"model"`
	Effort          string `json:"effort"`
	ServiceTier     string `json:"service_tier,omitempty"`
	ServiceTierName string `json:"service_tier_name"`
	Default         bool   `json:"is_default"`
}

type PersonCandidate struct {
	ID              uint     `json:"id"`
	DisplayName     string   `json:"display_name"`
	Aliases         []string `json:"aliases"`
	IdentityNote    string   `json:"identity_note"`
	Role            string   `json:"role"`
	Status          string   `json:"status"`
	StatusReason    string   `json:"status_reason"`
	EvidenceKind    string   `json:"evidence_kind"`
	EvidenceLocator string   `json:"evidence_locator"`
}

type ContextScope struct {
	EpisodeID            uint                `json:"episode_id"`
	ShowNotesAvailable   bool                `json:"show_notes_available"`
	TranscriptAvailable  bool                `json:"transcript_available"`
	PrivateNoteAvailable bool                `json:"private_note_available"`
	Profiles             []ProfileDescriptor `json:"profiles"`
	DefaultProfileID     string              `json:"default_profile_id"`
	People               []PersonCandidate   `json:"people,omitempty"`
	IndexReady           bool                `json:"index_ready"`
	IndexStatus          string              `json:"index_status,omitempty"`
}

type EpisodeContext struct {
	EpisodeID    uint
	EpisodeTitle string
	PodcastTitle string
	ShowNotes    string
	Transcript   string
	PrivateNotes string
}

type ContextLoader interface {
	Describe(context.Context, uint) (ContextScope, error)
	Load(context.Context, uint, bool) (EpisodeContext, error)
}

type Module interface {
	ContextScope(context.Context, uint) (ContextScope, error)
	Ask(context.Context, QuestionRequest) (<-chan StreamEvent, error)
}

type EventType string

const (
	EventTypeContext     EventType = "context"
	EventTypeStatus      EventType = "status"
	EventTypeAnswerDelta EventType = "answer_delta"
	EventTypeError       EventType = "error"
	EventTypeComplete    EventType = "complete"
)

type StreamEvent struct {
	Type                EventType     `json:"type"`
	Stage               string        `json:"stage,omitempty"`
	Message             string        `json:"message,omitempty"`
	Code                string        `json:"code,omitempty"`
	Retryable           bool          `json:"retryable,omitempty"`
	TranscriptUsed      bool          `json:"transcript_used"`
	PrivateNoteIncluded bool          `json:"private_note_included"`
	ProfileID           string        `json:"profile_id,omitempty"`
	FirstContentMS      int64         `json:"first_content_ms,omitempty"`
	TotalMS             int64         `json:"total_ms,omitempty"`
	Activity            *Activity     `json:"activity,omitempty"`
	StageTimings        *StageTimings `json:"stage_timings,omitempty"`
}

var _ Module = (*Service)(nil)
