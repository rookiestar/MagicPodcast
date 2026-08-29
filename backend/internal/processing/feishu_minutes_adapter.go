package processing

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	feishuMinutesAdapterName    = "feishu-minutes-cli"
	feishuMinutesAdapterVersion = "feishu-minutes-cli-v1"
	feishuCheckpointVersion     = 1
	maxTranscriptBytes          = 128 << 20

	feishuPhaseDriveReady       = "drive_upload_ready"
	feishuPhaseDriveIntent      = "drive_upload_intent"
	feishuPhaseDriveUploaded    = "drive_uploaded"
	feishuPhaseMinutesReady     = "minutes_upload_ready"
	feishuPhaseMinutesIntent    = "minutes_upload_intent"
	feishuPhaseMinutesCreated   = "minutes_created"
	feishuPhaseTranscriptStored = "transcript_stored"
)

var larkTokenPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{4,512}$`)

type ReadyAudioLookup func(
	context.Context,
	uint,
) (path string, sha256 string, err error)

type FeishuMinutesAdapter struct {
	runner     larkCommandRunner
	workRoot   string
	readyAudio ReadyAudioLookup
}

type feishuCheckpoint struct {
	Version                int    `json:"version"`
	Phase                  string `json:"phase"`
	AudioDigest            string `json:"audio_digest"`
	FileToken              string `json:"file_token,omitempty"`
	MinuteToken            string `json:"minute_token,omitempty"`
	MinuteURL              string `json:"minute_url,omitempty"`
	TranscriptRelativePath string `json:"transcript_relative_path,omitempty"`
	DetailRelativePath     string `json:"detail_relative_path,omitempty"`
}

func NewFeishuMinutesAdapter(
	command string,
	workRoot string,
	readyAudio ReadyAudioLookup,
) (*FeishuMinutesAdapter, error) {
	runner, err := newExecLarkCLI(command)
	if err != nil {
		return nil, err
	}
	return newFeishuMinutesAdapterWithRunner(runner, workRoot, readyAudio)
}

func newFeishuMinutesAdapterWithRunner(
	runner larkCommandRunner,
	workRoot string,
	readyAudio ReadyAudioLookup,
) (*FeishuMinutesAdapter, error) {
	if runner == nil || readyAudio == nil {
		return nil, fmt.Errorf("Feishu Minutes adapter dependencies are required")
	}
	workRoot = strings.TrimSpace(workRoot)
	if !filepath.IsAbs(workRoot) {
		return nil, fmt.Errorf("Feishu Minutes work root must be absolute")
	}
	if err := os.MkdirAll(workRoot, 0o700); err != nil {
		return nil, fmt.Errorf("create Feishu Minutes work root: %w", err)
	}
	if err := os.Chmod(workRoot, 0o700); err != nil {
		return nil, fmt.Errorf("protect Feishu Minutes work root: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(filepath.Clean(workRoot))
	if err != nil {
		return nil, fmt.Errorf("resolve Feishu Minutes work root: %w", err)
	}
	return &FeishuMinutesAdapter{
		runner:     runner,
		workRoot:   filepath.Clean(resolved),
		readyAudio: readyAudio,
	}, nil
}

func (a *FeishuMinutesAdapter) Name() string {
	return feishuMinutesAdapterName
}

func (a *FeishuMinutesAdapter) Version() string {
	return feishuMinutesAdapterVersion
}

func (a *FeishuMinutesAdapter) Begin(
	ctx context.Context,
	request TranscriptionRequest,
) (TranscriptionProgress, error) {
	if err := validateFeishuRequest(request); err != nil {
		return TranscriptionProgress{}, err
	}
	checkpoint := feishuCheckpoint{
		Version:     feishuCheckpointVersion,
		Phase:       feishuPhaseDriveReady,
		AudioDigest: request.AudioDigest,
	}
	return a.uploadDriveFile(ctx, request, checkpoint)
}

func (a *FeishuMinutesAdapter) Resume(
	ctx context.Context,
	request TranscriptionRequest,
	state json.RawMessage,
) (TranscriptionProgress, error) {
	if err := validateFeishuRequest(request); err != nil {
		return TranscriptionProgress{}, err
	}
	checkpoint, err := decodeFeishuCheckpoint(state)
	if err != nil || checkpoint.AudioDigest != request.AudioDigest {
		return TranscriptionProgress{}, NewAdapterError(
			"invalid_external_checkpoint",
			"Feishu processing checkpoint is invalid",
			false,
		)
	}
	switch checkpoint.Phase {
	case feishuPhaseDriveReady:
		return a.uploadDriveFile(ctx, request, checkpoint)
	case feishuPhaseDriveIntent:
		return progressWithCheckpoint(checkpoint), NewUnknownExternalResultError(
			"lark_drive_result_unknown",
			"Feishu Drive upload result is unknown and will not be repeated automatically",
		)
	case feishuPhaseDriveUploaded, feishuPhaseMinutesReady:
		return a.createMinute(ctx, request, checkpoint)
	case feishuPhaseMinutesIntent:
		return progressWithCheckpoint(checkpoint), NewUnknownExternalResultError(
			"lark_minutes_result_unknown",
			"Feishu Minutes creation result is unknown and will not be repeated automatically",
		)
	case feishuPhaseMinutesCreated:
		return a.readMinute(ctx, request, checkpoint)
	case feishuPhaseTranscriptStored:
		return a.readStoredResult(ctx, request.PipelineVersion, checkpoint)
	default:
		return TranscriptionProgress{}, NewAdapterError(
			"invalid_external_checkpoint",
			"Feishu processing checkpoint has an unknown phase",
			false,
		)
	}
}

func (a *FeishuMinutesAdapter) Cancel(
	context.Context,
	uint,
	json.RawMessage,
) error {
	// lark-cli has no supported cancellation API for an already-created
	// upload/Minute. Cancelling the command context stops local work; remote
	// resources are deliberately retained and traceable.
	return nil
}

func (a *FeishuMinutesAdapter) CancellationDisposition(
	state json.RawMessage,
) (TranscriptionCancellationDisposition, error) {
	if len(state) == 0 {
		return TranscriptionCancellationDisposition{}, nil
	}
	checkpoint, err := decodeFeishuCheckpoint(state)
	if err != nil {
		return TranscriptionCancellationDisposition{}, fmt.Errorf("decode cancellation checkpoint: %w", err)
	}
	switch checkpoint.Phase {
	case feishuPhaseDriveIntent,
		feishuPhaseDriveUploaded,
		feishuPhaseMinutesReady,
		feishuPhaseMinutesIntent,
		feishuPhaseMinutesCreated,
		feishuPhaseTranscriptStored:
		return TranscriptionCancellationDisposition{
			RemoteMayContinue: true,
			Message:           "已取消本机加工；飞书端任务可能继续，已创建的远端资源会保留。",
		}, nil
	default:
		return TranscriptionCancellationDisposition{}, nil
	}
}

func (a *FeishuMinutesAdapter) uploadDriveFile(
	ctx context.Context,
	request TranscriptionRequest,
	checkpoint feishuCheckpoint,
) (TranscriptionProgress, error) {
	audioPath, audioDigest, err := a.readyAudio(ctx, request.EpisodeID)
	if err != nil {
		return TranscriptionProgress{}, NewAdapterError(
			"audio_not_ready",
			"downloaded audio is not ready",
			false,
		)
	}
	if audioDigest != request.AudioDigest {
		return TranscriptionProgress{}, NewAdapterError(
			"audio_digest_mismatch",
			"downloaded audio no longer matches the processing run",
			false,
		)
	}
	audioDirectory, audioName, err := validateReadyAudioPath(audioPath)
	if err != nil {
		return TranscriptionProgress{}, err
	}

	checkpoint.Phase = feishuPhaseDriveReady
	if err := persistFeishuCheckpoint(ctx, request, checkpoint); err != nil {
		return TranscriptionProgress{}, err
	}
	checkpoint.Phase = feishuPhaseDriveIntent
	if err := persistFeishuCheckpoint(ctx, request, checkpoint); err != nil {
		return TranscriptionProgress{}, err
	}

	output, commandErr := a.runner.Run(
		ctx,
		audioDirectory,
		"drive",
		"+upload",
		"--file",
		"./"+audioName,
		"--as",
		"user",
		"--format",
		"json",
	)
	if commandErr != nil {
		return a.handleWriteFailure(ctx, request, checkpoint, feishuPhaseDriveReady, commandErr)
	}
	fileToken, err := parseDriveFileToken(output)
	if err != nil {
		return progressWithCheckpoint(checkpoint), NewUnknownExternalResultError(
			"lark_drive_result_unknown",
			"Feishu Drive upload succeeded without a recoverable file identity",
		)
	}
	checkpoint.Phase = feishuPhaseDriveUploaded
	checkpoint.FileToken = fileToken
	if err := persistFeishuCheckpoint(ctx, request, checkpoint); err != nil {
		return progressWithCheckpoint(checkpoint), NewUnknownExternalResultError(
			"lark_drive_result_unknown",
			"Feishu Drive upload identity could not be recorded",
		)
	}
	return progressWithCheckpoint(checkpoint), nil
}

func (a *FeishuMinutesAdapter) createMinute(
	ctx context.Context,
	request TranscriptionRequest,
	checkpoint feishuCheckpoint,
) (TranscriptionProgress, error) {
	if !larkTokenPattern.MatchString(checkpoint.FileToken) {
		return TranscriptionProgress{}, NewAdapterError(
			"invalid_external_checkpoint",
			"Feishu Drive identity is invalid",
			false,
		)
	}
	runDirectory, err := a.runDirectory(request.RunID)
	if err != nil {
		return TranscriptionProgress{}, err
	}
	checkpoint.Phase = feishuPhaseMinutesReady
	if err := persistFeishuCheckpoint(ctx, request, checkpoint); err != nil {
		return TranscriptionProgress{}, err
	}
	checkpoint.Phase = feishuPhaseMinutesIntent
	if err := persistFeishuCheckpoint(ctx, request, checkpoint); err != nil {
		return TranscriptionProgress{}, err
	}

	output, commandErr := a.runner.Run(
		ctx,
		runDirectory,
		"minutes",
		"+upload",
		"--file-token",
		checkpoint.FileToken,
		"--as",
		"user",
		"--format",
		"json",
	)
	if commandErr != nil {
		return a.handleWriteFailure(ctx, request, checkpoint, feishuPhaseMinutesReady, commandErr)
	}
	minuteURL, minuteToken, err := parseMinuteIdentity(output)
	if err != nil {
		return progressWithCheckpoint(checkpoint), NewUnknownExternalResultError(
			"lark_minutes_result_unknown",
			"Feishu Minutes creation succeeded without a recoverable identity",
		)
	}
	checkpoint.Phase = feishuPhaseMinutesCreated
	checkpoint.MinuteURL = minuteURL
	checkpoint.MinuteToken = minuteToken
	if err := persistFeishuCheckpoint(ctx, request, checkpoint); err != nil {
		return progressWithCheckpoint(checkpoint), NewUnknownExternalResultError(
			"lark_minutes_result_unknown",
			"Feishu Minutes identity could not be recorded",
		)
	}
	return progressWithCheckpoint(checkpoint), nil
}

func (a *FeishuMinutesAdapter) readMinute(
	ctx context.Context,
	request TranscriptionRequest,
	checkpoint feishuCheckpoint,
) (TranscriptionProgress, error) {
	if !larkTokenPattern.MatchString(checkpoint.MinuteToken) {
		return TranscriptionProgress{}, NewAdapterError(
			"invalid_external_checkpoint",
			"Feishu Minutes identity is invalid",
			false,
		)
	}
	runDirectory, err := a.runDirectory(request.RunID)
	if err != nil {
		return TranscriptionProgress{}, err
	}
	detailDirectory := filepath.Join(runDirectory, "detail")
	if err := os.MkdirAll(detailDirectory, 0o700); err != nil {
		return TranscriptionProgress{}, NewAdapterError(
			"artifact_write_failed",
			"Feishu transcript directory could not be created",
			true,
		)
	}
	output, commandErr := a.runner.Run(
		ctx,
		runDirectory,
		"minutes",
		"+detail",
		"--minute-tokens",
		checkpoint.MinuteToken,
		"--summary",
		"--chapter",
		"--keyword",
		"--transcript",
		"--overwrite",
		"--output-dir",
		"./detail",
		"--as",
		"user",
		"--format",
		"json",
	)
	if commandErr != nil {
		// lark-cli can return exit 1 while still returning a structured per-Minute
		// "not ready" entry on stdout. That is the normal asynchronous state, not
		// a provider outage: retain the existing checkpoint and let the Worker poll
		// again without consuming the retry budget or repeating an external write.
		var larkErr *larkCommandError
		if errors.As(commandErr, &larkErr) {
			if detail, found, parseErr := parseMinuteDetail(
				larkErr.stdout,
				checkpoint.MinuteToken,
			); parseErr == nil && found && detail.Pending &&
				strings.TrimSpace(detail.TranscriptFile) == "" {
				return progressWithCheckpoint(checkpoint), nil
			}
		}
		mapped, _ := classifyLarkCommandError(commandErr)
		if errors.Is(mapped, errLarkMinutesPending) {
			return progressWithCheckpoint(checkpoint), nil
		}
		var adapterErr *AdapterError
		if errors.As(mapped, &adapterErr) && adapterErr.ResultUnknown {
			mapped = NewAdapterError(
				"lark_minutes_unavailable",
				"Feishu Minutes details could not be read",
				true,
			)
		}
		return progressWithCheckpoint(checkpoint), mapped
	}
	detail, found, err := parseMinuteDetail(output, checkpoint.MinuteToken)
	if err != nil {
		return progressWithCheckpoint(checkpoint), NewAdapterError(
			"lark_protocol_error",
			"Feishu Minutes detail output is invalid",
			false,
		)
	}
	if !found || detail.Pending || !detail.TranscriptFilePresent {
		return progressWithCheckpoint(checkpoint), nil
	}
	if request.PipelineVersion == NativeMinutesPipelineVersion &&
		!detail.SummaryPresent {
		return progressWithCheckpoint(checkpoint), nil
	}
	if request.PipelineVersion == NativeMinutesPipelineVersion &&
		strings.TrimSpace(detail.Summary) == "" {
		return progressWithCheckpoint(checkpoint), NewAdapterError(
			"summary_empty",
			"Feishu Minutes summary is empty; retry after the Minute finishes processing",
			false,
		)
	}
	if strings.TrimSpace(detail.TranscriptFile) == "" {
		if request.PipelineVersion != NativeMinutesPipelineVersion {
			return progressWithCheckpoint(checkpoint), nil
		}
		return progressWithCheckpoint(checkpoint), NewAdapterError(
			"transcript_empty",
			"Feishu transcript is empty; retry after the Minute finishes processing",
			false,
		)
	}
	transcriptPath, transcript, err := readManagedTranscript(
		runDirectory,
		detail.TranscriptFile,
	)
	if err != nil {
		return progressWithCheckpoint(checkpoint), err
	}
	if len(bytes.TrimSpace(transcript)) == 0 {
		if request.PipelineVersion != NativeMinutesPipelineVersion {
			return progressWithCheckpoint(checkpoint), nil
		}
		return progressWithCheckpoint(checkpoint), NewAdapterError(
			"transcript_empty",
			"Feishu transcript is empty; retry after the Minute finishes processing",
			false,
		)
	}

	detailPath := filepath.Join(runDirectory, "minutes-detail.json")
	if err := os.WriteFile(detailPath, output, 0o600); err != nil {
		return progressWithCheckpoint(checkpoint), NewAdapterError(
			"artifact_write_failed",
			"Feishu raw detail output could not be stored",
			true,
		)
	}
	transcriptRelative, err := filepath.Rel(a.workRoot, transcriptPath)
	if err != nil || pathEscapesRoot(transcriptRelative) {
		return TranscriptionProgress{}, NewAdapterError(
			"lark_protocol_error",
			"Feishu transcript path escaped the managed directory",
			false,
		)
	}
	detailRelative, err := filepath.Rel(a.workRoot, detailPath)
	if err != nil || pathEscapesRoot(detailRelative) {
		return TranscriptionProgress{}, NewAdapterError(
			"lark_protocol_error",
			"Feishu detail path escaped the managed directory",
			false,
		)
	}
	checkpoint.Phase = feishuPhaseTranscriptStored
	checkpoint.TranscriptRelativePath = filepath.ToSlash(transcriptRelative)
	checkpoint.DetailRelativePath = filepath.ToSlash(detailRelative)
	if err := persistFeishuCheckpoint(ctx, request, checkpoint); err != nil {
		return progressWithCheckpoint(checkpoint), err
	}
	return completedFeishuProgress(
		checkpoint,
		request.PipelineVersion,
		detail.Summary,
		transcript,
		output,
	)
}

func (a *FeishuMinutesAdapter) readStoredResult(
	ctx context.Context,
	pipelineVersion string,
	checkpoint feishuCheckpoint,
) (TranscriptionProgress, error) {
	transcript, err := a.readStoredFile(
		ctx,
		checkpoint.TranscriptRelativePath,
		maxTranscriptBytes,
	)
	if err != nil || len(bytes.TrimSpace(transcript)) == 0 {
		return TranscriptionProgress{}, NewAdapterError(
			"stored_transcript_unavailable",
			"stored Feishu transcript is unavailable",
			false,
		)
	}
	detail, err := a.readStoredFile(
		ctx,
		checkpoint.DetailRelativePath,
		maxLarkCLIOutputBytes,
	)
	if err != nil {
		return TranscriptionProgress{}, NewAdapterError(
			"stored_transcript_unavailable",
			"stored Feishu detail output is unavailable",
			false,
		)
	}
	summary := ""
	if pipelineVersion == NativeMinutesPipelineVersion {
		parsedDetail, found, parseErr := parseMinuteDetail(
			detail,
			checkpoint.MinuteToken,
		)
		if parseErr != nil || !found ||
			!parsedDetail.SummaryPresent ||
			strings.TrimSpace(parsedDetail.Summary) == "" {
			return TranscriptionProgress{}, NewAdapterError(
				"stored_summary_unavailable",
				"stored Feishu Minutes summary is unavailable",
				false,
			)
		}
		summary = parsedDetail.Summary
	}
	return completedFeishuProgress(
		checkpoint,
		pipelineVersion,
		summary,
		transcript,
		detail,
	)
}

func (a *FeishuMinutesAdapter) handleWriteFailure(
	ctx context.Context,
	request TranscriptionRequest,
	checkpoint feishuCheckpoint,
	readyPhase string,
	commandErr error,
) (TranscriptionProgress, error) {
	mapped, knownNoWrite := classifyLarkCommandError(commandErr)
	if !knownNoWrite {
		code := "lark_result_unknown"
		message := "Feishu write result is unknown and will not be repeated automatically"
		if checkpoint.Phase == feishuPhaseDriveIntent {
			code = "lark_drive_result_unknown"
			message = "Feishu Drive upload result is unknown and will not be repeated automatically"
		} else if checkpoint.Phase == feishuPhaseMinutesIntent {
			code = "lark_minutes_result_unknown"
			message = "Feishu Minutes creation result is unknown and will not be repeated automatically"
		}
		return progressWithCheckpoint(checkpoint), NewUnknownExternalResultError(code, message)
	}
	if errors.Is(mapped, errLarkMinutesPending) {
		mapped = NewAdapterError(
			"lark_request_rejected",
			"Feishu write request was rejected",
			false,
		)
	}
	checkpoint.Phase = readyPhase
	if err := persistFeishuCheckpoint(ctx, request, checkpoint); err != nil {
		return progressWithCheckpoint(checkpoint), err
	}
	return progressWithCheckpoint(checkpoint), mapped
}

func (a *FeishuMinutesAdapter) runDirectory(runID uint) (string, error) {
	if runID == 0 {
		return "", NewAdapterError(
			"invalid_transcription_request",
			"transcription run identity is missing",
			false,
		)
	}
	runDirectory := filepath.Join(a.workRoot, fmt.Sprintf("run-%d", runID))
	if err := os.MkdirAll(runDirectory, 0o700); err != nil {
		return "", NewAdapterError(
			"lark_workdir_unavailable",
			"Feishu CLI working directory could not be created",
			true,
		)
	}
	if err := os.Chmod(runDirectory, 0o700); err != nil {
		return "", NewAdapterError(
			"lark_workdir_unavailable",
			"Feishu CLI working directory could not be protected",
			true,
		)
	}
	resolved, err := filepath.EvalSymlinks(runDirectory)
	if err != nil || filepath.Clean(resolved) != filepath.Clean(runDirectory) {
		return "", NewAdapterError(
			"lark_workdir_unavailable",
			"Feishu CLI working directory is not canonical",
			false,
		)
	}
	relative, err := filepath.Rel(a.workRoot, resolved)
	if err != nil || pathEscapesRoot(relative) {
		return "", NewAdapterError(
			"lark_workdir_unavailable",
			"Feishu CLI working directory escaped the managed root",
			false,
		)
	}
	return filepath.Clean(resolved), nil
}

func (a *FeishuMinutesAdapter) readStoredFile(
	ctx context.Context,
	relativePath string,
	limit int64,
) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if relativePath == "" || filepath.IsAbs(relativePath) {
		return nil, ErrInvalidArtifact
	}
	cleanRelative := filepath.Clean(filepath.FromSlash(relativePath))
	if pathEscapesRoot(cleanRelative) {
		return nil, ErrInvalidArtifact
	}
	path := filepath.Join(a.workRoot, cleanRelative)
	return readBoundedRegularUTF8File(path, limit)
}

func validateFeishuRequest(request TranscriptionRequest) error {
	if request.RunID == 0 ||
		request.EpisodeID == 0 ||
		!sha256Pattern.MatchString(request.AudioDigest) ||
		strings.TrimSpace(request.PipelineVersion) == "" ||
		request.PersistCheckpoint == nil {
		return NewAdapterError(
			"invalid_transcription_request",
			"Feishu transcription request is incomplete",
			false,
		)
	}
	return nil
}

func validateReadyAudioPath(path string) (string, string, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return "", "", NewAdapterError(
			"audio_not_ready",
			"downloaded audio path is invalid",
			false,
		)
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return "", "", NewAdapterError(
			"audio_not_ready",
			"downloaded audio is unavailable",
			false,
		)
	}
	directory := filepath.Dir(path)
	resolvedDirectory, err := filepath.EvalSymlinks(directory)
	if err != nil || filepath.Clean(resolvedDirectory) != filepath.Clean(directory) {
		return "", "", NewAdapterError(
			"audio_not_ready",
			"downloaded audio directory is not canonical",
			false,
		)
	}
	name := filepath.Base(path)
	if name == "." || name == ".." || strings.ContainsAny(name, `/\`) {
		return "", "", NewAdapterError(
			"audio_not_ready",
			"downloaded audio filename is invalid",
			false,
		)
	}
	return filepath.Clean(directory), name, nil
}

func persistFeishuCheckpoint(
	ctx context.Context,
	request TranscriptionRequest,
	checkpoint feishuCheckpoint,
) error {
	state, err := encodeFeishuCheckpoint(checkpoint)
	if err != nil {
		return NewAdapterError(
			"invalid_external_checkpoint",
			"Feishu checkpoint could not be encoded",
			false,
		)
	}
	status := ExternalProgressWaiting
	if checkpoint.Phase == feishuPhaseTranscriptStored {
		status = ExternalProgressCompleted
	}
	if err := request.PersistCheckpoint(context.WithoutCancel(ctx), status, state); err != nil {
		return NewUnknownExternalResultError(
			"checkpoint_write_failed",
			"Feishu processing checkpoint could not be saved",
		)
	}
	return nil
}

func progressWithCheckpoint(checkpoint feishuCheckpoint) TranscriptionProgress {
	state, _ := encodeFeishuCheckpoint(checkpoint)
	return TranscriptionProgress{
		Status:     ExternalProgressWaiting,
		Checkpoint: state,
	}
}

func completedFeishuProgress(
	checkpoint feishuCheckpoint,
	pipelineVersion string,
	summary string,
	transcript []byte,
	detail []byte,
) (TranscriptionProgress, error) {
	state, _ := encodeFeishuCheckpoint(checkpoint)
	if pipelineVersion != NativeMinutesPipelineVersion {
		text := strings.TrimSpace(string(transcript))
		if !strings.HasPrefix(text, "#") {
			text = "# Transcript\n\n" + text
		}
		return TranscriptionProgress{
			Status:     ExternalProgressCompleted,
			Checkpoint: state,
			Transcript: text + "\n",
			RawArtifacts: map[string][]byte{
				"minutes-detail.json":    append([]byte(nil), detail...),
				"minutes-transcript.txt": append([]byte(nil), transcript...),
			},
			SourceRefs: map[string]string{
				"transcription":    "feishu-minutes",
				"feishu_drive_ref": restrictedIdentityRef(checkpoint.FileToken),
				"feishu_minute_ref": restrictedIdentityRef(
					checkpoint.MinuteToken,
				),
			},
			SkillVersions: map[string]string{
				"lark-drive":   "1.0.0",
				"lark-minutes": "1.0.0",
			},
		}, nil
	}
	normalizedSummary, err := normalizeMinutesSummary(summary)
	if err != nil {
		return TranscriptionProgress{}, err
	}
	normalizedTranscript, segments, err := normalizeTranscript(string(transcript))
	if err != nil {
		return TranscriptionProgress{}, err
	}
	return TranscriptionProgress{
		Status:         ExternalProgressCompleted,
		Checkpoint:     state,
		MinutesSummary: normalizedSummary,
		Transcript:     normalizedTranscript,
		Segments:       segments,
		RawArtifacts: map[string][]byte{
			"minutes-detail.json":    append([]byte(nil), detail...),
			"minutes-transcript.txt": append([]byte(nil), transcript...),
		},
		SourceRefs: map[string]string{
			"transcription":    "feishu-minutes",
			"feishu_drive_ref": restrictedIdentityRef(checkpoint.FileToken),
			"feishu_minute_ref": restrictedIdentityRef(
				checkpoint.MinuteToken,
			),
		},
		SkillVersions: map[string]string{
			"lark-drive":   "1.0.0",
			"lark-minutes": "1.0.0",
		},
	}, nil
}

func restrictedIdentityRef(identity string) string {
	sum := sha256.Sum256([]byte(identity))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func encodeFeishuCheckpoint(checkpoint feishuCheckpoint) (json.RawMessage, error) {
	if checkpoint.Version != feishuCheckpointVersion ||
		!sha256Pattern.MatchString(checkpoint.AudioDigest) {
		return nil, fmt.Errorf("invalid Feishu checkpoint")
	}
	return json.Marshal(checkpoint)
}

func decodeFeishuCheckpoint(state json.RawMessage) (feishuCheckpoint, error) {
	if len(state) == 0 || !json.Valid(state) {
		return feishuCheckpoint{}, fmt.Errorf("invalid Feishu checkpoint")
	}
	var checkpoint feishuCheckpoint
	decoder := json.NewDecoder(bytes.NewReader(state))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&checkpoint); err != nil {
		return feishuCheckpoint{}, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return feishuCheckpoint{}, fmt.Errorf("trailing Feishu checkpoint data")
	}
	if checkpoint.Version != feishuCheckpointVersion ||
		!sha256Pattern.MatchString(checkpoint.AudioDigest) {
		return feishuCheckpoint{}, fmt.Errorf("invalid Feishu checkpoint")
	}
	return checkpoint, nil
}

type driveUploadOutput struct {
	FileToken string `json:"file_token"`
	Data      struct {
		FileToken string `json:"file_token"`
	} `json:"data"`
	Result struct {
		FileToken string `json:"file_token"`
	} `json:"result"`
}

func parseDriveFileToken(output []byte) (string, error) {
	var decoded driveUploadOutput
	if err := strictJSONDecode(output, &decoded); err != nil {
		return "", err
	}
	return uniqueLarkToken(
		decoded.FileToken,
		decoded.Data.FileToken,
		decoded.Result.FileToken,
	)
}

type minutesUploadOutput struct {
	MinuteURL string `json:"minute_url"`
	Data      struct {
		MinuteURL string `json:"minute_url"`
	} `json:"data"`
	Result struct {
		MinuteURL string `json:"minute_url"`
	} `json:"result"`
}

func parseMinuteIdentity(output []byte) (string, string, error) {
	var decoded minutesUploadOutput
	if err := strictJSONDecode(output, &decoded); err != nil {
		return "", "", err
	}
	minuteURL, err := uniqueNonEmptyString(
		decoded.MinuteURL,
		decoded.Data.MinuteURL,
		decoded.Result.MinuteURL,
	)
	if err != nil {
		return "", "", err
	}
	parsed, err := url.ParseRequestURI(minuteURL)
	if err != nil ||
		parsed.Scheme != "https" ||
		parsed.Host == "" ||
		parsed.User != nil ||
		parsed.RawQuery != "" ||
		parsed.Fragment != "" {
		return "", "", fmt.Errorf("invalid minute URL")
	}
	segments := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(segments) < 2 || segments[len(segments)-2] != "minutes" {
		return "", "", fmt.Errorf("invalid minute URL path")
	}
	token := segments[len(segments)-1]
	if !larkTokenPattern.MatchString(token) {
		return "", "", fmt.Errorf("invalid minute token")
	}
	return minuteURL, token, nil
}

type minuteDetail struct {
	MinuteToken           string
	Summary               string
	SummaryPresent        bool
	TranscriptFile        string
	TranscriptFilePresent bool
	Pending               bool
}

type minuteDetailEntry struct {
	MinuteToken string `json:"minute_token"`
	Artifacts   struct {
		Summary        *string `json:"summary"`
		TranscriptFile *string `json:"transcript_file"`
	} `json:"artifacts"`
	Error json.RawMessage `json:"error"`
}

type minuteDetailOutput struct {
	Minutes []minuteDetailEntry `json:"minutes"`
	Data    struct {
		Minutes []minuteDetailEntry `json:"minutes"`
	} `json:"data"`
	Result struct {
		Minutes []minuteDetailEntry `json:"minutes"`
	} `json:"result"`
}

func parseMinuteDetail(
	output []byte,
	expectedToken string,
) (minuteDetail, bool, error) {
	var decoded minuteDetailOutput
	if err := strictJSONDecode(output, &decoded); err != nil {
		return minuteDetail{}, false, err
	}
	entries := decoded.Minutes
	if len(entries) == 0 {
		entries = decoded.Data.Minutes
	}
	if len(entries) == 0 {
		entries = decoded.Result.Minutes
	}
	for _, entry := range entries {
		if entry.MinuteToken == expectedToken {
			detail := minuteDetail{
				MinuteToken: entry.MinuteToken,
				Pending:     minuteDetailEntryPending(entry.Error),
			}
			if entry.Artifacts.Summary != nil {
				detail.Summary = *entry.Artifacts.Summary
				detail.SummaryPresent = true
			}
			if entry.Artifacts.TranscriptFile != nil {
				detail.TranscriptFile = *entry.Artifacts.TranscriptFile
				detail.TranscriptFilePresent = true
			}
			return detail, true, nil
		}
	}
	return minuteDetail{}, false, nil
}

func minuteDetailEntryPending(raw json.RawMessage) bool {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return false
	}
	var message string
	if err := json.Unmarshal(raw, &message); err == nil {
		return minutePendingMessage(message)
	}
	var structured struct {
		Type    string          `json:"type"`
		Code    json.RawMessage `json:"code"`
		Message string          `json:"message"`
		Status  string          `json:"status"`
	}
	if err := json.Unmarshal(raw, &structured); err != nil {
		return false
	}
	return larkMinutesPending(
		structured.Type,
		rawJSONScalar(structured.Code),
		structured.Message,
		structured.Status,
	)
}

func minutePendingMessage(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	return strings.Contains(value, "not ready") ||
		strings.Contains(value, "still processing") ||
		strings.Contains(value, "currently processing") ||
		strings.Contains(value, "being processed")
}

func readManagedTranscript(
	runDirectory string,
	reportedPath string,
) (string, []byte, error) {
	reportedPath = strings.TrimSpace(reportedPath)
	if reportedPath == "" {
		return "", nil, NewAdapterError(
			"transcript_empty",
			"Feishu transcript is empty",
			false,
		)
	}
	path := reportedPath
	if !filepath.IsAbs(path) {
		path = filepath.Join(runDirectory, filepath.FromSlash(path))
	}
	path = filepath.Clean(path)
	relative, err := filepath.Rel(runDirectory, path)
	if err != nil || pathEscapesRoot(relative) {
		return "", nil, NewAdapterError(
			"lark_protocol_error",
			"Feishu transcript path escaped the managed run directory",
			false,
		)
	}
	content, err := readBoundedRegularUTF8File(path, maxTranscriptBytes)
	if err != nil {
		return "", nil, NewAdapterError(
			"transcript_unavailable",
			"Feishu transcript file is unavailable",
			true,
		)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return "", nil, NewAdapterError(
			"artifact_write_failed",
			"Feishu transcript file could not be protected",
			false,
		)
	}
	return path, content, nil
}

func readBoundedRegularUTF8File(path string, limit int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return nil, ErrInvalidArtifact
	}
	if info.Size() < 0 || info.Size() > limit {
		return nil, ErrInvalidArtifact
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil || filepath.Clean(resolved) != filepath.Clean(path) {
		return nil, ErrInvalidArtifact
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil || int64(len(content)) > limit || !utf8.Valid(content) {
		return nil, ErrInvalidArtifact
	}
	return content, nil
}

func uniqueLarkToken(values ...string) (string, error) {
	token, err := uniqueNonEmptyString(values...)
	if err != nil || !larkTokenPattern.MatchString(token) {
		return "", fmt.Errorf("invalid lark token")
	}
	return token, nil
}

func uniqueNonEmptyString(values ...string) (string, error) {
	result := ""
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if result != "" && result != value {
			return "", fmt.Errorf("conflicting output identities")
		}
		result = value
	}
	if result == "" {
		return "", fmt.Errorf("missing output identity")
	}
	return result, nil
}

func pathEscapesRoot(relative string) bool {
	return relative == "." ||
		relative == ".." ||
		strings.HasPrefix(relative, ".."+string(filepath.Separator)) ||
		filepath.IsAbs(relative)
}
