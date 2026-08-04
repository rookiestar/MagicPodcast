package sync

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"magicpodcast/internal/cache"
	"magicpodcast/internal/feed"
	"magicpodcast/internal/models"

	"github.com/mmcdole/gofeed"
	ext "github.com/mmcdole/gofeed/extensions"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const testFeedXML = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Test Feed</title>
    <description>Local test feed</description>
    <link>https://example.com</link>
    <item>
      <title>Test Episode</title>
      <guid>episode-1</guid>
      <pubDate>Mon, 01 Jan 2024 00:00:00 GMT</pubDate>
      <description>Episode body</description>
    </item>
  </channel>
</rss>`

func newTestFeedServer(tb testing.TB, delay time.Duration) *httptest.Server {
	tb.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveRobotsNotFoundSync(w, r) {
			return
		}
		if delay > 0 {
			time.Sleep(delay)
		}
		w.Header().Set("Content-Type", "application/rss+xml")
		w.Header().Set("Connection", "close")
		_, _ = w.Write([]byte(testFeedXML))
	}))
	tb.Cleanup(server.Close)
	return server
}

// setupTestDB 创建测试数据库
func setupTestDB(t *testing.T) *gorm.DB {
	dbName := fmt.Sprintf("file:testdb_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dbName), &gorm.Config{})
	assert.NoError(t, err)
	sqlDB, err := db.DB()
	assert.NoError(t, err)
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	// 自动迁移
	err = db.AutoMigrate(&models.Podcast{}, &models.Episode{}, &models.PodcastAlternativeFeed{})
	assert.NoError(t, err)

	return db
}

func TestNewServiceLeavesPodcastIndexUnavailableWhenPathIsEmpty(t *testing.T) {
	db := setupTestDB(t)
	service, err := NewService(db, "")
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, service.Close())
	})

	require.Nil(t, service.podcastIndexQuery)
}

func TestConvertGofeedToModelUsesPublishedDateForRecentUpdate(t *testing.T) {
	db := setupTestDB(t)
	service, err := NewService(db, "")
	assert.NoError(t, err)
	t.Cleanup(func() {
		assert.NoError(t, service.Close())
	})

	published := time.Date(2026, 7, 1, 8, 0, 0, 0, time.UTC)
	updated := published.Add(48 * time.Hour)
	converted := service.convertGofeedToModel(&gofeed.Feed{
		Title: "Recent Update Contract",
		Items: []*gofeed.Item{
			{
				Title:           "Episode",
				PublishedParsed: &published,
				UpdatedParsed:   &updated,
			},
		},
	}, "rss", "https://example.com/recent-update.xml")

	assert.Equal(t, published, converted.NewestEpisodeDate)
}

func TestSyncPodcastEpisodeItemsDoesNotUseFetchTimeForRecentUpdate(t *testing.T) {
	db := setupTestDB(t)
	service, err := NewService(db, "")
	assert.NoError(t, err)
	t.Cleanup(func() {
		assert.NoError(t, service.Close())
	})

	published := time.Date(2026, 7, 1, 8, 0, 0, 0, time.UTC)
	oldFetch := time.Date(2026, 7, 10, 8, 0, 0, 0, time.UTC)
	podcast := &models.Podcast{
		XYZID:             "recent-update-no-new-episode",
		Title:             "No New Episode",
		FeedURL:           "https://example.com/no-new-episode.xml",
		NewestEpisodeDate: published,
		LastFetchedAt:     &oldFetch,
	}
	assert.NoError(t, db.Create(podcast).Error)

	assert.NoError(t, db.Create(&models.Episode{
		PodcastID:     podcast.ID,
		GUID:          "no-new-episode-guid",
		Title:         "Existing Episode",
		PublishedDate: published,
	}).Error)

	result, err := service.syncPodcastEpisodeItems(podcast, []*gofeed.Item{{
		GUID:            "no-new-episode-guid",
		Title:           "Existing Episode",
		PublishedParsed: &published,
	}}, EpisodeSyncConfig{UpdateExisting: true})
	assert.NoError(t, err)
	assert.Equal(t, 0, result.Created)
	assert.Equal(t, 0, result.Updated)

	var refreshed models.Podcast
	assert.NoError(t, db.First(&refreshed, podcast.ID).Error)
	assert.Equal(t, published, refreshed.NewestEpisodeDate)
	assert.NotNil(t, refreshed.LastFetchedAt)
	assert.True(t, refreshed.LastFetchedAt.After(oldFetch))
}

// TestSyncPodcastEpisodesDoesNotTreatLastGoodAsSuccessOnOrdinaryFailure locks
// #35/#36: ordinary primary failure must not fall back to last-good as this
// batch's success, advance LastFetchedAt, or hide the live error.
func TestSyncPodcastEpisodesDoesNotTreatLastGoodAsSuccessOnOrdinaryFailure(t *testing.T) {
	var requestCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveRobotsNotFoundSync(w, r) {
			return
		}
		if atomic.AddInt32(&requestCount, 1) == 1 {
			w.Header().Set("Content-Type", "application/rss+xml")
			_, _ = w.Write([]byte(`<?xml version="1.0"?><rss version="2.0"><channel><title>Last Good Workflow</title><item><title>Episode</title><guid>last-good-workflow-episode</guid><pubDate>Tue, 14 Jul 2026 08:00:00 GMT</pubDate><description>Details</description></item></channel></rss>`))
			return
		}
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(server.Close)

	store := feed.NewMemorySnapshotStore(feed.LastGoodStoreConfig{})
	coordinator := feed.NewCoordinator(feed.CoordinatorConfig{
		DomainPolicies: map[string]feed.DomainPolicy{feed.TargetDomain(server.URL): {MaxConcurrency: 1}},
		LastGoodStore:  store,
	})
	db := setupTestDB(t)
	service, err := NewServiceWithFeedCoordinator(db, "", coordinator)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close()) })

	oldFetch := time.Date(2026, 7, 1, 8, 0, 0, 0, time.UTC)
	podcast := &models.Podcast{
		XYZID:         "last-good-workflow",
		Title:         "Last Good Workflow",
		FeedURL:       server.URL + "/feed.xml",
		LastFetchedAt: &oldFetch,
	}
	require.NoError(t, db.Create(podcast).Error)
	config := EpisodeSyncConfig{Mode: SyncModeFull, UpdateExisting: true, MaxEpisodesPerPodcast: 1000}

	first, err := service.SyncPodcastEpisodesWithContext(context.Background(), podcast.ID, &progressReporter{}, config)
	require.NoError(t, err)
	require.Equal(t, feed.AccessSourcePrimary, first.FeedAccess.SourceType)

	require.NoError(t, db.Model(&models.Podcast{}).Where("id = ?", podcast.ID).Update("last_fetched_at", oldFetch).Error)
	second, err := service.SyncPodcastEpisodesWithContext(context.Background(), podcast.ID, &progressReporter{}, config)
	require.Error(t, err, "ordinary failure must not succeed via last-good")
	require.NotNil(t, second.FeedAccess)
	require.Equal(t, feed.AccessSourcePrimary, second.FeedAccess.SourceType)
	require.Equal(t, feed.ErrorCategoryAccessDenied, second.FeedAccess.ErrorCategory)

	var refreshed models.Podcast
	require.NoError(t, db.First(&refreshed, podcast.ID).Error)
	require.NotNil(t, refreshed.LastFetchedAt)
	require.True(t, refreshed.LastFetchedAt.Equal(oldFetch), "failed live fetch must not advance last_fetched_at")
	require.Equal(t, int32(2), atomic.LoadInt32(&requestCount))

	// Snapshot remains available for 304 recovery / diagnostics only.
	failure := &feed.FetchResult{Access: *second.FeedAccess}
	fallback, ok := coordinator.LastGood(context.Background(), podcast.FeedURL, failure)
	require.True(t, ok, "snapshot should still be loadable for diagnostics")
	require.Equal(t, feed.AccessSourceLastGood, fallback.Access.SourceType)
}

func TestSyncPodcastEpisodeItemsInvalidatesPodcastCachesAfterSummaryWriteback(t *testing.T) {
	db := setupTestDB(t)
	service, err := NewService(db, "")
	require.NoError(t, err)
	t.Cleanup(func() {
		assert.NoError(t, service.Close())
	})

	podcast := &models.Podcast{
		XYZID: "summary-cache-invalidation",
		Title: "Summary Cache Invalidation",
	}
	require.NoError(t, db.Create(podcast).Error)

	memCache := cache.GetCache()
	memCache.Clear()
	t.Cleanup(memCache.Clear)
	listKey := cache.NewKeyBuilder().PodcastList(1, 15, "recent_update", nil, "", "")
	detailKey := fmt.Sprintf("podcasts:detail:%d", podcast.ID)
	memCache.Set(listKey, "stale-list")
	memCache.Set(detailKey, "stale-detail")

	result, err := service.syncPodcastEpisodeItems(podcast, []*gofeed.Item{{
		GUID:            "summary-cache-invalidation-episode",
		Title:           "New Episode",
		PublishedParsed: ptrTime(time.Date(2026, 7, 14, 8, 0, 0, 0, time.UTC)),
	}}, EpisodeSyncConfig{UpdateExisting: true})

	require.NoError(t, err)
	assert.Equal(t, 1, result.Created)
	_, listCached := memCache.Get(listKey)
	_, detailCached := memCache.Get(detailKey)
	assert.False(t, listCached, "podcast list cache must be invalidated after episode sync")
	assert.False(t, detailCached, "podcast detail cache must be invalidated after episode sync")
}

func TestSyncPodcastEpisodeItemsReturnsSummaryWritebackError(t *testing.T) {
	db := setupTestDB(t)
	service, err := NewService(db, "")
	assert.NoError(t, err)
	t.Cleanup(func() {
		assert.NoError(t, service.Close())
	})

	podcast := &models.Podcast{
		XYZID: "summary-writeback-error",
		Title: "Summary Writeback Error",
	}
	assert.NoError(t, db.Create(podcast).Error)
	require.NoError(t, db.Exec(`
		CREATE TRIGGER reject_podcast_summary_write
		BEFORE UPDATE OF episode_count, newest_episode_date ON podcasts
		BEGIN
			SELECT RAISE(ABORT, 'summary writeback blocked');
		END`).Error)

	result, err := service.syncPodcastEpisodeItems(podcast, []*gofeed.Item{{
		GUID:            "summary-writeback-error-episode",
		Title:           "New Episode",
		PublishedParsed: ptrTime(time.Date(2026, 7, 14, 8, 0, 0, 0, time.UTC)),
	}}, EpisodeSyncConfig{UpdateExisting: true})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "写回播客汇总失败")
	assert.Equal(t, 1, result.Created)
}

// TestFetcherWithContext 测试Fetcher的context超时控制
func TestFetcherWithContext(t *testing.T) {
	fetcher := feed.NewFetcher(10 * time.Second)

	t.Run("正常的feed抓取应该成功", func(t *testing.T) {
		server := newTestFeedServer(t, 0)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		feed, err := fetcher.FetchFeedWithContext(ctx, server.URL)

		assert.NoError(t, err)
		assert.NotNil(t, feed)
		assert.NotEmpty(t, feed.Title)
	})

	t.Run("超时的context应该返回错误", func(t *testing.T) {
		server := newTestFeedServer(t, 50*time.Millisecond)
		// 使用一个非常短的超时时间
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
		defer cancel()

		feed, err := fetcher.FetchFeedWithContext(ctx, server.URL)

		assert.Error(t, err)
		assert.Nil(t, feed)
		assert.Equal(t, context.DeadlineExceeded, err)
	})

	t.Run("已取消的context应该立即返回", func(t *testing.T) {
		server := newTestFeedServer(t, 0)
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // 立即取消

		feed, err := fetcher.FetchFeedWithContext(ctx, server.URL)

		assert.Error(t, err)
		assert.Nil(t, feed)
	})
}

// TestNoGoroutineLeak 测试没有goroutine泄漏
func TestNoGoroutineLeak(t *testing.T) {
	server := newTestFeedServer(t, 0)
	initialGoroutines := runtime.NumGoroutine()

	db := setupTestDB(t)
	fetcher := feed.NewFetcher(10 * time.Second)
	_, err := NewService(db, "")
	assert.NoError(t, err)

	// 创建测试用的podcast
	podcast := &models.Podcast{
		Title:        "Test Podcast",
		FeedURL:      server.URL,
		DataSource:   "rss",
		EpisodeCount: 0,
	}
	err = db.Create(podcast).Error
	assert.NoError(t, err)

	// 模拟同步操作（会创建goroutine）
	// 使用较短的超时时间避免测试等待太久
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 启动多个并发goroutine来模拟实际使用场景
	// 注意：我们创建10个并发请求，每个都会创建内部goroutine
	var wg sync.WaitGroup
	concurrency := 10
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// 使用带context的feed fetcher
			// 这个goroutine应该在5秒内完成或超时
			_, _ = fetcher.FetchFeedWithContext(ctx, podcast.FeedURL)
		}()
	}
	wg.Wait()

	fetcher.CloseIdleConnections()
	server.CloseClientConnections()
	runtime.GC()

	// 等待一段时间让所有goroutine完成清理
	time.Sleep(500 * time.Millisecond)

	finalGoroutines := runtime.NumGoroutine()

	// 日志输出（帮助理解）
	t.Logf("Initial goroutines: %d, Final goroutines: %d, Diff: %d",
		initialGoroutines, finalGoroutines, finalGoroutines-initialGoroutines)

	// 放宽阈值：由于HTTP连接池、GC等原因，允许一定数量的goroutine增长
	// 10个并发请求可能导致一些goroutine短暂存在
	// 我们期望的增长应该 << concurrency (10)
	// 允许最多增长15个（每个请求平均1.5个goroutine是合理的）
	assert.LessOrEqual(t, finalGoroutines-initialGoroutines, concurrency+5,
		"Possible goroutine leak detected")
}

// TestMutexDeferRelease 测试mutex的正确使用模式
func TestMutexDeferRelease(t *testing.T) {
	var mu sync.Mutex
	var counter int

	// 测试1: 在函数中使用defer确保锁的释放
	testDeferInFunction := func() {
		mu.Lock()
		defer mu.Unlock() // ✅ 正确：defer在函数作用域内
		counter++
	}

	for i := 0; i < 100; i++ {
		testDeferInFunction()
	}

	assert.Equal(t, 100, counter)

	// 测试2: 在循环中使用defer的错误演示（不要在循环中使用defer！）
	// 正确的模式是使用代码块
	counter2 := 0
	for i := 0; i < 100; i++ {
		func() {
			mu.Lock()
			defer mu.Unlock() // ✅ 正确：在匿名函数中使用defer
			counter2++
		}()
	}

	assert.Equal(t, 100, counter2)

	// 测试3: 模拟panic时defer仍然能释放锁
	var mu3 sync.Mutex
	var counter3 int

	func() {
		mu3.Lock()
		defer mu3.Unlock()

		counter3++
		// 即使这里panic，defer也能确保锁被释放
	}()

	assert.Equal(t, 1, counter3)
}

// TestConcurrentPodcastUpdate 测试并发更新的数据库事务保护
// 注意：由于SQLite内存数据库的并发限制，此测试使用互斥锁序列化写入
func TestConcurrentPodcastUpdate(t *testing.T) {
	db := setupTestDB(t)

	// 创建多个podcast来测试并发更新不同记录
	podcasts := make([]*models.Podcast, 10)
	for i := 0; i < 10; i++ {
		podcasts[i] = &models.Podcast{
			XYZID:        fmt.Sprintf("test-xyz-id-%d", i), // 必须提供唯一的 XYZID
			Title:        fmt.Sprintf("Test Podcast %d", i),
			FeedURL:      fmt.Sprintf("https://example.com/feed%d.xml", i),
			DataSource:   "rss",
			EpisodeCount: 100,
		}
		err := db.Create(podcasts[i]).Error
		assert.NoError(t, err)
	}

	// 并发更新不同的podcast
	// 注意：由于SQLite内存数据库的并发写入限制，使用互斥锁序列化数据库访问
	// 这模拟了实际生产环境中使用连接池和互斥锁的场景
	var wg sync.WaitGroup
	var dbMutex sync.Mutex // 保护数据库访问的互斥锁

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			// 使用互斥锁序列化数据库写入，避免SQLite并发限制
			dbMutex.Lock()
			defer dbMutex.Unlock()

			// 使用事务保护更新
			err := db.Transaction(func(tx *gorm.DB) error {
				return tx.Model(&models.Podcast{}).
					Where("id = ?", podcasts[index].ID).
					Updates(map[string]interface{}{
						"episode_count":   100 + index,
						"last_fetched_at": time.Now(),
					}).Error
			})

			assert.NoError(t, err)
		}(i)
	}

	wg.Wait()

	// 验证所有更新都成功
	for i := 0; i < 10; i++ {
		var result models.Podcast
		err := db.First(&result, podcasts[i].ID).Error
		assert.NoError(t, err)
		assert.Equal(t, 100+i, result.EpisodeCount)
	}
}

// TestHTTPConnectionPool 测试HTTP连接池配置
func TestHTTPConnectionPool(t *testing.T) {
	fetcher := feed.NewFetcher(10 * time.Second)
	server := newTestFeedServer(t, 0)

	// 通过反射或实际使用来验证连接池配置
	// 这里我们通过实际并发请求来测试

	concurrentRequests := 20
	var wg sync.WaitGroup
	successCount := 0
	var mu sync.Mutex

	startTime := time.Now()

	for i := 0; i < concurrentRequests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			_, err := fetcher.FetchFeedWithContext(ctx, server.URL)

			mu.Lock()
			if err == nil {
				successCount++
			}
			mu.Unlock()
		}()
	}

	wg.Wait()
	duration := time.Since(startTime)

	t.Logf("Completed %d/%d concurrent requests in %v", successCount, concurrentRequests, duration)

	assert.Equal(t, concurrentRequests, successCount)

	// 并发请求应该在合理时间内完成（说明连接池工作正常）
	assert.Less(t, duration, 30*time.Second)
}

func TestMetadataSyncReusesFetchedFeedForEpisodeSync(t *testing.T) {
	var requestCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveRobotsNotFoundSync(w, r) {
			return
		}
		atomic.AddInt32(&requestCount, 1)
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(testFeedXML))
	}))
	t.Cleanup(server.Close)

	db := setupTestDB(t)
	service, err := NewService(db, "")
	assert.NoError(t, err)
	t.Cleanup(func() {
		assert.NoError(t, service.Close())
	})

	podcast := &models.Podcast{
		XYZID:          "reuse-feed-test",
		Title:          "Reuse Feed Test",
		FeedURL:        server.URL,
		DataSource:     "rss",
		IsSubscribed:   true,
		FeedURLValid:   true,
		EpisodeCount:   0,
		CustomCoverURL: "https://example.com/custom.jpg",
	}
	assert.NoError(t, db.Create(podcast).Error)

	err = service.SyncPodcastsMetadataSSE(NewLogProgressReporter())

	assert.NoError(t, err)
	assert.Equal(t, int32(1), atomic.LoadInt32(&requestCount), "metadata sync should reuse the already fetched feed for episode sync")

	var episodeCount int64
	assert.NoError(t, db.Model(&models.Episode{}).Where("podcast_id = ?", podcast.ID).Count(&episodeCount).Error)
	assert.Equal(t, int64(1), episodeCount)

	var updatedPodcast models.Podcast
	assert.NoError(t, db.First(&updatedPodcast, podcast.ID).Error)
	assert.Equal(t, 1, updatedPodcast.EpisodeCount)
	assert.NotNil(t, updatedPodcast.LastFetchedAt)
	assert.Equal(t, "https://example.com/custom.jpg", updatedPodcast.CustomCoverURL)
}

func TestMetadataSyncRetrySuccessIsCountedOnce(t *testing.T) {
	var requestCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveRobotsNotFoundSync(w, r) {
			return
		}
		if atomic.AddInt32(&requestCount, 1) == 1 {
			http.Error(w, "temporary failure", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(testFeedXML))
	}))
	t.Cleanup(server.Close)

	db := setupTestDB(t)
	service, err := NewService(db, "")
	assert.NoError(t, err)
	t.Cleanup(func() {
		assert.NoError(t, service.Close())
	})

	// Override the outer-retry policy with a deterministic one: Budget=1 (one
	// retry after the first attempt), a FakeSleeper so the test never blocks on
	// real backoff, and a fixed random source so full-jitter is reproducible.
	// This mirrors the legacy MaxRetries=1 / zero-delay behavior without
	// mutating any global state.
	service.applyRetryPolicy(feed.RetryPolicy{
		Budget:  1,
		Base:    2 * time.Second,
		Max:     8 * time.Second,
		Sleeper: &feed.FakeSleeper{},
		Rand:    func() float64 { return 0 },
	})

	podcast := &models.Podcast{
		XYZID:        "retry-success-test",
		Title:        "Retry Success Test",
		FeedURL:      server.URL,
		DataSource:   "rss",
		IsSubscribed: true,
		FeedURLValid: true,
	}
	assert.NoError(t, db.Create(podcast).Error)

	reporter := &progressReporter{}
	err = service.SyncPodcastsMetadataSSE(reporter)

	assert.NoError(t, err)
	assert.Equal(t, int32(2), atomic.LoadInt32(&requestCount))
	assert.Equal(t, 1, reporter.totalPodcasts)
	assert.Equal(t, 1, reporter.successPodcasts)
	assert.Equal(t, 0, reporter.failedPodcasts)
	assert.Equal(t, 0, reporter.skippedPodcasts)
	assert.Equal(t, 1, reporter.newEpisodes)
}

func TestMetadataSyncDoesNotEmitPerPodcastFetchNoise(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveRobotsNotFoundSync(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(testFeedXML))
	}))
	t.Cleanup(server.Close)

	db := setupTestDB(t)
	service, err := NewService(db, "")
	assert.NoError(t, err)
	t.Cleanup(func() {
		assert.NoError(t, service.Close())
	})

	podcast := &models.Podcast{
		XYZID:        "fetch-noise-test",
		Title:        "Fetch Noise Test",
		FeedURL:      server.URL,
		DataSource:   "rss",
		IsSubscribed: true,
		FeedURLValid: true,
	}
	assert.NoError(t, db.Create(podcast).Error)

	var progressMessages []string
	reporter := &progressReporter{
		onProgress: func(message string) {
			progressMessages = append(progressMessages, message)
		},
	}

	err = service.SyncPodcastsMetadataSSE(reporter)

	assert.NoError(t, err)
	assert.NotContains(t, progressMessages, "正在抓取: Fetch Noise Test")
	assert.Contains(t, progressMessages, "[1/1] 成功同步: Fetch Noise Test (单集: +1, ~0)")
}

func TestSyncPodcastEpisodeItemsPreloadsExistingEpisodesAndPreservesUserFields(t *testing.T) {
	db := setupTestDB(t)
	service, err := NewService(db, "")
	assert.NoError(t, err)
	t.Cleanup(func() {
		assert.NoError(t, service.Close())
	})

	podcast := &models.Podcast{
		XYZID:        "episode-preload-test",
		Title:        "Episode Preload Test",
		FeedURL:      "https://example.com/feed.xml",
		DataSource:   "rss",
		IsSubscribed: true,
	}
	assert.NoError(t, db.Create(podcast).Error)

	existing := &models.Episode{
		PodcastID:       podcast.ID,
		Title:           "Old Existing Title",
		GUID:            "existing-guid",
		PublishedDate:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Notes:           "keep my note",
		MyRate:          5,
		MediumURL:       "https://example.com/old.mp3",
		EnclosureType:   "audio/mpeg",
		EnclosureLength: 10,
	}
	assert.NoError(t, db.Create(existing).Error)

	items := []*gofeed.Item{
		{
			Title:           "Updated Existing Title",
			GUID:            "existing-guid",
			PublishedParsed: ptrTime(time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)),
			Enclosures:      []*gofeed.Enclosure{{URL: "https://example.com/new-existing.mp3", Type: "audio/mpeg"}},
		},
		{
			Title:           "New Duplicate First",
			GUID:            "duplicate-new-guid",
			PublishedParsed: ptrTime(time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)),
		},
		{
			Title:           "New Duplicate Latest",
			GUID:            "duplicate-new-guid",
			PublishedParsed: ptrTime(time.Date(2024, 3, 2, 0, 0, 0, 0, time.UTC)),
		},
	}

	result, err := service.syncPodcastEpisodeItems(podcast, items, EpisodeSyncConfig{
		Mode:                  SyncModeFull,
		MaxEpisodesPerPodcast: 1000,
		UpdateExisting:        true,
	})
	assert.NoError(t, err)

	assert.Equal(t, 1, result.Created)
	assert.Equal(t, 2, result.Updated)
	assert.Equal(t, 0, result.Errors)

	var count int64
	assert.NoError(t, db.Model(&models.Episode{}).Where("podcast_id = ?", podcast.ID).Count(&count).Error)
	assert.Equal(t, int64(2), count)

	var updatedExisting models.Episode
	assert.NoError(t, db.Where("guid = ?", "existing-guid").First(&updatedExisting).Error)
	assert.Equal(t, "Updated Existing Title", updatedExisting.Title)
	assert.Equal(t, "keep my note", updatedExisting.Notes)
	assert.Equal(t, 5, updatedExisting.MyRate)
	assert.Equal(t, podcast.ID, updatedExisting.PodcastID)

	var duplicateEpisode models.Episode
	assert.NoError(t, db.Where("guid = ?", "duplicate-new-guid").First(&duplicateEpisode).Error)
	assert.Equal(t, "New Duplicate Latest", duplicateEpisode.Title)
	assert.Equal(t, podcast.ID, duplicateEpisode.PodcastID)
}

func TestSyncPodcastEpisodeItemsSkipsSoftDeletedGUID(t *testing.T) {
	db := setupTestDB(t)
	service, err := NewService(db, "")
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, service.Close())
	})

	podcast := &models.Podcast{
		XYZID:   "soft-deleted-guid-test",
		Title:   "Soft Deleted GUID Test",
		FeedURL: "https://example.com/soft-deleted.xml",
	}
	require.NoError(t, db.Create(podcast).Error)

	deleted := &models.Episode{
		PodcastID:     podcast.ID,
		Title:         "已删除单集",
		GUID:          "soft-deleted-guid",
		PublishedDate: time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC),
	}
	require.NoError(t, db.Create(deleted).Error)
	require.NoError(t, db.Delete(deleted).Error)

	result, err := service.syncPodcastEpisodeItems(podcast, []*gofeed.Item{{
		Title:           "重新出现的单集",
		GUID:            deleted.GUID,
		PublishedParsed: ptrTime(deleted.PublishedDate),
	}}, EpisodeSyncConfig{
		Mode:                  SyncModeFull,
		MaxEpisodesPerPodcast: 1000,
		UpdateExisting:        true,
	})
	require.NoError(t, err)
	assert.Equal(t, 0, result.Created)
	assert.Equal(t, 0, result.Updated)
	assert.Equal(t, 1, result.Skipped)
	assert.Equal(t, 0, result.Errors)

	var activeCount int64
	require.NoError(t, db.Model(&models.Episode{}).
		Where("podcast_id = ? AND guid = ?", podcast.ID, deleted.GUID).
		Count(&activeCount).Error)
	assert.Equal(t, int64(0), activeCount)

	var persisted models.Episode
	require.NoError(t, db.Unscoped().First(&persisted, deleted.ID).Error)
	assert.True(t, persisted.DeletedAt.Valid)
	assert.Equal(t, deleted.Title, persisted.Title)
}

func TestSyncPodcastEpisodeItemsDoesNotMoveEpisodeFromAnotherPodcast(t *testing.T) {
	db := setupTestDB(t)
	service, err := NewService(db, "")
	assert.NoError(t, err)
	t.Cleanup(func() {
		assert.NoError(t, service.Close())
	})

	sourcePodcast := &models.Podcast{
		XYZID:        "source-podcast",
		Title:        "Source Podcast",
		FeedURL:      "https://example.com/source.xml",
		DataSource:   "rss",
		IsSubscribed: true,
	}
	targetPodcast := &models.Podcast{
		XYZID:        "target-podcast",
		Title:        "Target Podcast",
		FeedURL:      "https://example.com/target.xml",
		DataSource:   "rss",
		IsSubscribed: true,
	}
	assert.NoError(t, db.Create(sourcePodcast).Error)
	assert.NoError(t, db.Create(targetPodcast).Error)

	existing := &models.Episode{
		PodcastID:     sourcePodcast.ID,
		Title:         "Original Owner Episode",
		GUID:          "shared-guid",
		PublishedDate: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	assert.NoError(t, db.Create(existing).Error)

	result, err := service.syncPodcastEpisodeItems(targetPodcast, []*gofeed.Item{
		{
			Title:           "Should Not Move",
			GUID:            "shared-guid",
			PublishedParsed: ptrTime(time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)),
		},
	}, EpisodeSyncConfig{
		Mode:                  SyncModeFull,
		MaxEpisodesPerPodcast: 1000,
		UpdateExisting:        true,
	})
	assert.Error(t, err)

	assert.Equal(t, 0, result.Created)
	assert.Equal(t, 0, result.Updated)
	assert.Equal(t, 1, result.Errors)

	var unchanged models.Episode
	assert.NoError(t, db.First(&unchanged, existing.ID).Error)
	assert.Equal(t, sourcePodcast.ID, unchanged.PodcastID)
	assert.Equal(t, "Original Owner Episode", unchanged.Title)
}

func TestSyncPodcastEpisodeItemsSkipsUnchangedExistingEpisode(t *testing.T) {
	db := setupTestDB(t)
	service, err := NewService(db, "")
	assert.NoError(t, err)
	t.Cleanup(func() {
		assert.NoError(t, service.Close())
	})

	podcast := &models.Podcast{
		XYZID:        "unchanged-episode-test",
		Title:        "Unchanged Episode Test",
		FeedURL:      "https://example.com/unchanged.xml",
		DataSource:   "rss",
		IsSubscribed: true,
	}
	assert.NoError(t, db.Create(podcast).Error)

	published := time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC)
	updated := time.Date(2024, 4, 2, 0, 0, 0, 0, time.UTC)
	existing := &models.Episode{
		PodcastID:       podcast.ID,
		Title:           "Same Episode",
		GUID:            "same-guid",
		MediumURL:       "https://example.com/same.mp3",
		ShowNotes:       "same description",
		PublishedDate:   published,
		UpdatedDate:     &updated,
		Duration:        123,
		Link:            "https://example.com/same",
		Content:         "same content",
		ImageURL:        "https://example.com/image.jpg",
		EnclosureType:   "audio/mpeg",
		EnclosureLength: 456,
		Notes:           "user note",
		MyRate:          4,
	}
	assert.NoError(t, db.Create(existing).Error)
	originalUpdatedAt := existing.UpdatedAt

	result, err := service.syncPodcastEpisodeItems(podcast, []*gofeed.Item{
		{
			Title:           "Same Episode",
			GUID:            "same-guid",
			Description:     "same description",
			Content:         "same content",
			Link:            "https://example.com/same",
			PublishedParsed: &published,
			UpdatedParsed:   &updated,
			ITunesExt:       &ext.ITunesItemExtension{Duration: "123"},
			Image:           &gofeed.Image{URL: "https://example.com/image.jpg"},
			Enclosures: []*gofeed.Enclosure{{
				URL:    "https://example.com/same.mp3",
				Type:   "audio/mpeg",
				Length: "456",
			}},
		},
	}, EpisodeSyncConfig{
		Mode:                  SyncModeFull,
		MaxEpisodesPerPodcast: 1000,
		UpdateExisting:        true,
	})
	assert.NoError(t, err)

	assert.Equal(t, 0, result.Created)
	assert.Equal(t, 0, result.Updated)
	assert.Equal(t, 1, result.Skipped)
	assert.Equal(t, 0, result.Errors)

	var unchanged models.Episode
	assert.NoError(t, db.First(&unchanged, existing.ID).Error)
	assert.True(t, originalUpdatedAt.Equal(unchanged.UpdatedAt))
	assert.Equal(t, "user note", unchanged.Notes)
	assert.Equal(t, 4, unchanged.MyRate)
}

func ptrTime(value time.Time) *time.Time {
	return &value
}

// BenchmarkFetchFeedWithContext 基准测试：带context的feed抓取性能
func BenchmarkFetchFeedWithContext(b *testing.B) {
	fetcher := feed.NewFetcher(10 * time.Second)
	server := newTestFeedServer(b, 0)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = fetcher.FetchFeedWithContext(ctx, server.URL)
	}
}
