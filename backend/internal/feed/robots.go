package feed

import (
	"context"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/temoto/robotstxt"
)

// feedRobotsAgent is the stable product token tested against robots.txt groups.
// Site operators who want to address MagicPodcast specifically write
// "User-agent: MagicPodcast"; everyone else falls under the universal "*"
// group. It intentionally carries no version so a rule is not silently
// invalidated by a release.
const feedRobotsAgent = "MagicPodcast"

const (
	// defaultRobotsCacheTTL is the maximum time a successfully fetched
	// robots.txt decision is cached per domain. RFC 9309 does not mandate a
	// refresh interval; 24h is the standard cautious ceiling and matches the
	// ticket's hard cap.
	defaultRobotsCacheTTL = 24 * time.Hour
	// maxRobotsCacheTTL clamps any configured TTL so no path can cache a
	// robots decision longer than 24h.
	maxRobotsCacheTTL = 24 * time.Hour
	// defaultRobotsFailTTL is the negative-cache window used when the robots
	// fetch itself fails (timeout / 5xx / network / unparseable body). The
	// decision during that window is "allow" (see RobotsDecision), and the
	// window bounds how often a failing robots endpoint is re-hit so a Feed
	// storm never becomes a robots storm.
	defaultRobotsFailTTL = 5 * time.Minute
)

// RobotsDecision is the outcome of a robots.txt admission check for one Feed URL.
type RobotsDecision struct {
	// Allowed reports whether the Feed path may be fetched.
	Allowed bool
	// Reason is a short structured label for logs and diagnostics. It
	// distinguishes a real rule denial from a fetch failure so operators can
	// tell a policy rejection apart from an HTTP 403 upstream. Values:
	// rule_allow | rule_disallow | fetch_failed_allow | no_host | no_gate.
	Reason string
}

// robotsEntry is one cached per-domain robots decision. source is "rule" when a
// robots document was parsed (2xx or 4xx-allowAll) and the per-path decision is
// deferred to decide(); "fail" when the fetch itself failed (network/timeout/
// 5xx/unparseable), in which case the decision is a flat allow for the
// negative-cache window.
type robotsEntry struct {
	data      *robotstxt.RobotsData
	source    string
	expiresAt time.Time
}

type robotsCall struct {
	done  chan struct{}
	entry robotsEntry
}

// RobotsGate caches per-domain robots.txt decisions and coalesces concurrent
// lookups for the same domain into a single fetch. It is advisory ONLY: a
// fetch failure (timeout / 5xx / unparseable body) is treated as "allow" for a
// bounded negative-cache window rather than blocking every Feed on the domain,
// because a flaky robots endpoint must never masquerade as an access-denied
// upstream. Only an explicit universal Disallow rule for the Feed path blocks
// the fetch, and that block surfaces as a distinct policy_rejected category —
// never an HTTP 403.
//
// The gate is concurrency-safe and deterministic in tests via the injected now
// and fetchRobots seam.
type RobotsGate struct {
	mu          sync.Mutex
	cache       map[string]robotsEntry
	inFlight    map[string]*robotsCall
	ttl         time.Duration
	failTTL     time.Duration
	userAgent   string
	now         func() time.Time
	fetchRobots func(ctx context.Context, scheme, host string) (status int, body []byte, err error)
}

// RobotsGateOption configures a RobotsGate at construction.
type RobotsGateOption func(*RobotsGate)

// WithRobotsTTL sets the success-cache TTL (clamped to maxRobotsCacheTTL).
func WithRobotsTTL(d time.Duration) RobotsGateOption {
	return func(g *RobotsGate) {
		if d > 0 {
			g.ttl = d
		}
	}
}

// WithRobotsFailTTL sets the negative-cache TTL applied on fetch failure.
func WithRobotsFailTTL(d time.Duration) RobotsGateOption {
	return func(g *RobotsGate) {
		if d > 0 {
			g.failTTL = d
		}
	}
}

// WithRobotsClock injects a fixed clock for deterministic tests.
func WithRobotsClock(now func() time.Time) RobotsGateOption {
	return func(g *RobotsGate) {
		if now != nil {
			g.now = now
		}
	}
}

// NewRobotsGate builds a gate that uses fetchRobots to retrieve a domain's
// robots.txt (scheme+host → status/body/err). fetchRobots performs a single
// bounded attempt; the Fetcher wraps it with the shared RetryPolicy so robots
// fetches share the same retry budget, and the gate's singleflight + cache
// ensure a Feed storm never becomes a robots storm.
func NewRobotsGate(userAgent string, fetchRobots func(ctx context.Context, scheme, host string) (int, []byte, error), opts ...RobotsGateOption) *RobotsGate {
	g := &RobotsGate{
		cache:       make(map[string]robotsEntry),
		inFlight:    make(map[string]*robotsCall),
		ttl:         defaultRobotsCacheTTL,
		failTTL:     defaultRobotsFailTTL,
		userAgent:   feedRobotsAgent,
		now:         time.Now,
		fetchRobots: fetchRobots,
	}
	for _, opt := range opts {
		opt(g)
	}
	if g.ttl > maxRobotsCacheTTL {
		g.ttl = maxRobotsCacheTTL
	}
	return g
}

// Allowed reports whether feedURL may be fetched under the cached robots.txt
// decision for its domain. It never returns an error: a robots fetch failure
// is an "allow" decision with a bounded negative cache, so callers always get a
// definitive admission answer and the Feed pipeline never blocks on robots
// infrastructure.
func (g *RobotsGate) Allowed(ctx context.Context, feedURL string) RobotsDecision {
	scheme, host, path, ok := splitRobotsURL(feedURL)
	if !ok {
		// Unparseable Feed URL or no host: do not block on robots. The fetch
		// itself will fail with a proper invalid-request classification.
		return RobotsDecision{Allowed: true, Reason: "no_host"}
	}

	g.mu.Lock()
	if entry, ok := g.cache[host]; ok && g.now().Before(entry.expiresAt) {
		g.mu.Unlock()
		return g.decide(entry, path)
	}
	if call, ok := g.inFlight[host]; ok {
		g.mu.Unlock()
		select {
		case <-call.done:
			return g.decide(call.entry, path)
		case <-ctx.Done():
			// Context cancelled while waiting on another goroutine's robots
			// fetch: do not block the Feed. Allow and let the Feed request's
			// own context propagate cancellation.
			return RobotsDecision{Allowed: true, Reason: "fetch_failed_allow"}
		}
	}
	call := &robotsCall{done: make(chan struct{})}
	g.inFlight[host] = call
	g.mu.Unlock()

	entry := g.fetchEntry(ctx, scheme, host)

	g.mu.Lock()
	g.cache[host] = entry
	delete(g.inFlight, host)
	call.entry = entry
	close(call.done)
	g.mu.Unlock()

	return g.decide(entry, path)
}

func (g *RobotsGate) fetchEntry(ctx context.Context, scheme, host string) robotsEntry {
	now := g.now()
	status, body, err := g.fetchRobots(ctx, scheme, host)
	if err != nil {
		// Transport/timeout/DNS failure: allow for the negative-cache window.
		return robotsEntry{source: "fail", expiresAt: now.Add(g.failTTL)}
	}
	data, parseErr := robotstxt.FromStatusAndBytes(status, body)
	if parseErr != nil {
		// 2xx with an unparseable body: treat like a fetch failure (allow,
		// negative cache) rather than guessing at partial rules. (robotstxt is
		// lenient and rarely errors on body content, but this stays defensive.)
		return robotsEntry{source: "fail", expiresAt: now.Add(g.failTTL)}
	}
	if status >= 500 && status < 600 {
		// robotstxt returns disallowAll for 5xx per RFC 9309, but a transient
		// robots-server error must not block every Feed on the domain. Override
		// to allow for the negative-cache window so the real Feed request can
		// proceed and its own circuitry handles upstream health.
		return robotsEntry{source: "fail", expiresAt: now.Add(g.failTTL)}
	}
	// 2xx (parsed) or 4xx (allowAll): a definitive robots decision, cached for
	// the full success TTL. The per-path allow/deny is evaluated lazily in
	// decide() so one cached robots document serves every Feed path on the
	// domain.
	return robotsEntry{data: data, source: "rule", expiresAt: now.Add(g.ttl)}
}

// decide evaluates a cached entry against a Feed path. A "rule" entry tests the
// path explicitly (rule_allow/rule_disallow); a "fail" entry is a flat
// fetch_failed_allow (negative cache). The reason is derived purely from the
// source + the per-path TestAgent result, so it never mislabels a denial.
func (g *RobotsGate) decide(entry robotsEntry, path string) RobotsDecision {
	if entry.source == "fail" || entry.data == nil {
		return RobotsDecision{Allowed: true, Reason: "fetch_failed_allow"}
	}
	if entry.data.TestAgent(path, g.userAgent) {
		return RobotsDecision{Allowed: true, Reason: "rule_allow"}
	}
	return RobotsDecision{Allowed: false, Reason: "rule_disallow"}
}

func splitRobotsURL(rawURL string) (scheme, host, path string, ok bool) {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" {
		return "", "", "", false
	}
	scheme = strings.ToLower(parsed.Scheme)
	if scheme == "" {
		scheme = "https"
	}
	if scheme != "http" && scheme != "https" {
		// Non-HTTP(S) Feed URLs are rejected by the redirect policy already; do
		// not attempt a robots lookup for them.
		return "", "", "", false
	}
	host = strings.ToLower(parsed.Hostname())
	if host == "" {
		return "", "", "", false
	}
	if parsed.Path == "" {
		path = "/"
	} else {
		path = parsed.Path
	}
	return scheme, host, path, true
}
