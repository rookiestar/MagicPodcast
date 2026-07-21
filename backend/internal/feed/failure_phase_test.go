package feed

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"magicpodcast/internal/logger"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

const feedFailureLogMessage = "feed fetch failed"

// capturedLogEntry is a snapshot of one structured log entry: its message and
// the whitelisted field map, copied so later mutation of a shared entry cannot
// rewrite history.
type capturedLogEntry struct {
	Message string
	Fields  map[string]interface{}
}

// logCaptureHook captures Warn/Error entries from the process-wide logger.
// logrus has no RemoveHook, so the package registers one shared hook at init
// and each test calls reset() before triggering exactly one failure; tests in
// this package run sequentially.
type logCaptureHook struct {
	mu      sync.Mutex
	entries []capturedLogEntry
}

func (h *logCaptureHook) Fire(entry *logrus.Entry) error {
	fields := make(map[string]interface{}, len(entry.Data))
	for k, v := range entry.Data {
		fields[k] = v
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.entries = append(h.entries, capturedLogEntry{Message: entry.Message, Fields: fields})
	return nil
}

func (h *logCaptureHook) Levels() []logrus.Level {
	return []logrus.Level{logrus.WarnLevel, logrus.ErrorLevel}
}

func (h *logCaptureHook) reset() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.entries = nil
}

func (h *logCaptureHook) failureEntry() (capturedLogEntry, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, e := range h.entries {
		if e.Message == feedFailureLogMessage {
			return e, true
		}
	}
	return capturedLogEntry{}, false
}

var sharedCapture = &logCaptureHook{}

func init() {
	logger.GetLogger().AddHook(sharedCapture)
}

// newFailurePhaseFetcher builds a fetcher with a nil coordinator so the
// failure-phase and structured-log logic in the Fetcher layer is exercised in
// isolation, without the shared circuit breaker or concurrency gate.
func newFailurePhaseFetcher() *Fetcher {
	return NewFetcherWithHTTPConfig(DefaultFeedHTTPConfig(5*time.Second), nil)
}

// fetchFailure resets the capture, performs exactly one failing fetch, and
// asserts both that the fetch failed and that a structured "feed fetch failed"
// Warn entry was emitted.
func fetchFailure(t *testing.T, fetcher *Fetcher, feedURL string) (*FetchResult, capturedLogEntry) {
	t.Helper()
	sharedCapture.reset()
	result, err := fetcher.FetchFeedWithContextDetailed(context.Background(), feedURL)
	require.Error(t, err, "expected the fetch to fail")
	require.NotNil(t, result, "an access result must be recorded even on failure")
	entry, ok := sharedCapture.failureEntry()
	require.True(t, ok, "expected a structured %q Warn entry", feedFailureLogMessage)
	return result, entry
}

// newTLSFailingServer returns an https:// URL whose TCP listener accepts the
// connection then immediately closes it, forcing the TLS handshake to fail
// after ConnectDone — so the observed phase is tls, not connect.
func newTLSFailingServer(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()
	t.Cleanup(func() { _ = ln.Close() })
	return "https://" + ln.Addr().String() + "/feed.xml"
}

// TestFailurePhaseDNS is the #28 acceptance that a DNS resolution failure is
// classified as dns (httptrace DNSStart) rather than connect.
func TestFailurePhaseDNS(t *testing.T) {
	fetcher := newFailurePhaseFetcher()
	_, entry := fetchFailure(t, fetcher, "https://magicpodcast-no-such-host-dns.invalid/feed.xml")
	require.Equal(t, string(FailurePhaseDNS), entry.Fields["failure_phase"])
}

// TestFailurePhaseConnect is the #28 acceptance that a TCP-level refusal is
// classified as connect (the listener is closed so the dial sees an immediate
// connection-refused, never a hang).
func TestFailurePhaseConnect(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close())

	fetcher := newFailurePhaseFetcher()
	_, entry := fetchFailure(t, fetcher, "http://"+addr+"/feed.xml")
	require.Equal(t, string(FailurePhaseConnect), entry.Fields["failure_phase"])
}

// TestFailurePhaseTLS is the #28 acceptance that a TLS handshake failure is
// classified as tls (it occurs after a successful TCP connect).
func TestFailurePhaseTLS(t *testing.T) {
	fetcher := newFailurePhaseFetcher()
	_, entry := fetchFailure(t, fetcher, newTLSFailingServer(t))
	require.Equal(t, string(FailurePhaseTLS), entry.Fields["failure_phase"])
}

// TestFailurePhaseResponseHeaderFor403 is the core #28 guarantee: an HTTP 403
// is response_header (headers received), never connect — so a fast WAF/CDN
// refusal cannot be misread as a network/TLS problem.
func TestFailurePhaseResponseHeaderFor403(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(server.Close)

	fetcher := newFailurePhaseFetcher()
	result, entry := fetchFailure(t, fetcher, server.URL+"/feed.xml")
	require.Equal(t, string(FailurePhaseResponseHeader), entry.Fields["failure_phase"], "403 must be response_header, never connect")
	require.Equal(t, ErrorCategoryAccessDenied, result.Access.ErrorCategory)
	require.NotNil(t, result.Access.HTTPStatus)
	require.Equal(t, http.StatusForbidden, *result.Access.HTTPStatus)
	require.Equal(t, "60", entry.Fields["retry_after"])
}

// TestFailurePhaseBodyReadForInvalidXML is the #28 acceptance that a parse
// failure after headers arrived is classified as body_read.
func TestFailurePhaseBodyReadForInvalidXML(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte("<<< this is not valid XML and must fail to parse >>>"))
	}))
	t.Cleanup(server.Close)

	fetcher := newFailurePhaseFetcher()
	result, entry := fetchFailure(t, fetcher, server.URL+"/feed.xml")
	require.Equal(t, string(FailurePhaseBodyRead), entry.Fields["failure_phase"])
	require.Equal(t, ErrorCategoryParse, result.Access.ErrorCategory)
	require.NotNil(t, result.Access.HTTPStatus)
	require.Equal(t, http.StatusOK, *result.Access.HTTPStatus)
}

// TestStructuredFailureLogWhitelistAndEgressLabel verifies the failure log
// emits only the bounded, whitelisted field set — no body, cookies, credentials,
// or arbitrary response headers — and that ConfiguredEgressLabel flows from the
// Fetcher config into both the configured_egress_label log field and the
// AccessOutcome.EgressID execution-history field.
func TestStructuredFailureLogWhitelistAndEgressLabel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Set-Cookie", "session=super-secret")
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(server.Close)

	fetcher := newFailurePhaseFetcher()
	fetcher.SetConfiguredEgressLabel("experiment-xyzfm-egress")
	result, entry := fetchFailure(t, fetcher, server.URL+"/feed.xml?token=secret")

	require.Equal(t, "experiment-xyzfm-egress", entry.Fields["configured_egress_label"])
	require.Equal(t, "experiment-xyzfm-egress", result.Access.EgressID)
	require.Equal(t, "127.0.0.1", entry.Fields["target_domain"])

	for _, key := range []string{
		"feed_url", "attempt", "retry_count", "target_domain", "error_category", "failure_phase",
		"configured_egress_label", "circuit_state", "response_time_ms",
		"response_bytes", "cache_status", "freshness", "http_status",
	} {
		_, ok := entry.Fields[key]
		require.True(t, ok, "expected whitelisted field %q in failure log", key)
	}
	require.Equal(t, 1, entry.Fields["attempt"])
	require.Equal(t, 0, entry.Fields["retry_count"])

	for _, forbidden := range []string{"body", "raw_content", "cookie", "cookies", "authorization", "token", "set_cookie"} {
		_, ok := entry.Fields[forbidden]
		require.False(t, ok, "forbidden field %q must not appear in failure log", forbidden)
	}

	feedURL, _ := entry.Fields["feed_url"].(string)
	// SanitizeFeedURL re-encodes the query, so the redaction marker may be
	// percent-encoded; assert the marker is present and the raw token is gone.
	require.Contains(t, feedURL, "REDACTED", "query credentials must be redacted in feed_url")
	require.NotContains(t, feedURL, "secret", "feed_url must not leak the raw token")
}
