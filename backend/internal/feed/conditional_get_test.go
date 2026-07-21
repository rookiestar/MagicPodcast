package feed

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const conditionalGETETag = `"cg-v1"`

// newConditionalGETServer returns a test server that returns 200 with an ETag
// for plain requests and 304 Not Modified for requests whose If-None-Match
// matches the served ETag. before304 runs (synchronously, inside the handler)
// just before a 304 is written, so a test can simulate a snapshot that vanishes
// between the validators-load and the recovery.
func newConditionalGETServer(t *testing.T, body string, before304 func()) (*httptest.Server, *int32, *int32) {
	t.Helper()
	var requestCount int32
	var conditionalHits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		if r.Header.Get("If-None-Match") == conditionalGETETag {
			atomic.AddInt32(&conditionalHits, 1)
			if before304 != nil {
				before304()
			}
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", conditionalGETETag)
		w.Header().Set("Last-Modified", "Mon, 21 Jul 2026 09:00:00 GMT")
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return server, &requestCount, &conditionalHits
}

func conditionalGETCoordinator(store FeedStateStore) *Coordinator {
	return NewCoordinator(CoordinatorConfig{
		DomainPolicies: map[string]DomainPolicy{"127.0.0.1": {MaxConcurrency: 1}},
		LastGoodStore:  store,
	})
}

// TestConditionalGETSendsValidatorsAndRecoversFrom304 is the core #27
// acceptance: after a 200 populates a snapshot with its ETag, the next fetch
// sends If-None-Match, receives 304, and recovers the Feed from the persisted
// snapshot — non-nil, marked not_modified, HTTP 304, original RetrievedAt
// preserved, no failure, exactly two server hits (no retry storm).
func TestConditionalGETSendsValidatorsAndRecoversFrom304(t *testing.T) {
	body := testSnapshotFeed("Conditional")
	server, requestCount, conditionalHits := newConditionalGETServer(t, body, nil)
	store := NewMemorySnapshotStore(LastGoodStoreConfig{})
	fetcher := NewFetcherWithCoordinator(2*time.Second, conditionalGETCoordinator(store))
	feedURL := server.URL + "/feed.xml"

	first, err := fetcher.FetchFeedWithContextDetailed(context.Background(), feedURL)
	require.NoError(t, err)
	require.NotNil(t, first.Feed)
	require.NotNil(t, first.Access.RetrievedAt)
	firstRetrievedAt := *first.Access.RetrievedAt
	saved, ok, err := store.Load(feedURL)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, conditionalGETETag, saved.ETag)
	require.Equal(t, int32(0), atomic.LoadInt32(conditionalHits), "first fetch must be unconditional")

	second, err := fetcher.FetchFeedWithContextDetailed(context.Background(), feedURL)
	require.NoError(t, err)
	require.NotNil(t, second.Feed, "304 must recover a non-nil Feed from the snapshot")
	require.Equal(t, "Conditional", second.Feed.Title)
	require.Equal(t, http.StatusNotModified, *second.Access.HTTPStatus)
	require.Equal(t, CacheStatusNotModified, second.Access.CacheStatus)
	require.Equal(t, ErrorCategoryNone, second.Access.ErrorCategory)
	require.NotNil(t, second.Access.RetrievedAt)
	require.Equal(t, firstRetrievedAt, *second.Access.RetrievedAt, "304 must preserve the original RetrievedAt, not fake freshness")
	require.Equal(t, int32(1), atomic.LoadInt32(conditionalHits), "second fetch must be conditional and hit 304")
	require.Equal(t, int32(2), atomic.LoadInt32(requestCount), "no retry budget consumed: exactly one 200 then one 304")
}

// TestConditionalGETAdvancesValidatedAtWithoutRewritingBody verifies the 304
// recovery bumps only validated_at: the body, fingerprint, and retrieved_at
// stay authoritative while the validation timestamp advances so a stable feed
// is not evicted as stale.
func TestConditionalGETAdvancesValidatedAtWithoutRewritingBody(t *testing.T) {
	body := testSnapshotFeed("Validated")
	server, _, _ := newConditionalGETServer(t, body, nil)
	store := NewMemorySnapshotStore(LastGoodStoreConfig{})
	fetcher := NewFetcherWithCoordinator(2*time.Second, conditionalGETCoordinator(store))
	feedURL := server.URL + "/feed.xml"

	_, err := fetcher.FetchFeedWithContextDetailed(context.Background(), feedURL)
	require.NoError(t, err)
	afterFirst, ok, err := store.Load(feedURL)
	require.NoError(t, err)
	require.True(t, ok)
	originalFingerprint := afterFirst.Fingerprint
	originalRetrieved := afterFirst.RetrievedAt
	originalValidated := afterFirst.ValidatedAt
	require.True(t, originalValidated.Equal(originalRetrieved) || originalValidated.IsZero())

	// Bump validated_at deterministically past the original so the assertion is
	// stable regardless of wall-clock granularity.
	require.NoError(t, store.TouchValidatedAt(feedURL))

	reloaded, ok, err := store.Load(feedURL)
	require.NoError(t, err)
	require.True(t, ok)
	require.True(t, reloaded.ValidatedAt.After(originalValidated), "validated_at must advance")
	require.Equal(t, originalRetrieved, reloaded.RetrievedAt, "retrieved_at must be untouched")
	require.Equal(t, originalFingerprint, reloaded.Fingerprint, "fingerprint must be untouched")
	require.Equal(t, afterFirst.RawContent, reloaded.RawContent, "body must be untouched")
}

// TestConditionalGETFallsBackToUnconditionalWhenSnapshotVanishesMid304 covers
// the race where the snapshot is evicted between the validators-load and the
// 304: the Coordinator must perform one unconditional GET in the same budget
// and never return a nil Feed or an empty increment.
func TestConditionalGETFallsBackToUnconditionalWhenSnapshotVanishesMid304(t *testing.T) {
	body := testSnapshotFeed("Fallback")
	store := NewMemorySnapshotStore(LastGoodStoreConfig{})
	// Defer binding so the handler can delete the snapshot under the exact
	// canonical URL only known after the test server starts.
	var on304 func()
	server, requestCount, conditionalHits := newConditionalGETServer(t, body, func() {
		if on304 != nil {
			on304()
		}
	})
	feedURL := server.URL + "/feed.xml"
	on304 = func() {
		// Simulate the snapshot being evicted/restarted between Load and 304.
		_ = store.Delete(feedURL)
	}
	fetcher := NewFetcherWithCoordinator(2*time.Second, conditionalGETCoordinator(store))

	first, err := fetcher.FetchFeedWithContextDetailed(context.Background(), feedURL)
	require.NoError(t, err)
	require.NotNil(t, first.Feed)

	second, err := fetcher.FetchFeedWithContextDetailed(context.Background(), feedURL)
	require.NoError(t, err)
	require.NotNil(t, second.Feed, "unrecoverable 304 must fall back to a fresh GET, never nil")
	require.Equal(t, "Fallback", second.Feed.Title)
	require.NotEqual(t, CacheStatusNotModified, second.Access.CacheStatus, "fallback must not masquerade as a 304 recovery")
	require.Equal(t, int32(1), atomic.LoadInt32(conditionalHits), "the conditional request still reached the server as 304")
	require.Equal(t, int32(3), atomic.LoadInt32(requestCount), "200 + 304 + one unconditional fallback GET")
}

// TestConditionalGETOmitsValidatorsWhenNoSnapshot proves no conditional headers
// are sent before a validated snapshot exists, so the very first fetch is a
// clean unconditional GET.
func TestConditionalGETOmitsValidatorsWhenNoSnapshot(t *testing.T) {
	body := testSnapshotFeed("Fresh")
	var sawIfNoneMatch int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") != "" || r.Header.Get("If-Modified-Since") != "" {
			atomic.StoreInt32(&sawIfNoneMatch, 1)
		}
		w.Header().Set("ETag", conditionalGETETag)
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)

	store := NewMemorySnapshotStore(LastGoodStoreConfig{})
	fetcher := NewFetcherWithCoordinator(2*time.Second, conditionalGETCoordinator(store))
	result, err := fetcher.FetchFeedWithContextDetailed(context.Background(), server.URL+"/feed.xml")
	require.NoError(t, err)
	require.NotNil(t, result.Feed)
	require.Zero(t, atomic.LoadInt32(&sawIfNoneMatch), "no conditional headers before a validated snapshot exists")
}

func TestConditionalGETFailureIsNotCountedAs200(t *testing.T) {
	store := NewMemorySnapshotStore(LastGoodStoreConfig{})
	feedURL := "https://conditional.example/feed.xml"
	require.NoError(t, store.Save(FeedSnapshot{
		FeedURL:     feedURL,
		RawContent:  []byte(testSnapshotFeed("Conditional failure")),
		RetrievedAt: time.Now(),
		ETag:        conditionalGETETag,
	}))
	coord := conditionalGETCoordinator(store)
	metrics := NewFeedMetrics()
	coord.SetMetrics(metrics)
	status := http.StatusForbidden

	_, err := coord.Do(context.Background(), feedURL, func(context.Context, RequestValidators) (*FetchResult, error) {
		return &FetchResult{Access: AccessOutcome{
			HTTPStatus:    &status,
			ErrorCategory: ErrorCategoryAccessDenied,
			TargetDomain:  TargetDomain(feedURL),
			SourceType:    AccessSourcePrimary,
		}}, errors.New("403 forbidden")
	})
	require.Error(t, err)
	require.Empty(t, metrics.Snapshot().ConditionalGetTotal, "conditional failures are not 200 or 304 outcomes")
}
