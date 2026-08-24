package episodecopilot

import (
	"context"
	"errors"
)

var (
	ErrEpisodeNotFound    = errors.New("episode not found")
	ErrInvalidQuestion    = errors.New("invalid episode copilot question")
	ErrContextUnavailable = errors.New("episode copilot context is unavailable")
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
}

type ContextScope struct {
	EpisodeID            uint `json:"episode_id"`
	ShowNotesAvailable   bool `json:"show_notes_available"`
	TranscriptAvailable  bool `json:"transcript_available"`
	PrivateNoteAvailable bool `json:"private_note_available"`
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
	Type                EventType `json:"type"`
	Stage               string    `json:"stage,omitempty"`
	Message             string    `json:"message,omitempty"`
	Code                string    `json:"code,omitempty"`
	Retryable           bool      `json:"retryable,omitempty"`
	TranscriptUsed      bool      `json:"transcript_used"`
	PrivateNoteIncluded bool      `json:"private_note_included"`
	FirstContentMS      int64     `json:"first_content_ms,omitempty"`
	TotalMS             int64     `json:"total_ms,omitempty"`
}

var _ Module = (*Service)(nil)
