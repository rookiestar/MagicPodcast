package feed

import (
	"bytes"
	"compress/gzip"
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestCoordinatorHalfOpenClosesAfterSuccessThreshold exercises the half-open
// recovery introduced in #25: a recovering domain must prove
// SuccessesToClose consecutive probe successes before the circuit fully
// closes, and a single probe success while below the threshold keeps the
// circuit in probe state.
func TestCoordinatorHalfOpenClosesAfterSuccessThreshold(t *testing.T) {
	var requests int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveRobotsNotFound(w, r) {
			return
		}
		if atomic.AddInt32(&requests, 1) == 1 {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(testSnapshotFeed("Probe")))
	}))
	t.Cleanup(server.Close)

	coordinator := NewCoordinator(CoordinatorConfig{DomainPolicies: map[string]DomainPolicy{
		TargetDomain(server.URL): {
			MaxConcurrency:                 1,
			CircuitCooldown:                20 * time.Millisecond,
			SuccessesToClose:               2,
			HalfOpenMaxRequests:            1,
			ImmediateCircuitOnAccessDenied: true,
		},
	}})
	coordinator.jitter = func() float64 { return 1.0 } // pin backoff to its upper bound for determinism
	fetcher := NewFetcherWithCoordinator(2*time.Second, coordinator)
	feedURL := server.URL + "/feed.xml"

	first, err := fetcher.FetchFeedWithContextDetailed(context.Background(), feedURL)
	require.Error(t, err)
	require.Equal(t, CircuitStateOpen, first.Access.CircuitState)

	time.Sleep(30 * time.Millisecond)

	// First probe succeeds but stays half-open (1 of 2).
	probe1, err := fetcher.FetchFeedWithContextDetailed(context.Background(), feedURL)
	require.NoError(t, err)
	require.Equal(t, CircuitStateProbe, probe1.Access.CircuitState)

	// Second probe succeeds and crosses SuccessesToClose, closing the circuit.
	probe2, err := fetcher.FetchFeedWithContextDetailed(context.Background(), feedURL)
	require.NoError(t, err)
	require.Equal(t, CircuitStateProbe, probe2.Access.CircuitState)

	// A third request is admitted as a normal (non-probe) request on a closed
	// circuit, so its observed circuit state is closed.
	closed, err := fetcher.FetchFeedWithContextDetailed(context.Background(), feedURL)
	require.NoError(t, err)
	require.Equal(t, CircuitStateClosed, closed.Access.CircuitState)
}

// TestCoordinatorHalfOpenReopensOnProbeFailure verifies that a failing probe
// while half-open re-opens the circuit instead of letting queued work through.
func TestCoordinatorHalfOpenReopensOnProbeFailure(t *testing.T) {
	var requests int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveRobotsNotFound(w, r) {
			return
		}
		switch atomic.AddInt32(&requests, 1) {
		case 1:
			w.WriteHeader(http.StatusForbidden)
		case 2:
			w.WriteHeader(http.StatusServiceUnavailable)
		default:
			w.Header().Set("Content-Type", "application/rss+xml")
			_, _ = w.Write([]byte(testSnapshotFeed("Recovered")))
		}
	}))
	t.Cleanup(server.Close)

	coordinator := NewCoordinator(CoordinatorConfig{DomainPolicies: map[string]DomainPolicy{
		TargetDomain(server.URL): {
			MaxConcurrency:                 1,
			CircuitCooldown:                20 * time.Millisecond,
			RetryBackoffInitial:            40 * time.Millisecond,
			RetryBackoffMax:                40 * time.Millisecond,
			SuccessesToClose:               2,
			ImmediateCircuitOnAccessDenied: true,
		},
	}})
	coordinator.jitter = func() float64 { return 1.0 }
	fetcher := NewFetcherWithCoordinator(2*time.Second, coordinator)
	feedURL := server.URL + "/feed.xml"

	first, err := fetcher.FetchFeedWithContextDetailed(context.Background(), feedURL)
	require.Error(t, err)
	require.Equal(t, CircuitStateOpen, first.Access.CircuitState)

	time.Sleep(30 * time.Millisecond)

	// The recovery probe itself fails (503) and must re-open the circuit.
	probe, err := fetcher.FetchFeedWithContextDetailed(context.Background(), feedURL)
	require.Error(t, err)
	require.Equal(t, CircuitStateOpen, probe.Access.CircuitState)

	// Immediately afterwards the domain must still be blocked.
	blocked, err := fetcher.FetchFeedWithContextDetailed(context.Background(), feedURL)
	require.ErrorIs(t, err, ErrFeedCircuitOpen)
	require.Equal(t, ErrorCategoryCircuitOpen, blocked.Access.ErrorCategory)
}

// TestCoordinatorEvidenceThresholdGuardsDomainCircuit verifies the evidence
// gate from #25: with DomainEvidenceMinDistinctFeeds=2 a single feed's 5xx
// does NOT open the domain circuit, but failures from two distinct feeds do.
func TestCoordinatorEvidenceThresholdGuardsDomainCircuit(t *testing.T) {
	var aHits, bHits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveRobotsNotFound(w, r) {
			return
		}
		switch r.URL.Path {
		case "/a.xml":
			atomic.AddInt32(&aHits, 1)
			w.WriteHeader(http.StatusServiceUnavailable)
		case "/b.xml":
			if atomic.AddInt32(&bHits, 1) == 1 {
				w.Header().Set("Content-Type", "application/rss+xml")
				_, _ = w.Write([]byte(testSnapshotFeed("B ok")))
				return
			}
			w.WriteHeader(http.StatusServiceUnavailable)
		default:
			w.Header().Set("Content-Type", "application/rss+xml")
			_, _ = w.Write([]byte(testSnapshotFeed("Other")))
		}
	}))
	t.Cleanup(server.Close)

	coordinator := NewCoordinator(CoordinatorConfig{DomainPolicies: map[string]DomainPolicy{
		TargetDomain(server.URL): {
			DomainEvidenceMinDistinctFeeds: 2,
			EvidenceWindow:                 time.Minute,
		},
	}})
	fetcher := NewFetcherWithCoordinator(2*time.Second, coordinator)

	// Single feed A fails 503: evidence is below threshold, circuit stays usable.
	aFirst, err := fetcher.FetchFeedWithContextDetailed(context.Background(), server.URL+"/a.xml")
	require.Error(t, err)
	require.Equal(t, ErrorCategoryServiceUnavailable, aFirst.Access.ErrorCategory)
	require.Equal(t, CircuitStateNotUsed, aFirst.Access.CircuitState)

	// A different feed on the same domain still succeeds: not blocked.
	bOk, err := fetcher.FetchFeedWithContextDetailed(context.Background(), server.URL+"/b.xml")
	require.NoError(t, err)
	require.Equal(t, "B ok", bOk.Feed.Title)

	// Feed A fails again (still only one distinct feed in evidence).
	_, err = fetcher.FetchFeedWithContextDetailed(context.Background(), server.URL+"/a.xml")
	require.Error(t, err)

	// Feed B now also fails: two distinct feeds → evidence threshold reached, OPEN.
	bFail, err := fetcher.FetchFeedWithContextDetailed(context.Background(), server.URL+"/b.xml")
	require.Error(t, err)
	require.Equal(t, CircuitStateOpen, bFail.Access.CircuitState)

	// Any further work on the domain is blocked.
	blocked, err := fetcher.FetchFeedWithContextDetailed(context.Background(), server.URL+"/c.xml")
	require.ErrorIs(t, err, ErrFeedCircuitOpen)
	require.Equal(t, ErrorCategoryCircuitOpen, blocked.Access.ErrorCategory)
}

// TestCoordinatorCategoryThresholdOverridesEvidenceDefault verifies that a
// startup-configured category threshold is applied to distinct Feed evidence,
// rather than merely being accepted by config validation.
func TestCoordinatorCategoryThresholdOverridesEvidenceDefault(t *testing.T) {
	var aHits, bHits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveRobotsNotFound(w, r) {
			return
		}
		switch r.URL.Path {
		case "/a.xml":
			atomic.AddInt32(&aHits, 1)
			w.WriteHeader(http.StatusServiceUnavailable)
		case "/b.xml":
			atomic.AddInt32(&bHits, 1)
			w.WriteHeader(http.StatusServiceUnavailable)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	coordinator := NewCoordinator(CoordinatorConfig{DomainPolicies: map[string]DomainPolicy{
		TargetDomain(server.URL): {EvidenceWindow: time.Minute},
	}})
	coordinator.SetCircuitDefaults(CircuitDefaults{
		DomainEvidenceMinDistinctFeeds: 1,
		EvidenceWindow:                 time.Minute,
		ThresholdsPerCategory:          map[ErrorCategory]int{ErrorCategoryServiceUnavailable: 2},
	})
	fetcher := NewFetcherWithCoordinator(2*time.Second, coordinator)

	aResult, err := fetcher.FetchFeedWithContextDetailed(context.Background(), server.URL+"/a.xml")
	require.Error(t, err)
	require.Equal(t, CircuitStateNotUsed, aResult.Access.CircuitState)

	bResult, err := fetcher.FetchFeedWithContextDetailed(context.Background(), server.URL+"/b.xml")
	require.Error(t, err)
	require.Equal(t, CircuitStateOpen, bResult.Access.CircuitState)
	require.Equal(t, int32(1), atomic.LoadInt32(&aHits))
	require.Equal(t, int32(1), atomic.LoadInt32(&bHits))
}

// TestFetcherSendsHonestHeadersAndDecompressesGzip verifies the honest UA /
// Accept headers and transparent gzip handling introduced in #25.
func TestFetcherSendsHonestHeadersAndDecompressesGzip(t *testing.T) {
	var seenUserAgent, seenAccept string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveRobotsNotFound(w, r) {
			return
		}
		seenUserAgent = r.Header.Get("User-Agent")
		seenAccept = r.Header.Get("Accept")
		body := &bytes.Buffer{}
		gz := gzip.NewWriter(body)
		_, _ = gz.Write([]byte(testSnapshotFeed("Gzipped")))
		_ = gz.Close()
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write(body.Bytes())
	}))
	t.Cleanup(server.Close)

	result, err := NewFetcher(2*time.Second).FetchFeedWithContextDetailed(context.Background(), server.URL+"/feed.xml")
	require.NoError(t, err)
	require.NotNil(t, result.Feed)
	require.Equal(t, "Gzipped", result.Feed.Title)
	require.Equal(t, defaultFeedUserAgent, seenUserAgent)
	require.Contains(t, seenAccept, "application/rss+xml")
}
