package services

import (
	"testing"

	"magicpodcast/internal/models"
	"magicpodcast/internal/repository"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestTagRelationService(t *testing.T) {
	db := newTagRelationServiceTestDB(t)
	repos := repository.NewRepositoriesWithDB(db)
	service := NewTagRelationService(repos)

	// 创建测试数据
	// 1. 创建标签
	testTag := &models.Tag{
		Name:  "测试标签",
		Color: "#FF0000",
	}
	if err := db.Create(testTag).Error; err != nil {
		t.Fatalf("创建测试标签失败: %v", err)
	}
	defer db.Delete(testTag)

	// 2. 创建测试播客
	testPodcast := &models.Podcast{
		Title:    "测试播客",
		Author:   "测试作者",
		XYZID:    "test_podcast_xyz_123",
		FeedURL:  "https://example.com/feed.xml",
		CoverURL: "https://example.com/cover.jpg",
	}
	if err := db.Create(testPodcast).Error; err != nil {
		t.Fatalf("创建测试播客失败: %v", err)
	}
	defer db.Delete(testPodcast)

	// 3. 创建测试单集
	testEpisode := &models.Episode{
		PodcastID: testPodcast.ID,
		Title:     "测试单集",
		MediumURL: "https://example.com/episode.mp3",
	}
	if err := db.Create(testEpisode).Error; err != nil {
		t.Fatalf("创建测试单集失败: %v", err)
	}
	defer db.Delete(testEpisode)

	t.Run("AddTagToPodcast", func(t *testing.T) {
		// 添加标签到播客
		result, err := service.AddTag(TargetTypePodcast, testPodcast.ID, testTag.ID)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "标签已添加", result.Message)
		assert.Equal(t, testTag.ID, result.TagID)
		assert.Equal(t, testTag.Name, result.TagName)

		// 验证关联已创建
		var podcast models.Podcast
		err = db.Preload("Tags").First(&podcast, testPodcast.ID).Error
		assert.NoError(t, err)
		assert.Len(t, podcast.Tags, 1)
		assert.Equal(t, testTag.ID, podcast.Tags[0].ID)

		// 重复添加应保持幂等，不创建重复关联
		result, err = service.AddTag(TargetTypePodcast, testPodcast.ID, testTag.ID)
		assert.NoError(t, err)
		assert.NotNil(t, result)

		var count int64
		err = db.Table("podcasts_tags").
			Where("podcast_id = ? AND tag_id = ?", testPodcast.ID, testTag.ID).
			Count(&count).Error
		assert.NoError(t, err)
		assert.Equal(t, int64(1), count)
	})

	t.Run("AddTagToEpisode", func(t *testing.T) {
		// 添加标签到单集
		result, err := service.AddTag(TargetTypeEpisode, testEpisode.ID, testTag.ID)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "标签已添加", result.Message)

		// 验证关联已创建
		var episode models.Episode
		err = db.Preload("Tags").First(&episode, testEpisode.ID).Error
		assert.NoError(t, err)
		assert.Len(t, episode.Tags, 1)
		assert.Equal(t, testTag.ID, episode.Tags[0].ID)

		// 重复添加应保持幂等，不创建重复关联
		result, err = service.AddTag(TargetTypeEpisode, testEpisode.ID, testTag.ID)
		assert.NoError(t, err)
		assert.NotNil(t, result)

		var count int64
		err = db.Table("episodes_tags").
			Where("episode_id = ? AND tag_id = ?", testEpisode.ID, testTag.ID).
			Count(&count).Error
		assert.NoError(t, err)
		assert.Equal(t, int64(1), count)
	})

	t.Run("GetPodcastTags", func(t *testing.T) {
		tags, err := service.GetTags(TargetTypePodcast, testPodcast.ID)
		assert.NoError(t, err)
		assert.Len(t, tags, 1)
		assert.Equal(t, testTag.ID, tags[0].ID)
		assert.Equal(t, testTag.Name, tags[0].Name)
		assert.Equal(t, testTag.Color, tags[0].Color)
	})

	t.Run("GetEpisodeTags", func(t *testing.T) {
		tags, err := service.GetTags(TargetTypeEpisode, testEpisode.ID)
		assert.NoError(t, err)
		assert.Len(t, tags, 1)
		assert.Equal(t, testTag.ID, tags[0].ID)
		assert.Equal(t, testTag.Name, tags[0].Name)
		assert.Equal(t, testTag.Color, tags[0].Color)
	})

	t.Run("RemoveTagFromPodcast", func(t *testing.T) {
		err := service.RemoveTag(TargetTypePodcast, testPodcast.ID, testTag.ID)
		assert.NoError(t, err)

		// 验证关联已删除
		var podcast models.Podcast
		err = db.Preload("Tags").First(&podcast, testPodcast.ID).Error
		assert.NoError(t, err)
		assert.Len(t, podcast.Tags, 0)
	})

	t.Run("RemoveTagFromEpisode", func(t *testing.T) {
		err := service.RemoveTag(TargetTypeEpisode, testEpisode.ID, testTag.ID)
		assert.NoError(t, err)

		// 验证关联已删除
		var episode models.Episode
		err = db.Preload("Tags").First(&episode, testEpisode.ID).Error
		assert.NoError(t, err)
		assert.Len(t, episode.Tags, 0)
	})

	t.Run("NonExistentPodcast", func(t *testing.T) {
		_, err := service.AddTag(TargetTypePodcast, 99999, testTag.ID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "podcast with id '99999' not found")

		err = service.RemoveTag(TargetTypePodcast, 99999, testTag.ID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "podcast with id '99999' not found")

		_, err = service.GetTags(TargetTypePodcast, 99999)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "podcast with id '99999' not found")
	})

	t.Run("NonExistentEpisode", func(t *testing.T) {
		_, err := service.AddTag(TargetTypeEpisode, 99999, testTag.ID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "episode with id '99999' not found")

		err = service.RemoveTag(TargetTypeEpisode, 99999, testTag.ID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "episode with id '99999' not found")

		_, err = service.GetTags(TargetTypeEpisode, 99999)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "episode with id '99999' not found")
	})

	t.Run("NonExistentTag", func(t *testing.T) {
		_, err := service.AddTag(TargetTypePodcast, testPodcast.ID, 99999)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "tag with id '99999' not found")

		err = service.RemoveTag(TargetTypePodcast, testPodcast.ID, 99999)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "tag with id '99999' not found")
	})

	t.Run("InvalidTargetType", func(t *testing.T) {
		_, err := service.AddTag(TargetType("invalid"), testPodcast.ID, testTag.ID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported target type")

		err = service.RemoveTag(TargetType("invalid"), testPodcast.ID, testTag.ID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported target type")

		_, err = service.GetTags(TargetType("invalid"), testPodcast.ID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported target type")
	})
}

func newTagRelationServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.Podcast{}, &models.Episode{}, &models.Tag{}))

	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})

	return db
}
