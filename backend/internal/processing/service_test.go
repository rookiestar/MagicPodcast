package processing

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"magicpodcast/internal/database"
	"magicpodcast/internal/models"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestServiceStartIsIdempotentAndSeparateFromConsumption(t *testing.T) {
	db := openProcessingTestDB(t)
	now := time.Date(2026, 8, 24, 9, 0, 0, 0, time.UTC)
	service := newProcessingService(db, WithClock(func() time.Time { return now }))
	episode := createProcessingEpisode(t, db, true, "idempotent")
	request := processingStartRequest(episode.ID)

	first, err := service.StartEpisodeProcessing(context.Background(), request)
	require.NoError(t, err)
	require.False(t, first.ReusedActive)
	require.Equal(t, models.ProcessingRunStatusQueued, first.Run.Status)

	second, err := service.StartEpisodeProcessing(context.Background(), request)
	require.NoError(t, err)
	require.True(t, second.ReusedActive)
	require.Equal(t, first.Run.ID, second.Run.ID)

	cancelled, err := service.CancelProcessingRun(context.Background(), first.Run.ID)
	require.NoError(t, err)
	require.Equal(t, models.ProcessingRunStatusCancelled, cancelled.Status)
	require.NotNil(t, cancelled.CancelledAt)

	cancelledAgain, err := service.CancelProcessingRun(context.Background(), first.Run.ID)
	require.NoError(t, err)
	require.Equal(t, cancelled.ID, cancelledAgain.ID)
	require.Equal(t, models.ProcessingRunStatusCancelled, cancelledAgain.Status)

	var decision models.EpisodeTriageDecision
	require.NoError(t, db.Where("episode_id = ?", episode.ID).First(&decision).Error)
	require.NotNil(t, decision.QueueState)
	require.Equal(t, models.QueueStateFocus, *decision.QueueState)
	require.ErrorContains(
		t,
		db.Model(&models.EpisodeProcessingRun{ID: first.Run.ID}).
			Update("status", models.ProcessingRunStatusRunning).Error,
		"terminal processing run status is immutable",
	)
}

func TestServiceCancellationClearsPreviousErrorAndKeepsCancellationNotice(t *testing.T) {
	db := openProcessingTestDB(t)
	service := newProcessingService(db)
	episode := createProcessingEpisode(t, db, true, "cancellation-notice")
	started, err := service.StartEpisodeProcessing(
		context.Background(),
		processingStartRequest(episode.ID),
	)
	require.NoError(t, err)
	require.NoError(t, db.Model(&models.EpisodeProcessingRun{}).
		Where("id = ?", started.Run.ID).
		Updates(map[string]any{
			"error_code":      "stale_error",
			"error_message":   "stale error message",
			"error_retryable": true,
		}).Error)

	cancelled, err := service.CancelProcessingRun(context.Background(), started.Run.ID)
	require.NoError(t, err)
	require.Equal(t, models.ProcessingRunStatusCancelled, cancelled.Status)
	require.Empty(t, cancelled.ErrorCode)
	require.Empty(t, cancelled.ErrorMessage)
	require.False(t, cancelled.ErrorRetryable)

	noticed, err := service.recordCancellationNotice(
		context.Background(),
		started.Run.ID,
		cancellationExternalResultUnknown,
		"已取消本机加工；外部转写状态无法确认，任务可能继续。",
	)
	require.NoError(t, err)
	require.Equal(t, models.ProcessingRunStatusCancelled, noticed.Status)
	require.Equal(t, cancellationExternalResultUnknown, noticed.ErrorCode)
	require.Contains(t, noticed.ErrorMessage, "外部转写状态无法确认")
	require.False(t, noticed.ErrorRetryable)
}

func TestServiceDuplicateStartReturnsActiveRunAfterEpisodeLeavesFocus(t *testing.T) {
	db := openProcessingTestDB(t)
	service := newProcessingService(db)
	episode := createProcessingEpisode(t, db, true, "active-after-focus")
	request := processingStartRequest(episode.ID)

	first, err := service.StartEpisodeProcessing(context.Background(), request)
	require.NoError(t, err)

	someday := models.QueueStateSomeday
	require.NoError(t, db.Model(&models.EpisodeTriageDecision{}).
		Where("episode_id = ?", episode.ID).
		Update("queue_state", someday).Error)

	reused, err := service.StartEpisodeProcessing(context.Background(), request)
	require.NoError(t, err)
	require.True(t, reused.ReusedActive)
	require.Equal(t, first.Run.ID, reused.Run.ID)

	_, err = service.CancelProcessingRun(context.Background(), first.Run.ID)
	require.NoError(t, err)
	var decision models.EpisodeTriageDecision
	require.NoError(t, db.Where("episode_id = ?", episode.ID).First(&decision).Error)
	require.NotNil(t, decision.QueueState)
	require.Equal(t, models.QueueStateSomeday, *decision.QueueState)
	_, err = service.StartEpisodeProcessing(context.Background(), request)
	require.ErrorIs(t, err, ErrEpisodeNotFocused)
}

func TestServiceScheduledAndManualTriggersShareOneStartContract(t *testing.T) {
	db := openProcessingTestDB(t)
	service := newProcessingService(db)
	episode := createProcessingEpisode(t, db, true, "shared-start-contract")
	scheduledRequest := processingStartRequest(episode.ID)
	scheduledRequest.TriggerSource = models.ProcessingTriggerScheduled

	scheduled, err := service.StartEpisodeProcessing(context.Background(), scheduledRequest)
	require.NoError(t, err)
	require.Equal(t, models.ProcessingTriggerScheduled, scheduled.Run.TriggerSource)

	manualRequest := processingStartRequest(episode.ID)
	manual, err := service.StartEpisodeProcessing(context.Background(), manualRequest)
	require.NoError(t, err)
	require.True(t, manual.ReusedActive)
	require.Equal(t, scheduled.Run.ID, manual.Run.ID)
	require.Equal(t, models.ProcessingTriggerScheduled, manual.Run.TriggerSource)
}

func TestServiceResolvesAuthoritativeInputOnlyForNewRun(t *testing.T) {
	db := openProcessingTestDB(t)
	service, resolver := newProcessingServiceWithResolver(db)
	episode := createProcessingEpisode(t, db, true, "authoritative-input")
	request := processingStartRequest(episode.ID)

	first, err := service.StartEpisodeProcessing(context.Background(), request)
	require.NoError(t, err)
	require.Equal(t, strings.Repeat("a", 64), first.Run.AudioDigest)
	require.Equal(t, "pipeline-v1", first.Run.PipelineVersion)
	require.Equal(t, 1, resolver.CallCount())

	resolver.SetError(errors.New("private resolver failure"))
	reused, err := service.StartEpisodeProcessing(context.Background(), request)
	require.NoError(t, err)
	require.True(t, reused.ReusedActive)
	require.Equal(t, first.Run.ID, reused.Run.ID)
	require.Equal(t, 1, resolver.CallCount())

	_, err = service.CancelProcessingRun(context.Background(), first.Run.ID)
	require.NoError(t, err)
	_, err = service.StartEpisodeProcessing(context.Background(), request)
	require.ErrorIs(t, err, ErrProcessingInputUnavailable)
	require.Equal(t, 2, resolver.CallCount())
}

func TestServiceScheduledStartClassifiesOnlyExpectedReadyAudioErrorsAsSkips(t *testing.T) {
	for _, testCase := range []struct {
		name             string
		resolverErr      error
		want             error
		preservedErrCode string
	}{
		{
			name:        "not ready",
			resolverErr: newAudioStoreError(AudioErrorNotReady, "managed episode audio is not ready", true),
			want:        ErrProcessingInputUnavailable,
		},
		{
			name:        "missing episode",
			resolverErr: newAudioStoreError(AudioErrorAssetNotFound, "episode was not found", false),
			want:        ErrEpisodeNotFound,
		},
		{
			name:             "storage failure",
			resolverErr:      newAudioStoreError(AudioErrorStorageFailed, "managed audio state is unavailable", true),
			preservedErrCode: AudioErrorStorageFailed,
		},
		{
			name:             "invalid ready file",
			resolverErr:      newAudioStoreError(AudioErrorReadyFileInvalid, "managed episode audio file is unavailable", true),
			preservedErrCode: AudioErrorReadyFileInvalid,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			db := openProcessingTestDB(t)
			service, resolver := newProcessingServiceWithResolver(db)
			episode := createProcessingEpisode(t, db, true, "scheduled-ready-"+testCase.name)
			resolver.SetError(testCase.resolverErr)

			_, err := service.StartEpisodeProcessing(context.Background(), StartRequest{
				EpisodeID:         episode.ID,
				TriggerSource:     models.ProcessingTriggerScheduled,
				RequireReadyAudio: true,
			})
			require.Error(t, err)
			if testCase.want != nil {
				require.ErrorIs(t, err, testCase.want)
				return
			}
			require.False(t, errors.Is(err, ErrProcessingInputUnavailable))
			var audioErr *AudioStoreError
			require.True(t, errors.As(err, &audioErr))
			require.Equal(t, testCase.preservedErrCode, audioErr.Code)
		})
	}
}

func TestServiceQueuesManagedAudioInsideOneCancelableRun(t *testing.T) {
	db := openProcessingTestDB(t)
	resolver := &staticProcessingInputResolver{
		input: ProcessingInput{PipelineVersion: "pipeline-v1"},
		err:   errors.New("not ready"),
	}
	audio := &workerFakeAudio{enqueueResult: AudioEnqueueResult{
		Asset: models.EpisodeAudioAsset{
			ID:           88,
			SourceDigest: strings.Repeat("b", 64),
			Status:       models.EpisodeAudioAssetStatusQueued,
		},
	}}
	service := NewService(
		db,
		WithProcessingInputResolver(resolver),
		WithAudioPreparer(audio),
	)
	episode := createProcessingEpisode(t, db, true, "audio-prepare")
	audio.enqueueResult.Asset.EpisodeID = episode.ID

	result, err := service.StartEpisodeProcessing(
		context.Background(),
		processingStartRequest(episode.ID),
	)
	require.NoError(t, err)
	require.True(t, result.PreparingAudio)
	require.NotNil(t, result.AudioAsset)
	require.Equal(t, uint(88), result.AudioAsset.ID)
	require.NotZero(t, result.Run.ID)
	require.Equal(t, models.ProcessingRunStatusQueued, result.Run.Status)
	require.Equal(t, StepAudioPreparation, result.Run.CurrentStep)
	require.Empty(t, result.Run.AudioDigest)

	duplicate, err := service.StartEpisodeProcessing(
		context.Background(),
		processingStartRequest(episode.ID),
	)
	require.NoError(t, err)
	require.True(t, duplicate.ReusedActive)
	require.Equal(t, result.Run.ID, duplicate.Run.ID)

	cancelled, err := service.CancelProcessingRun(context.Background(), result.Run.ID)
	require.NoError(t, err)
	require.Equal(t, models.ProcessingRunStatusCancelled, cancelled.Status)
	var runCount int64
	require.NoError(t, db.Model(&models.EpisodeProcessingRun{}).
		Where("episode_id = ?", episode.ID).
		Count(&runCount).Error)
	require.EqualValues(t, 1, runCount)
}

func TestServiceRetriesAudioFailureThroughPreparationRun(t *testing.T) {
	db := openProcessingTestDB(t)
	resolver := &staticProcessingInputResolver{
		input: ProcessingInput{PipelineVersion: "pipeline-v1"},
		err:   errors.New("not ready"),
	}
	audio := &workerFakeAudio{enqueueResult: AudioEnqueueResult{
		Asset: models.EpisodeAudioAsset{
			ID:           89,
			SourceDigest: strings.Repeat("b", 64),
			Status:       models.EpisodeAudioAssetStatusQueued,
		},
	}}
	service := NewService(
		db,
		WithProcessingInputResolver(resolver),
		WithAudioPreparer(audio),
	)
	episode := createProcessingEpisode(t, db, true, "audio-retry")
	audio.enqueueResult.Asset.EpisodeID = episode.ID

	started, err := service.StartEpisodeProcessing(
		context.Background(),
		processingStartRequest(episode.ID),
	)
	require.NoError(t, err)
	require.NoError(t, service.failAudioPreparation(
		context.Background(),
		episode.ID,
		AudioErrorSourceUnavailable,
		"episode audio source is unavailable",
		true,
	))

	audio.enqueueResult.Asset.ID = 90
	retry, err := service.RetryProcessingRun(context.Background(), started.Run.ID)
	require.NoError(t, err)
	require.True(t, retry.PreparingAudio)
	require.Equal(t, StepAudioPreparation, retry.Run.CurrentStep)
	require.NotNil(t, retry.Run.PreviousRunID)
	require.Equal(t, started.Run.ID, *retry.Run.PreviousRunID)
	require.NotNil(t, retry.AudioAsset)
	require.Equal(t, uint(90), retry.AudioAsset.ID)
}

func TestServiceReusesSuccessfulArtifactAndForceLinksNewRun(t *testing.T) {
	db := openProcessingTestDB(t)
	now := time.Date(2026, 8, 24, 9, 30, 0, 0, time.UTC)
	service, resolver := newProcessingServiceWithResolver(
		db,
		WithClock(func() time.Time { return now }),
	)
	episode := createProcessingEpisode(t, db, true, "force")
	request := processingStartRequest(episode.ID)

	started, err := service.StartEpisodeProcessing(context.Background(), request)
	require.NoError(t, err)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.EpisodeProcessingRun{}).
			Where("id = ?", started.Run.ID).
			Updates(map[string]any{
				"status":      models.ProcessingRunStatusCompleted,
				"finished_at": now,
				"updated_at":  now,
			}).Error; err != nil {
			return err
		}
		return tx.Create(&models.EpisodeArtifactSet{
			RunID:            started.Run.ID,
			EpisodeID:        episode.ID,
			PipelineVersion:  started.Run.PipelineVersion,
			RootPath:         "/managed/internal/path",
			ManifestPath:     "manifest.json",
			ManifestSHA256:   strings.Repeat("1", 64),
			TranscriptSHA256: strings.Repeat("2", 64),
			NotesSHA256:      strings.Repeat("3", 64),
			IsCurrent:        true,
			CreatedAt:        now,
		}).Error
	}))

	reused, err := service.StartEpisodeProcessing(context.Background(), request)
	require.NoError(t, err)
	require.True(t, reused.ReusedSuccessful)
	require.Equal(t, started.Run.ID, reused.Run.ID)

	resolver.Set(ProcessingInput{
		AudioDigest:     strings.Repeat("b", 64),
		PipelineVersion: "pipeline-v1",
	})
	changed, err := service.StartEpisodeProcessing(context.Background(), request)
	require.NoError(t, err)
	require.False(t, changed.ReusedSuccessful)
	require.NotEqual(t, started.Run.ProcessingKey, changed.Run.ProcessingKey)
	_, err = service.CancelProcessingRun(context.Background(), changed.Run.ID)
	require.NoError(t, err)

	resolver.Set(ProcessingInput{
		AudioDigest:     strings.Repeat("a", 64),
		PipelineVersion: "pipeline-v1",
	})
	request.Force = true
	forced, err := service.StartEpisodeProcessing(context.Background(), request)
	require.NoError(t, err)
	require.False(t, forced.ReusedSuccessful)
	require.NotEqual(t, started.Run.ID, forced.Run.ID)
	require.NotNil(t, forced.Run.PreviousRunID)
	require.Equal(t, started.Run.ID, *forced.Run.PreviousRunID)
	require.Equal(t, models.ProcessingRunStatusQueued, forced.Run.Status)
}

func TestServiceConcurrentStartCreatesOneActiveRun(t *testing.T) {
	db := openProcessingTestDB(t)
	service := newProcessingService(db)
	episode := createProcessingEpisode(t, db, true, "concurrent")
	request := processingStartRequest(episode.ID)

	const callers = 12
	results := make(chan StartResult, callers)
	errs := make(chan error, callers)
	var wait sync.WaitGroup
	for index := 0; index < callers; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			result, err := service.StartEpisodeProcessing(context.Background(), request)
			results <- result
			errs <- err
		}()
	}
	wait.Wait()
	close(results)
	close(errs)

	for err := range errs {
		require.NoError(t, err)
	}
	var runID uint
	for result := range results {
		if runID == 0 {
			runID = result.Run.ID
		}
		require.Equal(t, runID, result.Run.ID)
	}
	var activeCount int64
	require.NoError(t, db.Model(&models.EpisodeProcessingRun{}).
		Where("episode_id = ? AND status IN ?", episode.ID, models.ProcessingRunActiveStatuses).
		Count(&activeCount).Error)
	require.Equal(t, int64(1), activeCount)
}

func TestServiceRetryCopiesSafeCheckpointAndBlocksUnknownResult(t *testing.T) {
	db := openProcessingTestDB(t)
	now := time.Date(2026, 8, 24, 11, 0, 0, 0, time.UTC)
	service := newProcessingService(db, WithClock(func() time.Time { return now }))
	episode := createProcessingEpisode(t, db, true, "safe-retry")
	source := startProcessingRun(t, service, episode.ID)
	state := `{"version":1,"phase":"transcript_stored"}`
	sum := sha256.Sum256([]byte(state))
	require.NoError(t, db.Create(&models.ProcessingCheckpoint{
		RunID:          source.ID,
		Step:           StepTranscription,
		Adapter:        "fake-minutes",
		AdapterVersion: "fake-minutes-v1",
		Status:         ExternalProgressCompleted,
		StateJSON:      state,
		StateHash:      hex.EncodeToString(sum[:]),
		CreatedAt:      now,
		UpdatedAt:      now,
	}).Error)
	require.NoError(t, db.Model(&models.EpisodeProcessingRun{}).
		Where("id = ?", source.ID).
		Updates(map[string]any{
			"status":          models.ProcessingRunStatusFailed,
			"error_code":      "runtime_unavailable",
			"error_message":   "runtime unavailable",
			"error_retryable": true,
			"finished_at":     now,
			"updated_at":      now,
		}).Error)

	retry, err := service.RetryProcessingRun(context.Background(), source.ID)
	require.NoError(t, err)
	require.Equal(t, models.ProcessingRunStatusQueued, retry.Run.Status)
	require.NotNil(t, retry.Run.PreviousRunID)
	require.Equal(t, source.ID, *retry.Run.PreviousRunID)
	var copied models.ProcessingCheckpoint
	require.NoError(t, db.Where(
		"run_id = ? AND step = ?",
		retry.Run.ID,
		StepTranscription,
	).First(&copied).Error)
	require.Equal(t, state, copied.StateJSON)
	require.Equal(t, ExternalProgressCompleted, copied.Status)

	unknownEpisode := createProcessingEpisode(t, db, true, "unknown-retry")
	unknown := startProcessingRun(t, service, unknownEpisode.ID)
	require.NoError(t, db.Model(&models.EpisodeProcessingRun{}).
		Where("id = ?", unknown.ID).
		Updates(map[string]any{
			"status":          models.ProcessingRunStatusFailed,
			"error_code":      "lark_drive_result_unknown",
			"error_message":   "unknown",
			"error_retryable": false,
			"finished_at":     now,
			"updated_at":      now,
		}).Error)
	_, err = service.RetryProcessingRun(context.Background(), unknown.ID)
	require.ErrorIs(t, err, ErrRetryUnsafe)
}

func TestServiceRetryNativeMinutesCheckpointPolicy(t *testing.T) {
	testCases := []struct {
		name                 string
		errorCode            string
		errorRetryable       bool
		restartTranscription bool
	}{
		{
			name:                 "invalid timeline",
			errorCode:            "transcript_timeline_invalid",
			restartTranscription: true,
		},
		{
			name:                 "stored transcript unavailable",
			errorCode:            "stored_transcript_unavailable",
			restartTranscription: true,
		},
		{
			name:                 "stored summary unavailable",
			errorCode:            "stored_summary_unavailable",
			restartTranscription: true,
		},
		{
			name:                 "artifact exceeds public read limit",
			errorCode:            artifactPublicReadLimitExceededCode,
			restartTranscription: true,
		},
		{
			name:                 "artifact text invalid",
			errorCode:            artifactTextInvalidCode,
			restartTranscription: true,
		},
		{
			name:           "other retryable failure",
			errorCode:      "runtime_unavailable",
			errorRetryable: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			db := openProcessingTestDB(t)
			now := time.Date(2026, 8, 29, 13, 0, 0, 0, time.UTC)
			service, resolver := newProcessingServiceWithResolver(
				db,
				WithClock(func() time.Time { return now }),
			)
			resolver.Set(ProcessingInput{
				AudioDigest:     strings.Repeat("a", 64),
				PipelineVersion: NativeMinutesPipelineVersion,
			})
			episode := createProcessingEpisode(
				t,
				db,
				true,
				"native-minutes-retry-"+strings.ReplaceAll(testCase.name, " ", "-"),
			)
			source := startProcessingRun(t, service, episode.ID)
			state := `{"version":1,"phase":"transcript_stored"}`
			sum := sha256.Sum256([]byte(state))
			require.NoError(t, db.Create(&models.ProcessingCheckpoint{
				RunID:          source.ID,
				Step:           StepTranscription,
				Adapter:        "feishu-minutes",
				AdapterVersion: "feishu-minutes-v1",
				Status:         ExternalProgressCompleted,
				StateJSON:      state,
				StateHash:      hex.EncodeToString(sum[:]),
				CreatedAt:      now,
				UpdatedAt:      now,
			}).Error)
			require.NoError(t, db.Model(&models.EpisodeProcessingRun{}).
				Where("id = ?", source.ID).
				Updates(map[string]any{
					"status":          models.ProcessingRunStatusFailed,
					"error_code":      testCase.errorCode,
					"error_message":   testCase.name,
					"error_retryable": testCase.errorRetryable,
					"finished_at":     now,
					"updated_at":      now,
				}).Error)

			retry, err := service.RetryProcessingRun(context.Background(), source.ID)
			require.NoError(t, err)
			require.Equal(t, models.ProcessingRunStatusQueued, retry.Run.Status)
			require.Equal(t, NativeMinutesPipelineVersion, retry.Run.PipelineVersion)
			require.NotNil(t, retry.Run.PreviousRunID)
			require.Equal(t, source.ID, *retry.Run.PreviousRunID)

			var sourceCheckpoint models.ProcessingCheckpoint
			require.NoError(t, db.Where(
				"run_id = ? AND step = ?",
				source.ID,
				StepTranscription,
			).First(&sourceCheckpoint).Error)
			require.Equal(t, state, sourceCheckpoint.StateJSON)

			var retryCheckpoints []models.ProcessingCheckpoint
			require.NoError(t, db.Where(
				"run_id = ? AND step = ?",
				retry.Run.ID,
				StepTranscription,
			).Find(&retryCheckpoints).Error)
			if testCase.restartTranscription {
				require.Empty(t, retryCheckpoints)
			} else {
				require.Len(t, retryCheckpoints, 1)
				require.Equal(t, state, retryCheckpoints[0].StateJSON)
				require.Equal(t, ExternalProgressCompleted, retryCheckpoints[0].Status)
			}
		})
	}
}

func TestServiceRecoveryClosesInterruptedAndKeepsRecoverableRuns(t *testing.T) {
	db := openProcessingTestDB(t)
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	service := newProcessingService(db, WithClock(func() time.Time { return now }))

	queued := startProcessingRun(t, service, createProcessingEpisode(t, db, true, "queued").ID)
	running := startProcessingRun(t, service, createProcessingEpisode(t, db, true, "running").ID)
	waiting := startProcessingRun(t, service, createProcessingEpisode(t, db, true, "waiting").ID)
	missing := startProcessingRun(t, service, createProcessingEpisode(t, db, true, "missing").ID)
	corrupt := startProcessingRun(t, service, createProcessingEpisode(t, db, true, "corrupt").ID)
	versionless := startProcessingRun(t, service, createProcessingEpisode(t, db, true, "versionless").ID)

	old := now.Add(-2 * time.Hour)
	require.NoError(t, db.Model(&models.EpisodeProcessingRun{}).
		Where("id = ?", running.ID).
		Updates(map[string]any{
			"status":     models.ProcessingRunStatusRunning,
			"updated_at": now,
		}).Error)
	for _, runID := range []uint{waiting.ID, missing.ID, corrupt.ID, versionless.ID} {
		require.NoError(t, db.Model(&models.EpisodeProcessingRun{}).
			Where("id = ?", runID).
			Updates(map[string]any{
				"status":     models.ProcessingRunStatusWaitingExternal,
				"updated_at": old,
			}).Error)
	}
	checkpointState := `{"minute":"known"}`
	checkpointHash := sha256.Sum256([]byte(checkpointState))
	require.NoError(t, db.Create(&models.ProcessingCheckpoint{
		RunID:          waiting.ID,
		Step:           StepTranscription,
		Adapter:        "fake-minutes",
		AdapterVersion: "fake-minutes-v1",
		Status:         ExternalProgressWaiting,
		StateJSON:      checkpointState,
		StateHash:      hex.EncodeToString(checkpointHash[:]),
		CreatedAt:      old,
		UpdatedAt:      old,
	}).Error)
	require.NoError(t, db.Create(&models.ProcessingCheckpoint{
		RunID:          corrupt.ID,
		Step:           StepTranscription,
		Adapter:        "fake-minutes",
		AdapterVersion: "fake-minutes-v1",
		Status:         ExternalProgressWaiting,
		StateJSON:      `{"minute":"tampered"}`,
		StateHash:      strings.Repeat("4", 64),
		CreatedAt:      old,
		UpdatedAt:      old,
	}).Error)
	versionlessState := `{"minute":"known-without-version"}`
	versionlessHash := sha256.Sum256([]byte(versionlessState))
	require.NoError(t, db.Create(&models.ProcessingCheckpoint{
		RunID:     versionless.ID,
		Step:      StepTranscription,
		Adapter:   "fake-minutes",
		Status:    ExternalProgressWaiting,
		StateJSON: versionlessState,
		StateHash: hex.EncodeToString(versionlessHash[:]),
		CreatedAt: old,
		UpdatedAt: old,
	}).Error)

	recovery, err := service.RecoverNonTerminalRuns(context.Background(), now)
	require.NoError(t, err)
	require.ElementsMatch(t, []uint{queued.ID, waiting.ID}, recovery.RecoverableRunIDs)
	require.ElementsMatch(
		t,
		[]uint{running.ID, missing.ID, corrupt.ID, versionless.ID},
		recovery.FailedRunIDs,
	)

	runningDetail, err := service.GetProcessingRun(context.Background(), running.ID)
	require.NoError(t, err)
	require.Equal(t, models.ProcessingRunStatusFailed, runningDetail.Run.Status)
	require.Equal(t, "interrupted_by_restart", runningDetail.Run.ErrorCode)
	missingDetail, err := service.GetProcessingRun(context.Background(), missing.ID)
	require.NoError(t, err)
	require.Equal(t, "missing_external_checkpoint", missingDetail.Run.ErrorCode)
	corruptDetail, err := service.GetProcessingRun(context.Background(), corrupt.ID)
	require.NoError(t, err)
	require.Equal(t, "invalid_external_checkpoint", corruptDetail.Run.ErrorCode)
	versionlessDetail, err := service.GetProcessingRun(context.Background(), versionless.ID)
	require.NoError(t, err)
	require.Equal(t, "invalid_external_checkpoint", versionlessDetail.Run.ErrorCode)
	waitingDetail, err := service.GetProcessingRun(context.Background(), waiting.ID)
	require.NoError(t, err)
	require.Equal(t, models.ProcessingRunStatusWaitingExternal, waitingDetail.Run.Status)
}

func TestServiceRecoveryFailsClosedInterruptedKnowledgeDelivery(t *testing.T) {
	db := openProcessingTestDB(t)
	now := time.Date(2026, 8, 24, 12, 30, 0, 0, time.UTC)
	service := newProcessingService(db, WithClock(func() time.Time { return now }))
	episode := createProcessingEpisode(t, db, true, "delivery-recovery")
	run := startProcessingRun(t, service, episode.ID)
	require.NoError(t, db.Model(&models.EpisodeProcessingRun{}).
		Where("id = ?", run.ID).
		Updates(map[string]any{
			"status":      models.ProcessingRunStatusCompleted,
			"finished_at": now.Add(-time.Minute),
			"updated_at":  now.Add(-time.Minute),
		}).Error)
	artifact := models.EpisodeArtifactSet{
		RunID:            run.ID,
		EpisodeID:        episode.ID,
		PipelineVersion:  run.PipelineVersion,
		RootPath:         "/managed/artifacts",
		ManifestPath:     "manifest.json",
		ManifestSHA256:   strings.Repeat("1", 64),
		TranscriptSHA256: strings.Repeat("2", 64),
		NotesSHA256:      strings.Repeat("3", 64),
		IsCurrent:        true,
		CreatedAt:        now.Add(-time.Minute),
	}
	require.NoError(t, db.Create(&artifact).Error)
	delivery := models.KnowledgeDelivery{
		ArtifactSetID:  artifact.ID,
		Target:         "fake-knowledge",
		Destination:    "destination",
		AdapterVersion: "fake-v1",
		DeliveryKey:    strings.Repeat("4", 64),
		Status:         models.DeliveryStatusDelivering,
		AttemptCount:   1,
		CreatedAt:      now.Add(-time.Minute),
		UpdatedAt:      now.Add(-time.Minute),
	}
	require.NoError(t, db.Create(&delivery).Error)

	recovery, err := service.RecoverNonTerminalRuns(context.Background(), now)
	require.NoError(t, err)
	require.Empty(t, recovery.RecoverableRunIDs)
	require.Empty(t, recovery.FailedRunIDs)
	require.Equal(t, []uint{delivery.ID}, recovery.FailedDeliveryIDs)

	detail, err := service.GetProcessingRun(context.Background(), run.ID)
	require.NoError(t, err)
	require.Equal(t, models.ProcessingRunStatusCompleted, detail.Run.Status)
	require.Len(t, detail.Deliveries, 1)
	require.Equal(t, models.DeliveryStatusFailed, detail.Deliveries[0].Status)
	require.Equal(t, "external_result_unknown", detail.Deliveries[0].ErrorCode)
	require.False(t, detail.Deliveries[0].ErrorRetryable)
}

func TestServiceRejectsNonFocusAndInvalidProcessingKeyInput(t *testing.T) {
	db := openProcessingTestDB(t)
	service := newProcessingService(db)
	episode := createProcessingEpisode(t, db, false, "not-focus")
	request := processingStartRequest(episode.ID)

	_, err := service.StartEpisodeProcessing(context.Background(), request)
	require.ErrorIs(t, err, ErrEpisodeNotFocused)

	episode = createProcessingEpisode(t, db, true, "invalid-input")
	request = processingStartRequest(episode.ID)
	resolver := service.inputResolver.(*staticProcessingInputResolver)
	resolver.Set(ProcessingInput{
		AudioDigest:     "not-a-digest",
		PipelineVersion: "pipeline-v1",
	})
	_, err = service.StartEpisodeProcessing(context.Background(), request)
	require.ErrorIs(t, err, ErrProcessingInputUnavailable)
}

func openProcessingTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "processing.db")
	dsn := fmt.Sprintf("%s?_journal_mode=WAL&_foreign_keys=on&_busy_timeout=5000", path)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, database.ApplyMigrations(db))
	return db
}

func createProcessingEpisode(
	t *testing.T,
	db *gorm.DB,
	focused bool,
	suffix string,
) models.Episode {
	t.Helper()
	podcast := models.Podcast{
		Title:        "Processing " + suffix,
		FeedURL:      "https://example.com/" + suffix + ".xml",
		XYZID:        "processing-" + suffix,
		IsSubscribed: true,
	}
	require.NoError(t, db.Create(&podcast).Error)
	episode := models.Episode{
		PodcastID:     podcast.ID,
		Title:         "Episode " + suffix,
		GUID:          "processing-" + suffix,
		MediumURL:     "https://example.com/" + suffix + ".mp3",
		PublishedDate: time.Date(2026, 8, 24, 8, 0, 0, 0, time.UTC),
	}
	require.NoError(t, db.Create(&episode).Error)
	if focused {
		focus := models.QueueStateFocus
		now := time.Date(2026, 8, 24, 8, 30, 0, 0, time.UTC)
		require.NoError(t, db.Create(&models.EpisodeTriageDecision{
			EpisodeID:      episode.ID,
			State:          models.TriageStateShortlisted,
			DecidedAt:      now,
			QueueState:     &focus,
			QueueUpdatedAt: &now,
		}).Error)
	}
	return episode
}

func processingStartRequest(episodeID uint) StartRequest {
	return StartRequest{
		EpisodeID:     episodeID,
		TriggerSource: models.ProcessingTriggerManual,
	}
}

func startProcessingRun(
	t *testing.T,
	service *Service,
	episodeID uint,
) models.EpisodeProcessingRun {
	t.Helper()
	result, err := service.StartEpisodeProcessing(
		context.Background(),
		processingStartRequest(episodeID),
	)
	require.NoError(t, err)
	return result.Run
}

type staticProcessingInputResolver struct {
	mu        sync.Mutex
	input     ProcessingInput
	err       error
	callCount int
}

func (r *staticProcessingInputResolver) PipelineVersion() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.input.PipelineVersion
}

func (r *staticProcessingInputResolver) ResolveProcessingInput(
	_ context.Context,
	_ uint,
) (ProcessingInput, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.callCount++
	return r.input, r.err
}

func (r *staticProcessingInputResolver) Set(input ProcessingInput) {
	r.mu.Lock()
	r.input = input
	r.mu.Unlock()
}

func (r *staticProcessingInputResolver) SetError(err error) {
	r.mu.Lock()
	r.err = err
	r.mu.Unlock()
}

func (r *staticProcessingInputResolver) CallCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.callCount
}

func newProcessingService(db *gorm.DB, options ...ServiceOption) *Service {
	service, _ := newProcessingServiceWithResolver(db, options...)
	return service
}

func newProcessingServiceWithResolver(
	db *gorm.DB,
	options ...ServiceOption,
) (*Service, *staticProcessingInputResolver) {
	resolver := &staticProcessingInputResolver{input: ProcessingInput{
		AudioDigest:     strings.Repeat("a", 64),
		PipelineVersion: "pipeline-v1",
	}}
	allOptions := append(
		[]ServiceOption{WithProcessingInputResolver(resolver)},
		options...,
	)
	return NewService(db, allOptions...), resolver
}
