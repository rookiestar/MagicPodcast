package repository

import (
	
	"magicpodcast/internal/models"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEpisodeRepository_Create(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewEpisodeRepository(db)

	// 先创建播客
	podcast := &models.Podcast{
		Title:   "测试播客",
		FeedURL: "https://example.com/feed.xml",
	}
	require.NoError(t, db.Create(podcast).Error)

	// 创建测试单集
	episode := &models.Episode{
		PodcastID:     podcast.ID,
		EpisodeNo:     "1",
		Title:         "测试单集",
		MediumURL:     "https://example.com/episode1.mp3",
		PublishedDate: time.Now(),
	}

	err := repo.Create(episode)
	require.NoError(t, err)
	assert.NotZero(t, episode.ID)
}

func TestEpisodeRepository_BatchCreate(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewEpisodeRepository(db)

	// 先创建播客
	podcast := &models.Podcast{
		Title:   "测试播客",
		FeedURL: "https://example.com/feed.xml",
	}
	require.NoError(t, db.Create(podcast).Error)

	// 创建多个测试单集
	episodes := []*models.Episode{
		{
			PodcastID:     podcast.ID,
			EpisodeNo:     "1",
			Title:         "单集1",
			MediumURL:     "https://example.com/ep1.mp3",
			PublishedDate: time.Now(),
		},
		{
			PodcastID:     podcast.ID,
			EpisodeNo:     "2",
			Title:         "单集2",
			MediumURL:     "https://example.com/ep2.mp3",
			PublishedDate: time.Now(),
		},
		{
			PodcastID:     podcast.ID,
			EpisodeNo:     "3",
			Title:         "单集3",
			MediumURL:     "https://example.com/ep3.mp3",
			PublishedDate: time.Now(),
		},
	}

	err := repo.BatchCreate(episodes)
	require.NoError(t, err)

	// 验证
	var count int64
	db.Model(&models.Episode{}).Where("podcast_id = ?", podcast.ID).Count(&count)
	assert.Equal(t, int64(3), count)
}

func TestEpisodeRepository_GetByID(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewEpisodeRepository(db)

	// 创建测试数据
	podcast := &models.Podcast{
		Title:   "测试播客",
		FeedURL: "https://example.com/feed.xml",
	}
	require.NoError(t, db.Create(podcast).Error)

	episode := &models.Episode{
		PodcastID:     podcast.ID,
		EpisodeNo:     "1",
		Title:         "测试单集",
		MediumURL:     "https://example.com/episode1.mp3",
		PublishedDate: time.Now(),
	}
	require.NoError(t, repo.Create(episode))

	// 测试查询
	found, err := repo.GetByID(episode.ID)
	require.NoError(t, err)
	assert.Equal(t, "测试单集", found.Title)
	assert.Equal(t, podcast.ID, found.PodcastID)
}

func TestEpisodeRepository_List(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewEpisodeRepository(db)

	// 先创建播客
	podcast := &models.Podcast{
		Title:   "测试播客",
		FeedURL: "https://example.com/feed.xml",
	}
	require.NoError(t, db.Create(podcast).Error)

	// 创建多个测试单集
	for i := 1; i <= 5; i++ {
		episode := &models.Episode{
			PodcastID:     podcast.ID,
			EpisodeNo:     string(rune(i + '0')),
			Title:         "测试单集",
			MediumURL:     "https://example.com/episode1.mp3",
			PublishedDate: time.Now(),
		}
		require.NoError(t, repo.Create(episode))
	}

	// 测试列表查询
	episodes, total, err := repo.List(EpisodeFilters{
		Page:     1,
		PageSize: 10,
	})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, total, int64(5))
	assert.GreaterOrEqual(t, len(episodes), 5)
}

func TestEpisodeRepository_List_WithPodcastFilter(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewEpisodeRepository(db)

	// 创建两个播客
	podcast1 := &models.Podcast{
		Title:   "播客1",
		FeedURL: "https://example.com/feed1.xml",
	}
	podcast2 := &models.Podcast{
		Title:   "播客2",
		FeedURL: "https://example.com/feed2.xml",
	}
	require.NoError(t, db.Create(podcast1).Error)
	require.NoError(t, db.Create(podcast2).Error)

	// 为每个播客创建单集
	episode1 := &models.Episode{
		PodcastID:     podcast1.ID,
		Title:         "播客1的单集",
		MediumURL:     "https://example.com/ep1.mp3",
		PublishedDate: time.Now(),
	}
	episode2 := &models.Episode{
		PodcastID:     podcast2.ID,
		Title:         "播客2的单集",
		MediumURL:     "https://example.com/ep2.mp3",
		PublishedDate: time.Now(),
	}
	require.NoError(t, repo.Create(episode1))
	require.NoError(t, repo.Create(episode2))

	// 测试按播客筛选
	episodes, total, err := repo.List(EpisodeFilters{
		PodcastID: &podcast1.ID,
		Page:      1,
		PageSize:  10,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Equal(t, 1, len(episodes))
	assert.Equal(t, "播客1的单集", episodes[0].Title)
}

func TestEpisodeRepository_List_WithSearch(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewEpisodeRepository(db)

	// 先创建播客
	podcast := &models.Podcast{
		Title:   "测试播客",
		FeedURL: "https://example.com/feed.xml",
	}
	require.NoError(t, db.Create(podcast).Error)

	// 创建测试单集
	episode1 := &models.Episode{
		PodcastID:     podcast.ID,
		Title:         "人工智能技术",
		MediumURL:     "https://example.com/ep1.mp3",
		ShowNotes:     "讨论 AI 技术发展",
		PublishedDate: time.Now(),
	}
	episode2 := &models.Episode{
		PodcastID:     podcast.ID,
		Title:         "音乐欣赏",
		MediumURL:     "https://example.com/ep2.mp3",
		ShowNotes:     "经典音乐作品",
		PublishedDate: time.Now(),
	}
	require.NoError(t, repo.Create(episode1))
	require.NoError(t, repo.Create(episode2))

	// 搜索"人工智能"
	episodes, total, err := repo.List(EpisodeFilters{
		Search:   "人工智能",
		Page:     1,
		PageSize: 10,
	})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, total, int64(1))
	if len(episodes) > 0 {
		assert.Contains(t, episodes[0].Title, "人工智能")
	}
}

func TestEpisodeRepository_Update(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewEpisodeRepository(db)

	// 创建测试数据
	podcast := &models.Podcast{
		Title:   "测试播客",
		FeedURL: "https://example.com/feed.xml",
	}
	require.NoError(t, db.Create(podcast).Error)

	episode := &models.Episode{
		PodcastID:     podcast.ID,
		Title:         "原标题",
		MediumURL:     "https://example.com/episode1.mp3",
		PublishedDate: time.Now(),
	}
	require.NoError(t, repo.Create(episode))

	// 更新
	episode.Title = "新标题"
	err := repo.Update(episode)
	require.NoError(t, err)

	// 验证
	found, _ := repo.GetByID(episode.ID)
	assert.Equal(t, "新标题", found.Title)
}

func TestEpisodeRepository_Delete(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewEpisodeRepository(db)

	// 创建测试数据
	podcast := &models.Podcast{
		Title:   "测试播客",
		FeedURL: "https://example.com/feed.xml",
	}
	require.NoError(t, db.Create(podcast).Error)

	episode := &models.Episode{
		PodcastID:     podcast.ID,
		Title:         "待删除单集",
		MediumURL:     "https://example.com/episode1.mp3",
		PublishedDate: time.Now(),
	}
	require.NoError(t, repo.Create(episode))

	// 删除
	err := repo.Delete(episode.ID)
	require.NoError(t, err)

	// 验证已删除
	_, err = repo.GetByID(episode.ID)
	assert.Error(t, err)
}

func TestEpisodeRepository_GetByPodcastID(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewEpisodeRepository(db)

	// 创建播客
	podcast := &models.Podcast{
		Title:   "测试播客",
		FeedURL: "https://example.com/feed.xml",
	}
	require.NoError(t, db.Create(podcast).Error)

	// 创建多个单集
	for i := 1; i <= 3; i++ {
		episode := &models.Episode{
			PodcastID:     podcast.ID,
			EpisodeNo:     string(rune(i + '0')),
			Title:         "测试单集",
			MediumURL:     "https://example.com/episode1.mp3",
			PublishedDate: time.Now(),
		}
		require.NoError(t, repo.Create(episode))
	}

	// 查询播客的单集
	episodes, total, err := repo.GetByPodcastID(podcast.ID, 1, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(3), total)
	assert.Equal(t, 3, len(episodes))
}

func TestEpisodeRepository_Search(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewEpisodeRepository(db)

	// 创建播客
	podcast := &models.Podcast{
		Title:   "测试播客",
		FeedURL: "https://example.com/feed.xml",
	}
	require.NoError(t, db.Create(podcast).Error)

	// 创建测试单集
	episode1 := &models.Episode{
		PodcastID:     podcast.ID,
		Title:         "机器学习入门",
		MediumURL:     "https://example.com/ep1.mp3",
		PublishedDate: time.Now(),
	}
	episode2 := &models.Episode{
		PodcastID:     podcast.ID,
		Title:         "音乐欣赏",
		MediumURL:     "https://example.com/ep2.mp3",
		PublishedDate: time.Now(),
	}
	require.NoError(t, repo.Create(episode1))
	require.NoError(t, repo.Create(episode2))

	// 搜索"机器学习"
	episodes, total, err := repo.Search("机器学习", 1, 10)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, total, int64(1))
	if len(episodes) > 0 {
		assert.Contains(t, episodes[0].Title, "机器学习")
	}
}

func TestEpisodeRepository_BatchCreateWithTx(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewEpisodeRepository(db)

	// 创建播客
	podcast := &models.Podcast{
		Title:   "测试播客",
		FeedURL: "https://example.com/feed.xml",
	}
	require.NoError(t, db.Create(podcast).Error)

	// 开始事务
	tx := db.Begin()
	defer tx.Rollback()

	// 在事务中批量创建
	episodes := []*models.Episode{
		{
			PodcastID:     podcast.ID,
			Title:         "单集1",
			MediumURL:     "https://example.com/ep1.mp3",
			PublishedDate: time.Now(),
		},
		{
			PodcastID:     podcast.ID,
			Title:         "单集2",
			MediumURL:     "https://example.com/ep2.mp3",
			PublishedDate: time.Now(),
		},
	}

	err := repo.BatchCreateWithTx(tx, episodes)
	require.NoError(t, err)

	// 在事务中验证
	var count int64
	tx.Model(&models.Episode{}).Where("podcast_id = ?", podcast.ID).Count(&count)
	assert.Equal(t, int64(2), count)

	// 回滚后不应该有数据
	// 这个测试主要是为了验证事务功能正常工作
}
