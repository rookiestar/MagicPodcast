package sync

import (
	"magicpodcast/internal/opml"
	"testing"

	"magicpodcast/internal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestImportRecordsCreationFactsPerEntry 通过真实导入链路验证逐条新建事实
// （#417/#418 AC1）：只有实际新建的记录（新建成功、新建待同步）带 created
// 标记；已有更新、确认合并与失败未落库不算新建。
func TestImportRecordsCreationFactsPerEntry(t *testing.T) {
	freshServer := newCursorFeedServer(t, identityFeedXML(""))
	db := setupTestDB(t)
	service, err := NewService(db, "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close()) })
	applyNoRetryPolicy(t, service)

	// 已有节目（本次资料有变化）：预置旧标题，抓取后应记为 updated。
	existingUpdated := models.Podcast{XYZID: "updated-show", Title: "旧标题", FeedURL: freshServer.URL + "/updated.xml", IsSubscribed: true}
	require.NoError(t, db.Create(&existingUpdated).Error)
	// 已有节目但 RSS 不可达：待同步空壳刷新失败，不得算新建。
	existingPending := models.Podcast{XYZID: "pending-show", Title: "已有待同步", FeedURL: "http://127.0.0.1:9/existing-pending.xml", IsSubscribed: true}
	require.NoError(t, db.Create(&existingPending).Error)
	// 换址合并目标：itunes id 与新地址的 Feed 一致，确认后复用本地记录。
	moved := models.Podcast{XYZID: "moved-show", Title: "换址节目", FeedURL: "http://127.0.0.1:9/old.xml", ITunesID: "123", IsSubscribed: true, Notes: "保留备注"}
	require.NoError(t, db.Create(&moved).Error)

	mergedServer := newCursorFeedServer(t, identityFeedXML("123"))
	mergedURL := mergedServer.URL + "/feed.xml"
	opmlContent := `<opml version="2.0"><body>`
	opmlContent += `<outline title="Fresh Show" type="rss" xmlUrl="` + freshServer.URL + `/feed.xml"/>`
	opmlContent += `<outline title="Updated Show" type="rss" xmlUrl="` + freshServer.URL + `/updated.xml"/>`
	opmlContent += `<outline title="New Pending" type="rss" xmlUrl="http://127.0.0.1:9/new-pending.xml"/>`
	opmlContent += `<outline title="Existing Pending" type="rss" xmlUrl="http://127.0.0.1:9/existing-pending.xml"/>`
	opmlContent += `<outline title="Moved Show" type="rss" xmlUrl="` + mergedURL + `"/>`
	opmlContent += `<outline title="Bad URL" type="rss" xmlUrl="ftp://example.invalid/rss"/>`
	opmlContent += `</body></opml>`
	path := writeIdentityOPML(t, opmlContent)

	result, err := service.ImportOPMLWithDecisions(path, map[string]string{mergedURL: ImportDecisionConfirm})
	require.NoError(t, err)

	byURL := map[string]ImportEntryResult{}
	for _, entry := range result.Entries {
		byURL[entry.FeedURL] = entry
	}

	newEntry := byURL[freshServer.URL+"/feed.xml"]
	assert.Equal(t, ImportOutcomeNew, newEntry.Outcome)
	assert.True(t, newEntry.Created, "新建成功必须记录创建事实")
	assert.NotZero(t, newEntry.PodcastID)

	updatedEntry := byURL[freshServer.URL+"/updated.xml"]
	assert.Equal(t, ImportOutcomeUpdated, updatedEntry.Outcome)
	assert.False(t, updatedEntry.Created, "已有节目资料更新不算新建")

	newPending := byURL["http://127.0.0.1:9/new-pending.xml"]
	assert.Equal(t, ImportOutcomePending, newPending.Outcome)
	assert.True(t, newPending.Created, "新建待同步空壳必须记录创建事实")
	assert.NotZero(t, newPending.PodcastID)

	existingPendingEntry := byURL["http://127.0.0.1:9/existing-pending.xml"]
	assert.Equal(t, ImportOutcomePending, existingPendingEntry.Outcome)
	assert.False(t, existingPendingEntry.Created, "已有节目刷新失败不算新建")
	assert.Equal(t, existingPending.ID, existingPendingEntry.PodcastID)

	mergedEntry := byURL[mergedURL]
	assert.Equal(t, ImportOutcomeMerged, mergedEntry.Outcome)
	assert.False(t, mergedEntry.Created, "确认合并复用旧记录不算新建")
	assert.Equal(t, moved.ID, mergedEntry.PodcastID)

	failedEntry := byURL["ftp://example.invalid/rss"]
	assert.Equal(t, ImportOutcomeFailed, failedEntry.Outcome)
	assert.False(t, failedEntry.Created)
	assert.Zero(t, failedEntry.PodcastID, "失败未落库不得携带节目 ID")

	var total int64
	require.NoError(t, db.Model(&models.Podcast{}).Count(&total).Error)
	assert.Equal(t, int64(5), total, "只新增一条真实记录（新建空壳除外）")
}

// TestOldTaskCreationEvidenceStaysExact 验证缺少 created 字段的旧任务结果
// 解析：仅 outcome=new 仍作为确切的新建证据；旧 pending 不回填为新建。
func TestOldTaskCreationEvidenceStaysExact(t *testing.T) {
	entries := []ImportEntryResult{
		{Title: "新", FeedURL: "http://a/rss", Outcome: ImportOutcomeNew, PodcastID: 10},
		{Title: "旧待同步", FeedURL: "http://b/rss", Outcome: ImportOutcomePending, PodcastID: 11},
		{Title: "新待同步", FeedURL: "http://c/rss", Outcome: ImportOutcomePending, PodcastID: 12, Created: true},
		{Title: "合并", FeedURL: "http://d/rss", Outcome: ImportOutcomeMerged, PodcastID: 13},
	}
	ids := map[uint]bool{}
	for _, entry := range entries {
		if id, created := importEntryCreatedID(entry); created {
			ids[id] = true
		}
	}
	assert.Equal(t, map[uint]bool{10: true, 12: true}, ids)
}

// TestRetryTaskPersistsParentLink 验证重试任务持久化父任务链（#417/#418）。
func TestRetryTaskPersistsParentLink(t *testing.T) {
	db := setupTestDB(t)
	require.NoError(t, db.AutoMigrate(&models.ImportTask{}))
	parent, err := CreateImportTask(db, "parent.opml", 1)
	require.NoError(t, err)
	child, err := CreateChildImportTask(db, parent.ID, "retry-task-1(1条)", 1)
	require.NoError(t, err)
	require.NotNil(t, child.ParentTaskID)
	assert.Equal(t, parent.ID, *child.ParentTaskID)

	// 汇总跨链新建事实：父任务 pending 新建 + 子任务重试后成功。
	parent.ResultJSON = `[{"title":"A","feed_url":"http://a/rss","outcome":"pending","podcast_id":7,"created":true}]`
	require.NoError(t, db.Model(&models.ImportTask{}).Where("id = ?", parent.ID).Update("result_json", parent.ResultJSON).Error)
	child.ResultJSON = `[{"title":"A","feed_url":"http://a/rss","outcome":"updated","podcast_id":7},{"title":"B","feed_url":"http://b/rss","outcome":"new","podcast_id":8,"created":true}]`
	require.NoError(t, db.Model(&models.ImportTask{}).Where("id = ?", child.ID).Update("result_json", child.ResultJSON).Error)

	ids, err := CollectImportBatchCreatedPodcastIDs(db, parent.ID)
	require.NoError(t, err)
	assert.Equal(t, []uint{7, 8}, ids, "重试后的原批新建 ID 不丢失、不重复")

	// 从子任务查询也覆盖整条链。
	idsFromChild, err := CollectImportBatchCreatedPodcastIDs(db, child.ID)
	require.NoError(t, err)
	assert.ElementsMatch(t, ids, idsFromChild)
}

func TestDeepRetryChainKeepsRootCreationFacts(t *testing.T) {
	db := setupTestDB(t)
	require.NoError(t, db.AutoMigrate(&models.ImportTask{}))
	root := models.ImportTask{Status: "completed", ResultJSON: `[{"outcome":"new","podcast_id":7}]`}
	require.NoError(t, db.Create(&root).Error)
	last := root.ID
	for i := 0; i < 40; i++ {
		parent := last
		child := models.ImportTask{Status: "completed", ParentTaskID: &parent}
		require.NoError(t, db.Create(&child).Error)
		last = child.ID
	}
	ids, err := CollectImportBatchCreatedPodcastIDs(db, last)
	require.NoError(t, err)
	require.Equal(t, []uint{7}, ids)
	require.NoError(t, db.Model(&root).Update("parent_task_id", last).Error)
	_, err = CollectImportBatchCreatedPodcastIDs(db, last)
	require.Error(t, err)
}

func TestCreationSurvivesBeforeProgressDelivery(t *testing.T) {
	db := setupTestDB(t)
	require.NoError(t, db.AutoMigrate(&models.ImportTask{}))
	task, err := CreateImportTask(db, "crash.opml", 2)
	require.NoError(t, err)
	defer ActiveImportTasks.Delete(task.ID)
	reporter := NewTaskProgressReporter(NewLogProgressReporter(), db, task.ID)
	require.NoError(t, InitializeImportResults(reporter, []opml.Outline{{XMLURL: "https://a/feed"}, {XMLURL: "https://b/feed"}}))
	service, err := NewService(db, "")
	require.NoError(t, err)
	defer service.Close()
	p := models.Podcast{Title: "New", FeedURL: "https://a/feed", IsSubscribed: true}
	_, err = service.saveImportPodcast(&p, resolvedPodcastIdentity{}, importCreationRecorder(reporter, p.FeedURL))
	require.NoError(t, err)
	// Another worker reports first; the first result never reached the collector.
	require.NoError(t, reporter.(*taskProgressReporter).RecordImportResult(ImportEntryResult{FeedURL: "https://b/feed", Outcome: ImportOutcomeFailed}, 1))
	ids, err := CollectImportBatchCreatedPodcastIDs(db, task.ID)
	require.NoError(t, err)
	require.Equal(t, []uint{p.ID}, ids)
}

func TestCreationRollsBackWhenOriginCannotBeRecorded(t *testing.T) {
	db := setupTestDB(t)
	require.NoError(t, db.AutoMigrate(&models.ImportTask{}))
	task, err := CreateImportTask(db, "rollback.opml", 1)
	require.NoError(t, err)
	defer ActiveImportTasks.Delete(task.ID)
	reporter := NewTaskProgressReporter(NewLogProgressReporter(), db, task.ID)
	service, err := NewService(db, "")
	require.NoError(t, err)
	defer service.Close()
	p := models.Podcast{Title: "New", FeedURL: "https://rollback/feed", IsSubscribed: true}
	_, err = service.saveImportPodcast(&p, resolvedPodcastIdentity{}, importCreationRecorder(reporter, p.FeedURL))
	require.Error(t, err)
	var count int64
	require.NoError(t, db.Model(&models.Podcast{}).Where("feed_url = ?", p.FeedURL).Count(&count).Error)
	require.Zero(t, count, "failure to save origin must roll back the podcast too")
}
