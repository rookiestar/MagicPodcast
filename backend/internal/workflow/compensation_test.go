package workflow

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"magicpodcast/internal/feed"
	"magicpodcast/internal/models"
	syncsvc "magicpodcast/internal/sync"

	"github.com/stretchr/testify/require"
)

func TestValidateCompensationSourceGuards(t *testing.T) {
	require.ErrorIs(t, ValidateCompensationSource(&models.Job{Status: models.JobStatusCompleted}), ErrCompensationNotAllowed)
	require.ErrorIs(t, ValidateCompensationSource(&models.Job{Status: models.JobStatusFailed}), ErrCompensationNotAllowed)
	require.ErrorIs(t, ValidateCompensationSource(&models.Job{Status: models.JobStatusRunning}), ErrCompensationNotAllowed)
	cid := uint(9)
	require.ErrorIs(t, ValidateCompensationSource(&models.Job{Status: models.JobStatusPartial, CompensatedByJobID: &cid}), ErrCompensationAlreadyExists)
	require.NoError(t, ValidateCompensationSource(&models.Job{Status: models.JobStatusPartial}))
}

func TestExecuteCompensationRetriesOnlyFailuresAndLinks(t *testing.T) {
	var hitsOK, hitsFail int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		switch r.URL.Path {
		case "/ok.xml":
			atomic.AddInt32(&hitsOK, 1)
			w.Header().Set("Content-Type", "application/rss+xml")
			_, _ = w.Write([]byte(batchRSS("OK", "ok-ep")))
		default:
			atomic.AddInt32(&hitsFail, 1)
			// Succeed on compensation (2nd+ hits for fail path after first batch).
			if atomic.LoadInt32(&hitsFail) >= 2 {
				w.Header().Set("Content-Type", "application/rss+xml")
				_, _ = w.Write([]byte(batchRSS("FailRecovered", "fail-ep")))
				return
			}
			w.WriteHeader(http.StatusForbidden)
		}
	}))
	t.Cleanup(server.Close)

	db := setupLifecycleDB(t)
	coordinator := feed.NewCoordinator(feed.CoordinatorConfig{
		DomainPolicies: map[string]feed.DomainPolicy{feed.TargetDomain(server.URL): {MaxConcurrency: 1}},
	})
	service, err := syncsvc.NewServiceWithFeedCoordinator(db, "", coordinator)
	require.NoError(t, err)
	t.Cleanup(func() { _ = service.Close() })

	okPod := models.Podcast{XYZID: "comp-ok", Title: "OK", FeedURL: server.URL + "/ok.xml", IsSubscribed: true}
	failPod := models.Podcast{XYZID: "comp-fail", Title: "Fail", FeedURL: server.URL + "/fail.xml", IsSubscribed: true}
	require.NoError(t, db.Create(&okPod).Error)
	require.NoError(t, db.Create(&failPod).Error)

	wf := &models.Workflow{
		Name:        "comp-wf",
		ScopeType:   models.ScopeTypeSpecificPodcasts,
		ScopeConfig: models.ScopeConfig{PodcastIDs: []int{int(okPod.ID), int(failPod.ID)}},
		RulesConfig: models.RulesConfig{TimeRange: 0},
		IsEnabled:   true,
	}
	require.NoError(t, db.Create(wf).Error)

	executor := NewExecutor(db, service, nil, nil)
	executor.UseInstantBatchClock()
	executor.workerConcurrency = 1

	// Force first batch to partial without waiting retries: sleep jumps past deadline.
	// UseInstantBatchClock advances on sleep; access_denied would retry — pin batch sleep to deadline.
	// Already UseInstantBatchClock advances by requested duration; after first-pass failures it may retry.
	// For determinism set batchDuration and jump sleep to deadline after first waits.
	// Simpler: create a partial job manually, then compensate.
	source := &models.Job{WorkflowID: wf.ID, Status: models.JobStatusPartial, TriggeredBy: "manual", PodcastsProcessed: 2, ErrorCount: 1}
	require.NoError(t, db.Create(source).Error)
	require.NoError(t, db.Create(&models.JobExecution{
		JobID: source.ID, PodcastID: &okPod.ID, PodcastTitle: "OK", Status: models.ExecutionStatusSuccess,
	}).Error)
	require.NoError(t, db.Create(&models.JobExecution{
		JobID: source.ID, PodcastID: &failPod.ID, PodcastTitle: "Fail", Status: models.ExecutionStatusFailed,
		FeedErrorCategory: string(feed.ErrorCategoryAccessDenied),
	}).Error)
	// Seed a report that must not be overwritten.
	require.NoError(t, db.Create(&models.Report{JobID: source.ID, Title: "orig", Content: "original report"}).Error)

	comp, err := executor.ExecuteCompensation(context.Background(), source.ID)
	require.NoError(t, err)
	require.NotNil(t, comp)
	require.Equal(t, source.ID, *comp.CompensationOfJobID)

	var refreshed models.Job
	require.NoError(t, db.First(&refreshed, source.ID).Error)
	require.Equal(t, models.JobStatusPartial, refreshed.Status, "original status unchanged")
	require.NotNil(t, refreshed.CompensatedByJobID)
	require.Equal(t, comp.ID, *refreshed.CompensatedByJobID)

	var report models.Report
	require.NoError(t, db.Where("job_id = ?", source.ID).First(&report).Error)
	require.Equal(t, "original report", report.Content)

	// Compensation only targeted the failed podcast.
	var compExecs []models.JobExecution
	require.NoError(t, db.Where("job_id = ?", comp.ID).Find(&compExecs).Error)
	require.Len(t, compExecs, 1)
	require.Equal(t, failPod.ID, *compExecs[0].PodcastID)

	// Duplicate compensation rejected.
	_, err = executor.ExecuteCompensation(context.Background(), source.ID)
	require.ErrorIs(t, err, ErrCompensationAlreadyExists)
}
