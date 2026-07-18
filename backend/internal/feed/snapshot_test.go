package feed

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCoordinatorReusesFreshSnapshotWithoutContactingUpstream(t *testing.T) {
	var requestCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(testSnapshotFeed("Fresh snapshot")))
	}))
	t.Cleanup(server.Close)

	store := NewMemorySnapshotStore(LastGoodStoreConfig{})
	policy := DomainPolicy{MaxConcurrency: 1, MinRefreshInterval: time.Minute}
	firstCoordinator := NewCoordinator(CoordinatorConfig{
		DomainPolicies: map[string]DomainPolicy{TargetDomain(server.URL): policy},
		LastGoodStore:  store,
	})
	firstFetcher := NewFetcherWithCoordinator(2*time.Second, firstCoordinator)
	feedURL := server.URL + "/feed.xml"

	first, err := firstFetcher.FetchFeedWithContextDetailed(context.Background(), feedURL)
	require.NoError(t, err)
	require.Equal(t, AccessSourcePrimary, first.Access.SourceType)
	require.NotNil(t, first.Access.RetrievedAt)
	snapshot, ok := store.Load(feedURL)
	require.True(t, ok)
	require.NotEmpty(t, snapshot.Fingerprint)
	require.NotEmpty(t, snapshot.RawContent)

	secondCoordinator := NewCoordinator(CoordinatorConfig{
		DomainPolicies: map[string]DomainPolicy{TargetDomain(server.URL): policy},
		LastGoodStore:  store,
	})
	second, err := NewFetcherWithCoordinator(2*time.Second, secondCoordinator).FetchFeedWithContextDetailed(context.Background(), feedURL)
	require.NoError(t, err)
	require.Equal(t, "Fresh snapshot", second.Feed.Title)
	require.Equal(t, AccessSourceLocalCache, second.Access.SourceType)
	require.Equal(t, CacheStatusHit, second.Access.CacheStatus)
	require.Equal(t, FreshnessFresh, second.Access.Freshness)
	require.NotNil(t, second.Access.RetrievedAt)
	require.Equal(t, int32(1), atomic.LoadInt32(&requestCount))
}

func TestFetcherCanReturnStaleLastGoodWhilePreservingUpstreamFailure(t *testing.T) {
	var requestCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&requestCount, 1) == 1 {
			w.Header().Set("Content-Type", "application/rss+xml")
			_, _ = w.Write([]byte(testSnapshotFeed("Last good")))
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(server.Close)

	store := NewMemorySnapshotStore(LastGoodStoreConfig{})
	coordinator := NewCoordinator(CoordinatorConfig{
		DomainPolicies: map[string]DomainPolicy{TargetDomain(server.URL): {MaxConcurrency: 1}},
		LastGoodStore:  store,
	})
	fetcher := NewFetcherWithCoordinator(2*time.Second, coordinator)
	feedURL := server.URL + "/feed.xml"

	_, err := fetcher.FetchFeedWithContextDetailed(context.Background(), feedURL)
	require.NoError(t, err)
	failure, err := fetcher.FetchFeedWithContextDetailed(context.Background(), feedURL)
	require.Error(t, err)
	require.Equal(t, ErrorCategoryServiceUnavailable, failure.Access.ErrorCategory)

	fallback, ok := fetcher.FetchLastGoodWithContext(context.Background(), feedURL, failure)
	require.True(t, ok)
	require.NoError(t, fallback.Error)
	require.Equal(t, "Last good", fallback.Feed.Title)
	require.Equal(t, AccessSourceLastGood, fallback.Access.SourceType)
	require.Equal(t, CacheStatusHit, fallback.Access.CacheStatus)
	require.Equal(t, FreshnessStale, fallback.Access.Freshness)
	require.Equal(t, ErrorCategoryServiceUnavailable, fallback.Access.ErrorCategory)
	require.NotNil(t, fallback.Access.RetrievedAt)
	require.Equal(t, int32(2), atomic.LoadInt32(&requestCount))
}

func TestMemorySnapshotStoreEnforcesEntryResponseAndTotalBounds(t *testing.T) {
	store := NewMemorySnapshotStore(LastGoodStoreConfig{
		MaxEntries:       1,
		MaxResponseBytes: 128,
		MaxTotalBytes:    128,
	})
	retrievedAt := time.Date(2026, 7, 19, 1, 2, 3, 0, time.UTC)
	first := FeedSnapshot{FeedURL: "https://example.test/one.xml", RetrievedAt: retrievedAt, RawContent: []byte("first")}
	second := FeedSnapshot{FeedURL: "https://example.test/two.xml", RetrievedAt: retrievedAt.Add(time.Second), RawContent: []byte("second")}
	require.NoError(t, store.Save(first))
	require.NoError(t, store.Save(second))

	stats := store.Stats()
	require.Equal(t, 1, stats.Entries)
	require.LessOrEqual(t, stats.TotalBytes, int64(128))
	_, firstPresent := store.Load(first.FeedURL)
	_, secondPresent := store.Load(second.FeedURL)
	require.False(t, firstPresent)
	require.True(t, secondPresent)

	tooLarge := FeedSnapshot{FeedURL: "https://example.test/large.xml", RetrievedAt: retrievedAt, RawContent: make([]byte, 129)}
	require.ErrorIs(t, store.Save(tooLarge), ErrSnapshotResponseTooLarge)
	stats = store.Stats()
	require.Equal(t, 1, stats.Entries)
	require.LessOrEqual(t, stats.TotalBytes, int64(128))
}

func TestCorruptLastGoodSnapshotIsIgnored(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(server.Close)

	store := &corruptSnapshotStore{}
	coordinator := NewCoordinator(CoordinatorConfig{
		DomainPolicies: map[string]DomainPolicy{TargetDomain(server.URL): {MaxConcurrency: 1}},
		LastGoodStore:  store,
	})
	fetcher := NewFetcherWithCoordinator(2*time.Second, coordinator)
	feedURL := server.URL + "/feed.xml"
	failure, err := fetcher.FetchFeedWithContextDetailed(context.Background(), feedURL)
	require.Error(t, err)

	fallback, ok := fetcher.FetchLastGoodWithContext(context.Background(), feedURL, failure)
	require.False(t, ok)
	require.Nil(t, fallback)
}

type corruptSnapshotStore struct{}

func (s *corruptSnapshotStore) Save(FeedSnapshot) error { return nil }

func (s *corruptSnapshotStore) Load(feedURL string) (*FeedSnapshot, bool) {
	return &FeedSnapshot{
		FeedURL:     CanonicalizeURL(feedURL),
		RetrievedAt: time.Now(),
		Fingerprint: "not-the-fingerprint",
		RawContent:  []byte("this is not a feed"),
	}, true
}

func (s *corruptSnapshotStore) Stats() SnapshotStoreStats { return SnapshotStoreStats{} }

func testSnapshotFeed(title string) string {
	return fmt.Sprintf(`<?xml version="1.0"?><rss version="2.0"><channel><title>%s</title><item><title>Episode</title><guid>%s-episode</guid><pubDate>Tue, 14 Jul 2026 08:00:00 GMT</pubDate><description>Details</description></item></channel></rss>`, title, title)
}
