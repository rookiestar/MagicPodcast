package processing

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"magicpodcast/internal/codexruntime"

	"github.com/stretchr/testify/require"
)

func TestLocalRuntimeAdapterReturnsStructuredEpisodeNotes(t *testing.T) {
	workRoot := t.TempDir()
	runtime := newAdapterFakeRuntime(json.RawMessage(
		`{"episode_notes":"# Notes\n\nVerified."}`,
	))
	adapter, err := NewLocalRuntimeAdapter(runtime, workRoot)
	require.NoError(t, err)

	result, err := adapter.Execute(context.Background(), RuntimeRequest{
		RunID:           42,
		EpisodeID:       7,
		PipelineVersion: "pipeline-v1",
		Transcript:      "[00:00] A verified transcript.",
	})
	require.NoError(t, err)
	require.Equal(t, "# Notes\n\nVerified.", result.EpisodeNotes)
	require.Equal(t, "fake-sdk-runtime-v1", result.RuntimeVersion)
	require.Equal(t, episodeNotesPromptVersion, result.PromptVersion)
	require.Empty(t, result.SkillVersions)

	request := runtime.request()
	require.Equal(t, codexruntime.ExecutionKindEpisodeNotes, request.Kind)
	require.Contains(t, request.Prompt, "<transcript>")
	require.NotContains(t, request.Prompt, "danger-full-access")
	require.JSONEq(t, string(episodeNotesOutputSchema), string(request.OutputSchema))
	resolvedRoot, err := filepath.EvalSymlinks(workRoot)
	require.NoError(t, err)
	relative, err := filepath.Rel(resolvedRoot, request.WorkingDirectory)
	require.NoError(t, err)
	require.NotEqual(t, ".", relative)
	require.NotContains(t, relative, "..")
	_, statErr := os.Stat(request.WorkingDirectory)
	require.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestLocalRuntimeAdapterCancellationTargetsActiveExecution(t *testing.T) {
	runtime := newAdapterBlockingRuntime()
	adapter, err := NewLocalRuntimeAdapter(runtime, t.TempDir())
	require.NoError(t, err)

	result := make(chan error, 1)
	go func() {
		_, executeErr := adapter.Execute(
			context.Background(),
			RuntimeRequest{
				RunID:           9,
				EpisodeID:       3,
				PipelineVersion: "pipeline-v1",
				Transcript:      "Transcript",
			},
		)
		result <- executeErr
	}()
	require.Eventually(t, runtime.started, 2*time.Second, 10*time.Millisecond)
	require.NoError(t, adapter.Cancel(context.Background(), 9))

	err = <-result
	var adapterErr *AdapterError
	require.ErrorAs(t, err, &adapterErr)
	require.Equal(t, "runtime_cancelled", adapterErr.ErrorCode)
	require.Equal(t, runtime.executionID(), runtime.cancelledExecutionID())
}

func TestLocalRuntimeAdapterContextCancellationReapsExecution(t *testing.T) {
	runtime := newAdapterBlockingRuntime()
	adapter, err := NewLocalRuntimeAdapter(runtime, t.TempDir())
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())

	result := make(chan error, 1)
	go func() {
		_, executeErr := adapter.Execute(ctx, RuntimeRequest{
			RunID:           10,
			EpisodeID:       4,
			PipelineVersion: "pipeline-v1",
			Transcript:      "Transcript",
		})
		result <- executeErr
	}()
	require.Eventually(t, runtime.started, 2*time.Second, 10*time.Millisecond)
	cancel()

	require.ErrorIs(t, <-result, context.Canceled)
	require.Equal(t, runtime.executionID(), runtime.cancelledExecutionID())
}

func TestLocalRuntimeAdapterSubscriptionFailureReapsExecution(t *testing.T) {
	runtime := newAdapterBlockingRuntime()
	runtime.subscribeErr = errors.New("subscription unavailable")
	adapter, err := NewLocalRuntimeAdapter(runtime, t.TempDir())
	require.NoError(t, err)

	_, err = adapter.Execute(context.Background(), RuntimeRequest{
		RunID:           11,
		EpisodeID:       5,
		PipelineVersion: "pipeline-v1",
		Transcript:      "Transcript",
	})
	var adapterErr *AdapterError
	require.ErrorAs(t, err, &adapterErr)
	require.Equal(t, "runtime_error", adapterErr.ErrorCode)
	require.Equal(t, runtime.executionID(), runtime.cancelledExecutionID())
}

func TestLocalRuntimeAdapterFailsClosedOnMalformedStructuredResult(
	t *testing.T,
) {
	runtime := newAdapterFakeRuntime(json.RawMessage(
		`{"episode_notes":"# Notes","unexpected":true}`,
	))
	adapter, err := NewLocalRuntimeAdapter(runtime, t.TempDir())
	require.NoError(t, err)

	_, err = adapter.Execute(context.Background(), RuntimeRequest{
		RunID:           10,
		EpisodeID:       4,
		PipelineVersion: "pipeline-v1",
		Transcript:      "Transcript",
	})
	var adapterErr *AdapterError
	require.ErrorAs(t, err, &adapterErr)
	require.Equal(t, "runtime_protocol_error", adapterErr.ErrorCode)
	require.False(t, adapterErr.CanRetry)
}

func TestLocalRuntimeAdapterMapsRuntimeUnavailableWithoutProviderFields(
	t *testing.T,
) {
	runtime := newAdapterFakeRuntime(nil)
	runtime.createErr = &codexruntime.RuntimeError{
		Code:        codexruntime.ErrorRuntimeUnavailable,
		SafeMessage: "runtime authentication is unavailable",
		Retryable:   false,
	}
	adapter, err := NewLocalRuntimeAdapter(runtime, t.TempDir())
	require.NoError(t, err)

	_, err = adapter.Execute(context.Background(), RuntimeRequest{
		RunID:           11,
		EpisodeID:       5,
		PipelineVersion: "pipeline-v1",
		Transcript:      "Transcript",
	})
	var adapterErr *AdapterError
	require.ErrorAs(t, err, &adapterErr)
	require.Equal(t, "runtime_unavailable", adapterErr.ErrorCode)
	require.NotContains(t, adapterErr.SafeMessage, "Codex")
	require.NotContains(t, adapterErr.SafeMessage, "OpenAI")
}

type adapterFakeRuntime struct {
	mu                 sync.Mutex
	nextID             int
	lastRequest        codexruntime.ExecutionRequest
	snapshots          map[codexruntime.ExecutionID]codexruntime.ExecutionSnapshot
	events             map[codexruntime.ExecutionID]chan codexruntime.Event
	result             json.RawMessage
	createErr          error
	subscribeErr       error
	block              bool
	cancelledID        codexruntime.ExecutionID
	startedExecutionID codexruntime.ExecutionID
}

func newAdapterFakeRuntime(result json.RawMessage) *adapterFakeRuntime {
	return &adapterFakeRuntime{
		snapshots: make(
			map[codexruntime.ExecutionID]codexruntime.ExecutionSnapshot,
		),
		events: make(
			map[codexruntime.ExecutionID]chan codexruntime.Event,
		),
		result: append(json.RawMessage(nil), result...),
	}
}

func newAdapterBlockingRuntime() *adapterFakeRuntime {
	runtime := newAdapterFakeRuntime(nil)
	runtime.block = true
	return runtime
}

func (f *adapterFakeRuntime) CreateExecution(
	_ context.Context,
	request codexruntime.ExecutionRequest,
) (codexruntime.ExecutionSnapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.createErr != nil {
		return codexruntime.ExecutionSnapshot{}, f.createErr
	}
	f.nextID++
	executionID := codexruntime.ExecutionID(
		"fake-execution-" + time.Now().UTC().Format("150405.000000000"),
	)
	f.lastRequest = request
	f.startedExecutionID = executionID
	now := time.Now().UTC()
	snapshot := codexruntime.ExecutionSnapshot{
		ID:             executionID,
		Kind:           request.Kind,
		Status:         codexruntime.StatusRunning,
		RuntimeVersion: "fake-sdk-runtime-v1",
		CreatedAt:      now,
		StartedAt:      &now,
	}
	events := make(chan codexruntime.Event, 3)
	events <- codexruntime.Event{
		ExecutionID: executionID,
		Sequence:    1,
		Type:        codexruntime.EventStarted,
	}
	if !f.block {
		snapshot.Status = codexruntime.StatusCompleted
		snapshot.Result = append(json.RawMessage(nil), f.result...)
		snapshot.CompletedAt = &now
		events <- codexruntime.Event{
			ExecutionID: executionID,
			Sequence:    2,
			Type:        codexruntime.EventOutputDelta,
			Text:        "notes",
		}
		events <- codexruntime.Event{
			ExecutionID: executionID,
			Sequence:    3,
			Type:        codexruntime.EventTerminal,
		}
		close(events)
	}
	f.snapshots[executionID] = snapshot
	f.events[executionID] = events
	return snapshot, nil
}

func (f *adapterFakeRuntime) SubscribeExecution(
	ctx context.Context,
	executionID codexruntime.ExecutionID,
) (<-chan codexruntime.Event, error) {
	f.mu.Lock()
	subscribeErr := f.subscribeErr
	source := f.events[executionID]
	f.mu.Unlock()
	if subscribeErr != nil {
		return nil, subscribeErr
	}
	if source == nil {
		return nil, errors.New("missing fake execution")
	}
	output := make(chan codexruntime.Event)
	go func() {
		defer close(output)
		for {
			select {
			case event, open := <-source:
				if !open {
					return
				}
				select {
				case output <- event:
				case <-ctx.Done():
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()
	return output, nil
}

func (f *adapterFakeRuntime) CancelExecution(
	_ context.Context,
	executionID codexruntime.ExecutionID,
) (codexruntime.CancellationResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	snapshot, exists := f.snapshots[executionID]
	if !exists {
		return codexruntime.CancellationResult{}, errors.New(
			"missing fake execution",
		)
	}
	f.cancelledID = executionID
	now := time.Now().UTC()
	snapshot.Status = codexruntime.StatusCancelled
	snapshot.CompletedAt = &now
	snapshot.CancellationMethod = codexruntime.CancellationNativeInterrupt
	f.snapshots[executionID] = snapshot
	if events := f.events[executionID]; events != nil {
		events <- codexruntime.Event{
			ExecutionID: executionID,
			Sequence:    2,
			Type:        codexruntime.EventTerminal,
		}
		close(events)
		f.events[executionID] = nil
	}
	return codexruntime.CancellationResult{
		ExecutionID: executionID,
		Status:      codexruntime.StatusCancelled,
		Method:      codexruntime.CancellationNativeInterrupt,
	}, nil
}

func (f *adapterFakeRuntime) GetExecution(
	_ context.Context,
	executionID codexruntime.ExecutionID,
) (codexruntime.ExecutionSnapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	snapshot, exists := f.snapshots[executionID]
	if !exists {
		return codexruntime.ExecutionSnapshot{}, errors.New(
			"missing fake execution",
		)
	}
	return snapshot, nil
}

func (f *adapterFakeRuntime) Close(context.Context) error {
	return nil
}

func (f *adapterFakeRuntime) request() codexruntime.ExecutionRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lastRequest
}

func (f *adapterFakeRuntime) started() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.startedExecutionID != ""
}

func (f *adapterFakeRuntime) executionID() codexruntime.ExecutionID {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.startedExecutionID
}

func (f *adapterFakeRuntime) cancelledExecutionID() codexruntime.ExecutionID {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.cancelledID
}
