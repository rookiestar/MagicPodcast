package workflow

import (
	"sync"
	"testing"

	"magicpodcast/internal/models"

	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupTestDB 创建测试数据库
func setupTestDB(t *testing.T) *gorm.DB {
	// 使用临时文件而不是内存数据库，以支持并发访问
	db, err := gorm.Open(sqlite.Open("file:testdb?mode=memory&cache=shared"), &gorm.Config{})
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
