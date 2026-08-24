package codexruntime

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

var episodeNotesSchema = json.RawMessage(`{
	"type":"object",
	"additionalProperties":false,
	"properties":{"episode_notes":{"type":"string"}},
	"required":["episode_notes"]
}`)

func TestProcessHostConformanceSuccessStreamsAndReplays(t *testing.T) {
	host, workRoot := newHelperHost(t, "success")
	workDir := newExecutionDir(t, workRoot, "success-")

	snapshot, err := host.CreateExecution(context.Background(), ExecutionRequest{
		Kind:             ExecutionKindEpisodeNotes,
		WorkingDirectory: workDir,
		Prompt:           "success",
		OutputSchema:     episodeNotesSchema,
	})
	require.NoError(t, err)
	require.Contains(
		t,
		[]ExecutionStatus{StatusRunning, StatusCompleted},
		snapshot.Status,
	)
	require.Equal(t, "helper-runtime-1", snapshot.RuntimeVersion)

	events, err := host.SubscribeExecution(
		context.Background(),
		snapshot.ID,
	)
	require.NoError(t, err)
	var received []Event
	for event := range events {
		received = append(received, event)
	}
	require.Equal(
		t,
		[]EventType{EventStarted, EventOutputDelta, EventTerminal},
		eventTypes(received),
	)
	for index, event := range received {
		require.Equal(t, snapshot.ID, event.ExecutionID)
		require.Equal(t, uint64(index+1), event.Sequence)
	}

	final, err := host.GetExecution(context.Background(), snapshot.ID)
	require.NoError(t, err)
	require.Equal(t, StatusCompleted, final.Status)
	require.JSONEq(
		t,
		`{"episode_notes":"# Helper notes"}`,
		string(final.Result),
	)

	replayed, err := host.SubscribeExecution(
		context.Background(),
		snapshot.ID,
	)
	require.NoError(t, err)
	require.Len(t, collectEvents(replayed), 3)
	require.NoError(t, closeHost(t, host))
	require.Equal(t, 0, host.Diagnostics().LiveProcessGroups)
}

func TestProcessHostBoundsTerminalRetention(t *testing.T) {
	host, workRoot := newHelperHost(t, "success")
	host.config.MaxRetainedExecutions = 2

	var completed []ExecutionID
	for index := 0; index < 3; index++ {
		snapshot, err := host.CreateExecution(
			context.Background(),
			ExecutionRequest{
				Kind: ExecutionKindEpisodeNotes,
				WorkingDirectory: newExecutionDir(
					t,
					workRoot,
					fmt.Sprintf("retained-%d-", index),
				),
				Prompt:       "success",
				OutputSchema: episodeNotesSchema,
			},
		)
		require.NoError(t, err)
		events, err := host.SubscribeExecution(
			context.Background(),
			snapshot.ID,
		)
		require.NoError(t, err)
		_ = collectEvents(events)
		final, err := host.GetExecution(context.Background(), snapshot.ID)
		require.NoError(t, err)
		require.Equal(t, StatusCompleted, final.Status)
		completed = append(completed, snapshot.ID)
	}

	require.Equal(t, 2, host.Diagnostics().TrackedExecutions)
	_, err := host.GetExecution(context.Background(), completed[0])
	require.Error(t, err)
	require.Equal(t, ErrorExecutionNotFound, ErrorCode(err))
	for _, executionID := range completed[1:] {
		snapshot, getErr := host.GetExecution(
			context.Background(),
			executionID,
		)
		require.NoError(t, getErr)
		require.Equal(t, StatusCompleted, snapshot.Status)
	}
	require.NoError(t, closeHost(t, host))
}

func TestProcessHostRetainsTerminalExecutionForActiveSubscriberResultRead(
	t *testing.T,
) {
	host, workRoot := newHelperHost(t, "success")
	host.config.MaxRetainedExecutions = 1
	host.config.SubscriberBufferSize = 1
	host.config.ResultReadGrace = time.Second

	longRunning, err := host.CreateExecution(
		context.Background(),
		ExecutionRequest{
			Kind:             ExecutionKindEpisodeNotes,
			WorkingDirectory: newExecutionDir(t, workRoot, "consumer-"),
			Prompt:           "success",
			OutputSchema:     episodeNotesSchema,
		},
	)
	require.NoError(t, err)
	longEvents, err := host.SubscribeExecution(
		context.Background(),
		longRunning.ID,
	)
	require.NoError(t, err)

	for index := 0; index < 2; index++ {
		snapshot, createErr := host.CreateExecution(
			context.Background(),
			ExecutionRequest{
				Kind: ExecutionKindEpisodeNotes,
				WorkingDirectory: newExecutionDir(
					t,
					workRoot,
					fmt.Sprintf("newer-%d-", index),
				),
				Prompt:       "success",
				OutputSchema: episodeNotesSchema,
			},
		)
		require.NoError(t, createErr)
		events, subscribeErr := host.SubscribeExecution(
			context.Background(),
			snapshot.ID,
		)
		require.NoError(t, subscribeErr)
		_ = collectEvents(events)
		_, getErr := host.GetExecution(context.Background(), snapshot.ID)
		require.NoError(t, getErr)
	}

	_, err = host.execution(longRunning.ID)
	require.NoError(t, err)
	_ = collectEvents(longEvents)
	final, err := host.GetExecution(context.Background(), longRunning.ID)
	require.NoError(t, err)
	require.Equal(t, StatusCompleted, final.Status)
	require.JSONEq(
		t,
		`{"episode_notes":"# Helper notes"}`,
		string(final.Result),
	)
	require.NoError(t, closeHost(t, host))
}

func TestProcessHostEvictsExpiredResultLeaseWithoutNewLifecycleEvent(
	t *testing.T,
) {
	host, workRoot := newHelperHost(t, "success")
	host.config.MaxRetainedExecutions = 1
	host.config.ResultReadGrace = 50 * time.Millisecond

	var completed []ExecutionID
	for index := 0; index < 2; index++ {
		snapshot, err := host.CreateExecution(
			context.Background(),
			ExecutionRequest{
				Kind: ExecutionKindEpisodeNotes,
				WorkingDirectory: newExecutionDir(
					t,
					workRoot,
					fmt.Sprintf("lease-%d-", index),
				),
				Prompt:       "success",
				OutputSchema: episodeNotesSchema,
			},
		)
		require.NoError(t, err)
		events, err := host.SubscribeExecution(
			context.Background(),
			snapshot.ID,
		)
		require.NoError(t, err)
		_ = collectEvents(events)
		completed = append(completed, snapshot.ID)
	}

	require.Eventually(t, func() bool {
		return host.Diagnostics().TrackedExecutions == 1
	}, time.Second, 10*time.Millisecond)
	_, err := host.GetExecution(context.Background(), completed[0])
	require.Error(t, err)
	require.Equal(t, ErrorExecutionNotFound, ErrorCode(err))
	require.NoError(t, closeHost(t, host))
}

func TestProcessHostFailsClosedOnIdentityOrderingAndProtocolConflicts(
	t *testing.T,
) {
	scenarios := []string{
		"wrong_identity",
		"missing_identity",
		"out_of_order",
		"malformed",
		"no_terminal",
		"terminal_then_invalid",
		"terminal_hang",
	}
	for _, scenario := range scenarios {
		t.Run(scenario, func(t *testing.T) {
			host, workRoot := newHelperHost(t, scenario)
			snapshot, err := host.CreateExecution(
				context.Background(),
				ExecutionRequest{
					Kind:             ExecutionKindEpisodeNotes,
					WorkingDirectory: newExecutionDir(t, workRoot, scenario+"-"),
					Prompt:           scenario,
					OutputSchema:     episodeNotesSchema,
				},
			)
			if err != nil {
				require.Equal(t, ErrorProtocol, ErrorCode(err))
			} else {
				events, subscribeErr := host.SubscribeExecution(
					context.Background(),
					snapshot.ID,
				)
				require.NoError(t, subscribeErr)
				_ = collectEvents(events)
				final, getErr := host.GetExecution(
					context.Background(),
					snapshot.ID,
				)
				require.NoError(t, getErr)
				require.Equal(t, StatusFailed, final.Status)
				require.Equal(t, ErrorProtocol, final.ErrorCode)
			}
			require.NoError(t, closeHost(t, host))
			require.Equal(t, 0, host.Diagnostics().LiveProcessGroups)
		})
	}
}

func TestProcessHostNativeCancellationAndTargetIsolation(t *testing.T) {
	host, workRoot := newHelperHost(t, "from_prompt")
	blockingDir := newExecutionDir(t, workRoot, "blocking-")
	successDir := newExecutionDir(t, workRoot, "success-")

	blocking, err := host.CreateExecution(
		context.Background(),
		ExecutionRequest{
			Kind:             ExecutionKindEpisodeNotes,
			WorkingDirectory: blockingDir,
			Prompt:           "block",
			OutputSchema:     episodeNotesSchema,
		},
	)
	require.NoError(t, err)

	var success ExecutionSnapshot
	var successErr error
	var waitGroup sync.WaitGroup
	waitGroup.Add(1)
	go func() {
		defer waitGroup.Done()
		success, successErr = host.CreateExecution(
			context.Background(),
			ExecutionRequest{
				Kind:             ExecutionKindEpisodeNotes,
				WorkingDirectory: successDir,
				Prompt:           "success",
				OutputSchema:     episodeNotesSchema,
			},
		)
	}()

	cancelled, err := host.CancelExecution(
		context.Background(),
		blocking.ID,
	)
	require.NoError(t, err)
	require.Equal(t, StatusCancelled, cancelled.Status)
	require.Equal(t, CancellationNativeInterrupt, cancelled.Method)
	waitGroup.Wait()
	require.NoError(t, successErr)

	successEvents, err := host.SubscribeExecution(
		context.Background(),
		success.ID,
	)
	require.NoError(t, err)
	_ = collectEvents(successEvents)
	successFinal, err := host.GetExecution(context.Background(), success.ID)
	require.NoError(t, err)
	require.Equal(t, StatusCompleted, successFinal.Status)

	blockingFinal, err := host.GetExecution(context.Background(), blocking.ID)
	require.NoError(t, err)
	require.Equal(t, StatusCancelled, blockingFinal.Status)
	require.Equal(
		t,
		CancellationNativeInterrupt,
		blockingFinal.CancellationMethod,
	)
	require.NoError(t, closeHost(t, host))
	require.Equal(t, 0, host.Diagnostics().LiveProcessGroups)
}

func TestProcessHostEscalatesOnlyTargetProcessGroupAndReapsChildren(
	t *testing.T,
) {
	host, workRoot := newHelperHost(t, "ignore_cancel")
	workDir := newExecutionDir(t, workRoot, "fallback-")
	snapshot, err := host.CreateExecution(
		context.Background(),
		ExecutionRequest{
			Kind:             ExecutionKindEpisodeNotes,
			WorkingDirectory: workDir,
			Prompt:           "ignore",
			OutputSchema:     episodeNotesSchema,
		},
	)
	require.NoError(t, err)
	childPID := waitForChildPID(t, filepath.Join(workDir, "child.pid"))

	cancelled, err := host.CancelExecution(context.Background(), snapshot.ID)
	require.NoError(t, err)
	require.Equal(t, StatusCancelled, cancelled.Status)
	require.Contains(
		t,
		[]CancellationMethod{CancellationSIGTERM, CancellationSIGKILL},
		cancelled.Method,
	)
	require.Eventually(t, func() bool {
		err := syscall.Kill(childPID, 0)
		return errors.Is(err, syscall.ESRCH)
	}, 3*time.Second, 20*time.Millisecond)
	require.NoError(t, closeHost(t, host))
	require.Equal(t, 0, host.Diagnostics().LiveProcessGroups)
}

func TestProcessHostCallerDeadlineDoesNotStopTargetedCancellationCleanup(
	t *testing.T,
) {
	host, workRoot := newHelperHost(t, "ignore_cancel")
	host.config.NativeCancelTimeout = 200 * time.Millisecond
	workDir := newExecutionDir(t, workRoot, "cancel-deadline-")
	snapshot, err := host.CreateExecution(
		context.Background(),
		ExecutionRequest{
			Kind:             ExecutionKindEpisodeNotes,
			WorkingDirectory: workDir,
			Prompt:           "ignore",
			OutputSchema:     episodeNotesSchema,
		},
	)
	require.NoError(t, err)
	childPID := waitForChildPID(t, filepath.Join(workDir, "child.pid"))

	cancelCtx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	_, err = host.CancelExecution(cancelCtx, snapshot.ID)
	cancel()
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Eventually(t, func() bool {
		childErr := syscall.Kill(childPID, 0)
		return errors.Is(childErr, syscall.ESRCH) &&
			host.Diagnostics().ActiveExecutions == 0 &&
			host.Diagnostics().LiveProcessGroups == 0
	}, 3*time.Second, 20*time.Millisecond)
	final, err := host.GetExecution(context.Background(), snapshot.ID)
	require.NoError(t, err)
	require.Equal(t, StatusCancelled, final.Status)
	require.Contains(
		t,
		[]CancellationMethod{CancellationSIGTERM, CancellationSIGKILL},
		final.CancellationMethod,
	)
	require.NoError(t, closeHost(t, host))
}

func TestProcessHostCancelRequestWithoutAckOrSupervisorSignalFailsProtocol(
	t *testing.T,
) {
	host, workRoot := newHelperHost(t, "cancel_then_exit")
	snapshot, err := host.CreateExecution(
		context.Background(),
		ExecutionRequest{
			Kind:             ExecutionKindEpisodeNotes,
			WorkingDirectory: newExecutionDir(t, workRoot, "cancel-crash-"),
			Prompt:           "crash after cancel",
			OutputSchema:     episodeNotesSchema,
		},
	)
	require.NoError(t, err)

	cancelled, err := host.CancelExecution(context.Background(), snapshot.ID)
	require.NoError(t, err)
	require.Equal(t, StatusFailed, cancelled.Status)
	require.Equal(t, CancellationNone, cancelled.Method)
	final, err := host.GetExecution(context.Background(), snapshot.ID)
	require.NoError(t, err)
	require.Equal(t, StatusFailed, final.Status)
	require.Equal(t, ErrorProtocol, final.ErrorCode)
	require.Equal(t, CancellationNone, final.CancellationMethod)
	require.NoError(t, closeHost(t, host))
}

func TestProcessHostCloseDeadlineStillKillsAndReapsTargetProcessGroup(
	t *testing.T,
) {
	host, workRoot := newHelperHost(t, "ignore_cancel")
	workDir := newExecutionDir(t, workRoot, "close-deadline-")
	_, err := host.CreateExecution(
		context.Background(),
		ExecutionRequest{
			Kind:             ExecutionKindEpisodeNotes,
			WorkingDirectory: workDir,
			Prompt:           "ignore",
			OutputSchema:     episodeNotesSchema,
		},
	)
	require.NoError(t, err)
	childPID := waitForChildPID(t, filepath.Join(workDir, "child.pid"))

	closeCtx, cancel := context.WithTimeout(
		context.Background(),
		50*time.Millisecond,
	)
	err = host.Close(closeCtx)
	cancel()
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Eventually(t, func() bool {
		childErr := syscall.Kill(childPID, 0)
		return errors.Is(childErr, syscall.ESRCH) &&
			host.Diagnostics().LiveProcessGroups == 0
	}, 3*time.Second, 20*time.Millisecond)
	require.NoError(t, closeHost(t, host))
}

func TestProcessHostBoundsInitialFrameWriteByStartupTimeout(t *testing.T) {
	host, workRoot := newHelperHost(t, "no_read")
	host.config.StartupTimeout = 100 * time.Millisecond
	startedAt := time.Now()

	_, err := host.CreateExecution(
		context.Background(),
		ExecutionRequest{
			Kind:             ExecutionKindEpisodeNotes,
			WorkingDirectory: newExecutionDir(t, workRoot, "blocked-write-"),
			Prompt:           strings.Repeat("x", 1<<20),
			OutputSchema:     episodeNotesSchema,
		},
	)
	require.Error(t, err)
	require.Equal(t, ErrorRuntimeUnavailable, ErrorCode(err))
	require.Less(t, time.Since(startedAt), time.Second)
	require.Eventually(t, func() bool {
		return host.Diagnostics().LiveProcessGroups == 0
	}, 3*time.Second, 20*time.Millisecond)
	require.NoError(t, closeHost(t, host))
}

func TestProcessHostRejectsOversizedEncodedFrameBeforeProcessLaunch(
	t *testing.T,
) {
	host, workRoot := newHelperHost(t, "success")
	host.config.MaxPromptBytes = 1024
	host.config.MaxFrameBytes = 512

	_, err := host.CreateExecution(
		context.Background(),
		ExecutionRequest{
			Kind:             ExecutionKindEpisodeNotes,
			WorkingDirectory: newExecutionDir(t, workRoot, "encoded-limit-"),
			Prompt:           strings.Repeat("\"", 400),
			OutputSchema:     episodeNotesSchema,
		},
	)
	require.Error(t, err)
	require.Equal(t, ErrorInvalidRequest, ErrorCode(err))
	require.Equal(t, 0, host.Diagnostics().TrackedExecutions)
	require.Equal(t, 0, host.Diagnostics().LiveProcessGroups)
	require.NoError(t, closeHost(t, host))
}

func TestProcessHostPreflightRejectsMissingCapabilitiesAndEscapedWorkdir(
	t *testing.T,
) {
	host, workRoot := newHelperHost(t, "success")
	validDir := newExecutionDir(t, workRoot, "valid-")

	_, err := host.CreateExecution(
		context.Background(),
		ExecutionRequest{
			Kind:                 ExecutionKindEpisodeNotes,
			WorkingDirectory:     validDir,
			Prompt:               "success",
			OutputSchema:         episodeNotesSchema,
			RequiredCapabilities: []string{"lark-minutes"},
		},
	)
	require.Error(t, err)
	require.Equal(t, ErrorRuntimeUnavailable, ErrorCode(err))

	outside := t.TempDir()
	_, err = host.CreateExecution(
		context.Background(),
		ExecutionRequest{
			Kind:             ExecutionKindEpisodeNotes,
			WorkingDirectory: outside,
			Prompt:           "success",
			OutputSchema:     episodeNotesSchema,
		},
	)
	require.Error(t, err)
	require.Equal(t, ErrorInvalidRequest, ErrorCode(err))
	require.NoError(t, closeHost(t, host))
}

func TestPythonSDKHostUsesStableRestrictedConfiguration(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 is not available")
	}
	_, currentFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	packageDir := filepath.Dir(currentFile)
	script := filepath.Join(packageDir, "runtime_host.py")
	fakeSDK := filepath.Join(packageDir, "testdata", "fake_sdk")
	workRoot := t.TempDir()
	environment, isolatedTempRoot := fakeSDKEnvironment(t, fakeSDK, nil)
	host, err := NewProcessHost(ProcessHostConfig{
		Command:             []string{python, script},
		WorkRoot:            workRoot,
		testEnvironment:     environment,
		StartupTimeout:      3 * time.Second,
		NativeCancelTimeout: 500 * time.Millisecond,
		TerminateTimeout:    500 * time.Millisecond,
		KillTimeout:         500 * time.Millisecond,
	})
	require.NoError(t, err)

	completed, err := host.CreateExecution(
		context.Background(),
		ExecutionRequest{
			Kind:             ExecutionKindEpisodeNotes,
			WorkingDirectory: newExecutionDir(t, workRoot, "python-success-"),
			Prompt:           "SUCCESS",
			OutputSchema:     episodeNotesSchema,
		},
	)
	require.NoError(t, err)
	events, err := host.SubscribeExecution(context.Background(), completed.ID)
	require.NoError(t, err)
	require.Contains(t, eventTypes(collectEvents(events)), EventOutputDelta)
	final, err := host.GetExecution(context.Background(), completed.ID)
	require.NoError(t, err)
	require.Equal(t, StatusCompleted, final.Status)
	require.Equal(
		t,
		"sdk/0.147.0;runtime/0.147.0 (Mac OS 26.6.1; arm64) "+
			"unknown (codex_python_sdk; 0.147.0)",
		final.RuntimeVersion,
	)

	assistant, err := host.CreateExecution(
		context.Background(),
		ExecutionRequest{
			Kind:             ExecutionKindAssistant,
			WorkingDirectory: newExecutionDir(t, workRoot, "python-assistant-"),
			Prompt:           "Answer using the supplied episode context.",
		},
	)
	require.NoError(t, err)
	assistantEvents, err := host.SubscribeExecution(
		context.Background(),
		assistant.ID,
	)
	require.NoError(t, err)
	require.Contains(t, eventTypes(collectEvents(assistantEvents)), EventOutputDelta)
	assistantFinal, err := host.GetExecution(context.Background(), assistant.ID)
	require.NoError(t, err)
	require.Equal(t, StatusCompleted, assistantFinal.Status)
	require.JSONEq(
		t,
		`{"text":"Plain assistant answer."}`,
		string(assistantFinal.Result),
	)

	blocked, err := host.CreateExecution(
		context.Background(),
		ExecutionRequest{
			Kind:             ExecutionKindEpisodeNotes,
			WorkingDirectory: newExecutionDir(t, workRoot, "python-cancel-"),
			Prompt:           "BLOCK_UNTIL_CANCEL",
			OutputSchema:     episodeNotesSchema,
		},
	)
	require.NoError(t, err)
	cancelled, err := host.CancelExecution(context.Background(), blocked.ID)
	require.NoError(t, err)
	require.Equal(t, StatusCancelled, cancelled.Status)
	require.Equal(t, CancellationNativeInterrupt, cancelled.Method)

	toolDir := newExecutionDir(t, workRoot, "python-tool-")
	toolExecution, err := host.CreateExecution(
		context.Background(),
		ExecutionRequest{
			Kind:             ExecutionKindEpisodeNotes,
			WorkingDirectory: toolDir,
			Prompt:           "TOOL_VIOLATION",
			OutputSchema:     episodeNotesSchema,
		},
	)
	require.NoError(t, err)
	toolEvents, err := host.SubscribeExecution(
		context.Background(),
		toolExecution.ID,
	)
	require.NoError(t, err)
	_ = collectEvents(toolEvents)
	toolFinal, err := host.GetExecution(
		context.Background(),
		toolExecution.ID,
	)
	require.NoError(t, err)
	require.Equal(t, StatusCompleted, toolFinal.Status)
	_, err = os.Stat(filepath.Join(toolDir, "forbidden-tool-dispatched"))
	require.ErrorIs(t, err, os.ErrNotExist)

	forcedToolExecution, err := host.CreateExecution(
		context.Background(),
		ExecutionRequest{
			Kind: ExecutionKindEpisodeNotes,
			WorkingDirectory: newExecutionDir(
				t,
				workRoot,
				"python-forced-tool-",
			),
			Prompt:       "FORCED_TOOL_EVENT",
			OutputSchema: episodeNotesSchema,
		},
	)
	require.NoError(t, err)
	forcedToolEvents, err := host.SubscribeExecution(
		context.Background(),
		forcedToolExecution.ID,
	)
	require.NoError(t, err)
	_ = collectEvents(forcedToolEvents)
	forcedToolFinal, err := host.GetExecution(
		context.Background(),
		forcedToolExecution.ID,
	)
	require.NoError(t, err)
	require.Equal(t, StatusFailed, forcedToolFinal.Status)
	require.Equal(t, ErrorCapabilityDenied, forcedToolFinal.ErrorCode)
	require.NoError(t, closeHost(t, host))
	require.Equal(t, 0, host.Diagnostics().LiveProcessGroups)
	entries, err := os.ReadDir(isolatedTempRoot)
	require.NoError(t, err)
	require.Empty(t, entries)
}

func TestProcessHostRejectsEnvironmentOutsideAllowlist(t *testing.T) {
	_, err := NewProcessHost(ProcessHostConfig{
		Command:  []string{os.Args[0]},
		WorkRoot: t.TempDir(),
		Environment: map[string]string{
			"OPENAI_API_KEY": "must-not-reach-runtime",
		},
	})
	require.Error(t, err)
	require.Equal(t, ErrorInvalidRequest, ErrorCode(err))
}

func TestPythonSDKHostRejectsMismatchedRuntimeVersion(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 is not available")
	}
	_, currentFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	packageDir := filepath.Dir(currentFile)
	fakeSDK := filepath.Join(packageDir, "testdata", "fake_sdk")
	workRoot := t.TempDir()
	environment, _ := fakeSDKEnvironment(t, fakeSDK, map[string]string{
		"FAKE_CODEX_RUNTIME_VERSION": "0.148.0",
	})
	host, err := NewProcessHost(ProcessHostConfig{
		Command: []string{
			python,
			filepath.Join(packageDir, "runtime_host.py"),
		},
		WorkRoot:        workRoot,
		testEnvironment: environment,
		StartupTimeout:  3 * time.Second,
	})
	require.NoError(t, err)

	_, err = host.CreateExecution(
		context.Background(),
		ExecutionRequest{
			Kind:             ExecutionKindEpisodeNotes,
			WorkingDirectory: newExecutionDir(t, workRoot, "bad-version-"),
			Prompt:           "SUCCESS",
			OutputSchema:     episodeNotesSchema,
		},
	)
	require.Error(t, err)
	require.Equal(t, ErrorRuntimeUnavailable, ErrorCode(err))
	require.NoError(t, closeHost(t, host))
}

func TestPythonSDKHostFailsPreflightWhenAuthenticationIsMissing(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 is not available")
	}
	_, currentFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	packageDir := filepath.Dir(currentFile)
	workRoot := t.TempDir()
	environment, _ := fakeSDKEnvironment(
		t,
		filepath.Join(packageDir, "testdata", "fake_sdk"),
		map[string]string{"FAKE_CODEX_UNAUTH": "1"},
	)
	host, err := NewProcessHost(ProcessHostConfig{
		Command: []string{
			python,
			filepath.Join(packageDir, "runtime_host.py"),
		},
		WorkRoot:        workRoot,
		testEnvironment: environment,
		StartupTimeout:  3 * time.Second,
	})
	require.NoError(t, err)
	_, err = host.CreateExecution(
		context.Background(),
		ExecutionRequest{
			Kind:             ExecutionKindEpisodeNotes,
			WorkingDirectory: newExecutionDir(t, workRoot, "unauth-"),
			Prompt:           "SUCCESS",
			OutputSchema:     episodeNotesSchema,
		},
	)
	require.Error(t, err)
	require.Equal(t, ErrorRuntimeUnavailable, ErrorCode(err))
	require.NoError(t, closeHost(t, host))
}

func TestProcessHostHelper(t *testing.T) {
	if os.Getenv("GO_WANT_RUNTIME_HELPER") != "1" {
		return
	}
	helperRuntimeMain()
	os.Exit(0)
}

func helperRuntimeMain() {
	scenario := os.Getenv("RUNTIME_HELPER_SCENARIO")
	if scenario == "no_read" {
		select {}
	}
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		os.Exit(2)
	}
	var request executeFrame
	if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
		os.Exit(2)
	}
	if scenario == "from_prompt" {
		scenario = request.Prompt
	}
	write := func(frame runtimeFrame) {
		payload, _ := json.Marshal(frame)
		_, _ = fmt.Fprintln(os.Stdout, string(payload))
	}
	base := runtimeFrame{
		ProtocolVersion: ProtocolVersion,
		ExecutionID:     request.ExecutionID,
	}
	switch scenario {
	case "success":
		base.Sequence = 1
		base.Type = "ready"
		base.RuntimeVersion = "helper-runtime-1"
		write(base)
		base.Sequence = 2
		base.Type = "output_delta"
		base.RuntimeVersion = ""
		base.Text = `{"episode_notes":"# Helper notes"}`
		write(base)
		base.Sequence = 3
		base.Type = "terminal"
		base.Text = ""
		base.Status = StatusCompleted
		base.Result = json.RawMessage(`{"episode_notes":"# Helper notes"}`)
		write(base)
	case "block":
		base.Sequence = 1
		base.Type = "ready"
		base.RuntimeVersion = "helper-runtime-1"
		write(base)
		if !scanner.Scan() {
			os.Exit(2)
		}
		base.Sequence = 2
		base.Type = "cancel_ack"
		base.RuntimeVersion = ""
		base.CancellationMethod = CancellationNativeInterrupt
		write(base)
		base.Sequence = 3
		base.Type = "terminal"
		base.CancellationMethod = ""
		base.Status = StatusCancelled
		write(base)
	case "wrong_identity":
		base.Sequence = 1
		base.ExecutionID = "conflicting-execution"
		base.Type = "ready"
		base.RuntimeVersion = "helper-runtime-1"
		write(base)
	case "missing_identity":
		base.Sequence = 1
		base.ExecutionID = ""
		base.Type = "ready"
		base.RuntimeVersion = "helper-runtime-1"
		write(base)
	case "out_of_order":
		base.Sequence = 1
		base.Type = "ready"
		base.RuntimeVersion = "helper-runtime-1"
		write(base)
		base.Sequence = 3
		base.Type = "output_delta"
		base.Text = "skipped sequence"
		base.RuntimeVersion = ""
		write(base)
	case "malformed":
		_, _ = fmt.Fprintln(os.Stdout, "{not-json")
	case "no_terminal":
		return
	case "terminal_then_invalid":
		base.Sequence = 1
		base.Type = "ready"
		base.RuntimeVersion = "helper-runtime-1"
		write(base)
		base.Sequence = 2
		base.Type = "terminal"
		base.RuntimeVersion = ""
		base.Status = StatusCompleted
		base.Result = json.RawMessage(`{"episode_notes":"# Helper notes"}`)
		write(base)
		base.Sequence = 3
		base.Type = "output_delta"
		base.Status = ""
		base.Result = nil
		base.Text = "late output"
		write(base)
	case "terminal_hang":
		base.Sequence = 1
		base.Type = "ready"
		base.RuntimeVersion = "helper-runtime-1"
		write(base)
		base.Sequence = 2
		base.Type = "terminal"
		base.RuntimeVersion = ""
		base.Status = StatusCompleted
		base.Result = json.RawMessage(`{"episode_notes":"# Helper notes"}`)
		write(base)
		select {}
	case "ignore_cancel":
		signalIgnoreTermination()
		child := exec.Command("sleep", "30")
		if err := child.Start(); err != nil {
			os.Exit(2)
		}
		_ = os.WriteFile(
			filepath.Join(request.WorkingDirectory, "child.pid"),
			[]byte(fmt.Sprintf("%d", child.Process.Pid)),
			0o600,
		)
		base.Sequence = 1
		base.Type = "ready"
		base.RuntimeVersion = "helper-runtime-1"
		write(base)
		_ = scanner.Scan()
		select {}
	case "cancel_then_exit":
		base.Sequence = 1
		base.Type = "ready"
		base.RuntimeVersion = "helper-runtime-1"
		write(base)
		if !scanner.Scan() {
			os.Exit(2)
		}
		os.Exit(3)
	default:
		os.Exit(2)
	}
}

func signalIgnoreTermination() {
	signalIgnore(syscall.SIGTERM)
}

func fakeSDKEnvironment(
	t *testing.T,
	fakeSDK string,
	extra map[string]string,
) (map[string]string, string) {
	t.Helper()
	sourceCodexHome := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(sourceCodexHome, "auth.json"),
		[]byte("{}"),
		0o600,
	))
	isolatedTempRoot := t.TempDir()
	environment := map[string]string{
		"CODEX_HOME":              sourceCodexHome,
		"PATH":                    os.Getenv("PATH"),
		"PYTHONDONTWRITEBYTECODE": "1",
		"PYTHONPATH":              fakeSDK,
		"TMPDIR":                  isolatedTempRoot,
	}
	for name, value := range extra {
		environment[name] = value
	}
	return environment, isolatedTempRoot
}

func newHelperHost(
	t *testing.T,
	scenario string,
) (*ProcessHost, string) {
	t.Helper()
	workRoot := t.TempDir()
	host, err := NewProcessHost(ProcessHostConfig{
		Command: []string{
			os.Args[0],
			"-test.run=^TestProcessHostHelper$",
		},
		WorkRoot: workRoot,
		testEnvironment: map[string]string{
			"GO_WANT_RUNTIME_HELPER":  "1",
			"GORACE":                  "atexit_sleep_ms=0",
			"PATH":                    os.Getenv("PATH"),
			"RUNTIME_HELPER_SCENARIO": scenario,
		},
		StartupTimeout:      2 * time.Second,
		NativeCancelTimeout: 500 * time.Millisecond,
		TerminateTimeout:    500 * time.Millisecond,
		KillTimeout:         2 * time.Second,
		PostExitGrace:       50 * time.Millisecond,
	})
	require.NoError(t, err)
	return host, workRoot
}

func newExecutionDir(
	t *testing.T,
	workRoot string,
	prefix string,
) string {
	t.Helper()
	path, err := os.MkdirTemp(workRoot, prefix)
	require.NoError(t, err)
	return path
}

func eventTypes(events []Event) []EventType {
	types := make([]EventType, 0, len(events))
	for _, event := range events {
		types = append(types, event.Type)
	}
	return types
}

func collectEvents(events <-chan Event) []Event {
	var collected []Event
	for event := range events {
		collected = append(collected, event)
	}
	return collected
}

func waitForChildPID(t *testing.T, path string) int {
	t.Helper()
	var pid int
	require.Eventually(t, func() bool {
		payload, err := os.ReadFile(path)
		if err != nil {
			return false
		}
		_, err = fmt.Sscanf(strings.TrimSpace(string(payload)), "%d", &pid)
		return err == nil && pid > 0
	}, 2*time.Second, 20*time.Millisecond)
	return pid
}

func closeHost(t *testing.T, host *ProcessHost) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return host.Close(ctx)
}
