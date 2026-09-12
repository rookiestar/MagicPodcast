package personidentity

import (
	"context"
	"errors"
	"time"

	"magicpodcast/internal/processing"
)

var (
	ErrEpisodeNotFound       = errors.New("episode not found")
	ErrPersonNotFound        = errors.New("person not found")
	ErrInvalidCorrection     = errors.New("invalid person correction")
	ErrNotEpisodeParticipant = errors.New("person is not an episode participant")
	ErrTranscriptRequired    = errors.New("episode transcript is required")
	ErrIdentityUnavailable   = errors.New("person identity runtime is unavailable")
	ErrSourcesChanged        = errors.New("person identity sources changed during preparation")
	ErrConfirmationProtected = errors.New("user confirmation cannot be overwritten by extraction")
)

const (
	RoleHost    = "host"
	RoleGuest   = "guest"
	RoleUnknown = "unknown"

	StatusConfirmed = "confirmed"
	StatusPending   = "pending"
	StatusRejected  = "rejected"

	SourceTranscript = "transcript"
	SourceShowNotes  = "show_notes"
)

type Segment struct {
	Order        int
	SpeakerLabel string
	StartMS      int64
	Text         string
}

type EpisodeSources struct {
	PodcastTitle         string
	PodcastAuthor        string
	PodcastDescription   string
	EpisodeTitle         string
	EpisodePublishedDate string
	EpisodeID            uint
	ShowNotes            string
	SourceKind           string
	SourceVersion        string
	Segments             []Segment
	OriginalBytes        []byte
}

type PersonView struct {
	RoleUserConfirmed bool     `json:"role_user_confirmed"`
	ID                uint     `json:"id"`
	StableKey         string   `json:"stable_key"`
	DisplayName       string   `json:"display_name"`
	Aliases           []string `json:"aliases"`
	IdentityNote      string   `json:"identity_note"`
	Role              string   `json:"role"`
	Status            string   `json:"status"`
	StatusReason      string   `json:"status_reason"`
	EvidenceKind      string   `json:"evidence_kind"`
	EvidenceLocator   string   `json:"evidence_locator"`
	ConfirmedSpeech   int      `json:"confirmed_speech_count"`
	PendingSpeech     int      `json:"pending_speech_count"`
}

type AttributionView struct {
	ID              uint   `json:"id"`
	PersonID        *uint  `json:"person_id"`
	DisplayName     string `json:"display_name,omitempty"`
	SourceKind      string `json:"source_kind"`
	SourceVersion   string `json:"source_version"`
	FragmentOrder   int    `json:"fragment_order"`
	SpeakerLabel    string `json:"speaker_label"`
	StartMS         int64  `json:"start_ms"`
	Text            string `json:"text"`
	Status          string `json:"status"`
	EvidenceKind    string `json:"evidence_kind"`
	EvidenceLocator string `json:"evidence_locator"`
	UserConfirmed   bool   `json:"user_confirmed"`
}

type EpisodePeople struct {
	Revision            uint         `json:"revision"`
	Draft               *ReviewDraft `json:"draft,omitempty"`
	PreparationState    string       `json:"preparation_state"`
	preparationRevision uint
	publishedRevision   uint
	EpisodeID           uint              `json:"episode_id"`
	SourceVersion       string            `json:"source_version"`
	IndexReady          bool              `json:"index_ready"`
	PublishedAt         time.Time         `json:"published_at,omitempty"`
	ExcludedPeople      []PersonView      `json:"excluded_people"`
	People              []PersonView      `json:"people"`
	Attributions        []AttributionView `json:"attributions"`
}

type NameCorrection struct {
	PersonID     uint
	DisplayName  string
	Aliases      []string
	IdentityNote string
}

type AttributionCorrection struct {
	SourceVersion    string
	SourceKind       string
	FragmentOrder    int
	AssignedPersonID *uint
	Status           string
}

type AttributionFact struct {
	EpisodeID     uint
	PersonID      *uint
	SourceKind    string
	SourceVersion string
	FragmentOrder int
	SpeakerLabel  string
	StartMS       int64
	Text          string
	Status        string
	Current       bool
}

type Module interface {
	Prepare(context.Context, EpisodeSources) (EpisodePeople, error)
	ListEpisodePeople(context.Context, uint) (EpisodePeople, error)
	CorrectName(context.Context, uint, NameCorrection) (EpisodePeople, error)
	CorrectAttribution(context.Context, uint, AttributionCorrection) (EpisodePeople, error)
	ReliableSpeech(context.Context, uint, uint) ([]AttributionFact, error)
	CurrentFacts(context.Context, uint) ([]AttributionFact, error)
	AccessibleEpisodeIDs(context.Context) ([]uint, error)
}

type CandidateSuggester interface {
	Suggest(context.Context, EpisodeSources) (Suggestions, error)
}

type SuggestedCandidate struct {
	// SourceNames are episode-local transcript spellings, not global aliases.
	SourceNames     []string
	Status          string
	SpeechOrders    []int
	DisplayName     string
	Aliases         []string
	IdentityNote    string
	Role            string
	EvidenceKind    string
	EvidenceLocator string
	MentionedOnly   bool
}

func SegmentsFromTranscript(segments []processing.TranscriptSegment) []Segment {
	out := make([]Segment, 0, len(segments))
	for _, segment := range segments {
		out = append(out, Segment{
			Order:        segment.Order,
			SpeakerLabel: segment.Speaker,
			StartMS:      segment.StartMS,
			Text:         segment.Text,
		})
	}
	return out
}

func nowUTC() time.Time {
	return time.Now().UTC()
}

// Suggestions keeps review-only relations distinct from published attribution.
type Suggestions struct {
	Candidates []SuggestedCandidate
	Matches    []ReviewMatch
}
