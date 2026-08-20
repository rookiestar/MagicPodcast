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
	gotCtx    context.Context
	showNotes []string
}

type ctxKey struct{}

func (s *stubSummarizer) GenerateForReport(ctx context.Context, data []llm.EpisodeReportData, workflowName string, userPrompt string, options llm.SummaryOptions) (*llm.SummaryResult, error) {
	s.gotCtx = ctx
	for _, podcast := range data {
		for _, episode := range podcast.Episodes {
			s.showNotes = append(s.showNotes, episode.ShowNotes)
		}
	}
	if s.err != nil {
		return nil, s.err
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
}
