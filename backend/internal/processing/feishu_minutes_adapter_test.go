package processing

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/png"
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
				output: []byte(`{"minutes":[{"minute_token":"obcn_minute_123","artifacts":{"summary":"## 核心观点\n\n原生妙记纪要","chapters":[],"keywords":[],"transcript_file":"detail/transcript.txt"}}]}`),
				beforeReturn: func(cwd string) {
					require.NoError(t, os.WriteFile(
						filepath.Join(cwd, "detail", "transcript.txt"),
						[]byte("张三 00:00:00.195\n第一段\n\n李四 00:01:02.340\n第二段\n"),
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
	require.Contains(t, completed.MinutesSummary, "原生妙记纪要")
	require.Contains(t, completed.Transcript, "张三 00:00:00.195")
	require.Equal(t, []TranscriptSegment{
		{Order: 1, Speaker: "张三", StartMS: 195, Text: "第一段"},
		{Order: 2, Speaker: "李四", StartMS: 62340, Text: "第二段"},
	}, completed.Segments)
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
	require.Equal(t, completed.MinutesSummary, replayed.MinutesSummary)
	require.Equal(t, completed.Transcript, replayed.Transcript)
	require.Equal(t, completed.Segments, replayed.Segments)
	require.Len(t, runner.calls, 3)
}

func TestFeishuMinutesAdapterPreservesLegacyCompletionWithoutV2Artifacts(t *testing.T) {
	workRoot := t.TempDir()
	digest := strings.Repeat("7", 64)
	runner := &scriptedLarkRunner{steps: []scriptedLarkStep{{
		output: []byte(`{"minutes":[{"minute_token":"obcn_legacy_123","artifacts":{"transcript_file":"detail/transcript.txt"}}]}`),
		beforeReturn: func(cwd string) {
			require.NoError(t, os.WriteFile(
				filepath.Join(cwd, "detail", "transcript.txt"),
				[]byte("没有说话人与时间戳的旧版逐字稿"),
				0o600,
			))
		},
	}}}
	adapter, err := newFeishuMinutesAdapterWithRunner(
		runner,
		workRoot,
		func(context.Context, uint) (string, string, error) {
			return "", "", errors.New("unused")
		},
	)
	require.NoError(t, err)
	request, _ := feishuTestRequest(digest)
	request.PipelineVersion = "focus-processing-v1"
	checkpoint, err := encodeFeishuCheckpoint(feishuCheckpoint{
		Version:     feishuCheckpointVersion,
		Phase:       feishuPhaseMinutesCreated,
		AudioDigest: digest,
		FileToken:   "boxcn_legacy_123",
		MinuteToken: "obcn_legacy_123",
		MinuteURL:   "https://example.feishu.cn/minutes/obcn_legacy_123",
	})
	require.NoError(t, err)

	completed, err := adapter.Resume(context.Background(), request, checkpoint)
	require.NoError(t, err)
	require.Equal(t, ExternalProgressCompleted, completed.Status)
	require.Equal(
		t,
		"# Transcript\n\n没有说话人与时间戳的旧版逐字稿\n",
		completed.Transcript,
	)
	require.Empty(t, completed.MinutesSummary)
	require.Empty(t, completed.Segments)

	replayed, err := adapter.Resume(
		context.Background(),
		request,
		completed.Checkpoint,
	)
	require.NoError(t, err)
	require.Equal(t, completed.Transcript, replayed.Transcript)
	require.Empty(t, replayed.MinutesSummary)
	require.Empty(t, replayed.Segments)
	require.Len(t, runner.calls, 1)
}

func TestFeishuMinutesAdapterWaitsForMissingAndTemporarilyEmptySummary(t *testing.T) {
	workRoot := t.TempDir()
	digest := strings.Repeat("8", 64)
	runner := &scriptedLarkRunner{steps: []scriptedLarkStep{
		{
			output: []byte(`{"minutes":[{"minute_token":"obcn_incomplete_123","artifacts":{"transcript_file":"detail/transcript.txt"}}]}`),
		},
		{
			output: []byte(`{"minutes":[{"minute_token":"obcn_incomplete_123","artifacts":{"summary":"","transcript_file":"detail/transcript.txt"}}]}`),
		},
		{
			output: []byte(`{"minutes":[{"minute_token":"obcn_incomplete_123","artifacts":{"summary":"完整纪要","transcript_file":"detail/transcript.txt"}}]}`),
			beforeReturn: func(cwd string) {
				require.NoError(t, os.WriteFile(
					filepath.Join(cwd, "detail", "transcript.txt"),
					[]byte("Speaker 1 00:00:01.890\n开头\n\nSpeaker 2 00:18:16.830\n中段\n"),
					0o600,
				))
			},
		},
	}}
	adapter, err := newFeishuMinutesAdapterWithRunner(
		runner,
		workRoot,
		func(context.Context, uint) (string, string, error) {
			return "", "", errors.New("unused")
		},
	)
	require.NoError(t, err)
	request, _ := feishuTestRequest(digest)
	checkpoint, err := encodeFeishuCheckpoint(feishuCheckpoint{
		Version:     feishuCheckpointVersion,
		Phase:       feishuPhaseMinutesCreated,
		AudioDigest: digest,
		FileToken:   "boxcn_incomplete_123",
		MinuteToken: "obcn_incomplete_123",
		MinuteURL:   "https://example.feishu.cn/minutes/obcn_incomplete_123",
	})
	require.NoError(t, err)

	waiting, err := adapter.Resume(context.Background(), request, checkpoint)
	require.NoError(t, err)
	require.Equal(t, ExternalProgressWaiting, waiting.Status)

	waiting, err = adapter.Resume(context.Background(), request, checkpoint)
	require.NoError(t, err)
	require.Equal(t, ExternalProgressWaiting, waiting.Status)

	completed, err := adapter.Resume(context.Background(), request, checkpoint)
	require.NoError(t, err)
	require.Equal(t, ExternalProgressCompleted, completed.Status)
	require.Contains(t, completed.MinutesSummary, "完整纪要")
	require.Len(t, completed.Segments, 2)
}

func TestFeishuMinutesAdapterRejectsTranscriptFormatDrift(t *testing.T) {
	workRoot := t.TempDir()
	digest := strings.Repeat("9", 64)
	runner := &scriptedLarkRunner{steps: []scriptedLarkStep{{
		output: []byte(`{"minutes":[{"minute_token":"obcn_drift_123","artifacts":{"summary":"完整纪要","transcript_file":"detail/transcript.txt"}}]}`),
		beforeReturn: func(cwd string) {
			require.NoError(t, os.WriteFile(
				filepath.Join(cwd, "detail", "transcript.txt"),
				[]byte("没有说话人与时间戳的内容"),
				0o600,
			))
		},
	}}}
	adapter, err := newFeishuMinutesAdapterWithRunner(
		runner,
		workRoot,
		func(context.Context, uint) (string, string, error) {
			return "", "", errors.New("unused")
		},
	)
	require.NoError(t, err)
	request, persisted := feishuTestRequest(digest)
	checkpoint, err := encodeFeishuCheckpoint(feishuCheckpoint{
		Version:     feishuCheckpointVersion,
		Phase:       feishuPhaseMinutesCreated,
		AudioDigest: digest,
		FileToken:   "boxcn_drift_123",
		MinuteToken: "obcn_drift_123",
		MinuteURL:   "https://example.feishu.cn/minutes/obcn_drift_123",
	})
	require.NoError(t, err)

	progress, err := adapter.Resume(context.Background(), request, checkpoint)
	var adapterErr *AdapterError
	require.ErrorAs(t, err, &adapterErr)
	require.Equal(t, "transcript_timeline_invalid", adapterErr.ErrorCode)
	require.Contains(t, adapterErr.SafeMessage, "timestamps")
	require.NotContains(t, adapterErr.SafeMessage, workRoot)
	stored := mustDecodeFeishuCheckpoint(t, progress.Checkpoint)
	require.Equal(t, feishuPhaseTranscriptStored, stored.Phase)
	require.NotEmpty(t, stored.TranscriptRelativePath)
	require.NotEmpty(t, stored.DetailRelativePath)
	require.Len(t, *persisted, 1)
	require.JSONEq(t, string((*persisted)[0]), string(progress.Checkpoint))
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

func TestFeishuMinutesCancellationDispositionIsConservativeAfterRemoteIntent(t *testing.T) {
	adapter := &FeishuMinutesAdapter{}
	digest := strings.Repeat("c", 64)
	for _, phase := range []string{
		feishuPhaseDriveIntent,
		feishuPhaseDriveUploaded,
		feishuPhaseMinutesReady,
		feishuPhaseMinutesIntent,
		feishuPhaseMinutesCreated,
		feishuPhaseTranscriptStored,
	} {
		state, err := encodeFeishuCheckpoint(feishuCheckpoint{
			Version:     feishuCheckpointVersion,
			Phase:       phase,
			AudioDigest: digest,
		})
		require.NoError(t, err)
		disposition, err := adapter.CancellationDisposition(state)
		require.NoError(t, err)
		require.True(t, disposition.RemoteMayContinue, phase)
		require.Contains(t, disposition.Message, "飞书端任务可能继续")
	}

	safeState, err := encodeFeishuCheckpoint(feishuCheckpoint{
		Version:     feishuCheckpointVersion,
		Phase:       feishuPhaseDriveReady,
		AudioDigest: digest,
	})
	require.NoError(t, err)
	safeDisposition, err := adapter.CancellationDisposition(safeState)
	require.NoError(t, err)
	require.False(t, safeDisposition.RemoteMayContinue)

	_, err = adapter.CancellationDisposition(json.RawMessage(`[]`))
	require.Error(t, err)
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

func TestFeishuMinutesAdapterTreatsNestedNotReadyDetailAsWaiting(t *testing.T) {
	workRoot := t.TempDir()
	digest := strings.Repeat("e", 64)
	notReadyOutput := []byte(`{"data":{"minutes":[{"minute_token":"obcn_pending_123","error":"transcript not ready"}]}}`)
	runner := &scriptedLarkRunner{steps: []scriptedLarkStep{{
		output: notReadyOutput,
		err: &larkCommandError{
			exitCode: 1,
			stdout:   notReadyOutput,
			stderr:   []byte("provider returned a nonzero exit"),
			cause:    errors.New("exit status 1"),
		},
	}}}
	adapter, err := newFeishuMinutesAdapterWithRunner(
		runner,
		workRoot,
		func(context.Context, uint) (string, string, error) {
			return "", "", errors.New("unused")
		},
	)
	require.NoError(t, err)
	request, _ := feishuTestRequest(digest)
	checkpoint, err := encodeFeishuCheckpoint(feishuCheckpoint{
		Version:     feishuCheckpointVersion,
		Phase:       feishuPhaseMinutesCreated,
		AudioDigest: digest,
		FileToken:   "boxcn_pending_123",
		MinuteToken: "obcn_pending_123",
		MinuteURL:   "https://example.feishu.cn/minutes/obcn_pending_123",
	})
	require.NoError(t, err)

	progress, err := adapter.Resume(context.Background(), request, checkpoint)
	require.NoError(t, err)
	require.Equal(t, ExternalProgressWaiting, progress.Status)
	require.JSONEq(t, string(checkpoint), string(progress.Checkpoint))
	require.Len(t, runner.calls, 1)
}

func TestFeishuMinutesAdapterTreatsNumericCodeNotReadyDetailAsWaiting(t *testing.T) {
	workRoot := t.TempDir()
	digest := strings.Repeat("f", 64)
	notReadyOutput := []byte(`{"data":{"minutes":[{"minute_token":"obcn_pending_456","error":{"code":123,"message":"transcript not ready"}}]}}`)
	runner := &scriptedLarkRunner{steps: []scriptedLarkStep{{
		output: notReadyOutput,
		err: &larkCommandError{
			exitCode: 1,
			stdout:   notReadyOutput,
			stderr:   []byte("provider returned a nonzero exit"),
			cause:    errors.New("exit status 1"),
		},
	}}}
	adapter, err := newFeishuMinutesAdapterWithRunner(
		runner,
		workRoot,
		func(context.Context, uint) (string, string, error) {
			return "", "", errors.New("unused")
		},
	)
	require.NoError(t, err)
	request, _ := feishuTestRequest(digest)
	checkpoint, err := encodeFeishuCheckpoint(feishuCheckpoint{
		Version:     feishuCheckpointVersion,
		Phase:       feishuPhaseMinutesCreated,
		AudioDigest: digest,
		FileToken:   "boxcn_pending_456",
		MinuteToken: "obcn_pending_456",
		MinuteURL:   "https://example.feishu.cn/minutes/obcn_pending_456",
	})
	require.NoError(t, err)

	progress, err := adapter.Resume(context.Background(), request, checkpoint)
	require.NoError(t, err)
	require.Equal(t, ExternalProgressWaiting, progress.Status)
	require.JSONEq(t, string(checkpoint), string(progress.Checkpoint))
	require.Len(t, runner.calls, 1)
}

func TestFeishuMinutesAdapterDoesNotTreatProcessingFailureDetailAsWaiting(t *testing.T) {
	workRoot := t.TempDir()
	digest := strings.Repeat("a", 64)
	failureOutput := []byte(`{"data":{"minutes":[{"minute_token":"obcn_failed_123","error":{"type":"processing_failed","code":123,"message":"transcript processing failed"}}]}}`)
	runner := &scriptedLarkRunner{steps: []scriptedLarkStep{{
		output: failureOutput,
		err: &larkCommandError{
			exitCode: 1,
			stdout:   failureOutput,
			stderr:   []byte("provider returned a nonzero exit"),
			cause:    errors.New("exit status 1"),
		},
	}}}
	adapter, err := newFeishuMinutesAdapterWithRunner(
		runner,
		workRoot,
		func(context.Context, uint) (string, string, error) {
			return "", "", errors.New("unused")
		},
	)
	require.NoError(t, err)
	request, _ := feishuTestRequest(digest)
	checkpoint, err := encodeFeishuCheckpoint(feishuCheckpoint{
		Version:     feishuCheckpointVersion,
		Phase:       feishuPhaseMinutesCreated,
		AudioDigest: digest,
		FileToken:   "boxcn_failed_123",
		MinuteToken: "obcn_failed_123",
		MinuteURL:   "https://example.feishu.cn/minutes/obcn_failed_123",
	})
	require.NoError(t, err)

	progress, err := adapter.Resume(context.Background(), request, checkpoint)
	var adapterErr *AdapterError
	require.ErrorAs(t, err, &adapterErr)
	require.Equal(t, "lark_minutes_unavailable", adapterErr.ErrorCode)
	require.Equal(t, ExternalProgressWaiting, progress.Status)
	require.JSONEq(t, string(checkpoint), string(progress.Checkpoint))
	require.Len(t, runner.calls, 1)
}

func TestFeishuMinutesAdapterRejectsTranscriptTraversal(t *testing.T) {
	workRoot := t.TempDir()
	runDirectory := filepath.Join(workRoot, "run-91")
	require.NoError(t, os.MkdirAll(runDirectory, 0o700))
	outside := filepath.Join(workRoot, "outside.txt")
	require.NoError(t, os.WriteFile(outside, []byte("secret"), 0o600))
	runner := &scriptedLarkRunner{steps: []scriptedLarkStep{{
		output: []byte(`{"minutes":[{"minute_token":"obcn_safe_123","artifacts":{"summary":"summary","transcript_file":"../outside.txt"}}]}`),
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

func TestFeishuMinutesAdapterCapturesReadOnlyNoteEnrichmentAfterCoreComplete(t *testing.T) {
	workRoot := t.TempDir()
	digest := strings.Repeat("1", 64)
	var pngBuf []byte
	pngBuf = mustTestPNG(t)
	runner := &scriptedLarkRunner{steps: []scriptedLarkStep{
		{
			output: []byte(`{"minutes":[{"minute_token":"obcn_rich_123","note_id":"note_abc_123","artifacts":{"summary":"完整纪要","chapters":[{"start_time":1500,"end_time":9000,"title":"开场","summary":"介绍背景"}],"keywords":["AI","产品"],"transcript_file":"detail/transcript.txt"}}]}`),
			beforeReturn: func(cwd string) {
				require.NoError(t, os.MkdirAll(filepath.Join(cwd, "detail"), 0o700))
				require.NoError(t, os.WriteFile(
					filepath.Join(cwd, "detail", "transcript.txt"),
					[]byte("张三 00:00:01.500\n开场\n"),
					0o600,
				))
			},
		},
		{output: []byte(`{"note_doc_token":"docx_note_123"}`)},
		{output: []byte(`{"data":{"document":{"content":"<h1>总结</h1><whiteboard token=\"wbcn_board_123\"/><h1>关键决策</h1><ul><li>采用方案 A</li></ul><h1>金句时刻</h1><blockquote>长期主义</blockquote><p>节奏说明</p><h1>相关链接</h1><p><a href=\"https://example.com/guide\">指南</a></p><p><a href=\"https://feishu.cn/minutes/secret\">内部</a></p>"}}}`)},
		{
			output: []byte(`{"ok":true}`),
			beforeReturn: func(cwd string) {
				require.NoError(t, os.WriteFile(filepath.Join(cwd, "whiteboard-preview"), pngBuf, 0o600))
			},
		},
	}}
	adapter, err := newFeishuMinutesAdapterWithRunner(
		runner,
		workRoot,
		func(context.Context, uint) (string, string, error) {
			return "", "", errors.New("unused")
		},
	)
	require.NoError(t, err)
	request, _ := feishuTestRequest(digest)
	checkpoint, err := encodeFeishuCheckpoint(feishuCheckpoint{
		Version:     feishuCheckpointVersion,
		Phase:       feishuPhaseMinutesCreated,
		AudioDigest: digest,
		FileToken:   "boxcn_rich_123",
		MinuteToken: "obcn_rich_123",
		MinuteURL:   "https://example.feishu.cn/minutes/obcn_rich_123",
	})
	require.NoError(t, err)

	completed, err := adapter.Resume(context.Background(), request, checkpoint)
	require.NoError(t, err)
	require.Equal(t, ExternalProgressCompleted, completed.Status)
	require.Contains(t, completed.MinutesSummary, "完整纪要")
	require.Len(t, completed.Segments, 1)
	require.Equal(t, []MinutesChapter{{
		Order: 1, StartMS: 1500, EndMS: 9000, Title: "开场", Summary: "介绍背景",
	}}, completed.MinutesEnrichment.Chapters)
	require.Equal(t, []string{"AI", "产品"}, completed.MinutesEnrichment.Keywords)
	require.Equal(t, []string{"采用方案 A"}, completed.MinutesEnrichment.Decisions)
	require.Equal(t, []MinutesQuote{{Quote: "长期主义", Explanation: "节奏说明"}}, completed.MinutesEnrichment.Quotes)
	require.Equal(t, []MinutesLink{{Title: "指南", URL: "https://example.com/guide"}}, completed.MinutesEnrichment.Links)
	require.NotNil(t, completed.MinutesEnrichment.Whiteboard)
	require.Equal(t, minutesWhiteboardMediaID, completed.MinutesEnrichment.Whiteboard.MediaID)
	require.Equal(t, "image/png", completed.MinutesEnrichment.Whiteboard.MediaType)
	require.NotEmpty(t, completed.WhiteboardPreview)
	require.Contains(t, completed.RawArtifacts, "note-detail.json")
	require.Contains(t, completed.RawArtifacts, "note-document.json")
	require.NotContains(t, string(mustJSON(t, completed.MinutesEnrichment)), "note_abc_123")
	require.NotContains(t, string(mustJSON(t, completed.MinutesEnrichment)), "docx_note_123")
	require.NotContains(t, string(mustJSON(t, completed.MinutesEnrichment)), "wbcn_board_123")
	require.NotContains(t, string(mustJSON(t, completed.MinutesEnrichment)), "obcn_rich_123")
	require.Len(t, runner.calls, 4)
	require.Equal(t, []string{
		"note", "+detail", "--note-id", "note_abc_123", "--as", "user", "--format", "json",
	}, runner.calls[1].args)
	require.Equal(t, []string{
		"docs", "+fetch", "--doc", "docx_note_123", "--doc-format", "xml", "--detail", "full",
		"--as", "user", "--format", "json",
	}, runner.calls[2].args)
	require.Equal(t, []string{
		"whiteboard", "+query", "--whiteboard-token", "wbcn_board_123",
		"--output_as", "image", "--output", "./whiteboard-preview", "--overwrite", "--as", "user",
	}, runner.calls[3].args)
	assertNoFeishuWriteCommands(t, runner.calls[1:])

	replayed, err := adapter.Resume(context.Background(), request, completed.Checkpoint)
	require.NoError(t, err)
	require.Equal(t, completed.MinutesSummary, replayed.MinutesSummary)
	require.Equal(t, completed.MinutesEnrichment.Chapters, replayed.MinutesEnrichment.Chapters)
	require.Equal(t, completed.MinutesEnrichment.Keywords, replayed.MinutesEnrichment.Keywords)
	require.Equal(t, completed.MinutesEnrichment.Decisions, replayed.MinutesEnrichment.Decisions)
	require.Contains(t, replayed.RawArtifacts, "note-detail.json")
	require.Contains(t, replayed.RawArtifacts, "note-document.json")
	require.Equal(t, "1.0.0", replayed.SkillVersions["lark-note"])
	require.Equal(t, "1.0.0", replayed.SkillVersions["lark-docs"])
	require.Equal(t, "1.0.0", replayed.SkillVersions["lark-whiteboard"])
	require.Len(t, runner.calls, 4)
}

func TestFeishuMinutesAdapterKeepsCoreCompleteWhenEnhancementFails(t *testing.T) {
	cases := []struct {
		name  string
		steps []scriptedLarkStep
	}{
		{
			name: "missing note_id still publishes chapters",
			steps: []scriptedLarkStep{{
				output:       []byte(`{"minutes":[{"minute_token":"obcn_core_123","artifacts":{"summary":"完整纪要","chapters":[{"start_time":0,"title":"开场","summary":"背景"}],"keywords":["主题"],"transcript_file":"detail/transcript.txt"}}]}`),
				beforeReturn: writeCoreTranscript,
			}},
		},
		{
			name: "empty note document",
			steps: []scriptedLarkStep{
				{
					output:       []byte(`{"minutes":[{"minute_token":"obcn_core_123","note_id":"note_empty_123","artifacts":{"summary":"完整纪要","transcript_file":"detail/transcript.txt"}}]}`),
					beforeReturn: writeCoreTranscript,
				},
				{output: []byte(`{"note_doc_token":"docx_empty_123"}`)},
				{output: []byte(`{"data":{"document":{"content":"<h1>总结</h1><p></p>"}}}`)},
			},
		},
		{
			name: "note waiting",
			steps: []scriptedLarkStep{
				{
					output:       []byte(`{"minutes":[{"minute_token":"obcn_core_123","note_id":"note_wait_123","artifacts":{"summary":"完整纪要","transcript_file":"detail/transcript.txt"}}]}`),
					beforeReturn: writeCoreTranscript,
				},
				{err: &larkCommandError{
					exitCode: 1,
					stderr:   []byte(`{"ok":false,"error":{"type":"not_ready","message":"note still processing"}}`),
					cause:    errors.New("exit status 1"),
				}},
			},
		},
		{
			name: "no permission",
			steps: []scriptedLarkStep{
				{
					output:       []byte(`{"minutes":[{"minute_token":"obcn_core_123","note_id":"note_denied_123","artifacts":{"summary":"完整纪要","transcript_file":"detail/transcript.txt"}}]}`),
					beforeReturn: writeCoreTranscript,
				},
				{err: &larkCommandError{
					exitCode: 1,
					stderr:   []byte(`{"ok":false,"error":{"type":"permission_denied","message":"no permission"}}`),
					cause:    errors.New("exit status 1"),
				}},
			},
		},
		{
			name: "command failure",
			steps: []scriptedLarkStep{
				{
					output:       []byte(`{"minutes":[{"minute_token":"obcn_core_123","note_id":"note_fail_123","artifacts":{"summary":"完整纪要","transcript_file":"detail/transcript.txt"}}]}`),
					beforeReturn: writeCoreTranscript,
				},
				{err: errors.New("network down")},
			},
		},
		{
			name: "format drift",
			steps: []scriptedLarkStep{
				{
					output:       []byte(`{"minutes":[{"minute_token":"obcn_core_123","note_id":"note_drift_123","artifacts":{"summary":"完整纪要","transcript_file":"detail/transcript.txt"}}]}`),
					beforeReturn: writeCoreTranscript,
				},
				{output: []byte(`{"note_doc_token":"docx_drift_123"}`)},
				{output: []byte(`{"data":{"document":{"content":"<weird><unknown-block token=\"abc\"/></weird>"}}}`)},
			},
		},
		{
			name: "whiteboard export failure",
			steps: []scriptedLarkStep{
				{
					output:       []byte(`{"minutes":[{"minute_token":"obcn_core_123","note_id":"note_board_123","artifacts":{"summary":"完整纪要","transcript_file":"detail/transcript.txt"}}]}`),
					beforeReturn: writeCoreTranscript,
				},
				{output: []byte(`{"note_doc_token":"docx_board_123"}`)},
				{output: []byte(`{"data":{"document":{"content":"<h1>总结</h1><whiteboard token=\"wbcn_board_fail\"/><h1>关键决策</h1><ul><li>继续推进</li></ul>"}}}`)},
				{err: errors.New("export failed")},
			},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			workRoot := t.TempDir()
			digest := strings.Repeat("2", 64)
			runner := &scriptedLarkRunner{steps: testCase.steps}
			adapter, err := newFeishuMinutesAdapterWithRunner(
				runner,
				workRoot,
				func(context.Context, uint) (string, string, error) {
					return "", "", errors.New("unused")
				},
			)
			require.NoError(t, err)
			request, _ := feishuTestRequest(digest)
			checkpoint, err := encodeFeishuCheckpoint(feishuCheckpoint{
				Version:     feishuCheckpointVersion,
				Phase:       feishuPhaseMinutesCreated,
				AudioDigest: digest,
				FileToken:   "boxcn_core_123",
				MinuteToken: "obcn_core_123",
				MinuteURL:   "https://example.feishu.cn/minutes/obcn_core_123",
			})
			require.NoError(t, err)
			completed, err := adapter.Resume(context.Background(), request, checkpoint)
			require.NoError(t, err)
			require.Equal(t, ExternalProgressCompleted, completed.Status)
			require.Contains(t, completed.MinutesSummary, "完整纪要")
			require.NotEmpty(t, completed.Transcript)
			require.NotEmpty(t, completed.Segments)
			assertNoFeishuWriteCommands(t, runner.calls)
			require.NotContains(t, string(mustJSON(t, completed.MinutesEnrichment)), "note_")
			require.NotContains(t, string(mustJSON(t, completed.MinutesEnrichment)), "docx_")
			require.NotContains(t, string(mustJSON(t, completed.MinutesEnrichment)), "wbcn_")
		})
	}
}

func writeCoreTranscript(cwd string) {
	_ = os.MkdirAll(filepath.Join(cwd, "detail"), 0o700)
	_ = os.WriteFile(
		filepath.Join(cwd, "detail", "transcript.txt"),
		[]byte("张三 00:00:01.500\n开场\n"),
		0o600,
	)
}

func assertNoFeishuWriteCommands(t *testing.T, calls []scriptedLarkCall) {
	t.Helper()
	for _, call := range calls {
		if len(call.args) < 2 {
			continue
		}
		resource, action := call.args[0], call.args[1]
		if action == "+upload" || action == "+update" || action == "+create" ||
			action == "+delete" || action == "+todo" || action == "+summary" ||
			action == "+speaker-replace" || action == "+word-replace" ||
			action == "+media-insert" {
			t.Fatalf("unexpected Feishu write command: %v", call.args)
		}
		if resource == "drive" && action != "+detail" {
			if action == "+upload" {
				t.Fatalf("unexpected Drive write: %v", call.args)
			}
		}
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	require.NoError(t, err)
	return encoded
}

func mustTestPNG(t *testing.T) []byte {
	t.Helper()
	var buffer bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	require.NoError(t, png.Encode(&buffer, img))
	return buffer.Bytes()
}

func feishuTestRequest(
	digest string,
) (TranscriptionRequest, *[]json.RawMessage) {
	persisted := &[]json.RawMessage{}
	return TranscriptionRequest{
		RunID:           91,
		EpisodeID:       42,
		AudioDigest:     digest,
		PipelineVersion: NativeMinutesPipelineVersion,
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
