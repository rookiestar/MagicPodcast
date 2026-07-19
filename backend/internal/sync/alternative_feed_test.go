package sync

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"magicpodcast/internal/feed"
	"magicpodcast/internal/models"

	"github.com/mmcdole/gofeed"
	ext "github.com/mmcdole/gofeed/extensions"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

type alternativeIndexRow struct {
	id       int
	title    string
	author   string
	feedURL  string
	itunesID int
	guid     string
	dead     int
	status   int
}

func createAlternativeIndexFixture(t *testing.T, rows []alternativeIndexRow) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "podcastindex.db")
	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	defer db.Close()
	_, err = db.Exec(`
CREATE TABLE podcasts (
  id INTEGER PRIMARY KEY,
  url TEXT NOT NULL,
  title TEXT NOT NULL,
  lastUpdate INTEGER,
  link TEXT,
  lastHttpStatus INTEGER,
  dead INTEGER,
  itunesAuthor TEXT,
  itunesId INTEGER,
  imageUrl TEXT,
  newestItemPubdate INTEGER,
  language TEXT,
  oldestItemPubdate INTEGER,
  episodeCount INTEGER,
  popularityScore INTEGER,
  priority INTEGER,
  updateFrequency INTEGER,
  newestEnclosureUrl TEXT,
  podcastGuid TEXT,
  description TEXT,
  newestEnclosureDuration INTEGER
)`)
	require.NoError(t, err)
	for _, row := range rows {
		_, err = db.Exec(`INSERT INTO podcasts
 (id, url, title, lastUpdate, link, lastHttpStatus, dead, itunesAuthor, itunesId,
  imageUrl, newestItemPubdate, language, oldestItemPubdate, episodeCount,
  popularityScore, priority, updateFrequency, newestEnclosureUrl, podcastGuid,
  description, newestEnclosureDuration)
 VALUES (?, ?, ?, 1, 'https://example.com', ?, ?, ?, ?,
         'https://example.com/image.jpg', 1, 'en', 1, 1, 1, 1, 1,
         'https://example.com/episode.mp3', ?, 'description', 60)`,
			row.id, row.feedURL, row.title, row.status, row.dead, row.author, row.itunesID, row.guid)
		require.NoError(t, err)
	}
	return path
}

func alternativeRSS(title, guid string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel><title>%s</title><description>verified content</description>
<item><title>Episode from %s</title><guid>%s</guid><pubDate>Tue, 14 Jul 2026 08:00:00 GMT</pubDate><description>Details</description></item>
</channel></rss>`, title, title, guid)
}

func newAlternativeCoordinator(primaryURL string) *feed.Coordinator {
	return feed.NewCoordinator(feed.CoordinatorConfig{
		DomainPolicies: map[string]feed.DomainPolicy{
			feed.TargetDomain(primaryURL): {MaxConcurrency: 1},
		},
		LastGoodStore: feed.NewMemorySnapshotStore(feed.LastGoodStoreConfig{}),
	})
}

func TestConvertGofeedToModelExtractsStableFeedIdentity(t *testing.T) {
	db := setupTestDB(t)
	service, err := NewService(db, "")
	require.NoError(t, err)
	defer service.Close()

	converted := service.convertGofeedToModel(&gofeed.Feed{
		Title: "Identity Feed",
		Extensions: ext.Extensions{
			"podcast": {
				"guid": []ext.Extension{{Value: "podcast-guid-from-rss"}},
			},
			"itunes": {
				"id": []ext.Extension{{Value: "98765"}},
			},
		},
	}, "rss", "https://example.com/identity.xml")

	require.Equal(t, "podcast-guid-from-rss", converted.PodcastGUID)
	require.Equal(t, "98765", converted.ITunesID)
}

func TestWorkflowUsesVerifiedPodcastIndexAlternativeAndRecordsIdentity(t *testing.T) {
	var primaryRequests, alternativeRequests int32
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&primaryRequests, 1)
		w.WriteHeader(http.StatusForbidden)
	}))
	defer primary.Close()
	alternative := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&alternativeRequests, 1)
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(alternativeRSS("稳定节目", "alternative-episode")))
	}))
	defer alternative.Close()

	indexPath := createAlternativeIndexFixture(t, []alternativeIndexRow{
		{id: 1, title: "稳定节目", author: "作者", feedURL: primary.URL + "/primary.xml", itunesID: 123, guid: "podcast-guid-123", dead: 0, status: 403},
		{id: 2, title: "稳定节目", author: "作者", feedURL: alternative.URL + "/alternative.xml", itunesID: 123, guid: "podcast-guid-123", dead: 0, status: 200},
	})
	db := setupTestDB(t)
	service, err := NewServiceWithFeedCoordinator(db, indexPath, newAlternativeCoordinator(primary.URL))
	require.NoError(t, err)
	defer service.Close()

	podcast := &models.Podcast{
		XYZID:       "verified-alternative",
		Title:       "稳定节目",
		Author:      "作者",
		FeedURL:     primary.URL + "/primary.xml",
		ITunesID:    "123",
		PodcastGUID: "podcast-guid-123",
	}
	require.NoError(t, db.Create(podcast).Error)

	result, err := service.SyncPodcastEpisodesWithContext(t.Context(), podcast.ID, &progressReporter{}, EpisodeSyncConfig{
		Mode:                  SyncModeFull,
		MaxEpisodesPerPodcast: 100,
		UpdateExisting:        true,
	})
	require.NoError(t, err)
	require.NotNil(t, result.FeedAccess)
	require.Equal(t, feed.AccessSourceAlternative, result.FeedAccess.SourceType)
	require.Equal(t, alternative.URL+"/alternative.xml", result.FeedAccess.SourceURL)
	require.Equal(t, feed.IdentityVerificationVerifiedMetadata, result.FeedAccess.IdentityVerification)
	require.Equal(t, int32(1), atomic.LoadInt32(&primaryRequests))
	require.Equal(t, int32(1), atomic.LoadInt32(&alternativeRequests))
	require.Equal(t, 1, result.Created)
}

func TestWorkflowRejectsSameTitleCandidateWithoutStableIdentity(t *testing.T) {
	var primaryRequests, alternativeRequests int32
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&primaryRequests, 1)
		w.WriteHeader(http.StatusForbidden)
	}))
	defer primary.Close()
	alternative := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&alternativeRequests, 1)
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(alternativeRSS("同名节目", "title-only-episode")))
	}))
	defer alternative.Close()

	indexPath := createAlternativeIndexFixture(t, []alternativeIndexRow{
		{id: 1, title: "同名节目", author: "作者", feedURL: primary.URL + "/primary.xml", itunesID: 0, guid: "", dead: 0, status: 403},
		{id: 2, title: "同名节目", author: "作者", feedURL: alternative.URL + "/alternative.xml", itunesID: 0, guid: "", dead: 0, status: 200},
	})
	db := setupTestDB(t)
	service, err := NewServiceWithFeedCoordinator(db, indexPath, newAlternativeCoordinator(primary.URL))
	require.NoError(t, err)
	defer service.Close()

	podcast := &models.Podcast{XYZID: "title-only", Title: "同名节目", Author: "作者", FeedURL: primary.URL + "/primary.xml"}
	require.NoError(t, db.Create(podcast).Error)
	result, err := service.SyncPodcastEpisodesWithContext(t.Context(), podcast.ID, &progressReporter{}, EpisodeSyncConfig{Mode: SyncModeFull, UpdateExisting: true})
	require.Error(t, err)
	require.NotNil(t, result.FeedAccess)
	require.Equal(t, feed.IdentityVerificationRejectedNoStableID, result.FeedAccess.IdentityVerification)
	require.Equal(t, feed.AccessSourcePrimary, result.FeedAccess.SourceType)
	require.Equal(t, int32(1), atomic.LoadInt32(&primaryRequests))
	require.Equal(t, int32(0), atomic.LoadInt32(&alternativeRequests))
}

func TestWorkflowUsesLastGoodWhenVerifiedAlternativeFails(t *testing.T) {
	var primaryRequests, alternativeRequests int32
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&primaryRequests, 1) == 1 {
			w.Header().Set("Content-Type", "application/rss+xml")
			_, _ = w.Write([]byte(alternativeRSS("稳定节目", "primary-episode")))
			return
		}
		w.WriteHeader(http.StatusForbidden)
	}))
	defer primary.Close()
	alternative := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&alternativeRequests, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer alternative.Close()

	indexPath := createAlternativeIndexFixture(t, []alternativeIndexRow{
		{id: 1, title: "稳定节目", author: "作者", feedURL: primary.URL + "/primary.xml", itunesID: 456, guid: "podcast-guid-456", dead: 0, status: 403},
		{id: 2, title: "稳定节目", author: "作者", feedURL: alternative.URL + "/alternative.xml", itunesID: 456, guid: "podcast-guid-456", dead: 0, status: 200},
	})
	db := setupTestDB(t)
	service, err := NewServiceWithFeedCoordinator(db, indexPath, newAlternativeCoordinator(primary.URL))
	require.NoError(t, err)
	defer service.Close()

	podcast := &models.Podcast{XYZID: "alternative-fails", Title: "稳定节目", Author: "作者", FeedURL: primary.URL + "/primary.xml", ITunesID: "456", PodcastGUID: "podcast-guid-456"}
	require.NoError(t, db.Create(podcast).Error)
	config := EpisodeSyncConfig{Mode: SyncModeFull, MaxEpisodesPerPodcast: 100, UpdateExisting: true}
	first, err := service.SyncPodcastEpisodesWithContext(t.Context(), podcast.ID, &progressReporter{}, config)
	require.NoError(t, err)
	require.Equal(t, feed.AccessSourcePrimary, first.FeedAccess.SourceType)

	second, err := service.SyncPodcastEpisodesWithContext(t.Context(), podcast.ID, &progressReporter{}, config)
	require.NoError(t, err)
	require.NotNil(t, second.FeedAccess)
	require.Equal(t, feed.AccessSourceLastGood, second.FeedAccess.SourceType)
	require.Equal(t, feed.FreshnessStale, second.FeedAccess.Freshness)
	require.Equal(t, feed.IdentityVerificationUnavailable, second.FeedAccess.IdentityVerification)
	require.Equal(t, int32(2), atomic.LoadInt32(&primaryRequests))
	require.Equal(t, int32(1), atomic.LoadInt32(&alternativeRequests))
}

func TestWorkflowReturnsToPrimaryAfterRecovery(t *testing.T) {
	var primaryRequests, alternativeRequests int32
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&primaryRequests, 1) == 1 {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(alternativeRSS("稳定节目", "recovered-primary-episode")))
	}))
	defer primary.Close()
	alternative := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&alternativeRequests, 1)
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(alternativeRSS("稳定节目", "alternative-episode")))
	}))
	defer alternative.Close()

	indexPath := createAlternativeIndexFixture(t, []alternativeIndexRow{
		{id: 1, title: "稳定节目", author: "作者", feedURL: primary.URL + "/primary.xml", itunesID: 789, guid: "podcast-guid-789", dead: 0, status: 403},
		{id: 2, title: "稳定节目", author: "作者", feedURL: alternative.URL + "/alternative.xml", itunesID: 789, guid: "podcast-guid-789", dead: 0, status: 200},
	})
	db := setupTestDB(t)
	service, err := NewServiceWithFeedCoordinator(db, indexPath, newAlternativeCoordinator(primary.URL))
	require.NoError(t, err)
	defer service.Close()

	podcast := &models.Podcast{XYZID: "primary-recovers", Title: "稳定节目", Author: "作者", FeedURL: primary.URL + "/primary.xml", ITunesID: "789", PodcastGUID: "podcast-guid-789"}
	require.NoError(t, db.Create(podcast).Error)
	config := EpisodeSyncConfig{Mode: SyncModeFull, MaxEpisodesPerPodcast: 100, UpdateExisting: true}
	first, err := service.SyncPodcastEpisodesWithContext(t.Context(), podcast.ID, &progressReporter{}, config)
	require.NoError(t, err)
	require.Equal(t, feed.AccessSourceAlternative, first.FeedAccess.SourceType)

	second, err := service.SyncPodcastEpisodesWithContext(t.Context(), podcast.ID, &progressReporter{}, config)
	require.NoError(t, err)
	require.Equal(t, feed.AccessSourcePrimary, second.FeedAccess.SourceType)
	require.Equal(t, int32(2), atomic.LoadInt32(&primaryRequests))
	require.Equal(t, int32(1), atomic.LoadInt32(&alternativeRequests))
}
