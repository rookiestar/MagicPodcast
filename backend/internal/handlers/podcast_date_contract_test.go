package handlers_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"magicpodcast/internal/cache"
	"magicpodcast/internal/database"
	"magicpodcast/internal/handlers"
	"magicpodcast/internal/middleware"
	"magicpodcast/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupDateContractRouter 构建隔离库上的节目列表/详情路由（#463 日期契约）。
func setupDateContractRouter(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:date_contract_%d?mode=memory&cache=shared", time.Now().UnixNano())), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&models.Podcast{}, &models.Tag{}, &models.Episode{}, &models.PodcastHistorySyncTask{}))
	database.SetTestDB(db)
	t.Cleanup(database.ResetDB)
	cache.GetCache().Clear()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.ErrorHandlerMiddleware())
	handler := handlers.NewPodcastHandler()
	router.GET("/api/v1/podcasts", handler.List)
	router.GET("/api/v1/podcasts/:id", handler.Get)
	return router, db
}

func getDateContractPodcast(t *testing.T, router *gin.Engine, id uint) map[string]interface{} {
	t.Helper()
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/podcasts/%d", id), nil))
	require.Equal(t, http.StatusOK, resp.Code)
	var payload struct {
		Data map[string]interface{} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &payload))
	return payload.Data
}

// TestPodcastDateContractNormalizesZeroDate 零值最新单集日期在 API 契约中
// 规范化为 null，且嵌入的 history_sync 缺省为 null（#463）。
func TestPodcastDateContractNormalizesZeroDate(t *testing.T) {
	router, db := setupDateContractRouter(t)

	zeroDatePodcast := models.Podcast{XYZID: "zero-date", Title: "零日期节目", FeedURL: "https://example.com/zero.xml", EpisodeCount: 0}
	require.NoError(t, db.Create(&zeroDatePodcast).Error)

	data := getDateContractPodcast(t, router, zeroDatePodcast.ID)

	raw, err := json.Marshal(data["newest_episode_date"])
	require.NoError(t, err)
	assert.Equal(t, "null", string(raw), "零值日期必须序列化为 null，不得输出 0001-01-01")

	raw, err = json.Marshal(data["history_sync"])
	require.NoError(t, err)
	assert.Equal(t, "null", string(raw), "从未同步的节目 history_sync 为 null")

	// 列表（summary 视图）同一契约。
	listResp := httptest.NewRecorder()
	router.ServeHTTP(listResp, httptest.NewRequest(http.MethodGet, "/api/v1/podcasts?view=summary", nil))
	require.Equal(t, http.StatusOK, listResp.Code)
	var list struct {
		Data []map[string]interface{} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(listResp.Body.Bytes(), &list))
	require.Len(t, list.Data, 1)
	raw, err = json.Marshal(list.Data[0]["newest_episode_date"])
	require.NoError(t, err)
	assert.Equal(t, "null", string(raw))
}

// TestPodcastRecentUpdateSortFallsBackForMissingDate 缺失日期的节目按创建
// 时间回退排序，且顺序分页稳定（#463）。
func TestPodcastRecentUpdateSortFallsBackForMissingDate(t *testing.T) {
	router, db := setupDateContractRouter(t)

	older := models.Podcast{XYZID: "sort-old", Title: "较早创建", FeedURL: "https://example.com/old.xml"}
	require.NoError(t, db.Create(&older).Error)
	require.NoError(t, db.Model(&older).Update("created_at", time.Now().Add(-time.Hour)).Error)

	newer := models.Podcast{XYZID: "sort-new", Title: "较新创建", FeedURL: "https://example.com/new.xml"}
	require.NoError(t, db.Create(&newer).Error)

	// 两档节目均无有效最新单集日期：按创建时间倒序，新创建者在前。
	listResp := httptest.NewRecorder()
	router.ServeHTTP(listResp, httptest.NewRequest(http.MethodGet, "/api/v1/podcasts?sort_by=recent_update", nil))
	require.Equal(t, http.StatusOK, listResp.Code)
	var list struct {
		Data []struct {
			ID uint `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(listResp.Body.Bytes(), &list))
	require.Len(t, list.Data, 2)
	assert.Equal(t, newer.ID, list.Data[0].ID)
	assert.Equal(t, older.ID, list.Data[1].ID)

	// 有效日期优先于创建时间回退：给同批节目一个近期有效日期。
	validDated := models.Podcast{XYZID: "sort-valid", Title: "有日期节目", FeedURL: "https://example.com/valid.xml"}
	require.NoError(t, db.Create(&validDated).Error)
	require.NoError(t, db.Model(&validDated).Updates(map[string]interface{}{
		"created_at":          time.Now().Add(-time.Hour),
		"newest_episode_date": time.Now(),
	}).Error)

	cache.GetCache().Clear()
	listResp = httptest.NewRecorder()
	router.ServeHTTP(listResp, httptest.NewRequest(http.MethodGet, "/api/v1/podcasts?sort_by=recent_update", nil))
	require.Equal(t, http.StatusOK, listResp.Code)
	var listWithValid struct {
		Data []struct {
			ID uint `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(listResp.Body.Bytes(), &listWithValid))
	require.Len(t, listWithValid.Data, 3)
	assert.Equal(t, validDated.ID, listWithValid.Data[0].ID, "近期有效日期应排在缺失日期（回退创建时间）之前")
}

// TestPodcastHistorySyncEmbeddedInList 节目列表嵌入历史同步状态摘要（#462）。
func TestPodcastHistorySyncEmbeddedInList(t *testing.T) {
	router, db := setupDateContractRouter(t)

	podcast := models.Podcast{XYZID: "embedded-sync", Title: "嵌入任务节目", FeedURL: "https://example.com/embedded.xml"}
	require.NoError(t, db.Create(&podcast).Error)
	task := models.PodcastHistorySyncTask{
		PodcastID:      podcast.ID,
		Trigger:        models.HistorySyncTriggerWorkflow,
		Status:         models.HistorySyncStatusRunning,
		Attempts:       1,
		ProcessedCount: 120,
		StartedAt:      ptrForDateTest(time.Now()),
	}
	require.NoError(t, db.Create(&task).Error)

	listResp := httptest.NewRecorder()
	router.ServeHTTP(listResp, httptest.NewRequest(http.MethodGet, "/api/v1/podcasts?view=summary", nil))
	require.Equal(t, http.StatusOK, listResp.Code)
	var list struct {
		Data []struct {
			ID          uint   `json:"id"`
			HistorySync *struct {
				TaskID         uint   `json:"task_id"`
				Status         string `json:"status"`
				ProcessedCount int    `json:"processed_count"`
			} `json:"history_sync"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(listResp.Body.Bytes(), &list))
	require.Len(t, list.Data, 1)
	require.NotNil(t, list.Data[0].HistorySync)
	assert.Equal(t, task.ID, list.Data[0].HistorySync.TaskID)
	assert.Equal(t, models.HistorySyncStatusRunning, list.Data[0].HistorySync.Status)
	assert.Equal(t, 120, list.Data[0].HistorySync.ProcessedCount)
}

func ptrForDateTest(t time.Time) *time.Time {
	return &t
}
