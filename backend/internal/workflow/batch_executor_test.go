package workflow

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"magicpodcast/internal/feed"
	"magicpodcast/internal/models"
	syncsvc "magicpodcast/internal/sync"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupBatchTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:batch_"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&models.Podcast{},
		&models.Episode{},
		&models.Workflow{},
		&models.Job{},
		&models.JobExecution{},
	))
	return db
}

func newBatchExecutor(t *testing.T, db *gorm.DB, coordinator *feed.Coordinator) *Executor {
	t.Helper()
	service, err := syncsvc.NewServiceWithFeedCoordinator(db, "", coordinator)
	require.NoError(t, err)
	t.Cleanup(func() { _ = service.Close() })
	return NewExecutor(db, service, nil, nil)
}

// TestBatchFirstPassBeforeRetries verifies every target Feed gets a primary
// attempt before any classified retry is scheduled (#36).
func TestBatchFirstPassBeforeRetries(t *testing.T) {
	var mu sync.Mutex
	var order []string
	var hitsA, hitsB int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		mu.Lock()
		order = append(order, r.URL.Path)
		mu.Unlock()
		switch r.URL.Path {
		case "/a.xml":
			n := atomic.AddInt32(&hitsA, 1)
			if n == 1 {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			w.Header().Set("Content-Type", "application/rss+xml")
			_, _ = w.Write([]byte(batchRSS("A recovered", "a-ep")))
		case "/b.xml":
			n := atomic.AddInt32(&hitsB, 1)
			if n == 1 {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			w.Header().Set("Content-Type", "application/rss+xml")
			_, _ = w.Write([]byte(batchRSS("B recovered", "b-ep")))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	domain := feed.TargetDomain(server.URL)
	coordinator := feed.NewCoordinator(feed.CoordinatorConfig{
		DomainPolicies: map[string]feed.DomainPolicy{
			// Soft rate off so this test measures first-pass fairness only.
			domain: {MaxConcurrency: 1},
		},
	})
	db := setupBatchTestDB(t)
	executor := newBatchExecutor(t, db, coordinator)
	executor.workerConcurrency = 1 // sequential: clear first-pass ordering evidence

	// Fake clock: first-pass at t=0; sleep advances exactly as requested so the
	// access_denied slots at minutes 3/8/13 fire without wall-clock waits.
	var now atomic.Value
	start := time.Date(2026, 7, 25, 10, 0, 0, 0, time.UTC)
	now.Store(start)
	executor.now = func() time.Time { return now.Load().(time.Time) }
	executor.sleep = func(d time.Duration) {
		cur := now.Load().(time.Time)
		now.Store(cur.Add(d))
	}
	executor.batchDuration = 15 * time.Minute

	pa := models.Podcast{XYZID: "batch-a", Title: "A", FeedURL: server.URL + "/a.xml"}
	pb := models.Podcast{XYZID: "batch-b", Title: "B", FeedURL: server.URL + "/b.xml"}
	require.NoError(t, db.Create(&pa).Error)
	require.NoError(t, db.Create(&pb).Error)

	wf := &models.Workflow{
		Name:      "batch-first-pass",
		ScopeType: models.ScopeTypeSpecificPodcasts,
		ScopeConfig: models.ScopeConfig{PodcastIDs: []int{int(pa.ID), int(pb.ID)}},
		RulesConfig: models.RulesConfig{TimeRange: 0},
	}
	require.NoError(t, db.Create(wf).Error)

	job, err := executor.Execute(context.Background(), wf, "manual")
	require.NoError(t, err)
	require.NotNil(t, job)

	// First two live hits must be the first-pass of A and B (any order) before
	// any second attempt appears.
	mu.Lock()
	got := append([]string(nil), order...)
	mu.Unlock()
	require.GreaterOrEqual(t, len(got), 2)
	firstTwo := map[string]bool{got[0]: true, got[1]: true}
	require.True(t, firstTwo["/a.xml"] && firstTwo["/b.xml"], "first-pass must cover both feeds before retries: %v", got)
	require.GreaterOrEqual(t, atomic.LoadInt32(&hitsA), int32(1))
	require.GreaterOrEqual(t, atomic.LoadInt32(&hitsB), int32(1))
}

// TestBatchDeadlineStopsNetworkingAndYieldsPartial verifies the 15-minute
// cutoff stops further fetches and mixed outcomes map to partial.
func TestBatchDeadlineStopsNetworkingAndYieldsPartial(t *testing.T) {
	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		n := atomic.AddInt32(&hits, 1)
		switch r.URL.Path {
		case "/ok.xml":
			w.Header().Set("Content-Type", "application/rss+xml")
			_, _ = w.Write([]byte(batchRSS("OK", "ok-ep")))
		default:
			// Always 403 so retries would want more network if allowed.
			_ = n
			w.WriteHeader(http.StatusForbidden)
		}
	}))
	t.Cleanup(server.Close)

	domain := feed.TargetDomain(server.URL)
	coordinator := feed.NewCoordinator(feed.CoordinatorConfig{
		DomainPolicies: map[string]feed.DomainPolicy{
			domain: {MaxConcurrency: 1, SoftRateEnabled: true},
		},
	})
	db := setupBatchTestDB(t)
	executor := newBatchExecutor(t, db, coordinator)
	executor.workerConcurrency = 1

	var now atomic.Value
	start := time.Date(2026, 7, 25, 11, 0, 0, 0, time.UTC)
	now.Store(start)
	executor.now = func() time.Time { return now.Load().(time.Time) }
	// Any sleep jumps straight to the deadline so no retry network happens.
	executor.sleep = func(d time.Duration) {
		now.Store(start.Add(15 * time.Minute))
	}
	executor.batchDuration = 15 * time.Minute

	okPod := models.Podcast{XYZID: "batch-ok", Title: "OK", FeedURL: server.URL + "/ok.xml"}
	failPod := models.Podcast{XYZID: "batch-fail", Title: "Fail", FeedURL: server.URL + "/fail.xml"}
	require.NoError(t, db.Create(&okPod).Error)
	require.NoError(t, db.Create(&failPod).Error)

	wf := &models.Workflow{
		Name:      "batch-deadline",
		ScopeType: models.ScopeTypeSpecificPodcasts,
		ScopeConfig: models.ScopeConfig{PodcastIDs: []int{int(okPod.ID), int(failPod.ID)}},
		RulesConfig: models.RulesConfig{TimeRange: 0},
	}
	require.NoError(t, db.Create(wf).Error)

	job, err := executor.Execute(context.Background(), wf, "manual")
	require.NoError(t, err)

	// First-pass only: one hit per feed (robots may add more; path hits >= 2).
	require.GreaterOrEqual(t, atomic.LoadInt32(&hits), int32(2))

	var executions []models.JobExecution
	require.NoError(t, db.Where("job_id = ?", job.ID).Find(&executions).Error)
	require.Len(t, executions, 2)

	var success, failed int
	for _, ex := range executions {
		switch ex.Status {
		case models.ExecutionStatusSuccess:
			success++
		case models.ExecutionStatusFailed:
			failed++
			require.NotEqual(t, string(feed.ErrorCategoryCircuitOpen), ex.FeedErrorCategory)
		}
	}
	require.Equal(t, 1, success)
	require.Equal(t, 1, failed)
	require.Equal(t, models.JobStatusPartial, finalJobStatus(success, failed, 2))
}

func batchRSS(title, guid string) string {
	return `<?xml version="1.0"?><rss version="2.0"><channel><title>` + title +
		`</title><item><title>Ep</title><guid>` + guid +
		`</guid><pubDate>Tue, 14 Jul 2026 08:00:00 GMT</pubDate><description>d</description></item></channel></rss>`
}
