package workflow

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
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

func setupLifecycleDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("%s?_journal_mode=WAL&_busy_timeout=5000", filepath.Join(t.TempDir(), "lifecycle.db"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&models.Podcast{},
		&models.Episode{},
		&models.Workflow{},
		&models.Job{},
		&models.JobExecution{},
		&models.Report{},
	))
	require.NoError(t, db.Exec(models.ActiveJobUniqueIndexSQL).Error)
	return db
}

func TestClaimActiveJobRejectsConcurrentActiveJobs(t *testing.T) {
	db := setupLifecycleDB(t)
	wf := &models.Workflow{Name: "lock-wf", ScopeType: models.ScopeTypeAllSubscribed, IsEnabled: true}
	require.NoError(t, db.Create(wf).Error)

	first, err := ClaimActiveJob(db, wf.ID, "manual")
	require.NoError(t, err)
	require.Equal(t, models.JobStatusRunning, first.Status)

	_, err = ClaimActiveJob(db, wf.ID, "cron")
	require.ErrorIs(t, err, ErrWorkflowJobActive)

	// After terminal status, a new claim succeeds.
	first.Status = models.JobStatusCompleted
	end := time.Now()
	first.EndTime = &end
	require.NoError(t, db.Save(first).Error)

	second, err := ClaimActiveJob(db, wf.ID, "manual")
	require.NoError(t, err)
	require.NotEqual(t, first.ID, second.ID)
}

func TestClaimActiveJobAllowsExactlyOneConcurrentWinner(t *testing.T) {
	db := setupLifecycleDB(t)
	wf := &models.Workflow{Name: "race-wf", ScopeType: models.ScopeTypeAllSubscribed, IsEnabled: true}
	require.NoError(t, db.Create(wf).Error)

	const contenders = 12
	start := make(chan struct{})
	results := make(chan error, contenders)
	var wg sync.WaitGroup
	for i := 0; i < contenders; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := ClaimActiveJob(db, wf.ID, "manual")
			results <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	successes := 0
	for err := range results {
		if err == nil {
			successes++
			continue
		}
		require.True(t, errors.Is(err, ErrWorkflowJobActive), "unexpected claim error: %v", err)
	}
	require.Equal(t, 1, successes)
}

func TestSettleInterruptedJobsCancelsLeftoverRunning(t *testing.T) {
	db := setupLifecycleDB(t)
	wf := &models.Workflow{Name: "settle-wf", ScopeType: models.ScopeTypeAllSubscribed, IsEnabled: true}
	require.NoError(t, db.Create(wf).Error)
	start := time.Now().Add(-time.Hour)
	job := &models.Job{WorkflowID: wf.ID, Status: models.JobStatusRunning, StartTime: &start, TriggeredBy: "cron"}
	require.NoError(t, db.Create(job).Error)
	pid := uint(1)
	ex := &models.JobExecution{JobID: job.ID, PodcastID: &pid, Status: models.ExecutionStatusRunning, PodcastTitle: "x"}
	require.NoError(t, db.Create(ex).Error)

	jobs, execs, err := SettleInterruptedJobs(db)
	require.NoError(t, err)
	require.Equal(t, 1, jobs)
	require.Equal(t, 1, execs)

	var settled models.Job
	require.NoError(t, db.First(&settled, job.ID).Error)
	require.Equal(t, models.JobStatusCancelled, settled.Status)
	require.NotNil(t, settled.EndTime)

	var settledEx models.JobExecution
	require.NoError(t, db.First(&settledEx, ex.ID).Error)
	require.Equal(t, models.ExecutionStatusFailed, settledEx.Status)
	require.Equal(t, ProcessInterruptedReason, settledEx.ErrorMessage)
}

func TestExecuteHoldsFinalizingUntilReportAndRejectsOverlap(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(`<?xml version="1.0"?><rss version="2.0"><channel><title>Life</title>
<item><title>E1</title><guid>life-e1</guid><pubDate>Tue, 14 Jul 2026 08:00:00 GMT</pubDate><description>d</description></item>
</channel></rss>`))
	}))
	t.Cleanup(server.Close)

	db := setupLifecycleDB(t)
	coordinator := feed.NewCoordinator(feed.CoordinatorConfig{
		DomainPolicies: map[string]feed.DomainPolicy{feed.TargetDomain(server.URL): {MaxConcurrency: 1}},
	})
	service, err := syncsvc.NewServiceWithFeedCoordinator(db, "", coordinator)
	require.NoError(t, err)
	t.Cleanup(func() { _ = service.Close() })

	podcast := &models.Podcast{XYZID: "life-pod", Title: "Life", FeedURL: server.URL + "/feed.xml", IsSubscribed: true}
	require.NoError(t, db.Create(podcast).Error)
	wf := &models.Workflow{
		Name:        "life-wf",
		ScopeType:   models.ScopeTypeSpecificPodcasts,
		ScopeConfig: models.ScopeConfig{PodcastIDs: []int{int(podcast.ID)}},
		RulesConfig: models.RulesConfig{TimeRange: 1, TimeRangeMode: "days"},
		IsEnabled:   true,
	}
	require.NoError(t, db.Create(wf).Error)

	executor := NewExecutor(db, service, nil, nil)
	executor.UseInstantBatchClock()

	job, err := executor.Execute(context.Background(), wf, "manual")
	require.NoError(t, err)
	require.False(t, models.IsActiveJobStatus(job.Status), "lock must be released after report")
	require.Contains(t, []models.JobStatus{models.JobStatusCompleted, models.JobStatusPartial, models.JobStatusFailed}, job.Status)

	var reportCount int64
	require.NoError(t, db.Model(&models.Report{}).Where("job_id = ?", job.ID).Count(&reportCount).Error)
	require.Equal(t, int64(1), reportCount, "exactly one report")

	// Concurrent second Execute while first would still be active is rejected;
	// after terminal, a second run is allowed.
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	// Hold an artificial active job and ensure claim fails.
	active, err := ClaimActiveJob(db, wf.ID, "hold")
	require.NoError(t, err)
	_, err = executor.Execute(context.Background(), wf, "manual")
	require.ErrorIs(t, err, ErrWorkflowJobActive)
	active.Status = models.JobStatusFailed
	end := time.Now()
	active.EndTime = &end
	require.NoError(t, db.Save(active).Error)

	wg.Add(1)
	go func() {
		defer wg.Done()
		_, e := executor.Execute(context.Background(), wf, "manual")
		errs <- e
	}()
	wg.Wait()
	close(errs)
	for e := range errs {
		require.NoError(t, e)
	}

	// Idempotent report path: GenerateForJob twice yields one row.
	rg := NewReportGenerator(db, nil)
	_, err = rg.GenerateForJob(context.Background(), job)
	require.NoError(t, err)
	require.NoError(t, db.Model(&models.Report{}).Where("job_id = ?", job.ID).Count(&reportCount).Error)
	require.Equal(t, int64(1), reportCount)
}
