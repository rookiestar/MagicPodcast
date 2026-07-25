package workflow

import (
	"testing"
	"time"

	"magicpodcast/internal/feed"
	"magicpodcast/internal/models"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestBuildRootCauseSummaryDoesNotDoubleCountCircuitOpen(t *testing.T) {
	pid := uint(1)
	attempts := []models.JobFeedAttempt{
		{PodcastID: &pid, AttemptNo: 1, SourceType: "primary", ErrorCategory: string(feed.ErrorCategoryAccessDenied), IsFinalResult: false},
		{PodcastID: &pid, AttemptNo: 2, SourceType: "primary", ErrorCategory: string(feed.ErrorCategoryCircuitOpen), DerivedPolicy: true, IsFinalResult: true},
	}
	summary := BuildRootCauseSummary(attempts)
	require.Equal(t, 1, summary.UpstreamRootCauses[string(feed.ErrorCategoryAccessDenied)])
	require.Equal(t, 1, summary.DerivedPolicyActions[string(feed.ErrorCategoryCircuitOpen)])
	// circuit_open must not appear in upstream map.
	_, has := summary.UpstreamRootCauses[string(feed.ErrorCategoryCircuitOpen)]
	require.False(t, has)
	require.Equal(t, "访问被拒绝 (403/401)", summary.UserLabels[string(feed.ErrorCategoryAccessDenied)])
}

func TestPersistAndListFeedAttemptsSafeFields(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:attempts_"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.JobFeedAttempt{}))

	pid := uint(7)
	status := 403
	require.NoError(t, PersistFeedAttempt(db, &models.JobFeedAttempt{
		JobID:         9,
		PodcastID:     &pid,
		AttemptNo:     1,
		SourceType:    "primary",
		AttemptedAt:   time.Now(),
		HTTPStatus:    &status,
		ErrorCategory: string(feed.ErrorCategoryAccessDenied),
		RetryDecision: "access_denied_scheduled",
		SourceURL:     "https://user:secret@feed.example.com/x?token=abc",
		IsFinalResult: true,
	}))

	rows, err := ListFeedAttempts(db, 9)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	safe := SanitizeAttemptForAPI(rows[0])
	require.NotContains(t, safe.SourceURL, "secret")
	require.NotContains(t, safe.SourceURL, "token=abc")
	require.True(t, safe.DerivedPolicy == false)
	require.Equal(t, "访问被拒绝 (403/401)", ErrorCategoryUserLabel(safe.ErrorCategory))
}

func TestHistoricalNotObservedStaysUnknown(t *testing.T) {
	require.Equal(t, "未观测", ErrorCategoryUserLabel(string(feed.ErrorCategoryNotObserved)))
	require.Equal(t, "未观测", ErrorCategoryUserLabel(""))
}
