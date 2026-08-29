// processing-real-smoke runs the actual #206 native Minutes adapter chain in an isolated
// directory. Its evidence intentionally contains hashes, sizes, statuses, and
// counts only; it never serializes transcripts, summaries, paths, remote
// identities, or credentials.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"magicpodcast/internal/processing"
)

type smokeEvidence struct {
	SchemaVersion string        `json:"schema_version"`
	Status        string        `json:"status"`
	Stage         string        `json:"stage"`
	ResumeOnly    bool          `json:"resume_only"`
	ErrorCode     string        `json:"error_code,omitempty"`
	StartedAt     string        `json:"started_at"`
	UpdatedAt     string        `json:"updated_at"`
	FinishedAt    string        `json:"finished_at,omitempty"`
	ElapsedMS     int64         `json:"elapsed_ms,omitempty"`
	Build         buildEvidence `json:"build"`
	Events        []string      `json:"events,omitempty"`

	Audio struct {
		Bytes  int64  `json:"bytes"`
		SHA256 string `json:"sha256"`
	} `json:"audio"`
	Feishu struct {
		Polls                  int    `json:"polls"`
		CheckpointWrites       int    `json:"checkpoint_writes"`
		LatestCheckpointSHA256 string `json:"latest_checkpoint_sha256,omitempty"`
		Status                 string `json:"status,omitempty"`
		SummaryBytes           int    `json:"summary_bytes,omitempty"`
		TranscriptBytes        int    `json:"transcript_bytes,omitempty"`
		SegmentCount           int    `json:"segment_count,omitempty"`
		RawArtifactCount       int    `json:"raw_artifact_count,omitempty"`
		RecoverableReadErrors  int    `json:"recoverable_read_errors,omitempty"`
		SourceRefCount         int    `json:"source_ref_count,omitempty"`
		SkillVersionCount      int    `json:"skill_version_count,omitempty"`
	} `json:"feishu"`
	Artifacts struct {
		Published                bool   `json:"published"`
		ManifestSHA256           string `json:"manifest_sha256,omitempty"`
		AudioSHA256              string `json:"audio_sha256,omitempty"`
		MinutesSummarySHA256     string `json:"minutes_summary_sha256,omitempty"`
		TranscriptSHA256         string `json:"transcript_sha256,omitempty"`
		TranscriptTimelineSHA256 string `json:"transcript_timeline_sha256,omitempty"`
		RawArtifactCount         int    `json:"raw_artifact_count,omitempty"`
		VerifiedFileCount        int    `json:"verified_file_count,omitempty"`
	} `json:"artifacts"`
}

type buildEvidence struct {
	GoVersion   string `json:"go_version,omitempty"`
	VCSRevision string `json:"vcs_revision,omitempty"`
	VCSModified string `json:"vcs_modified,omitempty"`
}

var (
	buildRevision = "unknown"
	buildModified = "unknown"
)

func main() {
	os.Exit(run())
}

func run() int {
	var (
		audioPath    = flag.String("audio", "", "managed local audio file")
		larkCLI      = flag.String("lark-cli", "", "isolated lark-cli wrapper")
		larkWorkRoot = flag.String("lark-work-root", "", "isolated Feishu work root")
		artifactRoot = flag.String("artifact-root", "", "artifact root")
		evidencePath = flag.String("evidence", "", "sanitized evidence JSON")
		pollInterval = flag.Duration("poll-interval", 30*time.Second, "Feishu artifact poll interval")
		timeout      = flag.Duration("timeout", 2*time.Hour, "whole smoke timeout")
		runID        = flag.Uint("run-id", 181001, "isolated run identity")
		episodeID    = flag.Uint("episode-id", 181001, "isolated episode identity")
		resumeOnly   = flag.Bool("resume-only", false, "resume the durable Feishu checkpoint without a new upload")
	)
	flag.Parse()

	started := time.Now().UTC()
	evidence := smokeEvidence{
		SchemaVersion: "2.0.0",
		Status:        "running",
		Stage:         "preflight",
		ResumeOnly:    *resumeOnly,
		Build:         buildMetadata(),
		StartedAt:     started.Format(time.RFC3339Nano),
	}
	defer func() {
		evidence.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		evidence.FinishedAt = evidence.UpdatedAt
		evidence.ElapsedMS = time.Since(started).Milliseconds()
		if *evidencePath != "" {
			_ = writeEvidence(*evidencePath, evidence)
		}
	}()

	fail := func(stage, code string) int {
		evidence.Status = "failed"
		evidence.Stage = stage
		evidence.ErrorCode = code
		record(*evidencePath, &evidence, "failed")
		return 1
	}
	if *runID == 0 || *episodeID == 0 || *pollInterval <= 0 || *timeout <= 0 {
		return fail("preflight", "invalid_smoke_request")
	}
	for _, value := range []string{
		*audioPath,
		*larkCLI,
		*larkWorkRoot,
		*artifactRoot,
		*evidencePath,
	} {
		if strings.TrimSpace(value) == "" || !filepath.IsAbs(value) {
			return fail("preflight", "invalid_smoke_path")
		}
	}

	bytes, digest, err := digestRegularFile(*audioPath)
	if err != nil || bytes <= 0 || bytes > 6*1024*1024*1024 {
		return fail("preflight", "invalid_audio")
	}
	evidence.Audio.Bytes = bytes
	evidence.Audio.SHA256 = digest
	if _, err := ensurePrivateDirectory(*larkWorkRoot); err != nil {
		return fail("preflight", "lark_work_root_unavailable")
	}
	if _, err := ensurePrivateDirectory(*artifactRoot); err != nil {
		return fail("preflight", "artifact_root_unavailable")
	}
	record(*evidencePath, &evidence, "preflight_complete")

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	checkpointPath := filepath.Join(*larkWorkRoot, "smoke-checkpoint.json")
	adapter, err := processing.NewFeishuMinutesAdapter(
		*larkCLI,
		*larkWorkRoot,
		func(context.Context, uint) (string, string, error) {
			return *audioPath, digest, nil
		},
	)
	if err != nil {
		return fail("feishu_setup", "lark_cli_unavailable")
	}
	request := processing.TranscriptionRequest{
		RunID:           uint(*runID),
		EpisodeID:       uint(*episodeID),
		AudioDigest:     digest,
		PipelineVersion: processing.NativeMinutesPipelineVersion,
		PersistCheckpoint: func(_ context.Context, _ string, state json.RawMessage) error {
			if err := writePrivateFile(checkpointPath, state); err != nil {
				return err
			}
			evidence.Feishu.CheckpointWrites++
			sum := sha256.Sum256(state)
			evidence.Feishu.LatestCheckpointSHA256 = hex.EncodeToString(sum[:])
			record(*evidencePath, &evidence, "checkpoint_persisted")
			return nil
		},
	}

	var progress processing.TranscriptionProgress
	if *resumeOnly {
		state, readErr := os.ReadFile(checkpointPath)
		if readErr != nil || !json.Valid(state) {
			return fail("feishu_resume", "checkpoint_unavailable")
		}
		evidence.Stage = "feishu_resume"
		record(*evidencePath, &evidence, "feishu_resume_started")
		progress, err = adapter.Resume(ctx, request, state)
	} else {
		evidence.Stage = "drive_upload"
		record(*evidencePath, &evidence, "drive_upload_started")
		progress, err = adapter.Begin(ctx, request)
	}
	for {
		if err != nil {
			if retryableRead(progress, err) {
				evidence.Stage = "feishu_retryable_read"
				evidence.Feishu.RecoverableReadErrors++
				record(*evidencePath, &evidence, "feishu_retryable_read")
				if waitErr := waitForNextPoll(ctx, *pollInterval); waitErr != nil {
					return fail("feishu_waiting", "smoke_timeout")
				}
				progress, err = adapter.Resume(ctx, request, progress.Checkpoint)
				continue
			}
			return fail("feishu_transcription", adapterErrorCode(err))
		}
		if progress.Status == processing.ExternalProgressCompleted {
			break
		}
		if progress.Status != processing.ExternalProgressWaiting || len(progress.Checkpoint) == 0 {
			return fail("feishu_transcription", "unexpected_external_status")
		}
		evidence.Stage = "feishu_waiting"
		evidence.Feishu.Status = progress.Status
		evidence.Feishu.Polls++
		record(*evidencePath, &evidence, "feishu_waiting")
		if err := waitForNextPoll(ctx, *pollInterval); err != nil {
			return fail("feishu_waiting", "smoke_timeout")
		}
		progress, err = adapter.Resume(ctx, request, progress.Checkpoint)
	}
	if strings.TrimSpace(progress.MinutesSummary) == "" ||
		strings.TrimSpace(progress.Transcript) == "" ||
		len(progress.Segments) == 0 {
		return fail("feishu_transcription", "incomplete_minutes_artifacts")
	}
	evidence.Feishu.Status = progress.Status
	evidence.Feishu.SummaryBytes = len(progress.MinutesSummary)
	evidence.Feishu.TranscriptBytes = len(progress.Transcript)
	evidence.Feishu.SegmentCount = len(progress.Segments)
	evidence.Feishu.RawArtifactCount = len(progress.RawArtifacts)
	evidence.Feishu.SourceRefCount = len(progress.SourceRefs)
	evidence.Feishu.SkillVersionCount = len(progress.SkillVersions)
	evidence.Stage = "feishu_complete"
	record(*evidencePath, &evidence, "feishu_complete")

	store, err := processing.NewDiskArtifactStore(*artifactRoot)
	if err != nil {
		return fail("artifact_setup", "artifact_root_unavailable")
	}
	evidence.Stage = "artifact_publish"
	published, err := store.Publish(ctx, processing.ArtifactPublishRequest{
		RunID:                uint(*runID),
		EpisodeID:            uint(*episodeID),
		AudioDigest:          digest,
		PipelineVersion:      processing.NativeMinutesPipelineVersion,
		NativeMinutes:        true,
		MinutesSummary:       progress.MinutesSummary,
		Transcript:           progress.Transcript,
		TranscriptSegments:   progress.Segments,
		TranscriptionAdapter: adapter.Name(),
		TranscriptionVersion: adapter.Version(),
		SkillVersions:        progress.SkillVersions,
		Sources:              progress.SourceRefs,
		RawArtifacts:         progress.RawArtifacts,
		GeneratedAt:          time.Now().UTC(),
	})
	if err != nil {
		return fail("artifact_publish", "artifact_publish_failed")
	}
	files, err := verifyArtifactSet(published, len(progress.RawArtifacts))
	if err != nil {
		return fail("artifact_verify", "artifact_verify_failed")
	}
	evidence.Artifacts.Published = true
	evidence.Artifacts.ManifestSHA256 = published.ManifestSHA256
	evidence.Artifacts.AudioSHA256 = published.AudioSHA256
	evidence.Artifacts.MinutesSummarySHA256 = published.MinutesSummarySHA256
	evidence.Artifacts.TranscriptSHA256 = published.TranscriptSHA256
	evidence.Artifacts.TranscriptTimelineSHA256 = published.TranscriptTimelineSHA256
	evidence.Artifacts.RawArtifactCount = len(progress.RawArtifacts)
	evidence.Artifacts.VerifiedFileCount = files
	evidence.Status = "completed"
	evidence.Stage = "complete"
	record(*evidencePath, &evidence, "artifacts_published")
	return 0
}

func adapterErrorCode(err error) string {
	var adapterErr *processing.AdapterError
	if errors.As(err, &adapterErr) && strings.TrimSpace(adapterErr.ErrorCode) != "" {
		return adapterErr.ErrorCode
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "smoke_timeout"
	}
	return "smoke_step_failed"
}

func retryableRead(progress processing.TranscriptionProgress, err error) bool {
	if len(progress.Checkpoint) == 0 {
		return false
	}
	var adapterErr *processing.AdapterError
	return errors.As(err, &adapterErr) && adapterErr.CanRetry && !adapterErr.ResultUnknown
}

func waitForNextPoll(ctx context.Context, interval time.Duration) error {
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func digestRegularFile(path string) (int64, string, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return 0, "", fmt.Errorf("invalid audio")
	}
	file, err := os.Open(path)
	if err != nil {
		return 0, "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return 0, "", err
	}
	return info.Size(), hex.EncodeToString(hash.Sum(nil)), nil
}

func ensurePrivateDirectory(path string) (string, error) {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return "", err
	}
	if err := os.Chmod(path, 0o700); err != nil {
		return "", err
	}
	canonical, err := filepath.EvalSymlinks(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	info, err := os.Stat(canonical)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("invalid directory")
	}
	return canonical, nil
}

func writePrivateFile(path string, data []byte) error {
	if _, err := ensurePrivateDirectory(filepath.Dir(path)); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".tmp-")
	if err != nil {
		return err
	}
	temporary := file.Name()
	defer os.Remove(temporary)
	if err := file.Chmod(0o600); err != nil {
		file.Close()
		return err
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}

func writeEvidence(path string, evidence smokeEvidence) error {
	evidence.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	payload, err := json.MarshalIndent(evidence, "", "  ")
	if err != nil {
		return err
	}
	return writePrivateFile(path, append(payload, '\n'))
}

func record(path string, evidence *smokeEvidence, event string) {
	evidence.Events = append(evidence.Events, event)
	if path != "" {
		_ = writeEvidence(path, *evidence)
	}
	output, _ := json.Marshal(map[string]any{
		"event":      event,
		"stage":      evidence.Stage,
		"status":     evidence.Status,
		"elapsed_ms": time.Since(parseStartedAt(evidence.StartedAt)).Milliseconds(),
	})
	fmt.Println(string(output))
}

func buildMetadata() buildEvidence {
	evidence := buildEvidence{
		GoVersion:   "unknown",
		VCSRevision: buildRevision,
		VCSModified: buildModified,
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return evidence
	}
	evidence.GoVersion = info.GoVersion
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			evidence.VCSRevision = setting.Value
		case "vcs.modified":
			evidence.VCSModified = setting.Value
		}
	}
	return evidence
}

func parseStartedAt(value string) time.Time {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Now().UTC()
	}
	return parsed
}

func verifyArtifactSet(published processing.ArtifactPublishResult, rawCount int) (int, error) {
	rootInfo, err := os.Lstat(published.RootPath)
	if err != nil || !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return 0, fmt.Errorf("artifact root invalid")
	}
	files := map[string]string{
		"manifest.json":      published.ManifestSHA256,
		"minutes-summary.md": published.MinutesSummarySHA256,
		"transcript.md":      published.TranscriptSHA256,
		"transcript.json":    published.TranscriptTimelineSHA256,
	}
	for relative, expected := range files {
		_, actual, err := digestRegularFile(filepath.Join(published.RootPath, relative))
		if err != nil || actual != expected {
			return 0, fmt.Errorf("artifact digest mismatch")
		}
	}
	count := 0
	err = filepath.WalkDir(published.RootPath, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type().IsRegular() {
			count++
		}
		return nil
	})
	if err != nil || count != rawCount+4 {
		return 0, fmt.Errorf("artifact file count invalid")
	}
	return count, nil
}
