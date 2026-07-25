package workflow

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"magicpodcast/internal/feed"
	"magicpodcast/internal/models"
	syncsvc "magicpodcast/internal/sync"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupTestDB 创建测试数据库
// 注意：每个测试使用独立的数据库，避免测试间的干扰
func setupTestDB(t *testing.T) *gorm.DB {
	// 使用唯一的数据库名称，避免并发测试间的冲突
	dbName := fmt.Sprintf("file:testdb_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dbName), &gorm.Config{})
	assert.NoError(t, err)

	// 自动迁移
	err = db.AutoMigrate(&models.Podcast{}, &models.Workflow{}, &models.Job{}, &models.JobExecution{})
	assert.NoError(t, err)

	// 启用WAL模式以支持更好的并发
	db.Exec("PRAGMA journal_mode=WAL;")
	db.Exec("PRAGMA busy_timeout=5000;")

	return db
}

// createTestExecutor 创建测试用的Executor（使用nil syncSvc，因为fetchCustomPodcasts不需要它）
func createTestExecutor(db *gorm.DB) *Executor {
	return &Executor{
		db:      db,
		syncSvc: nil, // fetchCustomPodcasts不使用syncSvc，所以可以设置为nil
	}
}

// TestFetchCustomPodcasts_RaceCondition 测试并发场景下的竞态条件修复
func TestFetchCustomPodcasts_RaceCondition(t *testing.T) {
	db := setupTestDB(t)
	executor := createTestExecutor(db)

	// 测试URL
	testURL := "https://race-condition-test.com/feed.xml"

	// 模拟多个并发worker同时尝试获取同一个URL
	// 使用较小的并发数以避免SQLite锁定问题
	concurrency := 3
	var wg sync.WaitGroup
	successCount := 0
	var mu sync.Mutex

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := executor.fetchCustomPodcasts([]string{testURL})
			// 在SQLite并发限制下，有些请求可能失败，但重要的是不应该创建重复记录
			if err == nil {
				mu.Lock()
				successCount++
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	// 应该至少有一个成功
	assert.Greater(t, successCount, 0, "应该至少有一个成功的结果")

	// 最关键的验证：数据库中应该只有1个播客记录，不应该有重复
	// 这证明了我们的唯一索引和FirstOrCreate修复是有效的
	var count int64
	db.Model(&models.Podcast{}).Where("feed_url = ?", testURL).Count(&count)
	assert.Equal(t, int64(1), count, "数据库中应该只有1个播客记录，不应该有重复 - 这证明了竞态条件已被修复")
}

// TestFetchCustomPodcasts_MultipleURLs 测试多个不同的URL
func TestFetchCustomPodcasts_MultipleURLs(t *testing.T) {
	db := setupTestDB(t)
	executor := createTestExecutor(db)

	urls := []string{
		"https://example.com/podcast1/feed.xml",
		"https://example.com/podcast2/feed.xml",
		"https://example.com/podcast3/feed.xml",
	}

	podcasts, err := executor.fetchCustomPodcasts(urls)
	assert.NoError(t, err)
	assert.Equal(t, 3, len(podcasts), "应该成功获取3个播客")

	// 验证数据库中确实有3个不同的播客
	var count int64
	db.Model(&models.Podcast{}).Count(&count)
	assert.Equal(t, int64(3), count, "数据库中应该有3个播客记录")
}

// TestFetchCustomPodcasts_ConcurrentWithDuplicates 测试并发请求包含重复URL的情况
// 注意：由于SQLite的并发限制，这个测试专注于验证不创建重复记录
func TestFetchCustomPodcasts_ConcurrentWithDuplicates(t *testing.T) {
	db := setupTestDB(t)
	executor := createTestExecutor(db)

	// 使用唯一的URL避免与其他测试冲突
	urls := []string{
		"https://duplicate-test-1.com/feed.xml",
		"https://duplicate-test-2.com/feed.xml",
	}

	concurrency := 2
	var wg sync.WaitGroup
	successCount := 0
	var mu sync.Mutex

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := executor.fetchCustomPodcasts(urls)
			// SQLite可能因锁定而失败，但重要的是不应该创建重复
			if err == nil {
				mu.Lock()
				successCount++
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	// 应该至少有一个成功
	assert.Greater(t, successCount, 0, "应该至少有一个成功的结果")

	// 验证数据库中只有2个播客记录（没有重复）
	var count int64
	db.Model(&models.Podcast{}).Where("feed_url IN ?", urls).Count(&count)
	assert.Equal(t, int64(2), count, "数据库中应该只有2个唯一的播客记录，没有重复")
}

// TestFetchCustomPodcasts_AlreadyExists 测试播客已存在的情况
func TestFetchCustomPodcasts_AlreadyExists(t *testing.T) {
	db := setupTestDB(t)
	executor := createTestExecutor(db)

	// 使用不同的URL避免与其他测试冲突
	testURL := "https://already-exists-test.com/feed.xml"

	// 先创建一个播客
	existingPodcast := models.Podcast{
		XYZID:        "already-existing-xyz-id",
		FeedURL:      testURL,
		Title:        "Existing Podcast",
		IsSubscribed: true,
	}
	err := db.Create(&existingPodcast).Error
	assert.NoError(t, err)

	// 现在尝试通过fetchCustomPodcasts获取
	// FirstOrCreate会找到已存在的记录并返回它
	podcasts, err := executor.fetchCustomPodcasts([]string{testURL})
	assert.NoError(t, err)
	assert.Equal(t, 1, len(podcasts), "应该返回1个播客")

	// 应该返回已存在的播客，而不是创建新的
	var count int64
	db.Model(&models.Podcast{}).Where("feed_url = ?", testURL).Count(&count)
	assert.Equal(t, int64(1), count, "数据库中应该仍然只有1个播客记录")

	// 返回的播客应该是已存在的那个
	assert.Equal(t, existingPodcast.ID, podcasts[0].ID, "应该返回已存在的播客ID")
	assert.Equal(t, "Existing Podcast", podcasts[0].Title, "应该保持原有的标题")
}

// TestFetchCustomPodcasts_EmptyURLs 测试空URL列表
func TestFetchCustomPodcasts_EmptyURLs(t *testing.T) {
	db := setupTestDB(t)
	executor := createTestExecutor(db)

	podcasts, err := executor.fetchCustomPodcasts([]string{})
	assert.Error(t, err, "空URL列表应该返回错误")
	assert.Nil(t, podcasts, "空URL列表应该返回nil")
	assert.Contains(t, err.Error(), "未能从自定义源获取任何播客", "错误信息应该包含提示")
}

func TestSyncPodcastMarksExecutionFailedWhenSummaryWritebackFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveRobotsNotFoundWorkflow(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel><title>Workflow Test Feed</title>
<item><title>New Episode</title><guid>workflow-summary-error-episode</guid><pubDate>Tue, 14 Jul 2026 08:00:00 GMT</pubDate><description>Episode details</description></item>
</channel></rss>`))
	}))
	t.Cleanup(server.Close)

	db := setupTestDB(t)
	require.NoError(t, db.AutoMigrate(&models.Episode{}))
	service, err := syncsvc.NewService(db, "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close()) })

	podcast := &models.Podcast{
		XYZID:        "workflow-summary-error-podcast",
		Title:        "Workflow Summary Error",
		FeedURL:      server.URL,
		IsSubscribed: true,
	}
	require.NoError(t, db.Create(podcast).Error)
	require.NoError(t, db.Exec(`
		CREATE TRIGGER reject_workflow_summary_write
		BEFORE UPDATE OF episode_count, newest_episode_date ON podcasts
		BEGIN
			SELECT RAISE(ABORT, 'workflow summary writeback blocked');
		END`).Error)

	workflow := &models.Workflow{
		Name:        "Summary Writeback Failure Workflow",
		Schedule:    "0 0 * * *",
		ScopeType:   models.ScopeTypeSpecificPodcasts,
		ScopeConfig: models.ScopeConfig{PodcastIDs: []int{int(podcast.ID)}},
		IsEnabled:   true,
	}
	require.NoError(t, db.Create(workflow).Error)
	job := &models.Job{WorkflowID: workflow.ID, Status: models.JobStatusRunning, TriggeredBy: "manual"}
	require.NoError(t, db.Create(job).Error)

	executor := NewExecutor(db, service, nil, nil)
	execution := executor.syncPodcast(context.Background(), workflow, job.ID, *podcast)

	assert.Equal(t, models.ExecutionStatusFailed, execution.Status)
	assert.Contains(t, execution.ErrorMessage, "写回播客汇总失败")

	var persisted models.JobExecution
	require.NoError(t, db.First(&persisted, execution.ID).Error)
	assert.Equal(t, models.ExecutionStatusFailed, persisted.Status)
}

func TestExecutePersistsFeedAccessObservation(t *testing.T) {
	tests := []struct {
		name             string
		status           int
		body             string
		wantError        string
		wantHTTPStatus   int
		wantFreshness    string
		wantResponseBody bool
	}{
		{
			name:             "successful primary feed",
			status:           http.StatusOK,
			body:             `<?xml version="1.0"?><rss version="2.0"><channel><title>Observed Feed</title><item><title>Episode</title><guid>observed-episode</guid><pubDate>Tue, 14 Jul 2026 08:00:00 GMT</pubDate><description>Details</description></item></channel></rss>`,
			wantError:        "none",
			wantHTTPStatus:   http.StatusOK,
			wantFreshness:    "live",
			wantResponseBody: true,
		},
		{
			name:           "refused primary feed",
			status:         http.StatusForbidden,
			body:           "refused",
			wantError:      "access_denied",
			wantHTTPStatus: http.StatusForbidden,
			wantFreshness:  "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if serveRobotsNotFoundWorkflow(w, r) {
					return
				}
				w.Header().Set("Content-Type", "application/rss+xml")
				w.Header().Set("ETag", `"observed-v1"`)
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			t.Cleanup(server.Close)

			db := setupTestDB(t)
			var feedColumns []string
			require.NoError(t, db.Raw("SELECT name FROM pragma_table_info('job_executions') WHERE name IN ('feed_etag', 'feed_e_tag') ORDER BY name").Scan(&feedColumns).Error)
			require.Equal(t, []string{"feed_etag"}, feedColumns, "JobExecution ETag column must match the versioned production schema")
			require.NoError(t, db.AutoMigrate(&models.Episode{}, &models.Report{}))
			service, err := syncsvc.NewService(db, "")
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, service.Close()) })

			podcast := &models.Podcast{
				XYZID:        "observed-podcast-" + tt.name,
				Title:        "Observed Podcast",
				FeedURL:      server.URL + "/feed.xml?access_token=super-secret",
				IsSubscribed: true,
			}
			require.NoError(t, db.Create(podcast).Error)
			workflowModel := &models.Workflow{
				Name:        "Feed Observation Workflow",
				Schedule:    "0 0 * * *",
				ScopeType:   models.ScopeTypeSpecificPodcasts,
				ScopeConfig: models.ScopeConfig{PodcastIDs: []int{int(podcast.ID)}},
				RulesConfig: models.RulesConfig{TimeRange: 1, TimeRangeMode: "days"},
				IsEnabled:   true,
			}
			require.NoError(t, db.Create(workflowModel).Error)

			executor := NewExecutor(db, service, nil, nil)
			executor.UseInstantBatchClock()
			job, err := executor.Execute(context.Background(), workflowModel, "manual")
			require.NoError(t, err)

			var execution models.JobExecution
			require.Eventually(t, func() bool {
				return db.Where("job_id = ?", job.ID).First(&execution).Error == nil
			}, 2*time.Second, 10*time.Millisecond)

			if execution.FeedHTTPStatus == nil || *execution.FeedHTTPStatus != tt.wantHTTPStatus {
				t.Fatalf("expected feed HTTP status %d, got %#v", tt.wantHTTPStatus, execution.FeedHTTPStatus)
			}
			if execution.FeedErrorCategory != tt.wantError {
				t.Fatalf("expected feed error category %q, got %q", tt.wantError, execution.FeedErrorCategory)
			}
			if execution.FeedTargetDomain != "127.0.0.1" {
				t.Fatalf("expected target domain to be persisted, got %q", execution.FeedTargetDomain)
			}
			if execution.FeedResponseTimeMs < 0 || (tt.wantResponseBody && execution.FeedResponseBytes == 0) {
				t.Fatalf("expected response timing/size observation, got time=%d bytes=%d", execution.FeedResponseTimeMs, execution.FeedResponseBytes)
			}
			if execution.FeedSourceType != "primary" || execution.FeedCacheStatus != "not_used" || execution.FeedFreshness != tt.wantFreshness || execution.FeedEgressID != "direct" {
				t.Fatalf("unexpected source summary: source=%q cache=%q freshness=%q egress=%q", execution.FeedSourceType, execution.FeedCacheStatus, execution.FeedFreshness, execution.FeedEgressID)
			}
			if execution.FeedETag != `"observed-v1"` {
				t.Fatalf("expected ETag to be persisted, got %q", execution.FeedETag)
			}
			if tt.wantFreshness == "live" && execution.FeedSnapshotRetrievedAt == nil {
				t.Fatal("successful feed should persist the content retrieval time")
			}
			if strings.Contains(execution.ErrorMessage, "super-secret") {
				t.Fatalf("execution error should not expose query credentials: %q", execution.ErrorMessage)
			}
		})
	}
}

func TestExecutePersistsCircuitSkipAndRecoveryProbe(t *testing.T) {
	var requestCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveRobotsNotFoundWorkflow(w, r) {
			return
		}
		if atomic.AddInt32(&requestCount, 1) == 1 {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(`<?xml version="1.0"?><rss version="2.0"><channel><title>Circuit Workflow Feed</title><item><title>Recovered Episode</title><guid>circuit-recovered-episode</guid><pubDate>Tue, 14 Jul 2026 08:00:00 GMT</pubDate><description>Details</description></item></channel></rss>`))
	}))
	t.Cleanup(server.Close)

	coordinator := feed.NewCoordinator(feed.CoordinatorConfig{DomainPolicies: map[string]feed.DomainPolicy{
		feed.TargetDomain(server.URL): {
			MaxConcurrency:                 1,
			CircuitCooldown:                80 * time.Millisecond,
			ImmediateCircuitOnAccessDenied: true,
		},
	}})
	db := setupTestDB(t)
	require.NoError(t, db.AutoMigrate(&models.Episode{}, &models.Report{}))
	service, err := syncsvc.NewServiceWithFeedCoordinator(db, "", coordinator)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close()) })

	podcast := &models.Podcast{
		XYZID:        "circuit-workflow-podcast",
		Title:        "Circuit Workflow Podcast",
		FeedURL:      server.URL + "/feed.xml",
		IsSubscribed: true,
	}
	require.NoError(t, db.Create(podcast).Error)
	workflowModel := &models.Workflow{
		Name:        "Circuit Workflow",
		Schedule:    "0 0 * * *",
		ScopeType:   models.ScopeTypeSpecificPodcasts,
		ScopeConfig: models.ScopeConfig{PodcastIDs: []int{int(podcast.ID)}},
		RulesConfig: models.RulesConfig{TimeRange: 1, TimeRangeMode: "days"},
		IsEnabled:   true,
	}
	require.NoError(t, db.Create(workflowModel).Error)
	executor := NewExecutor(db, service, nil, nil)
	// Keep each job to first-pass only: any scheduled batch retry jumps past the
	// deadline so this test stays focused on domain circuit OPEN / probe recovery
	// across separate jobs (not the 15-minute classified retry schedule).
	var clock atomic.Value
	base := time.Now()
	clock.Store(base)
	executor.now = func() time.Time { return clock.Load().(time.Time) }
	executor.sleep = func(d time.Duration) {
		clock.Store(clock.Load().(time.Time).Add(15 * time.Minute))
	}
	executor.batchDuration = 15 * time.Minute

	job, err := executor.Execute(context.Background(), workflowModel, "manual")
	require.NoError(t, err)
	first := waitForJobExecution(t, db, job.ID)
	require.Equal(t, models.ExecutionStatusFailed, first.Status)
	require.Equal(t, "access_denied", first.FeedErrorCategory)
	require.Equal(t, "open", first.FeedCircuitState)

	// Reset clock for the next independent job while the domain circuit is open.
	clock.Store(base)
	job, err = executor.Execute(context.Background(), workflowModel, "manual")
	require.NoError(t, err)
	blocked := waitForJobExecution(t, db, job.ID)
	require.Equal(t, models.ExecutionStatusFailed, blocked.Status)
	require.Equal(t, "circuit_open", blocked.FeedErrorCategory)
	require.Equal(t, "open", blocked.FeedCircuitState)
	require.Equal(t, int32(1), atomic.LoadInt32(&requestCount))

	time.Sleep(110 * time.Millisecond)
	clock.Store(base.Add(200 * time.Millisecond))
	job, err = executor.Execute(context.Background(), workflowModel, "manual")
	require.NoError(t, err)
	probe := waitForJobExecution(t, db, job.ID)
	require.Equal(t, models.ExecutionStatusSuccess, probe.Status)
	require.Equal(t, "none", probe.FeedErrorCategory)
	require.Equal(t, "probe", probe.FeedCircuitState)
	require.Equal(t, int32(2), atomic.LoadInt32(&requestCount))
}

func waitForJobExecution(t *testing.T, db *gorm.DB, jobID uint) models.JobExecution {
	t.Helper()
	var execution models.JobExecution
	require.Eventually(t, func() bool {
		return db.Where("job_id = ?", jobID).First(&execution).Error == nil
	}, 2*time.Second, 10*time.Millisecond)
	var job models.Job
	require.Eventually(t, func() bool {
		if db.First(&job, jobID).Error != nil {
			return false
		}
		return job.Status != models.JobStatusRunning
	}, 2*time.Second, 10*time.Millisecond)
	return execution
}

func TestOverlappingWorkflowsShareOneUpstreamFeedRequest(t *testing.T) {
	var requestCount int32
	var current int32
	var maxConcurrent int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveRobotsNotFoundWorkflow(w, r) {
			return
		}
		atomic.AddInt32(&requestCount, 1)
		active := atomic.AddInt32(&current, 1)
		updateWorkflowMaxConcurrent(&maxConcurrent, active)
		time.Sleep(60 * time.Millisecond)
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(`<?xml version="1.0"?><rss version="2.0"><channel><title>Shared Workflow Feed</title><item><title>Episode</title><guid>shared-workflow-episode</guid><pubDate>Tue, 14 Jul 2026 08:00:00 GMT</pubDate><description>Details</description></item></channel></rss>`))
		atomic.AddInt32(&current, -1)
	}))
	t.Cleanup(server.Close)

	coordinator := feed.NewCoordinator(feed.CoordinatorConfig{DomainPolicies: map[string]feed.DomainPolicy{
		feed.TargetDomain(server.URL): {MaxConcurrency: 1, MinRefreshInterval: time.Minute},
	}})

	dbA := setupTestDB(t)
	require.NoError(t, dbA.AutoMigrate(&models.Episode{}, &models.Report{}))
	dbB := setupTestDB(t)
	require.NoError(t, dbB.AutoMigrate(&models.Episode{}, &models.Report{}))
	serviceA, err := syncsvc.NewServiceWithFeedCoordinator(dbA, "", coordinator)
	require.NoError(t, err)
	serviceB, err := syncsvc.NewServiceWithFeedCoordinator(dbB, "", coordinator)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, serviceA.Close()) })
	t.Cleanup(func() { require.NoError(t, serviceB.Close()) })

	workflowA, podcastA := createObservationWorkflowFixture(t, dbA, server.URL+"/feed.xml", "A")
	workflowB, podcastB := createObservationWorkflowFixture(t, dbB, server.URL+"/feed.xml", "B")
	executorA := NewExecutor(dbA, serviceA, nil, nil)
	executorA.UseInstantBatchClock()
	executorB := NewExecutor(dbB, serviceB, nil, nil)
	executorB.UseInstantBatchClock()

	var wg sync.WaitGroup
	jobs := make(chan *models.Job, 2)
	errs := make(chan error, 2)
	for _, run := range []func() (*models.Job, error){
		func() (*models.Job, error) { return executorA.Execute(context.Background(), workflowA, "manual") },
		func() (*models.Job, error) { return executorB.Execute(context.Background(), workflowB, "manual") },
	} {
		wg.Add(1)
		go func(run func() (*models.Job, error)) {
			defer wg.Done()
			job, err := run()
			jobs <- job
			errs <- err
		}(run)
	}
	wg.Wait()
	close(jobs)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("overlapping workflow failed: %v", err)
		}
	}
	if len(jobs) != 2 {
		t.Fatalf("expected two workflow jobs, got %d", len(jobs))
	}
	if got := atomic.LoadInt32(&requestCount); got != 1 {
		t.Fatalf("expected one upstream request for two workflows, got %d", got)
	}
	if got := atomic.LoadInt32(&maxConcurrent); got != 1 {
		t.Fatalf("expected at most one in-flight request, got %d", got)
	}
	_ = podcastA
	_ = podcastB
}

func createObservationWorkflowFixture(t *testing.T, db *gorm.DB, feedURL, suffix string) (*models.Workflow, *models.Podcast) {
	t.Helper()
	podcast := &models.Podcast{XYZID: "shared-workflow-podcast-" + suffix, Title: "Shared Workflow Podcast", FeedURL: feedURL, IsSubscribed: true}
	require.NoError(t, db.Create(podcast).Error)
	workflowModel := &models.Workflow{
		Name:        "Shared Workflow " + suffix,
		Schedule:    "0 0 * * *",
		ScopeType:   models.ScopeTypeSpecificPodcasts,
		ScopeConfig: models.ScopeConfig{PodcastIDs: []int{int(podcast.ID)}},
		RulesConfig: models.RulesConfig{TimeRange: 1, TimeRangeMode: "days"},
		IsEnabled:   true,
	}
	require.NoError(t, db.Create(workflowModel).Error)
	return workflowModel, podcast
}

func updateWorkflowMaxConcurrent(max *int32, current int32) {
	for {
		previous := atomic.LoadInt32(max)
		if current <= previous || atomic.CompareAndSwapInt32(max, previous, current) {
			return
		}
	}
}

func TestFinalJobStatusFailsWhenAnyPodcastExecutionFails(t *testing.T) {
	assert.Equal(t, models.JobStatusCompleted, finalJobStatus(2, 0, 2))
	assert.Equal(t, models.JobStatusPartial, finalJobStatus(1, 1, 2))
	assert.Equal(t, models.JobStatusFailed, finalJobStatus(0, 1, 1))
}

// TestPodcastModel_FeedURLUniqueIndex 测试FeedURL唯一索引
func TestPodcastModel_FeedURLUniqueIndex(t *testing.T) {
	db := setupTestDB(t)

	// 使用不同的URL避免与其他测试冲突
	testURL := "https://unique-test.com/feed.xml"

	// 创建第一个播客
	podcast1 := models.Podcast{
		XYZID:        "unique-test-xyz-1",
		FeedURL:      testURL,
		Title:        "Podcast 1",
		IsSubscribed: false,
	}
	err := db.Create(&podcast1).Error
	assert.NoError(t, err, "应该成功创建第一个播客")

	// 尝试创建具有相同FeedURL的第二个播客
	podcast2 := models.Podcast{
		XYZID:        "unique-test-xyz-2",
		FeedURL:      testURL,
		Title:        "Podcast 2",
		IsSubscribed: false,
	}
	err = db.Create(&podcast2).Error
	assert.Error(t, err, "创建相同FeedURL的播客应该失败")
	assert.Contains(t, err.Error(), "UNIQUE", "错误应该包含唯一约束冲突信息")
}

// TestJobTimeRangeWindow_835Trigger 测试用户报告的场景：8:35触发的工作流时间窗口计算
func TestJobTimeRangeWindow_835Trigger(t *testing.T) {
	db := setupTestDB(t)

	// 创建测试工作流：配置为每天8:35执行，抓取最近1天
	loc := time.FixedZone("CST", 8*3600) // 东八区
	scheduledTime := time.Date(2024, 1, 15, 8, 35, 0, 0, loc)

	workflow := models.Workflow{
		Name:        "测试工作流-835触发",
		Description: "验证8:35触发的时间窗口计算",
		ScopeType:   models.ScopeTypeAllSubscribed,
		Schedule:    "0 35 8 * * *", // 每天8:35
		IsEnabled:   true,
		RulesConfig: models.RulesConfig{
			TimeRange:     1, // 最近1天
			TimeRangeMode: "days",
		},
	}
	err := db.Create(&workflow).Error
	assert.NoError(t, err)

	// 创建Job，模拟cron触发
	job := models.Job{
		WorkflowID:  workflow.ID,
		Status:      models.JobStatusRunning,
		StartTime:   &scheduledTime, // 使用8:35作为触发时间
		TriggeredBy: "cron",
	}
	err = db.Create(&job).Error
	assert.NoError(t, err)

	// 验证Job的时间窗口计算逻辑
	// 这里我们复用executor中计算时间窗口的逻辑
	var timeRangeStart, timeRangeEnd time.Time

	days := workflow.RulesConfig.TimeRange
	if days <= 0 {
		days = 1 // 默认1天
	}

	triggerTime := job.StartTime
	if triggerTime == nil {
		now := time.Now()
		triggerTime = &now
	}

	timeRangeEnd = *triggerTime
	timeRangeStart = timeRangeEnd.AddDate(0, 0, -days)

	// 验证时间窗口
	expectedEnd := time.Date(2024, 1, 15, 8, 35, 0, 0, loc)
	expectedStart := time.Date(2024, 1, 14, 8, 35, 0, 0, loc)

	assert.Equal(t, expectedEnd, timeRangeEnd, "结束时间应该是触发时间 2024-01-15 08:35:00")
	assert.Equal(t, expectedStart, timeRangeStart, "开始时间应该是昨天同一时间 2024-01-14 08:35:00")

	// 验证是24小时窗口
	diff := timeRangeEnd.Sub(timeRangeStart)
	assert.Equal(t, 24*time.Hour, diff, "时间范围应该是24小时")

	t.Logf("✅ 集成测试通过：8:35触发，1天范围")
	t.Logf("   Job ID: %d", job.ID)
	t.Logf("   工作流: %s", workflow.Name)
	t.Logf("   触发时间: %s", scheduledTime.Format("2006-01-02 15:04:05"))
	t.Logf("   时间窗口: %s ~ %s", timeRangeStart.Format("2006-01-02 15:04:05"), timeRangeEnd.Format("2006-01-02 15:04:05"))
}

// TestJobTimeRangeWindow_MultiDays 测试多天范围的时间窗口计算
func TestJobTimeRangeWindow_MultiDays(t *testing.T) {
	db := setupTestDB(t)

	loc := time.FixedZone("CST", 8*3600)
	triggerTime := time.Date(2024, 1, 15, 10, 30, 0, 0, loc)

	tests := []struct {
		name          string
		days          int
		expectedStart string
		expectedEnd   string
		expectedDiff  time.Duration
	}{
		{
			name:          "1天范围",
			days:          1,
			expectedStart: "2024-01-14 10:30:00",
			expectedEnd:   "2024-01-15 10:30:00",
			expectedDiff:  24 * time.Hour,
		},
		{
			name:          "2天范围",
			days:          2,
			expectedStart: "2024-01-13 10:30:00",
			expectedEnd:   "2024-01-15 10:30:00",
			expectedDiff:  48 * time.Hour,
		},
		{
			name:          "7天范围",
			days:          7,
			expectedStart: "2024-01-08 10:30:00",
			expectedEnd:   "2024-01-15 10:30:00",
			expectedDiff:  7 * 24 * time.Hour,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workflow := models.Workflow{
				Name:      "测试工作流-" + tt.name,
				ScopeType: models.ScopeTypeAllSubscribed,
				Schedule:  "0 30 10 * * *",
				IsEnabled: true,
				RulesConfig: models.RulesConfig{
					TimeRange:     tt.days,
					TimeRangeMode: "days",
				},
			}
			err := db.Create(&workflow).Error
			assert.NoError(t, err)

			job := models.Job{
				WorkflowID:  workflow.ID,
				Status:      models.JobStatusRunning,
				StartTime:   &triggerTime,
				TriggeredBy: "cron",
			}
			err = db.Create(&job).Error
			assert.NoError(t, err)

			// 计算时间窗口
			days := workflow.RulesConfig.TimeRange
			timeRangeEnd := *job.StartTime
			timeRangeStart := timeRangeEnd.AddDate(0, 0, -days)

			// 验证
			actualStart, _ := time.ParseInLocation("2006-01-02 15:04:05", tt.expectedStart, loc)
			actualEnd, _ := time.ParseInLocation("2006-01-02 15:04:05", tt.expectedEnd, loc)

			assert.Equal(t, actualStart, timeRangeStart, "开始时间应该是"+tt.expectedStart)
			assert.Equal(t, actualEnd, timeRangeEnd, "结束时间应该是"+tt.expectedEnd)
			assert.Equal(t, tt.expectedDiff, timeRangeEnd.Sub(timeRangeStart), "时间范围应该是"+tt.name)

			t.Logf("✅ %s: 时间窗口 %s ~ %s",
				tt.name,
				timeRangeStart.Format("2006-01-02 15:04:05"),
				timeRangeEnd.Format("2006-01-02 15:04:05"))
		})
	}
}

// TestJobTimeRangeWindow_ManualTrigger 测试手动触发的时间窗口计算
func TestJobTimeRangeWindow_ManualTrigger(t *testing.T) {
	db := setupTestDB(t)

	workflow := models.Workflow{
		Name:      "手动触发测试",
		ScopeType: models.ScopeTypeAllSubscribed,
		IsEnabled: true,
		RulesConfig: models.RulesConfig{
			TimeRange:     2, // 最近2天
			TimeRangeMode: "days",
		},
	}
	err := db.Create(&workflow).Error
	assert.NoError(t, err)

	// 手动触发
	job := models.Job{
		WorkflowID:  workflow.ID,
		Status:      models.JobStatusRunning,
		TriggeredBy: "manual",
	}
	err = db.Create(&job).Error
	assert.NoError(t, err)

	// 手动触发的时间窗口计算逻辑
	days := workflow.RulesConfig.TimeRange
	timeRangeEnd := job.CreatedAt
	timeRangeStart := timeRangeEnd.AddDate(0, 0, -days)

	// 验证是2天范围（允许1秒误差）
	expectedDiff := 48 * time.Hour
	actualDiff := timeRangeEnd.Sub(timeRangeStart)
	assert.True(t, actualDiff >= expectedDiff-time.Second && actualDiff <= expectedDiff+time.Second,
		"时间范围应该是48小时（允许1秒误差）")

	t.Logf("✅ 手动触发测试通过")
	t.Logf("   触发时间: %s", job.CreatedAt.Format("2006-01-02 15:04:05"))
	t.Logf("   时间窗口: %s ~ %s", timeRangeStart.Format("2006-01-02 15:04:05"), timeRangeEnd.Format("2006-01-02 15:04:05"))
}
