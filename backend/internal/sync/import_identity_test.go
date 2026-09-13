package sync

import (
	"os"
	"path/filepath"
	"testing"

	"magicpodcast/internal/models"
	"magicpodcast/internal/opml"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// identityFeedXML 生成携带 itunes:id 的 RSS，用于稳定身份匹配。
func identityFeedXML(itunesID string) string {
	itunesTag := ""
	if itunesID != "" {
		itunesTag = `<itunes:id>` + itunesID + `</itunes:id>`
	}
	return `<?xml version="1.0" encoding="UTF-8"?>
<rss xmlns:itunes="http://www.itunes.com/dtds/podcast-1.0.dtd" version="2.0"><channel>
<title>Identity Show</title><link>https://example.com</link>` + itunesTag + `
<item><title>Episode</title><guid>id-ep-1</guid><pubDate>Mon, 01 Jan 2024 00:00:00 GMT</pubDate></item>
</channel></rss>`
}

func writeIdentityOPML(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "identity.opml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func TestXiaoyuzhouPodcastPathSegment(t *testing.T) {
	assert.Equal(t, "629bf20356b3d7e17a71cfa1",
		xiaoyuzhouPodcastPathSegment("https://www.xiaoyuzhoufm.com/podcast/629bf20356b3d7e17a71cfa1"))
	assert.Equal(t, "629bf20356b3d7e17a71cfa1",
		xiaoyuzhouPodcastPathSegment("https://www.xiaoyuzhoufm.com/podcast/629bf20356b3d7e17a71cfa1.rss"))
	assert.Equal(t, "",
		xiaoyuzhouPodcastPathSegment("https://example.com/podcast/629bf20356b3d7e17a71cfa1"))
	assert.Equal(t, "",
		xiaoyuzhouPodcastPathSegment("https://www.xiaoyuzhoufm.com/episode/629bf20356b3d7e17a71cfa1"))
	assert.Equal(t, "",
		xiaoyuzhouPodcastPathSegment("https://www.xiaoyuzhoufm.com/podcast/not-hex-id"))
}

func TestPreviewImportOPMLClassifiesEntries(t *testing.T) {
	server := newCursorFeedServer(t, identityFeedXML(""))
	db := setupTestDB(t)
	service, err := NewService(db, "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close()) })

	// 已有节目（精确 URL）、清单收录（xyz_id 无 RSS）、新节目、无效地址。
	existing := models.Podcast{XYZID: "ex1", Title: "已有节目", FeedURL: server.URL + "/feed.xml", IsSubscribed: true}
	require.NoError(t, db.Create(&existing).Error)
	collection := models.Podcast{XYZID: "629bf20356b3d7e17a71cfa1", Title: "清单节目", IsSubscribed: false}
	require.NoError(t, db.Create(&collection).Error)

	removed := models.Podcast{XYZID: "gone", Title: "已删除", FeedURL: server.URL + "/removed.xml"}
	require.NoError(t, db.Create(&removed).Error)
	require.NoError(t, db.Delete(&removed).Error)

	path := writeIdentityOPML(t, `<opml version="2.0"><body>
<outline title="已有节目" type="rss" xmlUrl="`+server.URL+`/feed.xml"/>
<outline title="已有节目重复" type="rss" xmlUrl="`+server.URL+`/feed.xml"/>
<outline title="清单节目" type="rss" xmlUrl="https://www.xiaoyuzhoufm.com/podcast/629bf20356b3d7e17a71cfa1"/>
<outline title="已删除" type="rss" xmlUrl="`+server.URL+`/removed.xml"/>
<outline title="全新节目" type="rss" xmlUrl="https://example.com/brand-new.xml"/>
<outline title="无效" type="rss" xmlUrl="not-a-url"/>
</body></opml>`)

	outlines, err := service.opmlParser.ParseFile(path)
	require.NoError(t, err)
	preview, err := service.PreviewImportOPML(outlines)
	require.NoError(t, err)
	require.Equal(t, 6, preview.Total)
	require.Len(t, preview.Entries, 5)

	byURL := make(map[string]ImportPreviewEntry)
	for _, entry := range preview.Entries {
		byURL[entry.XMLURL] = entry
	}
	assert.Equal(t, ImportEntryExisting, byURL[server.URL+"/feed.xml"].Kind)
	assert.Equal(t, 1, byURL[server.URL+"/feed.xml"].DuplicateInFile)
	assert.Equal(t, ImportEntryCollection, byURL["https://www.xiaoyuzhoufm.com/podcast/629bf20356b3d7e17a71cfa1"].Kind)
	assert.Equal(t, "xyz_id", byURL["https://www.xiaoyuzhoufm.com/podcast/629bf20356b3d7e17a71cfa1"].Evidence)
	assert.Equal(t, ImportEntryDeleted, byURL[server.URL+"/removed.xml"].Kind)
	assert.Equal(t, ImportEntryNew, byURL["https://example.com/brand-new.xml"].Kind)
	assert.Equal(t, ImportEntryInvalid, byURL["not-a-url"].Kind)

	assert.Equal(t, 1, preview.ExistingCount)
	assert.Equal(t, 1, preview.CollectionCount)
	assert.Equal(t, 1, preview.DeletedCount)
	assert.Equal(t, 1, preview.NewCount)
	assert.Equal(t, 1, preview.InvalidCount)
	assert.Equal(t, 1, preview.DuplicateMergedCount)
}

// TestIdentityMovedFeedRequiresConfirmation 验证换地址的节目在证据充分时
// 也必须先看到证据，确认后才复用本地记录（R3 / #401 AC1）。
func TestIdentityMovedFeedRequiresConfirmation(t *testing.T) {
	oldServer := newCursorFeedServer(t, identityFeedXML("123"))
	newServer := newCursorFeedServer(t, identityFeedXML("123"))
	db := setupTestDB(t)
	service, err := NewService(db, "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close()) })
	applyNoRetryPolicy(t, service)

	existing := models.Podcast{
		XYZID: "moved", Title: "旧地址节目", FeedURL: oldServer.URL + "/feed.xml",
		ITunesID: "123", IsSubscribed: true, Notes: "保留我的备注", EpisodeCount: 5,
	}
	require.NoError(t, db.Create(&existing).Error)
	require.NoError(t, db.AutoMigrate(&models.Tag{}))
	tag := models.Tag{Name: "我的标签"}
	require.NoError(t, db.Create(&tag).Error)
	require.NoError(t, db.Model(&existing).Association("Tags").Append(&tag))

	url := newServer.URL + "/feed.xml"
	path := writeIdentityOPML(t, `<opml version="2.0"><body><outline title="Moved Show" type="rss" xmlUrl="`+url+`"/></body></opml>`)

	// 第一次导入：无确认决策 → 跳过并展示证据，不合并、不新建。
	result, err := service.ImportOPML(path)
	require.NoError(t, err)
	require.Equal(t, 1, result.ConflictPodcasts)
	require.Zero(t, result.SuccessPodcasts)

	var count int64
	require.NoError(t, db.Model(&models.Podcast{}).Count(&count).Error)
	require.Equal(t, int64(1), count, "未确认前不得创建新记录")

	var untouched models.Podcast
	require.NoError(t, db.First(&untouched, existing.ID).Error)
	require.Equal(t, oldServer.URL+"/feed.xml", untouched.FeedURL)
	require.Equal(t, "保留我的备注", untouched.Notes)

	// 第二次导入：携带确认决策 → 复用本地记录、保留个人资料与标签、更新地址。
	result, err = service.ImportOPMLWithDecisions(path, map[string]string{url: ImportDecisionConfirm})
	require.NoError(t, err)
	require.Equal(t, 1, result.MergedPodcasts)

	var merged models.Podcast
	require.NoError(t, db.Preload("Tags").First(&merged, existing.ID).Error)
	require.Equal(t, url, merged.FeedURL, "确认后主 RSS 更新为本次地址")
	require.Equal(t, int64(existing.ID), int64(merged.ID))
	require.Equal(t, "保留我的备注", merged.Notes)
	require.Len(t, merged.Tags, 1)

	// 重复确认不创建新记录。
	result, err = service.ImportOPMLWithDecisions(path, map[string]string{url: ImportDecisionConfirm})
	require.NoError(t, err)
	require.NoError(t, db.Model(&models.Podcast{}).Count(&count).Error)
	require.Equal(t, int64(1), count)
	_ = result
}

// TestIdentityMultiCandidateNeverMerges 验证多候选冲突即使确认也不合并。
func TestIdentityMultiCandidateNeverMerges(t *testing.T) {
	newServer := newCursorFeedServer(t, identityFeedXML("456"))
	db := setupTestDB(t)
	service, err := NewService(db, "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close()) })
	applyNoRetryPolicy(t, service)

	first := models.Podcast{XYZID: "dup1", Title: "候选一", FeedURL: "https://a.example/f.xml", ITunesID: "456", IsSubscribed: true}
	second := models.Podcast{XYZID: "dup2", Title: "候选二", FeedURL: "https://b.example/f.xml", ITunesID: "456", IsSubscribed: true}
	require.NoError(t, db.Create(&first).Error)
	require.NoError(t, db.Create(&second).Error)

	url := newServer.URL + "/feed.xml"
	path := writeIdentityOPML(t, `<opml version="2.0"><body><outline title="Identity Show" type="rss" xmlUrl="`+url+`"/></body></opml>`)
	result, err := service.ImportOPMLWithDecisions(path, map[string]string{url: ImportDecisionConfirm})
	require.NoError(t, err)
	require.Equal(t, 1, result.ConflictPodcasts)
	require.Zero(t, result.SuccessPodcasts)

	var count int64
	require.NoError(t, db.Model(&models.Podcast{}).Count(&count).Error)
	require.Equal(t, int64(2), count)
	var unchanged models.Podcast
	require.NoError(t, db.First(&unchanged, first.ID).Error)
	require.Equal(t, "https://a.example/f.xml", unchanged.FeedURL)
}

// TestCollectionPodcastBindsAfterConfirmation 验证清单收录节目在确认后
// 转为关注并绑定订阅地址，保留原 ID 与已收录单集（#401 AC2）。
func TestCollectionPodcastBindsAfterConfirmation(t *testing.T) {
	db := setupTestDB(t)
	service, err := NewService(db, "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close()) })
	applyNoRetryPolicy(t, service)

	// 清单收录创建的无 RSS 记录 + 一条已收录单集。is_subscribed 必须用
	// map 写入绕过 GORM default:true 的零值覆盖（与收录服务同一做法）。
	collectionPodcast := models.Podcast{XYZID: "629bf20356b3d7e17a71cfa1", Title: "清单节目"}
	require.NoError(t, db.Create(&collectionPodcast).Error)
	require.NoError(t, db.Model(&models.Podcast{}).Where("id = ?", collectionPodcast.ID).
		Updates(map[string]interface{}{"is_subscribed": false}).Error)
	collected := models.Episode{PodcastID: collectionPodcast.ID, Title: "已收录单集", GUID: "collected-1", CollectionOnly: true}
	require.NoError(t, db.Create(&collected).Error)

	url := "https://www.xiaoyuzhoufm.com/podcast/629bf20356b3d7e17a71cfa1"
	path := writeIdentityOPML(t, `<opml version="2.0"><body><outline title="清单节目" text="简介" type="rss" xmlUrl="`+url+`"/></body></opml>`)

	// 未确认：跳过，清单记录保持未关注、无 RSS。
	result, err := service.ImportOPML(path)
	require.NoError(t, err)
	require.Equal(t, 1, result.ConflictPodcasts)
	var untouched models.Podcast
	require.NoError(t, db.First(&untouched, collectionPodcast.ID).Error)
	require.False(t, untouched.IsSubscribed)
	require.Empty(t, untouched.FeedURL)

	// 确认后的绑定写入路径（单测级，不发起对小宇宙域名的真实请求）：
	// 抓取失败 → 待同步空壳 + 绑定地址 + 转关注，复用原记录 ID。
	outline := outlineFor(url)
	podcast := pendingImportPodcastFrom(&outline, url, &collectionPodcast)
	changed, err := service.saveImportPodcast(podcast, service.resolveImportIdentity(url))
	require.NoError(t, err)
	require.True(t, changed)

	var bound models.Podcast
	require.NoError(t, db.First(&bound, collectionPodcast.ID).Error)
	require.True(t, bound.IsSubscribed)
	require.Equal(t, url, bound.FeedURL)
	require.False(t, bound.FeedURLValid, "绑定后仍待同步")

	var episode models.Episode
	require.NoError(t, db.First(&episode, collected.ID).Error)
	require.Equal(t, collectionPodcast.ID, episode.PodcastID)
	require.True(t, episode.CollectionOnly, "仅导入 OPML 不得自动清除 collection_only 资格")
}

// outlineFor 构造携带指定地址的订阅条目。
func outlineFor(xmlURL string) opml.Outline {
	return opml.Outline{Title: "清单节目", Text: "简介", XMLURL: xmlURL, Type: "rss"}
}

// TestSoftDeletedRecordNotSilentlyRevived 验证软删除记录不静默复活，
// 仅确认后恢复（#401 AC4）。
func TestSoftDeletedRecordNotSilentlyRevived(t *testing.T) {
	server := newCursorFeedServer(t, identityFeedXML(""))
	db := setupTestDB(t)
	service, err := NewService(db, "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close()) })
	applyNoRetryPolicy(t, service)

	removed := models.Podcast{XYZID: "revive", Title: "已删除节目", FeedURL: server.URL + "/feed.xml", IsSubscribed: true}
	require.NoError(t, db.Create(&removed).Error)
	require.NoError(t, db.Delete(&removed).Error)

	path := writeTestOPML(t, server.URL+"/feed.xml")

	result, err := service.ImportOPML(path)
	require.NoError(t, err)
	require.Equal(t, 1, result.ConflictPodcasts)
	require.Zero(t, result.SuccessPodcasts)

	var count int64
	require.NoError(t, db.Unscoped().Model(&models.Podcast{}).Where("deleted_at IS NULL").Count(&count).Error)
	require.Zero(t, count, "未确认时不得复活")

	result, err = service.ImportOPMLWithDecisions(path, map[string]string{server.URL + "/feed.xml": ImportDecisionConfirm})
	require.NoError(t, err)
	require.Equal(t, 1, result.MergedPodcasts)

	var revived models.Podcast
	require.NoError(t, db.Unscoped().First(&revived, removed.ID).Error)
	require.False(t, revived.DeletedAt.Valid)
	require.True(t, revived.IsSubscribed)
}
