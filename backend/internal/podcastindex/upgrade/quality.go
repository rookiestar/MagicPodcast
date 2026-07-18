package upgrade

import (
	"fmt"
	"math"
	"sort"
)

func CompareDatabaseMetrics(baseline, candidate DatabaseMetrics, maxChangeFraction float64) MetricsComparison {
	if maxChangeFraction <= 0 {
		maxChangeFraction = 0.20
	}
	comparison := MetricsComparison{
		TotalRows:             metricDelta(baseline.TotalRows, candidate.TotalRows),
		LiveRows:              metricDelta(baseline.LiveRows, candidate.LiveRows),
		HTTP200Rows:           metricDelta(baseline.HTTP200Rows, candidate.HTTP200Rows),
		DeadRows:              metricDelta(baseline.DeadRows, candidate.DeadRows),
		DeadDistributionDelta: make(map[string]int64),
		FreshestItemDateDelta: candidate.FreshestItemDate - baseline.FreshestItemDate,
		OldestItemDateDelta:   candidate.OldestItemDate - baseline.OldestItemDate,
		MaxChangeFraction:     maxChangeFraction,
		Passed:                true,
	}
	comparison.LiveRateBaseline = rate(baseline.LiveRows, baseline.TotalRows)
	comparison.LiveRateCandidate = rate(candidate.LiveRows, candidate.TotalRows)
	comparison.HTTP200RateBaseline = rate(baseline.HTTP200Rows, baseline.TotalRows)
	comparison.HTTP200RateCandidate = rate(candidate.HTTP200Rows, candidate.TotalRows)

	keys := make(map[string]struct{}, len(baseline.DeadDistribution)+len(candidate.DeadDistribution))
	for key := range baseline.DeadDistribution {
		keys[key] = struct{}{}
	}
	for key := range candidate.DeadDistribution {
		keys[key] = struct{}{}
	}
	orderedKeys := make([]string, 0, len(keys))
	for key := range keys {
		orderedKeys = append(orderedKeys, key)
	}
	sort.Strings(orderedKeys)
	for _, key := range orderedKeys {
		delta := candidate.DeadDistribution[key] - baseline.DeadDistribution[key]
		if delta != 0 {
			comparison.DeadDistributionDelta[key] = delta
		}
	}

	for name, delta := range map[string]MetricDelta{
		"total rows":    comparison.TotalRows,
		"live rows":     comparison.LiveRows,
		"HTTP 200 rows": comparison.HTTP200Rows,
		"dead rows":     comparison.DeadRows,
	} {
		if delta.ChangeFraction > maxChangeFraction {
			comparison.Reasons = append(comparison.Reasons,
				fmt.Sprintf("%s change %.2f%% exceeds %.2f%% review gate", name, delta.ChangeFraction*100, maxChangeFraction*100))
		}
	}

	if baseline.TotalRows > 0 && candidate.TotalRows > 0 {
		if baseline.LiveRows > 0 && comparison.LiveRateCandidate < comparison.LiveRateBaseline-maxChangeFraction {
			comparison.Reasons = append(comparison.Reasons,
				fmt.Sprintf("live-row ratio dropped from %.2f%% to %.2f%%", comparison.LiveRateBaseline*100, comparison.LiveRateCandidate*100))
		}
		if baseline.HTTP200Rows > 0 && comparison.HTTP200RateCandidate < comparison.HTTP200RateBaseline-maxChangeFraction {
			comparison.Reasons = append(comparison.Reasons,
				fmt.Sprintf("HTTP 200 ratio dropped from %.2f%% to %.2f%%", comparison.HTTP200RateBaseline*100, comparison.HTTP200RateCandidate*100))
		}
	}
	for key, delta := range comparison.DeadDistributionDelta {
		if math.Abs(float64(delta))/float64(maxInt64(1, baseline.TotalRows)) > maxChangeFraction {
			comparison.Reasons = append(comparison.Reasons,
				fmt.Sprintf("dead=%s distribution changed by %d rows", key, delta))
		}
	}
	if baseline.FreshestItemDate > 0 && candidate.FreshestItemDate > 0 && candidate.FreshestItemDate < baseline.FreshestItemDate {
		comparison.Reasons = append(comparison.Reasons, "candidate freshest item date is older than the baseline")
	}

	sort.Strings(comparison.Reasons)
	comparison.Passed = len(comparison.Reasons) == 0
	return comparison
}

func metricDelta(baseline, candidate int64) MetricDelta {
	delta := candidate - baseline
	return MetricDelta{
		Baseline:       baseline,
		Candidate:      candidate,
		Delta:          delta,
		ChangeFraction: changeFraction(baseline, candidate),
	}
}

func changeFraction(baseline, candidate int64) float64 {
	if baseline == 0 {
		if candidate == 0 {
			return 0
		}
		return 1
	}
	delta := float64(candidate - baseline)
	if delta < 0 {
		delta = -delta
	}
	return delta / math.Abs(float64(baseline))
}

func rate(numerator, denominator int64) float64 {
	if denominator <= 0 {
		return 0
	}
	return float64(numerator) / float64(denominator)
}

func maxInt64(left, right int64) int64 {
	if left > right {
		return left
	}
	return right
}
