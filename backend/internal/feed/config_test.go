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

// resetSharedRuntimeForTest restores the process-wide runtime, coordinator
// policies, circuit defaults, and metrics egress tag to the honest defaults.
// ConfigureSharedRuntime mutates singletons, so every config/redirect test
// registers this cleanup to stay isolated from other feed tests.
func resetSharedRuntimeForTest(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		sharedFeedRuntime = defaultFeedRuntime()
		coord := SharedCoordinator()
		coord.SetDomainPolicies(DefaultCoordinatorConfig().DomainPolicies)
		coord.SetCircuitDefaults(CircuitDefaults{
			HalfOpenMaxRequests:            defaultHalfOpenMaxRequests,
			SuccessesToClose:               defaultSuccessesToClose,
			DomainEvidenceMinDistinctFeeds: defaultDomainEvidenceMinDistinctFeeds,
			EvidenceWindow:                 defaultEvidenceWindow,
		})
		SharedFeedMetrics().SetConfiguredEgressLabel(EgressDirect)
	})
}

// feedBody is a minimal parseable RSS body used by the redirect servers.
const feedBody = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel><title>Redirect Target</title>
<item><title>ep1</title><guid>g1</guid></item></channel></rss>`

// redirectChainServer returns an httptest server that issues a redirect to the
// next URL hop, then a final server that returns feedBody. The chain length is
// the number of 302 redirects before the final 200. requestCount counts every
// request seen across the chain.
func redirectChainServer(t *testing.T, hops int) (entryURL, finalURL string, requestCount *int32) {
	t.Helper()
	var count int32
	final := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&count, 1)
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(feedBody))
	}))
	t.Cleanup(final.Close)

	current := final.URL
	servers := []*httptest.Server{final}
	for i := 0; i < hops; i++ {
		target := current
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&count, 1)
			http.Redirect(w, r, target, http.StatusFound)
		}))
		t.Cleanup(srv.Close)
		servers = append(servers, srv)
		current = srv.URL
	}
	return current, final.URL, &count
}

func TestRedirectFollowsHTTPSCrossDomainMigration(t *testing.T) {
	resetSharedRuntimeForTest(t)
	// A legitimate cross-domain Feed migration: the entry redirects to a
	// different host (the httptest final server). The default policy MUST allow
	// it because only scheme and hop count are bounded, never the host.
	entry, final, _ := redirectChainServer(t, 1)
	require.NotEqual(t, entry, final, "test must actually cross hosts")

	fetcher := NewFetcherWithHTTPConfig(DefaultFeedHTTPConfig(5*time.Second), nil)
	result, err := fetcher.FetchFeedWithContext(context.Background(), entry)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "Redirect Target", result.Title)
}

func TestRedirectRejectsNonHTTPSScheme(t *testing.T) {
	resetSharedRuntimeForTest(t)
	// The upstream returns a redirect to a file:// location. The policy must
	// reject following it and the error must carry no target URL.
	var seenLocation string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenLocation = "/etc/passwd"
		w.Header().Set("Location", "file:///etc/passwd")
		w.WriteHeader(http.StatusFound)
	}))
	t.Cleanup(server.Close)

	fetcher := NewFetcherWithHTTPConfig(DefaultFeedHTTPConfig(5*time.Second), nil)
	_, err := fetcher.FetchFeedWithContext(context.Background(), server.URL)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrFeedUnsafeRedirect)
	// The redirect target must not leak into the surfaced error.
	require.NotContains(t, err.Error(), seenLocation)
	require.NotContains(t, err.Error(), "etc/passwd")
}

func TestRedirectEnforcesFiveHopLimit(t *testing.T) {
	resetSharedRuntimeForTest(t)
	// Within the 5-redirect bound: a 5-hop chain must succeed.
	entry, _, _ := redirectChainServer(t, 5)
	fetcher := NewFetcherWithHTTPConfig(DefaultFeedHTTPConfig(10*time.Second), nil)
	result, err := fetcher.FetchFeedWithContext(context.Background(), entry)
	require.NoError(t, err)
	require.NotNil(t, result, "5-redirect chain must be followed")

	// The 6th redirect exceeds the bound and must be rejected.
	entry6, _, _ := redirectChainServer(t, 6)
	_, err = fetcher.FetchFeedWithContext(context.Background(), entry6)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrFeedUnsafeRedirect)
}

func TestRedirectTargetSanitization(t *testing.T) {
	resetSharedRuntimeForTest(t)
	// SanitizeFeedURL is the redaction boundary used by the redirect policy's
	// debug log. Assert it strips userinfo and redacts sensitive query values so
	// a redirect target carrying a token cannot leak.
	raw := "https://user:secret@feed.example.com/path?token=abc&keep=1"
	sanitized := SanitizeFeedURL(raw)
	// Userinfo must be stripped entirely.
	require.NotContains(t, sanitized, "secret")
	require.NotContains(t, sanitized, "user:secret")
	// The sensitive token VALUE must not leak (the key is redacted; url.Encode
	// percent-encodes the brackets, so check the inner marker text).
	require.NotContains(t, sanitized, "token=abc")
	require.Contains(t, sanitized, "REDACTED")
	// Non-sensitive query values and the host survive intact.
	require.Contains(t, sanitized, "keep=1")
	require.Contains(t, sanitized, "feed.example.com")
}

func TestConfigureSharedRuntimeAppliesHTTPConfig(t *testing.T) {
	resetSharedRuntimeForTest(t)
	cfg := FeedConfig{
		UserAgent: "MagicPodcast-Test/2.0 (+https://example.com)",
		Timeouts:  FeedTimeouts{Overall: 42 * time.Second, Connect: 7 * time.Second},
		Headers:   FeedHeaders{Accept: "application/rss+xml", AcceptLanguage: "zh-CN"},
	}
	require.NoError(t, ConfigureSharedRuntime(cfg))

	httpCfg := SharedHTTPConfig()
	require.Equal(t, "MagicPodcast-Test/2.0 (+https://example.com)", httpCfg.UserAgent)
	require.Equal(t, 42*time.Second, httpCfg.OverallTimeout)
	require.Equal(t, 7*time.Second, httpCfg.ConnectTimeout)
	require.Equal(t, "application/rss+xml", httpCfg.Accept)
	require.Equal(t, "zh-CN", httpCfg.AcceptLanguage)

	// NewFetcherWithCoordinator must honor the configured HTTP behavior.
	fetcher := NewFetcherWithCoordinator(time.Second, SharedCoordinator())
	require.Equal(t, "MagicPodcast-Test/2.0 (+https://example.com)", fetcher.userAgent())
	require.Equal(t, "zh-CN", fetcher.acceptLanguage())
}

func TestConfigureSharedRuntimePreservesDefaultsWhenEmpty(t *testing.T) {
	resetSharedRuntimeForTest(t)
	require.NoError(t, ConfigureSharedRuntime(FeedConfig{}))
	httpCfg := SharedHTTPConfig()
	require.Equal(t, defaultFeedUserAgent, httpCfg.UserAgent)
	require.Equal(t, defaultFeedAccept, httpCfg.Accept)
	require.Equal(t, defaultFeedOverallTimeout, httpCfg.OverallTimeout)
	require.Equal(t, EgressDirect, httpCfg.ConfiguredEgressLabel)

	// The xyzfm first-403-immediate-open policy must remain in place.
	policy := SharedCoordinator().policyFor(XiaoyuzhouFeedDomain)
	require.True(t, policy.ImmediateCircuitOnAccessDenied, "xyzfm immediate-circuit rule must survive empty config")
}

func TestConfigureSharedRuntimePreservesXiaoyuzhouImmediateCircuitInvariant(t *testing.T) {
	resetSharedRuntimeForTest(t)
	// An operator might try to relax xyzfm via domain_policies. The xyzfm
	// first-403-immediate-open safety rule is an invariant that must survive.
	cfg := FeedConfig{
		DomainPolicies: []FeedDomainPolicy{
			{
				Domain:             XiaoyuzhouFeedDomain,
				MaxConcurrency:     4,
				CircuitCooldown:    time.Minute,
			},
			{
				Domain:             "feed.example.com",
				MaxConcurrency:     2,
				MinRefreshInterval: 2 * time.Second,
			},
		},
	}
	require.NoError(t, ConfigureSharedRuntime(cfg))
	policy := SharedCoordinator().policyFor(XiaoyuzhouFeedDomain)
	require.True(t, policy.ImmediateCircuitOnAccessDenied, "xyzfm immediate-circuit invariant must be preserved")
	require.Equal(t, 4, policy.MaxConcurrency, "non-safety fields may be tuned")

	// A custom domain policy is added without weakening xyzfm or defaults.
	custom := SharedCoordinator().policyFor("feed.example.com")
	require.Equal(t, 2, custom.MaxConcurrency)
	require.Equal(t, 2*time.Second, custom.MinRefreshInterval)
}

func TestConfigureSharedRuntimeAppliesCircuitDefaults(t *testing.T) {
	resetSharedRuntimeForTest(t)
	cfg := FeedConfig{
		Circuit: FeedCircuitConfig{
			HalfOpenMax:                 3,
			SuccessesToClose:            5,
			DomainEvidenceMinDistinctFeeds: 2,
		},
	}
	require.NoError(t, ConfigureSharedRuntime(cfg))
	coord := SharedCoordinator()
	coord.mu.Lock()
	defer coord.mu.Unlock()
	require.Equal(t, 3, coord.circuitDefaults.HalfOpenMaxRequests)
	require.Equal(t, 5, coord.circuitDefaults.SuccessesToClose)
	require.Equal(t, 2, coord.circuitDefaults.DomainEvidenceMinDistinctFeeds)
}

func TestConfigureSharedRuntimeEgressLabelFlowsToMetricsAndDiagnostics(t *testing.T) {
	resetSharedRuntimeForTest(t)
	cfg := FeedConfig{
		Diagnostics: FeedDiagnostics{ConfiguredEgressLabel: "cloudflare-tunnel"},
	}
	require.NoError(t, ConfigureSharedRuntime(cfg))
	require.Equal(t, "cloudflare-tunnel", SharedHTTPConfig().ConfiguredEgressLabel)
	require.Equal(t, "cloudflare-tunnel", SharedDiagnosticsConfig().ConfiguredEgressLabel)
	require.Equal(t, "cloudflare-tunnel", SharedFeedMetrics().Snapshot().ConfiguredEgressLabel)
}

func TestConfigureSharedRuntimeAdminEnabledDefaultsTrue(t *testing.T) {
	resetSharedRuntimeForTest(t)
	// Unset (nil) AdminEnabled keeps the default: route stays registered.
	require.NoError(t, ConfigureSharedRuntime(FeedConfig{}))
	require.True(t, SharedDiagnosticsConfig().AdminEnabled)

	// Explicit opt-out disables the route without changing other defaults.
	disabled := false
	require.NoError(t, ConfigureSharedRuntime(FeedConfig{Diagnostics: FeedDiagnostics{AdminEnabled: &disabled}}))
	require.False(t, SharedDiagnosticsConfig().AdminEnabled)
}

func TestConfigureSharedRuntimeRejectsInvalidConfig(t *testing.T) {
	resetSharedRuntimeForTest(t)
	cases := []struct {
		name string
		cfg  FeedConfig
	}{
		{"budget over cap", FeedConfig{Retry: FeedRetryConfig{Budget: maxRetryBudget + 1}}},
		{"negative budget", FeedConfig{Retry: FeedRetryConfig{Budget: -1}}},
		{"negative timeout", FeedConfig{Timeouts: FeedTimeouts{Overall: -time.Second}}},
		{"header over overall", FeedConfig{Timeouts: FeedTimeouts{Header: 20 * time.Second, Overall: 10 * time.Second}}},
		{"empty domain policy", FeedConfig{DomainPolicies: []FeedDomainPolicy{{Domain: " "}}}},
		{"negative max concurrency", FeedConfig{DomainPolicies: []FeedDomainPolicy{{Domain: "a.com", MaxConcurrency: -1}}}},
		{"unknown category threshold", FeedConfig{Circuit: FeedCircuitConfig{ThresholdsPerCategory: map[string]int{"bogus": 1}}}},
		{"zero category threshold", FeedConfig{Circuit: FeedCircuitConfig{ThresholdsPerCategory: map[string]int{"timeout": 0}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ConfigureSharedRuntime(tc.cfg)
			require.Error(t, err, "invalid config must fail startup, not silently apply")
		})
	}
}

func TestValidateAcceptsValidCategoryThresholds(t *testing.T) {
	resetSharedRuntimeForTest(t)
	cfg := FeedConfig{
		Circuit: FeedCircuitConfig{ThresholdsPerCategory: map[string]int{
			string(ErrorCategoryTimeout):    2,
			string(ErrorCategoryRateLimited): 3,
		}},
	}
	require.NoError(t, ConfigureSharedRuntime(cfg))
}

func TestLastGoodStoreConfigFromBoundsAppliesDefaults(t *testing.T) {
	// Unset bounds fall through to the honest defaults inside the store.
	cfg := LastGoodStoreConfigFromBounds(FeedSnapshotBounds{})
	store := NewMemorySnapshotStore(cfg)
	require.Equal(t, defaultLastGoodMaxEntries, store.maxEntries)
	require.Equal(t, defaultLastGoodMaxResponseBytes, store.maxResponseBytes)
	require.Equal(t, int64(defaultLastGoodMaxTotalBytes), store.maxTotalBytes)

	cfg2 := LastGoodStoreConfigFromBounds(FeedSnapshotBounds{MaxEntries: 17, MaxResponseBytes: 1024, MaxTotalBytes: 4096})
	store2 := NewMemorySnapshotStore(cfg2)
	require.Equal(t, 17, store2.maxEntries)
	require.Equal(t, 1024, store2.maxResponseBytes)
	require.Equal(t, int64(4096), store2.maxTotalBytes)
}

func TestRedirectErrorClassifiedAsInvalidRequest(t *testing.T) {
	resetSharedRuntimeForTest(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "gopher://feed.example.com/x")
		w.WriteHeader(http.StatusFound)
	}))
	t.Cleanup(server.Close)

	fetcher := NewFetcherWithHTTPConfig(DefaultFeedHTTPConfig(5*time.Second), nil)
	result, err := fetcher.FetchFeedWithContextDetailed(context.Background(), server.URL)
	require.Error(t, err)
	require.NotNil(t, result)
	require.Equal(t, ErrorCategoryInvalidRequest, result.Access.ErrorCategory, "redirect rejection must not be classified as a retryable network/server error")
}

func TestEgressLabelIsConfigurationTagNotProof(t *testing.T) {
	// ConfiguredEgressLabel is a configuration tag only. The default is "direct"
	// and the labels stay bounded strings; this test pins the documented
	// invariant that the tag carries no claim about the real public egress.
	resetSharedRuntimeForTest(t)
	require.Equal(t, EgressDirect, SharedHTTPConfig().ConfiguredEgressLabel)
	require.NoError(t, ConfigureSharedRuntime(FeedConfig{Diagnostics: FeedDiagnostics{ConfiguredEgressLabel: ""}}))
	require.Equal(t, EgressDirect, SharedHTTPConfig().ConfiguredEgressLabel, "empty label must fall back to direct, never become proof of a specific egress")
}
