package feed

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// robotsFetchRecorder is the injected robots fetch seam: it records every call
// (so tests assert fetch counts) and returns a scripted response. No real
// network is touched.
type robotsFetchRecorder struct {
	mu      sync.Mutex
	calls   []string
	respond func(scheme, host string) (int, []byte, error)
}

// serveRobotsNotFound keeps Feed httptest handlers from consuming their
// scripted Feed response when the production Fetcher performs its origin
// robots lookup. A missing robots document is an advisory allow-all result.
func serveRobotsNotFound(w http.ResponseWriter, r *http.Request) bool {
	if r.URL.Path != "/robots.txt" {
		return false
	}
	w.WriteHeader(http.StatusNotFound)
	return true
}

func (r *robotsFetchRecorder) fetch(_ context.Context, scheme, host string) (int, []byte, error) {
	r.mu.Lock()
	r.calls = append(r.calls, scheme+"://"+host)
	r.mu.Unlock()
	if r.respond != nil {
		return r.respond(scheme, host)
	}
	return 0, nil, errors.New("no response scripted")
}

func (r *robotsFetchRecorder) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.calls)
}

func (r *robotsFetchRecorder) callsSnapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.calls...)
}

// newAdvancingGate builds a gate whose clock the test advances via the returned
// func. All durations are simulated — no real sleeping.
func newAdvancingGate(rec *robotsFetchRecorder, opts ...RobotsGateOption) (*RobotsGate, func(time.Duration)) {
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	clockOpts := append([]RobotsGateOption{}, opts...)
	clockOpts = append(clockOpts, WithRobotsClock(func() time.Time { return now }))
	gate := NewRobotsGate(feedRobotsAgent, rec.fetch, clockOpts...)
	return gate, func(d time.Duration) { now = now.Add(d) }
}

func TestRobotsDisallowRuleBlocksPath(t *testing.T) {
	body := []byte("User-agent: *\nDisallow: /private\n")
	rec := &robotsFetchRecorder{respond: func(string, string) (int, []byte, error) { return 200, body, nil }}
	gate, _ := newAdvancingGate(rec)

	d := gate.Allowed(context.Background(), "https://example.com/private/feed.xml")
	assert.False(t, d.Allowed, "Disallow /private must block /private/feed.xml")
	assert.Equal(t, "rule_disallow", d.Reason)
}

func TestRobotsAllowRulePermitsOtherPaths(t *testing.T) {
	body := []byte("User-agent: *\nDisallow: /private\n")
	rec := &robotsFetchRecorder{respond: func(string, string) (int, []byte, error) { return 200, body, nil }}
	gate, _ := newAdvancingGate(rec)

	d := gate.Allowed(context.Background(), "https://example.com/feed.xml")
	assert.True(t, d.Allowed)
	assert.Equal(t, "rule_allow", d.Reason)
}

func TestRobotsAgentSpecificGroupOverridesStar(t *testing.T) {
	// MagicPodcast is explicitly allowed where "*" is denied; the agent-specific
	// group wins, so the path is permitted.
	body := []byte("User-agent: *\nDisallow: /\nUser-agent: MagicPodcast\nAllow: /\n")
	rec := &robotsFetchRecorder{respond: func(string, string) (int, []byte, error) { return 200, body, nil }}
	gate, _ := newAdvancingGate(rec)

	d := gate.Allowed(context.Background(), "https://example.com/feed.xml")
	assert.True(t, d.Allowed)
}

func TestRobots4xxIsAllowAll(t *testing.T) {
	rec := &robotsFetchRecorder{respond: func(string, string) (int, []byte, error) { return 404, nil, nil }}
	gate, _ := newAdvancingGate(rec)

	d := gate.Allowed(context.Background(), "https://example.com/feed.xml")
	assert.True(t, d.Allowed, "4xx robots must be treated as allowAll (RFC 9309)")
}

func TestRobots5xxIsAllowNegativeCache(t *testing.T) {
	rec := &robotsFetchRecorder{respond: func(string, string) (int, []byte, error) { return 503, nil, nil }}
	gate, _ := newAdvancingGate(rec)

	d := gate.Allowed(context.Background(), "https://example.com/feed.xml")
	assert.True(t, d.Allowed, "5xx robots must NOT block the whole domain (advisory-only override)")
	assert.Equal(t, "fetch_failed_allow", d.Reason)
}

func TestRobotsNetworkErrorIsAllowNegativeCache(t *testing.T) {
	rec := &robotsFetchRecorder{respond: func(string, string) (int, []byte, error) { return 0, nil, errors.New("i/o timeout") }}
	gate, _ := newAdvancingGate(rec)

	d := gate.Allowed(context.Background(), "https://example.com/feed.xml")
	assert.True(t, d.Allowed, "a robots fetch failure must never masquerade as access-denied")
	assert.Equal(t, "fetch_failed_allow", d.Reason)
}

func TestRobotsUnparseableBodyIsAllow(t *testing.T) {
	// robotstxt parses leniently: a 200 body that is not a valid robots document
	// parses to an allow-all ruleset (no error), so the safe outcome is
	// rule_allow — the Feed is fetched, and a broken robots endpoint never blocks
	// the domain.
	rec := &robotsFetchRecorder{respond: func(string, string) (int, []byte, error) { return 200, []byte("<<<not robots>>>"), nil }}
	gate, _ := newAdvancingGate(rec)

	d := gate.Allowed(context.Background(), "https://example.com/feed.xml")
	assert.True(t, d.Allowed, "a non-robots 2xx body must not block the Feed")
	assert.Equal(t, "rule_allow", d.Reason)
}

func TestRobotsCacheHitSharesOneFetchAcrossPaths(t *testing.T) {
	body := []byte("User-agent: *\nAllow: /\n")
	rec := &robotsFetchRecorder{respond: func(string, string) (int, []byte, error) { return 200, body, nil }}
	gate, _ := newAdvancingGate(rec)

	for _, path := range []string{"/a.xml", "/b.xml", "/c.xml"} {
		d := gate.Allowed(context.Background(), "https://example.com"+path)
		assert.True(t, d.Allowed)
	}
	assert.Equal(t, 1, rec.callCount(), "one robots fetch serves every path on the domain")
}

func TestRobotsCacheSeparatesSchemes(t *testing.T) {
	body := []byte("User-agent: *\nAllow: /\n")
	rec := &robotsFetchRecorder{respond: func(string, string) (int, []byte, error) { return 200, body, nil }}
	gate, _ := newAdvancingGate(rec)

	assert.True(t, gate.Allowed(context.Background(), "http://example.com/feed.xml").Allowed)
	assert.True(t, gate.Allowed(context.Background(), "https://example.com/feed.xml").Allowed)
	assert.Equal(t, []string{"http://example.com", "https://example.com"}, rec.callsSnapshot(),
		"HTTP and HTTPS are different robots origins")
}

func TestRobotsCacheSeparatesPorts(t *testing.T) {
	body := []byte("User-agent: *\nAllow: /\n")
	rec := &robotsFetchRecorder{respond: func(string, string) (int, []byte, error) { return 200, body, nil }}
	gate, _ := newAdvancingGate(rec)

	assert.True(t, gate.Allowed(context.Background(), "http://example.com:8080/feed.xml").Allowed)
	assert.True(t, gate.Allowed(context.Background(), "http://example.com:8081/feed.xml").Allowed)
	assert.Equal(t, []string{"http://example.com:8080", "http://example.com:8081"}, rec.callsSnapshot(),
		"different ports are different robots origins")
}

func TestFetcherRobotsRequestPreservesAuthorityPort(t *testing.T) {
	var gotHost, gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHost = r.Host
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(server.Close)

	authority := strings.TrimPrefix(server.URL, "http://")
	fetcher := &Fetcher{
		httpClient: server.Client(),
		httpConfig: DefaultFeedHTTPConfig(time.Second),
	}
	status, _, err := fetcher.fetchRobotsTXT(context.Background(), "http", authority)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, status)
	assert.Equal(t, authority, gotHost, "robots request must target the feed origin port")
	assert.Equal(t, "/robots.txt", gotPath)
}

func TestRobotsCacheExpiresAfterTTL(t *testing.T) {
	body := []byte("User-agent: *\nAllow: /\n")
	rec := &robotsFetchRecorder{respond: func(string, string) (int, []byte, error) { return 200, body, nil }}
	gate, advance := newAdvancingGate(rec, WithRobotsTTL(time.Minute))

	_ = gate.Allowed(context.Background(), "https://example.com/feed.xml")
	assert.Equal(t, 1, rec.callCount())

	advance(30 * time.Second)
	_ = gate.Allowed(context.Background(), "https://example.com/feed.xml")
	assert.Equal(t, 1, rec.callCount(), "within TTL: still a cache hit")

	advance(time.Minute) // total 90s > 60s TTL
	_ = gate.Allowed(context.Background(), "https://example.com/feed.xml")
	assert.Equal(t, 2, rec.callCount(), "past TTL: re-fetch")
}

func TestRobotsNegativeCacheExpiresAfterFailTTL(t *testing.T) {
	rec := &robotsFetchRecorder{respond: func(string, string) (int, []byte, error) { return 0, nil, errors.New("down") }}
	gate, advance := newAdvancingGate(rec, WithRobotsFailTTL(time.Minute))

	_ = gate.Allowed(context.Background(), "https://example.com/feed.xml")
	assert.Equal(t, 1, rec.callCount())

	advance(30 * time.Second)
	_ = gate.Allowed(context.Background(), "https://example.com/feed.xml")
	assert.Equal(t, 1, rec.callCount(), "within failTTL: negative cache hit, no re-fetch")

	advance(40 * time.Second) // total 70s > 60s failTTL
	_ = gate.Allowed(context.Background(), "https://example.com/feed.xml")
	assert.Equal(t, 2, rec.callCount(), "past failTTL: bounded re-fetch")
}

func TestRobotsSingleflightCoalescesConcurrentCalls(t *testing.T) {
	body := []byte("User-agent: *\nAllow: /\n")
	started := make(chan struct{})
	release := make(chan struct{})
	var count int32
	rec := &robotsFetchRecorder{respond: func(string, string) (int, []byte, error) {
		if atomic.AddInt32(&count, 1) == 1 {
			close(started)
			<-release // block the leader so followers must coalesce
		}
		return 200, body, nil
	}}
	gate, _ := newAdvancingGate(rec)

	const n = 4
	results := make([]RobotsDecision, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = gate.Allowed(context.Background(), "https://example.com/feed.xml")
		}(i)
	}
	<-started      // leader is mid-fetch
	close(release) // let it complete
	wg.Wait()

	assert.Equal(t, int32(1), atomic.LoadInt32(&count), "concurrent same-domain calls share one robots fetch")
	for _, d := range results {
		assert.True(t, d.Allowed)
	}
}

func TestRobotsTTLCappedAt24Hours(t *testing.T) {
	g := NewRobotsGate(feedRobotsAgent, func(context.Context, string, string) (int, []byte, error) {
		return 404, nil, nil
	}, WithRobotsTTL(48*time.Hour))
	assert.Equal(t, maxRobotsCacheTTL, g.ttl, "no path may cache a robots decision longer than 24h")
}

func TestRobotsAllowedNoHostAllowsAndSkipsFetch(t *testing.T) {
	rec := &robotsFetchRecorder{respond: func(string, string) (int, []byte, error) {
		t.Fatal("robots must not be fetched for a hostless/non-HTTP URL")
		return 0, nil, nil
	}}
	gate, _ := newAdvancingGate(rec)

	d := gate.Allowed(context.Background(), "not a url with a host")
	assert.True(t, d.Allowed)
	assert.Equal(t, "no_host", d.Reason)
	assert.Equal(t, 0, rec.callCount())
}

func TestSplitRobotsURLParsing(t *testing.T) {
	// HTTP scheme + upper-case host normalized.
	scheme, host, path, ok := splitRobotsURL("http://EXAMPLE.com/private/x.xml")
	assert.True(t, ok)
	assert.Equal(t, "http", scheme)
	assert.Equal(t, "example.com", host)
	assert.Equal(t, "/private/x.xml", path)

	// Explicit ports remain part of the authority used for robots caching and
	// the robots request target.
	_, authority, _, ok := splitRobotsURL("https://EXAMPLE.com:8443/feed.xml")
	assert.True(t, ok)
	assert.Equal(t, "example.com:8443", authority)

	// Non-HTTP(S) scheme rejected.
	_, _, _, ok = splitRobotsURL("ftp://example.com/feed.xml")
	assert.False(t, ok)

	// Hostless URL rejected.
	_, _, _, ok = splitRobotsURL("/just/a/path")
	assert.False(t, ok)

	// Empty path collapses to "/".
	_, _, path, ok = splitRobotsURL("https://example.com")
	assert.True(t, ok)
	assert.Equal(t, "/", path)
}
