package codexruntime

import (
	"context"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestProcessHostStreamsValidProgressActivities(t *testing.T) {
	host, workRoot := newHelperHost(t, "progress_ok")

	snapshot, err := host.CreateExecution(context.Background(), ExecutionRequest{
		Kind:             ExecutionKindEpisodeNotes,
		WorkingDirectory: newExecutionDir(t, workRoot, "progress-ok-"),
		Prompt:           "success",
		OutputSchema:     episodeNotesSchema,
	})
	require.NoError(t, err)
	events, err := host.SubscribeExecution(context.Background(), snapshot.ID)
	require.NoError(t, err)
	received := collectEvents(events)

	require.Equal(
		t,
		[]EventType{
			EventStarted,
			EventProgress,
			EventProgress,
			EventProgress,
			EventProgress,
			EventOutputDelta,
			EventTerminal,
		},
		eventTypes(received),
	)
	for index, event := range received {
		require.Equal(t, snapshot.ID, event.ExecutionID)
		require.Equal(t, uint64(index+1), event.Sequence)
		require.False(t, event.ObservedAt.IsZero())
	}

	first := received[1].Progress
	require.NotNil(t, first)
	require.Equal(t, "a1", first.ActivityID)
	require.Equal(t, uint64(1), first.Ordinal)
	require.Equal(t, CategoryWebSearch, first.Category)
	require.Equal(t, ProgressStarted, first.State)
	require.Equal(t, "contract topic", first.DisplayText)
	require.Equal(t, map[string]string{
		"candidate_domains": "a.example.com",
	}, first.Metadata)

	updated := received[2].Progress
	require.Equal(t, ProgressUpdated, updated.State)
	require.Equal(t, int64(120), updated.ElapsedMS)

	completed := received[3].Progress
	require.Equal(t, ProgressCompleted, completed.State)
	require.Equal(t, map[string]string{
		"candidate_domains": "a.example.com b.example.org",
		"candidate_count":   "2",
	}, completed.Metadata)

	second := received[4].Progress
	require.Equal(t, "a2", second.ActivityID)
	require.Equal(t, uint64(2), second.Ordinal)
	require.Equal(t, CategoryReasoning, second.Category)

	final, err := host.GetExecution(context.Background(), snapshot.ID)
	require.NoError(t, err)
	require.Equal(t, StatusCompleted, final.Status)
	require.NoError(t, closeHost(t, host))
}

func TestProcessHostFailsClosedOnProgressProtocolViolations(t *testing.T) {
	scenarios := []struct {
		violation string
	}{
		{violation: "update_unknown_activity"},
		{violation: "complete_unknown_activity"},
		{violation: "duplicate_start"},
		{violation: "update_after_closed"},
		{violation: "progress_after_terminal"},
		{violation: "progress_carries_text"},
		{violation: "ordinal_gap"},
		{violation: "invalid_state"},
		{violation: "invalid_category"},
		{violation: "oversized_text"},
		{violation: "too_many_metadata"},
		{violation: "oversized_metadata_value"},
		{violation: "empty_activity_id"},
		{violation: "zero_ordinal"},
	}
	for _, scenario := range scenarios {
		t.Run(scenario.violation, func(t *testing.T) {
			host, workRoot := newHelperHost(t, "progress_violation_flow")
			snapshot, err := host.CreateExecution(
				context.Background(),
				ExecutionRequest{
					Kind:             ExecutionKindEpisodeNotes,
					WorkingDirectory: newExecutionDir(t, workRoot, "progress-bad-"),
					Prompt:           scenario.violation,
					OutputSchema:     episodeNotesSchema,
				},
			)
			if err == nil {
				events, subscribeErr := host.SubscribeExecution(
					context.Background(),
					snapshot.ID,
				)
				require.NoError(t, subscribeErr)
				_ = collectEvents(events)
			}
			final, getErr := host.GetExecution(
				context.Background(),
				snapshot.ID,
			)
			require.NoError(t, getErr)
			require.Equal(t, StatusFailed, final.Status)
			require.Equal(t, ErrorProtocol, final.ErrorCode)
			require.NoError(t, closeHost(t, host))
		})
	}
}

// TestPythonSDKHostStreamsSanitizedNeutralProgress is the Host conformance
// seam: the Fake SDK emits stable Turn, web search, reasoning summary, plan,
// agent text, unknown, and terminal notifications, and the host must surface
// provider-neutral sanitized progress without leaking identities, raw
// reasoning, managed paths, or provider payloads.
func TestPythonSDKHostStreamsSanitizedNeutralProgress(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 is not available")
	}
	_, currentFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	packageDir := filepath.Dir(currentFile)
	fakeSDK := filepath.Join(packageDir, "testdata", "fake_sdk")
	workRoot := t.TempDir()
	environment, _ := fakeSDKEnvironment(t, fakeSDK, nil)
	host, err := NewProcessHost(ProcessHostConfig{
		Command: []string{
			python,
			filepath.Join(packageDir, "runtime_host.py"),
		},
		WorkRoot:        workRoot,
		testEnvironment: environment,
		StartupTimeout:  5 * time.Second,
	})
	require.NoError(t, err)

	workDir := newExecutionDir(t, workRoot, "progress-flow-")
	snapshot, err := host.CreateExecution(
		context.Background(),
		ExecutionRequest{
			Kind:             ExecutionKindAssistant,
			WorkingDirectory: workDir,
			Prompt:           "PROGRESS_FLOW",
		},
	)
	require.NoError(t, err)
	events, err := host.SubscribeExecution(context.Background(), snapshot.ID)
	require.NoError(t, err)
	received := collectEvents(events)

	require.NotEmpty(t, received)
	require.Equal(t, EventStarted, received[0].Type)
	require.Equal(t, EventTerminal, received[len(received)-1].Type)

	var progressEvents []Progress
	for _, event := range received {
		require.Equal(t, snapshot.ID, event.ExecutionID)
		if event.Type == EventProgress {
			require.NotNil(t, event.Progress)
			progressEvents = append(progressEvents, *event.Progress)
		}
	}
	require.NotEmpty(t, progressEvents)

	neutralID := regexp.MustCompile(`^a[0-9]+$`)
	expectedOrdinals := map[string]uint64{}
	nextOrdinal := uint64(0)
	for _, progress := range progressEvents {
		require.Regexp(t, neutralID, progress.ActivityID)
		switch progress.State {
		case ProgressStarted:
			nextOrdinal++
			require.Equal(t, nextOrdinal, progress.Ordinal)
			expectedOrdinals[progress.ActivityID] = progress.Ordinal
		case ProgressUpdated, ProgressCompleted:
			require.Equal(
				t,
				expectedOrdinals[progress.ActivityID],
				progress.Ordinal,
			)
		default:
			t.Fatalf("unexpected progress state %q", progress.State)
		}
	}

	for _, progress := range progressEvents {
		text := progress.DisplayText
		require.LessOrEqual(t, len([]rune(text)), maxProgressTextRunes+1)
		for _, banned := range []string{
			"\n",
			"\x00",
			"PROGRESS_FLOW",
			"managed-notes.txt",
			workDir,
			"PRIVATE MODEL THOUGHTS",
			"RAW CHAIN OF THOUGHT",
			"should never reach progress frames",
			"/a/path?q=1",
			"https://",
		} {
			require.NotContains(t, text, banned)
		}
		for key, value := range progress.Metadata {
			require.NotContains(t, value, "user:leak")
			require.NotContains(t, value, "file://")
			require.NotContains(t, value, "/a/path?q=1")
			require.NotContains(t, key, "https://")
		}
	}

	// turn started, web search, reasoning, plan, agent message lifecycle.
	require.Equal(t, CategoryTurn, progressEvents[0].Category)
	require.Equal(t, ProgressStarted, progressEvents[0].State)

	var searchStates []ProgressState
	var reasoningStates []ProgressState
	var planStates []ProgressState
	var agentStates []ProgressState
	for _, progress := range progressEvents {
		switch progress.Category {
		case CategoryWebSearch:
			searchStates = append(searchStates, progress.State)
			if progress.State == ProgressCompleted {
				require.Equal(t, "episode contract", progress.DisplayText)
				require.Equal(t, map[string]string{
					"candidate_domains": "authority.example.com",
					"candidate_count":   "5",
				}, progress.Metadata)
			}
		case CategoryReasoning:
			reasoningStates = append(reasoningStates, progress.State)
			if progress.State == ProgressCompleted {
				require.Equal(t, "Final visible summary.", progress.DisplayText)
			}
		case CategoryPlan:
			planStates = append(planStates, progress.State)
			if progress.State == ProgressCompleted {
				require.Equal(t, "先核对来源，再回答。", progress.DisplayText)
			}
		case CategoryAgentMsg:
			agentStates = append(agentStates, progress.State)
		}
	}
	require.Equal(t, []ProgressState{ProgressStarted, ProgressCompleted}, searchStates)
	require.Equal(t, ProgressStarted, reasoningStates[0])
	require.Equal(t, ProgressCompleted, reasoningStates[len(reasoningStates)-1])
	require.GreaterOrEqual(t, len(reasoningStates), 3)
	require.Equal(t, []ProgressState{ProgressStarted, ProgressCompleted}, planStates)
	require.Equal(t, []ProgressState{ProgressStarted, ProgressCompleted}, agentStates)

	final, err := host.GetExecution(context.Background(), snapshot.ID)
	require.NoError(t, err)
	require.Equal(t, StatusCompleted, final.Status)
	require.NoError(t, closeHost(t, host))
}

// TestPythonSDKHostKeepsProgressBeforeCancellation proves cancellation stays
// targeted: activities observed before the interrupt are preserved and no
// progress follows the terminal event.
func TestPythonSDKHostKeepsProgressBeforeCancellation(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 is not available")
	}
	_, currentFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	packageDir := filepath.Dir(currentFile)
	fakeSDK := filepath.Join(packageDir, "testdata", "fake_sdk")
	workRoot := t.TempDir()
	environment, _ := fakeSDKEnvironment(t, fakeSDK, nil)
	host, err := NewProcessHost(ProcessHostConfig{
		Command: []string{
			python,
			filepath.Join(packageDir, "runtime_host.py"),
		},
		WorkRoot:            workRoot,
		testEnvironment:     environment,
		StartupTimeout:      5 * time.Second,
		NativeCancelTimeout: 500 * time.Millisecond,
		TerminateTimeout:    500 * time.Millisecond,
		KillTimeout:         500 * time.Millisecond,
	})
	require.NoError(t, err)

	snapshot, err := host.CreateExecution(
		context.Background(),
		ExecutionRequest{
			Kind:             ExecutionKindAssistant,
			WorkingDirectory: newExecutionDir(t, workRoot, "progress-cancel-"),
			Prompt:           "PROGRESS_FLOW BLOCK_UNTIL_CANCEL",
		},
	)
	require.NoError(t, err)
	events, err := host.SubscribeExecution(context.Background(), snapshot.ID)
	require.NoError(t, err)

	var received []Event
	cancelIssued := false
	for event := range events {
		received = append(received, event)
		if event.Type == EventOutputDelta && !cancelIssued {
			cancelIssued = true
			result, cancelErr := host.CancelExecution(
				context.Background(),
				snapshot.ID,
			)
			require.NoError(t, cancelErr)
			require.Equal(t, StatusCancelled, result.Status)
		}
	}
	// The terminal event closes the stream: activities observed before it
	// are preserved and nothing may follow it.
	require.NotEmpty(t, received)
	require.Equal(t, EventTerminal, received[len(received)-1].Type)
	progressBeforeTerminal := 0
	for index, event := range received {
		if index == len(received)-1 {
			continue
		}
		if event.Type == EventProgress {
			progressBeforeTerminal++
		}
	}
	require.NotEqual(t, 0, progressBeforeTerminal)

	final, err := host.GetExecution(context.Background(), snapshot.ID)
	require.NoError(t, err)
	require.Equal(t, StatusCancelled, final.Status)
	require.NoError(t, closeHost(t, host))
	require.Equal(t, 0, host.Diagnostics().LiveProcessGroups)
}

func TestProgressJSONContractStaysBounded(t *testing.T) {
	// The wire payload of one progress frame stays within the module bounds
	// even for maximal metadata, so protocol upgrade can never silently
	// widen the event budget.
	progress := Progress{
		ActivityID:  strings.Repeat("a", maxActivityIDBytes),
		Ordinal:     1,
		Category:    CategoryWebSearch,
		State:       ProgressStarted,
		DisplayText: strings.Repeat("题", maxProgressTextRunes),
		Metadata: map[string]string{
			"candidate_domains": strings.Repeat("d", maxMetadataValueRunes),
		},
	}
	encoded, err := json.Marshal(progress)
	require.NoError(t, err)
	require.LessOrEqual(t, len(encoded), 2*1024)
	require.LessOrEqual(
		t,
		progressEventBytes(&progress),
		8*1024,
	)
}
