package episodecopilot

import (
	"context"
	"time"

	"magicpodcast/internal/codexruntime"
)

// User-visible question stages, in fixed presentation order. Stage IDs are
// part of the SSE contract; pages render their own labels.
const (
	StageReadContext        = "read_context"
	StageResearchRuntime    = "research_runtime"
	StagePublicResearch     = "public_research"
	StageSourceValidation   = "source_validation"
	StageAnswerRuntime      = "answer_runtime"
	StageComposeAnswer      = "compose_answer"
	StageCitationValidation = "citation_validation"
)

// activityCategoryStage marks service-side deterministic stage activities.
// Activities forwarded from the runtime keep their neutral host categories.
const activityCategoryStage = "stage"

const (
	activityIDPrefixResearch = "research"
	activityIDPrefixAnswer   = "answer"
	activityIDPrefixStage    = "stage"

	// Service-side bounds mirror the runtime progress bounds so forwarded
	// activities can never widen the payload the page receives.
	maxActivityTextRunes     = 200
	maxActivityMetadata      = 8
	maxActivityMetadataRunes = 200
)

// Activity is one sanitized execution activity on the question stream.
// Runtime-forwarded activities reuse the host's stable activity ID (stage
// prefixed); service stage activities use stable "stage:<stage>" IDs so the
// page can update them in place.
type Activity struct {
	ID         string            `json:"id"`
	Ordinal    uint64            `json:"ordinal"`
	Stage      string            `json:"stage"`
	Category   string            `json:"category"`
	State      string            `json:"state"`
	Text       string            `json:"text,omitempty"`
	ObservedAt time.Time         `json:"observed_at"`
	ElapsedMS  int64             `json:"elapsed_ms,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

// StageTimings reports per-stage durations for one completed question. A
// zero value means the stage did not run (for example a degraded research
// phase never produced a ready runtime).
type StageTimings struct {
	ResearchRuntimeReadyMS int64 `json:"research_runtime_ready_ms,omitempty"`
	PublicResearchMS       int64 `json:"public_research_ms,omitempty"`
	SourceValidationMS     int64 `json:"source_validation_ms,omitempty"`
	AnswerRuntimeReadyMS   int64 `json:"answer_runtime_ready_ms,omitempty"`
	CitationValidationMS   int64 `json:"citation_validation_ms,omitempty"`
}

// questionActivities assigns the question-scoped continuous ordinal and
// bounds every forwarded payload. The page never receives runtime event
// identities, only neutral stage-prefixed activity IDs.
type questionActivities struct {
	ordinal uint64
}

// forward converts one runtime progress event into a status-carried
// activity. It returns false when the question context ended.
func (q *questionActivities) forward(
	ctx context.Context,
	events chan<- StreamEvent,
	base StreamEvent,
	stage string,
	prefix string,
	event codexruntime.Event,
) bool {
	progress := event.Progress
	if progress == nil {
		return true
	}
	q.ordinal++
	activity := &Activity{
		ID:         prefix + ":" + progress.ActivityID,
		Ordinal:    q.ordinal,
		Stage:      stage,
		Category:   string(progress.Category),
		State:      string(progress.State),
		Text:       truncateRunes(progress.DisplayText, maxActivityTextRunes),
		ObservedAt: event.ObservedAt,
		ElapsedMS:  progress.ElapsedMS,
		Metadata:   boundActivityMetadata(progress.Metadata),
	}
	streamEvent := base
	streamEvent.Type = EventTypeStatus
	streamEvent.Stage = stage
	streamEvent.Activity = activity
	return emit(ctx, events, streamEvent)
}

// announce emits one deterministic service-side stage activity so the page
// has honest feedback even while no runtime is running yet.
func (q *questionActivities) announce(
	ctx context.Context,
	events chan<- StreamEvent,
	base StreamEvent,
	observedAt time.Time,
	stage string,
	state string,
	text string,
) bool {
	q.ordinal++
	activity := &Activity{
		ID:         activityIDPrefixStage + ":" + stage,
		Ordinal:    q.ordinal,
		Stage:      stage,
		Category:   activityCategoryStage,
		State:      state,
		Text:       text,
		ObservedAt: observedAt,
	}
	streamEvent := base
	streamEvent.Type = EventTypeStatus
	streamEvent.Stage = stage
	streamEvent.Activity = activity
	streamEvent.Message = text
	return emit(ctx, events, streamEvent)
}

func boundActivityMetadata(
	input map[string]string,
) map[string]string {
	if len(input) == 0 {
		return nil
	}
	output := make(map[string]string, min(len(input), maxActivityMetadata))
	for key, value := range input {
		if len(output) >= maxActivityMetadata {
			break
		}
		cleanKey := truncateRunes(key, 64)
		cleanValue := truncateRunes(value, maxActivityMetadataRunes)
		if cleanKey == "" || cleanValue == "" {
			continue
		}
		output[cleanKey] = cleanValue
	}
	if len(output) == 0 {
		return nil
	}
	return output
}
