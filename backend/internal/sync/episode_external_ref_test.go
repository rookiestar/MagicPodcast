package sync

import (
	"testing"
	"time"

	"magicpodcast/internal/models"

	"github.com/mmcdole/gofeed"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const refTestEID = "6a1c07b0ac7bdb080c3397a9"

func refTestItem(link string) *gofeed.Item {
	return &gofeed.Item{
		GUID:            "rss-guid-original-1",
		Link:            link,
		Title:           "E185 芯片规律 × AI浪潮",
		Description:     "RSS 版 Show Notes",
		PublishedParsed: ptrTime(time.Date(2026, 6, 1, 8, 0, 0, 0, time.UTC)),
		Enclosures:      []*gofeed.Enclosure{{URL: "https://media.example.com/e185.m4a", Type: "audio/mp4"}},
	}
}

func createCollectionAdoptedEpisode(t *testing.T, db *gorm.DB, podcastID uint) models.Episode {
	t.Helper()
	episode := models.Episode{
		PodcastID:      podcastID,
		Title:          "E185 芯片规律 × AI浪潮",
		GUID:           "xiaoyuzhoufm:episode:" + refTestEID,
		Link:           "https://www.xiaoyuzhoufm.com/episode/" + refTestEID,
		ShowNotes:      "清单快照 Show Notes",
		Notes:          "我的个人笔记",
		CollectionOnly: true,
		PublishedDate:  time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
	}
	require.NoError(t, db.Create(&episode).Error)
	require.NoError(t, db.Create(&models.EpisodeExternalRef{
		SourcePlatform:    models.SourcePlatformXiaoyuzhoufm,
		ExternalEpisodeID: refTestEID,
		ExternalPodcastID: "643cdf1ad3d94ec2ad39ae94",
		EpisodeID:         episode.ID,
		PodcastID:         podcastID,
	}).Error)
	focus := models.QueueStateInbox
	require.NoError(t, db.Create(&models.EpisodeTriageDecision{
		EpisodeID:  episode.ID,
		State:      models.TriageStateShortlisted,
		DecidedAt:  time.Now().UTC(),
		QueueState: &focus,
	}).Error)
	return episode
}

func TestSyncReusesCollectionAdoptedEpisodeViaExternalRef(t *testing.T) {
	db := setupTestDB(t)
	service, err := NewService(db, "")
	require.NoError(t, err)
	t.Cleanup(func() { _ = service.Close() })

	podcast := models.Podcast{
		Title:        "投资实战派",
		XYZID:        "643cdf1ad3d94ec2ad39ae94",
		FeedURL:      "https://example.com/tzsp.xml",
		IsSubscribed: true,
	}
	require.NoError(t, db.Create(&podcast).Error)
	adopted := createCollectionAdoptedEpisode(t, db, podcast.ID)

	config := EpisodeSyncConfig{Mode: SyncModeFull, UpdateExisting: true}
	result, err := service.syncPodcastEpisodeItems(&podcast, []*gofeed.Item{refTestItem(adopted.Link)}, config)
	require.NoError(t, err)
	assert.Equal(t, 0, result.Created, "后续 RSS 不得重复建集")
	assert.Equal(t, 1, result.Skipped)

	var episodes []models.Episode
	require.NoError(t, db.Order("id").Find(&episodes).Error)
	require.Len(t, episodes, 1, "同一集只有一份本地身份")
	assert.Equal(t, adopted.ID, episodes[0].ID)
	assert.Equal(t, "xiaoyuzhoufm:episode:"+refTestEID, episodes[0].GUID, "保留清单收录时的身份")
	assert.False(t, episodes[0].CollectionOnly, "普通同步实际识别后转换资格")
	assert.Equal(t, "我的个人笔记", episodes[0].Notes, "用户笔记不被同步覆盖")

	var decision models.EpisodeTriageDecision
	require.NoError(t, db.Where("episode_id = ?", adopted.ID).First(&decision).Error)
	assert.Equal(t, models.QueueStateInbox, *decision.QueueState, "队列保持")
}

func TestSyncRefMatchDoesNotRefreshTimeWithoutContentChange(t *testing.T) {
	db := setupTestDB(t)
	service, err := NewService(db, "")
	require.NoError(t, err)
	t.Cleanup(func() { _ = service.Close() })

	podcast := models.Podcast{
		Title: "投资实战派", XYZID: "643cdf1ad3d94ec2ad39ae94",
		FeedURL: "https://example.com/tzsp.xml", IsSubscribed: true,
	}
	require.NoError(t, db.Create(&podcast).Error)
	adopted := createCollectionAdoptedEpisode(t, db, podcast.ID)
	item := refTestItem(adopted.Link)

	config := EpisodeSyncConfig{Mode: SyncModeFull, UpdateExisting: true}
	_, err = service.syncPodcastEpisodeItems(&podcast, []*gofeed.Item{item}, config)
	require.NoError(t, err)

	var first models.Episode
	require.NoError(t, db.First(&first, adopted.ID).Error)

	// 无变化轮询：再次同步不刷新时间字段。
	time.Sleep(10 * time.Millisecond)
	_, err = service.syncPodcastEpisodeItems(&podcast, []*gofeed.Item{item}, config)
	require.NoError(t, err)
	var second models.Episode
	require.NoError(t, db.First(&second, adopted.ID).Error)
	assert.Equal(t, first.UpdatedAt, second.UpdatedAt, "无变化轮询不刷新单集时间")
	assert.Equal(t, first.FetchedAt, second.FetchedAt)
}

func TestSyncExternalRefBelongingToAnotherPodcastIsRejected(t *testing.T) {
	db := setupTestDB(t)
	service, err := NewService(db, "")
	require.NoError(t, err)
	t.Cleanup(func() { _ = service.Close() })

	owner := models.Podcast{Title: "归属节目", XYZID: "pid-owner", FeedURL: "https://example.com/owner.xml", IsSubscribed: true}
	require.NoError(t, db.Create(&owner).Error)
	other := models.Podcast{Title: "另一档节目", XYZID: "pid-other", FeedURL: "https://example.com/other.xml", IsSubscribed: true}
	require.NoError(t, db.Create(&other).Error)
	adopted := createCollectionAdoptedEpisode(t, db, owner.ID)

	// 另一档节目的 Feed 突然给出同一小宇宙链接：保持跨节目拒绝行为。
	config := EpisodeSyncConfig{Mode: SyncModeFull, UpdateExisting: true}
	result, err := service.syncPodcastEpisodeItems(&other, []*gofeed.Item{refTestItem(adopted.Link)}, config)
	require.Error(t, err)
	assert.Equal(t, 1, result.Errors)

	var episodes []models.Episode
	require.NoError(t, db.Find(&episodes).Error)
	require.Len(t, episodes, 1)
	assert.Equal(t, owner.ID, episodes[0].PodcastID, "不跨节目合并")
}

func TestSyncUnrecognizedRSSItemCreatesNormalEpisode(t *testing.T) {
	db := setupTestDB(t)
	service, err := NewService(db, "")
	require.NoError(t, err)
	t.Cleanup(func() { _ = service.Close() })

	podcast := models.Podcast{
		Title: "投资实战派", XYZID: "643cdf1ad3d94ec2ad39ae94",
		FeedURL: "https://example.com/tzsp.xml", IsSubscribed: true,
	}
	require.NoError(t, db.Create(&podcast).Error)
	adopted := createCollectionAdoptedEpisode(t, db, podcast.ID)

	// 无可靠键关联的 RSS 条目：不与清单独有集误合并，按普通新增创建。
	item := &gofeed.Item{
		GUID:            "rss-brand-new-guid",
		Link:            "https://example.com/episode/xyz-999",
		Title:           "E186 全新单集",
		PublishedParsed: ptrTime(time.Date(2026, 6, 8, 8, 0, 0, 0, time.UTC)),
	}
	config := EpisodeSyncConfig{Mode: SyncModeFull, UpdateExisting: true}
	result, err := service.syncPodcastEpisodeItems(&podcast, []*gofeed.Item{item, refTestItem(adopted.Link)}, config)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Created, "无法可靠对应的条目按普通新增处理")
	assert.Equal(t, 1, result.Skipped, "可靠对应的条目复用同一集")

	var adoptedAfter models.Episode
	require.NoError(t, db.First(&adoptedAfter, adopted.ID).Error)
	assert.False(t, adoptedAfter.CollectionOnly)
}
