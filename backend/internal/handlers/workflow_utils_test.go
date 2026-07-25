package handlers

import (
	"testing"
	"time"

	"magicpodcast/internal/feed"
	"magicpodcast/internal/models"

	"github.com/stretchr/testify/require"
)

func TestBatchRemainingMsForFinishedAndActiveJobs(t *testing.T) {
	start := time.Date(2026, 7, 25, 10, 0, 0, 0, time.UTC)
	end := start.Add(5 * time.Minute)
	finished := &models.Job{Status: models.JobStatusCompleted, StartTime: &start, EndTime: &end}
	rem := batchRemainingMs(finished)
	require.NotNil(t, rem)
	// 10-minute window minus 5 minutes elapsed → 5 minutes remaining (#44).
	require.Equal(t, int64((5 * time.Minute).Milliseconds()), *rem)
	require.Equal(t, 10*time.Minute, feed.DefaultBatchDuration)

	activeStart := time.Now().Add(-2 * time.Minute)
	active := &models.Job{Status: models.JobStatusRunning, StartTime: &activeStart}
	activeRem := batchRemainingMs(active)
	require.NotNil(t, activeRem)
	// Roughly 8 minutes left of the 10-minute window; allow clock skew.
	require.Greater(t, *activeRem, int64((7 * time.Minute).Milliseconds()))
	require.Less(t, *activeRem, int64((9 * time.Minute).Milliseconds()))
}
