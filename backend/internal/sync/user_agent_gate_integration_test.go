package sync

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"magicpodcast/internal/feed"
	"magicpodcast/internal/models"

	"github.com/stretchr/testify/require"
)

func TestNewServiceWiresPersistentUserAgentGateAcrossJobs(t *testing.T) {
	var primaryHits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveRobotsNotFoundSync(w, r) {
			return
		}
		atomic.AddInt32(&primaryHits, 1)
		w.Header().Set("X-Tengine-Error", "denied by UA ACL = blacklist")
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(server.Close)

	db := setupTestDB(t)
	require.NoError(t, db.Exec(feed.FeedUserAgentGatesCreateTableSQL).Error)
	require.NoError(t, db.Exec(feed.FeedUserAgentGatesCreateIndexSQL).Error)
	require.NoError(t, db.Exec(feed.FeedUserAgentGateRecoveryFeedsCreateTableSQL).Error)
	require.NoError(t, db.Exec(feed.FeedUserAgentGateRecoveryFeedsCreateIndexSQL).Error)
	first := &models.Podcast{XYZID: "persistent-gate-first", Title: "First", FeedURL: server.URL + "/first.xml"}
	second := &models.Podcast{XYZID: "persistent-gate-second", Title: "Second", FeedURL: server.URL + "/second.xml"}
	require.NoError(t, db.Create(first).Error)
	require.NoError(t, db.Create(second).Error)

	service, err := NewServiceWithFeedCoordinator(db, "", newAlternativeCoordinator(server.URL))
	require.NoError(t, err)
	firstResult, firstErr := service.SyncPodcastEpisodesWithContext(context.Background(), first.ID, &progressReporter{}, EpisodeSyncConfig{Mode: SyncModeFull})
	require.Error(t, firstErr)
	require.NotNil(t, firstResult.FeedAccess)
	require.Equal(t, feed.ErrorCategoryUserAgentDenied, firstResult.FeedAccess.ErrorCategory)
	require.NoError(t, service.Close())

	// A newly constructed service represents a new process/job boundary. The
	// persisted domain/fingerprint block must suppress the sibling primary URL.
	service, err = NewServiceWithFeedCoordinator(db, "", newAlternativeCoordinator(server.URL))
	require.NoError(t, err)
	t.Cleanup(func() { _ = service.Close() })
	secondResult, secondErr := service.SyncPodcastEpisodesWithContext(context.Background(), second.ID, &progressReporter{}, EpisodeSyncConfig{Mode: SyncModeFull})
	require.Error(t, secondErr)
	require.NotNil(t, secondResult.FeedAccess)
	require.Equal(t, feed.ErrorCategoryUserAgentBlocked, secondResult.FeedAccess.ErrorCategory)
	require.Equal(t, int32(1), atomic.LoadInt32(&primaryHits))
}
