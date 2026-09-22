package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"magicpodcast/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// newHistorySyncAPIRouter 构建隔离库上的节目历史同步 API 路由（手动启动/查询）。
func newHistorySyncAPIRouter(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	t.Chdir(t.TempDir())
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&models.Podcast{}, &models.Workflow{}, &models.PodcastHistorySyncTask{}))
	handler := &SyncHandler{db: db}
	router := gin.New()
	router.POST("/podcasts/:id/episodes/sync", handler.SyncPodcastEpisodes)
	router.GET("/podcasts/:id/episodes/sync", handler.GetPodcastSyncTask)
	return router, db
}

func seedHistorySyncPodcast(t *testing.T, db *gorm.DB, title string) models.Podcast {
	t.Helper()
	podcast := models.Podcast{
		XYZID:        "hist-api-" + strconv.FormatInt(time.Now().UnixNano(), 10),
		Title:        title,
		FeedURL:      fmt.Sprintf("https://example.com/%s.xml", title),
		IsSubscribed: true,
	}
	require.NoError(t, db.Create(&podcast).Error)
	return podcast
}

// TestManualHistorySyncAPIEnqueueReuseAndQuery 走真实 HTTP 入口验证手动同步
// 的启动、幂等复用与查询契约（#464 手动启动合同）。
func TestManualHistorySyncAPIEnqueueReuseAndQuery(t *testing.T) {
	router, db := newHistorySyncAPIRouter(t)
	podcast := seedHistorySyncPodcast(t, db, "手动同步节目")

	// 未同步时查询返回 null 任务。
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/podcasts/%d/episodes/sync", podcast.ID), nil))
	require.Equal(t, http.StatusOK, resp.Code)
	var before struct {
		Data *map[string]interface{} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &before))
	assert.Nil(t, before.Data)

	// 首次启动：202 + 任务标识（标准 data 信封）。
	resp = httptest.NewRecorder()
	router.ServeHTTP(resp, httptest.NewRequest(http.MethodPost, fmt.Sprintf("/podcasts/%d/episodes/sync", podcast.ID), bytes.NewBufferString("{}")))
	require.Equal(t, http.StatusAccepted, resp.Code, resp.Body.String())
	var started struct {
		Data struct {
			Created bool `json:"created"`
			Task    struct {
				ID     uint   `json:"id"`
				Status string `json:"status"`
			} `json:"task"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &started))
	assert.True(t, started.Data.Created)
	assert.Equal(t, models.HistorySyncStatusPending, started.Data.Task.Status)

	// 重复点击：复用同一任务，不再新建。
	resp = httptest.NewRecorder()
	router.ServeHTTP(resp, httptest.NewRequest(http.MethodPost, fmt.Sprintf("/podcasts/%d/episodes/sync", podcast.ID), bytes.NewBufferString("{}")))
	require.Equal(t, http.StatusOK, resp.Code)
	var reused struct {
		Data struct {
			Created bool `json:"created"`
			Task    struct {
				ID uint `json:"id"`
			} `json:"task"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &reused))
	assert.False(t, reused.Data.Created)
	assert.Equal(t, started.Data.Task.ID, reused.Data.Task.ID)

	// 查询返回最新任务。
	resp = httptest.NewRecorder()
	router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/podcasts/%d/episodes/sync", podcast.ID), nil))
	require.Equal(t, http.StatusOK, resp.Code)
	var latest struct {
		Data struct {
			ID     uint   `json:"id"`
			Status string `json:"status"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &latest))
	assert.Equal(t, started.Data.Task.ID, latest.Data.ID)

	// 不存在的节目：404。
	resp = httptest.NewRecorder()
	router.ServeHTTP(resp, httptest.NewRequest(http.MethodPost, "/podcasts/99999/episodes/sync", bytes.NewBufferString("{}")))
	assert.Equal(t, http.StatusNotFound, resp.Code)
}

// TestAppendPodcastsRegistersHistorySyncTaskInResponse 验证成员追加在同一
// 事务登记历史同步任务，并在响应中区分成员保存与同步状态（#462）。
func TestAppendPodcastsRegistersHistorySyncTaskInResponse(t *testing.T) {
	router, db := newAppendRouter(t)
	seedAppendPodcast(t, db, 7)
	workflow := models.Workflow{
		Name:      "历史同步工作流",
		Schedule:  "0 6 * * *",
		ScopeType: models.ScopeTypeSpecificPodcasts,
		ScopeConfig: models.ScopeConfig{
			PodcastIDs: []int{},
		},
		IsEnabled: true,
	}
	require.NoError(t, db.Create(&workflow).Error)

	status, body := postAppend(t, router, strconv.FormatUint(uint64(workflow.ID), 10), []int{7})
	require.Equal(t, http.StatusOK, status, body)

	// 响应包含 history_sync 数组与任务标识。
	historyEntries, ok := body["history_sync"].([]interface{})
	require.True(t, ok, "响应应包含 history_sync")
	require.Len(t, historyEntries, 1)
	entry := historyEntries[0].(map[string]interface{})
	assert.Equal(t, float64(7), entry["podcast_id"])
	assert.NotNil(t, entry["task_id"])
	assert.Equal(t, models.HistorySyncStatusPending, entry["status"])

	// 任务行与成员同事务落库。
	var count int64
	require.NoError(t, db.Model(&models.PodcastHistorySyncTask{}).
		Where("podcast_id = ?", 7).Count(&count).Error)
	assert.Equal(t, int64(1), count)

	// 已是成员的重复追加：不再新建任务，状态指向既有任务。
	status, body = postAppend(t, router, strconv.FormatUint(uint64(workflow.ID), 10), []int{7})
	require.Equal(t, http.StatusOK, status, body)
	require.NoError(t, db.Model(&models.PodcastHistorySyncTask{}).
		Where("podcast_id = ?", 7).Count(&count).Error)
	assert.Equal(t, int64(1), count)
	historyEntries = body["history_sync"].([]interface{})
	require.Len(t, historyEntries, 1)
	entry = historyEntries[0].(map[string]interface{})
	assert.Equal(t, float64(7), entry["podcast_id"])
}

// TestAppendPodcastsWithoutCompletionDoesNotDuplicate 回归：已完成的节目重复
// 加入其他工作流不重复全量同步（任务计数保持不变）。
func TestAppendPodcastsWithoutCompletionDoesNotDuplicate(t *testing.T) {
	router, db := newAppendRouter(t)
	seedAppendPodcast(t, db, 9)
	workflowA := models.Workflow{Name: "A", Schedule: "0 6 * * *", ScopeType: models.ScopeTypeSpecificPodcasts, IsEnabled: true}
	require.NoError(t, db.Create(&workflowA).Error)
	workflowB := models.Workflow{Name: "B", Schedule: "0 7 * * *", ScopeType: models.ScopeTypeSpecificPodcasts, IsEnabled: true}
	require.NoError(t, db.Create(&workflowB).Error)

	status, body := postAppend(t, router, strconv.FormatUint(uint64(workflowA.ID), 10), []int{9})
	require.Equal(t, http.StatusOK, status, body)

	// 模拟既有完成记录后追加到第二个工作流。
	require.NoError(t, db.Model(&models.PodcastHistorySyncTask{}).Where("podcast_id = ?", 9).
		Updates(map[string]interface{}{"status": models.HistorySyncStatusCompleted, "finished_at": time.Now()}).Error)
	status, body = postAppend(t, router, strconv.FormatUint(uint64(workflowB.ID), 10), []int{9})
	require.Equal(t, http.StatusOK, status, body)

	var count int64
	require.NoError(t, db.Model(&models.PodcastHistorySyncTask{}).
		Where("podcast_id = ?", 9).Count(&count).Error)
	assert.Equal(t, int64(1), count, "历史已完成节目重复加入不新建任务")
}
