package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"magicpodcast/internal/models"
	syncpkg "magicpodcast/internal/sync"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// newNewPodcastsRouter 构建带隔离库的「本批新建节目」查询路由。
func newNewPodcastsRouter(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	t.Chdir(t.TempDir())
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&models.Podcast{}, &models.ImportTask{}, &models.Workflow{}))
	service, err := syncpkg.NewService(db, "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close()) })
	handler := &SyncHandler{syncService: service, db: db}
	router := gin.New()
	router.GET("/tasks/:id/new-podcasts", handler.GetImportTaskNewPodcasts)
	return router, db
}

func getNewPodcasts(t *testing.T, router *gin.Engine, taskID string) (int, newPodcastsPayload) {
	t.Helper()
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/tasks/"+taskID+"/new-podcasts", nil))
	var payload newPodcastsPayload
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
	return response.Code, payload
}

type newPodcastsPayload struct {
	Success  bool                   `json:"success"`
	TaskID   uint                   `json:"task_id"`
	Total    int                    `json:"total"`
	Podcasts []importNewPodcastItem `json:"podcasts"`
	Message  string                 `json:"message"`
}

// TestImportTaskNewPodcastsEndpointClassifiesBatch 验证清单只收录本批实际
// 新建的节目，携带当前资料状态与工作流归属；已有更新、确认合并与已删除
// 记录不进入（#417/#418 AC1/AC2/AC7/AC11）。
func TestImportTaskNewPodcastsEndpointClassifiesBatch(t *testing.T) {
	router, db := newNewPodcastsRouter(t)

	seed := func(id uint, title, feedURL string, ready, subscribed bool) models.Podcast {
		podcast := models.Podcast{
			XYZID: "seed-" + strconv.FormatUint(uint64(id), 10),
			Title: title, FeedURL: feedURL,
		}
		podcast.ID = id
		require.NoError(t, db.Create(&podcast).Error)
		// 布尔列带 default:true：显式 Update 才能写入 false。
		require.NoError(t, db.Model(&models.Podcast{}).Where("id = ?", id).
			Updates(map[string]interface{}{"feed_url_valid": ready, "is_subscribed": subscribed}).Error)
		podcast.FeedURLValid = ready
		podcast.IsSubscribed = subscribed
		return podcast
	}
	p10 := seed(10, "新建就绪", "http://f/10.xml", true, true)
	p11 := seed(11, "新建待同步", "http://f/11.xml", false, true)
	p12 := seed(12, "新建后删除", "http://f/12.xml", false, true)
	seed(13, "合并目标", "http://f/13.xml", true, true)
	p14 := seed(14, "重试新建", "http://f/14.xml", true, false)

	require.NoError(t, db.Delete(&models.Podcast{}, p12.ID).Error, "预置一条创建后被删除的记录")

	specific := models.Workflow{Name: "专题工作流", Schedule: "0 6 * * *", ScopeType: models.ScopeTypeSpecificPodcasts,
		ScopeConfig: models.ScopeConfig{PodcastIDs: []int{int(p10.ID), 999}}}
	require.NoError(t, db.Create(&specific).Error)
	disabledAll := models.Workflow{Name: "停用全订阅", Schedule: "0 7 * * *", ScopeType: models.ScopeTypeAllSubscribed, IsEnabled: false}
	require.NoError(t, db.Create(&disabledAll).Error)
	custom := models.Workflow{Name: "自定义源", Schedule: "0 8 * * *", ScopeType: models.ScopeTypeCustomSources,
		ScopeConfig: models.ScopeConfig{CustomURLs: []string{p14.FeedURL}}}
	require.NoError(t, db.Create(&custom).Error)

	// 父任务：新建就绪、新建待同步、新建后被删除、确认合并（不算新建）。
	parent, err := syncpkg.CreateImportTask(db, "batch.opml", 4)
	require.NoError(t, err)
	parent.ResultJSON = `[` +
		`{"title":"新建就绪","feed_url":"http://f/10.xml","outcome":"new","podcast_id":10,"created":true},` +
		`{"title":"新建待同步","feed_url":"http://f/11.xml","outcome":"pending","podcast_id":11,"created":true},` +
		`{"title":"新建后删除","feed_url":"http://f/12.xml","outcome":"pending","podcast_id":12,"created":true},` +
		`{"title":"合并目标","feed_url":"http://f/13.xml","outcome":"merged","podcast_id":13}` +
		`]`
	require.NoError(t, db.Model(&models.ImportTask{}).Where("id = ?", parent.ID).Update("result_json", parent.ResultJSON).Error)

	// 重试任务：原批待同步重试成功（updated，不重复计），并新建另一档。
	child, err := syncpkg.CreateChildImportTask(db, parent.ID, "重试任务", 2)
	require.NoError(t, err)
	child.ResultJSON = `[` +
		`{"title":"新建待同步","feed_url":"http://f/11.xml","outcome":"updated","podcast_id":11},` +
		`{"title":"重试新建","feed_url":"http://f/14.xml","outcome":"new","podcast_id":14,"created":true}` +
		`]`
	require.NoError(t, db.Model(&models.ImportTask{}).Where("id = ?", child.ID).Update("result_json", child.ResultJSON).Error)
	// 重试成功的库效应：原批待同步空壳补全资料后就绪。
	require.NoError(t, db.Model(&models.Podcast{}).Where("id = ?", p11.ID).Update("feed_url_valid", true).Error)

	code, payload := getNewPodcasts(t, router, "1")
	require.Equal(t, http.StatusOK, code)
	assert.True(t, payload.Success)
	require.Len(t, payload.Podcasts, 3, "只收录实际新建：10、11、14；合并与已删除不进入")
	assert.Equal(t, []uint{10, 11, 14}, []uint{payload.Podcasts[0].ID, payload.Podcasts[1].ID, payload.Podcasts[2].ID})

	byID := map[uint]importNewPodcastItem{}
	for _, item := range payload.Podcasts {
		byID[item.ID] = item
	}
	assert.True(t, byID[10].Ready)
	assert.True(t, byID[11].Ready, "重试成功后资料状态应取当前记录")
	assert.False(t, byID[14].Ready == false && byID[14].IsSubscribed == true, "状态与订阅标记取当前记录")

	// 归属：指定成员 + 停用的全部订阅（停用仍计入覆盖）+ 自定义源按地址。
	wfNames := func(item importNewPodcastItem) map[uint]bool {
		refs := map[uint]bool{}
		for _, ref := range item.Workflows {
			refs[ref.ID] = true
		}
		return refs
	}
	assert.Equal(t, map[uint]bool{specific.ID: true, disabledAll.ID: true}, wfNames(byID[10]))
	assert.Equal(t, map[uint]bool{disabledAll.ID: true}, wfNames(byID[11]), "待同步但已订阅也计入全部订阅覆盖")
	assert.Equal(t, map[uint]bool{custom.ID: true}, wfNames(byID[14]), "自定义源按 Feed 地址明确对应")

	// 从子任务查询同一批次（刷新后页面可能停在重试任务上）。
	codeFromChild, payloadFromChild := getNewPodcasts(t, router, "2")
	require.Equal(t, http.StatusOK, codeFromChild)
	assert.Len(t, payloadFromChild.Podcasts, 3)
}

// TestImportTaskNewPodcastsLegacyTaskUsesExactEvidence 验证旧任务（缺少
// created 字段）只按确切的 outcome=new 展示，不按状态或时间回填。
func TestImportTaskNewPodcastsLegacyTaskUsesExactEvidence(t *testing.T) {
	router, db := newNewPodcastsRouter(t)
	legacyNew := models.Podcast{XYZID: "legacy-new", Title: "旧新建", FeedURL: "http://f/legacy.xml", FeedURLValid: true, IsSubscribed: true}
	require.NoError(t, db.Create(&legacyNew).Error)
	legacyPending := models.Podcast{XYZID: "legacy-pending", Title: "旧待同步", FeedURL: "http://f/legacy-pending.xml", FeedURLValid: false, IsSubscribed: true}
	require.NoError(t, db.Create(&legacyPending).Error)

	task, err := syncpkg.CreateImportTask(db, "legacy.opml", 2)
	require.NoError(t, err)
	// 旧格式：没有 created 字段。
	task.ResultJSON = `[` +
		`{"title":"旧新建","feed_url":"http://f/legacy.xml","outcome":"new","podcast_id":` + uintString(legacyNew.ID) + `},` +
		`{"title":"旧待同步","feed_url":"http://f/legacy-pending.xml","outcome":"pending","podcast_id":` + uintString(legacyPending.ID) + `}` +
		`]`
	require.NoError(t, db.Model(&models.ImportTask{}).Where("id = ?", task.ID).Update("result_json", task.ResultJSON).Error)

	code, payload := getNewPodcasts(t, router, "1")
	require.Equal(t, http.StatusOK, code)
	require.Len(t, payload.Podcasts, 1)
	assert.Equal(t, legacyNew.ID, payload.Podcasts[0].ID)
}

// TestImportTaskNewPodcastsReturnsEmptyWorkflowArray 保证未被工作流覆盖的节目
// 也返回 JSON 数组，避免前端把合法的「无覆盖」状态当成 null 处理。
func TestImportTaskNewPodcastsReturnsEmptyWorkflowArray(t *testing.T) {
	router, db := newNewPodcastsRouter(t)
	podcast := models.Podcast{
		XYZID: "no-workflow", Title: "未加入工作流", FeedURL: "https://f/no-workflow.xml",
		FeedURLValid: true, IsSubscribed: false,
	}
	require.NoError(t, db.Create(&podcast).Error)

	task, err := syncpkg.CreateImportTask(db, "no-workflow.opml", 1)
	require.NoError(t, err)
	task.ResultJSON = `[{"title":"未加入工作流","feed_url":"https://f/no-workflow.xml","outcome":"new","podcast_id":` + uintString(podcast.ID) + `,"created":true}]`
	require.NoError(t, db.Model(&models.ImportTask{}).Where("id = ?", task.ID).Update("result_json", task.ResultJSON).Error)

	code, payload := getNewPodcasts(t, router, strconv.FormatUint(uint64(task.ID), 10))
	require.Equal(t, http.StatusOK, code)
	require.Len(t, payload.Podcasts, 1)
	assert.NotNil(t, payload.Podcasts[0].Workflows)
	assert.Empty(t, payload.Podcasts[0].Workflows)
}

// TestImportTaskNewPodcastsEmptyAndMissing 验证空清单与不存在任务的响应。
func TestImportTaskNewPodcastsEmptyAndMissing(t *testing.T) {
	router, db := newNewPodcastsRouter(t)

	task, err := syncpkg.CreateImportTask(db, "empty.opml", 1)
	require.NoError(t, err)
	task.ResultJSON = `[{"title":"已有","feed_url":"http://f/x.xml","outcome":"unchanged","podcast_id":0}]`
	require.NoError(t, db.Model(&models.ImportTask{}).Where("id = ?", task.ID).Update("result_json", task.ResultJSON).Error)

	code, payload := getNewPodcasts(t, router, "1")
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, 0, payload.Total)
	assert.NotNil(t, payload.Podcasts, "空结果应是空数组而非 null")

	code, _ = getNewPodcasts(t, router, "404")
	assert.Equal(t, http.StatusNotFound, code)
}

func uintString(value uint) string {
	return strconv.FormatUint(uint64(value), 10)
}
