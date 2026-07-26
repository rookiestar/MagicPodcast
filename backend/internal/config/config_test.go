package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/viper"
)

func TestLoadAppliesRuntimeEnvOverrides(t *testing.T) {
	t.Cleanup(func() {
		cfg = nil
		viper.Reset()
	})
	viper.Reset()

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	t.Setenv("MAGICPODCAST_SERVER_MODE", "release")
	t.Setenv("MAGICPODCAST_SERVER_HOST", "127.0.0.1")
	t.Setenv("MAGICPODCAST_SERVER_PORT", "18080")
	t.Setenv("MAGICPODCAST_DATABASE_DEBUG", "false")
	t.Setenv("MAGICPODCAST_DATABASE_PATH", "/tmp/magicpodcast-test.db")
	t.Setenv("MAGICPODCAST_DATABASE_BUSY_TIMEOUT_MS", "2500")

	writeTestConfig(t, configPath, `
server:
  host: 127.0.0.1
  port: 8080
  mode: debug
database:
  path: ./data/test.db
  debug: true
xyz_api:
  url: http://localhost:8081
`)

	loaded, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if loaded.Server.Mode != "release" {
		t.Fatalf("Server.Mode = %q, want release", loaded.Server.Mode)
	}
	if loaded.Server.Port != 18080 {
		t.Fatalf("Server.Port = %d, want 18080", loaded.Server.Port)
	}
	if loaded.Server.Host != "127.0.0.1" {
		t.Fatalf("Server.Host = %q, want 127.0.0.1", loaded.Server.Host)
	}
	if loaded.Database.Path != "/tmp/magicpodcast-test.db" {
		t.Fatalf("Database.Path = %q, want /tmp/magicpodcast-test.db", loaded.Database.Path)
	}
	if loaded.Database.BusyTimeoutMS != 2500 {
		t.Fatalf("Database.BusyTimeoutMS = %d, want 2500", loaded.Database.BusyTimeoutMS)
	}
	if loaded.Database.Debug {
		t.Fatal("Database.Debug = true, want false")
	}
}

func TestLoadDefaultsServerHostToLoopback(t *testing.T) {
	t.Cleanup(func() {
		cfg = nil
		viper.Reset()
	})
	viper.Reset()

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	writeTestConfig(t, configPath, `
server:
  port: 8080
  mode: release
database:
  path: ./data/test.db
xyz_api:
  url: http://localhost:8081
`)

	loaded, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.Server.Host != "127.0.0.1" {
		t.Fatalf("Server.Host = %q, want 127.0.0.1", loaded.Server.Host)
	}
}

func TestValidateRejectsNonLoopbackServerHost(t *testing.T) {
	base := Config{
		Server:   ServerConfig{Port: 8080, Mode: "release"},
		Database: DatabaseConfig{Path: "./data/test.db"},
		XYZAPI:   XYZAPIConfig{URL: "http://127.0.0.1:8081"},
	}

	for _, host := range []string{"", "localhost", "0.0.0.0", "192.168.1.10", "example.com"} {
		t.Run(host, func(t *testing.T) {
			config := base
			config.Server.Host = host
			if err := config.Validate(); err == nil {
				t.Fatalf("Validate() succeeded for non-loopback host %q", host)
			}
		})
	}

	for _, host := range []string{"127.0.0.1", "::1"} {
		t.Run(host, func(t *testing.T) {
			config := base
			config.Server.Host = host
			if err := config.Validate(); err != nil {
				t.Fatalf("Validate() error for loopback host %q: %v", host, err)
			}
		})
	}
}

func writeTestConfig(t *testing.T, path string, contents string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}
}

// minimalBaseConfigYAML is the smallest valid YAML carrying the non-feed
// sections Load + Validate require, so a feed-focused test can append a feed
// section without re-asserting unrelated defaults.
const minimalBaseConfigYAML = `
server:
  host: 127.0.0.1
  port: 8080
  mode: release
database:
  path: ./data/test.db
xyz_api:
  url: http://127.0.0.1:8081
`

func writeFeedTestConfig(t *testing.T, feedSection string) string {
	t.Helper()
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	writeTestConfig(t, configPath, minimalBaseConfigYAML+feedSection)
	return configPath
}

func TestLoadFeedYAMLPopulatesFeedConfig(t *testing.T) {
	t.Cleanup(func() {
		cfg = nil
		viper.Reset()
	})
	viper.Reset()

	configPath := writeFeedTestConfig(t, `
feed:
  user_agent: "MagicPodcast-YAML/1.0"
  timeouts:
    connect: 4s
    tls: 5s
    header: 9s
    overall: 33s
  headers:
    accept: "application/atom+xml"
    accept_language: "en-US"
  retry:
    budget: 2
    jitter: 250ms
  circuit:
    half_open_max: 3
    successes_to_close: 4
    domain_evidence_min_distinct_feeds: 2
  user_agent_recovery:
    initial_cooldown: 7h
    probe_failure_cooldown: 30h
    required_successes: 4
  snapshot:
    durable: false
    bounds:
      max_entries: 64
      max_response_bytes: 524288
      max_total_bytes: 8388608
  diagnostics:
    admin_enabled: false
    configured_egress_label: "cloudflare-tunnel"
`)

	loaded, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	f := loaded.Feed
	if f.UserAgent != "MagicPodcast-YAML/1.0" {
		t.Fatalf("UserAgent = %q", f.UserAgent)
	}
	if f.Timeouts.Connect != 4*time.Second || f.Timeouts.Overall != 33*time.Second {
		t.Fatalf("Timeouts = %+v", f.Timeouts)
	}
	if f.Headers.AcceptLanguage != "en-US" {
		t.Fatalf("AcceptLanguage = %q", f.Headers.AcceptLanguage)
	}
	if f.Retry.Budget != 2 || f.Retry.Jitter != 250*time.Millisecond {
		t.Fatalf("Retry = %+v", f.Retry)
	}
	if f.Circuit.HalfOpenMax != 3 || f.Circuit.SuccessesToClose != 4 || f.Circuit.DomainEvidenceMinDistinctFeeds != 2 {
		t.Fatalf("Circuit = %+v", f.Circuit)
	}
	if f.UserAgentRecovery.InitialCooldown != 7*time.Hour || f.UserAgentRecovery.ProbeFailureCooldown != 30*time.Hour || f.UserAgentRecovery.RequiredSuccesses != 4 {
		t.Fatalf("UserAgentRecovery = %+v", f.UserAgentRecovery)
	}
	if f.Snapshot.Durable == nil || *f.Snapshot.Durable != false {
		t.Fatalf("Snapshot.Durable = %v", f.Snapshot.Durable)
	}
	if f.Snapshot.Bounds.MaxEntries != 64 || f.Snapshot.Bounds.MaxResponseBytes != 524288 || f.Snapshot.Bounds.MaxTotalBytes != 8388608 {
		t.Fatalf("Snapshot.Bounds = %+v", f.Snapshot.Bounds)
	}
	if f.Diagnostics.AdminEnabled == nil || *f.Diagnostics.AdminEnabled != false {
		t.Fatalf("Diagnostics.AdminEnabled = %v", f.Diagnostics.AdminEnabled)
	}
	if f.Diagnostics.ConfiguredEgressLabel != "cloudflare-tunnel" {
		t.Fatalf("ConfiguredEgressLabel = %q", f.Diagnostics.ConfiguredEgressLabel)
	}
}

func TestLoadFeedDefaultsWhenSectionAbsent(t *testing.T) {
	t.Cleanup(func() {
		cfg = nil
		viper.Reset()
	})
	viper.Reset()

	configPath := writeFeedTestConfig(t, "")
	loaded, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	// No feed section: every field stays at its zero/honest default. The
	// *bool fields stay nil so ConfigureSharedRuntime applies the documented
	// defaults (durable=true, admin_enabled=true) rather than an explicit false.
	if loaded.Feed.UserAgent != "" {
		t.Fatalf("UserAgent = %q, want empty default", loaded.Feed.UserAgent)
	}
	if loaded.Feed.Snapshot.Durable != nil {
		t.Fatalf("Snapshot.Durable = %v, want nil (unset)", loaded.Feed.Snapshot.Durable)
	}
	if loaded.Feed.Diagnostics.AdminEnabled != nil {
		t.Fatalf("Diagnostics.AdminEnabled = %v, want nil (unset)", loaded.Feed.Diagnostics.AdminEnabled)
	}
	if loaded.Feed.Retry.Budget != 0 {
		t.Fatalf("Retry.Budget = %d, want 0", loaded.Feed.Retry.Budget)
	}
}

func TestLoadFeedENVOverridesFeedConfig(t *testing.T) {
	t.Cleanup(func() {
		cfg = nil
		viper.Reset()
	})
	viper.Reset()

	t.Setenv("MAGICPODCAST_FEED_USER_AGENT", "MagicPodcast-ENV/2.0")
	t.Setenv("MAGICPODCAST_FEED_DIAGNOSTICS_CONFIGURED_EGRESS_LABEL", "fixed-egress")
	t.Setenv("MAGICPODCAST_FEED_RETRY_BUDGET", "4")
	t.Setenv("MAGICPODCAST_FEED_TIMEOUTS_OVERALL", "50s")
	t.Setenv("MAGICPODCAST_FEED_USER_AGENT_RECOVERY_INITIAL_COOLDOWN", "9h")
	t.Setenv("MAGICPODCAST_FEED_USER_AGENT_RECOVERY_PROBE_FAILURE_COOLDOWN", "42h")
	t.Setenv("MAGICPODCAST_FEED_USER_AGENT_RECOVERY_REQUIRED_SUCCESSES", "5")

	configPath := writeFeedTestConfig(t, "")
	loaded, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.Feed.UserAgent != "MagicPodcast-ENV/2.0" {
		t.Fatalf("UserAgent = %q, want ENV value", loaded.Feed.UserAgent)
	}
	if loaded.Feed.Diagnostics.ConfiguredEgressLabel != "fixed-egress" {
		t.Fatalf("ConfiguredEgressLabel = %q, want ENV value", loaded.Feed.Diagnostics.ConfiguredEgressLabel)
	}
	if loaded.Feed.Retry.Budget != 4 {
		t.Fatalf("Retry.Budget = %d, want 4", loaded.Feed.Retry.Budget)
	}
	if loaded.Feed.Timeouts.Overall != 50*time.Second {
		t.Fatalf("Timeouts.Overall = %v, want 50s", loaded.Feed.Timeouts.Overall)
	}
	if loaded.Feed.UserAgentRecovery.InitialCooldown != 9*time.Hour || loaded.Feed.UserAgentRecovery.ProbeFailureCooldown != 42*time.Hour || loaded.Feed.UserAgentRecovery.RequiredSuccesses != 5 {
		t.Fatalf("UserAgentRecovery = %+v, want ENV values", loaded.Feed.UserAgentRecovery)
	}
}

func TestLoadFeedENVTakesPrecedenceOverYAML(t *testing.T) {
	t.Cleanup(func() {
		cfg = nil
		viper.Reset()
	})
	viper.Reset()

	t.Setenv("MAGICPODCAST_FEED_USER_AGENT", "MagicPodcast-Wins/3.0")
	configPath := writeFeedTestConfig(t, `
feed:
  user_agent: "MagicPodcast-YAML-Loses/1.0"
`)
	loaded, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.Feed.UserAgent != "MagicPodcast-Wins/3.0" {
		t.Fatalf("UserAgent = %q, want ENV value to win over YAML", loaded.Feed.UserAgent)
	}
}

func TestLoadFeedENVDecodesCollectionOverrides(t *testing.T) {
	t.Cleanup(func() {
		cfg = nil
		viper.Reset()
	})
	viper.Reset()

	t.Setenv("MAGICPODCAST_FEED_CIRCUIT_THRESHOLDS_PER_CATEGORY", `{"service_unavailable":2,"timeout":3}`)
	t.Setenv("MAGICPODCAST_FEED_DOMAIN_POLICIES", `[{"domain":"feed.example.com","max_concurrency":2,"min_refresh_interval":"30m","evidence_window":"5m"}]`)

	loaded, err := Load(writeFeedTestConfig(t, ""))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.Feed.Circuit.ThresholdsPerCategory["service_unavailable"] != 2 || loaded.Feed.Circuit.ThresholdsPerCategory["timeout"] != 3 {
		t.Fatalf("ThresholdsPerCategory = %+v", loaded.Feed.Circuit.ThresholdsPerCategory)
	}
	if len(loaded.Feed.DomainPolicies) != 1 {
		t.Fatalf("DomainPolicies = %+v, want one policy", loaded.Feed.DomainPolicies)
	}
	policy := loaded.Feed.DomainPolicies[0]
	if policy.Domain != "feed.example.com" || policy.MaxConcurrency != 2 || policy.MinRefreshInterval != 30*time.Minute || policy.EvidenceWindow != 5*time.Minute {
		t.Fatalf("DomainPolicy = %+v", policy)
	}
}

func TestLoadFeedENVRejectsInvalidCollectionOverride(t *testing.T) {
	t.Cleanup(func() {
		cfg = nil
		viper.Reset()
	})
	viper.Reset()

	t.Setenv("MAGICPODCAST_FEED_CIRCUIT_THRESHOLDS_PER_CATEGORY", `{"timeout":`)
	_, err := Load(writeFeedTestConfig(t, ""))
	if err == nil || !strings.Contains(err.Error(), "MAGICPODCAST_FEED_CIRCUIT_THRESHOLDS_PER_CATEGORY") {
		t.Fatalf("Load() error = %v, want collection override parse failure", err)
	}
}

func TestLoadFeedRejectsInvalidBudget(t *testing.T) {
	t.Cleanup(func() {
		cfg = nil
		viper.Reset()
	})
	viper.Reset()

	configPath := writeFeedTestConfig(t, `
feed:
  retry:
    budget: 99
`)
	_, err := Load(configPath)
	if err == nil {
		t.Fatal("Load() succeeded for retry.budget above the hard cap, want startup failure")
	}
	if !strings.Contains(err.Error(), "feed") {
		t.Fatalf("error should mention feed config: %v", err)
	}
}

func TestLoadFeedRejectsNegativeTimeout(t *testing.T) {
	t.Cleanup(func() {
		cfg = nil
		viper.Reset()
	})
	viper.Reset()

	configPath := writeFeedTestConfig(t, `
feed:
  timeouts:
    overall: -5s
`)
	_, err := Load(configPath)
	if err == nil {
		t.Fatal("Load() succeeded for negative timeout, want startup failure")
	}
}
