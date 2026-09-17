package codexruntime

import (
	"context"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// newPythonSDKHost starts the real host against the fake SDK, mirroring the
// conformance seam used by the progress contract tests.
func newPythonSDKHost(t *testing.T, timeouts ...time.Duration) (*ProcessHost, string) {
	t.Helper()
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
	config := ProcessHostConfig{
		Command: []string{
			python,
			filepath.Join(packageDir, "runtime_host.py"),
		},
		WorkRoot:        workRoot,
		testEnvironment: environment,
		StartupTimeout:  5 * time.Second,
	}
	for _, timeout := range timeouts {
		config.NativeCancelTimeout = timeout
		config.TerminateTimeout = timeout
		config.KillTimeout = timeout
	}
	host, err := NewProcessHost(config)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = host.Close(context.Background())
	})
	return host, workRoot
}

// TestPythonSDKHostMapsRecoverableConnectionErrors proves SDK error
// notifications survive as provider-neutral connection activity: recoverable
// reconnects are visible before the final result, structured facts replace
// provider payloads, and observed activity never becomes a terminal failure.
func TestPythonSDKHostMapsRecoverableConnectionErrors(t *testing.T) {
	host, workRoot := newPythonSDKHost(t)

	workDir := newExecutionDir(t, workRoot, "connection-ok-")
	snapshot, err := host.CreateExecution(context.Background(), ExecutionRequest{
		Kind:             ExecutionKindAssistant,
		WorkingDirectory: workDir,
		Prompt:           "CONNECTION_ERRORS",
	})
	require.NoError(t, err)
	events, err := host.SubscribeExecution(context.Background(), snapshot.ID)
	require.NoError(t, err)
	received := collectEvents(events)

	require.Equal(t, EventStarted, received[0].Type)
	require.Equal(t, EventTerminal, received[len(received)-1].Type)

	var connections []Progress
	var sawOutputAfterReconnect bool
	var sawReconnect bool
	for _, event := range received {
		require.Equal(t, snapshot.ID, event.ExecutionID)
		switch {
		case event.Type == EventProgress && event.Progress.Category == CategoryConnection:
			sawReconnect = true
			connections = append(connections, *event.Progress)
		case event.Type == EventOutputDelta:
			sawOutputAfterReconnect = sawReconnect
		}
	}
	require.True(t, sawReconnect, "reconnect signals must reach subscribers before the terminal event")
	require.True(t, sawOutputAfterReconnect, "execution must recover and produce output after reconnects")

	// started, updated, updated: one activity, continuous lifecycle, only
	// provider-confirmed facts in metadata.
	require.Len(t, connections, 3)
	require.Equal(t, ProgressStarted, connections[0].State)
	require.Equal(t, ProgressUpdated, connections[1].State)
	require.Equal(t, ProgressUpdated, connections[2].State)
	firstID := connections[0].ActivityID
	for _, progress := range connections {
		require.Equal(t, firstID, progress.ActivityID)
		require.Equal(t, "true", progress.Metadata["will_retry"])
		require.NotContains(t, progress.DisplayText, "internal diagnostic")
	}
	require.Equal(t, "response_stream_disconnected", connections[0].Metadata["error_class"])
	require.Equal(t, "502", connections[0].Metadata["http_status"])
	require.Equal(t, "response_stream_connection_failed", connections[1].Metadata["error_class"])
	require.NotContains(t, connections[1].Metadata, "http_status")
	require.Equal(t, "server_overloaded", connections[2].Metadata["error_class"])

	final, err := host.GetExecution(context.Background(), snapshot.ID)
	require.NoError(t, err)
	require.Equal(t, StatusCompleted, final.Status, "recoverable reconnects must not end the execution")
}

// TestPythonSDKHostMapsNonRecoverableConnectionError proves the non-retryable
// signal closes the connection activity with will_retry=false and the failed
// terminal classification, without leaking provider diagnostics.
func TestPythonSDKHostMapsNonRecoverableConnectionError(t *testing.T) {
	host, workRoot := newPythonSDKHost(t)

	workDir := newExecutionDir(t, workRoot, "connection-fail-")
	snapshot, err := host.CreateExecution(context.Background(), ExecutionRequest{
		Kind:             ExecutionKindAssistant,
		WorkingDirectory: workDir,
		Prompt:           "CONNECTION_ERRORS NONRECOVERABLE",
	})
	require.NoError(t, err)
	events, err := host.SubscribeExecution(context.Background(), snapshot.ID)
	require.NoError(t, err)
	received := collectEvents(events)

	var connections []Progress
	for _, event := range received {
		if event.Type == EventProgress && event.Progress.Category == CategoryConnection {
			connections = append(connections, *event.Progress)
		}
	}
	require.GreaterOrEqual(t, len(connections), 2)
	last := connections[len(connections)-1]
	require.Equal(t, ProgressFailed, last.State)
	require.Equal(t, "false", last.Metadata["will_retry"])
	require.Equal(t, "unauthorized", last.Metadata["error_class"])
	for _, progress := range connections {
		require.NotContains(t, progress.DisplayText, "final diagnostic")
	}

	final, err := host.GetExecution(context.Background(), snapshot.ID)
	require.NoError(t, err)
	require.Equal(t, StatusFailed, final.Status)
	require.Equal(t, ErrorAuthentication, final.ErrorCode)
}

func TestPythonSDKHostClassifiesQuotaWithoutAnEarlierErrorNotification(t *testing.T) {
	host, root := newPythonSDKHost(t)
	snap, err := host.CreateExecution(context.Background(), ExecutionRequest{Kind: ExecutionKindAssistant, WorkingDirectory: newExecutionDir(t, root, "quota-"), Prompt: "PROVIDER_TERMINAL_QUOTA"})
	require.NoError(t, err)
	events, err := host.SubscribeExecution(context.Background(), snap.ID)
	require.NoError(t, err)
	collectEvents(events)
	final, err := host.GetExecution(context.Background(), snap.ID)
	require.NoError(t, err)
	require.Equal(t, ErrorQuota, final.ErrorCode)
	require.NotContains(t, final.SafeMessage, "private")
}
