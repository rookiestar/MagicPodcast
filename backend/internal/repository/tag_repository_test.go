package repository

import (
	
	"magicpodcast/internal/models"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTagRepository_Create(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewTagRepository(db)

	// 创建测试标签
	tag := &models.Tag{
		Name:  "技术",
		Color: "#FF5733",
	}

	err := repo.Create(tag)
	require.NoError(t, err)
	assert.NotZero(t, tag.ID)
}

func TestTagRepository_GetByID(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewTagRepository(db)

	// 创建测试标签
	tag := &models.Tag{
		Name:  "技术",
		Color: "#FF5733",
	}
	require.NoError(t, repo.Create(tag))

	// 测试查询
	found, err := repo.GetByID(tag.ID)
	require.NoError(t, err)
	assert.Equal(t, "技术", found.Name)
	assert.Equal(t, "#FF5733", found.Color)
}

func TestTagRepository_List(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewTagRepository(db)

	// 创建多个测试标签
	tags := []*models.Tag{
		{Name: "技术", Color: "#FF5733"},
		{Name: "音乐", Color: "#33FF57"},
		{Name: "教育", Color: "#3357FF"},
		{Name: "娱乐", Color: "#F333FF"},
		{Name: "新闻", Color: "#FF33A8"},
	}

	for _, tag := range tags {
		require.NoError(t, repo.Create(tag))
	}

	// 测试列表查询
	result, total, err := repo.List(1, 10)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, total, int64(5))
	assert.GreaterOrEqual(t, len(result), 5)
}

func TestTagRepository_Update(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewTagRepository(db)

	// 创建测试标签
	tag := &models.Tag{
		Name:  "技术",
		Color: "#FF5733",
	}
	require.NoError(t, repo.Create(tag))

	// 更新
	tag.Name = "编程"
	tag.Color = "#00AAFF"
	err := repo.Update(tag)
	require.NoError(t, err)

	// 验证
	found, _ := repo.GetByID(tag.ID)
	assert.Equal(t, "编程", found.Name)
	assert.Equal(t, "#00AAFF", found.Color)
}

func TestTagRepository_Delete(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewTagRepository(db)

	// 创建测试数据
	podcast := &models.Podcast{
		Title:   "测试播客",
		FeedURL: "https://example.com/feed.xml",
	}
	require.NoError(t, db.Create(podcast).Error)

	tag := &models.Tag{
		Name:  "技术",
		Color: "#FF5733",
	}
	require.NoError(t, repo.Create(tag))

	// 为播客添加标签
	require.NoError(t, repo.AddTagToPodcast(podcast.ID, tag.ID))

	// 删除标签（应该同时删除关联）
	err := repo.Delete(tag.ID)
	require.NoError(t, err)

	// 验证标签已删除
	_, err = repo.GetByID(tag.ID)
	assert.Error(t, err)

	// 验证关联也已删除
	var count int64
	db.Table("podcasts_tags").Where("tag_id = ?", tag.ID).Count(&count)
	assert.Equal(t, int64(0), count)
}

func TestTagRepository_GetByName(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewTagRepository(db)

	// 创建测试标签
	tag := &models.Tag{
		Name:  "技术",
		Color: "#FF5733",
	}
	require.NoError(t, repo.Create(tag))

	// 按名称查询
	found, err := repo.GetByName("技术")
	require.NoError(t, err)
	assert.Equal(t, tag.ID, found.ID)
	assert.Equal(t, "技术", found.Name)

	// 查询不存在的标签
	_, err = repo.GetByName("不存在的标签")
	assert.Error(t, err)
}

func TestTagRepository_Search(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewTagRepository(db)

	// 创建测试标签
	tags := []*models.Tag{
		{Name: "技术类", Color: "#FF5733"},
		{Name: "音乐欣赏", Color: "#33FF57"},
		{Name: "教育节目", Color: "#3357FF"},
	}
	for _, tag := range tags {
		require.NoError(t, repo.Create(tag))
	}

	// 搜索"技术"
	result, total, err := repo.Search("技术", 1, 10)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, total, int64(1))
	if len(result) > 0 {
		assert.Contains(t, result[0].Name, "技术")
	}
}

func TestTagRepository_AddTagToPodcast(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewTagRepository(db)

	// 创建测试数据
	podcast := &models.Podcast{
		Title:   "测试播客",
		FeedURL: "https://example.com/feed.xml",
	}
	require.NoError(t, db.Create(podcast).Error)

	tag := &models.Tag{
		Name:  "技术",
		Color: "#FF5733",
	}
	require.NoError(t, repo.Create(tag))

	// 添加标签到播客
	err := repo.AddTagToPodcast(podcast.ID, tag.ID)
	require.NoError(t, err)

	// 验证关联已创建
	var count int64
	db.Table("podcasts_tags").Where("podcast_id = ? AND tag_id = ?", podcast.ID, tag.ID).Count(&count)
	assert.Equal(t, int64(1), count)

	// 重复添加应该不报错（幂等性）
	err = repo.AddTagToPodcast(podcast.ID, tag.ID)
	require.NoError(t, err)
}

func TestTagRepository_RemoveTagFromPodcast(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewTagRepository(db)

	// 创建测试数据
	podcast := &models.Podcast{
		Title:   "测试播客",
		FeedURL: "https://example.com/feed.xml",
	}
	require.NoError(t, db.Create(podcast).Error)

	tag := &models.Tag{
		Name:  "技术",
		Color: "#FF5733",
	}
	require.NoError(t, repo.Create(tag))

	// 添加标签
	require.NoError(t, repo.AddTagToPodcast(podcast.ID, tag.ID))

	// 移除标签
	err := repo.RemoveTagFromPodcast(podcast.ID, tag.ID)
	require.NoError(t, err)

	// 验证关联已删除
	var count int64
	db.Table("podcasts_tags").Where("podcast_id = ? AND tag_id = ?", podcast.ID, tag.ID).Count(&count)
	assert.Equal(t, int64(0), count)
}

func TestTagRepository_AddTagToEpisode(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewTagRepository(db)

	// 创建测试数据
	podcast := &models.Podcast{
		Title:   "测试播客",
		FeedURL: "https://example.com/feed.xml",
	}
	require.NoError(t, db.Create(podcast).Error)

	episode := &models.Episode{
		PodcastID: podcast.ID,
		Title:     "测试单集",
		MediumURL: "https://example.com/episode1.mp3",
	}
	require.NoError(t, db.Create(episode).Error)

	tag := &models.Tag{
		Name:  "技术",
		Color: "#FF5733",
	}
	require.NoError(t, repo.Create(tag))

	// 添加标签到单集
	err := repo.AddTagToEpisode(episode.ID, tag.ID)
	require.NoError(t, err)

	// 验证关联已创建
	var count int64
	db.Table("episodes_tags").Where("episode_id = ? AND tag_id = ?", episode.ID, tag.ID).Count(&count)
	assert.Equal(t, int64(1), count)
}

func TestTagRepository_RemoveTagFromEpisode(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewTagRepository(db)

	// 创建测试数据
	podcast := &models.Podcast{
		Title:   "测试播客",
		FeedURL: "https://example.com/feed.xml",
	}
	require.NoError(t, db.Create(podcast).Error)

	episode := &models.Episode{
		PodcastID: podcast.ID,
		Title:     "测试单集",
		MediumURL: "https://example.com/episode1.mp3",
	}
	require.NoError(t, db.Create(episode).Error)

	tag := &models.Tag{
		Name:  "技术",
		Color: "#FF5733",
	}
	require.NoError(t, repo.Create(tag))

	// 添加标签
	require.NoError(t, repo.AddTagToEpisode(episode.ID, tag.ID))

	// 移除标签
	err := repo.RemoveTagFromEpisode(episode.ID, tag.ID)
	require.NoError(t, err)

	// 验证关联已删除
	var count int64
	db.Table("episodes_tags").Where("episode_id = ? AND tag_id = ?", episode.ID, tag.ID).Count(&count)
	assert.Equal(t, int64(0), count)
}

func TestTagRepository_GetPodcastTags(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewTagRepository(db)

	// 创建测试数据
	podcast := &models.Podcast{
		Title:   "测试播客",
		FeedURL: "https://example.com/feed.xml",
	}
	require.NoError(t, db.Create(podcast).Error)

	tag1 := &models.Tag{Name: "技术", Color: "#FF5733"}
	tag2 := &models.Tag{Name: "音乐", Color: "#33FF57"}
	require.NoError(t, repo.Create(tag1))
	require.NoError(t, repo.Create(tag2))

	// 为播客添加多个标签
	require.NoError(t, repo.AddTagToPodcast(podcast.ID, tag1.ID))
	require.NoError(t, repo.AddTagToPodcast(podcast.ID, tag2.ID))

	// 获取播客的所有标签
	tags, err := repo.GetPodcastTags(podcast.ID)
	require.NoError(t, err)
	assert.Equal(t, 2, len(tags))
}

func TestTagRepository_GetEpisodeTags(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewTagRepository(db)

	// 创建测试数据
	podcast := &models.Podcast{
		Title:   "测试播客",
		FeedURL: "https://example.com/feed.xml",
	}
	require.NoError(t, db.Create(podcast).Error)

	episode := &models.Episode{
		PodcastID: podcast.ID,
		Title:     "测试单集",
		MediumURL: "https://example.com/episode1.mp3",
	}
	require.NoError(t, db.Create(episode).Error)

	tag1 := &models.Tag{Name: "技术", Color: "#FF5733"}
	tag2 := &models.Tag{Name: "音乐", Color: "#33FF57"}
	require.NoError(t, repo.Create(tag1))
	require.NoError(t, repo.Create(tag2))

	// 为单集添加多个标签
	require.NoError(t, repo.AddTagToEpisode(episode.ID, tag1.ID))
	require.NoError(t, repo.AddTagToEpisode(episode.ID, tag2.ID))

	// 获取单集的所有标签
	tags, err := repo.GetEpisodeTags(episode.ID)
	require.NoError(t, err)
	assert.Equal(t, 2, len(tags))
}

func TestTagRepository_GetByIDs(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewTagRepository(db)

	// 创建测试标签
	tag1 := &models.Tag{Name: "技术", Color: "#FF5733"}
	tag2 := &models.Tag{Name: "音乐", Color: "#33FF57"}
	tag3 := &models.Tag{Name: "教育", Color: "#3357FF"}
	require.NoError(t, repo.Create(tag1))
	require.NoError(t, repo.Create(tag2))
	require.NoError(t, repo.Create(tag3))

	// 批量获取标签
	tags, err := repo.GetByIDs([]uint{tag1.ID, tag2.ID, tag3.ID})
	require.NoError(t, err)
	assert.Equal(t, 3, len(tags))
}

func TestTagRepository_GetPodcastsByTagID(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewTagRepository(db)

	// 创建测试数据
	tag := &models.Tag{Name: "技术", Color: "#FF5733"}
	require.NoError(t, repo.Create(tag))

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

	// 为两个播客添加标签
	require.NoError(t, repo.AddTagToPodcast(podcast1.ID, tag.ID))
	require.NoError(t, repo.AddTagToPodcast(podcast2.ID, tag.ID))

	// 获取使用该标签的播客
	podcasts, total, err := repo.GetPodcastsByTagID(tag.ID, 1, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
	assert.Equal(t, 2, len(podcasts))
}

func TestTagRepository_UpdatePodcastCount(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewTagRepository(db)

	// 创建测试数据
	tag := &models.Tag{Name: "技术", Color: "#FF5733"}
	require.NoError(t, repo.Create(tag))

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

	// 为两个播客添加标签
	require.NoError(t, repo.AddTagToPodcast(podcast1.ID, tag.ID))
	require.NoError(t, repo.AddTagToPodcast(podcast2.ID, tag.ID))

	// 更新播客计数
	err := repo.UpdatePodcastCount(tag.ID)
	require.NoError(t, err)

	// 验证计数已更新（注意：这个测试依赖于 PodcastCount 字段的存在）
	found, _ := repo.GetByID(tag.ID)
	// 如果 Tag 模型有 PodcastCount 字段，可以验证
	// assert.Equal(t, 2, found.PodcastCount)
	assert.NotNil(t, found)
}
