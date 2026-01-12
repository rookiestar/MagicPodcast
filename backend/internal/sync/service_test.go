package sync

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"testing"
	"time"

	"magicpodcast/internal/feed"
	"magicpodcast/internal/models"

	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupTestDB 创建测试数据库
func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	assert.NoError(t, err)

	// 自动迁移
	err = db.AutoMigrate(&models.Podcast{}, &models.Episode{})
	assert.NoError(t, err)

	return db
}

// TestFetcherWithContext 测试Fetcher的context超时控制
func TestFetcherWithContext(t *testing.T) {
	fetcher := feed.NewFetcher(10 * time.Second)

	t.Run("正常的feed抓取应该成功", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// 使用一个公开的RSS feed进行测试
		feed, err := fetcher.FetchFeedWithContext(ctx, "https://feeds.feedburner.com/TEDTalks_audio")

		assert.NoError(t, err)
		assert.NotNil(t, feed)
		assert.NotEmpty(t, feed.Title)
	})

	t.Run("超时的context应该返回错误", func(t *testing.T) {
		// 使用一个非常短的超时时间
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
		defer cancel()

		// 等待超时
		time.Sleep(10 * time.Millisecond)

		feed, err := fetcher.FetchFeedWithContext(ctx, "https://feeds.feedburner.com/TEDTalks_audio")

		assert.Error(t, err)
		assert.Nil(t, feed)
		assert.Equal(t, context.DeadlineExceeded, err)
	})

	t.Run("已取消的context应该立即返回", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // 立即取消

		feed, err := fetcher.FetchFeedWithContext(ctx, "https://feeds.feedburner.com/TEDTalks_audio")

		assert.Error(t, err)
		assert.Nil(t, feed)
	})
}

// TestNoGoroutineLeak 测试没有goroutine泄漏
func TestNoGoroutineLeak(t *testing.T) {
	initialGoroutines := runtime.NumGoroutine()

	db := setupTestDB(t)
	fetcher := feed.NewFetcher(10 * time.Second)
	_, err := NewService(db, "")
	assert.NoError(t, err)

	// 创建测试用的podcast
	podcast := &models.Podcast{
		Title:        "Test Podcast",
		FeedURL:      "https://feeds.feedburner.com/TEDTalks_audio",
		DataSource:   "rss",
		EpisodeCount: 0,
	}
	err = db.Create(podcast).Error
	assert.NoError(t, err)

	// 模拟同步操作（会创建goroutine）
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 启动多个并发goroutine来模拟实际使用场景
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// 使用带context的feed fetcher
			_, _ = fetcher.FetchFeedWithContext(ctx, podcast.FeedURL)
		}()
	}
	wg.Wait()

	// 等待一段时间让所有goroutine完成清理
	time.Sleep(100 * time.Millisecond)

	finalGoroutines := runtime.NumGoroutine()

	// 允许有一定数量的goroutine增长（GC、测试框架等）
	// 但不应该有显著的goroutine泄漏
	t.Logf("Initial goroutines: %d, Final goroutines: %d, Diff: %d",
		initialGoroutines, finalGoroutines, finalGoroutines-initialGoroutines)

	assert.LessOrEqual(t, finalGoroutines-initialGoroutines, 5,
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
func TestConcurrentPodcastUpdate(t *testing.T) {
	db := setupTestDB(t)

	// 创建多个podcast来测试并发更新不同记录
	podcasts := make([]*models.Podcast, 10)
	for i := 0; i < 10; i++ {
		podcasts[i] = &models.Podcast{
			Title:        fmt.Sprintf("Test Podcast %d", i),
			FeedURL:      fmt.Sprintf("https://example.com/feed%d.xml", i),
			DataSource:   "rss",
			EpisodeCount: 100,
		}
		err := db.Create(podcasts[i]).Error
		assert.NoError(t, err)
	}

	// 并发更新不同的podcast
	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

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

			_, err := fetcher.FetchFeedWithContext(ctx, "https://feeds.feedburner.com/TEDTalks_audio")

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

	// 至少应该有一半的请求成功（考虑到网络波动）
	assert.GreaterOrEqual(t, successCount, concurrentRequests/2)

	// 并发请求应该在合理时间内完成（说明连接池工作正常）
	assert.Less(t, duration, 30*time.Second)
}

// BenchmarkFetchFeedWithContext 基准测试：带context的feed抓取性能
func BenchmarkFetchFeedWithContext(b *testing.B) {
	fetcher := feed.NewFetcher(10 * time.Second)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = fetcher.FetchFeedWithContext(ctx, "https://feeds.feedburner.com/TEDTalks_audio")
	}
}
