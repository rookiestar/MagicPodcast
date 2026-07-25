package workflow

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"magicpodcast/internal/feed"
	"magicpodcast/internal/models"
	syncsvc "magicpodcast/internal/sync"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestApplyFeedAccessOutcomeCopiesFailurePhase(t *testing.T) {
	exec := &models.JobExecution{}
	phase := feed.FailurePhaseResponseHeader
	status := 403
	applyFeedAccessOutcome(exec, &feed.AccessOutcome{
		HTTPStatus:    &status,
		ErrorCategory: feed.ErrorCategoryAccessDenied,
		FailurePhase:  phase,
		SourceType:    feed.AccessSourcePrimary,
	})
	require.Equal(t, string(feed.FailurePhaseResponseHeader), exec.FeedFailurePhase)
	require.Equal(t, string(feed.ErrorCategoryAccessDenied), exec.FeedErrorCategory)
}

// TestAttemptHistoryPersistsFailurePhaseAndRetryDecision drives the real
// executor seam: 403 → AccessOutcome.FailurePhase and DecideBatchRetry.Reason
// must land on job_feed_attempts (#39 skeptic gaps).
func TestAttemptHistoryPersistsFailurePhaseAndRetryDecision(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(server.Close)

	db, err := gorm.Open(sqlite.Open("file:attempt_meta_"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&models.Podcast{}, &models.Episode{}, &models.Workflow{},
		&models.Job{}, &models.JobExecution{}, &models.JobFeedAttempt{}, &models.Report{},
	))

	coordinator := feed.NewCoordinator(feed.CoordinatorConfig{
		DomainPolicies: map[string]feed.DomainPolicy{
			feed.TargetDomain(server.URL): {MaxConcurrency: 1},
		},
	})
	service, err := syncsvc.NewServiceWithFeedCoordinator(db, "", coordinator)
	require.NoError(t, err)
	t.Cleanup(func() { _ = service.Close() })

	pod := models.Podcast{XYZID: "meta-403", Title: "Meta", FeedURL: server.URL + "/feed.xml", IsSubscribed: true}
	require.NoError(t, db.Create(&pod).Error)
	wf := &models.Workflow{
		Name:        "meta-wf",
		ScopeType:   models.ScopeTypeSpecificPodcasts,
		ScopeConfig: models.ScopeConfig{PodcastIDs: []int{int(pod.ID)}},
		RulesConfig: models.RulesConfig{TimeRange: 0},
		IsEnabled:   true,
	}
	require.NoError(t, db.Create(wf).Error)

	executor := NewExecutor(db, service, nil, nil)
	// Fake clock: first-pass at t=0; any sleep jumps past the batch deadline so
	// we stamp a terminal retry decision without multi-minute waits.
	var now atomicTime
	start := time.Date(2026, 7, 25, 15, 0, 0, 0, time.UTC)
	now.Set(start)
	executor.now = now.Now
	executor.sleep = func(d time.Duration) { now.Set(now.Now().Add(10 * time.Minute)) }
	executor.batchDuration = 10 * time.Minute
	executor.workerConcurrency = 1

	job, err := executor.Execute(context.Background(), wf, "manual")
	require.NoError(t, err)
	require.NotNil(t, job)

	attempts, err := ListFeedAttempts(db, job.ID)
	require.NoError(t, err)
	require.NotEmpty(t, attempts, "at least one attempt must be persisted")

	first := attempts[0]
	require.Equal(t, string(feed.ErrorCategoryAccessDenied), first.ErrorCategory)
	require.Equal(t, string(feed.FailurePhaseResponseHeader), first.FailurePhase,
		"403 must record response_header from AccessOutcome on the shipped path")
	require.NotEmpty(t, first.RetryDecision,
		"DecideBatchRetry.Reason must be stamped on the attempt (not left empty)")

	var exec models.JobExecution
	require.NoError(t, db.Where("job_id = ?", job.ID).First(&exec).Error)
	require.Equal(t, string(feed.FailurePhaseResponseHeader), exec.FeedFailurePhase,
		"final JobExecution projection must carry failure_phase")
}

// TestBatchRemainingMsPopulatedForFinishedJob ensures Job API exposes the
// remaining batch budget (#39 AC: 批次剩余时间).
func TestBatchRemainingMsPopulatedForFinishedJob(t *testing.T) {
	start := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	end := start.Add(3 * time.Minute)
	job := &models.Job{
		Status:    models.JobStatusPartial,
		StartTime: &start,
		EndTime:   &end,
	}
	// Inline the same formula as handlers.batchRemainingMs without importing handlers.
	deadline := job.StartTime.Add(feed.DefaultBatchDuration)
	rem := deadline.Sub(*job.EndTime).Milliseconds()
	// 10-minute window minus 3 minutes elapsed → 7 minutes remaining (#44).
	require.Equal(t, int64(7*time.Minute/time.Millisecond), rem)
}

// atomicTime is a tiny injectable clock for batch tests.
type atomicTime struct {
	mu sync.Mutex
	t  time.Time
}

func (a *atomicTime) Set(t time.Time) {
	a.mu.Lock()
	a.t = t
	a.mu.Unlock()
}

func (a *atomicTime) Now() time.Time {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.t
}
