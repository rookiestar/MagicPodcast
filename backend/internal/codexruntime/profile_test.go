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

func TestResolveModelProfileMapsAllThreeTiersExactly(t *testing.T) {
	expected := map[ModelProfileID]ModelProfile{
		DefaultModelProfileID: {
			ID:              DefaultModelProfileID,
			Model:           "gpt-5.6-luna",
			Effort:          "max",
			ServiceTier:     "priority",
			ServiceTierName: "Fast",
		},
		ModelProfileID("quick"): {
			ID:              ModelProfileID("quick"),
			Model:           "gpt-5.6-sol",
			Effort:          "medium",
			ServiceTier:     "priority",
			ServiceTierName: "Fast",
		},
		ModelProfileID("deep"): {
			ID:              ModelProfileID("deep"),
			Model:           "gpt-5.6-sol",
			Effort:          "xhigh",
			ServiceTier:     "",
			ServiceTierName: "Standard",
		},
	}
	require.Len(t, expected, 3)
	for id, want := range expected {
		resolved, ok := ResolveModelProfile(id)
		require.True(t, ok, "profile %q must resolve", id)
		require.Equal(t, want, resolved)
	}

	for _, unknown := range []ModelProfileID{
		"",
		"turbo",
		"gpt-5.6-sol",
		"medium",
		"QUICK",
	} {
		resolved, ok := ResolveModelProfile(unknown)
		require.False(t, ok, "profile %q must not resolve", unknown)
		require.Zero(t, resolved)
	}

	// The deep tier must not express Standard by weakening the model or
	// effort: only the Fast service tier is dropped.
	deep, ok := ResolveModelProfile("deep")
	require.True(t, ok)
	require.Equal(t, "gpt-5.6-sol", deep.Model)
	require.Equal(t, "xhigh", deep.Effort)
	require.Empty(t, deep.ServiceTier)

	// Fast tiers stay explicit for quick and balanced, under the account
	// catalog's wire ID for the Fast speed tier.
	for _, id := range []ModelProfileID{"quick", "balanced"} {
		fast, ok := ResolveModelProfile(id)
		require.True(t, ok)
		require.Equal(t, "priority", fast.ServiceTier)
		require.Equal(t, "Fast", fast.ServiceTierName)
	}

	// Every catalog entry exposes a safe token set for the wire profile.
	for _, profile := range ModelProfiles() {
		require.NotEmpty(t, profile.Model)
		require.NotEmpty(t, profile.Effort)
	}
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

func newFakeSDKHostForProfiles(
	t *testing.T,
	extra map[string]string,
) (*ProcessHost, string) {
	t.Helper()
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
	environment, _ := fakeSDKEnvironment(t, fakeSDK, extra)
	host, err := NewProcessHost(ProcessHostConfig{
		Command:          []string{python, script},
		WorkRoot:         workRoot,
		testEnvironment:  environment,
		StartupTimeout:   3 * time.Second,
		TerminateTimeout: 500 * time.Millisecond,
		KillTimeout:      500 * time.Millisecond,
	})
	require.NoError(t, err)
	return host, workRoot
}

func observedTurnParameters(
	t *testing.T,
	dir string,
) struct {
	Model       string `json:"model"`
	Effort      string `json:"effort"`
	ServiceTier string `json:"service_tier"`
} {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, "fake-turn-params.json"))
	require.NoError(t, err)
	var observed struct {
		Model       string `json:"model"`
		Effort      string `json:"effort"`
		ServiceTier string `json:"service_tier"`
	}
	require.NoError(t, json.Unmarshal(raw, &observed))
	return observed
}

func TestPythonSDKHostCarriesQuickAndDeepProfilesToSDK(t *testing.T) {
	host, workRoot := newFakeSDKHostForProfiles(t, nil)

	quickDir := newExecutionDir(t, workRoot, "python-quick-")
	quick, err := host.CreateExecution(
		context.Background(),
		ExecutionRequest{
			Kind:             ExecutionKindAssistant,
			WorkingDirectory: quickDir,
			Prompt:           "Answer using the supplied episode context.",
			ModelProfile:     ModelProfileID("quick"),
		},
	)
	require.NoError(t, err)
	events, err := host.SubscribeExecution(context.Background(), quick.ID)
	require.NoError(t, err)
	_ = collectEvents(events)
	quickFinal, err := host.GetExecution(context.Background(), quick.ID)
	require.NoError(t, err)
	require.Equal(t, StatusCompleted, quickFinal.Status)
	quickObserved := observedTurnParameters(t, quickDir)
	require.Equal(t, "gpt-5.6-sol", quickObserved.Model)
	require.Equal(t, "medium", quickObserved.Effort)
	require.Equal(t, "priority", quickObserved.ServiceTier)

	deepDir := newExecutionDir(t, workRoot, "python-deep-")
	deep, err := host.CreateExecution(
		context.Background(),
		ExecutionRequest{
			Kind:             ExecutionKindAssistant,
			WorkingDirectory: deepDir,
			Prompt:           "Answer using the supplied episode context.",
			ModelProfile:     ModelProfileID("deep"),
		},
	)
	require.NoError(t, err)
	deepEvents, err := host.SubscribeExecution(context.Background(), deep.ID)
	require.NoError(t, err)
	_ = collectEvents(deepEvents)
	deepFinal, err := host.GetExecution(context.Background(), deep.ID)
	require.NoError(t, err)
	require.Equal(t, StatusCompleted, deepFinal.Status)
	deepObserved := observedTurnParameters(t, deepDir)
	require.Equal(t, "gpt-5.6-sol", deepObserved.Model)
	require.Equal(t, "xhigh", deepObserved.Effort)
	// Standard is expressed by NOT setting a service tier; the Fast tier is
	// never silently attached to a standard profile.
	require.Empty(t, deepObserved.ServiceTier)
}

func TestPythonSDKHostReportsProfileUnavailableStably(t *testing.T) {
	// The fake account only supports sol at medium with a fast tier, so the
	// deep profile (sol/xhigh/standard) must fail before the turn starts.
	host, workRoot := newFakeSDKHostForProfiles(t, map[string]string{
		"FAKE_CODEX_MODEL_CATALOG": `[{"model":"gpt-5.6-sol","efforts":["medium"],"tiers":["priority"]}]`,
	})
	deepDir := newExecutionDir(t, workRoot, "python-unsupported-")

	// The account check runs in the host before the turn starts, so the
	// failure surfaces synchronously from CreateExecution as a stable,
	// non-retryable profile error with no substituted profile.
	_, err := host.CreateExecution(
		context.Background(),
		ExecutionRequest{
			Kind:             ExecutionKindAssistant,
			WorkingDirectory: deepDir,
			Prompt:           "Answer using the supplied episode context.",
			ModelProfile:     ModelProfileID("deep"),
		},
	)
	require.Error(t, err)
	require.Equal(t, ErrorProfileUnavailable, ErrorCode(err))

	var runtimeErr *RuntimeError
	require.True(t, errors.As(err, &runtimeErr))
	require.False(t, runtimeErr.Retryable)
	require.NotEmpty(t, runtimeErr.SafeMessage)
}

func TestPythonSDKHostPrefersRuntimeVersionFailureOverProfileFailure(
	t *testing.T,
) {
	host, workRoot := newFakeSDKHostForProfiles(t, map[string]string{
		"FAKE_CODEX_RUNTIME_VERSION": "0.148.0",
		"FAKE_CODEX_MODEL_CATALOG":   `[{"model":"gpt-5.6-sol","efforts":["medium"],"tiers":["priority"]}]`,
	})

	_, err := host.CreateExecution(
		context.Background(),
		ExecutionRequest{
			Kind:             ExecutionKindAssistant,
			WorkingDirectory: newExecutionDir(t, workRoot, "python-version-first-"),
			Prompt:           "Do not hide an incompatible runtime version.",
			ModelProfile:     ModelProfileID("deep"),
		},
	)
	require.Error(t, err)
	require.Equal(t, ErrorRuntimeUnavailable, ErrorCode(err))
}

func TestPythonSDKHostRejectsFastDefaultForStandardProfile(t *testing.T) {
	host, workRoot := newFakeSDKHostForProfiles(t, map[string]string{
		"FAKE_CODEX_MODEL_CATALOG": `[{"model":"gpt-5.6-sol","efforts":["xhigh"],"tiers":["priority"],"default_tier":"priority"}]`,
	})

	_, err := host.CreateExecution(
		context.Background(),
		ExecutionRequest{
			Kind:             ExecutionKindAssistant,
			WorkingDirectory: newExecutionDir(t, workRoot, "python-standard-"),
			Prompt:           "Standard must not inherit Fast.",
			ModelProfile:     ModelProfileID("deep"),
		},
	)
	require.Error(t, err)
	require.Equal(t, ErrorProfileUnavailable, ErrorCode(err))
}

func TestPythonSDKHostCarriesResolvedBalancedProfileToSDK(t *testing.T) {
	host, workRoot := newFakeSDKHostForProfiles(t, nil)

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
	observed := observedTurnParameters(t, assistantDir)
	require.Equal(t, "gpt-5.6-luna", observed.Model)
	require.Equal(t, "max", observed.Effort)
	require.Equal(t, "priority", observed.ServiceTier)
}

func TestPythonSDKHostKeepsDefaultTurnParametersWithoutProfile(t *testing.T) {
	host, workRoot := newFakeSDKHostForProfiles(t, nil)

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
	packageDir := filepath.Dir(currentFile)
	script := filepath.Join(
		packageDir,
		"runtime_host.py",
	)
	fakeSDK := filepath.Join(packageDir, "testdata", "fake_sdk")

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
			workDir := t.TempDir()
			encodedWorkDir, err := json.Marshal(workDir)
			require.NoError(t, err)
			frame = strings.Replace(frame, `"/tmp"`, string(encodedWorkDir), 1)
			environment, _ := fakeSDKEnvironment(t, fakeSDK, nil)
			environment[runtimeHomeEnvironment] = t.TempDir()
			command := exec.CommandContext(ctx, python, script)
			command.Stdin = strings.NewReader(frame + "\n")
			command.Env = environmentMap(environment)
			output, err := command.CombinedOutput()
			require.Error(t, err, "incompatible frame must fail the host")
			require.Contains(t, string(output), ErrorProtocol)
			_, statErr := os.Stat(filepath.Join(workDir, "fake-turn-params.json"))
			require.Error(t, statErr, "invalid frame must not reach the SDK turn")
		})
	}
}
