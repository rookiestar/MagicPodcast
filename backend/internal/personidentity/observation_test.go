package personidentity

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"magicpodcast/internal/codexruntime"
	"magicpodcast/internal/models"
)

// scriptedRuntime replays a fixed provider-neutral event script and returns a
// fixed terminal snapshot, standing in for the ProcessHost.
type scriptedRuntime struct {
	mu        sync.Mutex
	events    []codexruntime.Event
	snapshot  codexruntime.ExecutionSnapshot
	createCtx context.Context
	cancelled bool
}

func (r *scriptedRuntime) CreateExecution(ctx context.Context, _ codexruntime.ExecutionRequest) (codexruntime.ExecutionSnapshot, error) {
	r.mu.Lock()
	r.createCtx = ctx
	r.mu.Unlock()
	return codexruntime.ExecutionSnapshot{ID: "exec-scripted", Status: codexruntime.StatusStarting, CreatedAt: time.Now().UTC()}, nil
}

func (r *scriptedRuntime) SubscribeExecution(ctx context.Context, _ codexruntime.ExecutionID) (<-chan codexruntime.Event, error) {
	output := make(chan codexruntime.Event, len(r.events))
	for index, event := range r.events {
		event.Sequence = uint64(index + 1)
		output <- event
	}
	close(output)
	_ = ctx
	return output, nil
}

func (r *scriptedRuntime) CancelExecution(_ context.Context, id codexruntime.ExecutionID) (codexruntime.CancellationResult, error) {
	r.mu.Lock()
	r.cancelled = true
	r.mu.Unlock()
	return codexruntime.CancellationResult{ExecutionID: id, Status: codexruntime.StatusCancelled}, nil
}

func (r *scriptedRuntime) GetExecution(_ context.Context, _ codexruntime.ExecutionID) (codexruntime.ExecutionSnapshot, error) {
	return r.snapshot, nil
}

func (r *scriptedRuntime) Close(context.Context) error { return nil }

func scriptedProgressEvent(category codexruntime.ProgressCategory, state codexruntime.ProgressState, metadata map[string]string) codexruntime.Event {
	return codexruntime.Event{
		Type: codexruntime.EventProgress,
		Progress: &codexruntime.Progress{
			ActivityID: "a1",
			Ordinal:    1,
			Category:   category,
			State:      state,
			Metadata:   metadata,
		},
	}
}

var scriptedIdentityResult = json.RawMessage(`{"people": [{"name": "林言", "kind": "participant", "status": "confirmed", "role": "host", "presence_basis": "self_introduction", "name_evidence": {"source": "show_notes", "fragment": 0, "quote": "主播林言"}, "presence_evidence": {"source": "transcript", "fragment": 1, "quote": "我是林言。"}, "role_evidence": {"source": "show_notes", "fragment": 0, "quote": "主播林言"}, "speech_bindings": [{"speaker_label": "A", "basis": "self_introduction", "evidence": {"source": "transcript", "fragment": 1, "quote": "我是林言。"}, "scope": "stable_speaker", "orders": [], "excluded_orders": []}]}], "speakers": [{"speaker_label": "A", "reason": "明确自我介绍", "candidates": [{"person_index": 0, "level": "direct", "basis": "self_introduction", "reason": "明确自我介绍", "evidence": [{"source": "transcript", "fragment": 1, "quote": "我是林言。"}], "counter_evidence": []}]}]}`)

func scriptedSources() EpisodeSources {
	return EpisodeSources{
		EpisodeID:     1,
		SourceKind:    SourceTranscript,
		SourceVersion: "artifact-19",
		ShowNotes:     "本集主播林言",
		Segments:      []Segment{{Order: 1, SpeakerLabel: "A", Text: "我是林言。"}},
	}
}

func collectPreparationReports(ctx context.Context, observation *Observation) (context.Context, *[]PreparationProgress) {
	reports := &[]PreparationProgress{}
	wired := WithPreparationProgress(WithObservation(ctx, observation), func(event PreparationProgress) {
		*reports = append(*reports, event)
	})
	return wired, reports
}

func TestRuntimeSuggesterObservesConnectionRecovery(t *testing.T) {
	runtime := &scriptedRuntime{
		events: []codexruntime.Event{
			{Type: codexruntime.EventStarted},
			scriptedProgressEvent(codexruntime.CategoryConnection, codexruntime.ProgressStarted, map[string]string{"will_retry": "true", "error_class": "response_stream_disconnected", "http_status": "502"}),
			scriptedProgressEvent(codexruntime.CategoryConnection, codexruntime.ProgressUpdated, map[string]string{"will_retry": "true", "error_class": "server_overloaded"}),
			scriptedProgressEvent(codexruntime.CategoryTurn, codexruntime.ProgressStarted, nil),
			{Type: codexruntime.EventOutputDelta, Text: "{\"people\":"},
			{Type: codexruntime.EventOutputDelta, Text: "1}"},
			{Type: codexruntime.EventTerminal},
		},
		snapshot: codexruntime.ExecutionSnapshot{
			ID:             "exec-scripted",
			Status:         codexruntime.StatusCompleted,
			Result:         scriptedIdentityResult,
			RuntimeVersion: "sdk/0.147.0;runtime/0.147.0",
		},
	}
	observation := NewObservation("req-obs-1", 1)
	ctx, reports := collectPreparationReports(context.Background(), observation)
	suggester := NewRuntimeSuggester(runtime, t.TempDir())

	suggestions, err := suggester.Suggest(ctx, scriptedSources())
	require.NoError(t, err)
	require.Len(t, suggestions.Matches, 1)
	require.Equal(t, "林言", suggestions.Matches[0].DisplayName)

	summary := observation.Summary("completed", nil)
	require.Equal(t, "exec-scripted", summary["execution_id"])
	require.Equal(t, "artifact-19", summary["source_version"])
	require.Equal(t, "sdk/0.147.0;runtime/0.147.0", summary["runtime_version"])
	require.Greater(t, summary["result_bytes"], 0)
	require.Equal(t, 2, summary["reconnect_events_observed"])
	require.Equal(t, 2, summary["output_deltas"])
	for _, phase := range []string{
		"runtime_ready_ms", "first_provider_event_ms", "first_output_ms",
		"runtime_wait_ms", "decode_validate_ms",
	} {
		require.Contains(t, summary, phase, "reached phase %s must be reported", phase)
	}
	require.NotContains(t, summary, "cancel_cleanup_ms", "a completed run never performs cancellation cleanup")

	// Runtime phases reach the caller through the existing progress seam.
	// Every reconnect signal is visible and bounded; generating wins after
	// real output, and waiting never downgrades it.
	phases := []string{}
	for _, report := range *reports {
		if report.Runtime != nil {
			phases = append(phases, report.Runtime.Phase)
		}
	}
	require.Equal(t, []string{RuntimePhaseReady, RuntimePhaseReconnecting, RuntimePhaseReconnecting, RuntimePhaseGenerating}, phases)
	reconnecting := (*reports)[1].Runtime
	require.NotNil(t, reconnecting.WillRetry)
	require.True(t, *reconnecting.WillRetry)
}

func TestRuntimeSuggesterRecordsUnreachedPhasesAsNull(t *testing.T) {
	runtime := &scriptedRuntime{
		events: []codexruntime.Event{
			{Type: codexruntime.EventStarted},
			scriptedProgressEvent(codexruntime.CategoryConnection, codexruntime.ProgressStarted, map[string]string{"will_retry": "true", "error_class": "http_connection_failed", "http_status": "504"}),
			{Type: codexruntime.EventTerminal},
		},
		snapshot: codexruntime.ExecutionSnapshot{
			ID:          "exec-failed",
			Status:      codexruntime.StatusFailed,
			ErrorCode:   codexruntime.ErrorRuntimeUnavailable,
			SafeMessage: "runtime preflight failed",
		},
	}
	observation := NewObservation("req-obs-2", 1)
	ctx, _ := collectPreparationReports(context.Background(), observation)
	suggester := NewRuntimeSuggester(runtime, t.TempDir())

	_, err := suggester.Suggest(ctx, scriptedSources())
	require.Error(t, err)
	require.Equal(t, FailureRuntimeUnavailable, ClassifyFailure(err).Code)
	require.True(t, ClassifyFailure(err).Retryable)

	summary := observation.Summary("failed", nil)
	require.Equal(t, FailureConnection, summary["first_error_class"])
	require.Equal(t, FailureRuntimeUnavailable, summary["last_error_class"])
	for _, unreached := range []string{
		"first_output_ms", "decode_validate_ms", "save_ms",
	} {
		require.NotContains(t, summary, unreached, "unreached phase %s must stay null", unreached)
	}
	for _, reached := range []string{"runtime_ready_ms", "first_provider_event_ms", "runtime_wait_ms"} {
		require.Contains(t, summary, reached, "reached phase %s must be reported", reached)
	}
	// A terminal provider failure never needs cancellation cleanup; that phase
	// only exists when the parent abandons a still-running execution.
	require.NotContains(t, summary, "cancel_cleanup_ms")
}

func TestRuntimeSuggesterKeepsCallerDeadlineAndBoundedDefault(t *testing.T) {
	runtime := &scriptedRuntime{
		snapshot: codexruntime.ExecutionSnapshot{
			ID:     "exec-scripted",
			Status: codexruntime.StatusCompleted,
			Result: scriptedIdentityResult,
		},
	}
	suggester := NewRuntimeSuggester(runtime, t.TempDir())

	// A caller without a deadline gets the bounded budget.
	observation := NewObservation("req-deadline-1", 1)
	ctx, _ := collectPreparationReports(context.Background(), observation)
	_, err := suggester.Suggest(ctx, scriptedSources())
	require.NoError(t, err)
	deadline, ok := runtime.createCtx.Deadline()
	require.True(t, ok, "suggest must keep a bounded budget without a caller deadline")
	require.Greater(t, time.Until(deadline), time.Duration(0))
	require.LessOrEqual(t, time.Until(deadline), 151*time.Second)

	// A tighter caller deadline wins; the inner 150-second race is gone.
	runtime2 := &scriptedRuntime{
		snapshot: codexruntime.ExecutionSnapshot{
			ID:     "exec-scripted-2",
			Status: codexruntime.StatusCompleted,
			Result: scriptedIdentityResult,
		},
	}
	suggester2 := NewRuntimeSuggester(runtime2, t.TempDir())
	callerCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	observation2 := NewObservation("req-deadline-2", 1)
	ctx2, _ := collectPreparationReports(callerCtx, observation2)
	_, err = suggester2.Suggest(ctx2, scriptedSources())
	require.NoError(t, err)
	innerDeadline, ok := runtime2.createCtx.Deadline()
	require.True(t, ok)
	require.Less(t, innerDeadline.Sub(time.Now()), 3*time.Second, "the caller deadline must stay effective")
}

func TestClassifyFailureDistinguishesUserActions(t *testing.T) {
	scenarios := []struct {
		name      string
		err       error
		code      string
		retryable bool
	}{
		{"sources_changed", ErrSourcesChanged, FailureSourcesChanged, false},
		{"deadline", context.DeadlineExceeded, FailureDeadline, true},
		{"cancelled", context.Canceled, FailureCancelled, false},
		{"runtime_unavailable", &codexruntime.RuntimeError{Code: codexruntime.ErrorRuntimeUnavailable, Retryable: true}, FailureRuntimeUnavailable, true},
		{"protocol", &codexruntime.RuntimeError{Code: codexruntime.ErrorProtocol}, FailureRuntimeProtocol, false},
		{"profile", &codexruntime.RuntimeError{Code: codexruntime.ErrorProfileUnavailable}, FailureProfileUnavailable, false},
		{"save", &SaveError{Err: errors.New("private database failure")}, FailureSaveFailed, false},
		{"invalid_result", &InvalidResultError{Err: errors.New("evidence missing")}, FailureInvalidResult, false},
		{"save_keeps_sources_changed", &SaveError{Err: ErrSourcesChanged}, FailureSourcesChanged, false},
		{"unknown", errors.New("private runtime diagnostics must not be exposed"), FailureUnknown, false},
	}
	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			class := ClassifyFailure(scenario.err)
			require.Equal(t, scenario.code, class.Code)
			require.Equal(t, scenario.retryable, class.Retryable)
		})
	}
}

func TestPrepareWrapsSaveFailuresWithoutLosingSourceConflicts(t *testing.T) {
	db := openPersonIdentityDB(t)
	pod := models.Podcast{XYZID: "observation-save", Title: "访谈", FeedURL: "https://example.test/obs"}
	require.NoError(t, db.Create(&pod).Error)
	ep := models.Episode{PodcastID: pod.ID, GUID: "obs", Title: "对话"}
	require.NoError(t, db.Create(&ep).Error)
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("fail_draft_save_obs", func(tx *gorm.DB) {
		if tx.Statement.Table == "person_drafts" {
			tx.AddError(errors.New("private database failure"))
		}
	}))

	runtime := &scriptedRuntime{
		snapshot: codexruntime.ExecutionSnapshot{
			ID:             "exec-save",
			Status:         codexruntime.StatusCompleted,
			Result:         scriptedIdentityResult,
			RuntimeVersion: "sdk/0.147.0;runtime/0.147.0",
		},
	}
	observation := NewObservation("req-save-1", ep.ID)
	ctx, _ := collectPreparationReports(context.Background(), observation)
	service, err := NewService(db, NewRuntimeSuggester(runtime, t.TempDir()))
	require.NoError(t, err)

	sources := scriptedSources()
	sources.EpisodeID = ep.ID
	// No artifact exists for this episode, so the run must reach save with the
	// non-artifact source version instead of failing the source check first.
	sources.SourceVersion = "v1"
	_, err = service.Prepare(ctx, sources)
	require.Error(t, err)
	require.Equal(t, FailureSaveFailed, ClassifyFailure(err).Code)
	summary := observation.Summary("failed", nil)
	require.Equal(t, FailureSaveFailed, summary["last_error_class"])
	require.Contains(t, summary, "save_ms", "a failed save still records the save phase")
}

func TestRuntimeSuggesterDoesNotExpandBudgetForLongerCaller(t *testing.T) {
	r := &scriptedRuntime{snapshot: codexruntime.ExecutionSnapshot{Status: codexruntime.StatusCompleted, Result: scriptedIdentityResult}}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	_, err := NewRuntimeSuggester(r, t.TempDir()).Suggest(ctx, scriptedSources())
	require.NoError(t, err)
	deadline, ok := r.createCtx.Deadline()
	require.True(t, ok)
	require.LessOrEqual(t, time.Until(deadline), 150*time.Second)
}

func TestRuntimeSuggesterReportsFailedValidationDuration(t *testing.T) {
	r := &scriptedRuntime{events: []codexruntime.Event{{Type: codexruntime.EventStarted}, {Type: codexruntime.EventTerminal}}, snapshot: codexruntime.ExecutionSnapshot{Status: codexruntime.StatusCompleted, Result: json.RawMessage(`{"people": "invalid"}`)}}
	o := NewObservation("review-validation", 1)
	_, err := NewRuntimeSuggester(r, t.TempDir()).Suggest(WithObservation(context.Background(), o), scriptedSources())
	require.Error(t, err)
	summary := o.Summary("failed", nil)
	require.Contains(t, summary, "decode_validate_ms")
	require.Contains(t, summary, "total_ms")
}

func TestPreparationActivityRefreshesWhileGeneratingWithoutFakingRecovery(t *testing.T) {
	o := NewObservation("review-activity", 1)
	ctx, reports := collectPreparationReports(context.Background(), o)
	o.observeRuntimeEvent(ctx, codexruntime.Event{Type: codexruntime.EventOutputDelta, Text: "first"})
	o.lastReportedAt = time.Now().Add(-2 * time.Second)
	o.observeRuntimeEvent(ctx, codexruntime.Event{Type: codexruntime.EventOutputDelta, Text: "next"})
	require.Len(t, *reports, 2, "same-phase output must refresh the activity timestamp")
	require.NotNil(t, (*reports)[1].Runtime.LastActivityAgeMS)
	o.observeRuntimeEvent(ctx, scriptedProgressEvent(codexruntime.CategoryConnection, codexruntime.ProgressStarted, map[string]string{"will_retry": "true"}))
	o.observeRuntimeEvent(ctx, scriptedProgressEvent(codexruntime.CategoryGenericItem, codexruntime.ProgressCompleted, nil))
	require.Equal(t, RuntimePhaseReconnecting, (*reports)[len(*reports)-1].Runtime.Phase, "local user-message completion does not prove reconnection")
}
