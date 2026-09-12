package services

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"magicpodcast/internal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const adoptionSampleEID = "6a1c07b0ac7bdb080c3397a9"
const adoptionSamplePID = "643cdf1ad3d94ec2ad39ae94"

func setupAdoptionService(t *testing.T) (*CollectionAdoptionService, *gorm.DB, models.EpisodeCollection) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:adoption_%d?mode=memory&cache=shared", time.Now().UnixNano())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&models.Podcast{},
		&models.Episode{},
		&models.EpisodeTriageDecision{},
		&models.ConsumptionQueueOrder{},
		&models.EpisodeCompletion{},
		&models.EpisodeCollection{},
		&models.EpisodeCollectionItem{},
		&models.EpisodeExternalRef{},
		&models.EpisodeCollectionAdoption{},
	))
	require.NoError(t, db.Create(&[]models.ConsumptionQueueOrder{
		{QueueState: models.QueueStateInbox, Revision: 1},
		{QueueState: models.QueueStateFocus, Revision: 1},
		{QueueState: models.QueueStateSomeday, Revision: 1},
		{QueueState: models.QueueStateDone, Revision: 1},
	}).Error)
	t.Cleanup(func() {
		sqlDB, _ := db.DB()
		_ = sqlDB.Close()
	})

	collection := models.EpisodeCollection{
		SourcePlatform: models.SourcePlatformXiaoyuzhoufm,
		ExternalID:     "6a20323b78a52c96d821a769",
		Title:          "穿透半导体迷雾",
		Author:         "小宇宙领航员",
		SourceURL:      "https://www.xiaoyuzhoufm.com/collection/episode/6a20323b78a52c96d821a769",
	}
	require.NoError(t, db.Create(&collection).Error)

	item := models.EpisodeCollectionItem{
		CollectionID:        collection.ID,
		Position:            0,
		ExternalEpisodeID:   adoptionSampleEID,
		ExternalPodcastID:   adoptionSamplePID,
		PodcastTitle:        "投资实战派",
		PodcastAuthor:       "wong永庆",
		PodcastEpisodeCount: 200,
		EpisodeTitle:        "E185 芯片规律 × AI浪潮",
		Recommendation:      "存储芯片为何五年内持续短缺？",
		Shownotes:           "<p>时间轴</p>",
		Duration:            4692,
		PublishedAt:         ptrTime(time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)),
		ImageURL:            "https://image.xyzcdn.net/cover.jpg",
		EpisodeURL:          "https://www.xiaoyuzhoufm.com/episode/" + adoptionSampleEID,
		PayType:             "FREE",
		AudioURL:            "https://media.xyzcdn.net/example.m4a",
		AudioMimeType:       "audio/mp4",
		AudioSize:           75891851,
	}
	require.NoError(t, db.Create(&item).Error)

	service := NewCollectionAdoptionService(db)
	return service, db, collection
}

func ptrTime(value time.Time) *time.Time { return &value }

func inboxCount(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var count int64
	require.NoError(t, db.Model(&models.EpisodeTriageDecision{}).
		Where("queue_state = ?", models.QueueStateInbox).Count(&count).Error)
	return count
}

func TestAdopt_FromEightItemCollectionCreatesOneEpisodeAndUnsubscribedPodcast(t *testing.T) {
	service, db, collection := setupAdoptionService(t)
	// 另外七集仍只是清单条目。
	for i := 1; i < 8; i++ {
		require.NoError(t, db.Create(&models.EpisodeCollectionItem{
			CollectionID:      collection.ID,
			Position:          i,
			ExternalEpisodeID: fmt.Sprintf("eid-%02d", i),
			ExternalPodcastID: fmt.Sprintf("pid-%02d", i),
			PodcastTitle:      "其他节目",
			EpisodeTitle:      fmt.Sprintf("单集 %d", i),
			EpisodeURL:        "https://www.xiaoyuzhoufm.com/episode/eid-" + fmt.Sprintf("%02d", i),
		}).Error)
	}

	result, err := service.AdoptCollectionItem(collection.ID, firstAdoptionItemID(t, db, collection.ID))
	require.NoError(t, err)

	assert.True(t, result.EpisodeCreated)
	assert.True(t, result.PodcastCreated)
	assert.True(t, result.InboxWritten)
	require.NotNil(t, result.QueueState)
	assert.Equal(t, models.QueueStateInbox, *result.QueueState)
	assert.True(t, result.CollectionOnly, "清单新建单集必须带 collection_only 标识")
	assert.True(t, result.AudioAvailable)

	// 数据库回读：未关注必须真实持久化；缺 Feed 不得伪造。
	var podcast models.Podcast
	require.NoError(t, db.Where("xyz_id = ?", adoptionSamplePID).First(&podcast).Error)
	assert.False(t, podcast.IsSubscribed, "未关注标记回读必须为 false")
	assert.Empty(t, podcast.FeedURL)
	assert.Equal(t, 200, podcast.ExternalEpisodeCount)
	assert.Equal(t, models.SourcePlatformXiaoyuzhoufm, podcast.DataSource)
	assert.Equal(t, 1, podcast.EpisodeCount)

	var episodeCount, podcastCount int64
	require.NoError(t, db.Model(&models.Episode{}).Count(&episodeCount).Error)
	require.NoError(t, db.Model(&models.Podcast{}).Count(&podcastCount).Error)
	assert.Equal(t, int64(1), episodeCount, "最多新增 1 个个人单集")
	assert.Equal(t, int64(1), podcastCount, "最多新增 1 个归属节目")
	assert.Equal(t, int64(1), inboxCount(t, db))

	var episode models.Episode
	require.NoError(t, db.First(&episode, result.EpisodeID).Error)
	assert.Equal(t, externalGUID(models.SourcePlatformXiaoyuzhoufm, adoptionSampleEID), episode.GUID)
	assert.Equal(t, "https://www.xiaoyuzhoufm.com/episode/"+adoptionSampleEID, episode.Link)
	assert.True(t, episode.CollectionOnly)

	// 外部身份映射与采纳来源摘要都已落库。
	var ref models.EpisodeExternalRef
	require.NoError(t, db.Where("external_episode_id = ?", adoptionSampleEID).First(&ref).Error)
	assert.Equal(t, result.EpisodeID, ref.EpisodeID)
	assert.Equal(t, podcast.ID, ref.PodcastID)
	var adoption models.EpisodeCollectionAdoption
	require.NoError(t, db.Where("episode_id = ?", result.EpisodeID).First(&adoption).Error)
	assert.Equal(t, "穿透半导体迷雾", adoption.CollectionTitle)
}

func firstAdoptionItemID(t *testing.T, db *gorm.DB, collectionID uint) uint {
	t.Helper()
	var item models.EpisodeCollectionItem
	require.NoError(t, db.Where("collection_id = ? AND position = 0", collectionID).First(&item).Error)
	return item.ID
}

func TestAdopt_ReuseIsIdempotentAcrossRepeatAndConcurrentRequests(t *testing.T) {
	service, db, collection := setupAdoptionService(t)
	itemID := firstAdoptionItemID(t, db, collection.ID)

	first, err := service.AdoptCollectionItem(collection.ID, itemID)
	require.NoError(t, err)
	second, err := service.AdoptCollectionItem(collection.ID, itemID)
	require.NoError(t, err)
	assert.Equal(t, first.EpisodeID, second.EpisodeID)
	assert.False(t, second.EpisodeCreated)
	assert.False(t, second.PodcastCreated)
	assert.False(t, second.InboxWritten, "重复点击复用既有结果，不重复入队")

	var episodeCount, inboxEntries int64
	require.NoError(t, db.Model(&models.Episode{}).Count(&episodeCount).Error)
	require.NoError(t, db.Model(&models.EpisodeTriageDecision{}).Count(&inboxEntries).Error)
	assert.Equal(t, int64(1), episodeCount)
	assert.Equal(t, int64(1), inboxEntries)

	// 并发采纳同一条目：唯一约束 + 事务复用，只产生一个本地身份。
	var wg sync.WaitGroup
	results := make([]*AdoptResult, 4)
	errs := make([]error, 4)
	for i := range results {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			results[index], errs[index] = service.AdoptCollectionItem(collection.ID, itemID)
		}(i)
	}
	wg.Wait()
	for i := range results {
		if errs[i] != nil {
			t.Logf("并发分支返回错误（忙等可接受）: %v", errs[i])
			continue
		}
		assert.Equal(t, first.EpisodeID, results[i].EpisodeID)
	}
	require.NoError(t, db.Model(&models.Episode{}).Count(&episodeCount).Error)
	assert.Equal(t, int64(1), episodeCount, "并发请求不得重复建集")
}

func TestAdopt_ReusesExistingLibraryEpisodeAndKeepsQueueState(t *testing.T) {
	service, db, collection := setupAdoptionService(t)
	// 已订阅节目里已有同一集（普通同步识别过的单集，非 collection_only）。
	podcast := models.Podcast{Title: "投资实战派", XYZID: adoptionSamplePID, FeedURL: "https://example.com/feed.xml", IsSubscribed: true}
	require.NoError(t, db.Create(&podcast).Error)
	existing := models.Episode{
		PodcastID:     podcast.ID,
		Title:         "已有标题以同步为准",
		GUID:          "rss-guid-original",
		Link:          "https://www.xiaoyuzhoufm.com/episode/" + adoptionSampleEID,
		PublishedDate: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
	}
	require.NoError(t, db.Create(&existing).Error)
	// 已在 Focus：收录不得覆盖。
	focusState := models.QueueStateFocus
	require.NoError(t, db.Create(&models.EpisodeTriageDecision{
		EpisodeID:      existing.ID,
		State:          models.TriageStateShortlisted,
		DecidedAt:      time.Now().UTC(),
		QueueState:     &focusState,
		QueueUpdatedAt: ptrTime(time.Now().UTC()),
	}).Error)

	result, err := service.AdoptCollectionItem(collection.ID, firstAdoptionItemID(t, db, collection.ID))
	require.NoError(t, err)
	assert.Equal(t, existing.ID, result.EpisodeID, "已有单集必须复用")
	assert.False(t, result.EpisodeCreated)
	assert.False(t, result.PodcastCreated)
	assert.False(t, result.PodcastSubscribed == false && false, "")
	assert.True(t, result.PodcastSubscribed, "已有节目关注状态不变")
	require.NotNil(t, result.QueueState)
	assert.Equal(t, models.QueueStateFocus, *result.QueueState, "已有 Focus 状态不被采纳覆盖")
	assert.False(t, result.InboxWritten)
	assert.False(t, result.CollectionOnly, "已识别单集不改资格标识")

	var episodeCount int64
	require.NoError(t, db.Model(&models.Episode{}).Count(&episodeCount).Error)
	assert.Equal(t, int64(1), episodeCount)

	var unchanged models.Episode
	require.NoError(t, db.First(&unchanged, existing.ID).Error)
	assert.Equal(t, "rss-guid-original", unchanged.GUID, "复用不改写 GUID 与同步时间")
	assert.True(t, unchanged.FetchedAt == nil)
}

func TestAdopt_SoftDeletedEpisodeIsNotRevived(t *testing.T) {
	service, db, collection := setupAdoptionService(t)
	podcast := models.Podcast{Title: "已删除节目", XYZID: adoptionSamplePID, FeedURL: "https://example.com/feed.xml", IsSubscribed: true}
	require.NoError(t, db.Create(&podcast).Error)
	deleted := models.Episode{
		PodcastID: podcast.ID,
		Title:     "已被删除的单集",
		GUID:      "deleted-guid",
		Link:      "https://www.xiaoyuzhoufm.com/episode/" + adoptionSampleEID,
	}
	require.NoError(t, db.Create(&deleted).Error)
	require.NoError(t, db.Delete(&deleted).Error)

	_, err := service.AdoptCollectionItem(collection.ID, firstAdoptionItemID(t, db, collection.ID))
	require.ErrorIs(t, err, ErrAdoptionEpisodeDeleted)

	var count int64
	require.NoError(t, db.Unscoped().Model(&models.Episode{}).Where("deleted_at IS NOT NULL").Count(&count).Error)
	assert.Equal(t, int64(1), count, "软删除单集保持删除状态，不自动复活")
	assert.Equal(t, int64(0), inboxCount(t, db))
}

func TestAdopt_CrossPodcastAndAmbiguousIdentitiesAreRejected(t *testing.T) {
	t.Run("跨节目冲突", func(t *testing.T) {
		service, db, collection := setupAdoptionService(t)
		other := models.Podcast{Title: "另一档节目", XYZID: "different-pid", FeedURL: "https://example.com/other.xml", IsSubscribed: true}
		require.NoError(t, db.Create(&other).Error)
		require.NoError(t, db.Create(&models.Episode{
			PodcastID: other.ID,
			Title:     "同名同链接但属于别的节目",
			GUID:      "other-guid",
			Link:      "https://www.xiaoyuzhoufm.com/episode/" + adoptionSampleEID,
		}).Error)

		_, err := service.AdoptCollectionItem(collection.ID, firstAdoptionItemID(t, db, collection.ID))
		require.ErrorIs(t, err, ErrAdoptionCrossPodcast)
		var episodeCount int64
		require.NoError(t, db.Model(&models.Episode{}).Count(&episodeCount).Error)
		assert.Equal(t, int64(1), episodeCount, "冲突时不新建单集")
	})

	t.Run("多个候选", func(t *testing.T) {
		service, db, collection := setupAdoptionService(t)
		podcast := models.Podcast{Title: "投资实战派", XYZID: adoptionSamplePID, FeedURL: "https://example.com/feed.xml", IsSubscribed: true}
		require.NoError(t, db.Create(&podcast).Error)
		for _, guid := range []string{"guid-a", "guid-b"} {
			require.NoError(t, db.Create(&models.Episode{
				PodcastID: podcast.ID,
				Title:     "同链接候选",
				GUID:      guid,
				Link:      "https://www.xiaoyuzhoufm.com/episode/" + adoptionSampleEID,
			}).Error)
		}

		_, err := service.AdoptCollectionItem(collection.ID, firstAdoptionItemID(t, db, collection.ID))
		require.ErrorIs(t, err, ErrAdoptionAmbiguous)
	})
}

func TestAdopt_TransactionFailureLeavesNoPartialAdoption(t *testing.T) {
	service, db, collection := setupAdoptionService(t)
	itemID := firstAdoptionItemID(t, db, collection.ID)

	// 用触发器在写入采纳摘要时中止事务，模拟落库中途失败。
	require.NoError(t, db.Exec(`
		CREATE TRIGGER abort_adoption BEFORE INSERT ON episode_collection_adoptions
		BEGIN
			SELECT RAISE(ABORT, 'simulated adoption failure');
		END
	`).Error)

	_, err := service.AdoptCollectionItem(collection.ID, itemID)
	require.Error(t, err)

	var episodeCount, podcastCount, refCount, adoptionCount, inboxEntries int64
	require.NoError(t, db.Model(&models.Episode{}).Count(&episodeCount).Error)
	require.NoError(t, db.Model(&models.Podcast{}).Count(&podcastCount).Error)
	require.NoError(t, db.Model(&models.EpisodeExternalRef{}).Count(&refCount).Error)
	require.NoError(t, db.Model(&models.EpisodeCollectionAdoption{}).Count(&adoptionCount).Error)
	require.NoError(t, db.Model(&models.EpisodeTriageDecision{}).Count(&inboxEntries).Error)
	assert.Zero(t, episodeCount, "失败不得留下半完成单集")
	assert.Zero(t, podcastCount, "失败不得留下半完成节目")
	assert.Zero(t, refCount)
	assert.Zero(t, adoptionCount)
	assert.Zero(t, inboxEntries)

	var item models.EpisodeCollectionItem
	require.NoError(t, db.First(&item, itemID).Error)
	assert.Nil(t, item.EpisodeID, "失败后条目不得关联单集")

	// 清除故障后重试返回真实结果。
	require.NoError(t, db.Exec("DROP TRIGGER abort_adoption").Error)
	result, err := service.AdoptCollectionItem(collection.ID, itemID)
	require.NoError(t, err)
	assert.True(t, result.EpisodeCreated)
	assert.True(t, result.InboxWritten)
}

func TestAdopt_MultipleFeedlessPodcastsPersistWithoutFabricatedFeeds(t *testing.T) {
	service, db, collection := setupAdoptionService(t)
	for i := 1; i <= 2; i++ {
		item := models.EpisodeCollectionItem{
			CollectionID:      collection.ID,
			Position:          i,
			ExternalEpisodeID: fmt.Sprintf("feedless-eid-%d", i),
			ExternalPodcastID: fmt.Sprintf("feedless-pid-%d", i),
			PodcastTitle:      fmt.Sprintf("无 Feed 节目 %d", i),
			EpisodeTitle:      fmt.Sprintf("无 Feed 单集 %d", i),
			EpisodeURL:        fmt.Sprintf("https://www.xiaoyuzhoufm.com/episode/feedless-eid-%d", i),
		}
		require.NoError(t, db.Create(&item).Error)
		result, err := service.AdoptCollectionItem(collection.ID, item.ID)
		require.NoError(t, err)
		var podcast models.Podcast
		require.NoError(t, db.First(&podcast, result.PodcastID).Error)
		assert.Empty(t, podcast.FeedURL, "不得伪造 Feed 地址")
		assert.False(t, podcast.IsSubscribed)
	}

	var feedlessCount int64
	require.NoError(t, db.Model(&models.Podcast{}).Where("feed_url IS NULL").Count(&feedlessCount).Error)
	assert.Equal(t, int64(2), feedlessCount, "多个无 Feed 节目可保存")

	// 非空 Feed 仍唯一。
	dup := models.Podcast{Title: "重复 Feed", XYZID: "dup-pid", FeedURL: "https://example.com/feed.xml"}
	require.NoError(t, db.Create(&dup).Error)
	conflict := models.Podcast{Title: "重复 Feed 2", XYZID: "dup-pid-2", FeedURL: "https://example.com/feed.xml"}
	require.Error(t, db.Create(&conflict).Error, "非空 Feed 唯一约束保持")
}
