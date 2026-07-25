package workflow

import (
	"context"
	"testing"
	"time"

	"magicpodcast/internal/feed"
	"magicpodcast/internal/models"
	syncsvc "magicpodcast/internal/sync"

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

func TestUserAgentCategoriesSeparateDirectAndDerivedPolicyActions(t *testing.T) {
	first, second := uint(1), uint(2)
	summary := BuildRootCauseSummary([]models.JobFeedAttempt{
		{PodcastID: &first, AttemptNo: 1, SourceType: string(feed.AccessSourcePrimary), ErrorCategory: string(feed.ErrorCategoryUserAgentDenied), IsFinalResult: true},
		{PodcastID: &second, AttemptNo: 1, SourceType: string(feed.AccessSourcePrimary), ErrorCategory: string(feed.ErrorCategoryUserAgentBlocked), DerivedPolicy: true, IsFinalResult: true},
	})
	require.Equal(t, 1, summary.UpstreamRootCauses[string(feed.ErrorCategoryUserAgentDenied)])
	require.Equal(t, 1, summary.DerivedPolicyActions[string(feed.ErrorCategoryUserAgentBlocked)])
	_, hasDerivedAsUpstream := summary.UpstreamRootCauses[string(feed.ErrorCategoryUserAgentBlocked)]
	require.False(t, hasDerivedAsUpstream)
	require.Equal(t, "User-Agent 被上游 ACL 拒绝", summary.UserLabels[string(feed.ErrorCategoryUserAgentDenied)])
	require.Equal(t, "User-Agent 已被同域策略阻断", summary.UserLabels[string(feed.ErrorCategoryUserAgentBlocked)])
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

func TestBuildRootCauseSummarySeparatesUnattemptedFeeds(t *testing.T) {
	first := uint(1)
	second := uint(2)
	summary := BuildRootCauseSummary([]models.JobFeedAttempt{
		{PodcastID: &first, AttemptNo: 1, SourceType: "primary", ErrorCategory: string(feed.ErrorCategoryAccessDenied), IsFinalResult: true},
		{PodcastID: &second, AttemptNo: 0, SourceType: string(feed.AccessSourceUnknown), ErrorCategory: string(feed.ErrorCategoryUnattempted), RetryDecision: "batch_deadline", IsFinalResult: true},
	})
	require.Equal(t, 2, summary.TotalFeeds)
	require.Equal(t, 1, summary.AttemptedFeeds)
	require.Equal(t, 1, summary.UnattemptedFeeds)
	require.Equal(t, 1, summary.FinalFailures)
	require.Equal(t, "未尝试（批次截止）", summary.UserLabels[string(feed.ErrorCategoryUnattempted)])
}

func TestRecordObservedFeedAttemptsPreservesObservationTime(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:observed_attempts_"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.JobFeedAttempt{}))

	executor := &Executor{db: db}
	podcast := &models.Podcast{BaseModel: models.BaseModel{ID: 11}, Title: "节目", FeedURL: "https://example.com/feed.xml"}
	execution := &models.JobExecution{JobID: 9, PodcastID: &podcast.ID, Status: models.ExecutionStatusFailed, FeedErrorCategory: string(feed.ErrorCategoryAccessDenied), FeedSourceType: string(feed.AccessSourcePrimary), FeedSourceURL: podcast.FeedURL}
	observedAt := time.Date(2026, 7, 25, 16, 0, 0, 0, time.UTC)
	executor.recordObservedFeedAttempts(9, podcast, execution, []feed.AttemptObservation{{
		Outcome:    feed.AccessOutcome{SourceType: feed.AccessSourcePrimary, SourceURL: podcast.FeedURL, ErrorCategory: feed.ErrorCategoryAccessDenied},
		ObservedAt: observedAt,
	}}, "batch_deadline")

	var attempt models.JobFeedAttempt
	require.NoError(t, db.Where("job_id = ?", 9).First(&attempt).Error)
	require.Equal(t, observedAt, attempt.AttemptedAt)
}

func TestCancelledBatchRecordsUnattemptedFeeds(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:unattempted_"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&models.Podcast{}, &models.Episode{}, &models.Workflow{}, &models.Job{}, &models.JobExecution{}, &models.JobFeedAttempt{}, &models.Report{},
	))
	service, err := syncsvc.NewServiceWithFeedCoordinator(db, "", feed.NewCoordinator(feed.CoordinatorConfig{}))
	require.NoError(t, err)
	t.Cleanup(func() { _ = service.Close() })
	podcast := models.Podcast{XYZID: "unattempted", Title: "未尝试", FeedURL: "https://example.com/feed.xml"}
	require.NoError(t, db.Create(&podcast).Error)
	wf := &models.Workflow{Name: "unattempted", ScopeType: models.ScopeTypeSpecificPodcasts, ScopeConfig: models.ScopeConfig{PodcastIDs: []int{int(podcast.ID)}}}
	require.NoError(t, db.Create(wf).Error)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	executor := NewExecutor(db, service, nil, nil)
	job, err := executor.Execute(ctx, wf, "manual")
	require.NoError(t, err)
	require.NotNil(t, job)

	var execution models.JobExecution
	require.NoError(t, db.Where("job_id = ?", job.ID).First(&execution).Error)
	require.Equal(t, string(feed.ErrorCategoryUnattempted), execution.FeedErrorCategory)
	var attempt models.JobFeedAttempt
	require.NoError(t, db.Where("job_id = ?", job.ID).First(&attempt).Error)
	require.Equal(t, -1, attempt.AttemptNo)
	require.Equal(t, string(feed.ErrorCategoryUnattempted), attempt.ErrorCategory)
}
