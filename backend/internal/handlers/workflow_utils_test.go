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
	require.Equal(t, int64((10 * time.Minute).Milliseconds()), *rem)

	activeStart := time.Now().Add(-2 * time.Minute)
	active := &models.Job{Status: models.JobStatusRunning, StartTime: &activeStart}
	activeRem := batchRemainingMs(active)
	require.NotNil(t, activeRem)
	// Roughly 13 minutes left; allow clock skew.
	require.Greater(t, *activeRem, int64((12 * time.Minute).Milliseconds()))
	require.Less(t, *activeRem, int64((14 * time.Minute).Milliseconds()))
	_ = feed.DefaultBatchDuration
}
