package sync

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"magicpodcast/internal/models"
	"magicpodcast/internal/xyzvideo"

	"github.com/mmcdole/gofeed"
	"github.com/stretchr/testify/require"
)

func xyzItem(title, guid, eid string, published time.Time) *gofeed.Item {
	return &gofeed.Item{
		Title:           title,
		GUID:            guid,
		PublishedParsed: ptrTime(published),
		Link:            "https://www.xiaoyuzhoufm.com/episode/" + eid + "?utm_source=rss",
		Enclosures:      []*gofeed.Enclosure{{URL: "https://media.example.invalid/" + guid + ".m4a", Type: "audio/mp4"}},
	}
}

func TestSyncPodcastEpisodeItemsProbesVideoTriStateWithoutPersistingHLS(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		switch {
		case strings.Contains(r.URL.Path, "6a734c29ab3a91c24a1067fa"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"playback":{"master":{"url":"https://video.xyzcdn.net/episode-video/x/hls/preview/master.m3u8?auth_key=TOP-SECRET"}}}`)
		case strings.Contains(r.URL.Path, "6a8cf80a1352af56ff3b7e2d"):
			http.Error(w, "Video playback not found", http.StatusNotFound)
		default:
			w.WriteHeader(http.StatusForbidden)
		}
	}))
	t.Cleanup(server.Close)

	db := setupTestDB(t)
	service, err := NewService(db, "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close()) })
	service.videoProber = xyzvideo.NewProber(xyzvideo.ProberConfig{
		BaseURL: server.URL,
		Getter:  xyzvideo.NewHTTPGetter(server.Client(), ""),
	})

	podcast := &models.Podcast{XYZID: "video-probe", Title: "Video Probe", FeedURL: "https://example.com/video.xml"}
	require.NoError(t, db.Create(podcast).Error)

	published := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	result, err := service.syncPodcastEpisodeItems(podcast, []*gofeed.Item{
		xyzItem("视频集", "video-guid", "6a734c29ab3a91c24a1067fa", published),
		xyzItem("音频集", "audio-guid", "6a8cf80a1352af56ff3b7e2d", published.Add(time.Hour)),
	}, DefaultEpisodeSyncConfig)
	require.NoError(t, err)
	require.Equal(t, 2, result.Created)
	require.Equal(t, int32(2), hits.Load())

	var stored []models.Episode
	require.NoError(t, db.Order("id").Find(&stored).Error)
	require.Len(t, stored, 2)
	require.Equal(t, models.VideoAvailabilityAvailable, stored[0].VideoAvailability)
	require.Equal(t, models.VideoAvailabilityUnavailable, stored[1].VideoAvailability)
	require.Equal(t, "https://media.example.invalid/video-guid.m4a", stored[0].MediumURL)
	require.NotContains(t, stored[0].MediumURL, "m3u8")
	require.NotContains(t, stored[0].Link, "auth_key")

	var blob string
	require.NoError(t, db.Raw(`
		SELECT group_concat(coalesce(medium_url,'') || coalesce(link,'') || coalesce(content,'') || coalesce(show_notes,'') || coalesce(video_availability,''))
		FROM episodes
	`).Scan(&blob).Error)
	require.NotContains(t, blob, "auth_key")
	require.NotContains(t, blob, "m3u8")
}

func TestSyncPodcastEpisodeItemsSkipsTerminalUnchangedAndCapsProbes(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	db := setupTestDB(t)
	service, err := NewService(db, "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close()) })
	service.videoProber = xyzvideo.NewProber(xyzvideo.ProberConfig{
		BaseURL: server.URL,
		Getter:  xyzvideo.NewHTTPGetter(server.Client(), ""),
	})

	podcast := &models.Podcast{XYZID: "video-cap", Title: "Video Cap", FeedURL: "https://example.com/cap.xml"}
	require.NoError(t, db.Create(podcast).Error)

	existingLink := "https://www.xiaoyuzhoufm.com/episode/6a734c29ab3a91c24a1067fa"
	existing := &models.Episode{
		PodcastID:         podcast.ID,
		Title:             "已判定视频集",
		GUID:              "already-available",
		Link:              existingLink,
		VideoAvailability: models.VideoAvailabilityAvailable,
		PublishedDate:     time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
	}
	require.NoError(t, db.Create(existing).Error)

	items := []*gofeed.Item{
		{
			Title:           existing.Title,
			GUID:            existing.GUID,
			Link:            existingLink,
			PublishedParsed: ptrTime(existing.PublishedDate),
		},
	}
	for i := 0; i < 8; i++ {
		eid := "aaaaaaaaaaaaaaaaaaaaaaaa" + string(rune('a'+i))
		items = append(items, xyzItem("未知集", "unknown-"+string(rune('a'+i)), eid, time.Date(2026, 8, 2+i, 0, 0, 0, 0, time.UTC)))
	}

	result, err := service.syncPodcastEpisodeItems(podcast, items, DefaultEpisodeSyncConfig)
	require.NoError(t, err)
	require.Equal(t, 8, result.Created)
	require.Equal(t, 1, result.Skipped)
	require.Equal(t, int32(maxVideoProbesPerPodcast), hits.Load())

	var unchanged models.Episode
	require.NoError(t, db.Where("guid = ?", "already-available").First(&unchanged).Error)
	require.Equal(t, models.VideoAvailabilityAvailable, unchanged.VideoAvailability)
}

func TestSyncPodcastEpisodeItemsLeavesUnknownOn403(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(server.Close)

	db := setupTestDB(t)
	service, err := NewService(db, "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close()) })
	service.videoProber = xyzvideo.NewProber(xyzvideo.ProberConfig{
		BaseURL: server.URL,
		Getter:  xyzvideo.NewHTTPGetter(server.Client(), ""),
	})

	podcast := &models.Podcast{XYZID: "video-403", Title: "Video 403", FeedURL: "https://example.com/403.xml"}
	require.NoError(t, db.Create(podcast).Error)

	_, err = service.syncPodcastEpisodeItems(podcast, []*gofeed.Item{
		xyzItem("待判定", "unknown-guid", "6a734c29ab3a91c24a1067fa", time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)),
	}, DefaultEpisodeSyncConfig)
	require.NoError(t, err)

	var stored models.Episode
	require.NoError(t, db.Where("guid = ?", "unknown-guid").First(&stored).Error)
	require.Equal(t, "", stored.VideoAvailability)
	require.Equal(t, models.VideoAvailabilityUnknown, models.NormalizeVideoAvailability(stored.VideoAvailability))
}

func TestSyncPodcastEpisodeItemsRechecksChangedTerminalStates(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "changed-missing") {
			http.Error(w, "Video playback not found", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(server.Close)

	db := setupTestDB(t)
	service, err := NewService(db, "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close()) })
	service.videoProber = xyzvideo.NewProber(xyzvideo.ProberConfig{
		BaseURL: server.URL,
		Getter:  xyzvideo.NewHTTPGetter(server.Client(), ""),
	})

	podcast := &models.Podcast{XYZID: "video-refresh", Title: "Video Refresh", FeedURL: "https://example.com/refresh.xml"}
	require.NoError(t, db.Create(podcast).Error)
	published := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	existing := []models.Episode{
		{
			PodcastID: podcast.ID, Title: "旧标题", GUID: "changed-missing-guid",
			Link:      "https://www.xiaoyuzhoufm.com/episode/changed-missing",
			MediumURL: "https://media.example.invalid/changed-missing-guid.m4a", EnclosureType: "audio/mp4",
			PublishedDate: published, VideoAvailability: models.VideoAvailabilityAvailable,
		},
		{
			PodcastID: podcast.ID, Title: "旧标题", GUID: "changed-denied-guid",
			Link:      "https://www.xiaoyuzhoufm.com/episode/changed-denied",
			MediumURL: "https://media.example.invalid/changed-denied-guid.m4a", EnclosureType: "audio/mp4",
			PublishedDate: published.Add(time.Hour), VideoAvailability: models.VideoAvailabilityAvailable,
		},
	}
	require.NoError(t, db.Create(&existing).Error)

	result, err := service.syncPodcastEpisodeItems(podcast, []*gofeed.Item{
		xyzItem("新标题", "changed-missing-guid", "changed-missing", published),
		xyzItem("新标题", "changed-denied-guid", "changed-denied", published.Add(time.Hour)),
	}, DefaultEpisodeSyncConfig)
	require.NoError(t, err)
	require.Equal(t, 2, result.Updated)

	var refreshed []models.Episode
	require.NoError(t, db.Order("id").Find(&refreshed).Error)
	require.Equal(t, models.VideoAvailabilityUnavailable, refreshed[0].VideoAvailability)
	require.Equal(t, models.VideoAvailabilityUnknown, models.NormalizeVideoAvailability(refreshed[1].VideoAvailability))
}

func TestSyncPodcastEpisodeItemsBoundsVideoProbeBatch(t *testing.T) {
	originalTimeout := maxVideoProbeBatchDuration
	maxVideoProbeBatchDuration = 25 * time.Millisecond
	t.Cleanup(func() { maxVideoProbeBatchDuration = originalTimeout })

	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	db := setupTestDB(t)
	service, err := NewService(db, "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close()) })
	service.videoProber = xyzvideo.NewProber(xyzvideo.ProberConfig{
		BaseURL: server.URL,
		Getter:  xyzvideo.NewHTTPGetter(server.Client(), ""),
	})
	podcast := &models.Podcast{XYZID: "video-timeout", Title: "Video Timeout", FeedURL: "https://example.com/timeout.xml"}
	require.NoError(t, db.Create(podcast).Error)
	published := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)

	started := time.Now()
	result, err := service.syncPodcastEpisodeItems(podcast, []*gofeed.Item{
		xyzItem("第一集", "timeout-a", "timeout-a", published),
		xyzItem("第二集", "timeout-b", "timeout-b", published.Add(time.Hour)),
	}, DefaultEpisodeSyncConfig)
	require.NoError(t, err)
	require.Equal(t, 2, result.Created)
	require.Less(t, time.Since(started), 150*time.Millisecond)
	require.Equal(t, int32(1), hits.Load())
}
