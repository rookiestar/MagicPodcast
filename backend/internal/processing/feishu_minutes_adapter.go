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
	"time"
	"unicode/utf8"

	"magicpodcast/internal/models"
)

const (
	feishuMinutesAdapterName            = "feishu-minutes-cli"
	feishuMinutesAdapterVersion         = "feishu-minutes-cli-v1"
	feishuCheckpointVersion             = 1
	maxTranscriptBytes                  = 128 << 20
	maxEnrichmentPendingAttempts        = 3
	minutesEnrichmentDiagnosticFileName = "minutes-enrichment-diagnostics.txt"

	feishuPhaseDriveReady        = "drive_upload_ready"
	feishuPhaseDriveIntent       = "drive_upload_intent"
	feishuPhaseDriveUploaded     = "drive_uploaded"
	feishuPhaseMinutesReady      = "minutes_upload_ready"
	feishuPhaseMinutesIntent     = "minutes_upload_intent"
	feishuPhaseMinutesCreated    = "minutes_created"
	feishuPhaseMinutesEnrichment = "minutes_enrichment_waiting"
	feishuPhaseTranscriptStored  = "transcript_stored"
)

var larkTokenPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{4,512}$`)

var errWhiteboardPreviewInvalid = errors.New("whiteboard preview validation failed")

type ReadyAudioLookup func(
	context.Context,
	uint,
) (path string, sha256 string, err error)

type FeishuMinutesAdapter struct {
	runner     larkCommandRunner
	workRoot   string
	readyAudio ReadyAudioLookup
	now        func() time.Time
}

type feishuCheckpoint struct {
	Version                   int               `json:"version"`
	Phase                     string            `json:"phase"`
	AudioDigest               string            `json:"audio_digest"`
	FileToken                 string            `json:"file_token,omitempty"`
	MinuteToken               string            `json:"minute_token,omitempty"`
	MinuteURL                 string            `json:"minute_url,omitempty"`
	TranscriptRelativePath    string            `json:"transcript_relative_path,omitempty"`
	DetailRelativePath        string            `json:"detail_relative_path,omitempty"`
	EnrichmentPendingAttempts int               `json:"enrichment_pending_attempts,omitempty"`
	CoreReadyAt               string            `json:"core_ready_at,omitempty"`
	EnrichmentDeadlineAt      string            `json:"enrichment_deadline_at,omitempty"`
	EnrichmentRelativePath    string            `json:"enrichment_relative_path,omitempty"`
	EnrichmentSHA256          string            `json:"enrichment_sha256,omitempty"`
	WhiteboardRelativePath    string            `json:"whiteboard_relative_path,omitempty"`
	WhiteboardSHA256          string            `json:"whiteboard_sha256,omitempty"`
	EnrichmentRawSHA256       map[string]string `json:"enrichment_raw_sha256,omitempty"`
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
	case feishuPhaseMinutesEnrichment:
		return a.waitForMinutesEnrichment(ctx, request, checkpoint, nil, nil)
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
		feishuPhaseMinutesEnrichment,
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
		var adapterErr *AdapterError
		if errors.As(err, &adapterErr) {
			return progressWithCheckpoint(checkpoint), err
		}
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
		// Real Minutes uploads can expose Transcript, chapters, and keywords
		// before Summary is populated. Keep the durable Minute checkpoint and
		// let the bounded external-artifact wait poll again.
		return progressWithCheckpoint(checkpoint), nil
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
	checkpoint.TranscriptRelativePath = filepath.ToSlash(transcriptRelative)
	checkpoint.DetailRelativePath = filepath.ToSlash(detailRelative)
	progress, completeErr := completedFeishuProgress(
		checkpoint,
		request.PipelineVersion,
		detail.Summary,
		transcript,
		output,
	)
	if completeErr != nil {
		checkpoint.Phase = feishuPhaseTranscriptStored
		if persistErr := persistFeishuCheckpoint(ctx, request, checkpoint); persistErr != nil {
			return progressWithCheckpoint(checkpoint), persistErr
		}
		progress.Checkpoint, _ = encodeFeishuCheckpoint(checkpoint)
		return progress, completeErr
	}
	if request.PipelineVersion != NativeMinutesPipelineVersion {
		checkpoint.Phase = feishuPhaseTranscriptStored
		if err := persistFeishuCheckpoint(ctx, request, checkpoint); err != nil {
			return progressWithCheckpoint(checkpoint), err
		}
		progress.Checkpoint, _ = encodeFeishuCheckpoint(checkpoint)
		return progress, nil
	}
	checkpoint = a.ensureEnrichmentWindow(checkpoint)
	checkpoint.Phase = feishuPhaseMinutesEnrichment
	if err := persistFeishuCheckpoint(ctx, request, checkpoint); err != nil {
		return progressWithCheckpoint(checkpoint), err
	}
	return a.waitForMinutesEnrichment(ctx, request, checkpoint, &detail, output)
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
	var parsedDetail minuteDetail
	if pipelineVersion == NativeMinutesPipelineVersion {
		var found bool
		var parseErr error
		parsedDetail, found, parseErr = parseMinuteDetail(
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
	progress, err := completedFeishuProgress(
		checkpoint,
		pipelineVersion,
		summary,
		transcript,
		detail,
	)
	if err != nil {
		return progress, err
	}
	if pipelineVersion == NativeMinutesPipelineVersion {
		progress, err = a.restoreStoredEnrichment(
			ctx,
			checkpoint,
			parsedDetail,
			progress,
		)
		if err != nil {
			return progressWithCheckpoint(checkpoint), err
		}
	}
	return progress, nil
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
		return progressWithCheckpoint(checkpoint), err
	}
	normalizedTranscript, segments, err := normalizeTranscript(string(transcript))
	if err != nil {
		return progressWithCheckpoint(checkpoint), err
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

func (a *FeishuMinutesAdapter) nowUTC() time.Time {
	if a != nil && a.now != nil {
		return a.now().UTC()
	}
	return time.Now().UTC()
}

func (a *FeishuMinutesAdapter) ensureEnrichmentWindow(checkpoint feishuCheckpoint) feishuCheckpoint {
	now := a.nowUTC()
	coreReady, err := parseCheckpointTime(checkpoint.CoreReadyAt)
	if err != nil || coreReady.IsZero() {
		coreReady = now
		checkpoint.CoreReadyAt = formatCheckpointTime(coreReady)
	}
	deadline, err := parseCheckpointTime(checkpoint.EnrichmentDeadlineAt)
	if err != nil || deadline.IsZero() {
		checkpoint.EnrichmentDeadlineAt = formatCheckpointTime(
			minutesEnrichmentDeadline(coreReady, now),
		)
	}
	return checkpoint
}

func enrichmentWaitingProgress(checkpoint feishuCheckpoint) TranscriptionProgress {
	progress := progressWithCheckpoint(checkpoint)
	progress.CurrentStep = StepMinutesEnrichment
	return progress
}

func (a *FeishuMinutesAdapter) waitForMinutesEnrichment(
	ctx context.Context,
	request TranscriptionRequest,
	checkpoint feishuCheckpoint,
	detail *minuteDetail,
	detailRaw []byte,
) (TranscriptionProgress, error) {
	checkpoint = a.ensureEnrichmentWindow(checkpoint)
	checkpoint.Phase = feishuPhaseMinutesEnrichment
	now := a.nowUTC()
	deadline, deadlineErr := parseCheckpointTime(checkpoint.EnrichmentDeadlineAt)
	if deadlineErr != nil {
		return enrichmentWaitingProgress(checkpoint), NewAdapterError(
			"invalid_external_checkpoint",
			"Feishu processing checkpoint is invalid",
			false,
		)
	}
	expired := enrichmentWaitExpired(deadline, now)
	probeCtx := ctx
	var cancelProbe context.CancelFunc
	if expired {
		probeCtx, cancelProbe = context.WithTimeout(
			ctx,
			minutesEnrichmentExpiredProbeTimeout,
		)
		defer cancelProbe()
	}

	if detail == nil {
		fetched, raw, wait, fetchErr := a.fetchMinuteDetailForEnrichment(probeCtx, request, checkpoint)
		if fetchErr != nil {
			if minutesErrorIsWaitable(fetchErr) {
				if expired {
					return a.failEnrichmentWait(ctx, request, checkpoint, minutesEnrichmentTimeoutDecision())
				}
				if err := persistFeishuCheckpoint(ctx, request, checkpoint); err != nil {
					return enrichmentWaitingProgress(checkpoint), err
				}
				return enrichmentWaitingProgress(checkpoint), nil
			}
			decision := minutesEnrichmentDecision{
				Code:       minutesEnrichmentNoteUnreadableCode,
				Message:    minutesEnrichmentNoteUnreadableMsg,
				Diagnostic: "note_detail_unavailable",
			}
			var adapterErr *AdapterError
			if errors.As(fetchErr, &adapterErr) && !adapterErr.CanRetry {
				switch adapterErr.ErrorCode {
				case minutesEnrichmentSectionCode, "invalid_external_checkpoint",
					"lark_auth_expired", "lark_permission_denied":
					decision.Code = adapterErr.ErrorCode
					decision.Message = adapterErr.SafeMessage
					decision.Retryable = adapterErr.CanRetry
					if isMinutesEnrichmentCredentialError(adapterErr.ErrorCode) {
						decision.Diagnostic = "note_permission_unavailable"
					}
				}
			}
			if expired && !isMinutesEnrichmentCredentialError(decision.Code) {
				return a.failEnrichmentWait(ctx, request, checkpoint, minutesEnrichmentTimeoutDecision())
			}
			return a.failEnrichmentWait(ctx, request, checkpoint, decision)
		}
		if wait {
			if expired {
				return a.failEnrichmentWait(ctx, request, checkpoint, minutesEnrichmentTimeoutDecision())
			}
			if err := persistFeishuCheckpoint(ctx, request, checkpoint); err != nil {
				return enrichmentWaitingProgress(checkpoint), err
			}
			return enrichmentWaitingProgress(checkpoint), nil
		}
		detail = &fetched
		detailRaw = raw
		checkpoint, fetchErr = a.persistRefreshedCoreSnapshot(
			ctx,
			request.RunID,
			checkpoint,
			fetched,
			raw,
		)
		if fetchErr != nil {
			return enrichmentWaitingProgress(checkpoint), NewAdapterError(
				minutesEnrichmentSnapshotWriteCode,
				"Feishu core snapshot could not be stored",
				true,
			)
		}
	}

	progress, completeErr := a.completedProgressFromStoredCore(
		ctx,
		request.PipelineVersion,
		checkpoint,
		*detail,
		detailRaw,
	)
	if completeErr != nil {
		return progress, completeErr
	}
	progress, decision := a.captureNoteEnrichment(
		probeCtx,
		request.RunID,
		*detail,
		progress,
	)
	if decision.Complete {
		var persistErr error
		checkpoint, persistErr = a.persistCompletedEnrichment(
			request.RunID,
			checkpoint,
			progress,
		)
		if persistErr != nil {
			return enrichmentWaitingProgress(checkpoint), NewAdapterError(
				minutesEnrichmentSnapshotWriteCode,
				"Feishu intelligent minutes snapshot could not be stored",
				true,
			)
		}
		return a.finishCompletedEnrichment(ctx, request, checkpoint, progress)
	}
	if decision.Wait && !expired {
		if err := persistFeishuCheckpoint(ctx, request, checkpoint); err != nil {
			return enrichmentWaitingProgress(checkpoint), err
		}
		waiting := enrichmentWaitingProgress(checkpoint)
		waiting.RawArtifacts = progress.RawArtifacts
		return waiting, nil
	}
	if decision.Wait && expired {
		decision = minutesEnrichmentTimeoutDecision()
	}
	return a.failEnrichmentWait(ctx, request, checkpoint, decision)
}

func (a *FeishuMinutesAdapter) persistRefreshedCoreSnapshot(
	ctx context.Context,
	runID uint,
	checkpoint feishuCheckpoint,
	detail minuteDetail,
	detailRaw []byte,
) (feishuCheckpoint, error) {
	runDirectory, err := a.runDirectory(runID)
	if err != nil {
		return checkpoint, err
	}
	var transcript []byte
	if detail.TranscriptFilePresent && strings.TrimSpace(detail.TranscriptFile) != "" {
		_, transcript, err = readManagedTranscript(runDirectory, detail.TranscriptFile)
	}
	if err != nil || len(bytes.TrimSpace(transcript)) == 0 {
		transcript, err = a.readStoredFile(
			ctx,
			checkpoint.TranscriptRelativePath,
			maxTranscriptBytes,
		)
	}
	if err != nil || len(bytes.TrimSpace(transcript)) == 0 {
		return checkpoint, ErrInvalidArtifact
	}
	if len(bytes.TrimSpace(detailRaw)) == 0 {
		return checkpoint, ErrInvalidArtifact
	}
	if _, err := writeArtifactFile(
		runDirectory,
		"minutes-transcript.txt",
		transcript,
	); err != nil {
		return checkpoint, err
	}
	if _, err := writeReadableArtifactFile(
		runDirectory,
		"minutes-detail.json",
		detailRaw,
	); err != nil {
		return checkpoint, err
	}
	transcriptRelative, err := filepath.Rel(
		a.workRoot,
		filepath.Join(runDirectory, "minutes-transcript.txt"),
	)
	if err != nil || pathEscapesRoot(transcriptRelative) {
		return checkpoint, ErrInvalidArtifact
	}
	detailRelative, err := filepath.Rel(
		a.workRoot,
		filepath.Join(runDirectory, "minutes-detail.json"),
	)
	if err != nil || pathEscapesRoot(detailRelative) {
		return checkpoint, ErrInvalidArtifact
	}
	checkpoint.TranscriptRelativePath = filepath.ToSlash(transcriptRelative)
	checkpoint.DetailRelativePath = filepath.ToSlash(detailRelative)
	return checkpoint, nil
}

func (a *FeishuMinutesAdapter) fetchMinuteDetailForEnrichment(
	ctx context.Context,
	request TranscriptionRequest,
	checkpoint feishuCheckpoint,
) (minuteDetail, []byte, bool, error) {
	if !larkTokenPattern.MatchString(checkpoint.MinuteToken) {
		return minuteDetail{}, nil, false, NewAdapterError(
			"invalid_external_checkpoint",
			"Feishu Minutes identity is invalid",
			false,
		)
	}
	runDirectory, err := a.runDirectory(request.RunID)
	if err != nil {
		return minuteDetail{}, nil, false, err
	}
	detailDirectory := filepath.Join(runDirectory, "detail")
	if err := os.MkdirAll(detailDirectory, 0o700); err != nil {
		return minuteDetail{}, nil, false, NewAdapterError(
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
		var larkErr *larkCommandError
		if errors.As(commandErr, &larkErr) {
			if parsed, found, parseErr := parseMinuteDetail(
				larkErr.stdout,
				checkpoint.MinuteToken,
			); parseErr == nil && found && parsed.Pending &&
				strings.TrimSpace(parsed.TranscriptFile) == "" {
				return minuteDetail{}, nil, true, nil
			}
		}
		mapped, _ := classifyLarkCommandError(commandErr)
		if errors.Is(mapped, errLarkMinutesPending) {
			return minuteDetail{}, nil, true, nil
		}
		return minuteDetail{}, nil, false, mapped
	}
	parsed, found, err := parseMinuteDetail(output, checkpoint.MinuteToken)
	if err != nil {
		var adapterErr *AdapterError
		if errors.As(err, &adapterErr) {
			return minuteDetail{}, nil, false, err
		}
		return minuteDetail{}, nil, false, NewAdapterError(
			"lark_protocol_error",
			"Feishu Minutes detail output is invalid",
			false,
		)
	}
	if !found || parsed.Pending {
		return minuteDetail{}, nil, true, nil
	}
	return parsed, output, false, nil
}

func (a *FeishuMinutesAdapter) completedProgressFromStoredCore(
	ctx context.Context,
	pipelineVersion string,
	checkpoint feishuCheckpoint,
	detail minuteDetail,
	detailRaw []byte,
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
	if len(bytes.TrimSpace(detailRaw)) == 0 {
		detailRaw, err = a.readStoredFile(
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
	}
	summary := detail.Summary
	if strings.TrimSpace(summary) == "" {
		storedDetail, found, parseErr := parseMinuteDetail(detailRaw, checkpoint.MinuteToken)
		if parseErr == nil && found {
			summary = storedDetail.Summary
		}
	}
	return completedFeishuProgress(
		checkpoint,
		pipelineVersion,
		summary,
		transcript,
		detailRaw,
	)
}

func (a *FeishuMinutesAdapter) finishCompletedEnrichment(
	ctx context.Context,
	request TranscriptionRequest,
	checkpoint feishuCheckpoint,
	progress TranscriptionProgress,
) (TranscriptionProgress, error) {
	checkpoint.Phase = feishuPhaseTranscriptStored
	if err := persistFeishuCheckpoint(ctx, request, checkpoint); err != nil {
		return enrichmentWaitingProgress(checkpoint), err
	}
	progress.Status = ExternalProgressCompleted
	progress.CurrentStep = ""
	progress.Checkpoint, _ = encodeFeishuCheckpoint(checkpoint)
	return progress, nil
}

func (a *FeishuMinutesAdapter) failEnrichmentWait(
	ctx context.Context,
	request TranscriptionRequest,
	checkpoint feishuCheckpoint,
	decision minutesEnrichmentDecision,
) (TranscriptionProgress, error) {
	waiting := enrichmentWaitingProgress(checkpoint)
	if decision.Diagnostic != "" {
		waiting = addMinutesEnrichmentDiagnostic(waiting, decision.Diagnostic)
		if err := a.persistEnrichmentDiagnostics(request.RunID, waiting); err != nil {
			return waiting, NewAdapterError(
				minutesEnrichmentSnapshotWriteCode,
				"Feishu intelligent minutes diagnostics could not be stored",
				true,
			)
		}
	}
	if err := persistFeishuCheckpoint(ctx, request, checkpoint); err != nil {
		return waiting, err
	}
	code := strings.TrimSpace(decision.Code)
	if code == "" {
		code = minutesEnrichmentNoteUnreadableCode
	}
	message := strings.TrimSpace(decision.Message)
	if message == "" {
		message = minutesEnrichmentNoteUnreadableMsg
	}
	return waiting, NewAdapterError(
		code,
		message,
		decision.Retryable,
	)
}

func (a *FeishuMinutesAdapter) restoreStoredEnrichment(
	ctx context.Context,
	checkpoint feishuCheckpoint,
	detail minuteDetail,
	progress TranscriptionProgress,
) (TranscriptionProgress, error) {
	if err := ctx.Err(); err != nil {
		progress.MinutesEnrichment = MinutesEnrichment{
			Chapters: append([]MinutesChapter(nil), detail.Chapters...),
			Keywords: append([]string(nil), detail.Keywords...),
		}.Public()
		return progress, err
	}
	if checkpoint.EnrichmentRelativePath != "" {
		return a.restoreCompletedEnrichmentSnapshot(ctx, checkpoint, progress)
	}
	enrichment := MinutesEnrichment{
		Chapters: append([]MinutesChapter(nil), detail.Chapters...),
		Keywords: append([]string(nil), detail.Keywords...),
	}
	if relative := enrichmentRelativePath(checkpoint.DetailRelativePath); relative != "" {
		if raw, err := a.readStoredFile(ctx, relative, maxArtifactTextBytes); err == nil {
			if decoded, decodeErr := decodeMinutesEnrichment(raw); decodeErr == nil {
				if len(decoded.Chapters) > 0 {
					enrichment.Chapters = decoded.Chapters
				}
				if len(decoded.Keywords) > 0 {
					enrichment.Keywords = decoded.Keywords
				}
				enrichment.Decisions = decoded.Decisions
				enrichment.Quotes = decoded.Quotes
				enrichment.Links = decoded.Links
				enrichment.Whiteboard = decoded.Whiteboard
			}
		}
	}
	if enrichment.Whiteboard != nil {
		if preview, err := a.readStoredBinaryFile(
			ctx,
			whiteboardRelativePath(checkpoint.DetailRelativePath),
			maxWhiteboardPreviewBytes,
		); err == nil {
			if sniffed, sniffErr := sniffManagedImage(preview); sniffErr == nil &&
				digestBytes(sniffed.Bytes) == enrichment.Whiteboard.SHA256 {
				progress.WhiteboardPreview = sniffed.Bytes
				enrichment.Whiteboard.MediaType = sniffed.MediaType
				enrichment.Whiteboard.Width = sniffed.Width
				enrichment.Whiteboard.Height = sniffed.Height
			} else {
				enrichment.Whiteboard = nil
			}
		} else {
			enrichment.Whiteboard = nil
		}
	}
	progress.MinutesEnrichment = enrichment.Public()
	return a.restoreStoredRawEnrichment(ctx, checkpoint, progress), nil
}

func (a *FeishuMinutesAdapter) captureNoteEnrichment(
	ctx context.Context,
	runID uint,
	detail minuteDetail,
	progress TranscriptionProgress,
) (TranscriptionProgress, minutesEnrichmentDecision) {
	enrichment := MinutesEnrichment{
		Chapters: append([]MinutesChapter(nil), detail.Chapters...),
		Keywords: append([]string(nil), detail.Keywords...),
	}
	noteID := strings.TrimSpace(detail.NoteID)
	if noteID == "" || !larkTokenPattern.MatchString(noteID) {
		progress.MinutesEnrichment = enrichment.Public()
		return progress, minutesEnrichmentDecision{Wait: true, Diagnostic: "note_id_pending"}
	}
	runDirectory, err := a.runDirectory(runID)
	if err != nil {
		progress.MinutesEnrichment = enrichment.Public()
		progress = addMinutesEnrichmentDiagnostic(progress, "managed_directory_unavailable")
		return progress, minutesEnrichmentDecision{
			Code:       minutesEnrichmentNoteUnreadableCode,
			Message:    minutesEnrichmentNoteUnreadableMsg,
			Diagnostic: "managed_directory_unavailable",
		}
	}
	noteOutput, noteErr := a.runner.Run(
		ctx,
		runDirectory,
		"note",
		"+detail",
		"--note-id",
		noteID,
		"--as",
		"user",
		"--format",
		"json",
	)
	if noteErr != nil {
		progress.MinutesEnrichment = enrichment.Public()
		if minutesErrorIsWaitable(noteErr) {
			return progress, minutesEnrichmentDecision{Wait: true, Diagnostic: "note_detail_pending"}
		}
		mapped, _ := classifyLarkCommandError(noteErr)
		diagnostic := "note_detail_unavailable"
		decision := minutesEnrichmentDecision{
			Code:       minutesEnrichmentNoteUnreadableCode,
			Message:    minutesEnrichmentNoteUnreadableMsg,
			Diagnostic: diagnostic,
		}
		var adapterErr *AdapterError
		if errors.As(mapped, &adapterErr) {
			switch adapterErr.ErrorCode {
			case "lark_permission_denied", "lark_auth_expired":
				diagnostic = "note_permission_unavailable"
				decision.Code = adapterErr.ErrorCode
				decision.Message = adapterErr.SafeMessage
				decision.Retryable = adapterErr.CanRetry
			}
		}
		progress = addMinutesEnrichmentDiagnostic(progress, diagnostic)
		decision.Diagnostic = diagnostic
		return progress, decision
	}
	progress.RawArtifacts = cloneByteMap(progress.RawArtifacts)
	progress.RawArtifacts["note-detail.json"] = append([]byte(nil), noteOutput...)
	noteDocToken, ok := parseNoteDocToken(noteOutput)
	if !ok {
		progress.MinutesEnrichment = enrichment.Public()
		return progress, minutesEnrichmentDecision{Wait: true, Diagnostic: "note_doc_pending"}
	}
	docOutput, docErr := a.runner.Run(
		ctx,
		runDirectory,
		"docs",
		"+fetch",
		"--doc",
		noteDocToken,
		"--doc-format",
		"xml",
		"--detail",
		"simple",
		"--as",
		"user",
		"--format",
		"json",
	)
	if docErr != nil {
		progress.MinutesEnrichment = enrichment.Public()
		if minutesErrorIsWaitable(docErr) {
			return progress, minutesEnrichmentDecision{Wait: true, Diagnostic: "note_document_pending"}
		}
		decision := minutesEnrichmentDecision{
			Code:       minutesEnrichmentNoteUnreadableCode,
			Message:    minutesEnrichmentNoteUnreadableMsg,
			Diagnostic: "note_document_unavailable",
		}
		mapped, _ := classifyLarkCommandError(docErr)
		var adapterErr *AdapterError
		if errors.As(mapped, &adapterErr) && !adapterErr.CanRetry {
			switch adapterErr.ErrorCode {
			case "lark_auth_expired", "lark_permission_denied":
				decision.Code = adapterErr.ErrorCode
				decision.Message = adapterErr.SafeMessage
				decision.Retryable = adapterErr.CanRetry
				decision.Diagnostic = "note_permission_unavailable"
			}
		}
		progress = addMinutesEnrichmentDiagnostic(progress, decision.Diagnostic)
		return progress, decision
	}
	progress.RawArtifacts["note-document.json"] = append([]byte(nil), docOutput...)
	content, ok := parseDocsFetchContent(docOutput)
	if !ok {
		progress.MinutesEnrichment = enrichment.Public()
		return progress, minutesEnrichmentDecision{Wait: true, Diagnostic: "note_document_pending"}
	}
	decisions, quotes, links, whiteboardToken := parseNoteSections(content)
	enrichment.Decisions = decisions
	enrichment.Quotes = quotes
	enrichment.Links = links
	whiteboardCaptured := false
	whiteboardWaitable := false
	if whiteboardToken != "" {
		preview, captureErr := a.captureWhiteboardPreview(
			ctx,
			runDirectory,
			whiteboardToken,
		)
		if captureErr == nil {
			progress.WhiteboardPreview = preview.Bytes
			enrichment.Whiteboard = &MinutesWhiteboard{
				MediaID:   minutesWhiteboardMediaID,
				MediaType: preview.MediaType,
				Width:     preview.Width,
				Height:    preview.Height,
				SHA256:    digestBytes(preview.Bytes),
				Alt:       minutesWhiteboardAlt,
			}
			whiteboardCaptured = true
		} else {
			whiteboardWaitable = !errors.Is(captureErr, errWhiteboardPreviewInvalid) &&
				minutesErrorIsWaitable(captureErr)
			progress = addMinutesEnrichmentDiagnostic(progress, "whiteboard_unavailable")
		}
	}
	decision := evaluateReadableNoteDocument(
		content,
		whiteboardToken,
		whiteboardCaptured,
		whiteboardWaitable,
	)
	if decision.Diagnostic != "" {
		progress = addMinutesEnrichmentDiagnostic(progress, decision.Diagnostic)
	}
	if progress.SkillVersions == nil {
		progress.SkillVersions = map[string]string{}
	}
	progress.SkillVersions["lark-note"] = "1.0.0"
	progress.SkillVersions["lark-docs"] = "1.0.0"
	if enrichment.Whiteboard != nil {
		progress.SkillVersions["lark-whiteboard"] = "1.0.0"
	}
	progress.MinutesEnrichment = enrichment.Public()
	return progress, decision
}

func (a *FeishuMinutesAdapter) captureWhiteboardPreview(
	ctx context.Context,
	runDirectory string,
	whiteboardToken string,
) (ManagedImage, error) {
	outputName := "whiteboard-preview"
	_, commandErr := a.runner.Run(
		ctx,
		runDirectory,
		"whiteboard",
		"+query",
		"--whiteboard-token",
		whiteboardToken,
		"--output_as",
		"image",
		"--output",
		"./"+outputName,
		"--overwrite",
		"--as",
		"user",
	)
	if commandErr != nil {
		return ManagedImage{}, commandErr
	}
	path := filepath.Join(runDirectory, outputName)
	content, err := readBoundedRegularFile(path, maxWhiteboardPreviewBytes)
	if err != nil {
		matches, matchErr := filepath.Glob(filepath.Join(runDirectory, outputName+".*"))
		if matchErr != nil || len(matches) == 0 {
			return ManagedImage{}, fmt.Errorf("%w: %v", errWhiteboardPreviewInvalid, err)
		}
		content, err = readBoundedRegularFile(matches[0], maxWhiteboardPreviewBytes)
		if err != nil {
			return ManagedImage{}, fmt.Errorf("%w: %v", errWhiteboardPreviewInvalid, err)
		}
	}
	preview, err := sniffManagedImage(content)
	if err != nil {
		return ManagedImage{}, fmt.Errorf("%w: %v", errWhiteboardPreviewInvalid, err)
	}
	return preview, nil
}

func (a *FeishuMinutesAdapter) persistCompletedEnrichment(
	runID uint,
	checkpoint feishuCheckpoint,
	progress TranscriptionProgress,
) (feishuCheckpoint, error) {
	runDirectory, err := a.runDirectory(runID)
	if err != nil {
		return checkpoint, err
	}
	encoded, err := json.MarshalIndent(progress.MinutesEnrichment.Public(), "", "  ")
	if err != nil {
		return checkpoint, fmt.Errorf("encode completed minutes enrichment: %w", err)
	}
	encoded = append(encoded, '\n')
	enrichmentHash, err := writeReadableArtifactFile(
		runDirectory,
		minutesEnrichmentFileName,
		encoded,
	)
	if err != nil {
		return checkpoint, err
	}
	enrichmentRelative, err := filepath.Rel(
		a.workRoot,
		filepath.Join(runDirectory, minutesEnrichmentFileName),
	)
	if err != nil || pathEscapesRoot(enrichmentRelative) {
		return checkpoint, ErrInvalidArtifact
	}
	checkpoint.EnrichmentRelativePath = filepath.ToSlash(enrichmentRelative)
	checkpoint.EnrichmentSHA256 = enrichmentHash
	checkpoint.WhiteboardRelativePath = ""
	checkpoint.WhiteboardSHA256 = ""

	whiteboard := progress.MinutesEnrichment.Public().Whiteboard
	if whiteboard != nil {
		preview, sniffErr := sniffManagedImage(progress.WhiteboardPreview)
		if sniffErr != nil ||
			digestBytes(preview.Bytes) != whiteboard.SHA256 ||
			preview.MediaType != whiteboard.MediaType {
			return checkpoint, ErrInvalidArtifact
		}
		whiteboardHash, writeErr := writeArtifactFile(
			runDirectory,
			"whiteboard-preview",
			preview.Bytes,
		)
		if writeErr != nil {
			return checkpoint, writeErr
		}
		whiteboardRelative, relativeErr := filepath.Rel(
			a.workRoot,
			filepath.Join(runDirectory, "whiteboard-preview"),
		)
		if relativeErr != nil || pathEscapesRoot(whiteboardRelative) {
			return checkpoint, ErrInvalidArtifact
		}
		checkpoint.WhiteboardRelativePath = filepath.ToSlash(whiteboardRelative)
		checkpoint.WhiteboardSHA256 = whiteboardHash
	} else if len(progress.WhiteboardPreview) > 0 {
		return checkpoint, ErrInvalidArtifact
	}

	checkpoint.EnrichmentRawSHA256 = make(map[string]string, 3)
	for _, name := range []string{
		"note-detail.json",
		"note-document.json",
		minutesEnrichmentDiagnosticFileName,
	} {
		body := progress.RawArtifacts[name]
		if len(body) == 0 {
			if name == "note-detail.json" || name == "note-document.json" {
				return checkpoint, fmt.Errorf("completed minutes enrichment is missing %s", name)
			}
			continue
		}
		hash, writeErr := writeArtifactFile(runDirectory, name, body)
		if writeErr != nil {
			return checkpoint, writeErr
		}
		checkpoint.EnrichmentRawSHA256[name] = hash
	}
	return checkpoint, nil
}

func (a *FeishuMinutesAdapter) persistEnrichmentDiagnostics(
	runID uint,
	progress TranscriptionProgress,
) error {
	body := progress.RawArtifacts[minutesEnrichmentDiagnosticFileName]
	if len(body) == 0 {
		return nil
	}
	runDirectory, err := a.runDirectory(runID)
	if err != nil {
		return err
	}
	_, err = writeReadableArtifactFile(
		runDirectory,
		minutesEnrichmentDiagnosticFileName,
		body,
	)
	return err
}

func (a *FeishuMinutesAdapter) restoreCompletedEnrichmentSnapshot(
	ctx context.Context,
	checkpoint feishuCheckpoint,
	progress TranscriptionProgress,
) (TranscriptionProgress, error) {
	if !sha256Pattern.MatchString(checkpoint.EnrichmentSHA256) {
		return progress, NewAdapterError(
			minutesEnrichmentSnapshotStoredCode,
			"stored Feishu intelligent minutes are unavailable",
			true,
		)
	}
	raw, err := a.readStoredFile(
		ctx,
		checkpoint.EnrichmentRelativePath,
		maxArtifactTextBytes,
	)
	if err != nil || digestBytes(raw) != checkpoint.EnrichmentSHA256 {
		return progress, NewAdapterError(
			minutesEnrichmentSnapshotStoredCode,
			"stored Feishu intelligent minutes are unavailable",
			true,
		)
	}
	enrichment, err := decodeMinutesEnrichment(raw)
	if err != nil {
		return progress, NewAdapterError(
			minutesEnrichmentSnapshotStoredCode,
			"stored Feishu intelligent minutes are unavailable",
			true,
		)
	}
	progress.MinutesEnrichment = enrichment.Public()
	if enrichment.Whiteboard != nil {
		if !sha256Pattern.MatchString(checkpoint.WhiteboardSHA256) ||
			checkpoint.WhiteboardSHA256 != enrichment.Whiteboard.SHA256 {
			return progress, NewAdapterError(
				minutesEnrichmentSnapshotStoredCode,
				"stored Feishu intelligent minutes are unavailable",
				true,
			)
		}
		preview, readErr := a.readStoredBinaryFile(
			ctx,
			checkpoint.WhiteboardRelativePath,
			maxWhiteboardPreviewBytes,
		)
		if readErr != nil || digestBytes(preview) != checkpoint.WhiteboardSHA256 {
			return progress, NewAdapterError(
				minutesEnrichmentSnapshotStoredCode,
				"stored Feishu intelligent minutes are unavailable",
				true,
			)
		}
		sniffed, sniffErr := sniffManagedImage(preview)
		if sniffErr != nil || sniffed.MediaType != enrichment.Whiteboard.MediaType {
			return progress, NewAdapterError(
				minutesEnrichmentSnapshotStoredCode,
				"stored Feishu intelligent minutes are unavailable",
				true,
			)
		}
		progress.WhiteboardPreview = sniffed.Bytes
	} else if checkpoint.WhiteboardRelativePath != "" || checkpoint.WhiteboardSHA256 != "" {
		return progress, NewAdapterError(
			minutesEnrichmentSnapshotStoredCode,
			"stored Feishu intelligent minutes are unavailable",
			true,
		)
	}

	progress.RawArtifacts = cloneByteMap(progress.RawArtifacts)
	for name, expectedHash := range checkpoint.EnrichmentRawSHA256 {
		relative := siblingRelativePath(checkpoint.EnrichmentRelativePath, name)
		stored, readErr := a.readStoredFile(ctx, relative, maxLarkCLIOutputBytes)
		if readErr != nil || digestBytes(stored) != expectedHash {
			return progress, NewAdapterError(
				minutesEnrichmentSnapshotStoredCode,
				"stored Feishu intelligent minutes are unavailable",
				true,
			)
		}
		progress.RawArtifacts[name] = stored
	}
	if _, ok := progress.RawArtifacts["note-detail.json"]; !ok {
		return progress, NewAdapterError(
			minutesEnrichmentSnapshotStoredCode,
			"stored Feishu intelligent minutes are unavailable",
			true,
		)
	}
	if _, ok := progress.RawArtifacts["note-document.json"]; !ok {
		return progress, NewAdapterError(
			minutesEnrichmentSnapshotStoredCode,
			"stored Feishu intelligent minutes are unavailable",
			true,
		)
	}
	if progress.SkillVersions == nil {
		progress.SkillVersions = map[string]string{}
	}
	progress.SkillVersions["lark-note"] = "1.0.0"
	progress.SkillVersions["lark-docs"] = "1.0.0"
	if enrichment.Whiteboard != nil {
		progress.SkillVersions["lark-whiteboard"] = "1.0.0"
	}
	return progress, nil
}

func (a *FeishuMinutesAdapter) restoreStoredRawEnrichment(
	ctx context.Context,
	checkpoint feishuCheckpoint,
	progress TranscriptionProgress,
) TranscriptionProgress {
	progress.RawArtifacts = cloneByteMap(progress.RawArtifacts)
	if progress.SkillVersions == nil {
		progress.SkillVersions = map[string]string{}
	}
	for _, name := range []string{
		"note-detail.json",
		"note-document.json",
		minutesEnrichmentDiagnosticFileName,
	} {
		relative := siblingRelativePath(checkpoint.DetailRelativePath, name)
		if relative == "" {
			continue
		}
		raw, err := a.readStoredFile(ctx, relative, maxLarkCLIOutputBytes)
		if err != nil {
			continue
		}
		progress.RawArtifacts[name] = raw
	}
	if _, ok := progress.RawArtifacts["note-detail.json"]; ok {
		progress.SkillVersions["lark-note"] = "1.0.0"
		progress.SkillVersions["lark-docs"] = "1.0.0"
	}
	if progress.MinutesEnrichment.Whiteboard != nil || len(progress.WhiteboardPreview) > 0 {
		progress.SkillVersions["lark-whiteboard"] = "1.0.0"
	}
	return progress
}

func addMinutesEnrichmentDiagnostic(
	progress TranscriptionProgress,
	code string,
) TranscriptionProgress {
	code = strings.TrimSpace(code)
	if code == "" {
		return progress
	}
	progress.RawArtifacts = cloneByteMap(progress.RawArtifacts)
	codes := strings.Fields(string(progress.RawArtifacts[minutesEnrichmentDiagnosticFileName]))
	for _, existing := range codes {
		if existing == code {
			return progress
		}
	}
	codes = append(codes, code)
	progress.RawArtifacts[minutesEnrichmentDiagnosticFileName] = []byte(
		strings.Join(codes, "\n") + "\n",
	)
	return progress
}

func (a *FeishuMinutesAdapter) readStoredBinaryFile(
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
	return readBoundedRegularFile(filepath.Join(a.workRoot, cleanRelative), limit)
}

func parseNoteDocToken(output []byte) (string, bool) {
	var decoded struct {
		NoteDocToken string `json:"note_doc_token"`
		Data         struct {
			NoteDocToken string `json:"note_doc_token"`
			Note         struct {
				NoteDocToken string `json:"note_doc_token"`
			} `json:"note"`
		} `json:"data"`
		Result struct {
			NoteDocToken string `json:"note_doc_token"`
			Note         struct {
				NoteDocToken string `json:"note_doc_token"`
			} `json:"note"`
		} `json:"result"`
		Note struct {
			NoteDocToken string `json:"note_doc_token"`
		} `json:"note"`
	}
	if err := strictJSONDecode(output, &decoded); err != nil {
		return "", false
	}
	token := extractJSONString(
		decoded.NoteDocToken,
		decoded.Data.NoteDocToken,
		decoded.Data.Note.NoteDocToken,
		decoded.Result.NoteDocToken,
		decoded.Result.Note.NoteDocToken,
		decoded.Note.NoteDocToken,
	)
	if !larkTokenPattern.MatchString(token) {
		return "", false
	}
	return token, true
}

func parseDocsFetchContent(output []byte) (string, bool) {
	var decoded struct {
		Data struct {
			Document struct {
				Content string `json:"content"`
			} `json:"document"`
			Content string `json:"content"`
		} `json:"data"`
		Result struct {
			Document struct {
				Content string `json:"content"`
			} `json:"document"`
			Content string `json:"content"`
		} `json:"result"`
		Content string `json:"content"`
	}
	if err := strictJSONDecode(output, &decoded); err != nil {
		return "", false
	}
	content := extractJSONString(
		decoded.Data.Document.Content,
		decoded.Data.Content,
		decoded.Result.Document.Content,
		decoded.Result.Content,
		decoded.Content,
	)
	if strings.TrimSpace(content) == "" {
		return "", false
	}
	return content, true
}

func siblingRelativePath(detailRelative, name string) string {
	detailRelative = strings.TrimSpace(detailRelative)
	name = strings.TrimSpace(name)
	if detailRelative == "" || name == "" {
		return ""
	}
	dir := filepath.ToSlash(filepath.Dir(filepath.FromSlash(detailRelative)))
	if dir == "." || dir == "" {
		return name
	}
	return dir + "/" + name
}

func enrichmentRelativePath(detailRelative string) string {
	return siblingRelativePath(detailRelative, minutesEnrichmentFileName)
}

func whiteboardRelativePath(detailRelative string) string {
	return siblingRelativePath(detailRelative, "whiteboard-preview")
}

func cloneByteMap(input map[string][]byte) map[string][]byte {
	if len(input) == 0 {
		return map[string][]byte{}
	}
	output := make(map[string][]byte, len(input)+2)
	for key, value := range input {
		output[key] = append([]byte(nil), value...)
	}
	return output
}

func readBoundedRegularFile(path string, limit int64) ([]byte, error) {
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
	if err != nil || int64(len(content)) > limit {
		return nil, ErrInvalidArtifact
	}
	return content, nil
}

func restrictedIdentityRef(identity string) string {
	sum := sha256.Sum256([]byte(identity))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func encodeFeishuCheckpoint(checkpoint feishuCheckpoint) (json.RawMessage, error) {
	if !validFeishuCheckpoint(checkpoint) {
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
	if !validFeishuCheckpoint(checkpoint) {
		return feishuCheckpoint{}, fmt.Errorf("invalid Feishu checkpoint")
	}
	return checkpoint, nil
}

func validFeishuCheckpoint(checkpoint feishuCheckpoint) bool {
	if checkpoint.Version != feishuCheckpointVersion ||
		!sha256Pattern.MatchString(checkpoint.AudioDigest) ||
		checkpoint.EnrichmentPendingAttempts < 0 ||
		checkpoint.EnrichmentPendingAttempts > maxEnrichmentPendingAttempts {
		return false
	}
	if _, err := parseCheckpointTime(checkpoint.CoreReadyAt); err != nil {
		return false
	}
	if _, err := parseCheckpointTime(checkpoint.EnrichmentDeadlineAt); err != nil {
		return false
	}
	if (checkpoint.EnrichmentRelativePath == "") != (checkpoint.EnrichmentSHA256 == "") ||
		(checkpoint.WhiteboardRelativePath == "") != (checkpoint.WhiteboardSHA256 == "") {
		return false
	}
	for _, pair := range [][2]string{
		{checkpoint.EnrichmentRelativePath, checkpoint.EnrichmentSHA256},
		{checkpoint.WhiteboardRelativePath, checkpoint.WhiteboardSHA256},
	} {
		if pair[0] == "" {
			continue
		}
		if filepath.IsAbs(pair[0]) || pathEscapesRoot(filepath.Clean(filepath.FromSlash(pair[0]))) ||
			!sha256Pattern.MatchString(pair[1]) {
			return false
		}
	}
	for name, hash := range checkpoint.EnrichmentRawSHA256 {
		if (name != "note-detail.json" &&
			name != "note-document.json" &&
			name != minutesEnrichmentDiagnosticFileName) ||
			!sha256Pattern.MatchString(hash) {
			return false
		}
	}
	if checkpoint.Phase == feishuPhaseTranscriptStored &&
		checkpoint.CoreReadyAt != "" &&
		(checkpoint.EnrichmentRelativePath == "" ||
			checkpoint.EnrichmentRawSHA256["note-detail.json"] == "" ||
			checkpoint.EnrichmentRawSHA256["note-document.json"] == "") {
		return false
	}
	return true
}

func resetCopiedFeishuCheckpoint(
	checkpoint models.ProcessingCheckpoint,
	now time.Time,
) (models.ProcessingCheckpoint, error) {
	if checkpoint.Adapter != feishuMinutesAdapterName {
		return checkpoint, nil
	}
	state, err := decodeFeishuCheckpoint(json.RawMessage(checkpoint.StateJSON))
	if err != nil {
		return checkpoint, err
	}
	if strings.TrimSpace(state.MinuteToken) == "" ||
		strings.TrimSpace(state.TranscriptRelativePath) == "" ||
		strings.TrimSpace(state.DetailRelativePath) == "" {
		return checkpoint, nil
	}
	if state.Phase == feishuPhaseTranscriptStored && state.CoreReadyAt == "" {
		return checkpoint, nil
	}
	state.Phase = feishuPhaseMinutesEnrichment
	state.CoreReadyAt = ""
	state.EnrichmentDeadlineAt = ""
	state.EnrichmentPendingAttempts = 0
	state.EnrichmentRelativePath = ""
	state.EnrichmentSHA256 = ""
	state.WhiteboardRelativePath = ""
	state.WhiteboardSHA256 = ""
	state.EnrichmentRawSHA256 = nil
	encoded, err := encodeFeishuCheckpoint(state)
	if err != nil {
		return checkpoint, err
	}
	sum := sha256.Sum256(encoded)
	checkpoint.StateJSON = string(encoded)
	checkpoint.StateHash = hex.EncodeToString(sum[:])
	checkpoint.Status = ExternalProgressWaiting
	checkpoint.UpdatedAt = now.UTC()
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
	NoteID                string
	Summary               string
	SummaryPresent        bool
	TranscriptFile        string
	TranscriptFilePresent bool
	Chapters              []MinutesChapter
	Keywords              []string
	Pending               bool
}

type minuteDetailEntry struct {
	MinuteToken string `json:"minute_token"`
	NoteID      string `json:"note_id"`
	Artifacts   struct {
		Summary        *string         `json:"summary"`
		TranscriptFile *string         `json:"transcript_file"`
		Chapters       json.RawMessage `json:"chapters"`
		Keywords       json.RawMessage `json:"keywords"`
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
			chapters, err := parseMinutesChaptersStrict(entry.Artifacts.Chapters)
			if err != nil {
				return minuteDetail{}, false, NewAdapterError(
					minutesEnrichmentSectionCode,
					"Feishu Minutes chapters could not be parsed",
					false,
				)
			}
			keywords, err := parseMinutesKeywordsStrict(entry.Artifacts.Keywords)
			if err != nil {
				return minuteDetail{}, false, NewAdapterError(
					minutesEnrichmentSectionCode,
					"Feishu Minutes keywords could not be parsed",
					false,
				)
			}
			detail := minuteDetail{
				MinuteToken: entry.MinuteToken,
				NoteID:      strings.TrimSpace(entry.NoteID),
				Pending:     minuteDetailEntryPending(entry.Error),
				Chapters:    chapters,
				Keywords:    keywords,
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
