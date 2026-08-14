package sync

import (
	"testing"
	"time"

	"magicpodcast/internal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectPodcastMetadataUpdate(t *testing.T) {
	date := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	nextDate := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)

	current := &models.Podcast{
		EpisodeCount:       10,
		NewestEpisodeDate:  date,
		NewestEnclosureURL: "https://example.com/old.mp3",
	}

	t.Run("no update when comparable fields are unchanged", func(t *testing.T) {
		updated := &models.Podcast{
			EpisodeCount:       10,
			NewestEpisodeDate:  date,
			NewestEnclosureURL: "https://example.com/old.mp3",
		}

		check := detectPodcastMetadataUpdate(current, updated)

		assert.False(t, check.hasUpdate)
		assert.Empty(t, check.reasons)
	})

	t.Run("detects count date and enclosure changes", func(t *testing.T) {
		updated := &models.Podcast{
			EpisodeCount:       11,
			NewestEpisodeDate:  nextDate,
			NewestEnclosureURL: "https://example.com/new.mp3",
		}

		check := detectPodcastMetadataUpdate(current, updated)

		assert.True(t, check.hasUpdate)
		assert.Len(t, check.reasons, 3)
	})

	t.Run("keeps old behavior when updated newest date is empty", func(t *testing.T) {
		updated := &models.Podcast{
			EpisodeCount:       10,
			NewestEpisodeDate:  time.Time{},
			NewestEnclosureURL: "https://example.com/old.mp3",
		}

		check := detectPodcastMetadataUpdate(current, updated)

		assert.False(t, check.hasUpdate)
	})
}

func TestPlanEpisodeSync(t *testing.T) {
	t.Run("syncs when no local episodes exist", func(t *testing.T) {
		plan := planEpisodeSync("Podcast", false, 0, 10)

		assert.True(t, plan.shouldSync)
		assert.Equal(t, SyncModeFull, plan.mode)
	})

	t.Run("syncs when metadata changed", func(t *testing.T) {
		plan := planEpisodeSync("Podcast", true, 10, 10)

		assert.True(t, plan.shouldSync)
		assert.Equal(t, SyncModeFull, plan.mode)
	})

	t.Run("syncs when episode counts differ", func(t *testing.T) {
		plan := planEpisodeSync("Podcast", false, 9, 10)

		assert.True(t, plan.shouldSync)
		assert.Equal(t, SyncModeFull, plan.mode)
	})

	t.Run("skips when metadata and episode counts are unchanged", func(t *testing.T) {
		plan := planEpisodeSync("Podcast", false, 10, 10)

		assert.False(t, plan.shouldSync)
	})
}

func TestSyncPodcastsMetadataSSEEmptyLibrarySendsSummary(t *testing.T) {
	db := setupTestDB(t)
	service, err := NewService(db, "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close()) })

	reporter := &recordingReporter{}
	require.NoError(t, service.SyncPodcastsMetadataSSE(reporter))

	_, _, summaries := reporter.snapshot()
	require.Len(t, summaries, 1)
	assert.Equal(t, "sync", summaries[0].Operation)
	assert.Equal(t, 0, summaries[0].TotalPodcasts)
	assert.Equal(t, 0, summaries[0].SuccessPodcasts)
}
