package repository

import (
	"fmt"
	"magicpodcast/internal/database"
	"magicpodcast/internal/models"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestPodcastRepository_Create(t *testing.T) {
	// 设置测试数据库
	db, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewPodcastRepository(db)

	// 创建测试播客
	podcast := &models.Podcast{
		Title:       "测试播客",
		Author:      "测试作者",
		Description: "这是一个测试播客",
		FeedURL:     "https://example.com/feed.xml",
	}

	err := repo.Create(podcast)
	require.NoError(t, err)
	assert.NotZero(t, podcast.ID)
}

func TestPodcastRepository_GetByID(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewPodcastRepository(db)

	// 创建测试数据
	podcast := &models.Podcast{
		Title:   "测试播客",
		Author:  "测试作者",
		FeedURL: "https://example.com/feed.xml",
	}
	require.NoError(t, repo.Create(podcast))

	// 测试查询
	found, err := repo.GetByID(podcast.ID)
	require.NoError(t, err)
	assert.Equal(t, "测试播客", found.Title)
	assert.Equal(t, "测试作者", found.Author)
}

func TestPodcastRepository_List(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewPodcastRepository(db)

	// 创建多个测试播客
	for i := 1; i <= 5; i++ {
		podcast := &models.Podcast{
			Title:   fmt.Sprintf("播客%d", i),
			Author:  "测试作者",
			FeedURL: fmt.Sprintf("https://example.com/feed%d.xml", i),
		}
		require.NoError(t, repo.Create(podcast))
	}

	// 测试列表查询
	podcasts, total, err := repo.List(PodcastFilters{
		Page:     1,
		PageSize: 10,
	})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, total, int64(5))
	assert.GreaterOrEqual(t, len(podcasts), 5)
}

func TestPodcastRepository_Update(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewPodcastRepository(db)

	// 创建测试播客
	podcast := &models.Podcast{
		Title:   "原标题",
		Author:  "测试作者",
		FeedURL: "https://example.com/feed.xml",
	}
	require.NoError(t, repo.Create(podcast))

	// 更新
	podcast.Title = "新标题"
	err := repo.Update(podcast)
	require.NoError(t, err)

	// 验证
	found, _ := repo.GetByID(podcast.ID)
	assert.Equal(t, "新标题", found.Title)
}

func TestPodcastRepository_Delete(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewPodcastRepository(db)

	// 创建测试播客
	podcast := &models.Podcast{
		Title:   "待删除播客",
		Author:  "测试作者",
		FeedURL: "https://example.com/feed.xml",
	}
	require.NoError(t, repo.Create(podcast))

	// 删除
	err := repo.Delete(podcast.ID)
	require.NoError(t, err)

	// 验证已删除
	_, err = repo.GetByID(podcast.ID)
	assert.Error(t, err)
}

func TestPodcastRepository_Search(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewPodcastRepository(db)

	// 创建测试数据
	podcast1 := &models.Podcast{
		Title:   "科技播客",
		Author:  "测试作者",
		FeedURL: "https://example.com/feed1.xml",
	}
	podcast2 := &models.Podcast{
		Title:   "音乐播客",
		Author:  "测试作者",
		FeedURL: "https://example.com/feed2.xml",
	}
	require.NoError(t, repo.Create(podcast1))
	require.NoError(t, repo.Create(podcast2))

	// 搜索科技相关播客
	podcasts, total, err := repo.Search("科技", 1, 10)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, total, int64(1))
	if len(podcasts) > 0 {
		assert.Contains(t, podcasts[0].Title, "科技")
	}
}

func TestBuildPagination(t *testing.T) {
	tests := []struct {
		name      string
		total     int64
		page      int
		pageSize  int
		wantPages int
	}{
		{
			name:      "正常分页",
			total:     100,
			page:      1,
			pageSize:  10,
			wantPages: 10,
		},
		{
			name:      "有余数",
			total:     105,
			page:      1,
			pageSize:  10,
			wantPages: 11,
		},
		{
			name:      "空结果",
			total:     0,
			page:      1,
			pageSize:  10,
			wantPages: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildPagination(tt.total, tt.page, tt.pageSize)
			assert.Equal(t, tt.wantPages, result.TotalPages)
			assert.Equal(t, tt.total, result.Total)
			assert.Equal(t, tt.page, result.Page)
			assert.Equal(t, tt.pageSize, result.PageSize)
		})
	}
}

// setupTestDB 设置测试数据库
func setupTestDB(t *testing.T) (*gorm.DB, func()) {
	// 使用内存数据库进行测试
	db := database.GetDB()

	// 清理测试数据
	database.GetDB().Where("1 = 1").Delete(&models.Podcast{})

	cleanup := func() {
		// 清理测试数据
		database.GetDB().Where("title LIKE ?", "测试%").Delete(&models.Podcast{})
	}

	return db, cleanup
}

// 注意：需要在文件开头添加导入
// "fmt"
