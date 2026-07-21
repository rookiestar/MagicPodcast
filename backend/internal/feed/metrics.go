package feed

import (
	"sort"
	"strconv"
	"sync"
)

// feedDurationBuckets are the fixed histogram bucket upper bounds (seconds) for
// feed_fetch_duration_seconds. Fixed buckets keep the cardinality bounded and
// make the distribution directly readable in the admin JSON without a
// Prometheus dependency.
var feedDurationBuckets = []float64{0.1, 0.5, 1, 2.5, 5, 10}

const (
	conditionalGetResult304  = "304"
	conditionalGetResult200  = "200"
	conditionalGetResultMiss = "miss"
	durationOverflowLabel    = "+Inf"
	statusNoResponse         = "0"
)

// FeedMetrics is a lightweight, in-process counter registry for Feed fetch
// diagnostics. It deliberately avoids Prometheus / expvar / a /metrics endpoint:
// counters are low-cardinality aggregations surfaced only through the protected
// admin JSON entry. Labels NEVER include a full Feed URL, podcast id, response
// body, cookie, credential, or arbitrary response header. The only per-Feed axis
// is target_domain, which stays low-cardinality for this single-user service;
// status / category / source / result are bounded enums.
type FeedMetrics struct {
	mu sync.Mutex

	fetchTotal        map[fetchCounterKey]int64
	durationBuckets   map[string]int64
	durationCount     int64
	durationSumMillis float64

	circuitTransitions map[circuitTransitionKey]int64
	circuitRetry       map[string]int64 // domain -> half-open probe admissions

	lastGoodHits   int64
	conditionalGet map[string]int64

	configuredEgressLabel string
}

type fetchCounterKey struct {
	Domain   string
	Status   string
	Category string
	Source   string
}

type circuitTransitionKey struct {
	Domain string
	From   string
	To     string
}

// NewFeedMetrics returns an empty registry with the default ("direct") egress
// label. The process-wide singleton is the production instance; tests construct
// their own and inject via Coordinator.SetMetrics for isolation.
func NewFeedMetrics() *FeedMetrics {
	return &FeedMetrics{
		fetchTotal:          make(map[fetchCounterKey]int64),
		durationBuckets:     make(map[string]int64),
		circuitTransitions:  make(map[circuitTransitionKey]int64),
		circuitRetry:        make(map[string]int64),
		conditionalGet:      make(map[string]int64),
		configuredEgressLabel: EgressDirect,
	}
}

var processFeedMetrics = NewFeedMetrics()

// SharedFeedMetrics is the process-wide registry fed by the shared Coordinator.
func SharedFeedMetrics() *FeedMetrics { return processFeedMetrics }

// Reset clears every counter. It is intended for tests that inject this
// instance via Coordinator.SetMetrics; production code never resets.
func (m *FeedMetrics) Reset() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.fetchTotal = make(map[fetchCounterKey]int64)
	m.durationBuckets = make(map[string]int64)
	m.durationCount = 0
	m.durationSumMillis = 0
	m.circuitTransitions = make(map[circuitTransitionKey]int64)
	m.circuitRetry = make(map[string]int64)
	m.lastGoodHits = 0
	m.conditionalGet = make(map[string]int64)
	m.configuredEgressLabel = EgressDirect
}

// RecordFetch aggregates one observed Feed access outcome. Duration is recorded
// only for outcomes that performed real network work (primary / alternative
// sources); cache hits and last-good recoveries set ResponseTimeMs=0 and would
// otherwise pollute the latency distribution.
func (m *FeedMetrics) RecordFetch(outcome AccessOutcome) {
	if m == nil {
		return
	}
	key := fetchCounterKey{
		Domain:   outcome.TargetDomain,
		Status:   fetchStatusLabel(outcome.HTTPStatus),
		Category: string(outcome.ErrorCategory),
		Source:   string(outcome.SourceType),
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.fetchTotal[key]++
	if outcome.SourceType == AccessSourcePrimary || outcome.SourceType == AccessSourceAlternative {
		m.durationCount++
		m.durationSumMillis += float64(outcome.ResponseTimeMs)
		m.durationBuckets[durationBucketLabel(float64(outcome.ResponseTimeMs)/1000.0)]++
	}
}

// RecordConditionalGet records a conditional-GET outcome: "304" (validators sent
// and the server confirmed Not Modified), "200" (validators sent but the content
// changed, returned in full), or "miss" (no validated snapshot existed, so the
// request was unconditional).
func (m *FeedMetrics) RecordConditionalGet(result string) {
	if m == nil {
		return
	}
	switch result {
	case conditionalGetResult304, conditionalGetResult200, conditionalGetResultMiss:
	default:
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.conditionalGet[result]++
}

// RecordLastGoodHit records that a persisted last-good snapshot successfully
// served a Feed after a live fetch failed.
func (m *FeedMetrics) RecordLastGoodHit() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastGoodHits++
}

// RecordRetry records one circuit half-open probe admission for a domain — the
// recovery re-attempt after its circuit had opened. There is no request-level
// retry loop in this service, so this counter reflects circuit-recovery retry
// pressure rather than per-request retries.
func (m *FeedMetrics) RecordRetry(domain string) {
	if m == nil || domain == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.circuitRetry[domain]++
}

// RecordCircuitTransition counts one domain circuit state transition (e.g.
// closed->open, open->probe, probe->closed).
func (m *FeedMetrics) RecordCircuitTransition(domain string, from, to CircuitState) {
	if m == nil || domain == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.circuitTransitions[circuitTransitionKey{Domain: domain, From: string(from), To: string(to)}]++
}

// SetConfiguredEgressLabel records the configured egress tag (default "direct").
// It is a configuration label ONLY and is never proof of the real public egress.
func (m *FeedMetrics) SetConfiguredEgressLabel(label string) {
	if m == nil || label == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.configuredEgressLabel = label
}

// FeedMetricsSnapshot is the serializable, whitelisted counter view. It contains
// only bounded aggregations — never raw URLs, bodies, cookies, or credentials.
type FeedMetricsSnapshot struct {
	FeedFetchTotal       []FetchCounterRow     `json:"feed_fetch_total"`
	FeedFetchDuration    DurationHistogram     `json:"feed_fetch_duration_seconds"`
	CircuitTransitions   []CircuitTransitionRow `json:"circuit_transitions_total"`
	LastGoodHitsTotal    int64                 `json:"last_good_hits_total"`
	ConditionalGetTotal  []ConditionalGetRow   `json:"conditional_get_total"`
	RetryTotal           []RetryRow            `json:"retry_total"`
	ConfiguredEgressLabel string                `json:"configured_egress_label"`
}

// Snapshot returns a sorted, deterministic copy of the counters.
func (m *FeedMetrics) Snapshot() FeedMetricsSnapshot {
	if m == nil {
		return FeedMetricsSnapshot{ConfiguredEgressLabel: EgressDirect}
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	fetchRows := make([]FetchCounterRow, 0, len(m.fetchTotal))
	for key, count := range m.fetchTotal {
		fetchRows = append(fetchRows, FetchCounterRow{
			Domain: key.Domain, Status: key.Status, Category: key.Category, Source: key.Source, Count: count,
		})
	}
	sort.Slice(fetchRows, func(i, j int) bool {
		a, b := fetchRows[i], fetchRows[j]
		if a.Domain != b.Domain {
			return a.Domain < b.Domain
		}
		if a.Status != b.Status {
			return a.Status < b.Status
		}
		if a.Category != b.Category {
			return a.Category < b.Category
		}
		return a.Source < b.Source
	})

	transitionRows := make([]CircuitTransitionRow, 0, len(m.circuitTransitions))
	for key, count := range m.circuitTransitions {
		transitionRows = append(transitionRows, CircuitTransitionRow{
			Domain: key.Domain, From: key.From, To: key.To, Count: count,
		})
	}
	sort.Slice(transitionRows, func(i, j int) bool {
		a, b := transitionRows[i], transitionRows[j]
		if a.Domain != b.Domain {
			return a.Domain < b.Domain
		}
		if a.From != b.From {
			return a.From < b.From
		}
		return a.To < b.To
	})

	retryRows := make([]RetryRow, 0, len(m.circuitRetry))
	for domain, count := range m.circuitRetry {
		retryRows = append(retryRows, RetryRow{Domain: domain, Count: count})
	}
	sort.Slice(retryRows, func(i, j int) bool { return retryRows[i].Domain < retryRows[j].Domain })

	condRows := make([]ConditionalGetRow, 0, len(m.conditionalGet))
	for result, count := range m.conditionalGet {
		condRows = append(condRows, ConditionalGetRow{Result: result, Count: count})
	}
	sort.Slice(condRows, func(i, j int) bool { return condRows[i].Result < condRows[j].Result })

	buckets := make(map[string]int64, len(feedDurationBuckets)+1)
	for _, b := range feedDurationBuckets {
		buckets[strconv.FormatFloat(b, 'g', -1, 64)] = m.durationBuckets[strconv.FormatFloat(b, 'g', -1, 64)]
	}
	buckets[durationOverflowLabel] = m.durationBuckets[durationOverflowLabel]

	return FeedMetricsSnapshot{
		FeedFetchTotal: fetchRows,
		FeedFetchDuration: DurationHistogram{
			Buckets:    buckets,
			Count:      m.durationCount,
			SumSeconds: m.durationSumMillis / 1000.0,
		},
		CircuitTransitions:    transitionRows,
		LastGoodHitsTotal:     m.lastGoodHits,
		ConditionalGetTotal:   condRows,
		RetryTotal:            retryRows,
		ConfiguredEgressLabel: m.configuredEgressLabel,
	}
}

func fetchStatusLabel(status *int) string {
	if status == nil {
		return statusNoResponse
	}
	return strconv.Itoa(*status)
}

func durationBucketLabel(seconds float64) string {
	for _, boundary := range feedDurationBuckets {
		if seconds <= boundary {
			return strconv.FormatFloat(boundary, 'g', -1, 64)
		}
	}
	return durationOverflowLabel
}
