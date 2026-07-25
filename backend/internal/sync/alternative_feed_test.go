package sync

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

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

func serveRobotsNotFoundSync(w http.ResponseWriter, r *http.Request) bool {
	if r.URL.Path != "/robots.txt" {
		return false
	}
	w.WriteHeader(http.StatusNotFound)
	return true
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

func TestResolveAlternativeIdentitySkipsPrimaryLookupWhenStableIDIsPersisted(t *testing.T) {
	db := setupTestDB(t)
	indexPath := filepath.Join(t.TempDir(), "empty-podcastindex.db")
	service, err := NewServiceWithFeedCoordinator(db, indexPath, newAlternativeCoordinator("https://feed.example.com/primary.xml"))
	require.NoError(t, err)
	defer service.Close()

	identity, err := service.resolveAlternativeIdentity(&models.Podcast{
		Title:    "已核验节目",
		Author:   "作者",
		FeedURL:  "https://feed.example.com/primary.xml",
		ITunesID: "1612954022",
	})
	require.NoError(t, err)
	require.Equal(t, 1612954022, identity.itunesID)
}

func TestWorkflowUsesVerifiedPodcastIndexAlternativeAndRecordsIdentity(t *testing.T) {
	var primaryRequests, alternativeRequests int32
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveRobotsNotFoundSync(w, r) {
			return
		}
		atomic.AddInt32(&primaryRequests, 1)
		w.WriteHeader(http.StatusForbidden)
	}))
	defer primary.Close()
	alternative := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveRobotsNotFoundSync(w, r) {
			return
		}
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
	coordinator := newAlternativeCoordinator(primary.URL)
	metrics := feed.NewFeedMetrics()
	coordinator.SetMetrics(metrics)
	service, err := NewServiceWithFeedCoordinator(db, indexPath, coordinator)
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
	var sawAlternative bool
	for _, row := range metrics.Snapshot().FeedFetchTotal {
		if row.Source == string(feed.AccessSourceAlternative) && row.Count == 1 {
			sawAlternative = true
		}
	}
	require.True(t, sawAlternative, "the real alternative fetch must be counted as source=alternative")
}

func TestWorkflowReusesEpisodeWhenAlternativeGUIDDiffers(t *testing.T) {
	require.Equal(t, "151", episodeNoFromTitle("E151. 新一期"))
	require.Equal(t, "136", episodeNoFromTitle("【年度巨献】5位顶级脑科学家（E136）"))
	var primaryRequests, alternativeRequests int32
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveRobotsNotFoundSync(w, r) {
			return
		}
		atomic.AddInt32(&primaryRequests, 1)
		w.WriteHeader(http.StatusForbidden)
	}))
	defer primary.Close()
	alternative := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveRobotsNotFoundSync(w, r) {
			return
		}
		atomic.AddInt32(&alternativeRequests, 1)
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:itunes="http://www.itunes.com/dtds/podcast-1.0.dtd"><channel><title>稳定节目</title><description>verified content</description>
<item><title>E150. 替代源标题</title><guid>xmly_track_977481188</guid><pubDate>Mon, 18 May 2026 23:00:00 GMT</pubDate><itunes:episode>5</itunes:episode><description>Details</description></item>
<item><title>E151. 新一期</title><guid>xmly_track_980487841</guid><pubDate>Mon, 1 Jun 2026 23:00:00 GMT</pubDate><itunes:episode>4</itunes:episode><description>Details</description></item>
<item><title>E151. 新一期重复副本</title><guid>xmly_track_980487842</guid><pubDate>Mon, 1 Jun 2026 23:00:00 GMT</pubDate><itunes:episode>4</itunes:episode><description>Details</description></item>
<item><title>E150. 同一期的重新发布</title><guid>xmly_track_977481189</guid><pubDate>Tue, 19 May 2026 23:00:00 GMT</pubDate><itunes:episode>5</itunes:episode><description>Details</description></item>
</channel></rss>`)
	}))
	defer alternative.Close()

	indexPath := createAlternativeIndexFixture(t, []alternativeIndexRow{
		{id: 1, title: "稳定节目", author: "作者", feedURL: primary.URL + "/primary.xml", itunesID: 654, guid: "podcast-guid-654", dead: 0, status: 403},
		{id: 2, title: "稳定节目", author: "作者", feedURL: alternative.URL + "/alternative.xml", itunesID: 654, guid: "podcast-guid-654", dead: 0, status: 200},
	})
	db := setupTestDB(t)
	service, err := NewServiceWithFeedCoordinator(db, indexPath, newAlternativeCoordinator(primary.URL))
	require.NoError(t, err)
	defer service.Close()

	podcast := &models.Podcast{
		XYZID:       "cross-platform-guid",
		Title:       "稳定节目",
		Author:      "作者",
		FeedURL:     primary.URL + "/primary.xml",
		ITunesID:    "654",
		PodcastGUID: "podcast-guid-654",
	}
	require.NoError(t, db.Create(podcast).Error)
	originalPublished := time.Date(2026, 5, 18, 23, 0, 0, 0, time.UTC)
	original := &models.Episode{
		PodcastID:     podcast.ID,
		GUID:          "6a0b0f8f1b7bd50295623a91",
		EpisodeNo:     "5",
		Title:         "E150. 主源标题",
		PublishedDate: originalPublished,
		Notes:         "保留原备注",
		MyRate:        5,
	}
	require.NoError(t, db.Create(original).Error)

	result, err := service.SyncPodcastEpisodesWithContext(t.Context(), podcast.ID, &progressReporter{}, EpisodeSyncConfig{
		Mode:                  SyncModeFull,
		MaxEpisodesPerPodcast: 100,
		UpdateExisting:        true,
	})
	require.NoError(t, err)
	require.Equal(t, 2, result.Created, "only the new episode and the different-date re-release should be created")
	require.Equal(t, 2, result.Skipped, "the existing episode and the same-feed duplicate should be skipped")
	require.Equal(t, 0, result.Errors)
	require.Equal(t, int32(1), atomic.LoadInt32(&primaryRequests))
	require.Equal(t, int32(1), atomic.LoadInt32(&alternativeRequests))

	var episodes []models.Episode
	require.NoError(t, db.Where("podcast_id = ?", podcast.ID).Order("id ASC").Find(&episodes).Error)
	require.Len(t, episodes, 3)
	require.Equal(t, original.ID, episodes[0].ID)
	require.Equal(t, original.GUID, episodes[0].GUID, "alternative GUID must not replace the primary GUID")
	require.Equal(t, original.Title, episodes[0].Title, "alternative title must not replace the primary title")
	require.Equal(t, original.Notes, episodes[0].Notes, "alternative source must not replace user notes")
	require.Equal(t, original.MyRate, episodes[0].MyRate, "alternative source must not replace user rating")
	require.NotContains(t, []string{episodes[0].GUID, episodes[1].GUID, episodes[2].GUID}, "xmly_track_977481188")
	var newEpisode models.Episode
	require.NoError(t, db.Where("guid = ?", "xmly_track_980487841").First(&newEpisode).Error)
	require.Equal(t, "151", newEpisode.EpisodeNo)
	var reRelease models.Episode
	require.NoError(t, db.Where("guid = ?", "xmly_track_977481189").First(&reRelease).Error)
	require.Equal(t, "150", reRelease.EpisodeNo)
}

func TestWorkflowDoesNotDuplicateAlternativeEpisodeWhenPrimaryRecovers(t *testing.T) {
	var primaryRequests, alternativeRequests int32
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveRobotsNotFoundSync(w, r) {
			return
		}
		if atomic.AddInt32(&primaryRequests, 1) == 1 {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = fmt.Fprint(w, `<?xml version="1.0"?><rss version="2.0"><channel><title>恢复节目</title>
<item><title>E151. 主源标题</title><guid>primary-episode-151</guid><pubDate>Mon, 1 Jun 2026 23:00:00 GMT</pubDate></item>
</channel></rss>`)
	}))
	defer primary.Close()
	alternative := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveRobotsNotFoundSync(w, r) {
			return
		}
		atomic.AddInt32(&alternativeRequests, 1)
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = fmt.Fprint(w, `<?xml version="1.0"?><rss version="2.0"><channel><title>恢复节目</title>
<item><title>E151. 替代源标题</title><guid>xmly_track_151</guid><pubDate>Mon, 1 Jun 2026 23:00:00 GMT</pubDate></item>
</channel></rss>`)
	}))
	defer alternative.Close()

	indexPath := createAlternativeIndexFixture(t, []alternativeIndexRow{
		{id: 1, title: "恢复节目", author: "作者", feedURL: primary.URL + "/primary.xml", itunesID: 655, guid: "podcast-guid-655", dead: 0, status: 403},
		{id: 2, title: "恢复节目", author: "作者", feedURL: alternative.URL + "/alternative.xml", itunesID: 655, guid: "podcast-guid-655", dead: 0, status: 200},
	})
	db := setupTestDB(t)
	service, err := NewServiceWithFeedCoordinator(db, indexPath, newAlternativeCoordinator(primary.URL))
	require.NoError(t, err)
	defer service.Close()

	podcast := &models.Podcast{
		XYZID:       "alternative-first",
		Title:       "恢复节目",
		Author:      "作者",
		FeedURL:     primary.URL + "/primary.xml",
		ITunesID:    "655",
		PodcastGUID: "podcast-guid-655",
	}
	require.NoError(t, db.Create(podcast).Error)
	config := EpisodeSyncConfig{Mode: SyncModeFull, MaxEpisodesPerPodcast: 100, UpdateExisting: true}

	first, err := service.SyncPodcastEpisodesWithContext(t.Context(), podcast.ID, &progressReporter{}, config)
	require.NoError(t, err)
	require.Equal(t, feed.AccessSourceAlternative, first.FeedAccess.SourceType)
	require.Equal(t, 1, first.Created)

	second, err := service.SyncPodcastEpisodesWithContext(t.Context(), podcast.ID, &progressReporter{}, config)
	require.NoError(t, err)
	require.Equal(t, feed.AccessSourcePrimary, second.FeedAccess.SourceType)
	require.Equal(t, 0, second.Created)
	require.Equal(t, 1, second.Skipped)
	require.Equal(t, int32(2), atomic.LoadInt32(&primaryRequests))
	require.Equal(t, int32(1), atomic.LoadInt32(&alternativeRequests))

	var count int64
	require.NoError(t, db.Model(&models.Episode{}).Where("podcast_id = ?", podcast.ID).Count(&count).Error)
	require.Equal(t, int64(1), count)
}

func TestWorkflowUsesVerifiedAlternativeWhenPrimaryIsAbsentFromPodcastIndex(t *testing.T) {
	var primaryRequests, alternativeRequests int32
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveRobotsNotFoundSync(w, r) {
			return
		}
		atomic.AddInt32(&primaryRequests, 1)
		w.WriteHeader(http.StatusForbidden)
	}))
	defer primary.Close()
	alternative := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveRobotsNotFoundSync(w, r) {
			return
		}
		atomic.AddInt32(&alternativeRequests, 1)
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(alternativeRSS("科学星球", "alternative-science-episode")))
	}))
	defer alternative.Close()

	indexPath := createAlternativeIndexFixture(t, []alternativeIndexRow{
		{id: 1, title: "科学星球", author: "BOX孙彬", feedURL: alternative.URL + "/science.xml", itunesID: 1612954022, guid: "alternative-science-guid", dead: 0, status: 200},
	})
	db := setupTestDB(t)
	service, err := NewServiceWithFeedCoordinator(db, indexPath, newAlternativeCoordinator(primary.URL))
	require.NoError(t, err)
	defer service.Close()

	podcast := &models.Podcast{
		XYZID:    "primary-absent-from-index",
		Title:    "科学星球",
		Author:   "孙彬_BIMBOX",
		FeedURL:  primary.URL + "/primary.xml",
		ITunesID: "1612954022",
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
	require.Equal(t, alternative.URL+"/science.xml", result.FeedAccess.SourceURL)
	require.Equal(t, feed.IdentityVerificationVerifiedMetadata, result.FeedAccess.IdentityVerification)
	require.Equal(t, int32(1), atomic.LoadInt32(&primaryRequests))
	require.Equal(t, int32(1), atomic.LoadInt32(&alternativeRequests))
}

func TestWorkflowRejectsSameTitleCandidateWithoutStableIdentity(t *testing.T) {
	var primaryRequests, alternativeRequests int32
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveRobotsNotFoundSync(w, r) {
			return
		}
		atomic.AddInt32(&primaryRequests, 1)
		w.WriteHeader(http.StatusForbidden)
	}))
	defer primary.Close()
	alternative := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveRobotsNotFoundSync(w, r) {
			return
		}
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

// TestWorkflowFailsWhenPrimaryAndVerifiedAlternativeFail locks #35/#36:
// when both primary and verified alternative fail, last-good is NOT treated as
// this-batch success.
func TestWorkflowFailsWhenPrimaryAndVerifiedAlternativeFail(t *testing.T) {
	var primaryRequests, alternativeRequests int32
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveRobotsNotFoundSync(w, r) {
			return
		}
		if atomic.AddInt32(&primaryRequests, 1) == 1 {
			w.Header().Set("Content-Type", "application/rss+xml")
			_, _ = w.Write([]byte(alternativeRSS("稳定节目", "primary-episode")))
			return
		}
		w.WriteHeader(http.StatusForbidden)
	}))
	defer primary.Close()
	alternative := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveRobotsNotFoundSync(w, r) {
			return
		}
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
	require.Error(t, err, "primary+alternative failure must not succeed via last-good")
	require.NotNil(t, second.FeedAccess)
	require.NotEqual(t, feed.AccessSourceLastGood, second.FeedAccess.SourceType)
	require.Equal(t, int32(2), atomic.LoadInt32(&primaryRequests))
	require.Equal(t, int32(1), atomic.LoadInt32(&alternativeRequests))
}

func TestWorkflowReturnsToPrimaryAfterRecovery(t *testing.T) {
	var primaryRequests, alternativeRequests int32
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveRobotsNotFoundSync(w, r) {
			return
		}
		if atomic.AddInt32(&primaryRequests, 1) == 1 {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(alternativeRSS("稳定节目", "recovered-primary-episode")))
	}))
	defer primary.Close()
	alternative := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveRobotsNotFoundSync(w, r) {
			return
		}
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
