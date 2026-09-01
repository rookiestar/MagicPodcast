package processing

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"magicpodcast/internal/models"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type recoveryFixture struct {
	db         *gorm.DB
	audio      *DiskAudioStore
	recovery   *AudioRecoveryStore
	downloader *fakeRecoveryDownloader
	artifact   models.EpisodeArtifactSet
	asset      models.EpisodeAudioAsset
	target     string
	now        time.Time
}

type fakeRecoveryDownloader struct {
	mu         sync.Mutex
	payload    []byte
	err        error
	mode       os.FileMode
	calls      int
	fileTokens []string
}

type blockingRecoveryDownloader struct{}

func (blockingRecoveryDownloader) Download(
	ctx context.Context,
	_ string,
	_ string,
	_ string,
) error {
	<-ctx.Done()
	return ctx.Err()
}

func (f *fakeRecoveryDownloader) Download(
	_ context.Context,
	fileToken string,
	directory string,
	fileName string,
) error {
	f.mu.Lock()
	f.calls++
	f.fileTokens = append(f.fileTokens, fileToken)
	payload := append([]byte(nil), f.payload...)
	err := f.err
	mode := f.mode
	f.mu.Unlock()
	if err != nil {
		return err
	}
	if mode == 0 {
		mode = 0o600
	}
	return os.WriteFile(filepath.Join(directory, fileName), payload, mode)
}

func (f *fakeRecoveryDownloader) Calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func newRecoveryFixture(t *testing.T, payload []byte) recoveryFixture {
	t.Helper()
	db := openProcessingTestDB(t)
	episode := createProcessingEpisode(t, db, false, "audio-recovery")
	require.NoError(t, db.Model(&episode).Updates(map[string]any{
		"duration": 60,
	}).Error)
	now := time.Date(2026, 9, 1, 5, 0, 0, 0, time.UTC)
	audioRoot := t.TempDir()
	audio, err := NewDiskAudioStore(db, audioRoot)
	require.NoError(t, err)
	digestBytes := sha256.Sum256(payload)
	digest := hex.EncodeToString(digestBytes[:])
	relativePath := filepath.Join("episodes", "1", "managed.mp3")
	if episode.ID != 1 {
		relativePath = filepath.Join("episodes", "episode", "managed.mp3")
	}
	target := filepath.Join(audio.root, relativePath)
	require.NoError(t, os.MkdirAll(filepath.Dir(target), 0o700))
	require.NoError(t, os.Chmod(filepath.Dir(target), 0o700))
	asset := models.EpisodeAudioAsset{
		EpisodeID:       episode.ID,
		SourceDigest:    strings.Repeat("s", 64),
		Status:          models.EpisodeAudioAssetStatusReady,
		RelativePath:    filepath.ToSlash(relativePath),
		SHA256:          digest,
		SizeBytes:       int64(len(payload)),
		DurationSeconds: 60,
		MediaType:       "audio/mpeg",
		Extension:       ".mp3",
		QueuedAt:        now,
		ReadyAt:         &now,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	require.NoError(t, db.Create(&asset).Error)
	run := models.EpisodeProcessingRun{
		EpisodeID:       episode.ID,
		ProcessingKey:   strings.Repeat("k", 64),
		AudioDigest:     digest,
		PipelineVersion: NativeMinutesPipelineVersion,
		TriggerSource:   models.ProcessingTriggerManual,
		Status:          models.ProcessingRunStatusCompleted,
		CurrentStep:     StepArtifactPublish,
		AttemptCount:    1,
		MaxAttempts:     3,
		RetryDeadlineAt: now.Add(24 * time.Hour),
		FinishedAt:      &now,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	require.NoError(t, db.Create(&run).Error)
	checkpointState, err := encodeFeishuCheckpoint(feishuCheckpoint{
		Version:     feishuCheckpointVersion,
		Phase:       feishuPhaseTranscriptStored,
		AudioDigest: digest,
		FileToken:   "boxcn_recovery_1234",
		MinuteToken: "obcn_recovery_1234",
	})
	require.NoError(t, err)
	checkpointHash := sha256.Sum256(checkpointState)
	require.NoError(t, db.Create(&models.ProcessingCheckpoint{
		RunID:          run.ID,
		Step:           StepTranscription,
		Adapter:        feishuMinutesAdapterName,
		AdapterVersion: feishuMinutesAdapterVersion,
		Status:         ExternalProgressCompleted,
		StateJSON:      string(checkpointState),
		StateHash:      hex.EncodeToString(checkpointHash[:]),
		CreatedAt:      now,
		UpdatedAt:      now,
	}).Error)
	artifact := models.EpisodeArtifactSet{
		RunID:                    run.ID,
		EpisodeID:                episode.ID,
		PipelineVersion:          NativeMinutesPipelineVersion,
		RootPath:                 filepath.Join(t.TempDir(), "artifact"),
		ManifestPath:             "manifest.json",
		ManifestSHA256:           strings.Repeat("1", 64),
		AudioSHA256:              digest,
		MinutesSummarySHA256:     strings.Repeat("2", 64),
		TranscriptSHA256:         strings.Repeat("3", 64),
		TranscriptTimelineSHA256: strings.Repeat("4", 64),
		NotesSHA256:              strings.Repeat("5", 64),
		IsCurrent:                true,
		CreatedAt:                now,
	}
	require.NoError(t, db.Create(&artifact).Error)
	downloader := &fakeRecoveryDownloader{payload: append([]byte(nil), payload...)}
	recovery, err := NewAudioRecoveryStore(
		db,
		audio,
		downloader,
		WithAudioRecoveryClock(func() time.Time { return now }),
		WithAudioRecoveryPolicy(AudioRecoveryPolicy{
			MaxAttempts: 3,
			MaxElapsed:  24 * time.Hour,
			BaseDelay:   time.Second,
			ClaimTTL:    time.Hour,
		}),
	)
	require.NoError(t, err)
	return recoveryFixture{
		db:         db,
		audio:      audio,
		recovery:   recovery,
		downloader: downloader,
		artifact:   artifact,
		asset:      asset,
		target:     target,
		now:        now,
	}
}

func TestAudioRecoveryCompletesIdempotentlyAndCanRequeueAfterLocalLoss(t *testing.T) {
	payload := []byte("ID3 managed recovery audio")
	fixture := newRecoveryFixture(t, payload)

	summary, err := fixture.recovery.Summary(context.Background(), fixture.artifact)
	require.NoError(t, err)
	require.True(t, summary.Recoverable)
	require.Empty(t, summary.Status)

	queued, err := fixture.recovery.Enqueue(context.Background(), fixture.artifact.ID)
	require.NoError(t, err)
	require.True(t, queued.Queued)
	require.Equal(t, models.EpisodeArtifactAudioRecoveryStatusQueued, queued.AudioRecovery.Status)

	duplicate, err := fixture.recovery.Enqueue(context.Background(), fixture.artifact.ID)
	require.NoError(t, err)
	require.True(t, duplicate.Reused)
	require.False(t, duplicate.Queued)

	ids, err := fixture.recovery.ListClaimable(context.Background(), 4)
	require.NoError(t, err)
	require.Len(t, ids, 1)
	claim, claimed, err := fixture.recovery.Claim(context.Background(), ids[0])
	require.NoError(t, err)
	require.True(t, claimed)
	_, claimed, err = fixture.recovery.Claim(context.Background(), ids[0])
	require.NoError(t, err)
	require.False(t, claimed)
	require.NoError(t, fixture.recovery.Recover(context.Background(), claim))

	content, err := os.ReadFile(fixture.target)
	require.NoError(t, err)
	require.Equal(t, payload, content)
	ready, err := fixture.audio.ResolveReadyAudioByDigest(
		context.Background(), fixture.artifact.EpisodeID, fixture.artifact.AudioSHA256,
	)
	require.NoError(t, err)
	require.Equal(t, fixture.target, ready.Path)
	require.Equal(t, 1, fixture.downloader.Calls())

	completed, err := fixture.recovery.Summary(context.Background(), fixture.artifact)
	require.NoError(t, err)
	require.False(t, completed.Recoverable)
	require.Equal(t, models.EpisodeArtifactAudioRecoveryStatusCompleted, completed.Status)

	require.NoError(t, os.Remove(fixture.target))
	missing, err := fixture.recovery.Summary(context.Background(), fixture.artifact)
	require.NoError(t, err)
	require.True(t, missing.Recoverable)
	require.Empty(t, missing.Status)

	requeued, err := fixture.recovery.Enqueue(context.Background(), fixture.artifact.ID)
	require.NoError(t, err)
	require.True(t, requeued.Queued)
	ids, err = fixture.recovery.ListClaimable(context.Background(), 4)
	require.NoError(t, err)
	claim, claimed, err = fixture.recovery.Claim(context.Background(), ids[0])
	require.NoError(t, err)
	require.True(t, claimed)
	require.NoError(t, fixture.recovery.Recover(context.Background(), claim))
	require.Equal(t, 2, fixture.downloader.Calls())

	var count int64
	require.NoError(t, fixture.db.Model(&models.EpisodeArtifactAudioRecovery{}).
		Where("artifact_set_id = ?", fixture.artifact.ID).Count(&count).Error)
	require.Equal(t, int64(1), count)
}

func TestAudioRecoveryHealthyLocalAudioWinsOverUnreadableRepairSource(t *testing.T) {
	fixture := newRecoveryFixture(t, []byte("already healthy"))
	require.NoError(t, os.WriteFile(fixture.target, []byte("already healthy"), 0o600))
	var checkpoint models.ProcessingCheckpoint
	require.NoError(t, fixture.db.Where("run_id = ?", fixture.artifact.RunID).First(&checkpoint).Error)
	require.NoError(t, fixture.db.Model(&checkpoint).Update("state_hash", strings.Repeat("0", 64)).Error)

	summary, err := fixture.recovery.Summary(context.Background(), fixture.artifact)
	require.NoError(t, err)
	require.False(t, summary.Recoverable)
	require.Equal(t, models.EpisodeArtifactAudioRecoveryStatusCompleted, summary.Status)

	result, err := fixture.recovery.Enqueue(context.Background(), fixture.artifact.ID)
	require.NoError(t, err)
	require.True(t, result.AlreadyAvailable)
	require.False(t, result.Queued)
	require.Zero(t, fixture.downloader.Calls())
}

func TestAudioRecoveryWorkerReclaimsExpiredLease(t *testing.T) {
	fixture := newRecoveryFixture(t, []byte("worker recovery"))
	service := NewService(fixture.db, WithAudioRecovery(fixture.recovery))
	artifactStore, err := NewDiskArtifactStore(t.TempDir())
	require.NoError(t, err)
	engine, err := NewEngine(
		service,
		recoveryTestTranscriber{},
		recoveryTestRuntime{},
		artifactStore,
		nil,
	)
	require.NoError(t, err)
	worker, err := NewWorker(service, engine, nil, WorkerConfig{
		ScanInterval:         time.Second,
		ExternalPollInterval: time.Minute,
		BatchSize:            4,
	})
	require.NoError(t, err)

	_, err = fixture.recovery.Enqueue(context.Background(), fixture.artifact.ID)
	require.NoError(t, err)
	ids, err := fixture.recovery.ListClaimable(context.Background(), 4)
	require.NoError(t, err)
	claim, claimed, err := fixture.recovery.Claim(context.Background(), ids[0])
	require.NoError(t, err)
	require.True(t, claimed)
	past := fixture.now.Add(-time.Minute)
	// The recovery row ID is not public in the enqueue result; find it through
	// the artifact identity before the worker reclaims the expired lease.
	var recovery models.EpisodeArtifactAudioRecovery
	require.NoError(t, fixture.db.Where("artifact_set_id = ?", fixture.artifact.ID).First(&recovery).Error)
	require.NoError(t, fixture.db.Model(&recovery).Update("claim_expires_at", past).Error)

	require.NoError(t, worker.RunOnce(context.Background()))
	require.Equal(t, 1, fixture.downloader.Calls())
	var completed models.EpisodeArtifactAudioRecovery
	require.NoError(t, fixture.db.First(&completed, recovery.ID).Error)
	require.Equal(t, models.EpisodeArtifactAudioRecoveryStatusCompleted, completed.Status)
	require.NotEqual(t, claim.Token, completed.ClaimToken)
}

func TestAudioRecoveryExhaustedExpiredLeaseBecomesTerminal(t *testing.T) {
	fixture := newRecoveryFixture(t, []byte("exhausted recovery"))
	_, err := fixture.recovery.Enqueue(context.Background(), fixture.artifact.ID)
	require.NoError(t, err)
	ids, err := fixture.recovery.ListClaimable(context.Background(), 4)
	require.NoError(t, err)
	claim, claimed, err := fixture.recovery.Claim(context.Background(), ids[0])
	require.NoError(t, err)
	require.True(t, claimed)
	var recovery models.EpisodeArtifactAudioRecovery
	require.NoError(t, fixture.db.Where("artifact_set_id = ?", fixture.artifact.ID).First(&recovery).Error)
	require.NoError(t, fixture.db.Model(&recovery).Updates(map[string]any{
		"attempt_count":    recovery.MaxAttempts,
		"claim_expires_at": fixture.now.Add(-time.Minute),
	}).Error)

	ids, err = fixture.recovery.ListClaimable(context.Background(), 4)
	require.NoError(t, err)
	require.Empty(t, ids)
	require.NotEmpty(t, claim.Token)
	require.NoError(t, fixture.db.First(&recovery, recovery.ID).Error)
	require.Equal(t, models.EpisodeArtifactAudioRecoveryStatusFailed, recovery.Status)
	require.Equal(t, AudioRecoveryErrorDownloadFailed, recovery.ErrorCode)
	require.False(t, recovery.ErrorRetryable)
	require.Zero(t, fixture.downloader.Calls())
}

func TestAudioRecoveryDigestFailureDoesNotOverwriteCurrentFile(t *testing.T) {
	payload := []byte("expected remote audio")
	fixture := newRecoveryFixture(t, payload)
	old := bytes.Repeat([]byte("x"), len(payload))
	require.NoError(t, os.WriteFile(fixture.target, old, 0o600))
	fixture.downloader.mu.Lock()
	fixture.downloader.payload = bytes.Repeat([]byte("y"), len(payload))
	fixture.downloader.mu.Unlock()

	_, err := fixture.recovery.Enqueue(context.Background(), fixture.artifact.ID)
	require.NoError(t, err)
	ids, err := fixture.recovery.ListClaimable(context.Background(), 4)
	require.NoError(t, err)
	claim, claimed, err := fixture.recovery.Claim(context.Background(), ids[0])
	require.NoError(t, err)
	require.True(t, claimed)
	var recoveryErr *AudioRecoveryError
	require.ErrorAs(t, fixture.recovery.Recover(context.Background(), claim), &recoveryErr)
	require.Equal(t, AudioRecoveryErrorDigestMismatch, recoveryErr.Code)
	restored, err := os.ReadFile(fixture.target)
	require.NoError(t, err)
	require.Equal(t, old, restored)
	var recovery models.EpisodeArtifactAudioRecovery
	require.NoError(t, fixture.db.Where("artifact_set_id = ?", fixture.artifact.ID).First(&recovery).Error)
	require.Equal(t, models.EpisodeArtifactAudioRecoveryStatusFailed, recovery.Status)
	require.False(t, recovery.ErrorRetryable)

	summary, err := fixture.recovery.Summary(context.Background(), fixture.artifact)
	require.NoError(t, err)
	require.True(t, summary.Recoverable)
	require.Equal(t, models.EpisodeArtifactAudioRecoveryStatusFailed, summary.Status)
	require.Equal(t, AudioRecoveryErrorDigestMismatch, summary.ErrorCode)
	require.NotContains(t, summary.ErrorMessage, fixture.target)
	require.NotContains(t, summary.ErrorMessage, fixture.artifact.AudioSHA256)
}

func TestAudioRecoveryRejectsInvalidCheckpointAndManagedPath(t *testing.T) {
	fixture := newRecoveryFixture(t, []byte("checkpoint guard"))
	var checkpoint models.ProcessingCheckpoint
	require.NoError(t, fixture.db.Where("run_id = ?", fixture.artifact.RunID).First(&checkpoint).Error)
	require.NoError(t, fixture.db.Model(&checkpoint).Update("state_hash", strings.Repeat("0", 64)).Error)
	summary, err := fixture.recovery.Summary(context.Background(), fixture.artifact)
	require.NoError(t, err)
	require.False(t, summary.Recoverable)
	require.Equal(t, AudioRecoveryErrorCheckpointInvalid, summary.ErrorCode)

	fixture = newRecoveryFixture(t, []byte("path guard"))
	require.NoError(t, fixture.db.Model(&models.EpisodeAudioAsset{}).
		Where("id = ?", fixture.asset.ID).
		Update("relative_path", "../outside.mp3").Error)
	summary, err = fixture.recovery.Summary(context.Background(), fixture.artifact)
	require.NoError(t, err)
	require.False(t, summary.Recoverable)
	require.Contains(t, summary.ErrorCode, "audio_recovery_")
}

func TestAudioRecoveryRetriesOnlyTransientDownloadFailures(t *testing.T) {
	fixture := newRecoveryFixture(t, []byte("transient recovery"))
	fixture.recovery.maxAttempts = 2
	fixture.recovery.baseDelay = 0
	fixture.downloader.err = newAudioRecoveryError(
		AudioRecoveryErrorDownloadFailed,
		"飞书 Drive 音频下载失败，稍后可重试",
		true,
	)
	_, err := fixture.recovery.Enqueue(context.Background(), fixture.artifact.ID)
	require.NoError(t, err)
	for attempt := 0; attempt < 2; attempt++ {
		ids, listErr := fixture.recovery.ListClaimable(context.Background(), 4)
		require.NoError(t, listErr)
		if attempt == 1 {
			var queued models.EpisodeArtifactAudioRecovery
			require.NoError(t, fixture.db.Where("artifact_set_id = ?", fixture.artifact.ID).First(&queued).Error)
			require.NoError(t, fixture.db.Model(&queued).Update("next_attempt_at", fixture.now.Add(-time.Second)).Error)
			ids, listErr = fixture.recovery.ListClaimable(context.Background(), 4)
			require.NoError(t, listErr)
		}
		claim, claimed, claimErr := fixture.recovery.Claim(context.Background(), ids[0])
		require.NoError(t, claimErr)
		require.True(t, claimed)
		var recoveryErr *AudioRecoveryError
		require.ErrorAs(t, fixture.recovery.Recover(context.Background(), claim), &recoveryErr)
	}
	var recovery models.EpisodeArtifactAudioRecovery
	require.NoError(t, fixture.db.Where("artifact_set_id = ?", fixture.artifact.ID).First(&recovery).Error)
	require.Equal(t, models.EpisodeArtifactAudioRecoveryStatusFailed, recovery.Status)
	require.Equal(t, 2, recovery.AttemptCount)
}

func TestAudioRecoveryBoundsDownloadBySupplierTimeout(t *testing.T) {
	fixture := newRecoveryFixture(t, []byte("bounded recovery"))
	fixture.recovery.downloader = blockingRecoveryDownloader{}
	fixture.recovery.downloadTimeout = time.Millisecond

	_, err := fixture.recovery.Enqueue(context.Background(), fixture.artifact.ID)
	require.NoError(t, err)
	ids, err := fixture.recovery.ListClaimable(context.Background(), 4)
	require.NoError(t, err)
	claim, claimed, err := fixture.recovery.Claim(context.Background(), ids[0])
	require.NoError(t, err)
	require.True(t, claimed)

	var recoveryErr *AudioRecoveryError
	require.ErrorAs(t, fixture.recovery.Recover(context.Background(), claim), &recoveryErr)
	require.Equal(t, AudioRecoveryErrorDownloadTimeout, recoveryErr.Code)
	var recovery models.EpisodeArtifactAudioRecovery
	require.NoError(t, fixture.db.Where("artifact_set_id = ?", fixture.artifact.ID).First(&recovery).Error)
	require.Equal(t, models.EpisodeArtifactAudioRecoveryStatusQueued, recovery.Status)
	require.Equal(t, AudioRecoveryErrorDownloadTimeout, recovery.ErrorCode)
	require.True(t, recovery.ErrorRetryable)
}

func TestAudioRecoveryRejectsExpiredClaimBeforeDownload(t *testing.T) {
	fixture := newRecoveryFixture(t, []byte("expired recovery"))
	_, err := fixture.recovery.Enqueue(context.Background(), fixture.artifact.ID)
	require.NoError(t, err)
	ids, err := fixture.recovery.ListClaimable(context.Background(), 4)
	require.NoError(t, err)
	claim, claimed, err := fixture.recovery.Claim(context.Background(), ids[0])
	require.NoError(t, err)
	require.True(t, claimed)
	var recovery models.EpisodeArtifactAudioRecovery
	require.NoError(t, fixture.db.Where("artifact_set_id = ?", fixture.artifact.ID).First(&recovery).Error)
	require.NoError(t, fixture.db.Model(&recovery).Update(
		"claim_expires_at",
		fixture.now.Add(-time.Second),
	).Error)

	var recoveryErr *AudioRecoveryError
	require.ErrorAs(t, fixture.recovery.Recover(context.Background(), claim), &recoveryErr)
	require.Equal(t, AudioRecoveryErrorClaimLost, recoveryErr.Code)
	require.Zero(t, fixture.downloader.Calls())
}

func TestAudioRecoveryRejectsUnprotectedManagedAncestor(t *testing.T) {
	fixture := newRecoveryFixture(t, []byte("ancestor guard"))
	episodesRoot := filepath.Join(fixture.audio.root, "episodes")
	require.NoError(t, os.Chmod(episodesRoot, 0o755))

	summary, err := fixture.recovery.Summary(context.Background(), fixture.artifact)
	require.NoError(t, err)
	require.False(t, summary.Recoverable)
	require.Equal(t, AudioRecoveryErrorPermission, summary.ErrorCode)
}

type recoveryTestTranscriber struct{}

func (recoveryTestTranscriber) Name() string    { return "recovery-test-transcriber" }
func (recoveryTestTranscriber) Version() string { return "recovery-test-v1" }
func (recoveryTestTranscriber) Begin(context.Context, TranscriptionRequest) (TranscriptionProgress, error) {
	return TranscriptionProgress{}, errors.New("unused")
}
func (recoveryTestTranscriber) Resume(context.Context, TranscriptionRequest, json.RawMessage) (TranscriptionProgress, error) {
	return TranscriptionProgress{}, errors.New("unused")
}
func (recoveryTestTranscriber) Cancel(context.Context, uint, json.RawMessage) error { return nil }

type recoveryTestRuntime struct{}

func (recoveryTestRuntime) Name() string { return "recovery-test-runtime" }
func (recoveryTestRuntime) Execute(context.Context, RuntimeRequest) (RuntimeResult, error) {
	return RuntimeResult{}, errors.New("unused")
}
func (recoveryTestRuntime) Cancel(context.Context, uint) error { return nil }
