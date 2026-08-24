package processing

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFeishuMinutesAdapterExecutesRecoverableOneWriteStages(t *testing.T) {
	audioRoot := t.TempDir()
	audioPath := filepath.Join(audioRoot, "episode.mp3")
	require.NoError(t, os.WriteFile(audioPath, []byte("audio"), 0o600))
	audioRoot = mustCanonicalPath(t, audioRoot)
	audioPath = filepath.Join(audioRoot, "episode.mp3")
	workRoot := t.TempDir()
	runner := &scriptedLarkRunner{
		steps: []scriptedLarkStep{
			{output: []byte(`{"file_token":"boxcn_file_123"}`)},
			{output: []byte(`{"minute_url":"https://example.feishu.cn/minutes/obcn_minute_123"}`)},
			{
				output: []byte(`{"minutes":[{"minute_token":"obcn_minute_123","artifacts":{"summary":"summary","chapters":[],"keywords":[],"transcript_file":"detail/transcript.txt"}}]}`),
				beforeReturn: func(cwd string) {
					require.NoError(t, os.WriteFile(
						filepath.Join(cwd, "detail", "transcript.txt"),
						[]byte("Speaker 00:00\nHello\n"),
						0o600,
					))
				},
			},
		},
	}
	digest := strings.Repeat("a", 64)
	adapter, err := newFeishuMinutesAdapterWithRunner(
		runner,
		workRoot,
		func(context.Context, uint) (string, string, error) {
			return audioPath, digest, nil
		},
	)
	require.NoError(t, err)
	request, persisted := feishuTestRequest(digest)

	driveProgress, err := adapter.Begin(context.Background(), request)
	require.NoError(t, err)
	require.Equal(t, ExternalProgressWaiting, driveProgress.Status)
	driveCheckpoint := mustDecodeFeishuCheckpoint(t, driveProgress.Checkpoint)
	require.Equal(t, feishuPhaseDriveUploaded, driveCheckpoint.Phase)
	require.Equal(t, "boxcn_file_123", driveCheckpoint.FileToken)
	require.Len(t, runner.calls, 1)
	require.Equal(t, audioRoot, runner.calls[0].cwd)
	require.Equal(t, []string{
		"drive", "+upload", "--file", "./episode.mp3",
		"--as", "user", "--format", "json",
	}, runner.calls[0].args)

	minutesProgress, err := adapter.Resume(
		context.Background(),
		request,
		driveProgress.Checkpoint,
	)
	require.NoError(t, err)
	minutesCheckpoint := mustDecodeFeishuCheckpoint(t, minutesProgress.Checkpoint)
	require.Equal(t, feishuPhaseMinutesCreated, minutesCheckpoint.Phase)
	require.Equal(t, "obcn_minute_123", minutesCheckpoint.MinuteToken)
	require.Len(t, runner.calls, 2)
	require.Equal(t, []string{
		"minutes", "+upload", "--file-token", "boxcn_file_123",
		"--as", "user", "--format", "json",
	}, runner.calls[1].args)

	completed, err := adapter.Resume(
		context.Background(),
		request,
		minutesProgress.Checkpoint,
	)
	require.NoError(t, err)
	require.Equal(t, ExternalProgressCompleted, completed.Status)
	require.Contains(t, completed.Transcript, "Speaker 00:00")
	require.Contains(t, completed.RawArtifacts, "minutes-detail.json")
	require.Contains(t, completed.RawArtifacts, "minutes-transcript.txt")
	require.Equal(t, "feishu-minutes", completed.SourceRefs["transcription"])
	require.Regexp(
		t,
		`^sha256:[0-9a-f]{64}$`,
		completed.SourceRefs["feishu_drive_ref"],
	)
	require.Regexp(
		t,
		`^sha256:[0-9a-f]{64}$`,
		completed.SourceRefs["feishu_minute_ref"],
	)
	require.NotContains(t, completed.SourceRefs, "minute_token")
	require.NotContains(t, completed.SourceRefs["feishu_drive_ref"], "file-token")
	require.NotContains(t, completed.SourceRefs["feishu_minute_ref"], "minute-token")
	require.Len(t, runner.calls, 3)
	require.Equal(t, []string{
		"minutes", "+detail", "--minute-tokens", "obcn_minute_123",
		"--summary", "--chapter", "--keyword", "--transcript", "--overwrite",
		"--output-dir", "./detail", "--as", "user", "--format", "json",
	}, runner.calls[2].args)

	// Every external-write intent and returned identity was persisted from
	// inside the adapter, before control returned to the engine.
	phases := make([]string, 0, len(*persisted))
	for _, state := range *persisted {
		phases = append(phases, mustDecodeFeishuCheckpoint(t, state).Phase)
	}
	require.Equal(t, []string{
		feishuPhaseDriveReady,
		feishuPhaseDriveIntent,
		feishuPhaseDriveUploaded,
		feishuPhaseMinutesReady,
		feishuPhaseMinutesIntent,
		feishuPhaseMinutesCreated,
		feishuPhaseTranscriptStored,
	}, phases)

	replayed, err := adapter.Resume(
		context.Background(),
		request,
		completed.Checkpoint,
	)
	require.NoError(t, err)
	require.Equal(t, completed.Transcript, replayed.Transcript)
	require.Len(t, runner.calls, 3)
}

func TestFeishuMinutesAdapterDoesNotRepeatUnknownDriveWrite(t *testing.T) {
	audioPath := filepath.Join(t.TempDir(), "episode.mp3")
	require.NoError(t, os.WriteFile(audioPath, []byte("audio"), 0o600))
	audioPath = mustCanonicalPath(t, audioPath)
	runner := &scriptedLarkRunner{
		steps: []scriptedLarkStep{{output: []byte(`{"unexpected":true}`)}},
	}
	digest := strings.Repeat("b", 64)
	adapter, err := newFeishuMinutesAdapterWithRunner(
		runner,
		t.TempDir(),
		func(context.Context, uint) (string, string, error) {
			return audioPath, digest, nil
		},
	)
	require.NoError(t, err)
	request, _ := feishuTestRequest(digest)

	progress, err := adapter.Begin(context.Background(), request)
	var adapterErr *AdapterError
	require.ErrorAs(t, err, &adapterErr)
	require.Equal(t, "lark_drive_result_unknown", adapterErr.ErrorCode)
	require.True(t, adapterErr.ResultUnknown)
	require.Equal(
		t,
		feishuPhaseDriveIntent,
		mustDecodeFeishuCheckpoint(t, progress.Checkpoint).Phase,
	)

	_, err = adapter.Resume(context.Background(), request, progress.Checkpoint)
	require.ErrorAs(t, err, &adapterErr)
	require.Equal(t, "lark_drive_result_unknown", adapterErr.ErrorCode)
	require.Len(t, runner.calls, 1)
}

func TestFeishuMinutesAdapterKnownAuthFailureReturnsSafeReadyCheckpoint(t *testing.T) {
	audioPath := filepath.Join(t.TempDir(), "episode.mp3")
	require.NoError(t, os.WriteFile(audioPath, []byte("audio"), 0o600))
	audioPath = mustCanonicalPath(t, audioPath)
	runner := &scriptedLarkRunner{
		steps: []scriptedLarkStep{{
			err: &larkCommandError{
				exitCode: 1,
				stderr: []byte(`{
					"ok":false,
					"error":{"type":"auth_required","message":"login required"}
				}`),
				cause: errors.New("exit status 1"),
			},
		}},
	}
	digest := strings.Repeat("c", 64)
	adapter, err := newFeishuMinutesAdapterWithRunner(
		runner,
		t.TempDir(),
		func(context.Context, uint) (string, string, error) {
			return audioPath, digest, nil
		},
	)
	require.NoError(t, err)
	request, _ := feishuTestRequest(digest)

	progress, err := adapter.Begin(context.Background(), request)
	var adapterErr *AdapterError
	require.ErrorAs(t, err, &adapterErr)
	require.Equal(t, "lark_auth_expired", adapterErr.ErrorCode)
	require.False(t, adapterErr.ResultUnknown)
	require.Equal(
		t,
		feishuPhaseDriveReady,
		mustDecodeFeishuCheckpoint(t, progress.Checkpoint).Phase,
	)
}

func TestFeishuMinutesAdapterRejectsTranscriptTraversal(t *testing.T) {
	workRoot := t.TempDir()
	runDirectory := filepath.Join(workRoot, "run-91")
	require.NoError(t, os.MkdirAll(runDirectory, 0o700))
	outside := filepath.Join(workRoot, "outside.txt")
	require.NoError(t, os.WriteFile(outside, []byte("secret"), 0o600))
	runner := &scriptedLarkRunner{steps: []scriptedLarkStep{{
		output: []byte(`{"minutes":[{"minute_token":"obcn_safe_123","artifacts":{"transcript_file":"../outside.txt"}}]}`),
	}}}
	adapter, err := newFeishuMinutesAdapterWithRunner(
		runner,
		workRoot,
		func(context.Context, uint) (string, string, error) {
			return "", "", errors.New("unused")
		},
	)
	require.NoError(t, err)
	digest := strings.Repeat("d", 64)
	request, _ := feishuTestRequest(digest)
	checkpoint, err := encodeFeishuCheckpoint(feishuCheckpoint{
		Version:     feishuCheckpointVersion,
		Phase:       feishuPhaseMinutesCreated,
		AudioDigest: digest,
		FileToken:   "boxcn_safe_123",
		MinuteToken: "obcn_safe_123",
		MinuteURL:   "https://example.feishu.cn/minutes/obcn_safe_123",
	})
	require.NoError(t, err)

	_, err = adapter.Resume(context.Background(), request, checkpoint)
	var adapterErr *AdapterError
	require.ErrorAs(t, err, &adapterErr)
	require.Equal(t, "lark_protocol_error", adapterErr.ErrorCode)
}

func feishuTestRequest(
	digest string,
) (TranscriptionRequest, *[]json.RawMessage) {
	persisted := &[]json.RawMessage{}
	return TranscriptionRequest{
		RunID:           91,
		EpisodeID:       42,
		AudioDigest:     digest,
		PipelineVersion: "pipeline-v1",
		PersistCheckpoint: func(
			_ context.Context,
			_ string,
			state json.RawMessage,
		) error {
			*persisted = append(*persisted, append(json.RawMessage(nil), state...))
			return nil
		},
	}, persisted
}

func mustDecodeFeishuCheckpoint(
	t *testing.T,
	state json.RawMessage,
) feishuCheckpoint {
	t.Helper()
	checkpoint, err := decodeFeishuCheckpoint(state)
	require.NoError(t, err)
	return checkpoint
}

type scriptedLarkCall struct {
	cwd  string
	args []string
}

type scriptedLarkStep struct {
	output       []byte
	err          error
	beforeReturn func(string)
}

type scriptedLarkRunner struct {
	steps []scriptedLarkStep
	calls []scriptedLarkCall
}

func mustCanonicalPath(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	require.NoError(t, err)
	return filepath.Clean(resolved)
}

func (r *scriptedLarkRunner) Run(
	_ context.Context,
	cwd string,
	args ...string,
) ([]byte, error) {
	r.calls = append(r.calls, scriptedLarkCall{
		cwd:  cwd,
		args: append([]string(nil), args...),
	})
	if len(r.steps) == 0 {
		return nil, errors.New("unexpected lark-cli call")
	}
	step := r.steps[0]
	r.steps = r.steps[1:]
	if step.beforeReturn != nil {
		step.beforeReturn(cwd)
	}
	return append([]byte(nil), step.output...), step.err
}
