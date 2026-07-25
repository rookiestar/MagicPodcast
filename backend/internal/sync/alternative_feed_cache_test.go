package sync

import (
	"context"
	"net/http"
	"net/http/httptest"
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
