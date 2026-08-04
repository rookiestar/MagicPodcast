package feed

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func openUserAgentGateRecoveryStore(t *testing.T) *SQLiteUserAgentGateStore {
	t.Helper()
	store, db := openUserAgentGateTestStore(t, "")
	require.NoError(t, db.Exec(FeedUserAgentGateAuditsCreateTableSQL).Error)
	require.NoError(t, db.Exec(FeedUserAgentGateRecoveryFeedsCreateTableSQL).Error)
	return store
}

func TestUserAgentGateApprovalDryRunAndApplyAreAuditedWithoutNetwork(t *testing.T) {
	store := openUserAgentGateRecoveryStore(t)
	now := time.Date(2026, 7, 26, 3, 0, 0, 0, time.UTC)
	fingerprint := UserAgentFingerprint("MagicPodcast/1.0 (+https://github.com/rookiestar/MagicPodcast)")
	require.NoError(t, store.Block(context.Background(), "feeds.example", fingerprint, now))

	dryRun, err := store.ApproveProbe(context.Background(), "feeds.example", fingerprint, "owner", now.Add(time.Hour), false)
	require.NoError(t, err)
	require.False(t, dryRun.Applied)
	require.False(t, dryRun.Eligible, "the configured initial cooldown must still apply")
	require.Equal(t, UserAgentGateStateBlocked, dryRun.Record.State)

	applied, err := store.ApproveProbe(context.Background(), "feeds.example", fingerprint, "owner", now.Add(25*time.Hour), true)
	require.NoError(t, err)
	require.True(t, applied.Applied)
	require.True(t, applied.Eligible)
	require.Equal(t, UserAgentGateStateProbePending, applied.Record.State)
	require.Equal(t, "owner", applied.Record.ApprovedBy)
	require.NotNil(t, applied.Record.ApprovedAt)

	audits, err := store.ListAudits(context.Background(), "feeds.example", fingerprint)
	require.NoError(t, err)
	require.Len(t, audits, 2)
	require.Equal(t, UserAgentGateAuditModeDryRun, audits[0].Mode)
	require.Equal(t, UserAgentGateAuditModeApply, audits[1].Mode)
	require.Equal(t, "owner", audits[1].Actor)
	require.Equal(t, UserAgentGateStateProbePending, audits[1].Result)
}

func TestUserAgentGateRecoveryRequiresThreeDistinctFeeds(t *testing.T) {
	store := openUserAgentGateRecoveryStore(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 26, 3, 0, 0, 0, time.UTC)
	fingerprint := UserAgentFingerprint("MagicPodcast/1.0 (+https://github.com/rookiestar/MagicPodcast)")
	require.NoError(t, store.Block(ctx, "feeds.example", fingerprint, now))

	approved, err := store.ApproveProbe(ctx, "feeds.example", fingerprint, "owner", now.Add(25*time.Hour), true)
	require.NoError(t, err)
	require.True(t, approved.Applied)

	firstFeed := UserAgentGateFeedFingerprint("https://feeds.example/one.xml")
	decision, err := store.PreparePrimaryFetchForFeed(ctx, "feeds.example", fingerprint, firstFeed, now.Add(25*time.Hour))
	require.NoError(t, err)
	require.Equal(t, UserAgentGateFetchModeProbe, decision.Mode)

	concurrent, err := store.PreparePrimaryFetchForFeed(ctx, "feeds.example", fingerprint, UserAgentGateFeedFingerprint("https://feeds.example/two.xml"), now.Add(25*time.Hour))
	require.NoError(t, err)
	require.Equal(t, UserAgentGateFetchModeBlocked, concurrent.Mode, "probe_in_flight admits exactly one request")

	completed, err := store.RecordPrimaryFetchResult(ctx, "feeds.example", fingerprint, firstFeed, AccessOutcome{
		HTTPStatus:    intPointer(http.StatusOK),
		ErrorCategory: ErrorCategoryNone,
	}, now.Add(25*time.Hour))
	require.NoError(t, err)
	require.Equal(t, UserAgentGateStateRecovering, completed.State)
	require.Equal(t, 1, completed.RecoverySuccessCount)

	sameFeed, err := store.PreparePrimaryFetchForFeed(ctx, "feeds.example", fingerprint, firstFeed, now.Add(25*time.Hour))
	require.NoError(t, err)
	require.Equal(t, UserAgentGateFetchModeBlocked, sameFeed.Mode, "one Feed must not be probed repeatedly")

	secondFeed := UserAgentGateFeedFingerprint("https://feeds.example/two.xml")
	decision, err = store.PreparePrimaryFetchForFeed(ctx, "feeds.example", fingerprint, secondFeed, now.Add(25*time.Hour))
	require.NoError(t, err)
	require.Equal(t, UserAgentGateFetchModeRecovery, decision.Mode)
	completed, err = store.RecordPrimaryFetchResult(ctx, "feeds.example", fingerprint, secondFeed, AccessOutcome{
		HTTPStatus:    intPointer(http.StatusNotModified),
		ErrorCategory: ErrorCategoryNone,
	}, now.Add(25*time.Hour+time.Minute))
	require.NoError(t, err)
	require.Equal(t, UserAgentGateStateRecovering, completed.State)
	require.Equal(t, 2, completed.RecoverySuccessCount)

	thirdFeed := UserAgentGateFeedFingerprint("https://feeds.example/three.xml")
	decision, err = store.PreparePrimaryFetchForFeed(ctx, "feeds.example", fingerprint, thirdFeed, now.Add(25*time.Hour+2*time.Minute))
	require.NoError(t, err)
	require.Equal(t, UserAgentGateFetchModeRecovery, decision.Mode)
	completed, err = store.RecordPrimaryFetchResult(ctx, "feeds.example", fingerprint, thirdFeed, AccessOutcome{
		HTTPStatus:    intPointer(http.StatusOK),
		ErrorCategory: ErrorCategoryNone,
	}, now.Add(25*time.Hour+3*time.Minute))
	require.NoError(t, err)
	require.Equal(t, UserAgentGateStateActive, completed.State)
	require.Equal(t, 3, completed.RecoverySuccessCount)

	active, err := store.PreparePrimaryFetchForFeed(ctx, "feeds.example", fingerprint, UserAgentGateFeedFingerprint("https://feeds.example/four.xml"), now.Add(25*time.Hour+4*time.Minute))
	require.NoError(t, err)
	require.Equal(t, UserAgentGateFetchModeActive, active.Mode)
}

func TestUserAgentGateRecoveryUsesConfiguredDistinctFeedSuccessThreshold(t *testing.T) {
	config := UserAgentGateRecoveryConfig{
		InitialCooldown:      6 * time.Hour,
		ProbeFailureCooldown: 24 * time.Hour,
		RequiredSuccesses:    4,
	}
	store, db := openUserAgentGateTestStore(t, "", config)
	require.NoError(t, db.Exec(FeedUserAgentGateAuditsCreateTableSQL).Error)

	ctx := context.Background()
	now := time.Date(2026, 7, 26, 3, 0, 0, 0, time.UTC)
	fingerprint := UserAgentFingerprint(defaultFeedUserAgent)
	require.NoError(t, store.Block(ctx, "feeds.example", fingerprint, now))
	approvedAt := now.Add(config.InitialCooldown)
	approval, err := store.ApproveProbe(ctx, "feeds.example", fingerprint, "owner", approvedAt, true)
	require.NoError(t, err)
	require.True(t, approval.Applied)

	for i, path := range []string{"/one.xml", "/two.xml", "/three.xml", "/four.xml"} {
		feedFingerprint := UserAgentGateFeedFingerprint("https://feeds.example" + path)
		decision, prepareErr := store.PreparePrimaryFetchForFeed(ctx, "feeds.example", fingerprint, feedFingerprint, approvedAt.Add(time.Duration(i)*time.Minute))
		require.NoError(t, prepareErr)
		if i == 0 {
			require.Equal(t, UserAgentGateFetchModeProbe, decision.Mode)
		} else {
			require.Equal(t, UserAgentGateFetchModeRecovery, decision.Mode)
		}
		completed, recordErr := store.RecordPrimaryFetchResult(ctx, "feeds.example", fingerprint, feedFingerprint, AccessOutcome{
			HTTPStatus:    intPointer(http.StatusOK),
			ErrorCategory: ErrorCategoryNone,
		}, approvedAt.Add(time.Duration(i+1)*time.Minute))
		require.NoError(t, recordErr)
		if i < config.RequiredSuccesses-1 {
			require.Equal(t, UserAgentGateStateRecovering, completed.State)
		} else {
			require.Equal(t, UserAgentGateStateActive, completed.State)
		}
	}
}

func TestUserAgentGateConcurrentApprovalAdmitsExactlyOneProbe(t *testing.T) {
	store := openUserAgentGateRecoveryStore(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 26, 3, 0, 0, 0, time.UTC)
	fingerprint := UserAgentFingerprint(defaultFeedUserAgent)
	require.NoError(t, store.Block(ctx, "feeds.example", fingerprint, now))
	_, err := store.ApproveProbe(ctx, "feeds.example", fingerprint, "owner", now.Add(25*time.Hour), true)
	require.NoError(t, err)

	start := make(chan struct{})
	results := make(chan UserAgentGateFetchMode, 8)
	var waitGroup sync.WaitGroup
	for i := 0; i < 8; i++ {
		i := i
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			<-start
			decision, prepareErr := store.PreparePrimaryFetchForFeed(
				ctx,
				"feeds.example",
				fingerprint,
				UserAgentGateFeedFingerprint(fmt.Sprintf("https://feeds.example/concurrent-%d.xml", i)),
				now.Add(25*time.Hour),
			)
			require.NoError(t, prepareErr)
			results <- decision.Mode
		}()
	}
	close(start)
	waitGroup.Wait()
	close(results)

	probes := 0
	for mode := range results {
		if mode == UserAgentGateFetchModeProbe {
			probes++
		}
	}
	require.Equal(t, 1, probes, "concurrent approval must claim exactly one probe")
}

func TestUserAgentGateProbeFailureReblocksAndUARefusalResetsProgress(t *testing.T) {
	store := openUserAgentGateRecoveryStore(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 26, 3, 0, 0, 0, time.UTC)
	fingerprint := UserAgentFingerprint("MagicPodcast/1.0 (+https://github.com/rookiestar/MagicPodcast)")
	require.NoError(t, store.Block(ctx, "feeds.example", fingerprint, now))
	_, err := store.ApproveProbe(ctx, "feeds.example", fingerprint, "owner", now.Add(25*time.Hour), true)
	require.NoError(t, err)

	feedFingerprint := UserAgentGateFeedFingerprint("https://feeds.example/one.xml")
	decision, err := store.PreparePrimaryFetchForFeed(ctx, "feeds.example", fingerprint, feedFingerprint, now.Add(25*time.Hour))
	require.NoError(t, err)
	require.Equal(t, UserAgentGateFetchModeProbe, decision.Mode)
	failed, err := store.RecordPrimaryFetchResult(ctx, "feeds.example", fingerprint, feedFingerprint, AccessOutcome{
		HTTPStatus:    intPointer(http.StatusServiceUnavailable),
		ErrorCategory: ErrorCategoryServiceUnavailable,
	}, now.Add(25*time.Hour+time.Minute))
	require.NoError(t, err)
	require.Equal(t, UserAgentGateStateBlocked, failed.State)
	require.Equal(t, string(ErrorCategoryServiceUnavailable), failed.LastProbeResult)
	require.Equal(t, now.Add(25*time.Hour+time.Minute+DefaultUserAgentProbeFailureCooldown), failed.ProbeEligibleAt)

	// A renewed explicit ACL refusal is terminal and clears any partial recovery.
	retryAt := failed.ProbeEligibleAt
	_, err = store.ApproveProbe(ctx, "feeds.example", fingerprint, "owner", retryAt, true)
	require.NoError(t, err)
	decision, err = store.PreparePrimaryFetchForFeed(ctx, "feeds.example", fingerprint, feedFingerprint, retryAt)
	require.NoError(t, err)
	require.Equal(t, UserAgentGateFetchModeProbe, decision.Mode)
	reset, err := store.RecordPrimaryFetchResult(ctx, "feeds.example", fingerprint, feedFingerprint, AccessOutcome{
		HTTPStatus:    intPointer(http.StatusForbidden),
		ErrorCategory: ErrorCategoryUserAgentDenied,
	}, retryAt.Add(time.Minute))
	require.NoError(t, err)
	require.Equal(t, UserAgentGateStateBlocked, reset.State)
	require.Equal(t, string(ErrorCategoryUserAgentDenied), reset.LastProbeResult)
	require.Equal(t, 0, reset.RecoverySuccessCount)
	require.Equal(t, retryAt.Add(time.Minute+DefaultUserAgentProbeFailureCooldown), reset.ProbeEligibleAt)
}

func TestFetcherUsesApprovedProbeAndGradualRecoveryWithoutLocalCache(t *testing.T) {
	var feedHits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveRobotsNotFound(w, r) {
			return
		}
		atomic.AddInt32(&feedHits, 1)
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = fmt.Fprint(w, testSnapshotFeed(strings.TrimPrefix(r.URL.Path, "/")))
	}))
	t.Cleanup(server.Close)

	store := openUserAgentGateRecoveryStore(t)
	now := time.Date(2026, 7, 26, 3, 0, 0, 0, time.UTC)
	fingerprint := UserAgentFingerprint(defaultFeedUserAgent)
	require.NoError(t, store.Block(context.Background(), TargetDomain(server.URL), fingerprint, now))
	approvedAt := now.Add(25 * time.Hour)
	approval, err := store.ApproveProbe(context.Background(), TargetDomain(server.URL), fingerprint, "owner", approvedAt, true)
	require.NoError(t, err)
	require.True(t, approval.Applied)

	fetcher := NewFetcherWithCoordinator(2*time.Second, NewCoordinator(CoordinatorConfig{}))
	fetcher.SetUserAgentGateStore(store)
	fetcher.SetClock(func() time.Time { return approvedAt })

	for _, path := range []string{"/one.xml", "/two.xml", "/three.xml"} {
		result, fetchErr := fetcher.FetchFeedWithContextDetailed(context.Background(), server.URL+path)
		require.NoError(t, fetchErr)
		require.Equal(t, ErrorCategoryNone, result.Access.ErrorCategory)
	}
	require.Equal(t, int32(3), atomic.LoadInt32(&feedHits), "each recovery Feed must reach the upstream once")

	blocked, record, err := store.IsBlocked(context.Background(), TargetDomain(server.URL), fingerprint, approvedAt)
	require.NoError(t, err)
	require.False(t, blocked)
	require.Equal(t, UserAgentGateStateActive, record.State)
	require.Equal(t, 3, record.RecoverySuccessCount)

	// Once active, ordinary traffic remains active and is not counted as a
	// fourth recovery proof.
	result, err := fetcher.FetchFeedWithContextDetailed(context.Background(), server.URL+"/four.xml")
	require.NoError(t, err)
	require.Equal(t, UserAgentGateStateActive, result.Access.UserAgentGateState)
	require.Equal(t, int32(4), atomic.LoadInt32(&feedHits))
}

func TestFetcherRecoveryFailureDoesNotUseSharedSuccessCache(t *testing.T) {
	var feedHits int32
	var failLive int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveRobotsNotFound(w, r) {
			return
		}
		atomic.AddInt32(&feedHits, 1)
		if atomic.LoadInt32(&failLive) == 0 {
			w.Header().Set("Content-Type", "application/rss+xml")
			_, _ = fmt.Fprint(w, testSnapshotFeed(strings.TrimPrefix(r.URL.Path, "/")))
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(server.Close)

	store := openUserAgentGateRecoveryStore(t)
	now := time.Date(2026, 7, 26, 3, 0, 0, 0, time.UTC)
	fingerprint := UserAgentFingerprint(defaultFeedUserAgent)
	domain := TargetDomain(server.URL)
	coordinator := NewCoordinator(CoordinatorConfig{DomainPolicies: map[string]DomainPolicy{
		domain: {MinRefreshInterval: time.Hour},
	}})
	fetcher := NewFetcherWithCoordinator(2*time.Second, coordinator)
	// Seed the Coordinator's shared success cache. The approved recovery below
	// must still make a live request instead of accepting this cached 200.
	first, err := fetcher.FetchFeedWithContextDetailed(context.Background(), server.URL+"/failure.xml")
	require.NoError(t, err)
	require.Equal(t, ErrorCategoryNone, first.Access.ErrorCategory)
	require.Equal(t, int32(1), atomic.LoadInt32(&feedHits))

	require.NoError(t, store.Block(context.Background(), domain, fingerprint, now))
	approvedAt := now.Add(25 * time.Hour)
	_, err = store.ApproveProbe(context.Background(), domain, fingerprint, "owner", approvedAt, true)
	require.NoError(t, err)

	fetcher.SetUserAgentGateStore(store)
	fetcher.SetClock(func() time.Time { return approvedAt })
	atomic.StoreInt32(&failLive, 1)
	result, fetchErr := fetcher.FetchFeedWithContextDetailed(context.Background(), server.URL+"/failure.xml")
	require.Error(t, fetchErr)
	require.Equal(t, ErrorCategoryServiceUnavailable, result.Access.ErrorCategory)
	require.Equal(t, int32(2), atomic.LoadInt32(&feedHits), "recovery must bypass the pre-existing shared success cache")

	blocked, record, err := store.IsBlocked(context.Background(), domain, fingerprint, approvedAt)
	require.NoError(t, err)
	require.True(t, blocked)
	require.Equal(t, UserAgentGateStateBlocked, record.State)
}

func intPointer(value int) *int { return &value }
