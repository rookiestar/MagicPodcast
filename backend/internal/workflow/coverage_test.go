package workflow

import (
	"strconv"
	"testing"

	"magicpodcast/internal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newCoverageDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&models.Podcast{}, &models.Workflow{}))
	return db
}

func seedCoveragePodcast(t *testing.T, db *gorm.DB, id int, feedURL string, subscribed bool) models.Podcast {
	t.Helper()
	podcast := models.Podcast{XYZID: "coverage-" + strconv.Itoa(id), Title: "节目" + strconv.Itoa(id), FeedURL: feedURL}
	podcast.ID = uint(id)
	require.NoError(t, db.Create(&podcast).Error)
	require.NoError(t, db.Model(&models.Podcast{}).Where("id = ?", id).
		Update("is_subscribed", subscribed).Error)
	podcast.IsSubscribed = subscribed
	return podcast
}

func refIDs(refs []WorkflowRef) []uint {
	ids := make([]uint, 0, len(refs))
	for _, ref := range refs {
		ids = append(ids, ref.ID)
	}
	return ids
}

// TestCoverageForPodcastsMatchesScopeRules 验证共享覆盖口径（#417 AC7）：
// 指定成员按 ID；全部订阅按订阅资格；自定义源按 Feed 地址明确对应（含
// http/https 互换）；停用工作流计入、已删除不计入；标题不参与匹配。
func TestCoverageForPodcastsMatchesScopeRules(t *testing.T) {
	db := newCoverageDB(t)
	pMember := seedCoveragePodcast(t, db, 1, "http://f/member.xml", true)
	pUnsubscribed := seedCoveragePodcast(t, db, 2, "http://f/unsub.xml", false)
	pCustom := seedCoveragePodcast(t, db, 3, "https://f/custom.xml", false)
	pTitleOnly := seedCoveragePodcast(t, db, 4, "http://f/other.xml", true)

	specific := models.Workflow{Name: "指定节目", Schedule: "0 6 * * *", ScopeType: models.ScopeTypeSpecificPodcasts,
		ScopeConfig: models.ScopeConfig{PodcastIDs: []int{int(pMember.ID)}}}
	require.NoError(t, db.Create(&specific).Error)
	disabledAll := models.Workflow{Name: "停用的全订阅", Schedule: "0 7 * * *", ScopeType: models.ScopeTypeAllSubscribed, IsEnabled: false}
	require.NoError(t, db.Create(&disabledAll).Error)
	custom := models.Workflow{Name: "同名节目", Schedule: "0 8 * * *", ScopeType: models.ScopeTypeCustomSources,
		ScopeConfig: models.ScopeConfig{CustomURLs: []string{"http://f/custom.xml"}}}
	require.NoError(t, db.Create(&custom).Error)
	deleted := models.Workflow{Name: "已删除", Schedule: "0 9 * * *", ScopeType: models.ScopeTypeSpecificPodcasts,
		ScopeConfig: models.ScopeConfig{PodcastIDs: []int{int(pMember.ID)}}}
	require.NoError(t, db.Create(&deleted).Error)
	require.NoError(t, db.Delete(&models.Workflow{}, deleted.ID).Error)

	coverage, err := CoverageForPodcasts(db, []uint{pMember.ID, pUnsubscribed.ID, pCustom.ID, pTitleOnly.ID})
	require.NoError(t, err)

	assert.ElementsMatch(t, []uint{specific.ID, disabledAll.ID}, refIDs(coverage[pMember.ID]),
		"指定成员命中；停用全订阅计入；已删除工作流不计入")
	assert.Empty(t, coverage[pUnsubscribed.ID], "未订阅节目不被全部订阅范围覆盖")
	assert.Equal(t, []uint{custom.ID}, refIDs(coverage[pCustom.ID]),
		"https 记录被 http 自定义源明确对应（scheme 互换）")
	assert.Equal(t, []uint{disabledAll.ID}, refIDs(coverage[pTitleOnly.ID]),
		"仅订阅资格与地址参与覆盖；同名不隐藏其他节目")

	covered, err := CoveredPodcastIDs(db)
	require.NoError(t, err)
	assert.Equal(t, map[uint]struct{}{pMember.ID: {}, pCustom.ID: {}, pTitleOnly.ID: {}}, covered)
}

// TestCoverageForPodcastsEmptyInput 返回空映射而非错误。
func TestCoverageForPodcastsEmptyInput(t *testing.T) {
	db := newCoverageDB(t)
	coverage, err := CoverageForPodcasts(db, nil)
	require.NoError(t, err)
	assert.Empty(t, coverage)
}
