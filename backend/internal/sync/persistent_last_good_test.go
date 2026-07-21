package sync

import (
	"context"
	"testing"
	"time"

	"magicpodcast/internal/feed"

	"github.com/stretchr/testify/require"
)

// TestAttachPersistentLastGoodPersistsSnapshotsToSQLite verifies the production
// wiring: when the feed_snapshots table exists, a successful fetch through a
// coordinator whose store was upgraded by attachPersistentLastGood writes a
// durable row, so a later restart can recover it.
func TestAttachPersistentLastGoodPersistsSnapshotsToSQLite(t *testing.T) {
	db := setupTestDB(t)
	require.NoError(t, db.Exec(feed.FeedSnapshotsCreateTableSQL).Error)
	require.NoError(t, db.Exec(feed.FeedSnapshotsCreateIndexSQL).Error)

	coordinator := feed.NewCoordinator(feed.DefaultCoordinatorConfig())
	attachPersistentLastGood(db, coordinator)

	server := newTestFeedServer(t, 0)
	fetcher := feed.NewFetcherWithCoordinator(2*time.Second, coordinator)
	feedURL := server.URL + "/feed.xml"

	result, err := fetcher.FetchFeedWithContextDetailed(context.Background(), feedURL)
	require.NoError(t, err)
	require.NotNil(t, result.Feed)

	var rows int
	require.NoError(t, db.Raw("SELECT COUNT(*) FROM feed_snapshots").Scan(&rows).Error)
	require.Equal(t, 1, rows, "successful fetch must persist last-good to the durable store")
}

// TestAttachPersistentLastGoodSkipsGracefullyWhenTableMissing verifies the
// fail-safe path: if feed_snapshots is absent the coordinator stays
// in-process-only and fetches still succeed without panicking or writing.
func TestAttachPersistentLastGoodSkipsGracefullyWhenTableMissing(t *testing.T) {
	db := setupTestDB(t) // no feed_snapshots table

	coordinator := feed.NewCoordinator(feed.DefaultCoordinatorConfig())
	attachPersistentLastGood(db, coordinator)

	server := newTestFeedServer(t, 0)
	fetcher := feed.NewFetcherWithCoordinator(2*time.Second, coordinator)
	_, err := fetcher.FetchFeedWithContextDetailed(context.Background(), server.URL+"/feed.xml")
	require.NoError(t, err)
}
