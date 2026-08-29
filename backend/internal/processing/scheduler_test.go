package processing

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"magicpodcast/internal/models"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestSchedulerSelectsFocusInPersistentOrderAndRecordsSkips(t *testing.T) {
	db := openProcessingTestDB(t)
	now := time.Date(2026, 8, 25, 3, 0, 0, 0, time.UTC)
	resolver := newEpisodeInputResolver()
	service := NewService(
		db,
		WithProcessingInputResolver(resolver),
		WithClock(func() time.Time { return now }),
	)
	scheduler := newTestScheduler(t, db, service, now, 1)

	active := createProcessingEpisode(t, db, true, "schedule-active")
	completed := createProcessingEpisode(t, db, true, "schedule-completed")
	noAudio := createProcessingEpisode(t, db, true, "schedule-no-audio")
	eligible := createProcessingEpisode(t, db, true, "schedule-eligible")
	for _, episode := range []models.Episode{active, completed, eligible} {
		resolver.SetReady(episode.ID)
	}
	resolver.SetError(noAudio.ID, newAudioStoreError(AudioErrorNotReady, "managed audio is not ready", true))
	setFocusPositions(t, db, active.ID, completed.ID, noAudio.ID, eligible.ID)

	activeRun, err := service.StartEpisodeProcessing(context.Background(), StartRequest{
		EpisodeID:     active.ID,
		TriggerSource: models.ProcessingTriggerManual,
	})
	require.NoError(t, err)

	completedRun, err := service.StartEpisodeProcessing(context.Background(), StartRequest{
		EpisodeID:     completed.ID,
		TriggerSource: models.ProcessingTriggerManual,
	})
	require.NoError(t, err)
	require.NoError(t, db.Model(&models.EpisodeProcessingRun{}).
		Where("id = ?", completedRun.Run.ID).
		Updates(map[string]any{
			"status":      models.ProcessingRunStatusCompleted,
			"finished_at": now,
			"updated_at":  now,
		}).Error)
	require.NoError(t, db.Create(&models.EpisodeArtifactSet{
		RunID:            completedRun.Run.ID,
		EpisodeID:        completed.ID,
		PipelineVersion:  "pipeline-v1",
		RootPath:         "/private/completed",
		ManifestPath:     "manifest.json",
		ManifestSHA256:   strings.Repeat("1", 64),
		TranscriptSHA256: strings.Repeat("2", 64),
		NotesSHA256:      strings.Repeat("3", 64),
		IsCurrent:        true,
		CreatedAt:        now,
	}).Error)

	detail, reused, err := scheduler.RunAt(context.Background(), now)
	require.NoError(t, err)
	require.False(t, reused)
	require.Equal(t, models.ProcessingScheduleRunStatusCompleted, detail.Run.Status)
	require.Equal(t, 4, detail.Run.CandidateCount)
	require.Equal(t, 1, detail.Run.StartedCount)
	require.Equal(t, 3, detail.Run.SkippedCount)
	require.Len(t, detail.Items, 4)
	require.Equal(t, []uint{active.ID, completed.ID, noAudio.ID, eligible.ID}, scheduleItemEpisodeIDs(detail.Items))
	require.Equal(t, scheduleSkipActiveRun, detail.Items[0].Reason)
	require.Equal(t, scheduleSkipCurrentArtifact, detail.Items[1].Reason)
	require.Equal(t, scheduleSkipAudioNotReady, detail.Items[2].Reason)
	require.Equal(t, models.ProcessingScheduleItemOutcomeStarted, detail.Items[3].Outcome)
	require.NotNil(t, detail.Items[3].ProcessingRunID)

	var scheduledRun models.EpisodeProcessingRun
	require.NoError(t, db.Where("id = ?", *detail.Items[3].ProcessingRunID).First(&scheduledRun).Error)
	require.Equal(t, models.ProcessingTriggerScheduled, scheduledRun.TriggerSource)
	require.Equal(t, detail.Run.ID, *scheduledRun.ScheduleRunID)
	var noAudioRuns int64
	require.NoError(t, db.Model(&models.EpisodeProcessingRun{}).Where("episode_id = ?", noAudio.ID).Count(&noAudioRuns).Error)
	require.Zero(t, noAudioRuns)
	require.NotZero(t, activeRun.Run.ID)
}

func TestSchedulerFailsUnexpectedReadyAudioError(t *testing.T) {
	db := openProcessingTestDB(t)
	now := time.Date(2026, 8, 25, 3, 10, 0, 0, time.UTC)
	resolver := newEpisodeInputResolver()
	service := NewService(
		db,
		WithProcessingInputResolver(resolver),
		WithClock(func() time.Time { return now }),
	)
	scheduler := newTestScheduler(t, db, service, now, 1)
	episode := createProcessingEpisode(t, db, true, "schedule-storage-failure")
	resolver.SetError(episode.ID, newAudioStoreError(
		AudioErrorStorageFailed,
		"managed audio state is unavailable",
		true,
	))
	setFocusPositions(t, db, episode.ID)

	detail, reused, err := scheduler.RunAt(context.Background(), now)
	require.NoError(t, err)
	require.False(t, reused)
	require.Equal(t, models.ProcessingScheduleRunStatusFailed, detail.Run.Status)
	require.Equal(t, "candidate_start_failed", detail.Run.ErrorCode)
	require.Len(t, detail.Items, 1)
	require.Equal(t, models.ProcessingScheduleItemOutcomeSkipped, detail.Items[0].Outcome)
	require.Equal(t, scheduleSkipStartFailed, detail.Items[0].Reason)

	var count int64
	require.NoError(t, db.Model(&models.EpisodeProcessingRun{}).Where("episode_id = ?", episode.ID).Count(&count).Error)
	require.Zero(t, count)
}

func TestSchedulerFinishRunUsesPersistedItemCounts(t *testing.T) {
	db := openProcessingTestDB(t)
	now := time.Date(2026, 8, 25, 3, 15, 0, 0, time.UTC)
	service := newProcessingService(db)
	scheduler := newTestScheduler(t, db, service, now, 1)
	scheduleRun, duplicate, err := scheduler.claimRun(context.Background(), now)
	require.NoError(t, err)
	require.False(t, duplicate)

	episode := createProcessingEpisode(t, db, true, "schedule-finish-persisted-count")
	require.NoError(t, db.Create(&models.ProcessingScheduleItem{
		ScheduleRunID: scheduleRun.ID,
		EpisodeID:     episode.ID,
		QueuePosition: 0,
		Outcome:       models.ProcessingScheduleItemOutcomeSkipped,
		Reason:        scheduleSkipNotFocused,
		CreatedAt:     now,
		UpdatedAt:     now,
	}).Error)

	// A worker can update a queued scheduled item after selection has counted it
	// as started. Finishing must use the durable item state, not that stale count.
	detail, _, err := scheduler.finishRun(
		context.Background(),
		scheduleRun.ID,
		models.ProcessingScheduleRunStatusCompleted,
		"",
		"",
		now,
	)
	require.NoError(t, err)
	require.Zero(t, detail.Run.StartedCount)
	require.Equal(t, 1, detail.Run.SkippedCount)
}

func TestSchedulerRecoveryMarksPendingCandidateAsInterrupted(t *testing.T) {
	db := openProcessingTestDB(t)
	now := time.Date(2026, 8, 25, 3, 20, 0, 0, time.UTC)
	scheduler := newTestScheduler(t, db, newProcessingService(db), now, 1)
	scheduleRun, duplicate, err := scheduler.claimRun(context.Background(), now)
	require.NoError(t, err)
	require.False(t, duplicate)

	episode := createProcessingEpisode(t, db, true, "schedule-pending-recovery")
	setFocusPositions(t, db, episode.ID)
	candidates, err := scheduler.planCandidates(context.Background(), scheduleRun.ID)
	require.NoError(t, err)
	require.Len(t, candidates, 1)

	recoveredAt := now.Add(time.Minute)
	require.NoError(t, scheduler.RecoverIncompleteRuns(context.Background(), recoveredAt))

	var item models.ProcessingScheduleItem
	require.NoError(t, db.Where("schedule_run_id = ? AND episode_id = ?", scheduleRun.ID, episode.ID).First(&item).Error)
	require.Equal(t, models.ProcessingScheduleItemOutcomeSkipped, item.Outcome)
	require.Equal(t, scheduleSkipSelectionRestart, item.Reason)
}

func TestSchedulerFailureMarksPendingCandidateAsInterrupted(t *testing.T) {
	db := openProcessingTestDB(t)
	now := time.Date(2026, 8, 25, 3, 25, 0, 0, time.UTC)
	scheduler := newTestScheduler(t, db, newProcessingService(db), now, 1)
	scheduleRun, duplicate, err := scheduler.claimRun(context.Background(), now)
	require.NoError(t, err)
	require.False(t, duplicate)

	episode := createProcessingEpisode(t, db, true, "schedule-pending-failure")
	setFocusPositions(t, db, episode.ID)
	_, err = scheduler.planCandidates(context.Background(), scheduleRun.ID)
	require.NoError(t, err)

	cause := errors.New("schedule selection interrupted")
	detail, _, err := scheduler.failRun(
		context.Background(),
		scheduleRun.ID,
		"schedule_interrupted",
		"schedule selection was interrupted",
		cause,
	)
	require.ErrorIs(t, err, cause)
	require.Equal(t, models.ProcessingScheduleRunStatusFailed, detail.Run.Status)
	require.Zero(t, detail.Run.StartedCount)
	require.Equal(t, 1, detail.Run.SkippedCount)

	var item models.ProcessingScheduleItem
	require.NoError(t, db.Where("schedule_run_id = ? AND episode_id = ?", scheduleRun.ID, episode.ID).First(&item).Error)
	require.Equal(t, models.ProcessingScheduleItemOutcomeSkipped, item.Outcome)
	require.Equal(t, scheduleSkipSelectionStopped, item.Reason)
}

func TestSchedulerRecordsCandidatesSkippedByBatchLimit(t *testing.T) {
	db := openProcessingTestDB(t)
	now := time.Date(2026, 8, 25, 3, 30, 0, 0, time.UTC)
	resolver := newEpisodeInputResolver()
	service := NewService(
		db,
		WithProcessingInputResolver(resolver),
		WithClock(func() time.Time { return now }),
	)
	scheduler := newTestScheduler(t, db, service, now, 1)
	first := createProcessingEpisode(t, db, true, "schedule-batch-first")
	second := createProcessingEpisode(t, db, true, "schedule-batch-second")
	third := createProcessingEpisode(t, db, true, "schedule-batch-third")
	for _, episode := range []models.Episode{first, second, third} {
		resolver.SetReady(episode.ID)
	}
	setFocusPositions(t, db, first.ID, second.ID, third.ID)

	detail, reused, err := scheduler.RunAt(context.Background(), now)
	require.NoError(t, err)
	require.False(t, reused)
	require.Equal(t, 3, detail.Run.CandidateCount)
	require.Equal(t, 1, detail.Run.StartedCount)
	require.Equal(t, 2, detail.Run.SkippedCount)
	require.Equal(t, []uint{first.ID, second.ID, third.ID}, scheduleItemEpisodeIDs(detail.Items))
	require.Equal(t, models.ProcessingScheduleItemOutcomeStarted, detail.Items[0].Outcome)
	require.Equal(t, scheduleSkipBatchLimit, detail.Items[1].Reason)
	require.Equal(t, scheduleSkipBatchLimit, detail.Items[2].Reason)

	var processingCount int64
	require.NoError(t, db.Model(&models.EpisodeProcessingRun{}).Count(&processingCount).Error)
	require.EqualValues(t, 1, processingCount)
}

func TestSchedulerDuplicateTriggerAndRestartRecoveryDoNotRepeat(t *testing.T) {
	db := openProcessingTestDB(t)
	now := time.Date(2026, 8, 25, 4, 0, 0, 0, time.UTC)
	resolver := newEpisodeInputResolver()
	service := NewService(
		db,
		WithProcessingInputResolver(resolver),
		WithClock(func() time.Time { return now }),
	)
	scheduler := newTestScheduler(t, db, service, now, 1)
	episode := createProcessingEpisode(t, db, true, "schedule-duplicate")
	resolver.SetReady(episode.ID)
	setFocusPositions(t, db, episode.ID)

	first, reused, err := scheduler.RunAt(context.Background(), now)
	require.NoError(t, err)
	require.False(t, reused)
	second, reused, err := scheduler.RunAt(context.Background(), now)
	require.NoError(t, err)
	require.True(t, reused)
	require.Equal(t, first.Run.ID, second.Run.ID)
	var firstCount int64
	require.NoError(t, db.Model(&models.EpisodeProcessingRun{}).Where("episode_id = ?", episode.ID).Count(&firstCount).Error)
	require.Equal(t, int64(1), firstCount)

	interruptedAt := now.Add(time.Minute)
	interrupted := models.ProcessingScheduleRun{
		TriggerKey:     scheduleTriggerKey("0 * * * * *", "UTC", interruptedAt),
		ScheduledFor:   interruptedAt,
		CronExpression: "0 * * * * *",
		Timezone:       "UTC",
		BatchSize:      1,
		Status:         models.ProcessingScheduleRunStatusRunning,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	require.NoError(t, db.Create(&interrupted).Error)
	recoveredEpisode := createProcessingEpisode(t, db, true, "schedule-recovered")
	resolver.SetReady(recoveredEpisode.ID)
	recoveredStart, err := service.StartEpisodeProcessing(context.Background(), StartRequest{
		EpisodeID:     recoveredEpisode.ID,
		TriggerSource: models.ProcessingTriggerScheduled,
		ScheduleRunID: &interrupted.ID,
	})
	require.NoError(t, err)

	require.NoError(t, scheduler.RecoverIncompleteRuns(context.Background(), now.Add(2*time.Minute)))
	var recovered models.ProcessingScheduleRun
	require.NoError(t, db.First(&recovered, interrupted.ID).Error)
	require.Equal(t, models.ProcessingScheduleRunStatusFailed, recovered.Status)
	require.Equal(t, "schedule_interrupted_by_restart", recovered.ErrorCode)
	require.Equal(t, 1, recovered.StartedCount)
	var recoveredItems []models.ProcessingScheduleItem
	require.NoError(t, db.Where("schedule_run_id = ?", interrupted.ID).Find(&recoveredItems).Error)
	require.Len(t, recoveredItems, 1)
	require.Equal(t, recoveredStart.Run.ID, *recoveredItems[0].ProcessingRunID)

	_, reused, err = scheduler.RunAt(context.Background(), interruptedAt)
	require.NoError(t, err)
	require.True(t, reused)
	var recoveredRunCount int64
	require.NoError(t, db.Model(&models.EpisodeProcessingRun{}).Where("episode_id = ?", recoveredEpisode.ID).Count(&recoveredRunCount).Error)
	require.Equal(t, int64(1), recoveredRunCount)
}

func TestSchedulerRecoversInterruptedRunsWhenScheduleIsDisabled(t *testing.T) {
	db := openProcessingTestDB(t)
	now := time.Date(2026, 8, 25, 4, 15, 0, 0, time.UTC)
	resolver := newEpisodeInputResolver()
	service := NewService(
		db,
		WithProcessingInputResolver(resolver),
		WithClock(func() time.Time { return now }),
	)
	scheduler, err := NewScheduler(db, service, SchedulerConfig{})
	require.NoError(t, err)
	interrupted := models.ProcessingScheduleRun{
		TriggerKey:     scheduleTriggerKey("0 * * * * *", "UTC", now),
		ScheduledFor:   now,
		CronExpression: "0 * * * * *",
		Timezone:       "UTC",
		BatchSize:      1,
		Status:         models.ProcessingScheduleRunStatusRunning,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	require.NoError(t, db.Create(&interrupted).Error)
	episode := createProcessingEpisode(t, db, true, "schedule-disabled-recovery")
	resolver.SetReady(episode.ID)
	started, err := service.StartEpisodeProcessing(context.Background(), StartRequest{
		EpisodeID:     episode.ID,
		TriggerSource: models.ProcessingTriggerScheduled,
		ScheduleRunID: &interrupted.ID,
	})
	require.NoError(t, err)

	require.NoError(t, scheduler.RecoverIncompleteRuns(context.Background(), now.Add(time.Minute)))
	var recovered models.ProcessingScheduleRun
	require.NoError(t, db.First(&recovered, interrupted.ID).Error)
	require.Equal(t, models.ProcessingScheduleRunStatusFailed, recovered.Status)
	require.Equal(t, "schedule_interrupted_by_restart", recovered.ErrorCode)
	var items []models.ProcessingScheduleItem
	require.NoError(t, db.Where("schedule_run_id = ?", interrupted.ID).Find(&items).Error)
	require.Len(t, items, 1)
	require.Equal(t, started.Run.ID, *items[0].ProcessingRunID)
}

func TestSchedulerDoesNotRecreateSameVersionTerminalRun(t *testing.T) {
	db := openProcessingTestDB(t)
	now := time.Date(2026, 8, 25, 4, 30, 0, 0, time.UTC)
	resolver := newEpisodeInputResolver()
	service := NewService(
		db,
		WithProcessingInputResolver(resolver),
		WithClock(func() time.Time { return now }),
	)
	scheduler := newTestScheduler(t, db, service, now, 1)
	episode := createProcessingEpisode(t, db, true, "schedule-terminal-run")
	resolver.SetReady(episode.ID)
	setFocusPositions(t, db, episode.ID)

	first, reused, err := scheduler.RunAt(context.Background(), now)
	require.NoError(t, err)
	require.False(t, reused)
	require.Len(t, first.Items, 1)
	require.NotNil(t, first.Items[0].ProcessingRunID)
	processingRunID := *first.Items[0].ProcessingRunID
	require.NoError(t, db.Model(&models.EpisodeProcessingRun{}).
		Where("id = ?", processingRunID).
		Updates(map[string]any{
			"status":          models.ProcessingRunStatusFailed,
			"finished_at":     now,
			"error_code":      "lark_result_unknown",
			"error_message":   "Feishu write result is unknown",
			"error_retryable": false,
			"updated_at":      now,
		}).Error)

	second, reused, err := scheduler.RunAt(context.Background(), now.Add(time.Minute))
	require.NoError(t, err)
	require.False(t, reused)
	require.Equal(t, models.ProcessingScheduleRunStatusCompleted, second.Run.Status)
	require.Equal(t, 0, second.Run.StartedCount)
	require.Equal(t, 1, second.Run.SkippedCount)
	require.Len(t, second.Items, 1)
	require.Equal(t, models.ProcessingScheduleItemOutcomeSkipped, second.Items[0].Outcome)
	require.Equal(t, scheduleSkipTerminalRun, second.Items[0].Reason)
	require.Equal(t, processingRunID, *second.Items[0].ProcessingRunID)

	var runCount int64
	require.NoError(t, db.Model(&models.EpisodeProcessingRun{}).
		Where("episode_id = ?", episode.ID).
		Count(&runCount).Error)
	require.Equal(t, int64(1), runCount)
}

func TestSchedulerDoesNotRecreateUnknownResultAcrossPipelineVersions(t *testing.T) {
	db := openProcessingTestDB(t)
	now := time.Date(2026, 8, 29, 18, 10, 0, 0, time.UTC)
	resolver := newEpisodeInputResolver()
	service := NewService(
		db,
		WithProcessingInputResolver(resolver),
		WithClock(func() time.Time { return now }),
	)
	scheduler := newTestScheduler(t, db, service, now, 1)
	episode := createProcessingEpisode(t, db, true, "schedule-cross-pipeline-unknown")
	resolver.SetReadyWithPipeline(episode.ID, "focus-processing-v1")
	setFocusPositions(t, db, episode.ID)

	first, reused, err := scheduler.RunAt(context.Background(), now)
	require.NoError(t, err)
	require.False(t, reused)
	require.Len(t, first.Items, 1)
	require.NotNil(t, first.Items[0].ProcessingRunID)
	processingRunID := *first.Items[0].ProcessingRunID
	require.NoError(t, db.Model(&models.EpisodeProcessingRun{}).
		Where("id = ?", processingRunID).
		Updates(map[string]any{
			"status":          models.ProcessingRunStatusFailed,
			"finished_at":     now,
			"error_code":      "lark_drive_result_unknown",
			"error_message":   "Feishu upload result is unknown",
			"error_retryable": false,
			"updated_at":      now,
		}).Error)
	resolver.SetReadyWithPipeline(episode.ID, NativeMinutesPipelineVersion)

	second, reused, err := scheduler.RunAt(context.Background(), now.Add(time.Minute))
	require.NoError(t, err)
	require.False(t, reused)
	require.Equal(t, models.ProcessingScheduleRunStatusCompleted, second.Run.Status)
	require.Equal(t, 0, second.Run.StartedCount)
	require.Equal(t, 1, second.Run.SkippedCount)
	require.Len(t, second.Items, 1)
	require.Equal(t, models.ProcessingScheduleItemOutcomeSkipped, second.Items[0].Outcome)
	require.Equal(t, scheduleSkipTerminalRun, second.Items[0].Reason)
	require.Equal(t, processingRunID, *second.Items[0].ProcessingRunID)

	var runCount int64
	require.NoError(t, db.Model(&models.EpisodeProcessingRun{}).
		Where("episode_id = ?", episode.ID).
		Count(&runCount).Error)
	require.Equal(t, int64(1), runCount)
}

func TestSchedulerAllowsV2AfterLegacyRuntimeResultUnknown(t *testing.T) {
	db := openProcessingTestDB(t)
	now := time.Date(2026, 8, 29, 18, 20, 0, 0, time.UTC)
	resolver := newEpisodeInputResolver()
	service := NewService(
		db,
		WithProcessingInputResolver(resolver),
		WithClock(func() time.Time { return now }),
	)
	scheduler := newTestScheduler(t, db, service, now, 1)
	episode := createProcessingEpisode(t, db, true, "schedule-runtime-unknown-upgrade")
	resolver.SetReadyWithPipeline(episode.ID, "focus-processing-v1")
	setFocusPositions(t, db, episode.ID)

	first, reused, err := scheduler.RunAt(context.Background(), now)
	require.NoError(t, err)
	require.False(t, reused)
	require.Len(t, first.Items, 1)
	require.NotNil(t, first.Items[0].ProcessingRunID)
	legacyRunID := *first.Items[0].ProcessingRunID
	require.NoError(t, db.Model(&models.EpisodeProcessingRun{}).
		Where("id = ?", legacyRunID).
		Updates(map[string]any{
			"status":          models.ProcessingRunStatusFailed,
			"finished_at":     now,
			"error_code":      "runtime_result_unknown",
			"error_message":   "local Codex result is unknown",
			"error_retryable": false,
			"updated_at":      now,
		}).Error)
	resolver.SetReadyWithPipeline(episode.ID, NativeMinutesPipelineVersion)

	second, reused, err := scheduler.RunAt(context.Background(), now.Add(time.Minute))
	require.NoError(t, err)
	require.False(t, reused)
	require.Equal(t, models.ProcessingScheduleRunStatusCompleted, second.Run.Status)
	require.Equal(t, 1, second.Run.StartedCount)
	require.Equal(t, 0, second.Run.SkippedCount)
	require.Len(t, second.Items, 1)
	require.Equal(t, models.ProcessingScheduleItemOutcomeStarted, second.Items[0].Outcome)
	require.NotNil(t, second.Items[0].ProcessingRunID)
	require.NotEqual(t, legacyRunID, *second.Items[0].ProcessingRunID)

	var runCount int64
	require.NoError(t, db.Model(&models.EpisodeProcessingRun{}).
		Where("episode_id = ?", episode.ID).
		Count(&runCount).Error)
	require.Equal(t, int64(2), runCount)
}

func TestSchedulerAllowsV2AfterLegacyTranscriptStoredCancellation(t *testing.T) {
	db := openProcessingTestDB(t)
	now := time.Date(2026, 8, 29, 18, 30, 0, 0, time.UTC)
	resolver := newEpisodeInputResolver()
	service := NewService(
		db,
		WithProcessingInputResolver(resolver),
		WithClock(func() time.Time { return now }),
	)
	scheduler := newTestScheduler(t, db, service, now, 1)
	episode := createProcessingEpisode(t, db, true, "schedule-stored-cancel-upgrade")
	resolver.SetReadyWithPipeline(episode.ID, "focus-processing-v1")
	setFocusPositions(t, db, episode.ID)

	first, reused, err := scheduler.RunAt(context.Background(), now)
	require.NoError(t, err)
	require.False(t, reused)
	require.Len(t, first.Items, 1)
	require.NotNil(t, first.Items[0].ProcessingRunID)
	legacyRunID := *first.Items[0].ProcessingRunID
	state := `{"version":1,"phase":"transcript_stored"}`
	sum := sha256.Sum256([]byte(state))
	require.NoError(t, db.Create(&models.ProcessingCheckpoint{
		RunID:          legacyRunID,
		Step:           StepTranscription,
		Adapter:        "feishu-minutes",
		AdapterVersion: "feishu-minutes-cli-v1",
		Status:         ExternalProgressCompleted,
		StateJSON:      state,
		StateHash:      hex.EncodeToString(sum[:]),
		CreatedAt:      now,
		UpdatedAt:      now,
	}).Error)
	require.NoError(t, db.Model(&models.EpisodeProcessingRun{}).
		Where("id = ?", legacyRunID).
		Updates(map[string]any{
			"status":          models.ProcessingRunStatusCancelled,
			"finished_at":     now,
			"cancelled_at":    now,
			"error_code":      cancellationExternalResultUnknown,
			"error_message":   "legacy cancellation warning",
			"error_retryable": false,
			"updated_at":      now,
		}).Error)
	resolver.SetReadyWithPipeline(episode.ID, NativeMinutesPipelineVersion)

	second, reused, err := scheduler.RunAt(context.Background(), now.Add(time.Minute))
	require.NoError(t, err)
	require.False(t, reused)
	require.Equal(t, models.ProcessingScheduleRunStatusCompleted, second.Run.Status)
	require.Equal(t, 1, second.Run.StartedCount)
	require.Equal(t, 0, second.Run.SkippedCount)
	require.Len(t, second.Items, 1)
	require.Equal(t, models.ProcessingScheduleItemOutcomeStarted, second.Items[0].Outcome)
	require.NotNil(t, second.Items[0].ProcessingRunID)
	require.NotEqual(t, legacyRunID, *second.Items[0].ProcessingRunID)
}

func TestSchedulerReprocessesAfterQueuedRunWasSkippedOutsideFocus(t *testing.T) {
	db := openProcessingTestDB(t)
	now := time.Date(2026, 8, 25, 4, 35, 0, 0, time.UTC)
	resolver := newEpisodeInputResolver()
	service := NewService(
		db,
		WithProcessingInputResolver(resolver),
		WithClock(func() time.Time { return now }),
	)
	scheduler := newTestScheduler(t, db, service, now, 1)
	episode := createProcessingEpisode(t, db, true, "schedule-returned-to-focus")
	resolver.SetReady(episode.ID)
	setFocusPositions(t, db, episode.ID)

	first, reused, err := scheduler.RunAt(context.Background(), now)
	require.NoError(t, err)
	require.False(t, reused)
	require.Len(t, first.Items, 1)
	require.NotNil(t, first.Items[0].ProcessingRunID)
	firstRunID := *first.Items[0].ProcessingRunID

	someday := models.QueueStateSomeday
	require.NoError(t, db.Model(&models.EpisodeTriageDecision{}).
		Where("episode_id = ?", episode.ID).
		Update("queue_state", someday).Error)
	skipped, err := service.cancelQueuedScheduledRunOutsideFocus(context.Background(), firstRunID)
	require.NoError(t, err)
	require.Equal(t, models.ProcessingRunStatusCancelled, skipped.Status)
	require.Equal(t, scheduledRunCancelledOutsideFocusCode, skipped.ErrorCode)

	focus := models.QueueStateFocus
	require.NoError(t, db.Model(&models.EpisodeTriageDecision{}).
		Where("episode_id = ?", episode.ID).
		Update("queue_state", focus).Error)

	second, reused, err := scheduler.RunAt(context.Background(), now.Add(time.Minute))
	require.NoError(t, err)
	require.False(t, reused)
	require.Equal(t, 1, second.Run.StartedCount)
	require.Len(t, second.Items, 1)
	require.Equal(t, models.ProcessingScheduleItemOutcomeStarted, second.Items[0].Outcome)
	require.NotNil(t, second.Items[0].ProcessingRunID)
	require.NotEqual(t, firstRunID, *second.Items[0].ProcessingRunID)

	var runCount int64
	require.NoError(t, db.Model(&models.EpisodeProcessingRun{}).
		Where("episode_id = ?", episode.ID).
		Count(&runCount).Error)
	require.Equal(t, int64(2), runCount)
}

func TestSchedulerDoesNotRecreateUserCancelledRun(t *testing.T) {
	db := openProcessingTestDB(t)
	now := time.Date(2026, 8, 25, 4, 40, 0, 0, time.UTC)
	resolver := newEpisodeInputResolver()
	service := NewService(
		db,
		WithProcessingInputResolver(resolver),
		WithClock(func() time.Time { return now }),
	)
	scheduler := newTestScheduler(t, db, service, now, 1)
	episode := createProcessingEpisode(t, db, true, "schedule-user-cancelled")
	resolver.SetReady(episode.ID)
	setFocusPositions(t, db, episode.ID)

	first, reused, err := scheduler.RunAt(context.Background(), now)
	require.NoError(t, err)
	require.False(t, reused)
	require.Len(t, first.Items, 1)
	require.NotNil(t, first.Items[0].ProcessingRunID)
	firstRunID := *first.Items[0].ProcessingRunID
	_, err = service.CancelProcessingRun(context.Background(), firstRunID)
	require.NoError(t, err)

	second, reused, err := scheduler.RunAt(context.Background(), now.Add(time.Minute))
	require.NoError(t, err)
	require.False(t, reused)
	require.Equal(t, 0, second.Run.StartedCount)
	require.Len(t, second.Items, 1)
	require.Equal(t, models.ProcessingScheduleItemOutcomeSkipped, second.Items[0].Outcome)
	require.Equal(t, scheduleSkipTerminalRun, second.Items[0].Reason)
	require.Equal(t, firstRunID, *second.Items[0].ProcessingRunID)
}

func TestSchedulerFailurePreservesRecordedAndBackfilledItems(t *testing.T) {
	db := openProcessingTestDB(t)
	now := time.Date(2026, 8, 25, 4, 45, 0, 0, time.UTC)
	resolver := newEpisodeInputResolver()
	service := NewService(
		db,
		WithProcessingInputResolver(resolver),
		WithClock(func() time.Time { return now }),
	)
	scheduler := newTestScheduler(t, db, service, now, 1)
	scheduleRun, duplicate, err := scheduler.claimRun(context.Background(), now)
	require.NoError(t, err)
	require.False(t, duplicate)

	startedEpisode := createProcessingEpisode(t, db, true, "schedule-failure-started")
	skippedEpisode := createProcessingEpisode(t, db, true, "schedule-failure-skipped")
	resolver.SetReady(startedEpisode.ID)
	setFocusPositions(t, db, startedEpisode.ID, skippedEpisode.ID)
	started, err := service.StartEpisodeProcessing(context.Background(), StartRequest{
		EpisodeID:         startedEpisode.ID,
		TriggerSource:     models.ProcessingTriggerScheduled,
		RequireReadyAudio: true,
		ScheduleRunID:     &scheduleRun.ID,
	})
	require.NoError(t, err)
	require.NoError(t, db.Create(&models.ProcessingScheduleItem{
		ScheduleRunID: scheduleRun.ID,
		EpisodeID:     skippedEpisode.ID,
		QueuePosition: 1,
		Outcome:       models.ProcessingScheduleItemOutcomeSkipped,
		Reason:        scheduleSkipAudioNotReady,
		CreatedAt:     now,
		UpdatedAt:     now,
	}).Error)

	cause := errors.New("schedule item write failed")
	detail, _, err := scheduler.failRun(
		context.Background(),
		scheduleRun.ID,
		"schedule_item_record_failed",
		"unable to record schedule result",
		cause,
	)
	require.ErrorIs(t, err, cause)
	require.Equal(t, models.ProcessingScheduleRunStatusFailed, detail.Run.Status)
	require.Equal(t, 1, detail.Run.StartedCount)
	require.Equal(t, 1, detail.Run.SkippedCount)
	require.Len(t, detail.Items, 2)

	var startedItem models.ProcessingScheduleItem
	require.NoError(t, db.Where(
		"schedule_run_id = ? AND episode_id = ?",
		scheduleRun.ID,
		startedEpisode.ID,
	).First(&startedItem).Error)
	require.NotNil(t, startedItem.ProcessingRunID)
	require.Equal(t, started.Run.ID, *startedItem.ProcessingRunID)
}

func TestSchedulerRechecksFocusBeforeCreatingScheduledRun(t *testing.T) {
	db := openProcessingTestDB(t)
	now := time.Date(2026, 8, 25, 5, 0, 0, 0, time.UTC)
	resolver := newEpisodeInputResolver()
	service := NewService(db, WithProcessingInputResolver(resolver), WithClock(func() time.Time { return now }))
	scheduler := newTestScheduler(t, db, service, now, 1)
	episode := createProcessingEpisode(t, db, true, "schedule-focus-recheck")
	resolver.SetReady(episode.ID)
	setFocusPositions(t, db, episode.ID)

	run, duplicate, err := scheduler.claimRun(context.Background(), now)
	require.NoError(t, err)
	require.False(t, duplicate)
	someday := models.QueueStateSomeday
	require.NoError(t, db.Model(&models.EpisodeTriageDecision{}).
		Where("episode_id = ?", episode.ID).
		Update("queue_state", someday).Error)

	outcome, reason, runID, _, err := scheduler.startCandidate(
		context.Background(),
		run.ID,
		scheduleCandidate{EpisodeID: episode.ID, QueuePosition: 0},
	)
	require.NoError(t, err)
	require.Equal(t, models.ProcessingScheduleItemOutcomeSkipped, outcome)
	require.Equal(t, scheduleSkipNotFocused, reason)
	require.Nil(t, runID)
	var processingCount int64
	require.NoError(t, db.Model(&models.EpisodeProcessingRun{}).Where("episode_id = ?", episode.ID).Count(&processingCount).Error)
	require.Zero(t, processingCount)
}

func TestScheduledStartRegistersScheduleItemWithProcessingRun(t *testing.T) {
	db := openProcessingTestDB(t)
	now := time.Date(2026, 8, 25, 5, 15, 0, 0, time.UTC)
	resolver := newEpisodeInputResolver()
	service := NewService(
		db,
		WithProcessingInputResolver(resolver),
		WithClock(func() time.Time { return now }),
	)
	episode := createProcessingEpisode(t, db, true, "schedule-atomic-item")
	resolver.SetReady(episode.ID)
	scheduleRun := models.ProcessingScheduleRun{
		TriggerKey:     "schedule-atomic-item",
		ScheduledFor:   now,
		CronExpression: "0 * * * * *",
		Timezone:       "UTC",
		BatchSize:      1,
		Status:         models.ProcessingScheduleRunStatusRunning,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	require.NoError(t, db.Create(&scheduleRun).Error)
	queuePosition := int64(7)
	started, err := service.StartEpisodeProcessing(context.Background(), StartRequest{
		EpisodeID:             episode.ID,
		TriggerSource:         models.ProcessingTriggerScheduled,
		RequireReadyAudio:     true,
		ScheduleRunID:         &scheduleRun.ID,
		ScheduleQueuePosition: &queuePosition,
	})
	require.NoError(t, err)

	var item models.ProcessingScheduleItem
	require.NoError(t, db.Where(
		"schedule_run_id = ? AND episode_id = ?",
		scheduleRun.ID,
		episode.ID,
	).First(&item).Error)
	require.Equal(t, models.ProcessingScheduleItemOutcomeStarted, item.Outcome)
	require.Equal(t, queuePosition, item.QueuePosition)
	require.NotNil(t, item.ProcessingRunID)
	require.Equal(t, started.Run.ID, *item.ProcessingRunID)
}

func TestSchedulerStatusAndConfigurationValidation(t *testing.T) {
	normalized, err := ValidateSchedulerConfig(SchedulerConfig{
		Enabled:   true,
		Cron:      "15 3 * * *",
		Timezone:  "Asia/Shanghai",
		BatchSize: 1,
	})
	require.NoError(t, err)
	require.Equal(t, "0 15 3 * * *", normalized.Cron)
	_, err = ValidateSchedulerConfig(SchedulerConfig{Enabled: true, Cron: "bad", Timezone: "UTC", BatchSize: 1})
	require.Error(t, err)

	db := openProcessingTestDB(t)
	service := newProcessingService(db)
	disabled, err := NewScheduler(db, service, SchedulerConfig{})
	require.NoError(t, err)
	status, err := disabled.Status(context.Background())
	require.NoError(t, err)
	require.False(t, status.Enabled)
	require.Nil(t, status.NextRunAt)
}

func newTestScheduler(
	t *testing.T,
	db *gorm.DB,
	service *Service,
	now time.Time,
	batchSize int,
) *Scheduler {
	t.Helper()
	scheduler, err := NewScheduler(
		db,
		service,
		SchedulerConfig{
			Enabled:   true,
			Cron:      "0 * * * * *",
			Timezone:  "UTC",
			BatchSize: batchSize,
		},
		WithSchedulerClock(func() time.Time { return now }),
	)
	require.NoError(t, err)
	return scheduler
}

func setFocusPositions(t *testing.T, db *gorm.DB, episodeIDs ...uint) {
	t.Helper()
	for position, episodeID := range episodeIDs {
		require.NoError(t, db.Model(&models.EpisodeTriageDecision{}).
			Where("episode_id = ?", episodeID).
			Update("queue_position", int64(position)).Error)
	}
}

func scheduleItemEpisodeIDs(items []models.ProcessingScheduleItem) []uint {
	result := make([]uint, 0, len(items))
	for _, item := range items {
		result = append(result, item.EpisodeID)
	}
	return result
}

type episodeInputResolver struct {
	mu     sync.Mutex
	inputs map[uint]ProcessingInput
	errors map[uint]error
}

func newEpisodeInputResolver() *episodeInputResolver {
	return &episodeInputResolver{
		inputs: make(map[uint]ProcessingInput),
		errors: make(map[uint]error),
	}
}

func (r *episodeInputResolver) PipelineVersion() string {
	return "pipeline-v1"
}

func (r *episodeInputResolver) ResolveProcessingInput(_ context.Context, episodeID uint) (ProcessingInput, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.errors[episodeID]; err != nil {
		return ProcessingInput{}, err
	}
	if input, ok := r.inputs[episodeID]; ok {
		return input, nil
	}
	return ProcessingInput{}, newAudioStoreError(AudioErrorNotReady, "managed audio is not ready", true)
}

func (r *episodeInputResolver) SetReady(episodeID uint) {
	r.SetReadyWithPipeline(episodeID, "pipeline-v1")
}

func (r *episodeInputResolver) SetReadyWithPipeline(
	episodeID uint,
	pipelineVersion string,
) {
	r.mu.Lock()
	r.inputs[episodeID] = ProcessingInput{
		AudioDigest:     strings.Repeat("a", 64),
		PipelineVersion: pipelineVersion,
	}
	delete(r.errors, episodeID)
	r.mu.Unlock()
}

func (r *episodeInputResolver) SetError(episodeID uint, err error) {
	r.mu.Lock()
	r.errors[episodeID] = err
	delete(r.inputs, episodeID)
	r.mu.Unlock()
}
