package feed

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"magicpodcast/internal/logger"
)

// maxFeedRedirectHops is the hard upper bound on HTTP redirect following for a
// Feed fetch. A Feed may legitimately migrate across domains, so cross-host
// redirects are followed by default; only the scheme (HTTP(S) only) and the hop
// count are bounded. This is a fixed safety boundary, not a startup-configured
// knob, matching the first-version decision to restrict only scheme and hops.
const maxFeedRedirectHops = 5

// ErrFeedUnsafeRedirect signals that a redirect target used a non-HTTP(S)
// scheme or the redirect chain exceeded the bounded hop limit. It never carries
// the redirect target URL — only the sanitized reason — so logs and diagnostics
// cannot leak a sensitive redirect location.
var ErrFeedUnsafeRedirect = errors.New("feed redirect rejected (scheme or hop limit)")

// FeedConfig is the startup-loaded configuration for the Feed fetcher and the
// access coordinator. It is decoded from the "feed" section of the application
// config (YAML) with selected knobs overridable via MAGICPODCAST_FEED_*
// environment variables, then applied exactly once at process start through
// ConfigureSharedRuntime. There is intentionally NO hot reload: changing any
// value requires a restart, so the shared semaphore/circuit/in-flight state
// never has to migrate concurrently.
//
// Only bounded, behavior-shaping values live here. A full Feed URL, response
// body, cookie, credential, or arbitrary response header is never part of this
// configuration.
type FeedConfig struct {
	UserAgent         string                      `mapstructure:"user_agent"`
	Timeouts          FeedTimeouts                `mapstructure:"timeouts"`
	Headers           FeedHeaders                 `mapstructure:"headers"`
	Retry             FeedRetryConfig             `mapstructure:"retry"`
	Circuit           FeedCircuitConfig           `mapstructure:"circuit"`
	UserAgentRecovery UserAgentGateRecoveryConfig `mapstructure:"user_agent_recovery"`
	Snapshot          FeedSnapshotConfig          `mapstructure:"snapshot"`
	Diagnostics       FeedDiagnostics             `mapstructure:"diagnostics"`
	DomainPolicies    []FeedDomainPolicy          `mapstructure:"domain_policies"`
}

// FeedTimeouts are the layered HTTP timeouts. Each layer fails fast so a slow
// or trickling server cannot consume the whole request budget.
type FeedTimeouts struct {
	Connect time.Duration `mapstructure:"connect"`
	TLS     time.Duration `mapstructure:"tls"`
	Header  time.Duration `mapstructure:"header"`
	Overall time.Duration `mapstructure:"overall"`
}

// FeedHeaders are the honest, low-load request headers applied to every Feed
// request. Accept-Encoding is intentionally absent: Go's http.Transport
// transparently negotiates and decompresses gzip, and setting it would disable
// that behavior.
type FeedHeaders struct {
	Accept         string `mapstructure:"accept"`
	AcceptLanguage string `mapstructure:"accept_language"`
}

// FeedRetryConfig bounds the OUTER retry behavior layered above the coordinator
// (the unified retry budget landed in the retry-convergence ticket).
//
// Budget is the maximum number of retries AFTER the first attempt for one Feed
// within a workflow step (so total attempts = Budget + 1). A value of 0 means
// "unset": SharedRetryPolicy falls back to DefaultRetryBudget (3), preserving
// the previously validated MaxRetries=3 behavior. It is validated to [0,
// maxRetryBudget] so no configuration can produce an unbounded retry path; an
// operator who wants fewer retries sets an explicit budget in [1, maxRetryBudget].
//
// Jitter is the full-jitter exponential-backoff base. A zero value means
// "unset": SharedRetryPolicy falls back to DefaultRetryBase (2s). Every wait is
// capped at DefaultRetryMax (8s) and the global MaxRetryAfter (60s), and
// Retry-After (delta-seconds or HTTP-date) wins over the backoff for 429/5xx
// when the upstream provides it.
type FeedRetryConfig struct {
	Budget int           `mapstructure:"budget"`
	Jitter time.Duration `mapstructure:"jitter"`
}

// FeedCircuitConfig carries the global circuit-recovery defaults applied to
// every domain policy that does not override the value. Defaults preserve the
// already-validated behavior and never expand the domain circuit scope.
//
// ThresholdsPerCategory maps an ErrorCategory to a distinct-feed evidence
// threshold. An empty map (the default) keeps the existing circuit logic; a
// non-empty entry is an explicit operator opt-in. The coordinator applies the
// override per category while preserving the unconditional xyzfm 403 rule.
type FeedCircuitConfig struct {
	ThresholdsPerCategory          map[string]int `mapstructure:"thresholds_per_category"`
	DomainEvidenceMinDistinctFeeds int            `mapstructure:"domain_evidence_min_distinct_feeds"`
	HalfOpenMax                    int            `mapstructure:"half_open_max"`
	SuccessesToClose               int            `mapstructure:"successes_to_close"`
}

// FeedSnapshotConfig controls the last-good fallback store. Durable is a
// pointer so an unset feed section keeps the already-validated default
// (durable persistence when the feed_snapshots table exists); an explicit
// durable:false opts the process out of on-disk snapshots without changing the
// default for operators who omit the field.
type FeedSnapshotConfig struct {
	Durable *bool              `mapstructure:"durable"`
	Bounds  FeedSnapshotBounds `mapstructure:"bounds"`
}

// FeedSnapshotBounds caps snapshot entry count, per-response bytes, and total
// retained bytes. Values <= 0 fall back to the honest defaults.
type FeedSnapshotBounds struct {
	MaxEntries       int   `mapstructure:"max_entries"`
	MaxResponseBytes int   `mapstructure:"max_response_bytes"`
	MaxTotalBytes    int64 `mapstructure:"max_total_bytes"`
}

// FeedDiagnostics controls the protected admin diagnostics surface and the
// configured-egress observation tag. AdminEnabled is a pointer so an unset feed
// section keeps the default (endpoint enabled); an explicit admin_enabled:false
// de-registers the route without changing the default for operators who omit it.
type FeedDiagnostics struct {
	AdminEnabled          *bool  `mapstructure:"admin_enabled"`
	ConfiguredEgressLabel string `mapstructure:"configured_egress_label"`
}

// FeedDomainPolicy is the startup-configurable, domain-scoped load-shaping
// policy. It maps 1:1 onto DomainPolicy for the fields that are safe to tune at
// runtime. SoftRateEnabled / ImmediateCircuitOnAccessDenied are intentionally
// NOT exposed for xyzfm: #35/#36 require shared single-queue soft rate and ban
// hard-open on first 403, so those invariants are force-preserved in code.
type FeedDomainPolicy struct {
	Domain                         string        `mapstructure:"domain"`
	MaxConcurrency                 int           `mapstructure:"max_concurrency"`
	MinRefreshInterval             time.Duration `mapstructure:"min_refresh_interval"`
	MaxJitter                      time.Duration `mapstructure:"max_jitter"`
	CircuitCooldown                time.Duration `mapstructure:"circuit_cooldown"`
	RetryBackoffInitial            time.Duration `mapstructure:"retry_backoff_initial"`
	RetryBackoffMax                time.Duration `mapstructure:"retry_backoff_max"`
	HalfOpenMaxRequests            int           `mapstructure:"half_open_max_requests"`
	SuccessesToClose               int           `mapstructure:"successes_to_close"`
	DomainEvidenceMinDistinctFeeds int           `mapstructure:"domain_evidence_min_distinct_feeds"`
	EvidenceWindow                 time.Duration `mapstructure:"evidence_window"`
}

// maxRetryBudget is the hard ceiling on FeedRetryConfig.Budget. No
// configuration may exceed it, guaranteeing there is no infinite or runaway
// retry path even before the outer retry layer enforces the budget.
const maxRetryBudget = 5

// validErrorCategories is the set of ErrorCategory values accepted as keys in
// Circuit.ThresholdsPerCategory.
var validErrorCategories = map[ErrorCategory]struct{}{
	ErrorCategoryAccessDenied:       {},
	ErrorCategoryRateLimited:        {},
	ErrorCategoryServiceUnavailable: {},
	ErrorCategoryHTTP:               {},
	ErrorCategoryTimeout:            {},
	ErrorCategoryNetwork:            {},
	ErrorCategoryParse:              {},
}

// Validate reports whether the FeedConfig is internally consistent. It is
// called from ConfigureSharedRuntime; an error fails process startup so an
// invalid configuration can never produce surprising fetch behavior.
func (c FeedConfig) Validate() error {
	if c.Timeouts.Overall < 0 || c.Timeouts.Connect < 0 || c.Timeouts.TLS < 0 || c.Timeouts.Header < 0 {
		return errors.New("feed.timeouts must be non-negative")
	}
	if c.Timeouts.Header > 0 && c.Timeouts.Overall > 0 && c.Timeouts.Header > c.Timeouts.Overall {
		return fmt.Errorf("feed.timeouts.header (%s) cannot exceed feed.timeouts.overall (%s)", c.Timeouts.Header, c.Timeouts.Overall)
	}
	if c.Retry.Budget < 0 {
		return fmt.Errorf("feed.retry.budget (%d) must be >= 0", c.Retry.Budget)
	}
	if c.Retry.Budget > maxRetryBudget {
		return fmt.Errorf("feed.retry.budget (%d) exceeds the hard cap %d", c.Retry.Budget, maxRetryBudget)
	}
	if c.Retry.Jitter < 0 {
		return errors.New("feed.retry.jitter must be non-negative")
	}
	if err := c.UserAgentRecovery.Validate(); err != nil {
		return err
	}
	if c.Circuit.DomainEvidenceMinDistinctFeeds < 0 {
		return fmt.Errorf("feed.circuit.domain_evidence_min_distinct_feeds (%d) must be >= 0", c.Circuit.DomainEvidenceMinDistinctFeeds)
	}
	if c.Circuit.HalfOpenMax < 0 {
		return fmt.Errorf("feed.circuit.half_open_max (%d) must be >= 0", c.Circuit.HalfOpenMax)
	}
	if c.Circuit.SuccessesToClose < 0 {
		return fmt.Errorf("feed.circuit.successes_to_close (%d) must be >= 0", c.Circuit.SuccessesToClose)
	}
	for category, threshold := range c.Circuit.ThresholdsPerCategory {
		if _, ok := validErrorCategories[ErrorCategory(category)]; !ok {
			return fmt.Errorf("feed.circuit.thresholds_per_category has unknown category %q", category)
		}
		if threshold < 1 {
			return fmt.Errorf("feed.circuit.thresholds_per_category[%q] = %d must be >= 1", category, threshold)
		}
	}
	if c.Snapshot.Bounds.MaxEntries < 0 || c.Snapshot.Bounds.MaxResponseBytes < 0 || c.Snapshot.Bounds.MaxTotalBytes < 0 {
		return errors.New("feed.snapshot.bounds must be non-negative")
	}
	for i, policy := range c.DomainPolicies {
		if strings.TrimSpace(policy.Domain) == "" {
			return fmt.Errorf("feed.domain_policies[%d].domain cannot be empty", i)
		}
		if policy.MaxConcurrency < 0 {
			return fmt.Errorf("feed.domain_policies[%d].max_concurrency must be >= 0", i)
		}
		if policy.MinRefreshInterval < 0 || policy.MaxJitter < 0 || policy.CircuitCooldown < 0 ||
			policy.RetryBackoffInitial < 0 || policy.RetryBackoffMax < 0 || policy.EvidenceWindow < 0 {
			return fmt.Errorf("feed.domain_policies[%d] (%s) has a negative duration", i, policy.Domain)
		}
		if policy.HalfOpenMaxRequests < 0 || policy.SuccessesToClose < 0 || policy.DomainEvidenceMinDistinctFeeds < 0 {
			return fmt.Errorf("feed.domain_policies[%d] (%s) has a negative circuit field", i, policy.Domain)
		}
	}
	return nil
}

// FeedDiagnosticsState is the resolved view of FeedDiagnostics used by the
// router and metrics. AdminEnabled is a plain bool (default true) and
// ConfiguredEgressLabel defaults to "direct".
type FeedDiagnosticsState struct {
	AdminEnabled          bool
	ConfiguredEgressLabel string
}

// feedRuntime is the bridge between the startup-loaded FeedConfig and the
// shared Coordinator / Fetcher HTTP behavior. ConfigureSharedRuntime is the
// only writer and runs once before any Feed fetch; readers consult the helpers
// below. Until configured, every value reflects the honest defaults so the
// process behaves exactly as before even when no feed section is present.
type feedRuntime struct {
	mu                sync.RWMutex
	configured        bool
	http              FeedHTTPConfig
	snapshotDurable   bool
	snapshotBounds    FeedSnapshotBounds
	retry             FeedRetryConfig
	userAgentRecovery UserAgentGateRecoveryConfig
	adminEnabled      bool
	egressLabel       string
}

var sharedFeedRuntime = defaultFeedRuntime()

func defaultFeedRuntime() *feedRuntime {
	httpConfig := DefaultFeedHTTPConfig(defaultFeedOverallTimeout)
	return &feedRuntime{
		configured:        false,
		http:              httpConfig,
		snapshotDurable:   true,
		snapshotBounds:    FeedSnapshotBounds{},
		retry:             FeedRetryConfig{},
		userAgentRecovery: DefaultUserAgentGateRecoveryConfig(),
		adminEnabled:      true,
		egressLabel:       EgressDirect,
	}
}

// ConfigureSharedRuntime validates the FeedConfig and applies it to the
// process-wide Coordinator, metrics registry, and the shared HTTP configuration
// used by every Fetcher constructed afterwards. It must run exactly once, before
// any Feed fetch. An error fails startup so an invalid configuration can never
// reach a live fetch.
//
// The application's xyzfm soft-rate invariant is preserved unconditionally:
// feed.xyzfm.space always keeps SoftRateEnabled=true and never force-opens the
// domain circuit on a single 403 (#35/#36).
func ConfigureSharedRuntime(config FeedConfig) error {
	if err := config.Validate(); err != nil {
		return err
	}
	userAgentRecovery := config.UserAgentRecovery.withDefaults()

	httpConfig := FeedHTTPConfig{
		UserAgent:             defaultFeedUserAgent,
		Accept:                defaultFeedAccept,
		ConnectTimeout:        defaultFeedConnectTimeout,
		TLSHandshakeTimeout:   defaultFeedTLSHandshakeTimeout,
		ResponseHeaderTimeout: defaultFeedResponseHeaderTimeout,
		OverallTimeout:        defaultFeedOverallTimeout,
		ConfiguredEgressLabel: EgressDirect,
	}
	if config.UserAgent != "" {
		httpConfig.UserAgent = config.UserAgent
	}
	if config.Headers.Accept != "" {
		httpConfig.Accept = config.Headers.Accept
	}
	httpConfig.AcceptLanguage = config.Headers.AcceptLanguage
	if config.Timeouts.Connect > 0 {
		httpConfig.ConnectTimeout = config.Timeouts.Connect
	}
	if config.Timeouts.TLS > 0 {
		httpConfig.TLSHandshakeTimeout = config.Timeouts.TLS
	}
	if config.Timeouts.Header > 0 {
		httpConfig.ResponseHeaderTimeout = config.Timeouts.Header
	}
	if config.Timeouts.Overall > 0 {
		httpConfig.OverallTimeout = config.Timeouts.Overall
	}
	egressLabel := strings.TrimSpace(config.Diagnostics.ConfiguredEgressLabel)
	if egressLabel == "" {
		egressLabel = EgressDirect
	}
	httpConfig.ConfiguredEgressLabel = egressLabel

	coordinator := SharedCoordinator()
	coordinator.SetDomainPolicies(buildDomainPolicies(config.DomainPolicies))
	thresholds := make(map[ErrorCategory]int, len(config.Circuit.ThresholdsPerCategory))
	for category, threshold := range config.Circuit.ThresholdsPerCategory {
		thresholds[ErrorCategory(category)] = threshold
	}
	coordinator.SetCircuitDefaults(CircuitDefaults{
		HalfOpenMaxRequests:            resolvePositive(config.Circuit.HalfOpenMax, defaultHalfOpenMaxRequests),
		SuccessesToClose:               resolvePositive(config.Circuit.SuccessesToClose, defaultSuccessesToClose),
		DomainEvidenceMinDistinctFeeds: resolvePositive(config.Circuit.DomainEvidenceMinDistinctFeeds, defaultDomainEvidenceMinDistinctFeeds),
		EvidenceWindow:                 defaultEvidenceWindow,
		ThresholdsPerCategory:          thresholds,
	})
	SharedFeedMetrics().SetConfiguredEgressLabel(egressLabel)

	snapshotDurable := true
	if config.Snapshot.Durable != nil {
		snapshotDurable = *config.Snapshot.Durable
	}
	snapshotBounds := config.Snapshot.Bounds

	adminEnabled := true
	if config.Diagnostics.AdminEnabled != nil {
		adminEnabled = *config.Diagnostics.AdminEnabled
	}

	rt := sharedFeedRuntime
	rt.mu.Lock()
	rt.http = httpConfig
	rt.snapshotDurable = snapshotDurable
	rt.snapshotBounds = snapshotBounds
	rt.retry = config.Retry
	rt.userAgentRecovery = userAgentRecovery
	rt.adminEnabled = adminEnabled
	rt.egressLabel = egressLabel
	rt.configured = true
	rt.mu.Unlock()
	return nil
}

func resolvePositive(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

// buildDomainPolicies merges operator-supplied policies onto the default
// policy set. The default always includes feed.xyzfm.space with soft rate
// enabled and ImmediateCircuitOnAccessDenied=false; those fields are
// force-preserved for xyzfm so no configuration can re-introduce first-403
// whole-domain hard-open (#35/#36).
func buildDomainPolicies(configured []FeedDomainPolicy) map[string]DomainPolicy {
	policies := DefaultCoordinatorConfig().DomainPolicies
	for _, policy := range configured {
		domain := strings.ToLower(strings.TrimSpace(policy.Domain))
		if domain == "" {
			continue
		}
		base := policies[domain]
		base.MaxConcurrency = policy.MaxConcurrency
		base.MinRefreshInterval = policy.MinRefreshInterval
		base.MaxJitter = policy.MaxJitter
		base.CircuitCooldown = policy.CircuitCooldown
		base.RetryBackoffInitial = policy.RetryBackoffInitial
		base.RetryBackoffMax = policy.RetryBackoffMax
		if policy.HalfOpenMaxRequests > 0 {
			base.HalfOpenMaxRequests = policy.HalfOpenMaxRequests
		}
		if policy.SuccessesToClose > 0 {
			base.SuccessesToClose = policy.SuccessesToClose
		}
		if policy.DomainEvidenceMinDistinctFeeds > 0 {
			base.DomainEvidenceMinDistinctFeeds = policy.DomainEvidenceMinDistinctFeeds
		}
		if policy.EvidenceWindow > 0 {
			base.EvidenceWindow = policy.EvidenceWindow
		}
		if domain == XiaoyuzhouFeedDomain {
			base.SoftRateEnabled = true
			base.ImmediateCircuitOnAccessDenied = false
			// Shared single queue is part of the xyzfm contract.
			base.MaxConcurrency = 1
		}
		policies[domain] = base
	}
	return policies
}

// SharedHTTPConfig returns the startup-configured HTTP behavior for new
// Fetchers, falling back to honest defaults when no feed section was loaded.
// NewFetcherWithCoordinator consults it so all workflow fetches honor the
// configured User-Agent, layered timeouts, and honest headers.
func SharedHTTPConfig() FeedHTTPConfig {
	sharedFeedRuntime.mu.RLock()
	defer sharedFeedRuntime.mu.RUnlock()
	return sharedFeedRuntime.http
}

// SharedSnapshotConfig returns whether durable last-good persistence is enabled
// and the configured capacity bounds. Defaults keep the already-validated
// behavior (durable when the table exists; honest capacity bounds).
func SharedSnapshotConfig() (durable bool, bounds FeedSnapshotBounds) {
	sharedFeedRuntime.mu.RLock()
	defer sharedFeedRuntime.mu.RUnlock()
	return sharedFeedRuntime.snapshotDurable, sharedFeedRuntime.snapshotBounds
}

// SharedDiagnosticsConfig returns the startup-configured diagnostics surface
// controls (admin endpoint enabled, configured-egress tag).
func SharedDiagnosticsConfig() FeedDiagnosticsState {
	sharedFeedRuntime.mu.RLock()
	defer sharedFeedRuntime.mu.RUnlock()
	return FeedDiagnosticsState{AdminEnabled: sharedFeedRuntime.adminEnabled, ConfiguredEgressLabel: sharedFeedRuntime.egressLabel}
}

// SharedRetryConfig returns the startup-configured outer retry budget and
// jitter. It is consumed by the unified retry layer above the coordinator.
func SharedRetryConfig() FeedRetryConfig {
	sharedFeedRuntime.mu.RLock()
	defer sharedFeedRuntime.mu.RUnlock()
	return sharedFeedRuntime.retry
}

// SharedUserAgentGateRecoveryConfig returns the startup-configured durable
// User-Agent ACL recovery policy used when the SQLite gate store is built.
func SharedUserAgentGateRecoveryConfig() UserAgentGateRecoveryConfig {
	sharedFeedRuntime.mu.RLock()
	defer sharedFeedRuntime.mu.RUnlock()
	return sharedFeedRuntime.userAgentRecovery
}

// LastGoodStoreConfigFromBounds translates the configured snapshot bounds into
// the LastGoodStoreConfig accepted by the snapshot stores, applying honest
// defaults for any unset bound.
func LastGoodStoreConfigFromBounds(bounds FeedSnapshotBounds) LastGoodStoreConfig {
	config := LastGoodStoreConfig{}
	if bounds.MaxEntries > 0 {
		config.MaxEntries = bounds.MaxEntries
	}
	if bounds.MaxResponseBytes > 0 {
		config.MaxResponseBytes = bounds.MaxResponseBytes
	}
	if bounds.MaxTotalBytes > 0 {
		config.MaxTotalBytes = bounds.MaxTotalBytes
	}
	return config
}

// feedRedirectPolicy is the CheckRedirect closure shared by every Fetcher
// client. It bounds redirects to maxFeedRedirectHops, rejects any non-HTTP(S)
// target scheme, and logs each hop's SANITIZED target so the cross-domain Feed
// migration that RSS legitimately relies on stays allowed without leaking a
// redirect location carrying credentials.
func feedRedirectPolicy(req *http.Request, via []*http.Request) error {
	// via holds every request already issued in this chain (original +
	// intermediates). We allow at most maxFeedRedirectHops redirects, so the
	// (maxFeedRedirectHops+1)th redirect is rejected.
	if len(via) > maxFeedRedirectHops {
		return fmt.Errorf("%w: stopped after %d redirects", ErrFeedUnsafeRedirect, maxFeedRedirectHops)
	}
	if req == nil || req.URL == nil {
		return fmt.Errorf("%w: missing redirect target", ErrFeedUnsafeRedirect)
	}
	scheme := strings.ToLower(req.URL.Scheme)
	if scheme != "http" && scheme != "https" {
		// Never follow a redirect to a non-HTTP(S) scheme (file:, gopher:,
		// etc.). The reason is logged without the target URL so a redirect
		// location cannot leak credentials or internal addresses.
		return fmt.Errorf("%w: non-http(s) scheme %q", ErrFeedUnsafeRedirect, scheme)
	}
	logger.Debugf("feed redirect %d/%d -> %s", len(via), maxFeedRedirectHops, SanitizeFeedURL(req.URL.String()))
	return nil
}
