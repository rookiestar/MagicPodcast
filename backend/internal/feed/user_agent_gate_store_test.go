package feed

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func openUserAgentGateTestStore(t *testing.T, path string, configs ...UserAgentGateRecoveryConfig) (*SQLiteUserAgentGateStore, *gorm.DB) {
	t.Helper()
	if path == "" {
		path = filepath.Join(t.TempDir(), "user-agent-gate.db")
	}
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec(FeedUserAgentGatesCreateTableSQL).Error)
	require.NoError(t, db.Exec(FeedUserAgentGatesCreateIndexSQL).Error)
	require.NoError(t, db.Exec(FeedUserAgentGateRecoveryFeedsCreateTableSQL).Error)
	require.NoError(t, db.Exec(FeedUserAgentGateRecoveryFeedsCreateIndexSQL).Error)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	store, err := NewSQLiteUserAgentGateStore(sqlDB, configs...)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return store, db
}

func TestSQLiteUserAgentGatePersistsBlockAcrossRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "user-agent-gate.db")
	now := time.Date(2026, 7, 26, 1, 2, 3, 0, time.UTC)
	userAgent := "MagicPodcast/1.0 (+https://github.com/rookiestar/MagicPodcast)"
	fingerprint := UserAgentFingerprint(userAgent)

	store, db := openUserAgentGateTestStore(t, path)
	require.NoError(t, store.Block(context.Background(), "FEED.EXAMPLE", fingerprint, now))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	store, db = openUserAgentGateTestStore(t, path)
	blocked, record, err := store.IsBlocked(context.Background(), "feed.example", fingerprint, now.Add(time.Minute))
	require.NoError(t, err)
	require.True(t, blocked)
	require.Equal(t, UserAgentGateStateBlocked, record.State)
	require.Equal(t, "feed.example", record.Domain)
	require.Equal(t, fingerprint, record.UserAgentFingerprint)
	require.Equal(t, now, record.DetectedAt)
	require.Equal(t, now.Add(DefaultUserAgentInitialCooldown), record.ProbeEligibleAt)

	rows, err := store.List(context.Background())
	require.NoError(t, err)
	encoded, err := json.Marshal(rows)
	require.NoError(t, err)
	require.Contains(t, string(encoded), fingerprint[:12])
	require.NotContains(t, string(encoded), userAgent)
}

func TestSQLiteUserAgentGateUsesConfiguredInitialAndProbeFailureCooldowns(t *testing.T) {
	config := UserAgentGateRecoveryConfig{
		InitialCooldown:      7 * time.Hour,
		ProbeFailureCooldown: 30 * time.Hour,
		RequiredSuccesses:    3,
	}
	store, db := openUserAgentGateTestStore(t, "", config)
	require.NoError(t, db.Exec(FeedUserAgentGateAuditsCreateTableSQL).Error)

	ctx := context.Background()
	now := time.Date(2026, 7, 26, 1, 2, 3, 0, time.UTC)
	fingerprint := UserAgentFingerprint(defaultFeedUserAgent)
	require.NoError(t, store.Block(ctx, "feeds.example", fingerprint, now))

	blocked, record, err := store.IsBlocked(ctx, "feeds.example", fingerprint, now)
	require.NoError(t, err)
	require.True(t, blocked)
	require.Equal(t, now.Add(config.InitialCooldown), record.ProbeEligibleAt)

	_, err = store.ApproveProbe(ctx, "feeds.example", fingerprint, "owner", record.ProbeEligibleAt, true)
	require.NoError(t, err)
	feedFingerprint := UserAgentGateFeedFingerprint("https://feeds.example/one.xml")
	decision, err := store.PreparePrimaryFetchForFeed(ctx, "feeds.example", fingerprint, feedFingerprint, record.ProbeEligibleAt)
	require.NoError(t, err)
	require.Equal(t, UserAgentGateFetchModeProbe, decision.Mode)

	failureAt := record.ProbeEligibleAt.Add(time.Minute)
	failed, err := store.RecordPrimaryFetchResult(ctx, "feeds.example", fingerprint, feedFingerprint, AccessOutcome{
		HTTPStatus:    intPointer(http.StatusServiceUnavailable),
		ErrorCategory: ErrorCategoryServiceUnavailable,
	}, failureAt)
	require.NoError(t, err)
	require.Equal(t, UserAgentGateStateBlocked, failed.State)
	require.Equal(t, failureAt.Add(config.ProbeFailureCooldown), failed.ProbeEligibleAt)
}

func TestFetcherUsesPersistentUserAgentGateAcrossInstancesAndKeepsDomainsSeparate(t *testing.T) {
	var firstDomainHits, secondDomainHits int32
	firstDomain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveRobotsNotFound(w, r) {
			return
		}
		atomic.AddInt32(&firstDomainHits, 1)
		w.Header().Set("X-Tengine-Error", "denied by UA ACL = blacklist")
		w.WriteHeader(http.StatusForbidden)
		_, _ = fmt.Fprint(w, "do-not-persist-this-body")
	}))
	t.Cleanup(firstDomain.Close)
	secondDomain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveRobotsNotFound(w, r) {
			return
		}
		atomic.AddInt32(&secondDomainHits, 1)
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = fmt.Fprint(w, testSnapshotFeed("different domain"))
	}))
	t.Cleanup(secondDomain.Close)

	store, _ := openUserAgentGateTestStore(t, "")
	firstFetcher := NewFetcherWithCoordinator(2*time.Second, NewCoordinator(CoordinatorConfig{}))
	firstFetcher.SetUserAgentGateStore(store)
	first, err := firstFetcher.FetchFeedWithContextDetailed(context.Background(), firstDomain.URL+"/one.xml")
	require.Error(t, err)
	require.Equal(t, ErrorCategoryUserAgentDenied, first.Access.ErrorCategory)

	// A fresh Fetcher and context model a new Job after a service restart. The
	// persisted block suppresses the sibling primary request before HTTP, while
	// another target domain remains eligible.
	secondFetcher := NewFetcherWithCoordinator(2*time.Second, NewCoordinator(CoordinatorConfig{}))
	secondFetcher.SetUserAgentGateStore(store)
	blocked, err := secondFetcher.FetchFeedWithContextDetailed(context.Background(), firstDomain.URL+"/two.xml")
	require.Error(t, err)
	require.Equal(t, ErrorCategoryUserAgentBlocked, blocked.Access.ErrorCategory)
	secondDomainURL := strings.Replace(secondDomain.URL, "127.0.0.1", "localhost", 1)
	allowed, err := secondFetcher.FetchFeedWithContextDetailed(context.Background(), secondDomainURL+"/feed.xml")
	require.NoError(t, err)
	require.Equal(t, ErrorCategoryNone, allowed.Access.ErrorCategory)
	require.Equal(t, int32(1), atomic.LoadInt32(&firstDomainHits))
	require.Equal(t, int32(1), atomic.LoadInt32(&secondDomainHits))
}

func TestPersistentUserAgentGateSerializesConcurrentSameKeyFetches(t *testing.T) {
	var feedHits int32
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveRobotsNotFound(w, r) {
			return
		}
		if atomic.AddInt32(&feedHits, 1) == 1 {
			close(firstStarted)
			<-releaseFirst
		}
		w.Header().Set("X-Tengine-Error", "denied by UA ACL = blacklist")
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(server.Close)

	store, _ := openUserAgentGateTestStore(t, "")
	firstFetcher := NewFetcherWithCoordinator(2*time.Second, NewCoordinator(CoordinatorConfig{}))
	firstFetcher.SetUserAgentGateStore(store)
	secondFetcher := NewFetcherWithCoordinator(2*time.Second, NewCoordinator(CoordinatorConfig{}))
	secondFetcher.SetUserAgentGateStore(store)

	firstResult := make(chan *FetchResult, 1)
	go func() {
		result, _ := firstFetcher.FetchFeedWithContextDetailed(context.Background(), server.URL+"/first.xml")
		firstResult <- result
	}()
	<-firstStarted

	secondResult := make(chan *FetchResult, 1)
	go func() {
		result, _ := secondFetcher.FetchFeedWithContextDetailed(context.Background(), server.URL+"/second.xml")
		secondResult <- result
	}()
	close(releaseFirst)

	firstOutcome := <-firstResult
	secondOutcome := <-secondResult
	require.Equal(t, ErrorCategoryUserAgentDenied, firstOutcome.Access.ErrorCategory)
	require.Equal(t, ErrorCategoryUserAgentBlocked, secondOutcome.Access.ErrorCategory)
	require.Equal(t, int32(1), atomic.LoadInt32(&feedHits), "concurrent Jobs must not create a second same-domain primary request")
}

func TestPersistentUserAgentGateDoesNotLetChangedUserAgentBypassBlockedDomain(t *testing.T) {
	store, _ := openUserAgentGateTestStore(t, "")
	now := time.Date(2026, 7, 26, 2, 0, 0, 0, time.UTC)
	oldFingerprint := UserAgentFingerprint("MagicPodcast/1.0 (+https://github.com/rookiestar/MagicPodcast)")
	newFingerprint := UserAgentFingerprint("MagicPodcast/1.1 (+https://github.com/rookiestar/MagicPodcast)")
	require.NoError(t, store.Block(context.Background(), "feeds.example", oldFingerprint, now))

	decision, err := store.PreparePrimaryFetch(context.Background(), "feeds.example", newFingerprint, now.Add(time.Hour))
	require.NoError(t, err)
	require.Equal(t, UserAgentGateFetchModeBlocked, decision.Mode,
		"a changed UA must not silently bypass a domain block")

	other, err := store.PreparePrimaryFetch(context.Background(), "other.example", newFingerprint, now.Add(time.Hour))
	require.NoError(t, err)
	require.Equal(t, UserAgentGateFetchModeActive, other.Mode,
		"a different domain remains isolated")
}
