package sync

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"magicpodcast/internal/feed"
	"magicpodcast/internal/models"

	"github.com/stretchr/testify/require"
)

// TestAlternativeCacheHitSkipsIndexRequeryAndNeverRewritesMainFeed locks #37:
// a pre-verified cache entry is used on primary failure without permanent
// main Feed rewrite.
func TestAlternativeCacheHitSkipsIndexRequeryAndNeverRewritesMainFeed(t *testing.T) {
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
		_, _ = w.Write([]byte(alternativeRSS("缓存节目", "cache-ep")))
	}))
	defer alternative.Close()

	mainURL := primary.URL + "/primary.xml"
	altURL := alternative.URL + "/alternative.xml"
	indexPath := createAlternativeIndexFixture(t, []alternativeIndexRow{
		{id: 1, title: "缓存节目", author: "作者", feedURL: mainURL, itunesID: 111, guid: "guid-cache-111", dead: 0, status: 403},
		{id: 2, title: "缓存节目", author: "作者", feedURL: altURL, itunesID: 111, guid: "guid-cache-111", dead: 0, status: 200},
	})
	db := setupTestDB(t)
	service, err := NewServiceWithFeedCoordinator(db, indexPath, newAlternativeCoordinator(primary.URL))
	require.NoError(t, err)
	defer service.Close()

	podcast := &models.Podcast{
		XYZID: "alt-cache-hit", Title: "缓存节目", Author: "作者",
		FeedURL: mainURL, ITunesID: "111", PodcastGUID: "guid-cache-111",
	}
	require.NoError(t, db.Create(podcast).Error)

	// Warm cache via explicit pre-verify.
	service.EnsureAlternativeVerified(context.Background(), podcast)
	var cached models.PodcastAlternativeFeed
	require.NoError(t, db.Where("podcast_id = ?", podcast.ID).First(&cached).Error)
	require.Equal(t, models.AlternativeCacheVerified, cached.Status)
	require.Equal(t, altURL, cached.AlternativeFeedURL)

	// Drop the PodcastIndex so a cache miss would fail.
	service.podcastIndexQuery = nil

	result, err := service.SyncPodcastEpisodesWithContext(context.Background(), podcast.ID, &progressReporter{}, EpisodeSyncConfig{
		Mode: SyncModeFull, MaxEpisodesPerPodcast: 100, UpdateExisting: true,
	})
	require.NoError(t, err)
	require.Equal(t, feed.AccessSourceAlternative, result.FeedAccess.SourceType)
	require.Equal(t, int32(1), atomic.LoadInt32(&primaryRequests))
	require.GreaterOrEqual(t, atomic.LoadInt32(&alternativeRequests), int32(1))

	var refreshed models.Podcast
	require.NoError(t, db.First(&refreshed, podcast.ID).Error)
	require.Equal(t, mainURL, refreshed.FeedURL, "alternative must never permanently replace main feed")
}

// TestSuccessfulPrimaryRefreshPrewarmsVerifiedAlternative locks the #47
// production seam: a healthy primary refresh must prepare the verified
// fallback before a later failure window, without changing the subscribed
// primary Feed URL.
func TestSuccessfulPrimaryRefreshPrewarmsVerifiedAlternative(t *testing.T) {
	var primaryRequests, alternativeRequests int32
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveRobotsNotFoundSync(w, r) {
			return
		}
		atomic.AddInt32(&primaryRequests, 1)
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(alternativeRSS("预热节目", "primary-warm-episode")))
	}))
	t.Cleanup(primary.Close)
	alternative := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveRobotsNotFoundSync(w, r) {
			return
		}
		atomic.AddInt32(&alternativeRequests, 1)
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(alternativeRSS("预热节目", "alternative-warm-episode")))
	}))
	t.Cleanup(alternative.Close)

	mainURL := primary.URL + "/primary.xml"
	altURL := alternative.URL + "/alternative.xml"
	indexPath := createAlternativeIndexFixture(t, []alternativeIndexRow{
		{id: 1, title: "预热节目", author: "作者", feedURL: mainURL, itunesID: 777, guid: "warm-guid-777", dead: 0, status: http.StatusOK},
		{id: 2, title: "预热节目", author: "作者", feedURL: altURL, itunesID: 777, guid: "warm-guid-777", dead: 0, status: http.StatusOK},
	})
	db := setupTestDB(t)
	service, err := NewServiceWithFeedCoordinator(db, indexPath, newAlternativeCoordinator(primary.URL))
	require.NoError(t, err)
	t.Cleanup(func() { _ = service.Close() })

	podcast := &models.Podcast{
		XYZID:        "alternative-prewarm",
		Title:        "预热节目",
		Author:       "作者",
		FeedURL:      mainURL,
		ITunesID:     "777",
		PodcastGUID:  "warm-guid-777",
		IsSubscribed: true,
	}
	require.NoError(t, db.Create(podcast).Error)

	result, err := service.SyncPodcastEpisodesWithContext(t.Context(), podcast.ID, &progressReporter{}, EpisodeSyncConfig{
		Mode:                  SyncModeFull,
		MaxEpisodesPerPodcast: 100,
		UpdateExisting:        true,
	})
	require.NoError(t, err)
	require.NotNil(t, result.FeedAccess)
	require.Equal(t, feed.AccessSourcePrimary, result.FeedAccess.SourceType)
	require.Equal(t, int32(1), atomic.LoadInt32(&primaryRequests))

	require.Eventually(t, func() bool {
		var cached models.PodcastAlternativeFeed
		if err := db.Where("podcast_id = ?", podcast.ID).First(&cached).Error; err != nil {
			return false
		}
		return cached.Status == models.AlternativeCacheVerified && cached.AlternativeFeedURL == altURL
	}, 2*time.Second, 10*time.Millisecond, "healthy primary refresh must prewarm the verified alternative")
	// The fixture already has independent PodcastIndex metadata evidence, so
	// prewarming verifies and caches the URL without downloading it. The later
	// failure path owns the actual alternative Feed fetch.
	require.Zero(t, atomic.LoadInt32(&alternativeRequests))

	var refreshed models.Podcast
	require.NoError(t, db.First(&refreshed, podcast.ID).Error)
	require.Equal(t, mainURL, refreshed.FeedURL)
}

func TestMetadataRefreshPrewarmsVerifiedAlternative(t *testing.T) {
	var primaryRequests, alternativeRequests int32
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveRobotsNotFoundSync(w, r) {
			return
		}
		atomic.AddInt32(&primaryRequests, 1)
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(alternativeRSS("元数据预热节目", "metadata-primary-episode")))
	}))
	t.Cleanup(primary.Close)
	alternative := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveRobotsNotFoundSync(w, r) {
			return
		}
		atomic.AddInt32(&alternativeRequests, 1)
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(alternativeRSS("元数据预热节目", "metadata-alternative-episode")))
	}))
	t.Cleanup(alternative.Close)

	mainURL := primary.URL + "/primary.xml"
	altURL := alternative.URL + "/alternative.xml"
	indexPath := createAlternativeIndexFixture(t, []alternativeIndexRow{
		{id: 1, title: "元数据预热节目", author: "作者", feedURL: mainURL, itunesID: 778, guid: "metadata-guid-778", dead: 0, status: http.StatusOK},
		{id: 2, title: "元数据预热节目", author: "作者", feedURL: altURL, itunesID: 778, guid: "metadata-guid-778", dead: 0, status: http.StatusOK},
	})
	db := setupTestDB(t)
	service, err := NewServiceWithFeedCoordinator(db, indexPath, newAlternativeCoordinator(primary.URL))
	require.NoError(t, err)
	t.Cleanup(func() { _ = service.Close() })

	podcast := &models.Podcast{
		XYZID:        "metadata-prewarm",
		Title:        "元数据预热节目",
		Author:       "作者",
		FeedURL:      mainURL,
		ITunesID:     "778",
		PodcastGUID:  "metadata-guid-778",
		IsSubscribed: true,
	}
	require.NoError(t, db.Create(podcast).Error)

	require.NoError(t, service.SyncPodcastsMetadataSSE(NewSilentProgressReporter(nil)))
	require.Eventually(t, func() bool {
		var cached models.PodcastAlternativeFeed
		if err := db.Where("podcast_id = ?", podcast.ID).First(&cached).Error; err != nil {
			return false
		}
		return cached.Status == models.AlternativeCacheVerified && cached.AlternativeFeedURL == altURL
	}, 2*time.Second, 10*time.Millisecond, "metadata refresh must prewarm the verified alternative")
	require.Equal(t, int32(1), atomic.LoadInt32(&primaryRequests))
	require.Zero(t, atomic.LoadInt32(&alternativeRequests), "prewarm with independent metadata evidence must not download the candidate")
}

// TestAlternativeCacheInvalidatedWhenMainFeedChanges locks #37 import/update
// invalidation: changing main Feed URL drops the old alternative binding.
func TestAlternativeCacheInvalidatedWhenMainFeedChanges(t *testing.T) {
	db := setupTestDB(t)
	service, err := NewServiceWithFeedCoordinator(db, "", newAlternativeCoordinator("https://old.example.com"))
	require.NoError(t, err)
	defer service.Close()

	podcast := &models.Podcast{XYZID: "alt-invalidate", Title: "节目", FeedURL: "https://old.example.com/feed.xml", ITunesID: "1", PodcastGUID: "g1"}
	require.NoError(t, db.Create(podcast).Error)
	require.NoError(t, db.Create(&models.PodcastAlternativeFeed{
		PodcastID:          podcast.ID,
		MainFeedURL:        feed.CanonicalizeURL(podcast.FeedURL),
		IdentityKey:        "1|g1",
		AlternativeFeedURL: "https://alt.example.com/feed.xml",
		Status:             models.AlternativeCacheVerified,
		Verification:       feed.IdentityVerificationVerifiedMetadata,
	}).Error)

	require.NoError(t, service.UpdatePodcastMainFeed(podcast.ID, "https://new.example.com/feed.xml"))

	var count int64
	require.NoError(t, db.Model(&models.PodcastAlternativeFeed{}).Where("podcast_id = ?", podcast.ID).Count(&count).Error)
	require.Zero(t, count, "main feed change must invalidate alternative cache")

	var refreshed models.Podcast
	require.NoError(t, db.First(&refreshed, podcast.ID).Error)
	require.Equal(t, "https://new.example.com/feed.xml", refreshed.FeedURL)
}

func TestOPMLUpdateInvalidatesAlternativeCacheWhenStableIdentityChanges(t *testing.T) {
	var feedRequests int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveRobotsNotFoundSync(w, r) {
			return
		}
		atomic.AddInt32(&feedRequests, 1)
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(`<?xml version="1.0"?>
<rss version="2.0" xmlns:itunes="http://www.itunes.com/dtds/podcast-1.0.dtd" xmlns:podcast="https://podcastindex.org/namespace/1.0">
  <channel><title>更新后的节目</title><description>metadata</description>
    <itunes:author>作者</itunes:author><itunes:id>202</itunes:id><podcast:guid>guid-new</podcast:guid>
    <item><title>最新单集</title><guid>episode-new</guid><pubDate>Tue, 14 Jul 2026 08:00:00 GMT</pubDate></item>
  </channel>
</rss>`))
	}))
	t.Cleanup(server.Close)

	db := setupTestDB(t)
	service, err := NewServiceWithFeedCoordinator(db, filepath.Join(t.TempDir(), "missing", "index.db"), newAlternativeCoordinator(server.URL))
	require.NoError(t, err)
	t.Cleanup(func() { _ = service.Close() })

	mainURL := server.URL + "/feed.xml"
	podcast := &models.Podcast{
		XYZID: "opml-identity-change", Title: "旧节目", FeedURL: mainURL,
		ITunesID: "101", PodcastGUID: "guid-old", IsSubscribed: true,
	}
	require.NoError(t, db.Create(podcast).Error)
	require.NoError(t, db.Create(&models.PodcastAlternativeFeed{
		PodcastID:          podcast.ID,
		MainFeedURL:        feed.CanonicalizeURL(mainURL),
		IdentityKey:        "101|guid-old",
		AlternativeFeedURL: "https://alternative.example/feed.xml",
		Status:             models.AlternativeCacheVerified,
		Verification:       feed.IdentityVerificationVerifiedMetadata,
		VerifiedAt:         time.Now(),
	}).Error)

	opmlPath := filepath.Join(t.TempDir(), "subscriptions.opml")
	require.NoError(t, os.WriteFile(opmlPath, []byte(`<?xml version="1.0" encoding="UTF-8"?>
<opml version="2.0"><head><title>subscriptions</title></head><body>
  <outline text="更新后的节目" title="更新后的节目" type="rss" xmlUrl="`+mainURL+`"/>
</body></opml>`), 0o600))

	result, err := service.ImportOPMLWithProgressAndConfig(opmlPath, NewSilentProgressReporter(nil), ImportConfig{Concurrency: 1})
	require.NoError(t, err)
	require.Equal(t, 1, result.SuccessPodcasts)
	require.Equal(t, int32(1), atomic.LoadInt32(&feedRequests))

	var refreshed models.Podcast
	require.NoError(t, db.First(&refreshed, podcast.ID).Error)
	require.Equal(t, "202", refreshed.ITunesID)
	require.Equal(t, "guid-new", refreshed.PodcastGUID)

	var cacheCount int64
	require.NoError(t, db.Model(&models.PodcastAlternativeFeed{}).Where("podcast_id = ?", podcast.ID).Count(&cacheCount).Error)
	require.Zero(t, cacheCount, "an OPML identity update must not retain an alternative bound to the old identity")
}

// TestAlternativeIdentityConflictIsCachedAsUnavailable records conflict reasons
// so the next failure window does not re-open a title-confused switch.
func TestAlternativeIdentityConflictIsCachedAsUnavailable(t *testing.T) {
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveRobotsNotFoundSync(w, r) {
			return
		}
		w.WriteHeader(http.StatusForbidden)
	}))
	defer primary.Close()
	alt := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveRobotsNotFoundSync(w, r) {
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(alternativeRSS("冲突节目", "x")))
	}))
	defer alt.Close()

	// Candidate shares itunes id but contradicts podcast GUID → conflict.
	indexPath := createAlternativeIndexFixture(t, []alternativeIndexRow{
		{id: 1, title: "冲突节目", author: "作者", feedURL: primary.URL + "/p.xml", itunesID: 222, guid: "guid-local", dead: 0, status: 403},
		{id: 2, title: "冲突节目", author: "作者", feedURL: alt.URL + "/a.xml", itunesID: 222, guid: "guid-other", dead: 0, status: 200},
	})
	db := setupTestDB(t)
	service, err := NewServiceWithFeedCoordinator(db, indexPath, newAlternativeCoordinator(primary.URL))
	require.NoError(t, err)
	defer service.Close()

	podcast := &models.Podcast{
		XYZID: "alt-conflict", Title: "冲突节目", Author: "作者",
		FeedURL: primary.URL + "/p.xml", ITunesID: "222", PodcastGUID: "guid-local",
	}
	require.NoError(t, db.Create(podcast).Error)

	result, err := service.SyncPodcastEpisodesWithContext(context.Background(), podcast.ID, &progressReporter{}, EpisodeSyncConfig{Mode: SyncModeFull})
	require.Error(t, err)
	require.Equal(t, feed.IdentityVerificationRejectedConflict, result.FeedAccess.IdentityVerification)

	var cached models.PodcastAlternativeFeed
	require.NoError(t, db.Where("podcast_id = ?", podcast.ID).First(&cached).Error)
	require.Equal(t, models.AlternativeCacheUnavailable, cached.Status)
	require.Equal(t, "identity_conflict", cached.UnavailableReason)
}

func TestAlternativeAmbiguousCandidatesAreRejectedAndCached(t *testing.T) {
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveRobotsNotFoundSync(w, r) {
			return
		}
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(primary.Close)

	mainURL := primary.URL + "/primary.xml"
	altOneURL := "https://alternative-one.example/feed.xml"
	altTwoURL := "https://alternative-two.example/feed.xml"
	indexPath := createAlternativeIndexFixture(t, []alternativeIndexRow{
		{id: 1, title: "歧义节目", author: "作者", feedURL: mainURL, itunesID: 223, guid: "ambiguous-guid-223", dead: 0, status: http.StatusForbidden},
		{id: 2, title: "歧义节目", author: "作者", feedURL: altOneURL, itunesID: 223, guid: "ambiguous-guid-223", dead: 0, status: http.StatusOK},
		{id: 3, title: "歧义节目", author: "作者", feedURL: altTwoURL, itunesID: 223, guid: "ambiguous-guid-223", dead: 0, status: http.StatusOK},
	})
	db := setupTestDB(t)
	service, err := NewServiceWithFeedCoordinator(db, indexPath, newAlternativeCoordinator(primary.URL))
	require.NoError(t, err)
	t.Cleanup(func() { _ = service.Close() })

	podcast := &models.Podcast{
		XYZID: "alt-ambiguous", Title: "歧义节目", Author: "作者",
		FeedURL: mainURL, ITunesID: "223", PodcastGUID: "ambiguous-guid-223",
	}
	require.NoError(t, db.Create(podcast).Error)

	result, err := service.SyncPodcastEpisodesWithContext(t.Context(), podcast.ID, &progressReporter{}, EpisodeSyncConfig{Mode: SyncModeFull})
	require.Error(t, err)
	require.NotNil(t, result.FeedAccess)
	require.Equal(t, feed.IdentityVerificationRejectedAmbiguous, result.FeedAccess.IdentityVerification)

	var cached models.PodcastAlternativeFeed
	require.NoError(t, db.Where("podcast_id = ?", podcast.ID).First(&cached).Error)
	require.Equal(t, models.AlternativeCacheUnavailable, cached.Status)
	require.Equal(t, "ambiguous_candidates", cached.UnavailableReason)
}

func TestAlternativeIdentityQueryHonorsCanceledContext(t *testing.T) {
	indexPath := createAlternativeIndexFixture(t, []alternativeIndexRow{
		{id: 1, title: "超时节目", author: "作者", feedURL: "https://primary.example/feed.xml", itunesID: 224, guid: "timeout-guid-224", dead: 0, status: http.StatusForbidden},
	})
	db := setupTestDB(t)
	service, err := NewServiceWithFeedCoordinator(db, indexPath, newAlternativeCoordinator("https://primary.example"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = service.Close() })

	podcast := &models.Podcast{
		XYZID: "alt-query-timeout", Title: "超时节目", Author: "作者",
		FeedURL: "https://primary.example/feed.xml", ITunesID: "224", PodcastGUID: "timeout-guid-224",
	}
	require.NoError(t, db.Create(podcast).Error)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	started := time.Now()
	_, verification, reason, ok := service.verifyAndCacheAlternative(ctx, podcast)
	require.False(t, ok)
	require.Equal(t, feed.IdentityVerificationUnavailable, verification)
	require.Equal(t, "live_query_timeout", reason)
	require.Less(t, time.Since(started), time.Second, "a canceled PodcastIndex lookup must return within the caller budget")

	var cached models.PodcastAlternativeFeed
	require.NoError(t, db.Where("podcast_id = ?", podcast.ID).First(&cached).Error)
	require.Equal(t, "live_query_timeout", cached.UnavailableReason)
}

func TestTransientUnavailableCacheExpiresBeforeBatchRetry(t *testing.T) {
	db := setupTestDB(t)
	service, err := NewServiceWithFeedCoordinator(db, "", newAlternativeCoordinator("https://feed.example.com/main.xml"))
	require.NoError(t, err)
	defer service.Close()

	podcast := &models.Podcast{
		XYZID:       "transient-alt-cache",
		Title:       "Transient",
		FeedURL:     "https://feed.example.com/main.xml",
		ITunesID:    "123",
		PodcastGUID: "guid-123",
	}
	require.NoError(t, db.Create(podcast).Error)
	identity := alternativeIdentity{itunesID: 123, podcastGUID: "guid-123"}
	require.NoError(t, db.Create(&models.PodcastAlternativeFeed{
		PodcastID:         podcast.ID,
		MainFeedURL:       feed.CanonicalizeURL(podcast.FeedURL),
		IdentityKey:       identity.key(),
		Status:            models.AlternativeCacheUnavailable,
		Verification:      feed.IdentityVerificationUnavailable,
		UnavailableReason: "live_query_timeout",
		VerifiedAt:        time.Now().Add(-2 * time.Minute),
	}).Error)

	_, ok := service.loadAlternativeCache(
		podcast.ID,
		feed.CanonicalizeURL(podcast.FeedURL),
		identity.key(),
	)
	require.False(t, ok, "transient failure must be retried within the 10-minute batch")
}
