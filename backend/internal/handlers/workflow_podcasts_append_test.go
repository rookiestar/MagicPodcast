package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"

	"magicpodcast/internal/database"
	"magicpodcast/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// newAppendRouter 构建隔离库上的成员追加路由。
func newAppendRouter(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	t.Chdir(t.TempDir())
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&models.Podcast{}, &models.Workflow{}, &models.Job{}))
	database.SetTestDB(db)
	t.Cleanup(database.ResetDB)
	handler := &WorkflowHandler{}
	router := gin.New()
	router.POST("/workflows/:id/podcasts/append", handler.AppendPodcasts)
	return router, db
}

func seedAppendPodcast(t *testing.T, db *gorm.DB, id int) models.Podcast {
	t.Helper()
	podcast := models.Podcast{XYZID: "append-" + strconv.Itoa(id), Title: "节目" + strconv.Itoa(id), FeedURL: fmt.Sprintf("http://f/%d.xml", id)}
	podcast.ID = uint(id)
	require.NoError(t, db.Create(&podcast).Error)
	return podcast
}

func postAppend(t *testing.T, router *gin.Engine, workflowID string, ids []int) (int, map[string]interface{}) {
	t.Helper()
	body, err := json.Marshal(map[string]interface{}{"podcast_ids": ids})
	require.NoError(t, err)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/workflows/"+workflowID+"/podcasts/append", bytes.NewReader(body)))
	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
	if response.Code != http.StatusOK {
		t.Logf("append response %d: %s", response.Code, response.Body.String())
	}
	return response.Code, payload
}

func loadAppendScope(t *testing.T, db *gorm.DB, workflowID uint) []int {
	t.Helper()
	var wf models.Workflow
	require.NoError(t, db.First(&wf, workflowID).Error)
	return wf.ScopeConfig.PodcastIDs
}

// TestAppendPodcastsSetSemanticsPreservingConfig 验证 AC3：目标原有 A、B，
// 提交 B、C、C 后恰为 A、B、C；实际新增 1；名称、调度、规则与启停状态
// 保持；不产生新 Job、不回写 last_job_id。
func TestAppendPodcastsSetSemanticsPreservingConfig(t *testing.T) {
	router, db := newAppendRouter(t)
	seedAppendPodcast(t, db, 1)
	seedAppendPodcast(t, db, 2)
	seedAppendPodcast(t, db, 3)

	workflow := models.Workflow{
		Name: "原工作流", Description: "原描述", Schedule: "30 7 * * 1",
		ScopeType:   models.ScopeTypeSpecificPodcasts,
		ScopeConfig: models.ScopeConfig{PodcastIDs: []int{1, 2}, CustomURLs: []string{"https://example.com/retained.xml"}},
		RulesConfig: models.RulesConfig{TimeRange: 7, Keywords: "关键词", LLMEnabled: true, LLMMaxEpisodes: 5},
	}
	require.NoError(t, db.Create(&workflow).Error)
	require.NoError(t, db.Model(&models.Workflow{}).Where("id = ?", workflow.ID).
		Update("is_enabled", false).Error)

	code, payload := postAppend(t, router, fmt.Sprint(workflow.ID), []int{2, 3, 3})
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, float64(1), payload["added"], "B 已在目标中，只新增 C")
	assert.Equal(t, float64(1), payload["already_member"])
	assert.Equal(t, float64(3), payload["podcast_count"])
	assert.Equal(t, []int{1, 2, 3}, loadAppendScope(t, db, workflow.ID))

	var reloaded models.Workflow
	require.NoError(t, db.First(&reloaded, workflow.ID).Error)
	assert.Equal(t, []string{"https://example.com/retained.xml"}, reloaded.ScopeConfig.CustomURLs)
	assert.Equal(t, "原工作流", reloaded.Name)
	assert.Equal(t, "原描述", reloaded.Description)
	assert.Equal(t, "30 7 * * 1", reloaded.Schedule)
	assert.False(t, reloaded.IsEnabled, "停用状态保持")
	assert.Equal(t, 7, reloaded.RulesConfig.TimeRange)
	assert.Equal(t, "关键词", reloaded.RulesConfig.Keywords)
	assert.True(t, reloaded.RulesConfig.LLMEnabled)
	assert.Equal(t, 5, reloaded.RulesConfig.LLMMaxEpisodes)
	assert.Nil(t, reloaded.LastJobID, "追加不回写 last_job_id")

	var jobCount int64
	require.NoError(t, db.Model(&models.Job{}).Count(&jobCount).Error)
	assert.Zero(t, jobCount, "追加不触发工作流运行")

	// AC4 重放：重复请求不重复添加。
	code, payload = postAppend(t, router, fmt.Sprint(workflow.ID), []int{3})
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, float64(0), payload["added"])
	assert.Equal(t, float64(1), payload["already_member"])
	assert.Equal(t, []int{1, 2, 3}, loadAppendScope(t, db, workflow.ID))
}

// TestAppendPodcastsConcurrentAddsKeepAllMembers 验证 AC4：并发追加 C、D
// 不丢成员、不重复。
func TestAppendPodcastsConcurrentAddsKeepAllMembers(t *testing.T) {
	router, db := newAppendRouter(t)
	for id := 1; id <= 4; id++ {
		seedAppendPodcast(t, db, id)
	}
	workflow := models.Workflow{Name: "并发", Schedule: "0 6 * * *", ScopeType: models.ScopeTypeSpecificPodcasts,
		ScopeConfig: models.ScopeConfig{PodcastIDs: []int{1, 2}}}
	require.NoError(t, db.Create(&workflow).Error)

	var wg sync.WaitGroup
	results := make([][]int, 2)
	for i, ids := range [][]int{{3}, {4}} {
		wg.Add(1)
		go func(slot int, appendIDs []int) {
			defer wg.Done()
			code, _ := postAppend(t, router, fmt.Sprint(workflow.ID), appendIDs)
			if code != http.StatusOK {
				t.Errorf("并发追加返回 %d", code)
			}
			results[slot] = appendIDs
		}(i, ids)
	}
	wg.Wait()

	scope := loadAppendScope(t, db, workflow.ID)
	assert.ElementsMatch(t, []int{1, 2, 3, 4}, scope, "并发追加后成员不丢")
	assert.Len(t, scope, 4, "成员不重复")
}

// TestAppendPodcastsRejectsInvalidTargets 验证 AC6：非法请求与失效目标
// 不产生部分追加。
func TestAppendPodcastsRejectsInvalidTargets(t *testing.T) {
	router, db := newAppendRouter(t)
	seedAppendPodcast(t, db, 1)
	seedAppendPodcast(t, db, 2)

	workflow := models.Workflow{Name: "目标", Schedule: "0 6 * * *", ScopeType: models.ScopeTypeSpecificPodcasts,
		ScopeConfig: models.ScopeConfig{PodcastIDs: []int{1}}}
	require.NoError(t, db.Create(&workflow).Error)
	allSubscribed := models.Workflow{Name: "全订阅", Schedule: "0 6 * * *", ScopeType: models.ScopeTypeAllSubscribed}
	require.NoError(t, db.Create(&allSubscribed).Error)

	// 不存在的节目：整体拒绝，成员保持不变。
	code, _ := postAppend(t, router, fmt.Sprint(workflow.ID), []int{2, 999})
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Equal(t, []int{1}, loadAppendScope(t, db, workflow.ID))

	// 已删除节目：整体拒绝。
	deleted := seedAppendPodcast(t, db, 5)
	require.NoError(t, db.Delete(&models.Podcast{}, deleted.ID).Error)
	code, _ = postAppend(t, router, fmt.Sprint(workflow.ID), []int{2, 5})
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Equal(t, []int{1}, loadAppendScope(t, db, workflow.ID))

	// 目标已删除：404。
	removed := models.Workflow{Name: "待删", Schedule: "0 6 * * *", ScopeType: models.ScopeTypeSpecificPodcasts,
		ScopeConfig: models.ScopeConfig{PodcastIDs: []int{}}}
	require.NoError(t, db.Create(&removed).Error)
	require.NoError(t, db.Delete(&models.Workflow{}, removed.ID).Error)
	code, _ = postAppend(t, router, fmt.Sprint(removed.ID), []int{2})
	assert.Equal(t, http.StatusNotFound, code)

	// 范围类型不符（例如范围已被改为全部订阅）：拒绝且不改写。
	require.NoError(t, db.Model(&models.Workflow{}).Where("id = ?", workflow.ID).
		Update("scope_type", models.ScopeTypeAllSubscribed).Error)
	code, _ = postAppend(t, router, fmt.Sprint(workflow.ID), []int{2})
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Equal(t, []int{1}, loadAppendScope(t, db, workflow.ID))

	// 空选择：拒绝。
	code, _ = postAppend(t, router, fmt.Sprint(workflow.ID), []int{})
	assert.Equal(t, http.StatusBadRequest, code)
}
