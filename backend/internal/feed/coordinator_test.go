package feed

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const coordinatorTestFeed = `<?xml version="1.0"?><rss version="2.0"><channel><title>Coordinator Feed</title><item><title>Episode</title><guid>coordinator-episode</guid><pubDate>Tue, 14 Jul 2026 08:00:00 GMT</pubDate><description>Details</description></item></channel></rss>`

func TestCoordinatorCoalescesConcurrentRequestsAndHonorsRefreshInterval(t *testing.T) {
	var requestCount int32
	var current int32
	var maxConcurrent int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		active := atomic.AddInt32(&current, 1)
		updateMaxConcurrent(&maxConcurrent, active)
		time.Sleep(60 * time.Millisecond)
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(coordinatorTestFeed))
		atomic.AddInt32(&current, -1)
	}))
	t.Cleanup(server.Close)

	coordinator := NewCoordinator(CoordinatorConfig{DomainPolicies: map[string]DomainPolicy{
		"127.0.0.1": {MaxConcurrency: 1, MinRefreshInterval: 150 * time.Millisecond},
	}})
	fetcherA := NewFetcherWithCoordinator(2*time.Second, coordinator)
	fetcherB := NewFetcherWithCoordinator(2*time.Second, coordinator)
	feedURL := server.URL + "/feed.xml#same-resource"

	start := make(chan struct{})
	results := make(chan *FetchResult, 2)
	errors := make(chan error, 2)
	for _, fetcher := range []*Fetcher{fetcherA, fetcherB} {
		go func(fetcher *Fetcher) {
			<-start
			result, err := fetcher.FetchFeedWithContextDetailed(context.Background(), feedURL)
			results <- result
			errors <- err
		}(fetcher)
	}
	close(start)

	var sharedCount int
	for i := 0; i < 2; i++ {
		if err := <-errors; err != nil {
			t.Fatalf("coalesced request failed: %v", err)
		}
		result := <-results
		if result == nil || result.Feed == nil {
			t.Fatal("coalesced request did not return a parsed feed")
		}
		if result.Access.SourceType == AccessSourceSharedCache {
			sharedCount++
		}
	}
	if got := atomic.LoadInt32(&requestCount); got != 1 {
		t.Fatalf("expected one upstream request for overlapping workflows, got %d", got)
	}
	if got := atomic.LoadInt32(&maxConcurrent); got != 1 {
		t.Fatalf("expected domain concurrency 1, got %d", got)
	}
	if sharedCount != 1 {
		t.Fatalf("expected one waiter to be marked shared_cache, got %d", sharedCount)
	}

	cached, err := fetcherA.FetchFeedWithContextDetailed(context.Background(), server.URL+"/feed.xml")
	if err != nil || cached.Access.SourceType != AccessSourceSharedCache || cached.Access.CacheStatus != CacheStatusHit {
		t.Fatalf("expected the minimum interval to reuse shared data, result=%#v err=%v", cached, err)
	}
	if got := atomic.LoadInt32(&requestCount); got != 1 {
		t.Fatalf("minimum refresh interval caused an extra request: %d", got)
	}

	time.Sleep(170 * time.Millisecond)
	if _, err := fetcherA.FetchFeedWithContextDetailed(context.Background(), server.URL+"/feed.xml"); err != nil {
		t.Fatalf("request after refresh interval failed: %v", err)
	}
	if got := atomic.LoadInt32(&requestCount); got != 2 {
		t.Fatalf("expected a new request after refresh interval, got %d", got)
	}
}

func TestCoordinatorDoesNotSerializeDifferentUnconfiguredDomains(t *testing.T) {
	var current int32
	var maxConcurrent int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		active := atomic.AddInt32(&current, 1)
		updateMaxConcurrent(&maxConcurrent, active)
		time.Sleep(50 * time.Millisecond)
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(coordinatorTestFeed))
		atomic.AddInt32(&current, -1)
	}))
	t.Cleanup(server.Close)

	fetcher := NewFetcherWithCoordinator(2*time.Second, NewCoordinator(DefaultCoordinatorConfig()))
	var wg sync.WaitGroup
	for _, path := range []string{"/one.xml", "/two.xml"} {
		wg.Add(1)
		go func(path string) {
			defer wg.Done()
			if _, err := fetcher.FetchFeedWithContextDetailed(context.Background(), server.URL+path); err != nil {
				t.Errorf("unconfigured domain request failed: %v", err)
			}
		}(path)
	}
	wg.Wait()
	if got := atomic.LoadInt32(&maxConcurrent); got < 2 {
		t.Fatalf("different unconfigured Feed URLs were serialized, max concurrency=%d", got)
	}
}

func TestCoordinatorReleasesFailedInflightRequest(t *testing.T) {
	var requestCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&requestCount, 1) == 1 {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(coordinatorTestFeed))
	}))
	t.Cleanup(server.Close)

	fetcher := NewFetcherWithCoordinator(2*time.Second, NewCoordinator(DefaultCoordinatorConfig()))
	if _, err := fetcher.FetchFeedWithContextDetailed(context.Background(), server.URL+"/feed.xml"); err == nil {
		t.Fatal("expected the first request to fail")
	}
	if _, err := fetcher.FetchFeedWithContextDetailed(context.Background(), server.URL+"/feed.xml"); err != nil {
		t.Fatalf("failed inflight state permanently blocked the next request: %v", err)
	}
	if got := atomic.LoadInt32(&requestCount); got != 2 {
		t.Fatalf("expected a second upstream attempt after failure, got %d", got)
	}
}

func TestCanonicalizeURLKeepsFeedIdentityWhileNormalizingRepresentation(t *testing.T) {
	got := CanonicalizeURL("HTTP://Example.COM:80/feed.xml?b=2&a=1#ignored")
	if got != "http://example.com/feed.xml?b=2&a=1" {
		t.Fatalf("unexpected canonical URL: %q", got)
	}
	if strings.Contains(got, "ignored") {
		t.Fatal("URL fragments must not affect Feed identity")
	}
}

func updateMaxConcurrent(max *int32, current int32) {
	for {
		previous := atomic.LoadInt32(max)
		if current <= previous || atomic.CompareAndSwapInt32(max, previous, current) {
			return
		}
	}
}
