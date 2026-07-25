package feed

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestSoftRateDomainMulti403DoesNotHardOpenSiblings is the #36 core seam:
// multiple 403s on a SoftRateEnabled domain must not produce circuit_open for
// sibling feeds on the same domain; each feed still gets a live primary attempt.
func TestSoftRateDomainMulti403DoesNotHardOpenSiblings(t *testing.T) {
	var aHits, bHits, cHits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveRobotsNotFound(w, r) {
			return
		}
		switch r.URL.Path {
		case "/a.xml":
			atomic.AddInt32(&aHits, 1)
			w.WriteHeader(http.StatusForbidden)
		case "/b.xml":
			atomic.AddInt32(&bHits, 1)
			w.WriteHeader(http.StatusForbidden)
		case "/c.xml":
			atomic.AddInt32(&cHits, 1)
			w.Header().Set("Content-Type", "application/rss+xml")
			_, _ = w.Write([]byte(testSnapshotFeed("Sibling OK")))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	domain := TargetDomain(server.URL)
	coordinator := NewCoordinator(CoordinatorConfig{DomainPolicies: map[string]DomainPolicy{
		domain: {
			MaxConcurrency:  1,
			SoftRateEnabled: true,
			// Explicitly disabled hard-open — mirrors xyzfm default.
			ImmediateCircuitOnAccessDenied: false,
		},
	}})
	// Soft-rate spacing would slow the test; pin clock advancement by using
	// normal tier only (no prior 403 state). After first 403 tier becomes
	// cautious (2s); keep waits short by not stacking many waits — three
	// sequential fetches are enough.
	fetcher := NewFetcherWithCoordinator(2*time.Second, coordinator)

	a, err := fetcher.FetchFeedWithContextDetailed(context.Background(), server.URL+"/a.xml")
	require.Error(t, err)
	require.Equal(t, ErrorCategoryAccessDenied, a.Access.ErrorCategory)
	require.NotEqual(t, CircuitStateOpen, a.Access.CircuitState)
	require.NotEqual(t, ErrorCategoryCircuitOpen, a.Access.ErrorCategory)

	b, err := fetcher.FetchFeedWithContextDetailed(context.Background(), server.URL+"/b.xml")
	require.Error(t, err)
	require.Equal(t, ErrorCategoryAccessDenied, b.Access.ErrorCategory)
	require.NotEqual(t, ErrorCategoryCircuitOpen, b.Access.ErrorCategory)

	c, err := fetcher.FetchFeedWithContextDetailed(context.Background(), server.URL+"/c.xml")
	require.NoError(t, err)
	require.Equal(t, "Sibling OK", c.Feed.Title)
	require.NotEqual(t, ErrorCategoryCircuitOpen, c.Access.ErrorCategory)

	require.Equal(t, int32(1), atomic.LoadInt32(&aHits), "feed A must receive a live primary attempt")
	require.Equal(t, int32(1), atomic.LoadInt32(&bHits), "feed B must receive a live primary attempt")
	require.Equal(t, int32(1), atomic.LoadInt32(&cHits), "feed C must receive a live primary attempt")
	require.Equal(t, SoftRateSlow, coordinator.SoftRateTierFor(domain), "two 403s degrade to slow")
}

// TestDefaultXiaoyuzhouPolicyUsesSoftRateNotHardOpen locks the production
// default for feed.xyzfm.space.
func TestDefaultXiaoyuzhouPolicyUsesSoftRateNotHardOpen(t *testing.T) {
	policy := DefaultCoordinatorConfig().DomainPolicies[XiaoyuzhouFeedDomain]
	require.True(t, policy.SoftRateEnabled)
	require.False(t, policy.ImmediateCircuitOnAccessDenied)
	require.Equal(t, 1, policy.MaxConcurrency)
}
