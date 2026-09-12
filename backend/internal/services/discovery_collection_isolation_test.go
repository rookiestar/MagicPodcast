package services

import (
	"testing"
	"time"

	"magicpodcast/internal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 清单独有、尚未被普通同步识别的单集不进入最近更新，即使父节目已关注（#378）。
func TestDiscoveryRecentUpdatesExcludeCollectionOnlyEpisodes(t *testing.T) {
	db := setupDiscoveryTestDB(t)
	service := NewDiscoveryService(db)
	now := time.Now().UTC()

	podcast := createDiscoveryPodcast(t, db, "已关注节目")
	require.NoError(t, db.Model(&models.Podcast{}).Where("id = ?", podcast.ID).
		Update("is_subscribed", true).Error)

	synced := createDiscoveryEpisode(t, db, podcast.ID, "普通同步单集", now.Add(-time.Hour), nil)
	collectionOnly := models.Episode{
		PodcastID:      podcast.ID,
		Title:          "清单独有单集",
		GUID:           "xiaoyuzhoufm:episode:isolated",
		CollectionOnly: true,
	}
	require.NoError(t, db.Create(&collectionOnly).Error)

	candidates, err := service.ListRecentCandidates(100)
	require.NoError(t, err)
	titles := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		titles = append(titles, candidate.EpisodeTitle)
	}
	assert.Contains(t, titles, synced.Title, "正常同步单集保留既有行为")
	assert.NotContains(t, titles, collectionOnly.Title, "清单独有单集不进入最近更新")
}

// 采纳转换资格后进入最近更新；单纯读取不清除标识由同步路径保证（见 sync 包）。
func TestDiscoveryRecentUpdatesIncludeConvertedEpisodes(t *testing.T) {
	db := setupDiscoveryTestDB(t)
	service := NewDiscoveryService(db)
	now := time.Now().UTC()

	podcast := createDiscoveryPodcast(t, db, "已关注节目")
	require.NoError(t, db.Model(&models.Podcast{}).Where("id = ?", podcast.ID).
		Update("is_subscribed", true).Error)
	converted := createDiscoveryEpisode(t, db, podcast.ID, "已转换单集", now.Add(-time.Hour), nil)
	require.NoError(t, db.Model(&models.Episode{}).Where("id = ?", converted.ID).
		Update("collection_only", false).Error)

	candidates, err := service.ListRecentCandidates(100)
	require.NoError(t, err)
	titles := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		titles = append(titles, candidate.EpisodeTitle)
	}
	assert.Contains(t, titles, converted.Title)
}
