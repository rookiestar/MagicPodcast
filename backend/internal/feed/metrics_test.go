package feed

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func intPtr(v int) *int { return &v }

// serializeFeedMetricsSnapshot marshals a snapshot to JSON for leakage
// assertions: the raw bytes let a test assert that no full URL, path, query,
// body, cookie, credential, or podcast id appears anywhere in the output.
func serializeFeedMetricsSnapshot(t *testing.T, m *FeedMetrics) string {
	t.Helper()
	raw, err := json.Marshal(m.Snapshot())
	require.NoError(t, err)
	return string(raw)
}

// TestFeedMetricsRecordFetchLabels verifies that RecordFetch aggregates by the
// bounded {domain,status,category,source} dimensions and that a full Feed URL
// never becomes a label — only the host surfaces, with no path/query/user info.
func TestFeedMetricsRecordFetchLabels(t *testing.T) {
	m := NewFeedMetrics()
	m.RecordFetch(AccessOutcome{
		HTTPStatus:    intPtr(http.StatusOK),
		ErrorCategory: ErrorCategoryNone,
		TargetDomain:  "example.com",
		SourceType:    AccessSourcePrimary,
		CacheStatus:   CacheStatusNotUsed,
		Freshness:     FreshnessLive,
	})
	// A second primary 200 from the same domain rolls into the same row.
	m.RecordFetch(AccessOutcome{
		HTTPStatus:    intPtr(http.StatusOK),
		ErrorCategory: ErrorCategoryNone,
		TargetDomain:  "example.com",
		SourceType:    AccessSourcePrimary,
	})
	// A 403 primary creates a distinct row.
	m.RecordFetch(AccessOutcome{
		HTTPStatus:    intPtr(http.StatusForbidden),
		ErrorCategory: ErrorCategoryAccessDenied,
		TargetDomain:  "example.com",
		SourceType:    AccessSourcePrimary,
	})
	// A last-good hit records status "0" (no HTTP status on recovery) but is
	// excluded from the latency histogram.
	m.RecordFetch(AccessOutcome{
		ErrorCategory: ErrorCategoryNone,
		TargetDomain:  "example.com",
		SourceType:    AccessSourceLastGood,
	})

	snap := m.Snapshot()
	require.Len(t, snap.FeedFetchTotal, 3, "distinct label tuples produce distinct rows")
	require.Equal(t, int64(2), findFetchRow(t, snap.FeedFetchTotal, "example.com", "200", string(ErrorCategoryNone), string(AccessSourcePrimary)).Count)
	require.Equal(t, int64(1), findFetchRow(t, snap.FeedFetchTotal, "example.com", "403", string(ErrorCategoryAccessDenied), string(AccessSourcePrimary)).Count)
	require.Equal(t, int64(1), findFetchRow(t, snap.FeedFetchTotal, "example.com", statusNoResponse, string(ErrorCategoryNone), string(AccessSourceLastGood)).Count)

	// The three primary outcomes (2x200 + 1x403) all contributed latency
	// samples; the last-good recovery is excluded. A fast 403 IS a latency
	// signal (edge vs. origin), so it belongs in the distribution.
	require.Equal(t, int64(3), snap.FeedFetchDuration.Count)
	for label := range snap.FeedFetchDuration.Buckets {
		// Bucket labels are fixed bucket bounds or +Inf — never a URL or status.
		require.NotContains(t, label, "http")
		require.NotContains(t, label, "example")
	}
}

// TestFeedMetricsNoHighCardinalityLabels asserts the cardinality guarantee: the
// only per-Feed axis is the low-cardinality target_domain, and a full Feed URL
// or podcast id never appears as a label value anywhere in the snapshot.
func TestFeedMetricsNoHighCardinalityLabels(t *testing.T) {
	m := NewFeedMetrics()
	m.RecordFetch(AccessOutcome{
		HTTPStatus:    intPtr(http.StatusForbidden),
		ErrorCategory: ErrorCategoryAccessDenied,
		TargetDomain:  "feed.xyzfm.space",
		SourceType:    AccessSourcePrimary,
	})
	m.RecordCircuitTransition("feed.xyzfm.space", CircuitStateClosed, CircuitStateOpen)
	m.RecordRetry("feed.xyzfm.space")

	raw := serializeFeedMetricsSnapshot(t, m)
	// A full Feed URL, path, or query must never leak into any label.
	require.NotContains(t, raw, "https://")
	require.NotContains(t, raw, "/feed.xml")
	require.NotContains(t, raw, "podcast_id")
	require.NotContains(t, raw, "token=")
}

// TestFeedMetricsConditionalGet verifies the three valid results and that an
// unknown result is rejected (no spurious label introduced).
func TestFeedMetricsConditionalGet(t *testing.T) {
	m := NewFeedMetrics()
	m.RecordConditionalGet(conditionalGetResult304)
	m.RecordConditionalGet(conditionalGetResult304)
	m.RecordConditionalGet(conditionalGetResult200)
	m.RecordConditionalGet(conditionalGetResultMiss)
	m.RecordConditionalGet("bogus")

	snap := m.Snapshot()
	require.Len(t, snap.ConditionalGetTotal, 3)
	require.Equal(t, int64(2), findConditionalGetRow(t, snap.ConditionalGetTotal, conditionalGetResult304).Count)
	require.Equal(t, int64(1), findConditionalGetRow(t, snap.ConditionalGetTotal, conditionalGetResult200).Count)
	require.Equal(t, int64(1), findConditionalGetRow(t, snap.ConditionalGetTotal, conditionalGetResultMiss).Count)
}

func TestFeedMetricsCircuitTransitionsAndRetry(t *testing.T) {
	m := NewFeedMetrics()
	m.RecordCircuitTransition("feed.xyzfm.space", CircuitStateClosed, CircuitStateOpen)
	m.RecordCircuitTransition("feed.xyzfm.space", CircuitStateClosed, CircuitStateOpen)
	m.RecordCircuitTransition("feed.xyzfm.space", CircuitStateOpen, CircuitStateProbe)
	m.RecordRetry("feed.xyzfm.space")
	m.RecordRetry("feed.xyzfm.space")

	snap := m.Snapshot()
	require.Equal(t, int64(2), findTransitionRow(t, snap.CircuitTransitions, "feed.xyzfm.space", string(CircuitStateClosed), string(CircuitStateOpen)).Count)
	require.Equal(t, int64(1), findTransitionRow(t, snap.CircuitTransitions, "feed.xyzfm.space", string(CircuitStateOpen), string(CircuitStateProbe)).Count)
	require.Equal(t, int64(2), findRetryRow(t, snap.RetryTotal, "feed.xyzfm.space").Count)
}

func TestFeedMetricsLastGoodHitAndEgressLabel(t *testing.T) {
	m := NewFeedMetrics()
	m.RecordLastGoodHit()
	m.RecordLastGoodHit()
	m.SetConfiguredEgressLabel("experiment-xyzfm-egress")

	snap := m.Snapshot()
	require.Equal(t, int64(2), snap.LastGoodHitsTotal)
	require.Equal(t, "experiment-xyzfm-egress", snap.ConfiguredEgressLabel)
}

// TestFeedMetricsReset clears every counter and restores the default egress
// label so tests sharing an injected registry start from a known baseline.
func TestFeedMetricsReset(t *testing.T) {
	m := NewFeedMetrics()
	m.RecordFetch(AccessOutcome{TargetDomain: "example.com", SourceType: AccessSourcePrimary})
	m.RecordLastGoodHit()
	m.SetConfiguredEgressLabel("experiment-xyzfm-egress")
	m.Reset()

	snap := m.Snapshot()
	require.Empty(t, snap.FeedFetchTotal)
	require.Zero(t, snap.LastGoodHitsTotal)
	require.Zero(t, snap.FeedFetchDuration.Count)
	require.Equal(t, EgressDirect, snap.ConfiguredEgressLabel)
}

func findFetchRow(t *testing.T, rows []FetchCounterRow, domain, status, category, source string) FetchCounterRow {
	t.Helper()
	for _, row := range rows {
		if row.Domain == domain && row.Status == status && row.Category == category && row.Source == source {
			return row
		}
	}
	t.Fatalf("fetch row not found: domain=%s status=%s category=%s source=%s", domain, status, category, source)
	return FetchCounterRow{}
}

func findTransitionRow(t *testing.T, rows []CircuitTransitionRow, domain, from, to string) CircuitTransitionRow {
	t.Helper()
	for _, row := range rows {
		if row.Domain == domain && row.From == from && row.To == to {
			return row
		}
	}
	t.Fatalf("transition row not found: domain=%s %s->%s", domain, from, to)
	return CircuitTransitionRow{}
}

func findConditionalGetRow(t *testing.T, rows []ConditionalGetRow, result string) ConditionalGetRow {
	t.Helper()
	for _, row := range rows {
		if row.Result == result {
			return row
		}
	}
	t.Fatalf("conditional_get row not found: result=%s", result)
	return ConditionalGetRow{}
}

func findRetryRow(t *testing.T, rows []RetryRow, domain string) RetryRow {
	t.Helper()
	for _, row := range rows {
		if row.Domain == domain {
			return row
		}
	}
	t.Fatalf("retry row not found: domain=%s", domain)
	return RetryRow{}
}
