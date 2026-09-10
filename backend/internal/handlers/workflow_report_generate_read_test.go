package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"magicpodcast/internal/cache"
	"magicpodcast/internal/database"
	"magicpodcast/internal/llm"
	"magicpodcast/internal/models"
	"magicpodcast/internal/scheduler"
	"magicpodcast/internal/workflow"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type reportSummarizerStub struct {
	err    error
	result *llm.SummaryResult
}

func (s *reportSummarizerStub) GenerateForReport(ctx context.Context, data []llm.EpisodeReportData, workflowName string, userPrompt string, options llm.SummaryOptions) (*llm.SummaryResult, error) {
	if s.result != nil || s.err != nil {
		return s.result, s.err
	}
	return &llm.SummaryResult{Summary: "可读摘要", ModelUsed: "deepseek-v4-flash", TokensUsed: 12}, nil
}

func setupReportAPI(t *testing.T, summarizer workflow.SummarizerInterface) (*gorm.DB, *gin.Engine) {
	t.Helper()
	cache.GetCache().Clear()
	db, err := gorm.Open(
		sqlite.Open(fmt.Sprintf("file:report_api_%d?mode=memory&cache=shared", time.Now().UnixNano())),
		&gorm.Config{},
	)
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&models.Workflow{}, &models.Job{}, &models.JobExecution{},
		&models.Report{}, &models.Podcast{}, &models.Episode{},
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
	handler := NewWorkflowHandler(nil, scheduler.NewScheduler(db, nil), summarizer)
	router.GET("/api/v1/jobs/:id/report", handler.GetJobReport)
	router.POST("/api/v1/jobs/:id/regenerate-llm", handler.RegenerateLLMSummary)
	return db, router
}

func seedReportJob(t *testing.T, db *gorm.DB, llmEnabled bool) models.Job {
	t.Helper()
	wf := models.Workflow{
		Name:        "教育精选",
		ScopeType:   models.ScopeTypeAllSubscribed,
		IsEnabled:   true,
		RulesConfig: models.RulesConfig{TimeRange: 1, LLMEnabled: llmEnabled},
	}
	require.NoError(t, db.Create(&wf).Error)
	podcast := models.Podcast{
		Title: "测试节目", Author: "主播", FeedURL: "https://example.com/feed.xml",
		XYZID: fmt.Sprintf("xyz-%d", time.Now().UnixNano()), IsSubscribed: true,
	}
	require.NoError(t, db.Create(&podcast).Error)
	now := time.Now().UTC().Truncate(time.Second)
	start := now.Add(-time.Minute)
	require.NoError(t, db.Create(&models.Episode{
		PodcastID: podcast.ID, GUID: fmt.Sprintf("guid-%d", time.Now().UnixNano()),
		Title: "本周单集", ShowNotes: "笔记", PublishedDate: start.Add(-time.Hour),
	}).Error)
	job := models.Job{
		WorkflowID: wf.ID, Status: models.JobStatusCompleted, TriggeredBy: "cron",
		StartTime: &start, EndTime: &now, PodcastsProcessed: 1,
	}
	require.NoError(t, db.Create(&job).Error)
	require.NoError(t, db.Create(&models.JobExecution{
		JobID: job.ID, PodcastID: &podcast.ID, PodcastTitle: podcast.Title,
		PodcastFeedURL: podcast.FeedURL, Status: models.ExecutionStatusSuccess,
		EpisodesMatched: 1,
	}).Error)
	return job
}

func getReportJSON(t *testing.T, router *gin.Engine, jobID uint) map[string]any {
	t.Helper()
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/jobs/%d/report", jobID), nil))
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	var envelope struct {
		Success bool           `json:"success"`
		Data    map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &envelope))
	require.True(t, envelope.Success)
	return envelope.Data
}

func TestGenerateThenReadReportStats(t *testing.T) {
	cases := []struct {
		name       string
		enabled    bool
		summarizer *reportSummarizerStub
		status     string
		lineHas    string
	}{
		{
			name:       "success",
			enabled:    true,
			summarizer: &reportSummarizerStub{result: &llm.SummaryResult{Summary: "可读摘要", ModelUsed: "deepseek-v4-flash", TokensUsed: 12}},
			status:     models.AIStatusGenerated,
			lineHas:    "AI 已生成 · deepseek-v4-flash · 12 Token",
		},
		{
			name:    "empty with tokens",
			enabled: true,
			summarizer: &reportSummarizerStub{
				err:    llm.ErrEmptyCompletion,
				result: &llm.SummaryResult{ModelUsed: "deepseek-v4-flash", TokensUsed: 7245},
			},
			status:  models.AIStatusNotGenerated,
			lineHas: "AI 未生成 · deepseek-v4-flash · 7.2K Token",
		},
		{
			name:    "truncated",
			enabled: true,
			summarizer: &reportSummarizerStub{
				err: llm.ErrIncompleteCompletion,
				result: &llm.SummaryResult{
					Summary: "半份", ModelUsed: "deepseek-v4-flash", TokensUsed: 120,
					Incomplete: true, FinishReason: "length",
				},
			},
			status:  models.AIStatusIncomplete,
			lineHas: "AI 不完整",
		},
		{
			name:       "call failed",
			enabled:    true,
			summarizer: &reportSummarizerStub{err: fmt.Errorf("upstream down")},
			status:     models.AIStatusNotGenerated,
			lineHas:    "AI 未生成",
		},
		{
			name:    "disabled",
			enabled: false,
			status:  models.AIStatusDisabled,
			lineHas: "AI 未启用 · 未调用 · —",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var summarizer workflow.SummarizerInterface
			if tc.summarizer != nil {
				summarizer = tc.summarizer
			}
			db, router := setupReportAPI(t, summarizer)
			job := seedReportJob(t, db, tc.enabled)
			rg := workflow.NewReportGenerator(db, summarizer)
			report, err := rg.GenerateForJob(context.Background(), &job)
			require.NoError(t, err)
			require.Contains(t, report.Content, "本周单集")

			body := getReportJSON(t, router, job.ID)
			stats, ok := body["report_stats"].(map[string]any)
			require.True(t, ok)
			require.Equal(t, tc.status, stats["ai_status"])
			require.Contains(t, stats["line"], tc.lineHas)
			require.Contains(t, body["content"], "本周单集")
			if tc.status != models.AIStatusGenerated {
				require.NotEqual(t, models.AIStatusGenerated, stats["ai_status"])
			}
		})
	}
}

func TestReadHistoricalReportUnknownDoesNotForgeZeroTokens(t *testing.T) {
	db, router := setupReportAPI(t, nil)
	job := seedReportJob(t, db, true)
	require.NoError(t, db.Create(&models.Report{
		JobID:         job.ID,
		Title:         "旧报告",
		Content:       "# 旧报告\n\n> quote 用户引用\n\n单集详情",
		PodcastsCount: 3,
		MatchedCount:  3,
		EpisodesCount: 3,
		GeneratedAt:   time.Now(),
	}).Error)

	body := getReportJSON(t, router, job.ID)
	stats := body["report_stats"].(map[string]any)
	require.Equal(t, models.AIStatusUnknown, stats["ai_status"])
	require.Equal(t, "unknown", stats["tokens_status"])
	require.Nil(t, stats["tokens"])
	require.Contains(t, stats["line"], "Token 未知")
	require.NotContains(t, stats["line"], "0 Token")
	require.Contains(t, body["content"], "> quote 用户引用")
}

func TestRegenerateLLMSummarySuccessAndFailure(t *testing.T) {
	stub := &reportSummarizerStub{result: &llm.SummaryResult{Summary: "第二版摘要", ModelUsed: "deepseek-v4-flash", TokensUsed: 20}}
	db, router := setupReportAPI(t, stub)
	job := seedReportJob(t, db, true)
	require.NoError(t, db.Create(&models.Report{
		JobID:         job.ID,
		Title:         "教育精选",
		Content:       workflow.ReplaceOrInsertLLMSummary("# 教育精选\n\n> **🕐 执行**: today\n\n## 📝 单集详情\n正文\n", "## 总体概览\n原有效摘要\n\n---\n\n## 核心内容\n旧观点"),
		PodcastsCount: 1,
		MatchedCount:  1,
		GeneratedAt:   time.Now(),
		LLMSummary:    "原有效摘要",
		LLMModelUsed:  "deepseek-v4-flash",
		LLMTokensUsed: 12,
	}).Error)

	confirm := fmt.Sprintf(`{"confirmation_text":"REGENERATE LLM %d"}`, job.ID)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/jobs/%d/regenerate-llm", job.ID), bytes.NewBufferString(confirm))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

	body := getReportJSON(t, router, job.ID)
	require.Equal(t, "第二版摘要", body["llm_summary"])
	require.Equal(t, 1, strings.Count(body["content"].(string), "## 🤖 AI智能摘要"))
	require.NotContains(t, body["content"], "原有效摘要")
	require.NotContains(t, body["content"], "旧观点")
	require.Contains(t, body["content"], "正文")

	stub.err = fmt.Errorf("本次失败")
	stub.result = nil
	failRec := httptest.NewRecorder()
	failReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/jobs/%d/regenerate-llm", job.ID), bytes.NewBufferString(confirm))
	failReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(failRec, failReq)
	require.Equal(t, http.StatusInternalServerError, failRec.Code)

	after := getReportJSON(t, router, job.ID)
	require.Equal(t, "第二版摘要", after["llm_summary"])
	require.Contains(t, after["llm_error"], "本次失败")
	stats := after["report_stats"].(map[string]any)
	require.Equal(t, models.AIStatusGenerated, stats["ai_status"])
}

func TestRegenerateLLMSummaryIncompleteDoesNotOverwriteValidSummary(t *testing.T) {
	stub := &reportSummarizerStub{
		err: llm.ErrIncompleteCompletion,
		result: &llm.SummaryResult{
			Summary:      "半份结论",
			ModelUsed:    "deepseek-v4-flash",
			TokensUsed:   80,
			FinishReason: "length",
			Incomplete:   true,
		},
	}
	db, router := setupReportAPI(t, stub)
	job := seedReportJob(t, db, true)
	original := "原有效摘要"
	require.NoError(t, db.Create(&models.Report{
		JobID:         job.ID,
		Title:         "教育精选",
		Content:       workflow.ReplaceOrInsertLLMSummary("# 教育精选\n\n正文\n", original),
		PodcastsCount: 1,
		MatchedCount:  1,
		GeneratedAt:   time.Now(),
		LLMSummary:    original,
		LLMModelUsed:  "deepseek-v4-flash",
		LLMTokensUsed: 12,
	}).Error)

	confirm := fmt.Sprintf(`{"confirmation_text":"REGENERATE LLM %d"}`, job.ID)
	post := func() *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/jobs/%d/regenerate-llm", job.ID), bytes.NewBufferString(confirm))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(rec, req)
		return rec
	}

	first := post()
	require.Equal(t, http.StatusInternalServerError, first.Code)
	afterFirst := getReportJSON(t, router, job.ID)
	require.Equal(t, original, afterFirst["llm_summary"])
	require.Contains(t, afterFirst["content"], original)
	require.NotContains(t, afterFirst["content"], "半份结论")
	stats := afterFirst["report_stats"].(map[string]any)
	require.Equal(t, models.AIStatusGenerated, stats["ai_status"])
	require.Contains(t, afterFirst["llm_error"], models.LLMErrorRegeneratePrefix)

	second := post()
	require.Equal(t, http.StatusInternalServerError, second.Code)
	afterSecond := getReportJSON(t, router, job.ID)
	require.Equal(t, original, afterSecond["llm_summary"])
	require.Contains(t, afterSecond["content"], original)
	require.NotContains(t, afterSecond["content"], "半份结论")
	require.Equal(t, 1, strings.Count(afterSecond["content"].(string), "## 🤖 AI智能摘要"))
	stats = afterSecond["report_stats"].(map[string]any)
	require.Equal(t, models.AIStatusGenerated, stats["ai_status"])
}

func TestRegenerateLLMSummaryRequiresConfirmation(t *testing.T) {
	db, router := setupReportAPI(t, &reportSummarizerStub{})
	job := seedReportJob(t, db, true)
	require.NoError(t, db.Create(&models.Report{
		JobID: job.ID, Title: "教育精选", Content: "# x", GeneratedAt: time.Now(),
	}).Error)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/jobs/%d/regenerate-llm", job.ID), bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusPreconditionRequired, recorder.Code)
}

func TestRegenerateIncompleteThenFailureDoesNotPromoteSummary(t *testing.T) {
	stub := &reportSummarizerStub{err: fmt.Errorf("network failed")}
	db, router := setupReportAPI(t, stub)
	job := seedReportJob(t, db, true)
	require.NoError(t, db.Create(&models.Report{JobID: job.ID, Title: "报告", Content: "# 报告", PodcastsCount: 1, MatchedCount: 1, LLMSummary: "半份", LLMError: models.LLMErrorIncompletePrefix + ": length"}).Error)
	confirm := fmt.Sprintf(`{"confirmation_text":"REGENERATE LLM %d"}`, job.ID)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/jobs/%d/regenerate-llm", job.ID), bytes.NewBufferString(confirm))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusInternalServerError, rec.Code)
	body := getReportJSON(t, router, job.ID)
	require.Equal(t, models.AIStatusIncomplete, body["report_stats"].(map[string]any)["ai_status"])
	require.Equal(t, "半份", body["llm_summary"])
}
