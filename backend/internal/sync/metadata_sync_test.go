package sync

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"magicpodcast/internal/models"
	"magicpodcast/internal/xyzvideo"

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

type metadataDeadlineGetter struct {
	deadline    time.Time
	hasDeadline bool
}

func (g *metadataDeadlineGetter) Get(ctx context.Context, rawURL string) (int, []byte, error) {
	g.deadline, g.hasDeadline = ctx.Deadline()
	_ = rawURL
	return http.StatusNotFound, nil, nil
}

func TestMetadataSyncPassesDeadlineToVideoProbe(t *testing.T) {
	originalProbeBatchDuration := maxVideoProbeBatchDuration
	maxVideoProbeBatchDuration = 2 * time.Minute
	t.Cleanup(func() { maxVideoProbeBatchDuration = originalProbeBatchDuration })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveRobotsNotFoundSync(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/rss+xml")
		_, err := io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Metadata Deadline Test</title>
    <description>Deadline propagation</description>
    <item>
      <title>Video Episode</title>
      <guid>metadata-deadline-video</guid>
      <link>https://www.xiaoyuzhoufm.com/episode/6a734c29ab3a91c24a1067fa</link>
      <pubDate>Sat, 01 Aug 2026 00:00:00 GMT</pubDate>
      <description>Video episode</description>
      <enclosure url="https://media.example.invalid/episode.m4a" type="audio/mp4" />
    </item>
  </channel>
</rss>`)
		require.NoError(t, err)
	}))
	t.Cleanup(server.Close)

	db := setupTestDB(t)
	service, err := NewService(db, "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close()) })

	getter := &metadataDeadlineGetter{}
	service.videoProber = xyzvideo.NewProber(xyzvideo.ProberConfig{
		BaseURL: "https://www.xiaoyuzhoufm.com",
		Getter:  getter,
	})

	podcast := &models.Podcast{
		XYZID:        "metadata-deadline",
		Title:        "Metadata Deadline",
		FeedURL:      server.URL,
		DataSource:   "rss",
		IsSubscribed: true,
		FeedURLValid: true,
	}
	require.NoError(t, db.Create(podcast).Error)

	err, noUpdate, episodeResult := service.syncPodcastMetadataWithUpdateCheck(podcast)
	require.NoError(t, err)
	require.False(t, noUpdate)
	require.NotNil(t, episodeResult)
	require.True(t, getter.hasDeadline)
	require.Greater(t, time.Until(getter.deadline), 0*time.Second)
	require.Less(t, time.Until(getter.deadline), 45*time.Second)
}
