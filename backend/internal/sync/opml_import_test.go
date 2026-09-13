package sync

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"magicpodcast/internal/feed"
	"magicpodcast/internal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeTestOPML(t *testing.T, xmlURL string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "feeds.opml")
	content := `<?xml version="1.0" encoding="UTF-8"?>
<opml version="2.0"><head><title>t</title></head><body>
  <outline text="Show" title="Show" type="rss" xmlUrl="` + xmlURL + `"/>
</body></opml>`
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func newImportTestServer(t *testing.T, status int, body []byte) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveRobotsNotFoundSync(w, r) {
			return
		}
		w.WriteHeader(status)
		if len(body) > 0 {
			_, _ = w.Write(body)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func TestImportOPMLFromPodcastIndexOnlyRetainsUnreachableSubscription(t *testing.T) {
	server := newImportTestServer(t, http.StatusNotFound, nil)
	db := setupTestDB(t)
	service, err := NewService(db, "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close()) })

	reporter := &recordingReporter{}
	result, err := service.ImportOPMLFromPodcastIndexOnly(writeTestOPML(t, server.URL+"/feed.xml"), reporter)
	require.NoError(t, err)
	assert.Equal(t, 0, result.SuccessPodcasts)

	skips, successes, summaries := reporter.snapshot()
	assert.Empty(t, skips)
	assert.NotContains(t, successes, "成功导入: Show")
	require.Len(t, summaries, 1)
	assert.Equal(t, "import", summaries[0].Operation)
	assert.Equal(t, 1, summaries[0].TotalPodcasts)
	assert.Equal(t, 0, summaries[0].SuccessPodcasts)
	assert.Equal(t, 0, summaries[0].SkippedPodcasts)
	assert.Equal(t, 1, summaries[0].StubPodcasts)
}

func TestImportOPMLFromPodcastIndexOnlyDoesNotCountStubAsSuccess(t *testing.T) {
	server := newImportTestServer(t, http.StatusInternalServerError, []byte("temporary"))
	db := setupTestDB(t)
	service, err := NewService(db, "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close()) })

	reporter := &recordingReporter{}
	result, err := service.ImportOPMLFromPodcastIndexOnly(writeTestOPML(t, server.URL+"/feed.xml"), reporter)
	require.NoError(t, err)
	assert.Equal(t, 0, result.SuccessPodcasts)
	assert.Equal(t, 0, result.FailedPodcasts)

	skips, successes, summaries := reporter.snapshot()
	assert.NotContains(t, successes, "成功导入: Show")
	assert.Empty(t, skips)
	require.Len(t, summaries, 1)
	assert.Equal(t, 0, summaries[0].SuccessPodcasts)
	assert.Equal(t, 1, summaries[0].StubPodcasts)

	var count int64
	require.NoError(t, db.Model(&models.Podcast{}).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

func TestImportOPMLFromPodcastIndexOnlyEmptyFileSendsSummary(t *testing.T) {
	db := setupTestDB(t)
	service, err := NewService(db, "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close()) })

	path := filepath.Join(t.TempDir(), "empty.opml")
	require.NoError(t, os.WriteFile(path, []byte(`<?xml version="1.0" encoding="UTF-8"?>
<opml version="2.0"><head><title>t</title></head><body></body></opml>`), 0o600))

	reporter := &recordingReporter{}
	result, err := service.ImportOPMLFromPodcastIndexOnly(path, reporter)
	require.NoError(t, err)
	assert.Equal(t, 0, result.SuccessPodcasts)

	_, _, summaries := reporter.snapshot()
	require.Len(t, summaries, 1)
	assert.Equal(t, "import", summaries[0].Operation)
	assert.Equal(t, 0, summaries[0].TotalPodcasts)
}

func TestAdditiveImportRetainsSubscriptionsAndPersonalData(t *testing.T) {
	for _, status := range []int{402, 403, 404, 503} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := newImportTestServer(t, status, nil)
			db := setupTestDB(t)
			require.NoError(t, db.AutoMigrate(&models.Tag{}))
			service, err := NewService(db, "")
			require.NoError(t, err)
			defer service.Close()
			path := writeTestOPML(t, server.URL+"/feed.xml")
			result, err := service.ImportOPMLFromPodcastIndexOnly(path, &recordingReporter{})
			require.NoError(t, err)
			require.Equal(t, 1, result.StubPodcasts)
			require.Zero(t, result.SuccessPodcasts)
			var podcast models.Podcast
			require.NoError(t, db.First(&podcast).Error)
			require.True(t, podcast.IsSubscribed)
			require.False(t, podcast.FeedURLValid)
			require.Nil(t, podcast.LastFetchedAt)
			tag := models.Tag{Name: "我的标签"}
			require.NoError(t, db.Create(&tag).Error)
			require.NoError(t, db.Model(&podcast).Association("Tags").Append(&tag))
			require.NoError(t, db.Model(&podcast).Updates(map[string]interface{}{"notes": "保留笔记", "is_subscribed": false, "episode_count": 10, "cover_url": "https://example.com/cover.jpg", "feed_url_valid": true}).Error)
			result, err = service.ImportOPMLFromPodcastIndexOnly(path, &recordingReporter{})
			require.NoError(t, err)
			require.Equal(t, 1, result.StubPodcasts)
			var got models.Podcast
			require.NoError(t, db.Preload("Tags").First(&got, podcast.ID).Error)
			require.True(t, got.IsSubscribed)
			require.False(t, got.FeedURLValid)
			require.Equal(t, "保留笔记", got.Notes)
			require.Equal(t, 10, got.EpisodeCount)
			require.Equal(t, "https://example.com/cover.jpg", got.CoverURL)
			require.Len(t, got.Tags, 1)
			var count int64
			require.NoError(t, db.Model(&models.Podcast{}).Count(&count).Error)
			require.Equal(t, int64(1), count)
		})
	}
}

func TestAdditiveImportIgnoresFoldersAndPreservesAbsentSubscriptions(t *testing.T) {
	server := newImportTestServer(t, 200, []byte(testFeedXML))
	db := setupTestDB(t)
	require.NoError(t, db.AutoMigrate(&models.Tag{}))
	service, err := NewService(db, "")
	require.NoError(t, err)
	defer service.Close()
	absent := models.Podcast{XYZID: "absent", Title: "文件外节目", FeedURL: "https://example.com/absent", IsSubscribed: true, Notes: "原笔记"}
	require.NoError(t, db.Create(&absent).Error)
	path := filepath.Join(t.TempDir(), "nested.opml")
	require.NoError(t, os.WriteFile(path, []byte(`<opml version="2.0"><body><outline text="不要创建这个标签"><outline text="简介" title="节目" xmlUrl="`+server.URL+`/rss"/></outline></body></opml>`), 0600))
	result, err := service.ImportOPMLFromPodcastIndexOnly(path, &recordingReporter{})
	require.NoError(t, err)
	require.Equal(t, 1, result.SuccessPodcasts)
	var got models.Podcast
	require.NoError(t, db.First(&got, absent.ID).Error)
	require.True(t, got.IsSubscribed)
	require.Equal(t, "原笔记", got.Notes)
	var count int64
	require.NoError(t, db.Model(&models.Tag{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestAdditiveImportRejectsMalformedAddresses(t *testing.T) {
	for _, url := range []string{"not-a-url", "file:///etc/passwd", "https://", "javascript:alert(1)"} {
		t.Run(url, func(t *testing.T) {
			db := setupTestDB(t)
			service, err := NewService(db, "")
			require.NoError(t, err)
			defer service.Close()
			result, err := service.ImportOPMLFromPodcastIndexOnly(writeTestOPML(t, url), &recordingReporter{})
			require.NoError(t, err)
			require.Equal(t, 1, result.FailedPodcasts)
			require.Zero(t, result.StubPodcasts)
			var count int64
			require.NoError(t, db.Model(&models.Podcast{}).Count(&count).Error)
			require.Zero(t, count)
		})
	}
}

func TestAdditiveNonStreamingImportKeepsTitleAndPendingStatus(t *testing.T) {
	server := newImportTestServer(t, 404, nil)
	db := setupTestDB(t)
	service, err := NewService(db, "")
	require.NoError(t, err)
	defer service.Close()
	path := filepath.Join(t.TempDir(), "show.opml")
	require.NoError(t, os.WriteFile(path, []byte(`<opml version="2.0"><body><outline title="正确标题" text="这里是长篇节目简介" xmlUrl="`+server.URL+`/rss"/></body></opml>`), 0600))
	result, err := service.ImportOPML(path)
	require.NoError(t, err)
	require.Equal(t, 1, result.StubPodcasts)
	require.Zero(t, result.SuccessPodcasts)
	var got models.Podcast
	require.NoError(t, db.First(&got).Error)
	require.Equal(t, "正确标题", got.Title)
	require.Equal(t, "这里是长篇节目简介", got.Description)
	require.False(t, got.FeedURLValid)
}

func TestAdditiveIndexFailurePreservesExistingContent(t *testing.T) {
	server := newImportTestServer(t, 503, nil)
	db := setupTestDB(t)
	url := server.URL + "/feed.xml"
	index := createAlternativeIndexFixture(t, []alternativeIndexRow{{id: 1, title: "Show", feedURL: url, status: 200}})
	service, err := NewService(db, index)
	require.NoError(t, err)
	defer service.Close()
	existing := models.Podcast{XYZID: "keep", Title: "已有节目", FeedURL: url, EpisodeCount: 100, Notes: "保留"}
	require.NoError(t, db.Create(&existing).Error)
	result, err := service.ImportOPMLFromPodcastIndexOnly(writeTestOPML(t, url), &recordingReporter{})
	require.NoError(t, err)
	require.Equal(t, 1, result.StubPodcasts)
	var got models.Podcast
	require.NoError(t, db.First(&got, existing.ID).Error)
	require.Equal(t, 100, got.EpisodeCount)
	require.Equal(t, "已有节目", got.Title)
	require.Equal(t, "保留", got.Notes)
	result, err = service.ImportOPML(writeTestOPML(t, url))
	require.NoError(t, err)
	require.Equal(t, 1, result.StubPodcasts)
	require.NoError(t, db.First(&got, existing.ID).Error)
	require.Equal(t, 100, got.EpisodeCount)
	require.Equal(t, "保留", got.Notes)
}

func TestPodcastIndexMatchUsesImportRetryPolicy(t *testing.T) {
	var requests int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveRobotsNotFoundSync(w, r) {
			return
		}
		if atomic.AddInt32(&requests, 1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(testFeedXML))
	}))
	t.Cleanup(server.Close)

	db := setupTestDB(t)
	url := server.URL + "/feed.xml"
	index := createAlternativeIndexFixture(t, []alternativeIndexRow{{id: 1, title: "Show", feedURL: url, status: 200}})
	service, err := NewService(db, index)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close()) })
	sleeper := &feed.FakeSleeper{}
	service.applyRetryPolicy(feed.RetryPolicy{
		Budget: 1, Base: 2 * time.Second, Max: 8 * time.Second,
		Sleeper: sleeper, Rand: func() float64 { return 0 },
	})

	result, err := service.ImportOPMLWithProgressAndConfig(writeTestOPML(t, url), NewSilentProgressReporter(nil), ImportConfig{Concurrency: 1})
	require.NoError(t, err)
	require.Equal(t, 1, result.SuccessPodcasts)
	require.Zero(t, result.StubPodcasts)
	require.Equal(t, int32(2), atomic.LoadInt32(&requests))
	require.Len(t, sleeper.Delays(), 1)
}

func TestEmptyValidFeedCountsAsSuccessfulImport(t *testing.T) {
	emptyFeed := []byte(`<?xml version="1.0"?><rss version="2.0"><channel><title>Empty feed</title></channel></rss>`)
	server := newImportTestServer(t, http.StatusOK, emptyFeed)
	db := setupTestDB(t)
	service, err := NewService(db, "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close()) })

	result, err := service.ImportOPMLWithProgressAndConfig(writeTestOPML(t, server.URL+"/feed.xml"), NewSilentProgressReporter(nil), ImportConfig{Concurrency: 1})
	require.NoError(t, err)
	require.Equal(t, 1, result.SuccessPodcasts)
	require.Zero(t, result.StubPodcasts)

	var podcast models.Podcast
	require.NoError(t, db.First(&podcast).Error)
	require.Equal(t, "Empty feed", podcast.Title)
	require.True(t, podcast.FeedURLValid)
}
