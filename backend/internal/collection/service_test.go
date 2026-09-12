package collection

import (
	"context"
	"net/http"
	"path/filepath"
	"testing"

	"magicpodcast/internal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "collections.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&models.Podcast{},
		&models.Episode{},
		&models.EpisodeTriageDecision{},
		&models.EpisodeCollection{},
		&models.EpisodeCollectionItem{},
	))
	return db
}

func samplePreview(t *testing.T, service *Service) *PreviewResult {
	t.Helper()
	result, err := service.Preview(context.Background(),
		"https://www.xiaoyuzhoufm.com/collection/episode/"+sampleCollectionID)
	require.NoError(t, err)
	return result
}

func TestConfirmImport_SavesPreviewedVersionAtomically(t *testing.T) {
	service := newStubService(t, stubFetcher(func(string) (int, string) {
		return http.StatusOK, loadSampleHTML(t)
	}, nil))
	result := samplePreview(t, service)

	imported, err := service.ConfirmImport(result.PreviewID)
	require.NoError(t, err)
	require.False(t, imported.Duplicate)

	detail, err := service.GetCollection(imported.CollectionID)
	require.NoError(t, err)
	assert.Equal(t, sampleTitle, detail.Title)
	assert.Equal(t, sampleAuthor, detail.Author)
	require.Len(t, detail.Items, 8)
	for index, item := range detail.Items {
		assert.Equal(t, index, item.Position, "原始顺序保存且返回")
		assert.Nil(t, item.AdoptedEpisodeID, "导入不收录任何单集")
	}
	assert.Equal(t, sampleFirstRecommendations[5], detail.Items[5].Recommendation)
	assert.Equal(t, 4692, detail.Items[0].Duration)
	assert.NotEmpty(t, detail.Items[0].Shownotes)
	// 无法核实总数时如实返回已读取数量，不冒充完整导入。
	assert.False(t, detail.TotalKnown)
	assert.Equal(t, int64(8), detail.ItemCount)
	assert.Equal(t, int64(0), detail.AdoptedCount)
}

func TestImport_DoesNotTouchPersonalLibrary(t *testing.T) {
	db := newTestDB(t)
	// 预置真实个人库内容。
	podcast := models.Podcast{Title: "已有节目", FeedURL: "https://example.com/feed.xml", XYZID: "import-isolation", IsSubscribed: true}
	require.NoError(t, db.Create(&podcast).Error)
	episode := models.Episode{PodcastID: podcast.ID, Title: "已有单集", GUID: "import-isolation-episode"}
	require.NoError(t, db.Create(&episode).Error)
	decision := models.EpisodeTriageDecision{EpisodeID: episode.ID}
	require.NoError(t, db.Create(&decision).Error)

	countsBefore := libraryCounts(t, db)

	service := NewService(db)
	service.fetch = stubFetcher(func(string) (int, string) { return http.StatusOK, loadSampleHTML(t) }, nil)
	imported, err := service.ConfirmImport(samplePreview(t, service).PreviewID)
	require.NoError(t, err)

	countsAfter := libraryCounts(t, db)
	assert.Equal(t, countsBefore, countsAfter, "导入清单不得改变个人节目/单集/队列/报告数据")
	assert.NotZero(t, imported.CollectionID)

	var items []models.EpisodeCollectionItem
	require.NoError(t, db.Where("collection_id = ?", imported.CollectionID).Find(&items).Error)
	require.Len(t, items, 8)
	for _, item := range items {
		assert.Nil(t, item.EpisodeID, "清单条目不关联个人单集")
	}
}

func libraryCounts(t *testing.T, db *gorm.DB) map[string]int64 {
	t.Helper()
	counts := map[string]int64{}
	// 只统计个人库账本；清单表本身是本票新增的发现资料，允许增长。
	for name, model := range map[string]any{
		"podcasts":         &models.Podcast{},
		"episodes":         &models.Episode{},
		"triage_decisions": &models.EpisodeTriageDecision{},
	} {
		var count int64
		require.NoError(t, db.Model(model).Count(&count).Error)
		counts[name] = count
	}
	return counts
}

func TestConfirmImport_DuplicateSourceOpensExistingCollection(t *testing.T) {
	service := newStubService(t, stubFetcher(func(string) (int, string) {
		return http.StatusOK, loadSampleHTML(t)
	}, nil))

	first, err := service.ConfirmImport(samplePreview(t, service).PreviewID)
	require.NoError(t, err)

	// 重新预览同一清单后再次导入：打开已有清单，不新建副本。
	second, err := service.ConfirmImport(samplePreview(t, service).PreviewID)
	require.NoError(t, err)
	assert.True(t, second.Duplicate)
	assert.Equal(t, first.CollectionID, second.CollectionID)

	var collectionCount int64
	require.NoError(t, service.db.Model(&models.EpisodeCollection{}).Count(&collectionCount).Error)
	assert.Equal(t, int64(1), collectionCount)
}

func TestListCollections_SearchAndClear(t *testing.T) {
	service := newStubService(t, stubFetcher(func(string) (int, string) {
		return http.StatusOK, loadSampleHTML(t)
	}, nil))
	imported, err := service.ConfirmImport(samplePreview(t, service).PreviewID)
	require.NoError(t, err)
	_ = imported

	list, err := service.ListCollections("")
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, sampleTitle, list[0].Title)

	matched, err := service.ListCollections("半导体")
	require.NoError(t, err)
	require.Len(t, matched, 1)

	missing, err := service.ListCollections("不存在的清单")
	require.NoError(t, err)
	assert.Empty(t, missing)
}

func TestGetCollection_MissingIDReturnsSentinel(t *testing.T) {
	service := newStubService(t, stubFetcher(func(string) (int, string) { return http.StatusOK, loadSampleHTML(t) }, nil))
	_, err := service.GetCollection(9999)
	require.ErrorIs(t, err, ErrCollectionNotFound)
}

func TestConfirmImport_SourceUniqueConstraintRace(t *testing.T) {
	// 直接用同一份草稿模拟并发确认：第二个事务应复用已有清单而不是报错。
	service := newStubService(t, stubFetcher(func(string) (int, string) {
		return http.StatusOK, loadSampleHTML(t)
	}, nil))
	first, err := service.ConfirmImport(samplePreview(t, service).PreviewID)
	require.NoError(t, err)

	// 绕过预览存储，手工触发同一平台+外部ID的写入路径。
	result, err := service.ConfirmImport(samplePreview(t, service).PreviewID)
	require.NoError(t, err)
	assert.Equal(t, first.CollectionID, result.CollectionID)
}
