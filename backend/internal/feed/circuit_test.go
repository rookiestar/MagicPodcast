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

func TestCoordinatorOpensCircuitAfter403AndAllowsOneRecoveryProbe(t *testing.T) {
	var requestCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&requestCount, 1) == 1 {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(testSnapshotFeed("Recovered")))
	}))
	t.Cleanup(server.Close)

	coordinator := NewCoordinator(CoordinatorConfig{DomainPolicies: map[string]DomainPolicy{
		TargetDomain(server.URL): {MaxConcurrency: 1, CircuitCooldown: 40 * time.Millisecond},
	}})
	fetcher := NewFetcherWithCoordinator(2*time.Second, coordinator)
	feedURL := server.URL + "/feed.xml"

	first, err := fetcher.FetchFeedWithContextDetailed(context.Background(), feedURL)
	require.Error(t, err)
	require.Equal(t, ErrorCategoryAccessDenied, first.Access.ErrorCategory)
	require.Equal(t, CircuitStateOpen, first.Access.CircuitState)

	blocked, err := fetcher.FetchFeedWithContextDetailed(context.Background(), feedURL)
	require.ErrorIs(t, err, ErrFeedCircuitOpen)
	require.Equal(t, ErrorCategoryCircuitOpen, blocked.Access.ErrorCategory)
	require.Equal(t, CircuitStateOpen, blocked.Access.CircuitState)
	require.Equal(t, int32(1), atomic.LoadInt32(&requestCount))

	time.Sleep(60 * time.Millisecond)
	probe, err := fetcher.FetchFeedWithContextDetailed(context.Background(), feedURL)
	require.NoError(t, err)
	require.Equal(t, "Recovered", probe.Feed.Title)
	require.Equal(t, CircuitStateProbe, probe.Access.CircuitState)
	require.Equal(t, int32(2), atomic.LoadInt32(&requestCount))
}

func TestCoordinatorHonorsRetryAfterBeforeRecoveryProbe(t *testing.T) {
	var requestCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&requestCount, 1) == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(testSnapshotFeed("Rate recovered")))
	}))
	t.Cleanup(server.Close)

	coordinator := NewCoordinator(CoordinatorConfig{DomainPolicies: map[string]DomainPolicy{
		TargetDomain(server.URL): {MaxConcurrency: 1, CircuitCooldown: time.Millisecond},
	}})
	fetcher := NewFetcherWithCoordinator(2*time.Second, coordinator)
	feedURL := server.URL + "/feed.xml"

	first, err := fetcher.FetchFeedWithContextDetailed(context.Background(), feedURL)
	require.Error(t, err)
	require.Equal(t, ErrorCategoryRateLimited, first.Access.ErrorCategory)
	require.Equal(t, "1", first.Access.RetryAfter)

	blocked, err := fetcher.FetchFeedWithContextDetailed(context.Background(), feedURL)
	require.ErrorIs(t, err, ErrFeedCircuitOpen)
	require.Equal(t, int32(1), atomic.LoadInt32(&requestCount))

	time.Sleep(1100 * time.Millisecond)
	probe, err := fetcher.FetchFeedWithContextDetailed(context.Background(), feedURL)
	require.NoError(t, err)
	require.Equal(t, "Rate recovered", probe.Feed.Title)
	require.Equal(t, CircuitStateProbe, probe.Access.CircuitState)
	require.Equal(t, int32(2), atomic.LoadInt32(&requestCount))
	_ = blocked
}

func TestCoordinatorUsesBoundedBackoffForInvalidRetryAfter(t *testing.T) {
	var requestCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&requestCount, 1) == 1 {
			w.Header().Set("Retry-After", "not-a-duration")
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(testSnapshotFeed("Backoff recovered")))
	}))
	t.Cleanup(server.Close)

	coordinator := NewCoordinator(CoordinatorConfig{DomainPolicies: map[string]DomainPolicy{
		TargetDomain(server.URL): {
			MaxConcurrency:      1,
			RetryBackoffInitial: 40 * time.Millisecond,
			RetryBackoffMax:     40 * time.Millisecond,
			CircuitCooldown:     time.Millisecond,
		},
	}})
	fetcher := NewFetcherWithCoordinator(2*time.Second, coordinator)
	feedURL := server.URL + "/feed.xml"

	_, err := fetcher.FetchFeedWithContextDetailed(context.Background(), feedURL)
	require.Error(t, err)
	_, err = fetcher.FetchFeedWithContextDetailed(context.Background(), feedURL)
	require.ErrorIs(t, err, ErrFeedCircuitOpen)
	require.Equal(t, int32(1), atomic.LoadInt32(&requestCount))

	time.Sleep(60 * time.Millisecond)
	probe, err := fetcher.FetchFeedWithContextDetailed(context.Background(), feedURL)
	require.NoError(t, err)
	require.Equal(t, "Backoff recovered", probe.Feed.Title)
	require.Equal(t, int32(2), atomic.LoadInt32(&requestCount))
}

func TestCoordinatorDoesNotReleaseDomainQueueDuringRecoveryProbe(t *testing.T) {
	var requestCount int32
	probeStarted := make(chan struct{})
	releaseProbe := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&requestCount, 1)
		if count == 1 {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if r.URL.Path == "/probe.xml" {
			close(probeStarted)
			<-releaseProbe
		}
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(testSnapshotFeed("Probe recovered")))
	}))
	t.Cleanup(server.Close)

	coordinator := NewCoordinator(CoordinatorConfig{DomainPolicies: map[string]DomainPolicy{
		TargetDomain(server.URL): {MaxConcurrency: 1, CircuitCooldown: 30 * time.Millisecond},
	}})
	fetcher := NewFetcherWithCoordinator(2*time.Second, coordinator)
	first, err := fetcher.FetchFeedWithContextDetailed(context.Background(), server.URL+"/initial.xml")
	require.Error(t, err)
	require.Equal(t, CircuitStateOpen, first.Access.CircuitState)

	time.Sleep(50 * time.Millisecond)
	probeResult := make(chan *FetchResult, 1)
	probeErr := make(chan error, 1)
	go func() {
		result, err := fetcher.FetchFeedWithContextDetailed(context.Background(), server.URL+"/probe.xml")
		probeResult <- result
		probeErr <- err
	}()
	select {
	case <-probeStarted:
	case <-time.After(time.Second):
		t.Fatal("recovery probe did not reach the upstream server")
	}

	blocked, err := fetcher.FetchFeedWithContextDetailed(context.Background(), server.URL+"/other.xml")
	require.ErrorIs(t, err, ErrFeedCircuitOpen)
	require.Equal(t, ErrorCategoryCircuitOpen, blocked.Access.ErrorCategory)
	require.Equal(t, int32(2), atomic.LoadInt32(&requestCount))

	close(releaseProbe)
	result := <-probeResult
	require.NoError(t, <-probeErr)
	require.Equal(t, "Probe recovered", result.Feed.Title)
	require.Equal(t, CircuitStateProbe, result.Access.CircuitState)
}

func TestCoordinatorBlocksQueuedDomainWorkAfterFirst403(t *testing.T) {
	var requestCount int32
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&requestCount, 1)
		if count == 1 {
			close(firstStarted)
			<-releaseFirst
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(testSnapshotFeed("Should not be fetched")))
	}))
	t.Cleanup(server.Close)

	coordinator := NewCoordinator(CoordinatorConfig{DomainPolicies: map[string]DomainPolicy{
		TargetDomain(server.URL): {MaxConcurrency: 1, CircuitCooldown: time.Minute},
	}})
	fetcher := NewFetcherWithCoordinator(2*time.Second, coordinator)

	firstResult := make(chan *FetchResult, 1)
	firstErr := make(chan error, 1)
	go func() {
		result, err := fetcher.FetchFeedWithContextDetailed(context.Background(), server.URL+"/first.xml")
		firstResult <- result
		firstErr <- err
	}()
	<-firstStarted

	secondResult := make(chan *FetchResult, 1)
	secondErr := make(chan error, 1)
	go func() {
		result, err := fetcher.FetchFeedWithContextDetailed(context.Background(), server.URL+"/second.xml")
		secondResult <- result
		secondErr <- err
	}()
	time.Sleep(20 * time.Millisecond)
	close(releaseFirst)

	require.Error(t, <-firstErr)
	require.Equal(t, CircuitStateOpen, (<-firstResult).Access.CircuitState)
	blocked := <-secondResult
	require.ErrorIs(t, <-secondErr, ErrFeedCircuitOpen)
	require.Equal(t, ErrorCategoryCircuitOpen, blocked.Access.ErrorCategory)
	require.Equal(t, int32(1), atomic.LoadInt32(&requestCount))
}
