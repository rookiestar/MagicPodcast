package processing

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"magicpodcast/internal/models"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestWorkerPreparesAudioStartsRunAndCompletesPipeline(t *testing.T) {
	db := openProcessingTestDB(t)
	now := time.Date(2026, 8, 24, 14, 0, 0, 0, time.UTC)
	episode := createProcessingEpisode(t, db, true, "worker-audio")
	asset := models.EpisodeAudioAsset{
		EpisodeID:       episode.ID,
		SourceDigest:    strings.Repeat("b", 64),
		Status:          models.EpisodeAudioAssetStatusQueued,
		DurationSeconds: 60,
		QueuedAt:        now,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	require.NoError(t, db.Create(&asset).Error)
	audio := &workerFakeAudio{
		assetIDs: []uint{asset.ID},
		claim: AudioClaim{
			AssetID:   asset.ID,
			EpisodeID: episode.ID,
			Token:     strings.Repeat("a", 64),
		},
		enqueueResult: AudioEnqueueResult{Asset: asset},
	}
	resolver := &staticProcessingInputResolver{
		input: ProcessingInput{PipelineVersion: "pipeline-v1"},
		err:   errors.New("audio not ready"),
	}
	service := NewService(
		db,
		WithClock(func() time.Time { return now }),
		WithProcessingInputResolver(resolver),
		WithAudioPreparer(audio),
	)
	started, err := service.StartEpisodeProcessing(
		context.Background(),
		processingStartRequest(episode.ID),
	)
	require.NoError(t, err)
	require.Equal(t, StepAudioPreparation, started.Run.CurrentStep)
	store, err := NewDiskArtifactStore(t.TempDir())
	require.NoError(t, err)
	engine, err := NewEngine(
		service,
		&fakeTranscriber{beginProgress: []TranscriptionProgress{{
			Status:        ExternalProgressCompleted,
			Checkpoint:    []byte(`{"state":"complete"}`),
			Transcript:    "# Transcript\n\n00:00 worker\n",
			SkillVersions: map[string]string{},
		}}},
		&fakeRuntime{result: RuntimeResult{
			EpisodeNotes:   "# Episode notes\n\nworker\n",
			RuntimeVersion: "fake-runtime-v1",
			PromptVersion:  "notes-v1",
			SkillVersions:  map[string]string{},
		}},
		store,
		nil,
	)
	require.NoError(t, err)
	worker, err := NewWorker(service, engine, audio, WorkerConfig{
		ScanInterval:         time.Second,
		ExternalPollInterval: 10 * time.Second,
		BatchSize:            4,
	})
	require.NoError(t, err)

	require.NoError(t, worker.RunOnce(context.Background()))
	require.Equal(t, 1, audio.claimCalls)
	require.Equal(t, 1, audio.prepareCalls)
	runs, err := service.ListEpisodeProcessingRuns(context.Background(), episode.ID)
	require.NoError(t, err)
	require.Len(t, runs, 1)
	require.Equal(t, models.ProcessingRunStatusCompleted, runs[0].Status)
}

func TestWorkerClosesAudioPreparationFailureOnOriginalRun(t *testing.T) {
	db := openProcessingTestDB(t)
	now := time.Date(2026, 8, 24, 14, 30, 0, 0, time.UTC)
	episode := createProcessingEpisode(t, db, true, "worker-audio-failure")
	asset := models.EpisodeAudioAsset{
		EpisodeID:    episode.ID,
		SourceDigest: strings.Repeat("b", 64),
		Status:       models.EpisodeAudioAssetStatusQueued,
		QueuedAt:     now,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	require.NoError(t, db.Create(&asset).Error)
	audio := &workerFakeAudio{
		assetIDs: []uint{asset.ID},
		claim: AudioClaim{
			AssetID:   asset.ID,
			EpisodeID: episode.ID,
			Token:     strings.Repeat("a", 64),
		},
		enqueueResult: AudioEnqueueResult{Asset: asset},
		prepareErr: newAudioStoreError(
			AudioErrorTooLarge,
			"episode audio exceeds the managed size limit",
			false,
		),
	}
	service, worker := newAudioPreparationWorker(t, db, now, audio)

	started, err := service.StartEpisodeProcessing(
		context.Background(),
		processingStartRequest(episode.ID),
	)
	require.NoError(t, err)
	require.NoError(t, worker.RunOnce(context.Background()))

	detail, err := service.GetProcessingRun(context.Background(), started.Run.ID)
	require.NoError(t, err)
	require.Equal(t, models.ProcessingRunStatusFailed, detail.Run.Status)
	require.Equal(t, AudioErrorTooLarge, detail.Run.ErrorCode)
	require.False(t, detail.Run.ErrorRetryable)
	runs, err := service.ListEpisodeProcessingRuns(context.Background(), episode.ID)
	require.NoError(t, err)
	require.Len(t, runs, 1)
}

func TestWorkerReconcilesReadyAndFailedAudioAfterRestart(t *testing.T) {
	t.Run("ready", func(t *testing.T) {
		db := openProcessingTestDB(t)
		now := time.Date(2026, 8, 24, 15, 0, 0, 0, time.UTC)
		episode := createProcessingEpisode(t, db, true, "worker-restart-ready")
		asset := models.EpisodeAudioAsset{
			EpisodeID:    episode.ID,
			SourceDigest: strings.Repeat("b", 64),
			Status:       models.EpisodeAudioAssetStatusReady,
			QueuedAt:     now,
			CreatedAt:    now,
			UpdatedAt:    now,
		}
		require.NoError(t, db.Create(&asset).Error)
		audio := &workerFakeAudio{
			enqueueResult: AudioEnqueueResult{Asset: asset},
			ready:         true,
		}
		service, worker := newAudioPreparationWorker(t, db, now, audio)
		started, err := service.startAudioPreparationRun(
			context.Background(),
			processingStartRequest(episode.ID),
			audio.enqueueResult,
			nil,
		)
		require.NoError(t, err)

		require.NoError(t, worker.RunOnce(context.Background()))

		detail, err := service.GetProcessingRun(context.Background(), started.Run.ID)
		require.NoError(t, err)
		require.Equal(t, models.ProcessingRunStatusCompleted, detail.Run.Status)
		require.Equal(t, strings.Repeat("a", 64), detail.Run.AudioDigest)
		require.Zero(t, audio.prepareCalls)
	})

	t.Run("failed", func(t *testing.T) {
		db := openProcessingTestDB(t)
		now := time.Date(2026, 8, 24, 15, 30, 0, 0, time.UTC)
		episode := createProcessingEpisode(t, db, true, "worker-restart-failed")
		asset := models.EpisodeAudioAsset{
			EpisodeID:    episode.ID,
			SourceDigest: strings.Repeat("b", 64),
			Status:       models.EpisodeAudioAssetStatusFailed,
			ErrorCode:    AudioErrorTooLarge,
			ErrorMessage: "episode audio exceeds the managed size limit",
			QueuedAt:     now,
			CreatedAt:    now,
			UpdatedAt:    now,
		}
		require.NoError(t, db.Create(&asset).Error)
		audio := &workerFakeAudio{
			enqueueResult: AudioEnqueueResult{Asset: asset},
			resolveErr: newAudioStoreError(
				AudioErrorTooLarge,
				"episode audio exceeds the managed size limit",
				false,
			),
		}
		service, worker := newAudioPreparationWorker(t, db, now, audio)
		started, err := service.startAudioPreparationRun(
			context.Background(),
			processingStartRequest(episode.ID),
			audio.enqueueResult,
			nil,
		)
		require.NoError(t, err)

		require.NoError(t, worker.RunOnce(context.Background()))

		detail, err := service.GetProcessingRun(context.Background(), started.Run.ID)
		require.NoError(t, err)
		require.Equal(t, models.ProcessingRunStatusFailed, detail.Run.Status)
		require.Equal(t, AudioErrorTooLarge, detail.Run.ErrorCode)
		require.False(t, detail.Run.ErrorRetryable)
		require.Zero(t, audio.prepareCalls)
	})
}

func newAudioPreparationWorker(
	t *testing.T,
	db *gorm.DB,
	now time.Time,
	audio *workerFakeAudio,
) (*Service, *Worker) {
	t.Helper()
	resolver := &staticProcessingInputResolver{
		input: ProcessingInput{PipelineVersion: "pipeline-v1"},
		err:   errors.New("audio not ready"),
	}
	service := NewService(
		db,
		WithClock(func() time.Time { return now }),
		WithProcessingInputResolver(resolver),
		WithAudioPreparer(audio),
	)
	store, err := NewDiskArtifactStore(t.TempDir())
	require.NoError(t, err)
	engine, err := NewEngine(
		service,
		&fakeTranscriber{beginProgress: []TranscriptionProgress{{
			Status:        ExternalProgressCompleted,
			Checkpoint:    []byte(`{"state":"complete"}`),
			Transcript:    "# Transcript\n\n00:00 worker\n",
			SkillVersions: map[string]string{},
		}}},
		&fakeRuntime{result: RuntimeResult{
			EpisodeNotes:   "# Episode notes\n\nworker\n",
			RuntimeVersion: "fake-runtime-v1",
			PromptVersion:  "notes-v1",
			SkillVersions:  map[string]string{},
		}},
		store,
		nil,
	)
	require.NoError(t, err)
	worker, err := NewWorker(service, engine, audio, WorkerConfig{
		ScanInterval:         time.Second,
		ExternalPollInterval: 10 * time.Second,
		BatchSize:            4,
	})
	require.NoError(t, err)
	return service, worker
}

type workerFakeAudio struct {
	assetIDs      []uint
	claim         AudioClaim
	enqueueResult AudioEnqueueResult
	enqueueErr    error
	prepareErr    error
	resolveErr    error
	claimCalls    int
	prepareCalls  int
	ready         bool
}

func (f *workerFakeAudio) Enqueue(
	context.Context,
	uint,
) (AudioEnqueueResult, error) {
	return f.enqueueResult, f.enqueueErr
}

func (f *workerFakeAudio) ListClaimable(
	context.Context,
	int,
) ([]uint, error) {
	output := append([]uint(nil), f.assetIDs...)
	f.assetIDs = nil
	return output, nil
}

func (f *workerFakeAudio) Claim(
	context.Context,
	uint,
) (AudioClaim, bool, error) {
	f.claimCalls++
	return f.claim, true, nil
}

func (f *workerFakeAudio) Prepare(
	context.Context,
	AudioClaim,
) (ReadyAudio, error) {
	f.prepareCalls++
	if f.prepareErr != nil {
		return ReadyAudio{}, f.prepareErr
	}
	f.ready = true
	return ReadyAudio{
		Path:            "/managed/audio.mp3",
		SHA256:          strings.Repeat("a", 64),
		SizeBytes:       5,
		DurationSeconds: 60,
		MediaType:       "audio/mpeg",
	}, nil
}

func (f *workerFakeAudio) GetReady(
	context.Context,
	uint,
) (models.EpisodeAudioAsset, error) {
	return models.EpisodeAudioAsset{}, nil
}

func (f *workerFakeAudio) ResolveReadyAudio(
	context.Context,
	uint,
) (ReadyAudio, error) {
	if f.resolveErr != nil {
		return ReadyAudio{}, f.resolveErr
	}
	if !f.ready {
		return ReadyAudio{}, newAudioStoreError(
			AudioErrorNotReady,
			"managed episode audio is not ready",
			true,
		)
	}
	return ReadyAudio{
		Path:            "/managed/audio.mp3",
		SHA256:          strings.Repeat("a", 64),
		SizeBytes:       5,
		DurationSeconds: 60,
		MediaType:       "audio/mpeg",
	}, nil
}
