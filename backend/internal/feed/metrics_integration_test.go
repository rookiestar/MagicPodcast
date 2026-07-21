package feed

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/mmcdole/gofeed"
	"github.com/stretchr/testify/require"
)

// TestCoordinatorMetricsRecordsFetchConditionalGetAndCircuit verifies the
// recording points wired into the Coordinator: a successful primary fetch
// records feed_fetch_total + conditional_get "miss", a 403 on an
// ImmediateCircuitOnAccessDenied domain records the closed/open transition,
// and recovery after cooldown records open/probe + retry + probe/closed.
func TestCoordinatorMetricsRecordsFetchConditionalGetAndCircuit(t *testing.T) {
	metrics := NewFeedMetrics()
	store := NewMemorySnapshotStore(LastGoodStoreConfig{})
	coord := NewCoordinator(CoordinatorConfig{
		DomainPolicies: map[string]DomainPolicy{
			"blocked.example": {
				ImmediateCircuitOnAccessDenied: true,
				CircuitCooldown:                 60 * time.Millisecond,
				HalfOpenMaxRequests:             1,
				SuccessesToClose:                1,
			},
		},
		LastGoodStore: store,
	})
	coord.SetMetrics(metrics)

	statusPtr := func(s int) *int { return &s }

	// 1. Successful primary fetch on a clean domain: no snapshot exists yet, so
	// validators are empty -> conditional_get "miss".
	okFetch := func(ctx context.Context, _ RequestValidators) (*FetchResult, error) {
		return &FetchResult{
			Feed: &gofeed.Feed{Title: "clean"},
			Access: AccessOutcome{
				HTTPStatus:    statusPtr(http.StatusOK),
				ErrorCategory: ErrorCategoryNone,
				TargetDomain:  "clean.example",
				SourceType:    AccessSourcePrimary,
				CacheStatus:   CacheStatusNotUsed,
				Freshness:     FreshnessLive,
			},
		}, nil
	}
	_, err := coord.Do(context.Background(), "https://clean.example/feed.xml", okFetch)
	require.NoError(t, err)

	snap := metrics.Snapshot()
	require.Equal(t, int64(1), findConditionalGetRow(t, snap.ConditionalGetTotal, conditionalGetResultMiss).Count)
	require.Equal(t, int64(1), findFetchRow(t, snap.FeedFetchTotal, "clean.example", "200", string(ErrorCategoryNone), string(AccessSourcePrimary)).Count)

	// 2. 403 on the immediate-circuit domain trips closed/open (the domain's
	// circuit starts NotUsed, so the transition is not_used->open).
	deniedFetch := func(ctx context.Context, _ RequestValidators) (*FetchResult, error) {
		return &FetchResult{
			Access: AccessOutcome{
				HTTPStatus:    statusPtr(http.StatusForbidden),
				ErrorCategory: ErrorCategoryAccessDenied,
				TargetDomain:  "blocked.example",
				SourceType:    AccessSourcePrimary,
			},
		}, errors.New("403 forbidden")
	}
	_, err = coord.Do(context.Background(), "https://blocked.example/feed.xml", deniedFetch)
	require.Error(t, err)

	snap = metrics.Snapshot()
	require.Equal(t, int64(1), findFetchRow(t, snap.FeedFetchTotal, "blocked.example", "403", string(ErrorCategoryAccessDenied), string(AccessSourcePrimary)).Count)
	require.Equal(t, int64(1), findTransitionRow(t, snap.CircuitTransitions, "blocked.example", string(CircuitStateNotUsed), string(CircuitStateOpen)).Count)

	// 3. After the cooldown elapses, a successful probe records open/probe +
	// retry, and probe success closes the circuit (probe/closed).
	time.Sleep(90 * time.Millisecond)
	probeFetch := func(ctx context.Context, _ RequestValidators) (*FetchResult, error) {
		return &FetchResult{
			Feed: &gofeed.Feed{Title: "recovered"},
			Access: AccessOutcome{
				HTTPStatus:    statusPtr(http.StatusOK),
				ErrorCategory: ErrorCategoryNone,
				TargetDomain:  "blocked.example",
				SourceType:    AccessSourcePrimary,
				CacheStatus:   CacheStatusNotUsed,
				Freshness:     FreshnessLive,
			},
		}, nil
	}
	_, err = coord.Do(context.Background(), "https://blocked.example/feed.xml", probeFetch)
	require.NoError(t, err)

	snap = metrics.Snapshot()
	require.Equal(t, int64(1), findTransitionRow(t, snap.CircuitTransitions, "blocked.example", string(CircuitStateOpen), string(CircuitStateProbe)).Count)
	require.Equal(t, int64(1), findTransitionRow(t, snap.CircuitTransitions, "blocked.example", string(CircuitStateProbe), string(CircuitStateClosed)).Count)
	require.Equal(t, int64(1), findRetryRow(t, snap.RetryTotal, "blocked.example").Count)

	// CircuitSnapshot reflects the recovered (closed) state.
	circuitRows := coord.CircuitSnapshot()
	require.NotEmpty(t, circuitRows)
	var blockedRow *CircuitStateRow
	for i := range circuitRows {
		if circuitRows[i].Domain == "blocked.example" {
			blockedRow = &circuitRows[i]
		}
	}
	require.NotNil(t, blockedRow)
	require.Equal(t, string(CircuitStateClosed), blockedRow.State)
	require.Nil(t, blockedRow.OpenUntil)
}

// TestCoordinatorMetricsRecordsLastGoodHit verifies that a successful last-good
// recovery increments last_good_hits_total and that LastGoodStats reports the
// store entry.
func TestCoordinatorMetricsRecordsLastGoodHit(t *testing.T) {
	metrics := NewFeedMetrics()
	store := NewMemorySnapshotStore(LastGoodStoreConfig{})
	coord := NewCoordinator(CoordinatorConfig{LastGoodStore: store})
	coord.SetMetrics(metrics)

	feedURL := "https://lg.example/feed.xml"
	require.NoError(t, store.Save(FeedSnapshot{
		FeedURL:      feedURL,
		RawContent:   []byte(testSnapshotFeed("LastGood")),
		RetrievedAt:  time.Now(),
		ValidatedAt:  time.Now(),
	}))

	recovered, ok := coord.LastGood(context.Background(), feedURL, &FetchResult{
		Access: AccessOutcome{TargetDomain: "lg.example", SourceType: AccessSourcePrimary},
	})
	require.True(t, ok)
	require.NotNil(t, recovered.Feed)

	snap := metrics.Snapshot()
	require.Equal(t, int64(1), snap.LastGoodHitsTotal)

	stats, err := coord.LastGoodStats()
	require.NoError(t, err)
	require.Equal(t, 1, stats.Entries)
}
