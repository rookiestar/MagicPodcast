package services

import (
	"testing"
	"time"

	"magicpodcast/internal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupConsumptionService(t *testing.T, now time.Time) (*ConsumptionService, models.Podcast) {
	t.Helper()
	db := setupDiscoveryTestDB(t)
	require.NoError(t, db.AutoMigrate(&models.EpisodeTriageDecision{}))
	service := NewConsumptionService(db)
	service.now = func() time.Time { return now }
	return service, createDiscoveryPodcast(t, db, "消费状态节目")
}

func TestConsumptionService_SetQueueCollectsIdempotentlyAndClearsDismissed(t *testing.T) {
	now := time.Date(2026, 8, 11, 9, 0, 0, 0, time.UTC)
	service, podcast := setupConsumptionService(t, now)
	episode := createDiscoveryEpisode(t, service.db, podcast.ID, "待收集", now.Add(-24*time.Hour), nil)

	dismissed, err := service.SetDismissed(episode.ID, true)
	require.NoError(t, err)
	require.NotNil(t, dismissed.DismissedAt)

	collected, err := service.SetQueue(episode.ID, models.QueueStateInbox, QueueWriteOptions{})
	require.NoError(t, err)
	require.NotNil(t, collected.QueueState)
	require.Equal(t, models.QueueStateInbox, *collected.QueueState)
	require.Nil(t, collected.DismissedAt)
	require.Equal(t, models.TriageStateShortlisted, collected.State)
	firstUpdatedAt := *collected.QueueUpdatedAt

	repeated, err := service.SetQueue(episode.ID, models.QueueStateInbox, QueueWriteOptions{})
	require.NoError(t, err)
	require.Equal(t, collected.ID, repeated.ID)
	require.NotNil(t, repeated.QueueUpdatedAt)
	require.True(t, repeated.QueueUpdatedAt.Equal(firstUpdatedAt))

	var count int64
	require.NoError(t, service.db.Model(&models.EpisodeTriageDecision{}).
		Where("episode_id = ?", episode.ID).Count(&count).Error)
	require.Equal(t, int64(1), count)
}

func TestConsumptionService_SetQueueMaintainsMutualExclusionAndManualDone(t *testing.T) {
	now := time.Date(2026, 8, 11, 9, 0, 0, 0, time.UTC)
	service, podcast := setupConsumptionService(t, now)
	episode := createDiscoveryEpisode(t, service.db, podcast.ID, "显式流转", now, nil)

	_, err := service.SetQueue(episode.ID, models.QueueStateInbox, QueueWriteOptions{})
	require.NoError(t, err)
	now = now.Add(time.Hour)
	focus, err := service.SetQueue(episode.ID, models.QueueStateFocus, QueueWriteOptions{})
	require.NoError(t, err)
	require.Equal(t, models.QueueStateFocus, *focus.QueueState)

	now = now.Add(time.Hour)
	progress, err := service.MarkInProgress(episode.ID)
	require.NoError(t, err)
	require.NotNil(t, progress.InProgressAt)
	require.Equal(t, models.QueueStateFocus, *progress.QueueState)

	now = now.Add(time.Hour)
	done, err := service.SetQueue(episode.ID, models.QueueStateDone, QueueWriteOptions{})
	require.NoError(t, err)
	require.Equal(t, models.QueueStateDone, *done.QueueState)
	require.Nil(t, done.InProgressAt)

	queues, err := service.QueueSummary()
	require.NoError(t, err)
	require.Equal(t, int64(0), queues.Counts[models.QueueStateInbox])
	require.Equal(t, int64(0), queues.Counts[models.QueueStateFocus])
	require.Equal(t, int64(1), queues.Counts[models.QueueStateDone])
}

func TestConsumptionService_FocusLimitRequiresExplicitAcknowledgement(t *testing.T) {
	now := time.Date(2026, 8, 11, 9, 0, 0, 0, time.UTC)
	service, podcast := setupConsumptionService(t, now)
	summary, err := service.QueueSummary()
	require.NoError(t, err)
	require.Equal(t, int64(0), summary.Counts[models.QueueStateFocus])
	require.False(t, summary.FocusOverLimit)

	var seventh, eighth models.Episode
	for index := 0; index < FocusSoftLimit+1; index++ {
		episode := createDiscoveryEpisode(t, service.db, podcast.ID, "Focus", now.Add(time.Duration(index)*time.Minute), nil)
		switch {
		case index < FocusSoftLimit-1:
			_, err := service.SetQueue(episode.ID, models.QueueStateFocus, QueueWriteOptions{})
			require.NoError(t, err)
		case index == FocusSoftLimit-1:
			seventh = episode
		default:
			eighth = episode
		}
	}

	summary, err = service.QueueSummary()
	require.NoError(t, err)
	require.Equal(t, int64(6), summary.Counts[models.QueueStateFocus])
	require.False(t, summary.FocusOverLimit)

	_, err = service.SetQueue(seventh.ID, models.QueueStateFocus, QueueWriteOptions{})
	require.NoError(t, err)
	summary, err = service.QueueSummary()
	require.NoError(t, err)
	require.Equal(t, int64(FocusSoftLimit), summary.Counts[models.QueueStateFocus])
	require.False(t, summary.FocusOverLimit)

	_, err = service.SetQueue(eighth.ID, models.QueueStateFocus, QueueWriteOptions{})
	var limitErr *FocusLimitConfirmationError
	require.ErrorAs(t, err, &limitErr)
	require.Equal(t, FocusSoftLimit, limitErr.CurrentCount)

	added, err := service.SetQueue(eighth.ID, models.QueueStateFocus, QueueWriteOptions{
		AcknowledgeFocusLimit: true,
	})
	require.NoError(t, err)
	require.Equal(t, models.QueueStateFocus, *added.QueueState)

	summary, err = service.QueueSummary()
	require.NoError(t, err)
	require.Equal(t, int64(FocusSoftLimit+1), summary.Counts[models.QueueStateFocus])
	require.True(t, summary.FocusOverLimit)
}

func TestConsumptionService_ListQueueUsesStableOrderAndAttentionBoundaries(t *testing.T) {
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	service, podcast := setupConsumptionService(t, now)
	review := createDiscoveryEpisode(t, service.db, podcast.ID, "30 天复盘", now.Add(-40*24*time.Hour), nil)
	stale29 := createDiscoveryEpisode(t, service.db, podcast.ID, "29 天陈旧", now.Add(-35*24*time.Hour), nil)
	stale7 := createDiscoveryEpisode(t, service.db, podcast.ID, "7 天陈旧", now.Add(-10*24*time.Hour), nil)
	fresh := createDiscoveryEpisode(t, service.db, podcast.ID, "6 天内", now.Add(-time.Hour), nil)

	for _, fixture := range []struct {
		episode models.Episode
		at      time.Time
	}{
		{review, now.Add(-30 * 24 * time.Hour)},
		{stale29, now.Add(-29 * 24 * time.Hour)},
		{stale7, now.Add(-7 * 24 * time.Hour)},
		{fresh, now.Add(-6 * 24 * time.Hour)},
	} {
		service.now = func() time.Time { return fixture.at }
		_, err := service.SetQueue(fixture.episode.ID, models.QueueStateInbox, QueueWriteOptions{})
		require.NoError(t, err)
	}
	service.now = func() time.Time { return now }

	items, err := service.ListQueue(models.QueueStateInbox)
	require.NoError(t, err)
	require.Len(t, items, 4)
	require.Equal(t, fresh.ID, items[0].EpisodeID)
	require.Equal(t, AttentionNone, items[0].Attention)
	require.Equal(t, stale7.ID, items[1].EpisodeID)
	require.Equal(t, AttentionStale, items[1].Attention)
	require.Equal(t, stale29.ID, items[2].EpisodeID)
	require.Equal(t, AttentionStale, items[2].Attention)
	require.Equal(t, review.ID, items[3].EpisodeID)
	require.Equal(t, AttentionReview, items[3].Attention)
}

func TestConsumptionService_MarkReadAndInProgressDoNotChangeQueue(t *testing.T) {
	now := time.Date(2026, 8, 11, 9, 0, 0, 0, time.UTC)
	service, podcast := setupConsumptionService(t, now)
	episode := createDiscoveryEpisode(t, service.db, podcast.ID, "阅读与外跳", now, nil)
	_, err := service.SetQueue(episode.ID, models.QueueStateSomeday, QueueWriteOptions{})
	require.NoError(t, err)

	now = now.Add(time.Hour)
	read, err := service.MarkRead(episode.ID)
	require.NoError(t, err)
	require.NotNil(t, read.ReadAt)
	require.Equal(t, models.QueueStateSomeday, *read.QueueState)

	firstRead := *read.ReadAt
	now = now.Add(time.Hour)
	readAgain, err := service.MarkRead(episode.ID)
	require.NoError(t, err)
	require.True(t, readAgain.ReadAt.Equal(firstRead))

	progress, err := service.MarkInProgress(episode.ID)
	require.NoError(t, err)
	require.NotNil(t, progress.InProgressAt)
	require.Equal(t, models.QueueStateSomeday, *progress.QueueState)
}

func TestConsumptionService_RejectsInvalidQueueAndMissingEpisode(t *testing.T) {
	now := time.Date(2026, 8, 11, 9, 0, 0, 0, time.UTC)
	service, podcast := setupConsumptionService(t, now)
	episode := createDiscoveryEpisode(t, service.db, podcast.ID, "合法单集", now, nil)

	_, err := service.SetQueue(episode.ID, "later", QueueWriteOptions{})
	require.ErrorIs(t, err, ErrInvalidQueueState)
	_, err = service.SetQueue(999999, models.QueueStateInbox, QueueWriteOptions{})
	require.ErrorIs(t, err, ErrConsumptionEpisodeNotFound)
	_, err = service.ListQueue("later")
	require.ErrorIs(t, err, ErrInvalidQueueState)
}

func TestConsumptionService_RestoringDismissedReturnsNeutralDiscoveryState(t *testing.T) {
	now := time.Date(2026, 8, 11, 9, 0, 0, 0, time.UTC)
	service, podcast := setupConsumptionService(t, now)
	episode := createDiscoveryEpisode(t, service.db, podcast.ID, "恢复不感兴趣", now, nil)
	_, err := service.SetQueue(episode.ID, models.QueueStateInbox, QueueWriteOptions{})
	require.NoError(t, err)

	dismissed, err := service.SetDismissed(episode.ID, true)
	require.NoError(t, err)
	require.Nil(t, dismissed.QueueState)
	require.NotNil(t, dismissed.DismissedAt)
	require.Equal(t, models.TriageStateDiscarded, dismissed.State)

	restored, err := service.SetDismissed(episode.ID, false)
	require.NoError(t, err)
	require.Nil(t, restored.QueueState)
	require.Nil(t, restored.DismissedAt)
	require.Equal(t, models.TriageStatePending, restored.State)

	items, err := service.ListQueue(models.QueueStateInbox)
	require.NoError(t, err)
	assert.Empty(t, items)
}
