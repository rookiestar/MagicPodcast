package contentsearch

import (
	"context"
	"errors"
	"time"
)

var (
	ErrInvalidRequest = errors.New("invalid content search request")
	ErrStaleDocument  = errors.New("content search document is superseded")
	ErrSearchFailed   = errors.New("content search failed")
)

const (
	SourceTranscript = "transcript"
	SourceShowNotes  = "show_notes"

	CoverageIndexNotReady = "index_not_ready"
	CoverageTruncated     = "truncated"
	CoverageComplete      = "complete"
	CoverageMiss          = "miss"

	maxLimit     = 50
	defaultLimit = 8
)

type Scope struct {
	EpisodeIDs []uint
}

type Filter struct {
	PersonID   *uint
	EpisodeID  *uint
	SourceKind string
}

type Request struct {
	Query  string
	Scope  Scope
	Filter Filter
	Limit  int
}

type Hit struct {
	EpisodeTitle      string `json:"episode_title"`
	PodcastTitle      string `json:"podcast_title"`
	EpisodeID         uint   `json:"episode_id"`
	SourceKind        string `json:"source_kind"`
	SourceVersion     string `json:"source_version"`
	FragmentOrder     int    `json:"fragment_order"`
	StartMS           int64  `json:"start_ms"`
	Text              string `json:"text"`
	ContextBefore     string `json:"context_before"`
	ContextAfter      string `json:"context_after"`
	PersonID          *uint  `json:"person_id,omitempty"`
	AttributionStatus string `json:"attribution_status,omitempty"`
	PublishedAt       string `json:"published_at"`
}

type Coverage struct {
	Complete        bool   `json:"complete"`
	Truncated       bool   `json:"truncated"`
	Reason          string `json:"reason"`
	MissingEpisodes []uint `json:"missing_episodes,omitempty"`
}

type Result struct {
	Hits     []Hit    `json:"hits"`
	Coverage Coverage `json:"coverage"`
}

type FragmentInput struct {
	Order             int
	StartMS           int64
	Text              string
	PersonID          *uint
	AttributionStatus string
}

// AttributionVersion identifies the facts read for an identity-aware index
// replacement. PublishedRevision also changes when an in-flight request commits.
type AttributionVersion struct {
	Revision          uint
	PublishedRevision uint
}

type EpisodeDocument struct {
	AttributionVersion *AttributionVersion
	EpisodeID          uint
	PublishedAt        time.Time
	ShowNotes          string
	SourceKind         string
	SourceVersion      string
	Fragments          []FragmentInput
	Complete           bool
	IncompleteReason   string
}

type Module interface {
	Search(context.Context, Request) (Result, error)
	ReplaceEpisode(context.Context, EpisodeDocument) error
	RemoveEpisode(context.Context, uint) error
}
