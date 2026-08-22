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
	require.NoError(t, db.AutoMigrate(&models.EpisodeTriageDecision{}, &models.ConsumptionQueueOrder{}))
	require.NoError(t, db.Create(&[]models.ConsumptionQueueOrder{
		{QueueState: models.QueueStateInbox, Revision: 1},
		{QueueState: models.QueueStateFocus, Revision: 1},
		{QueueState: models.QueueStateSomeday, Revision: 1},
		{QueueState: models.QueueStateDone, Revision: 1},
	}).Error)
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

	snapshot, err := service.ListQueue(models.QueueStateInbox)
	require.NoError(t, err)
	require.Len(t, snapshot.Items, 4)
	require.Equal(t, fresh.ID, snapshot.Items[0].EpisodeID)
	require.Equal(t, AttentionNone, snapshot.Items[0].Attention)
	require.Equal(t, stale7.ID, snapshot.Items[1].EpisodeID)
	require.Equal(t, AttentionStale, snapshot.Items[1].Attention)
	require.Equal(t, stale29.ID, snapshot.Items[2].EpisodeID)
	require.Equal(t, AttentionStale, snapshot.Items[2].Attention)
	require.Equal(t, review.ID, snapshot.Items[3].EpisodeID)
	require.Equal(t, AttentionReview, snapshot.Items[3].Attention)
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

	snapshot, err := service.ListQueue(models.QueueStateInbox)
	require.NoError(t, err)
	assert.Empty(t, snapshot.Items)
}

func TestConsumptionService_PlaceQueueReordersWithoutChangingActivityAndRejectsStaleRevision(t *testing.T) {
	now := time.Date(2026, 8, 11, 9, 0, 0, 0, time.UTC)
	service, podcast := setupConsumptionService(t, now)
	first := createDiscoveryEpisode(t, service.db, podcast.ID, "第一项", now, nil)
	second := createDiscoveryEpisode(t, service.db, podcast.ID, "第二项", now, nil)
	third := createDiscoveryEpisode(t, service.db, podcast.ID, "第三项", now, nil)

	for _, episode := range []models.Episode{first, second, third} {
		_, err := service.SetQueue(episode.ID, models.QueueStateInbox, QueueWriteOptions{})
		require.NoError(t, err)
	}
	progress, err := service.MarkInProgress(first.ID)
	require.NoError(t, err)
	require.NotNil(t, progress.QueueUpdatedAt)
	require.NotNil(t, progress.InProgressAt)
	originalQueueUpdatedAt := *progress.QueueUpdatedAt
	originalInProgressAt := *progress.InProgressAt

	initial, err := service.ListQueue(models.QueueStateInbox)
	require.NoError(t, err)
	require.Equal(t, []uint{third.ID, second.ID, first.ID}, consumptionItemIDs(initial.Items))

	before := third.ID
	result, err := service.PlaceQueue(first.ID, models.QueueStateInbox, QueuePlacementOptions{
		BeforeEpisodeID:   &before,
		ExpectedRevisions: map[string]int64{models.QueueStateInbox: initial.Revision},
	})
	require.NoError(t, err)
	require.Equal(t, []uint{first.ID, third.ID, second.ID}, consumptionItemIDs(result.Queues[models.QueueStateInbox].Items))
	require.Greater(t, result.Queues[models.QueueStateInbox].Revision, initial.Revision)

	var reordered models.EpisodeTriageDecision
	require.NoError(t, service.db.Where("episode_id = ?", first.ID).First(&reordered).Error)
	require.NotNil(t, reordered.QueueUpdatedAt)
	require.True(t, reordered.QueueUpdatedAt.Equal(originalQueueUpdatedAt))
	require.NotNil(t, reordered.InProgressAt)
	require.True(t, reordered.InProgressAt.Equal(originalInProgressAt))

	currentRevision := result.Queues[models.QueueStateInbox].Revision
	noOp, err := service.PlaceQueue(first.ID, models.QueueStateInbox, QueuePlacementOptions{
		BeforeEpisodeID:   &before,
		ExpectedRevisions: map[string]int64{models.QueueStateInbox: currentRevision},
	})
	require.NoError(t, err)
	require.Equal(t, currentRevision, noOp.Queues[models.QueueStateInbox].Revision)

	_, err = service.PlaceQueue(second.ID, models.QueueStateInbox, QueuePlacementOptions{
		ExpectedRevisions: map[string]int64{models.QueueStateInbox: initial.Revision},
	})
	require.ErrorIs(t, err, ErrQueueOrderConflict)

	afterConflict, err := service.ListQueue(models.QueueStateInbox)
	require.NoError(t, err)
	require.Equal(t, []uint{first.ID, third.ID, second.ID}, consumptionItemIDs(afterConflict.Items))
	require.Equal(t, currentRevision, afterConflict.Revision)
}

func TestConsumptionService_PlaceQueueMovesAcrossQueuesAndLegacyMovesUseQueueHead(t *testing.T) {
	now := time.Date(2026, 8, 11, 9, 0, 0, 0, time.UTC)
	service, podcast := setupConsumptionService(t, now)
	inboxFirst := createDiscoveryEpisode(t, service.db, podcast.ID, "Inbox 第一项", now, nil)
	inboxSecond := createDiscoveryEpisode(t, service.db, podcast.ID, "Inbox 第二项", now, nil)
	focusItem := createDiscoveryEpisode(t, service.db, podcast.ID, "Focus 原有项", now, nil)

	for _, fixture := range []struct {
		episode models.Episode
		queue   string
	}{
		{inboxFirst, models.QueueStateInbox},
		{inboxSecond, models.QueueStateInbox},
		{focusItem, models.QueueStateFocus},
	} {
		_, err := service.SetQueue(fixture.episode.ID, fixture.queue, QueueWriteOptions{})
		require.NoError(t, err)
	}
	inbox, err := service.ListQueue(models.QueueStateInbox)
	require.NoError(t, err)
	focus, err := service.ListQueue(models.QueueStateFocus)
	require.NoError(t, err)

	before := focusItem.ID
	result, err := service.PlaceQueue(inboxFirst.ID, models.QueueStateFocus, QueuePlacementOptions{
		BeforeEpisodeID: &before,
		ExpectedRevisions: map[string]int64{
			models.QueueStateInbox: inbox.Revision,
			models.QueueStateFocus: focus.Revision,
		},
	})
	require.NoError(t, err)
	require.Equal(t, []uint{inboxSecond.ID}, consumptionItemIDs(result.Queues[models.QueueStateInbox].Items))
	require.Equal(t, []uint{inboxFirst.ID, focusItem.ID}, consumptionItemIDs(result.Queues[models.QueueStateFocus].Items))
	require.Greater(t, result.Queues[models.QueueStateInbox].Revision, inbox.Revision)
	require.Greater(t, result.Queues[models.QueueStateFocus].Revision, focus.Revision)

	legacyFirst := createDiscoveryEpisode(t, service.db, podcast.ID, "Someday 旧项", now, nil)
	legacySecond := createDiscoveryEpisode(t, service.db, podcast.ID, "Someday 新项", now, nil)
	_, err = service.SetQueue(legacyFirst.ID, models.QueueStateSomeday, QueueWriteOptions{})
	require.NoError(t, err)
	_, err = service.SetQueue(legacySecond.ID, models.QueueStateSomeday, QueueWriteOptions{})
	require.NoError(t, err)
	someday, err := service.ListQueue(models.QueueStateSomeday)
	require.NoError(t, err)
	require.Equal(t, []uint{legacySecond.ID, legacyFirst.ID}, consumptionItemIDs(someday.Items))
}

func TestConsumptionService_PlaceQueueConflictsWhenAnotherDeviceMovedTheItem(t *testing.T) {
	now := time.Date(2026, 8, 11, 9, 0, 0, 0, time.UTC)
	service, podcast := setupConsumptionService(t, now)
	episode := createDiscoveryEpisode(t, service.db, podcast.ID, "已被移动", now, nil)

	_, err := service.SetQueue(episode.ID, models.QueueStateInbox, QueueWriteOptions{})
	require.NoError(t, err)
	inbox, err := service.ListQueue(models.QueueStateInbox)
	require.NoError(t, err)
	focus, err := service.ListQueue(models.QueueStateFocus)
	require.NoError(t, err)

	// Simulate a second device moving the item after this client loaded Inbox.
	_, err = service.SetQueue(episode.ID, models.QueueStateSomeday, QueueWriteOptions{})
	require.NoError(t, err)

	_, err = service.PlaceQueue(episode.ID, models.QueueStateFocus, QueuePlacementOptions{
		ExpectedRevisions: map[string]int64{
			models.QueueStateInbox: inbox.Revision,
			models.QueueStateFocus: focus.Revision,
		},
	})
	require.ErrorIs(t, err, ErrQueueOrderConflict)

	someday, err := service.ListQueue(models.QueueStateSomeday)
	require.NoError(t, err)
	require.Equal(t, []uint{episode.ID}, consumptionItemIDs(someday.Items))
	afterFocus, err := service.ListQueue(models.QueueStateFocus)
	require.NoError(t, err)
	require.Empty(t, afterFocus.Items)
}

func TestConsumptionService_PlaceQueueConflictsWhenAnotherDeviceClearedTheItem(t *testing.T) {
	now := time.Date(2026, 8, 11, 9, 0, 0, 0, time.UTC)
	service, podcast := setupConsumptionService(t, now)
	episode := createDiscoveryEpisode(t, service.db, podcast.ID, "已被移出", now, nil)

	_, err := service.SetQueue(episode.ID, models.QueueStateInbox, QueueWriteOptions{})
	require.NoError(t, err)
	inbox, err := service.ListQueue(models.QueueStateInbox)
	require.NoError(t, err)
	focus, err := service.ListQueue(models.QueueStateFocus)
	require.NoError(t, err)

	// A stale drag must not put an item back after another device cleared it.
	_, err = service.ClearQueue(episode.ID)
	require.NoError(t, err)

	_, err = service.PlaceQueue(episode.ID, models.QueueStateFocus, QueuePlacementOptions{
		ExpectedRevisions: map[string]int64{
			models.QueueStateInbox: inbox.Revision,
			models.QueueStateFocus: focus.Revision,
		},
	})
	require.ErrorIs(t, err, ErrQueueOrderConflict)

	item, err := service.GetItem(episode.ID)
	require.NoError(t, err)
	require.Nil(t, item.QueueState)
	afterFocus, err := service.ListQueue(models.QueueStateFocus)
	require.NoError(t, err)
	require.Empty(t, afterFocus.Items)
}

func TestConsumptionService_PlaceQueueIntoEmptyDoneKeepsDoneRules(t *testing.T) {
	now := time.Date(2026, 8, 11, 9, 0, 0, 0, time.UTC)
	service, podcast := setupConsumptionService(t, now)
	episode := createDiscoveryEpisode(t, service.db, podcast.ID, "完成空队列", now, nil)

	_, err := service.SetQueue(episode.ID, models.QueueStateFocus, QueueWriteOptions{})
	require.NoError(t, err)
	progress, err := service.MarkInProgress(episode.ID)
	require.NoError(t, err)
	require.NotNil(t, progress.InProgressAt)

	focus, err := service.ListQueue(models.QueueStateFocus)
	require.NoError(t, err)
	done, err := service.ListQueue(models.QueueStateDone)
	require.NoError(t, err)
	result, err := service.PlaceQueue(episode.ID, models.QueueStateDone, QueuePlacementOptions{
		ExpectedRevisions: map[string]int64{
			models.QueueStateFocus: focus.Revision,
			models.QueueStateDone:  done.Revision,
		},
	})
	require.NoError(t, err)
	require.Empty(t, result.Queues[models.QueueStateFocus].Items)
	require.Equal(t, []uint{episode.ID}, consumptionItemIDs(result.Queues[models.QueueStateDone].Items))
	require.Greater(t, result.Queues[models.QueueStateFocus].Revision, focus.Revision)
	require.Greater(t, result.Queues[models.QueueStateDone].Revision, done.Revision)

	placed, err := service.GetItem(episode.ID)
	require.NoError(t, err)
	require.NotNil(t, placed.QueueState)
	require.Equal(t, models.QueueStateDone, *placed.QueueState)
	require.Nil(t, placed.InProgressAt)
}

func TestConsumptionService_PlaceQueueFocusLimitDoesNotWriteBeforeConfirmation(t *testing.T) {
	now := time.Date(2026, 8, 11, 9, 0, 0, 0, time.UTC)
	service, podcast := setupConsumptionService(t, now)
	for index := 0; index < FocusSoftLimit; index++ {
		episode := createDiscoveryEpisode(t, service.db, podcast.ID, "Focus 已满", now, nil)
		_, err := service.SetQueue(episode.ID, models.QueueStateFocus, QueueWriteOptions{})
		require.NoError(t, err)
	}
	candidate := createDiscoveryEpisode(t, service.db, podcast.ID, "待确认", now, nil)
	_, err := service.SetQueue(candidate.ID, models.QueueStateInbox, QueueWriteOptions{})
	require.NoError(t, err)

	inbox, err := service.ListQueue(models.QueueStateInbox)
	require.NoError(t, err)
	focus, err := service.ListQueue(models.QueueStateFocus)
	require.NoError(t, err)
	_, err = service.PlaceQueue(candidate.ID, models.QueueStateFocus, QueuePlacementOptions{
		ExpectedRevisions: map[string]int64{
			models.QueueStateInbox: inbox.Revision,
			models.QueueStateFocus: focus.Revision,
		},
	})
	var limitErr *FocusLimitConfirmationError
	require.ErrorAs(t, err, &limitErr)
	require.Equal(t, FocusSoftLimit, limitErr.CurrentCount)

	afterInbox, err := service.ListQueue(models.QueueStateInbox)
	require.NoError(t, err)
	afterFocus, err := service.ListQueue(models.QueueStateFocus)
	require.NoError(t, err)
	require.Equal(t, inbox.Revision, afterInbox.Revision)
	require.Equal(t, focus.Revision, afterFocus.Revision)
	require.Equal(t, []uint{candidate.ID}, consumptionItemIDs(afterInbox.Items))
	require.Len(t, afterFocus.Items, FocusSoftLimit)
}

func consumptionItemIDs(items []ConsumptionItem) []uint {
	result := make([]uint, 0, len(items))
	for _, item := range items {
		result = append(result, item.EpisodeID)
	}
	return result
}
