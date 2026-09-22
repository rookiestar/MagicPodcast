package sync

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"magicpodcast/internal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// historyFeedXML 生成指定数量的单集 RSS，发布日期在 2020 年起按日递增，
// 保证超过单窗口上限（1000）的feed仍每集都有合法发布时间。
func historyFeedXML(items int) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel><title>History Feed</title><link>https://example.com</link><description>history</description>`)
	for i := 1; i <= items; i++ {
		published := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, i-1)
		b.WriteString(fmt.Sprintf(`<item><title>History %d</title><guid>history-ep-%d</guid>
<pubDate>%s</pubDate><description>body %d</description></item>`, i, i, published.Format(http.TimeFormat), i))
	}
	b.WriteString(`</channel></rss>`)
	return b.String()
}

func emptyChannelFeedXML() string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel><title>Empty Feed</title><link>https://example.com</link><description>empty</description></channel></rss>`
}

// newTestHistoryManager 构造独立于全局实例的管理器：注入空 sleep 使执行内
// 退避即时完成，测试不等待真实时钟。
func newTestHistoryManager(t *testing.T, db *gorm.DB, svc *Service) *HistorySyncManager {
	t.Helper()
	m := newHistorySyncManager(db, svc)
	m.sleep = func(time.Duration) {}
	m.start()
	t.Cleanup(m.Stop)
	return m
}

// allowAllCoverage 允许全部工作流触发任务执行（模拟存在启用覆盖）。
func allowAllCoverage(t *testing.T) {
	t.Helper()
	SetHistoryCoverageGuard(func(db *gorm.DB, podcastID uint) bool { return true })
	t.Cleanup(func() { SetHistoryCoverageGuard(nil) })
}

// waitForHistoryTaskStatus 等待任务到达任一期望状态。
func waitForHistoryTaskStatus(t *testing.T, db *gorm.DB, taskID uint, statuses ...string) models.PodcastHistorySyncTask {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		var task models.PodcastHistorySyncTask
		if err := db.First(&task, taskID).Error; err == nil {
			for _, status := range statuses {
				if task.Status == status {
					return task
				}
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	var task models.PodcastHistorySyncTask
	if err := db.First(&task, taskID).Error; err != nil {
		t.Fatalf("history sync task %d not found", taskID)
	}
	t.Fatalf("history sync task %d status = %s, want one of %s", taskID, task.Status, strings.Join(statuses, "/"))
	return task
}

func countEpisodesForPodcast(t *testing.T, db *gorm.DB, podcastID uint) int64 {
	t.Helper()
	var count int64
	require.NoError(t, db.Model(&models.Episode{}).Where("podcast_id = ?", podcastID).Count(&count).Error)
	return count
}

func TestEnsureHistoryTasksIsIdempotent(t *testing.T) {
	db := setupTestDB(t)
	podcast := seedSyncPodcast(t, db)

	created, err := EnsureHistoryTasks(db, []uint{podcast.ID}, models.HistorySyncTriggerWorkflow, false)
	require.NoError(t, err)
	require.Len(t, created, 1)
	assert.Equal(t, models.HistorySyncStatusPending, created[0].Status)

	// 重复登记（重复加入、并发触发、跨工作流覆盖）不产生第二个任务。
	again, err := EnsureHistoryTasks(db, []uint{podcast.ID}, models.HistorySyncTriggerWorkflow, false)
	require.NoError(t, err)
	assert.Empty(t, again)

	// 已有终态完成记录的节目不再自动登记。
	require.NoError(t, db.Model(&models.PodcastHistorySyncTask{}).Where("id = ?", created[0].ID).
		Updates(map[string]interface{}{"status": models.HistorySyncStatusCompleted, "finished_at": time.Now()}).Error)
	third, err := EnsureHistoryTasks(db, []uint{podcast.ID}, models.HistorySyncTriggerWorkflow, false)
	require.NoError(t, err)
	assert.Empty(t, third)
}

func TestEnsureHistoryTasksWatermarkFilterSkipsOldPodcasts(t *testing.T) {
	db := setupTestDB(t)
	oldPodcast := seedSyncPodcast(t, db)
	newPodcast := seedSyncPodcastWithName(t, db, "水位线之后")

	// 旧节目落库时间回拨到一小时前，水位线设回 30 分钟前：旧节目在水位线
	// 之前入库应被跳过，新节目（现在入库）应被登记。
	watermark := time.Now().Add(-time.Minute)
	require.NoError(t, db.Where("id = ?", oldPodcast.ID).
		Model(&models.Podcast{}).Update("created_at", watermark.Add(-time.Hour)).Error)

	// 先让水位线常量落库（模拟启动初始化），再回拨到测试值。
	initialized, err := HistorySyncWatermark(db)
	require.NoError(t, err)
	require.NoError(t, db.Model(&models.SyncConfig{}).Where("config_key = ?", historySyncWatermarkKey).
		Update("config_value", initialized.Add(-30*time.Minute).Format(time.RFC3339Nano)).Error)

	created, err := EnsureHistoryTasks(db, []uint{oldPodcast.ID, newPodcast.ID}, models.HistorySyncTriggerWorkflow, true)
	require.NoError(t, err)
	require.Len(t, created, 1)
	assert.Equal(t, newPodcast.ID, created[0].PodcastID)
}

func TestStartManualHistoryTaskReuseAndRetry(t *testing.T) {
	db := setupTestDB(t)
	podcast := seedSyncPodcast(t, db)

	// 首次手动启动：创建任务。
	task, created, err := StartManualHistoryTask(db, podcast.ID)
	require.NoError(t, err)
	assert.True(t, created)
	assert.Equal(t, models.HistorySyncTriggerManual, task.Trigger)

	// 重复点击/刷新：复用同一任务，不重复建任务。
	reused, createdAgain, err := StartManualHistoryTask(db, podcast.ID)
	require.NoError(t, err)
	assert.False(t, createdAgain)
	assert.Equal(t, task.ID, reused.ID)

	// 失败后重试：同一任务复位，任务标识保持稳定。
	require.NoError(t, db.Model(&models.PodcastHistorySyncTask{}).Where("id = ?", task.ID).
		Updates(map[string]interface{}{
			"status":        models.HistorySyncStatusFailed,
			"error_message": "network timeout",
			"finished_at":   time.Now(),
		}).Error)
	retried, createdRetry, err := StartManualHistoryTask(db, podcast.ID)
	require.NoError(t, err)
	assert.False(t, createdRetry, "failed 任务的复位重试不算新建")
	assert.Equal(t, task.ID, retried.ID)
	assert.Equal(t, models.HistorySyncStatusQueued, retried.Status)
	assert.Empty(t, retried.ErrorMessage)
	assert.Nil(t, retried.FinishedAt)

	// 已完成节目再次手动检查：创建新的检查任务。
	require.NoError(t, db.Model(&models.PodcastHistorySyncTask{}).Where("id = ?", task.ID).
		Update("status", models.HistorySyncStatusCompleted).Error)
	recheck, createdRecheck, err := StartManualHistoryTask(db, podcast.ID)
	require.NoError(t, err)
	assert.True(t, createdRecheck)
	assert.NotEqual(t, task.ID, recheck.ID)
}

func TestStartManualHistoryTaskConcurrentClicksCreateSingleTask(t *testing.T) {
	db := setupTestDB(t)
	podcast := seedSyncPodcast(t, db)

	const clickers = 8
	results := make(chan uint, clickers)
	for i := 0; i < clickers; i++ {
		go func() {
			task, _, err := StartManualHistoryTask(db, podcast.ID)
			if err != nil {
				results <- 0
				return
			}
			results <- task.ID
		}()
	}
	ids := make(map[uint]int, clickers)
	for i := 0; i < clickers; i++ {
		id := <-results
		require.NotZero(t, id)
		ids[id]++
	}
	assert.Len(t, ids, 1, "并发点击应全部返回同一活动任务")
}

func TestHistorySyncManagerCompletesFullHistory(t *testing.T) {
	allowAllCoverage(t)
	server := newCursorFeedServer(t, cursorFeedXML(3))
	db := setupTestDB(t)
	service, err := NewService(db, "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close()) })

	podcast := importCursorPodcast(t, service, server.URL)
	m := newTestHistoryManager(t, db, service)

	created, err := EnsureHistoryTasks(db, []uint{podcast.ID}, models.HistorySyncTriggerWorkflow, false)
	require.NoError(t, err)
	require.Len(t, created, 1)
	m.enqueue(podcast.ID)

	task := waitForHistoryTaskStatus(t, db, created[0].ID, models.HistorySyncStatusCompleted)
	assert.Equal(t, int64(3), countEpisodesForPodcast(t, db, podcast.ID))
	assert.Equal(t, 3, task.ProcessedCount)
	assert.Equal(t, 3, task.CreatedCount)
	assert.NotNil(t, task.FinishedAt)

	// 单集同步游标随完整提交推进；再次领取不重复入库。
	var refreshed models.Podcast
	require.NoError(t, db.First(&refreshed, podcast.ID).Error)
	require.NotNil(t, refreshed.LastEpisodeSyncAt)

	second, err := EnsureHistoryTasks(db, []uint{podcast.ID}, models.HistorySyncTriggerWorkflow, false)
	require.NoError(t, err)
	assert.Empty(t, second, "历史完成后不重复自动登记")
}

func TestHistorySyncManagerEmptySourceCompletesWithNote(t *testing.T) {
	server := newCursorFeedServer(t, emptyChannelFeedXML())
	db := setupTestDB(t)
	service, err := NewService(db, "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close()) })

	podcast := importCursorPodcast(t, service, server.URL)
	m := newTestHistoryManager(t, db, service)

	created, err := EnsureHistoryTasks(db, []uint{podcast.ID}, models.HistorySyncTriggerManual, false)
	require.NoError(t, err)
	require.Len(t, created, 1)
	m.enqueue(podcast.ID)

	task := waitForHistoryTaskStatus(t, db, created[0].ID, models.HistorySyncStatusCompleted)
	assert.Equal(t, int64(0), countEpisodesForPodcast(t, db, podcast.ID))
	assert.Contains(t, task.SourceNote, "源当前未提供可获取单集")
}

func TestHistorySyncManagerContinuesBeyondSingleWindow(t *testing.T) {
	allowAllCoverage(t)
	const totalItems = 1200
	server := newCursorFeedServer(t, historyFeedXML(totalItems))
	db := setupTestDB(t)
	service, err := NewService(db, "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close()) })

	podcast := importCursorPodcast(t, service, server.URL)
	m := newTestHistoryManager(t, db, service)

	created, err := EnsureHistoryTasks(db, []uint{podcast.ID}, models.HistorySyncTriggerWorkflow, false)
	require.NoError(t, err)
	require.Len(t, created, 1)
	m.enqueue(podcast.ID)

	task := waitForHistoryTaskStatus(t, db, created[0].ID, models.HistorySyncStatusCompleted, models.HistorySyncStatusPartial)
	assert.Equal(t, models.HistorySyncStatusCompleted, task.Status, "任务应自动续批直到全部可获取条目处理完成")
	assert.Equal(t, int64(totalItems), countEpisodesForPodcast(t, db, podcast.ID))
	assert.Equal(t, totalItems, task.ProcessedCount)
	assert.NotNil(t, task.TotalKnown)
	assert.Equal(t, totalItems, *task.TotalKnown)
}

func TestHistorySyncManagerRetriesFetchFailureThenCompletes(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveRobotsNotFoundSync(w, r) {
			return
		}
		if atomic.AddInt32(&calls, 1) <= 2 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(cursorFeedXML(2)))
	}))
	t.Cleanup(server.Close)

	db := setupTestDB(t)
	service, err := NewService(db, "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close()) })

	podcast := importCursorPodcast(t, service, server.URL)
	m := newTestHistoryManager(t, db, service)

	created, err := EnsureHistoryTasks(db, []uint{podcast.ID}, models.HistorySyncTriggerManual, false)
	require.NoError(t, err)
	require.Len(t, created, 1)
	m.enqueue(podcast.ID)

	waitForHistoryTaskStatus(t, db, created[0].ID, models.HistorySyncStatusCompleted)
	assert.GreaterOrEqual(t, atomic.LoadInt32(&calls), int32(3), "暂时失败应在执行内自动重试")
	assert.Equal(t, int64(2), countEpisodesForPodcast(t, db, podcast.ID))
}

func TestHistorySyncManagerMarksFailedWithRetryPlan(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveRobotsNotFoundSync(w, r) {
			return
		}
		// 首次请求成功供导入建立节目，其后全部失败供历史同步重试。
		if atomic.AddInt32(&calls, 1) > 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(cursorFeedXML(2)))
	}))
	t.Cleanup(server.Close)

	db := setupTestDB(t)
	service, err := NewService(db, "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close()) })

	podcast := importCursorPodcast(t, service, server.URL)
	m := newTestHistoryManager(t, db, service)

	created, err := EnsureHistoryTasks(db, []uint{podcast.ID}, models.HistorySyncTriggerManual, false)
	require.NoError(t, err)
	require.Len(t, created, 1)
	m.enqueue(podcast.ID)

	task := waitForHistoryTaskStatus(t, db, created[0].ID, models.HistorySyncStatusFailed)
	assert.NotEmpty(t, task.ErrorMessage)
	assert.NotNil(t, task.NextRetryAt, "可重试失败应安排自动退避重试")
	assert.NotNil(t, task.FinishedAt)
	assert.Equal(t, int64(0), countEpisodesForPodcast(t, db, podcast.ID))
}

func TestHistorySyncManagerResumesRunningTaskAfterRestart(t *testing.T) {
	allowAllCoverage(t)
	server := newCursorFeedServer(t, cursorFeedXML(3))
	db := setupTestDB(t)
	service, err := NewService(db, "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close()) })

	podcast := importCursorPodcast(t, service, server.URL)

	// 模拟上个进程崩溃遗留：任务行停留在 running 且未写任何单集。
	leftover := models.PodcastHistorySyncTask{
		PodcastID: podcast.ID,
		Trigger:   models.HistorySyncTriggerWorkflow,
		Status:    models.HistorySyncStatusRunning,
		Attempts:  1,
		StartedAt: ptrTime(time.Now().Add(-time.Hour)),
	}
	require.NoError(t, db.Create(&leftover).Error)

	// 新进程启动：恢复扫描把 running 回退为 queued 并继续执行。
	m := newTestHistoryManager(t, db, service)
	task := waitForHistoryTaskStatus(t, db, leftover.ID, models.HistorySyncStatusCompleted)
	assert.Equal(t, int64(3), countEpisodesForPodcast(t, db, podcast.ID))

	// 恢复后完成标记不得重复推进：重启一次同样只保留一份单集。
	m.Stop()
	m2 := newTestHistoryManager(t, db, service)
	m2.enqueueActiveTasks(100)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if countEpisodesForPodcast(t, db, podcast.ID) > 3 {
			t.Fatalf("恢复后重复入库")
		}
		time.Sleep(20 * time.Millisecond)
	}
	assert.Equal(t, int64(3), countEpisodesForPodcast(t, db, podcast.ID))
	assert.Equal(t, models.HistorySyncStatusCompleted, task.Status)
}

func TestHistorySyncWorkflowTriggerWaitsForEnabledCoverage(t *testing.T) {
	server := newCursorFeedServer(t, cursorFeedXML(2))
	db := setupTestDB(t)
	service, err := NewService(db, "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close()) })

	podcast := importCursorPodcast(t, service, server.URL)
	m := newTestHistoryManager(t, db, service)

	// 无启用覆盖（例如工作流停用）：工作流触发任务保持待同步。
	SetHistoryCoverageGuard(func(db *gorm.DB, podcastID uint) bool { return false })
	t.Cleanup(func() { SetHistoryCoverageGuard(nil) })

	created, err := EnsureHistoryTasks(db, []uint{podcast.ID}, models.HistorySyncTriggerWorkflow, false)
	require.NoError(t, err)
	require.Len(t, created, 1)
	m.enqueue(podcast.ID)

	time.Sleep(300 * time.Millisecond)
	var stalled models.PodcastHistorySyncTask
	require.NoError(t, db.First(&stalled, created[0].ID).Error)
	assert.Equal(t, models.HistorySyncStatusPending, stalled.Status, "无启用覆盖的工作流触发任务不得执行")
	assert.Equal(t, int64(0), countEpisodesForPodcast(t, db, podcast.ID))

	// 启用覆盖后同一任务继续执行。
	SetHistoryCoverageGuard(func(db *gorm.DB, podcastID uint) bool { return true })
	m.enqueue(podcast.ID)
	task := waitForHistoryTaskStatus(t, db, created[0].ID, models.HistorySyncStatusCompleted)
	assert.Equal(t, int64(2), countEpisodesForPodcast(t, db, podcast.ID))
	assert.Equal(t, models.HistorySyncStatusCompleted, task.Status)
}

func TestHistorySyncManualTriggerRunsRegardlessOfCoverage(t *testing.T) {
	server := newCursorFeedServer(t, cursorFeedXML(1))
	db := setupTestDB(t)
	service, err := NewService(db, "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close()) })

	podcast := importCursorPodcast(t, service, server.URL)
	m := newTestHistoryManager(t, db, service)

	// 手动同步不依赖工作流启用状态：守卫返回 false 也执行。
	SetHistoryCoverageGuard(func(db *gorm.DB, podcastID uint) bool { return false })
	t.Cleanup(func() { SetHistoryCoverageGuard(nil) })

	task, _, err := StartManualHistoryTask(db, podcast.ID)
	require.NoError(t, err)
	m.enqueue(podcast.ID)

	done := waitForHistoryTaskStatus(t, db, task.ID, models.HistorySyncStatusCompleted)
	assert.Equal(t, int64(1), countEpisodesForPodcast(t, db, podcast.ID))
	assert.Equal(t, models.HistorySyncStatusCompleted, done.Status)
}

func TestHistorySyncManagerSweepRetriesDueFailedTask(t *testing.T) {
	server := newCursorFeedServer(t, cursorFeedXML(2))
	db := setupTestDB(t)
	service, err := NewService(db, "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close()) })

	podcast := importCursorPodcast(t, service, server.URL)
	m := newTestHistoryManager(t, db, service)

	created, err := EnsureHistoryTasks(db, []uint{podcast.ID}, models.HistorySyncTriggerManual, false)
	require.NoError(t, err)
	require.Len(t, created, 1)

	// 失败任务到达重试时间：周期扫描复位为 queued 并执行到完成。
	require.NoError(t, db.Model(&models.PodcastHistorySyncTask{}).Where("id = ?", created[0].ID).
		Updates(map[string]interface{}{
			"status":        models.HistorySyncStatusFailed,
			"next_retry_at": time.Now().Add(-time.Minute),
			"finished_at":   time.Now(),
		}).Error)
	m.sweepRetryDue()
	task := waitForHistoryTaskStatus(t, db, created[0].ID, models.HistorySyncStatusCompleted)
	assert.Equal(t, int64(2), countEpisodesForPodcast(t, db, podcast.ID))
	assert.Nil(t, task.NextRetryAt)
}

func TestHistorySyncManagerSweepRetriesDuePartialTask(t *testing.T) {
	server := newCursorFeedServer(t, cursorFeedXML(2))
	db := setupTestDB(t)
	service, err := NewService(db, "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close()) })

	podcast := importCursorPodcast(t, service, server.URL)
	m := newTestHistoryManager(t, db, service)

	created, err := EnsureHistoryTasks(db, []uint{podcast.ID}, models.HistorySyncTriggerManual, false)
	require.NoError(t, err)
	require.Len(t, created, 1)

	// 续批预算耗尽的 partial 任务带 next_retry_at：周期扫描同样复位执行。
	require.NoError(t, db.Model(&models.PodcastHistorySyncTask{}).Where("id = ?", created[0].ID).
		Updates(map[string]interface{}{
			"status":        models.HistorySyncStatusPartial,
			"next_retry_at": time.Now().Add(-time.Minute),
			"finished_at":   time.Now(),
		}).Error)
	m.sweepRetryDue()
	waitForHistoryTaskStatus(t, db, created[0].ID, models.HistorySyncStatusCompleted)
	assert.Equal(t, int64(2), countEpisodesForPodcast(t, db, podcast.ID))
}

func TestHistorySyncManagerDefersWhileWorkflowSyncHoldsSlot(t *testing.T) {
	server := newCursorFeedServer(t, cursorFeedXML(2))
	db := setupTestDB(t)
	service, err := NewService(db, "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close()) })

	podcast := importCursorPodcast(t, service, server.URL)
	allowAllCoverage(t)
	m := newTestHistoryManager(t, db, service)

	// 模拟周期工作流正在同步同一节目：历史任务领取后立即归还，保持待同步。
	require.True(t, service.TryAcquirePodcastEpisodeSync(podcast.ID))
	created, err := EnsureHistoryTasks(db, []uint{podcast.ID}, models.HistorySyncTriggerWorkflow, false)
	require.NoError(t, err)
	require.Len(t, created, 1)
	m.enqueue(podcast.ID)

	time.Sleep(300 * time.Millisecond)
	var deferred models.PodcastHistorySyncTask
	require.NoError(t, db.First(&deferred, created[0].ID).Error)
	assert.Equal(t, models.HistorySyncStatusPending, deferred.Status, "工作流持有同步槽时历史任务不得执行")
	assert.Equal(t, 0, deferred.Attempts, "归还领取时应退还尝试计数")
	assert.Equal(t, int64(0), countEpisodesForPodcast(t, db, podcast.ID))

	// 工作流释放后任务正常执行。
	service.ReleasePodcastEpisodeSync(podcast.ID)
	m.enqueue(podcast.ID)
	waitForHistoryTaskStatus(t, db, created[0].ID, models.HistorySyncStatusCompleted)
	assert.Equal(t, int64(2), countEpisodesForPodcast(t, db, podcast.ID))
}

func TestPodcastEpisodeSyncSlotMutualExclusion(t *testing.T) {
	db := setupTestDB(t)
	service, err := NewService(db, "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close()) })

	require.True(t, service.TryAcquirePodcastEpisodeSync(42))
	require.False(t, service.TryAcquirePodcastEpisodeSync(42), "占用中不得重复获取")

	// 阻塞获取在 ctx 取消时返回错误。
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, acquireErr := service.AcquirePodcastEpisodeSync(ctx, 42)
	require.Error(t, acquireErr)

	service.ReleasePodcastEpisodeSync(42)
	require.True(t, service.TryAcquirePodcastEpisodeSync(42))
	service.ReleasePodcastEpisodeSync(42)
	release, acquireErr := service.AcquirePodcastEpisodeSync(context.Background(), 42)
	require.NoError(t, acquireErr)
	release()
	require.True(t, service.TryAcquirePodcastEpisodeSync(42), "释放后可重新占用")
}

func seedSyncPodcast(t *testing.T, db *gorm.DB) *models.Podcast {
	return seedSyncPodcastWithName(t, db, "历史同步测试节目")
}

func seedSyncPodcastWithName(t *testing.T, db *gorm.DB, title string) *models.Podcast {
	t.Helper()
	podcast := models.Podcast{
		XYZID:        fmt.Sprintf("hist-%d", time.Now().UnixNano()),
		Title:        title,
		FeedURL:      fmt.Sprintf("https://example.com/%d/feed.xml", time.Now().UnixNano()),
		IsSubscribed: true,
	}
	require.NoError(t, db.Create(&podcast).Error)
	return &podcast
}

// A failed outer fetch must be observed by the manager, not consumed during import.
func TestHistorySyncOuterRetryClearsRecoveredError(t *testing.T) {
	var healthy atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveRobotsNotFoundSync(w, r) {
			return
		}
		if !healthy.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(historyFeedXML(2)))
	}))
	defer server.Close()
	db := setupTestDB(t)
	svc, err := NewService(db, "")
	require.NoError(t, err)
	defer svc.Close()
	p := seedSyncPodcast(t, db)
	require.NoError(t, db.Model(&p).Update("feed_url", server.URL+"/feed.xml").Error)
	task, _, err := StartManualHistoryTask(db, p.ID)
	require.NoError(t, err)
	m := newHistorySyncManager(db, svc)
	m.sleep = func(time.Duration) { healthy.Store(true) }
	m.processPodcast(p.ID)
	require.NoError(t, db.First(task, task.ID).Error)
	require.Equal(t, models.HistorySyncStatusCompleted, task.Status, task.ErrorMessage)
	require.Empty(t, task.ErrorMessage)
	require.Nil(t, task.NextRetryAt)
	require.EqualValues(t, 2, countEpisodesForPodcast(t, db, p.ID))
}

func TestManualHistorySyncTakesOverPausedWorkflowTask(t *testing.T) {
	for _, status := range []string{models.HistorySyncStatusPending, models.HistorySyncStatusFailed, models.HistorySyncStatusPartial} {
		t.Run(status, func(t *testing.T) {
			SetHistoryCoverageGuard(func(*gorm.DB, uint) bool { return false })
			defer SetHistoryCoverageGuard(nil)
			server := newCursorFeedServer(t, historyFeedXML(2))
			db := setupTestDB(t)
			svc, err := NewService(db, "")
			require.NoError(t, err)
			defer svc.Close()
			p := seedSyncPodcast(t, db)
			require.NoError(t, db.Model(&p).Update("feed_url", server.URL+"/feed.xml").Error)
			task := models.PodcastHistorySyncTask{PodcastID: p.ID, Trigger: models.HistorySyncTriggerWorkflow, Status: status, Attempts: 3}
			require.NoError(t, db.Create(&task).Error)
			manual, _, err := StartManualHistoryTask(db, p.ID)
			require.NoError(t, err)
			m := newHistorySyncManager(db, svc)
			m.processPodcast(p.ID)
			require.NoError(t, db.First(manual, manual.ID).Error)
			require.Equal(t, models.HistorySyncStatusCompleted, manual.Status)
			require.EqualValues(t, 2, countEpisodesForPodcast(t, db, p.ID))
		})
	}
}

func TestHistorySyncStopsBetweenBatchesWhenCoverageDisabled(t *testing.T) {
	var enabled atomic.Bool
	enabled.Store(true)
	SetHistoryCoverageGuard(func(*gorm.DB, uint) bool { return enabled.Load() })
	defer SetHistoryCoverageGuard(nil)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveRobotsNotFoundSync(w, r) {
			return
		}
		enabled.Store(false)
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(historyFeedXML(1200)))
	}))
	defer server.Close()
	db := setupTestDB(t)
	svc, err := NewService(db, "")
	require.NoError(t, err)
	defer svc.Close()
	p := seedSyncPodcast(t, db)
	require.NoError(t, db.Model(&p).Update("feed_url", server.URL+"/feed.xml").Error)
	tasks, err := EnsureHistoryTasks(db, []uint{p.ID}, models.HistorySyncTriggerWorkflow, false)
	require.NoError(t, err)
	m := newHistorySyncManager(db, svc)
	m.processPodcast(p.ID)
	require.NoError(t, db.First(&tasks[0], tasks[0].ID).Error)
	require.Equal(t, models.HistorySyncStatusPending, tasks[0].Status)
	require.EqualValues(t, 1000, countEpisodesForPodcast(t, db, p.ID))
	enabled.Store(true)
	// Already cached RSS can be read safely; if source is fetched again it still pauses
	// only after its final batch, which is enough to complete the remaining 200.
	m.processPodcast(p.ID)
	require.NoError(t, db.First(&tasks[0], tasks[0].ID).Error)
	require.Equal(t, models.HistorySyncStatusCompleted, tasks[0].Status)
	require.EqualValues(t, 1200, countEpisodesForPodcast(t, db, p.ID))
}

func TestHistorySyncSweepAdvancesPastPausedTasks(t *testing.T) {
	db := setupTestDB(t)
	for i := 0; i < 205; i++ {
		require.NoError(t, db.Create(&models.PodcastHistorySyncTask{PodcastID: uint(i + 1), Trigger: models.HistorySyncTriggerWorkflow, Status: models.HistorySyncStatusPending}).Error)
	}
	m := newHistorySyncManager(db, nil)
	m.enqueueActiveTasks(200)
	for len(m.queue) > 0 {
		<-m.queue
	}
	m.enqueueActiveTasks(200)
	require.Equal(t, 5, len(m.queue), "later tasks must not be starved by the first 200 paused tasks")
	require.EqualValues(t, 201, <-m.queue)
}

func TestHistorySyncUsesSharedRetryBudget(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveRobotsNotFoundSync(w, r) {
			return
		}
		calls.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	db := setupTestDB(t)
	svc, err := NewService(db, "")
	require.NoError(t, err)
	defer svc.Close()
	p := seedSyncPodcast(t, db)
	require.NoError(t, db.Model(&p).Update("feed_url", server.URL+"/feed.xml").Error)
	task, _, err := StartManualHistoryTask(db, p.ID)
	require.NoError(t, err)
	m := newHistorySyncManager(db, svc)
	m.policy.Budget = 1
	sleeps := 0
	m.sleep = func(time.Duration) { sleeps++ }
	m.processPodcast(p.ID)
	require.Equal(t, 1, sleeps, "one outer retry, not forty history passes")
	require.NoError(t, db.First(task, task.ID).Error)
	require.Equal(t, models.HistorySyncStatusFailed, task.Status)
	require.NotNil(t, task.NextRetryAt)
}
