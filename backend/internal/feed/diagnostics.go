package feed

import (
	"context"
	"sort"
	"time"
)

// FetchCounterRow is one aggregated feed_fetch_total{domain,status,category,
// source} count. Every label is a bounded enum except target_domain, which is
// the only per-Feed axis and stays low-cardinality for this single-user
// service. A full Feed URL, podcast id, body, cookie, credential, or arbitrary
// response header NEVER appears as a label.
type FetchCounterRow struct {
	Domain   string `json:"domain"`
	Status   string `json:"status"`
	Category string `json:"category"`
	Source   string `json:"source"`
	Count    int64  `json:"count"`
}

// DurationHistogram is the fixed-bucket latency distribution for
// feed_fetch_duration_seconds. Buckets are upper bounds in seconds; "+Inf" is
// the overflow bucket beyond the largest configured bound.
type DurationHistogram struct {
	Buckets    map[string]int64 `json:"buckets"`
	Count      int64            `json:"count"`
	SumSeconds float64          `json:"sum_seconds"`
}

// CircuitTransitionRow is one aggregated circuit_transitions_total{domain,
// from,to} count. from/to are circuit states (not_used/open/probe/closed).
type CircuitTransitionRow struct {
	Domain string `json:"domain"`
	From   string `json:"from"`
	To     string `json:"to"`
	Count  int64  `json:"count"`
}

// ConditionalGetRow is one aggregated conditional_get_total{result} count.
// result is "304", "200", or "miss".
type ConditionalGetRow struct {
	Result string `json:"result"`
	Count  int64  `json:"count"`
}

// RetryRow is one aggregated retry_total{domain} count — the number of
// half-open probe admissions for that domain (there is no request-level retry
// loop in this service).
type RetryRow struct {
	Domain string `json:"domain"`
	Count  int64  `json:"count"`
}

// CircuitStateRow is the current gauge value of circuit_state{domain,state}:
// the live state (open/probe/closed) for one domain circuit, with the RFC3339
// instant its OPEN cooldown ends when applicable.
type CircuitStateRow struct {
	Domain    string  `json:"domain"`
	State     string  `json:"state"`
	OpenUntil *string `json:"open_until,omitempty"`
}

// UserAgentGateDiagnostic is the bounded external projection of one durable
// domain/User-Agent policy record. Only a short fingerprint prefix is exposed;
// the raw User-Agent and full digest remain outside the API response.
type UserAgentGateDiagnostic struct {
	Domain                     string     `json:"domain"`
	UserAgentFingerprintPrefix string     `json:"user_agent_fingerprint_prefix"`
	State                      string     `json:"state"`
	DetectedAt                 time.Time  `json:"detected_at"`
	ProbeEligibleAt            time.Time  `json:"probe_eligible_at"`
	ProbeEligible              bool       `json:"probe_eligible"`
	LastProbeResult            string     `json:"last_probe_result,omitempty"`
	RecoverySuccessCount       int        `json:"recovery_success_count"`
	ApprovedBy                 string     `json:"approved_by,omitempty"`
	ApprovedAt                 *time.Time `json:"approved_at,omitempty"`
	LastProbeAt                *time.Time `json:"last_probe_at,omitempty"`
}

// FeedDiagnosticsResponse is the complete, whitelisted view surfaced through
// the protected admin JSON entry. It contains only bounded aggregations and
// never a full Feed URL, response body, cookie, credential, or arbitrary
// response header.
type FeedDiagnosticsResponse struct {
	FeedFetchTotal        []FetchCounterRow         `json:"feed_fetch_total"`
	FeedFetchDuration     DurationHistogram         `json:"feed_fetch_duration_seconds"`
	CircuitState          []CircuitStateRow         `json:"circuit_state"`
	CircuitTransitions    []CircuitTransitionRow    `json:"circuit_transitions_total"`
	LastGoodHitsTotal     int64                     `json:"last_good_hits_total"`
	ConditionalGetTotal   []ConditionalGetRow       `json:"conditional_get_total"`
	RetryTotal            []RetryRow                `json:"retry_total"`
	UserAgentGates        []UserAgentGateDiagnostic `json:"user_agent_gates"`
	SnapshotStore         *SnapshotStoreStats       `json:"snapshot_store,omitempty"`
	ConfiguredEgressLabel string                    `json:"configured_egress_label"`
}

// BuildFeedDiagnostics composes the protected admin view from the counter
// registry and the live Coordinator state. The Coordinator's circuit gauges and
// last-good store stats are read here; counters come from the registry. Nothing
// returned escapes the bounded whitelist.
func BuildFeedDiagnostics(coord *Coordinator, metrics *FeedMetrics) FeedDiagnosticsResponse {
	return BuildFeedDiagnosticsWithUserAgentGate(coord, metrics, nil)
}

// BuildFeedDiagnosticsWithUserAgentGate composes the protected diagnostics
// view, including durable User-Agent policy state when an already-migrated
// store is wired. Store errors are omitted so this read-only view cannot change
// health/readiness or Feed behavior.
func BuildFeedDiagnosticsWithUserAgentGate(coord *Coordinator, metrics *FeedMetrics, gateStore UserAgentGateStore) FeedDiagnosticsResponse {
	if metrics == nil {
		metrics = SharedFeedMetrics()
	}
	snapshot := metrics.Snapshot()
	response := FeedDiagnosticsResponse{
		FeedFetchTotal:        snapshot.FeedFetchTotal,
		FeedFetchDuration:     snapshot.FeedFetchDuration,
		CircuitTransitions:    snapshot.CircuitTransitions,
		LastGoodHitsTotal:     snapshot.LastGoodHitsTotal,
		ConditionalGetTotal:   snapshot.ConditionalGetTotal,
		RetryTotal:            snapshot.RetryTotal,
		UserAgentGates:        buildUserAgentGateDiagnostics(context.Background(), gateStore, time.Now()),
		ConfiguredEgressLabel: snapshot.ConfiguredEgressLabel,
	}
	if coord != nil {
		response.CircuitState = coord.CircuitSnapshot()
		if stats, err := coord.LastGoodStats(); err == nil {
			response.SnapshotStore = &stats
		}
	}
	sort.Slice(response.CircuitState, func(i, j int) bool {
		return response.CircuitState[i].Domain < response.CircuitState[j].Domain
	})
	return response
}

func buildUserAgentGateDiagnostics(ctx context.Context, store UserAgentGateStore, now time.Time) []UserAgentGateDiagnostic {
	if store == nil {
		return []UserAgentGateDiagnostic{}
	}
	if now.IsZero() {
		now = time.Now()
	}
	records, err := store.List(ctx)
	if err != nil {
		return []UserAgentGateDiagnostic{}
	}
	result := make([]UserAgentGateDiagnostic, 0, len(records))
	for _, record := range records {
		result = append(result, UserAgentGateDiagnostic{
			Domain:                     record.Domain,
			UserAgentFingerprintPrefix: fingerprintPrefix(record.UserAgentFingerprint),
			State:                      record.State,
			DetectedAt:                 record.DetectedAt,
			ProbeEligibleAt:            record.ProbeEligibleAt,
			ProbeEligible:              record.State == UserAgentGateStateBlocked && !now.Before(record.ProbeEligibleAt),
			LastProbeResult:            record.LastProbeResult,
			RecoverySuccessCount:       record.RecoverySuccessCount,
			ApprovedBy:                 record.ApprovedBy,
			ApprovedAt:                 record.ApprovedAt,
			LastProbeAt:                record.LastProbeAt,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Domain != result[j].Domain {
			return result[i].Domain < result[j].Domain
		}
		return result[i].UserAgentFingerprintPrefix < result[j].UserAgentFingerprintPrefix
	})
	return result
}

func fingerprintPrefix(fingerprint string) string {
	if len(fingerprint) <= 12 {
		return fingerprint
	}
	return fingerprint[:12]
}
