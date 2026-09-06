package codexruntime

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestResolveModelProfileMapsBalancedExactly(t *testing.T) {
	profile, ok := ResolveModelProfile(DefaultModelProfileID)
	require.True(t, ok)
	require.Equal(t, ModelProfileID("balanced"), profile.ID)
	require.Equal(t, "gpt-5.6-luna", profile.Model)
	require.Equal(t, "max", profile.Effort)
	require.Equal(t, "fast", profile.ServiceTier)

	for _, unknown := range []ModelProfileID{"", "turbo", "gpt-5.6-luna", "BALANCED"} {
		resolved, ok := ResolveModelProfile(unknown)
		require.False(t, ok, "profile %q must not resolve", unknown)
		require.Zero(t, resolved)
	}

	catalog := ModelProfiles()
	require.Len(t, catalog, 1)
	require.Equal(t, DefaultModelProfileID, catalog[0].ID)
}

func TestProcessHostRejectsUnknownModelProfileBeforeLaunch(t *testing.T) {
	workRoot := t.TempDir()
	// The test binary satisfies command resolution but is never launched:
	// the unknown profile must be rejected before any process starts.
	host, err := NewProcessHost(ProcessHostConfig{
		Command:  []string{os.Args[0]},
		WorkRoot: workRoot,
	})
	require.NoError(t, err)

	_, err = host.CreateExecution(context.Background(), ExecutionRequest{
		Kind:             ExecutionKindAssistant,
		WorkingDirectory: newExecutionDir(t, workRoot, "profile-"),
		Prompt:           "SUCCESS",
		ModelProfile:     ModelProfileID("turbo"),
	})
	require.Error(t, err)
	require.Equal(t, ErrorInvalidRequest, ErrorCode(err))

	var runtimeErr *RuntimeError
	require.True(t, errors.As(err, &runtimeErr))
	require.False(t, runtimeErr.Retryable)
	require.Zero(t, host.Diagnostics().TrackedExecutions)
}

func TestPythonSDKHostCarriesResolvedBalancedProfileToSDK(t *testing.T) {
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
	environment, _ := fakeSDKEnvironment(t, fakeSDK, nil)
	host, err := NewProcessHost(ProcessHostConfig{
		Command:          []string{python, script},
		WorkRoot:         workRoot,
		testEnvironment:  environment,
		StartupTimeout:   3 * time.Second,
		TerminateTimeout: 500 * time.Millisecond,
		KillTimeout:      500 * time.Millisecond,
	})
	require.NoError(t, err)

	assistantDir := newExecutionDir(t, workRoot, "python-balanced-")
	execution, err := host.CreateExecution(
		context.Background(),
		ExecutionRequest{
			Kind:             ExecutionKindAssistant,
			WorkingDirectory: assistantDir,
			Prompt:           "Answer using the supplied episode context.",
			ModelProfile:     DefaultModelProfileID,
		},
	)
	require.NoError(t, err)
	events, err := host.SubscribeExecution(context.Background(), execution.ID)
	require.NoError(t, err)
	require.Contains(t, eventTypes(collectEvents(events)), EventOutputDelta)
	final, err := host.GetExecution(context.Background(), execution.ID)
	require.NoError(t, err)
	require.Equal(t, StatusCompleted, final.Status)

	// The fake SDK records the per-turn parameters it received, so the
	// contract is observed through the process boundary instead of private
	// call ordering.
	raw, err := os.ReadFile(filepath.Join(assistantDir, "fake-turn-params.json"))
	require.NoError(t, err)
	var observed struct {
		Model       string `json:"model"`
		Effort      string `json:"effort"`
		ServiceTier string `json:"service_tier"`
	}
	require.NoError(t, json.Unmarshal(raw, &observed))
	require.Equal(t, "gpt-5.6-luna", observed.Model)
	require.Equal(t, "max", observed.Effort)
	require.Equal(t, "fast", observed.ServiceTier)
}

func TestPythonSDKHostKeepsDefaultTurnParametersWithoutProfile(t *testing.T) {
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
	environment, _ := fakeSDKEnvironment(t, fakeSDK, nil)
	host, err := NewProcessHost(ProcessHostConfig{
		Command:          []string{python, script},
		WorkRoot:         workRoot,
		testEnvironment:  environment,
		StartupTimeout:   3 * time.Second,
		TerminateTimeout: 500 * time.Millisecond,
		KillTimeout:      500 * time.Millisecond,
	})
	require.NoError(t, err)

	notesDir := newExecutionDir(t, workRoot, "python-default-")
	execution, err := host.CreateExecution(
		context.Background(),
		ExecutionRequest{
			Kind:             ExecutionKindEpisodeNotes,
			WorkingDirectory: notesDir,
			Prompt:           "SUCCESS",
			OutputSchema:     episodeNotesSchema,
		},
	)
	require.NoError(t, err)
	events, err := host.SubscribeExecution(context.Background(), execution.ID)
	require.NoError(t, err)
	_ = collectEvents(events)
	final, err := host.GetExecution(context.Background(), execution.ID)
	require.NoError(t, err)
	require.Equal(t, StatusCompleted, final.Status)

	raw, err := os.ReadFile(filepath.Join(notesDir, "fake-turn-params.json"))
	require.NoError(t, err)
	require.JSONEq(
		t,
		`{"model":null,"effort":null,"service_tier":null}`,
		string(raw),
	)
}

func TestPythonHostRejectsIncompatibleProtocolFrames(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 is not available")
	}
	_, currentFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	script := filepath.Join(
		filepath.Dir(currentFile),
		"runtime_host.py",
	)

	frames := map[string]string{
		"legacy protocol version": `{"protocol_version":1,"type":"execute","execution_id":"legacy","kind":"assistant","working_directory":"/tmp","prompt":"p","sandbox":"read_only","allowed_tools":[]}`,
		"unknown frame field":     `{"protocol_version":2,"type":"execute","execution_id":"e","kind":"assistant","working_directory":"/tmp","prompt":"p","sandbox":"read_only","allowed_tools":[],"model_override":"gpt-5.6-luna"}`,
		"malformed model profile": `{"protocol_version":2,"type":"execute","execution_id":"e","kind":"assistant","working_directory":"/tmp","prompt":"p","sandbox":"read_only","allowed_tools":[],"model_profile":{"profile_id":"balanced","model":"gpt-5.6-luna","effort":"max"}}`,
		"non-token model profile": `{"protocol_version":2,"type":"execute","execution_id":"e","kind":"assistant","working_directory":"/tmp","prompt":"p","sandbox":"read_only","allowed_tools":[],"model_profile":{"profile_id":"balanced","model":"gpt 5.6 luna","effort":"max","service_tier":"fast"}}`,
	}
	for name, frame := range frames {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(
				context.Background(),
				15*time.Second,
			)
			defer cancel()
			command := exec.CommandContext(ctx, python, script)
			command.Stdin = strings.NewReader(frame + "\n")
			command.Env = append(os.Environ(), "CODEX_HOME="+t.TempDir())
			err := command.Run()
			require.Error(t, err, "incompatible frame must fail the host")
		})
	}
}
