package upgrade

import "testing"

func TestCompareDatabaseMetricsRecordsStableQualityAndDeadDistribution(t *testing.T) {
	baseline := DatabaseMetrics{
		TotalRows:        100,
		LiveRows:         90,
		HTTP200Rows:      80,
		DeadRows:         10,
		FreshestItemDate: 200,
		OldestItemDate:   10,
		DeadDistribution: map[string]int64{"0": 90, "1": 10},
	}
	candidate := DatabaseMetrics{
		TotalRows:        105,
		LiveRows:         94,
		HTTP200Rows:      84,
		DeadRows:         11,
		FreshestItemDate: 210,
		OldestItemDate:   5,
		DeadDistribution: map[string]int64{"0": 94, "1": 11},
	}
	comparison := CompareDatabaseMetrics(baseline, candidate, 0.20)
	if !comparison.Passed {
		t.Fatalf("comparison = %+v, want pass", comparison)
	}
	if comparison.TotalRows.Delta != 5 || comparison.LiveRows.Delta != 4 || comparison.HTTP200Rows.Delta != 4 {
		t.Fatalf("metric deltas = %+v", comparison)
	}
	if comparison.DeadDistributionDelta["0"] != 4 || comparison.DeadDistributionDelta["1"] != 1 {
		t.Fatalf("dead distribution delta = %+v", comparison.DeadDistributionDelta)
	}
}

func TestCompareDatabaseMetricsRejectsUnexplainedQualityRegression(t *testing.T) {
	baseline := DatabaseMetrics{
		TotalRows:        100,
		LiveRows:         90,
		HTTP200Rows:      80,
		DeadRows:         10,
		FreshestItemDate: 200,
		DeadDistribution: map[string]int64{"0": 90, "1": 10},
	}
	candidate := DatabaseMetrics{
		TotalRows:        160,
		LiveRows:         80,
		HTTP200Rows:      50,
		DeadRows:         80,
		FreshestItemDate: 100,
		DeadDistribution: map[string]int64{"0": 80, "1": 80},
	}
	comparison := CompareDatabaseMetrics(baseline, candidate, 0.20)
	if comparison.Passed || len(comparison.Reasons) == 0 {
		t.Fatalf("comparison = %+v, want quality regression", comparison)
	}
}
