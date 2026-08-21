package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"magicpodcast/internal/cache"
	"magicpodcast/internal/database"
	"magicpodcast/internal/feed"
	"magicpodcast/internal/models"
	"magicpodcast/internal/scheduler"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestBatchRemainingMsForFinishedAndActiveJobs(t *testing.T) {
	start := time.Date(2026, 7, 25, 10, 0, 0, 0, time.UTC)
	end := start.Add(5 * time.Minute)
	finished := &models.Job{Status: models.JobStatusCompleted, StartTime: &start, EndTime: &end}
	rem := batchRemainingMs(finished)
	require.NotNil(t, rem)
	// 10-minute window minus 5 minutes elapsed → 5 minutes remaining (#44).
	require.Equal(t, int64((5 * time.Minute).Milliseconds()), *rem)
	require.Equal(t, 10*time.Minute, feed.DefaultBatchDuration)

	activeStart := time.Now().Add(-2 * time.Minute)
	active := &models.Job{Status: models.JobStatusRunning, StartTime: &activeStart}
	activeRem := batchRemainingMs(active)
	require.NotNil(t, activeRem)
	// Roughly 8 minutes left of the 10-minute window; allow clock skew.
	require.Greater(t, *activeRem, int64((7 * time.Minute).Milliseconds()))
	require.Less(t, *activeRem, int64((9 * time.Minute).Milliseconds()))
}

func TestWorkflowHomepagePublishConfigIsAutomaticAndCronDerived(t *testing.T) {
	cache.GetCache().Clear()
	db, err := gorm.Open(
		sqlite.Open(fmt.Sprintf("file:workflow_homepage_config_%d?mode=memory&cache=shared", time.Now().UnixNano())),
		&gorm.Config{},
	)
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&models.Workflow{},
		&models.Job{},
		&models.Podcast{},
	))
	database.SetTestDB(db)
	t.Cleanup(func() {
		database.ResetDB()
		sqlDB, openErr := db.DB()
		if openErr == nil {
			_ = sqlDB.Close()
		}
	})

	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewWorkflowHandler(nil, scheduler.NewScheduler(db, nil), nil)
	router.POST("/api/v1/workflows", handler.Create)
	router.PUT("/api/v1/workflows/:id", handler.Update)

	createBody := map[string]any{
		"name":                "首页精选日报",
		"description":         "测试发布配置",
		"schedule":            "0 8 * * *",
		"scope_type":          models.ScopeTypeAllSubscribed,
		"scope_config":        models.ScopeConfig{},
		"rules_config":        models.RulesConfig{},
		"is_enabled":          false,
		"publish_to_homepage": true,
		"report_type":         "daily",
	}
	createJSON, err := json.Marshal(createBody)
	require.NoError(t, err)
	createRecorder := httptest.NewRecorder()
	router.ServeHTTP(
		createRecorder,
		httptest.NewRequest(http.MethodPost, "/api/v1/workflows", bytes.NewReader(createJSON)),
	)
	require.Equal(t, http.StatusCreated, createRecorder.Code, createRecorder.Body.String())

	var stored models.Workflow
	require.NoError(t, db.Where("name = ?", "首页精选日报").First(&stored).Error)
	require.True(t, stored.PublishToHomepage)
	require.Equal(t, "daily", stored.ReportType)

	updateBody := map[string]any{
		"name":                stored.Name,
		"description":         stored.Description,
		"schedule":            "0 0 8 * * 5",
		"scope_type":          stored.ScopeType,
		"scope_config":        stored.ScopeConfig,
		"rules_config":        stored.RulesConfig,
		"is_enabled":          false,
		"publish_to_homepage": true,
		"report_type":         "weekly",
		"confirmation_text":   fmt.Sprintf("UPDATE WORKFLOW %d", stored.ID),
	}
	updateJSON, err := json.Marshal(updateBody)
	require.NoError(t, err)
	updateRecorder := httptest.NewRecorder()
	router.ServeHTTP(
		updateRecorder,
		httptest.NewRequest(
			http.MethodPut,
			fmt.Sprintf("/api/v1/workflows/%d", stored.ID),
			bytes.NewReader(updateJSON),
		),
	)
	require.Equal(t, http.StatusOK, updateRecorder.Code, updateRecorder.Body.String())

	require.NoError(t, db.First(&stored, stored.ID).Error)
	require.True(t, stored.PublishToHomepage)
	require.Equal(t, "weekly", stored.ReportType)

	updateBody["publish_to_homepage"] = false
	updateBody["confirmation_text"] = fmt.Sprintf("UPDATE WORKFLOW %d", stored.ID)
	disableJSON, err := json.Marshal(updateBody)
	require.NoError(t, err)
	disableRecorder := httptest.NewRecorder()
	router.ServeHTTP(
		disableRecorder,
		httptest.NewRequest(
			http.MethodPut,
			fmt.Sprintf("/api/v1/workflows/%d", stored.ID),
			bytes.NewReader(disableJSON),
		),
	)
	require.Equal(t, http.StatusOK, disableRecorder.Code, disableRecorder.Body.String())

	require.NoError(t, db.First(&stored, stored.ID).Error)
	require.True(t, stored.PublishToHomepage)
	require.Equal(t, "weekly", stored.ReportType)
}

func TestListJobsSummaryIncludesLLMErrorWithoutLongSummary(t *testing.T) {
	cache.GetCache().Clear()
	db, err := gorm.Open(
		sqlite.Open(fmt.Sprintf("file:list_jobs_llm_error_%d?mode=memory&cache=shared", time.Now().UnixNano())),
		&gorm.Config{},
	)
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&models.Workflow{},
		&models.Job{},
		&models.Report{},
	))
	database.SetTestDB(db)
	t.Cleanup(func() {
		database.ResetDB()
		sqlDB, openErr := db.DB()
		if openErr == nil {
			_ = sqlDB.Close()
		}
	})

	workflow := models.Workflow{
		Name:      "教育精选",
		ScopeType: models.ScopeTypeAllSubscribed,
		IsEnabled: true,
	}
	require.NoError(t, db.Create(&workflow).Error)
	now := time.Now()
	job := models.Job{
		WorkflowID:        workflow.ID,
		Status:            models.JobStatusCompleted,
		TriggeredBy:       "cron",
		StartTime:         &now,
		EndTime:           &now,
		PodcastsProcessed: 9,
		ErrorCount:        0,
	}
	require.NoError(t, db.Create(&job).Error)
	require.NoError(t, db.Create(&models.Report{
		JobID:         job.ID,
		Title:         "教育精选",
		Content:       "# 报告正文",
		GeneratedAt:   now,
		LLMSummary:    strings.Repeat("LONG-SUMMARY-", 200),
		LLMError:      "读取响应失败: context deadline exceeded (Client.Timeout or context cancellation while reading body)",
		LLMModelUsed:  "",
		LLMTokensUsed: 0,
	}).Error)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewWorkflowHandler(nil, scheduler.NewScheduler(db, nil), nil)
	router.GET("/api/v1/workflows/:id/jobs", handler.ListJobs)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(
		recorder,
		httptest.NewRequest(
			http.MethodGet,
			fmt.Sprintf("/api/v1/workflows/%d/jobs?page=1&page_size=10&view=summary", workflow.ID),
			nil,
		),
	)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

	var envelope struct {
		Success bool `json:"success"`
		Data    struct {
			Jobs []map[string]any `json:"jobs"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &envelope))
	require.Len(t, envelope.Data.Jobs, 1)
	got := envelope.Data.Jobs[0]
	require.Equal(t, "completed", got["status"])
	require.Equal(t, float64(0), got["error_count"])
	require.Equal(t, "读取响应失败: context deadline exceeded (Client.Timeout or context cancellation while reading body)", got["llm_error"])
	_, hasSummary := got["llm_summary"]
	require.False(t, hasSummary)
}
