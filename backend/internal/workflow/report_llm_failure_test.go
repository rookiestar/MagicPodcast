package workflow

import (
	"context"
	"fmt"
	"testing"
	"time"

	"magicpodcast/internal/llm"
	"magicpodcast/internal/models"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type stubSummarizer struct {
	err       error
	result    *llm.SummaryResult
	gotCtx    context.Context
	showNotes []string
	calls     int
}

type ctxKey struct{}

func (s *stubSummarizer) GenerateForReport(ctx context.Context, data []llm.EpisodeReportData, workflowName string, userPrompt string, options llm.SummaryOptions) (*llm.SummaryResult, error) {
	s.gotCtx = ctx
	s.calls++
	for _, podcast := range data {
		for _, episode := range podcast.Episodes {
			s.showNotes = append(s.showNotes, episode.ShowNotes)
		}
	}
	if s.result != nil || s.err != nil {
		return s.result, s.err
	}
	return &llm.SummaryResult{Summary: "ok", ModelUsed: "deepseek-v4-flash", TokensUsed: 12}, nil
}

func TestFinalizeJobLLMFailureKeepsCompletedZeroErrorCount(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:llm_fail_job_%d?mode=memory&cache=shared", time.Now().UnixNano())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&models.Workflow{},
		&models.Job{},
		&models.JobExecution{},
		&models.Report{},
		&models.Podcast{},
		&models.Episode{},
	))

	const fullNotes = "这是一段完整的节目录音笔记，用于确认摘要不会在进模型前截断。ABCDEF123456"
	workflow := models.Workflow{
		Name:        "教育精选",
		ScopeType:   models.ScopeTypeAllSubscribed,
		IsEnabled:   true,
		RulesConfig: models.RulesConfig{TimeRange: 1, LLMEnabled: true},
	}
	require.NoError(t, db.Create(&workflow).Error)

	podcast := models.Podcast{
		Title:        "测试节目",
		Author:       "主播",
		FeedURL:      "https://example.com/feed.xml",
		XYZID:        "llm-fail-pod",
		IsSubscribed: true,
	}
	require.NoError(t, db.Create(&podcast).Error)

	now := time.Now().UTC().Truncate(time.Second)
	start := now.Add(-time.Minute)
	require.NoError(t, db.Create(&models.Episode{
		PodcastID:     podcast.ID,
		GUID:          "llm-fail-ep",
		Title:         "本周单集",
		ShowNotes:     fullNotes,
		PublishedDate: start.Add(-time.Hour),
	}).Error)
	job := models.Job{
		WorkflowID:        workflow.ID,
		Status:            models.JobStatusRunning,
		TriggeredBy:       "cron",
		StartTime:         &start,
		PodcastsProcessed: 1,
	}
	require.NoError(t, db.Create(&job).Error)
	exec := models.JobExecution{
		JobID:           job.ID,
		PodcastID:       &podcast.ID,
		PodcastTitle:    podcast.Title,
		PodcastFeedURL:  podcast.FeedURL,
		Status:          models.ExecutionStatusSuccess,
		EpisodesFound:   1,
		EpisodesCreated: 1,
		EpisodesMatched: 1,
	}
	require.NoError(t, db.Create(&exec).Error)

	summarizer := &stubSummarizer{
		err: fmt.Errorf("读取响应失败: context deadline exceeded (Client.Timeout or context cancellation while reading body)"),
	}
	executor := NewExecutor(db, nil, nil, summarizer)
	ctx := context.WithValue(context.Background(), ctxKey{}, "job-ctx")
	executor.finalizeJob(ctx, &job, []*models.JobExecution{&exec})

	require.Equal(t, models.JobStatusCompleted, job.Status)
	require.Equal(t, 0, job.ErrorCount)
	require.NotNil(t, summarizer.gotCtx)
	require.Equal(t, "job-ctx", summarizer.gotCtx.Value(ctxKey{}))
	require.Len(t, summarizer.showNotes, 1)
	require.Contains(t, summarizer.showNotes[0], fullNotes)

	var report models.Report
	require.NoError(t, db.Where("job_id = ?", job.ID).First(&report).Error)
	require.Contains(t, report.LLMError, "读取响应失败")
	require.Empty(t, report.LLMSummary)
	require.Zero(t, report.LLMTokensUsed)
	require.Empty(t, report.LLMModelUsed)
	require.Contains(t, report.Content, "本周单集")
	require.Equal(t, models.AIStatusNotGenerated, report.DeriveAIStatus())
}

func seedLLMJob(t *testing.T, db *gorm.DB, llmEnabled bool) (*models.Job, *models.JobExecution) {
	t.Helper()
	workflow := models.Workflow{
		Name:        "教育精选",
		ScopeType:   models.ScopeTypeAllSubscribed,
		IsEnabled:   true,
		RulesConfig: models.RulesConfig{TimeRange: 1, LLMEnabled: llmEnabled},
	}
	require.NoError(t, db.Create(&workflow).Error)
	podcast := models.Podcast{
		Title:        "测试节目",
		Author:       "主播",
		FeedURL:      "https://example.com/feed.xml",
		XYZID:        fmt.Sprintf("pod-%d", time.Now().UnixNano()),
		IsSubscribed: true,
	}
	require.NoError(t, db.Create(&podcast).Error)
	now := time.Now().UTC().Truncate(time.Second)
	start := now.Add(-time.Minute)
	require.NoError(t, db.Create(&models.Episode{
		PodcastID:     podcast.ID,
		GUID:          fmt.Sprintf("ep-%d", time.Now().UnixNano()),
		Title:         "本周单集",
		ShowNotes:     "完整笔记",
		PublishedDate: start.Add(-time.Hour),
	}).Error)
	job := models.Job{
		WorkflowID:        workflow.ID,
		Status:            models.JobStatusRunning,
		TriggeredBy:       "cron",
		StartTime:         &start,
		PodcastsProcessed: 1,
	}
	require.NoError(t, db.Create(&job).Error)
	exec := models.JobExecution{
		JobID:           job.ID,
		PodcastID:       &podcast.ID,
		PodcastTitle:    podcast.Title,
		PodcastFeedURL:  podcast.FeedURL,
		Status:          models.ExecutionStatusSuccess,
		EpisodesFound:   1,
		EpisodesCreated: 1,
		EpisodesMatched: 1,
	}
	require.NoError(t, db.Create(&exec).Error)
	return &job, &exec
}

func TestGenerateForJobLLMOutcomes(t *testing.T) {
	cases := []struct {
		name       string
		enabled    bool
		summarizer *stubSummarizer
		status     string
		summary    string
		hasError   bool
	}{
		{
			name:       "success",
			enabled:    true,
			summarizer: &stubSummarizer{result: &llm.SummaryResult{Summary: "可读摘要", ModelUsed: "deepseek-v4-flash", TokensUsed: 12}},
			status:     models.AIStatusGenerated,
			summary:    "可读摘要",
		},
		{
			name:    "empty body with tokens",
			enabled: true,
			summarizer: &stubSummarizer{
				err:    llm.ErrEmptyCompletion,
				result: &llm.SummaryResult{Summary: "", ModelUsed: "deepseek-v4-flash", TokensUsed: 7245},
			},
			status:   models.AIStatusNotGenerated,
			hasError: true,
		},
		{
			name:    "truncated",
			enabled: true,
			summarizer: &stubSummarizer{
				err: llm.ErrIncompleteCompletion,
				result: &llm.SummaryResult{
					Summary: "半份结论", ModelUsed: "deepseek-v4-flash", TokensUsed: 120,
					FinishReason: "length", Incomplete: true,
				},
			},
			status:   models.AIStatusIncomplete,
			summary:  "半份结论",
			hasError: true,
		},
		{
			name:       "call failure",
			enabled:    true,
			summarizer: &stubSummarizer{err: fmt.Errorf("读取响应失败: timeout")},
			status:     models.AIStatusNotGenerated,
			hasError:   true,
		},
		{
			name:    "disabled",
			enabled: false,
			status:  models.AIStatusDisabled,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:llm_outcome_%s_%d?mode=memory&cache=shared", tc.name, time.Now().UnixNano())), &gorm.Config{})
			require.NoError(t, err)
			require.NoError(t, db.AutoMigrate(
				&models.Workflow{}, &models.Job{}, &models.JobExecution{},
				&models.Report{}, &models.Podcast{}, &models.Episode{},
			))
			job, exec := seedLLMJob(t, db, tc.enabled)
			var summarizer SummarizerInterface
			if tc.summarizer != nil {
				summarizer = tc.summarizer
			}
			executor := NewExecutor(db, nil, nil, summarizer)
			executor.finalizeJob(context.Background(), job, []*models.JobExecution{exec})
			require.Equal(t, models.JobStatusCompleted, job.Status)
			require.Equal(t, 0, job.ErrorCount)

			var report models.Report
			require.NoError(t, db.Where("job_id = ?", job.ID).First(&report).Error)
			require.Contains(t, report.Content, "本周单集")
			require.Equal(t, tc.status, report.DeriveAIStatus())
			require.Equal(t, tc.summary, report.LLMSummary)
			if tc.hasError {
				require.NotEmpty(t, report.LLMError)
			}
			if tc.status == models.AIStatusGenerated {
				require.NotContains(t, report.Content, "## 🤖 AI智能摘要\n\n##")
				require.Contains(t, report.Content, "可读摘要")
			}
		})
	}
}

func TestGenerateForJobNoEpisodesIsNotNeeded(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:llm_none_%d?mode=memory&cache=shared", time.Now().UnixNano())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&models.Workflow{}, &models.Job{}, &models.JobExecution{},
		&models.Report{}, &models.Podcast{}, &models.Episode{},
	))
	workflow := models.Workflow{
		Name:        "空报告",
		ScopeType:   models.ScopeTypeAllSubscribed,
		IsEnabled:   true,
		RulesConfig: models.RulesConfig{TimeRange: 1, LLMEnabled: true},
	}
	require.NoError(t, db.Create(&workflow).Error)
	now := time.Now().UTC()
	job := models.Job{WorkflowID: workflow.ID, Status: models.JobStatusRunning, TriggeredBy: "manual", StartTime: &now}
	require.NoError(t, db.Create(&job).Error)
	rg := NewReportGenerator(db, &stubSummarizer{})
	report, err := rg.GenerateForJob(context.Background(), &job)
	require.NoError(t, err)
	require.Equal(t, models.AIStatusNotNeeded, report.DeriveAIStatus())
	require.Empty(t, report.LLMSummary)
	require.Empty(t, report.LLMError)
}
