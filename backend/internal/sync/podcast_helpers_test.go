package sync

import (
	"testing"

	"magicpodcast/internal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSaveOrUpdatePodcastPreservesUserFieldsOnUpdate(t *testing.T) {
	db := setupTestDB(t)
	service, err := NewService(db, "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close()) })

	existing := &models.Podcast{
		XYZID:          "cover-keep",
		Title:          "旧标题",
		FeedURL:        "https://example.com/feed.xml",
		CustomCoverURL: "https://example.com/custom.jpg",
		Notes:          "我的备注",
		MyRate:         5,
		IsDead:         true,
		IsSubscribed:   true,
		EpisodeCount:   3,
	}
	require.NoError(t, db.Create(existing).Error)

	incoming := &models.Podcast{
		Title:        "新标题",
		FeedURL:      "https://example.com/feed.xml",
		Description:  "from opml",
		CoverURL:     "https://cdn.example/cover.jpg",
		EpisodeCount: 10,
		FeedURLValid: true,
		IsSubscribed: true,
		DataSource:   "rss",
	}
	require.NoError(t, service.saveOrUpdatePodcast(incoming))

	var got models.Podcast
	require.NoError(t, db.First(&got, existing.ID).Error)
	assert.Equal(t, "新标题", got.Title)
	assert.Equal(t, 10, got.EpisodeCount)
	assert.Equal(t, "https://example.com/custom.jpg", got.CustomCoverURL)
	assert.Equal(t, "我的备注", got.Notes)
	assert.Equal(t, 5, got.MyRate)
	assert.True(t, got.IsDead)
	assert.Equal(t, "cover-keep", got.XYZID)
}

func TestSaveOrUpdatePodcastDoesNotOverwriteExistingWithStub(t *testing.T) {
	db := setupTestDB(t)
	service, err := NewService(db, "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close()) })

	existing := &models.Podcast{
		XYZID:        "keep-good-row",
		Title:        "已有节目",
		FeedURL:      "https://example.com/live.xml",
		EpisodeCount: 8,
		FeedURLValid: true,
		IsSubscribed: true,
		CoverURL:     "https://cdn.example/ok.jpg",
	}
	require.NoError(t, db.Create(existing).Error)

	stub := &models.Podcast{
		Title:        "已有节目",
		FeedURL:      "https://example.com/live.xml",
		IsSubscribed: true,
		DataSource:   "rss",
		FeedURLValid: false,
		EpisodeCount: 0,
	}
	require.NoError(t, service.saveOrUpdatePodcast(stub))

	var got models.Podcast
	require.NoError(t, db.First(&got, existing.ID).Error)
	assert.Equal(t, "已有节目", got.Title)
	assert.Equal(t, 8, got.EpisodeCount)
	assert.True(t, got.FeedURLValid)
	assert.Equal(t, "https://cdn.example/ok.jpg", got.CoverURL)
}
