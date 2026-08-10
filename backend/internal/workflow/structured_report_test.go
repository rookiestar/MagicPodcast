package workflow

import (
	"testing"
	"time"

	"magicpodcast/internal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestBuildStructuredEpisodes_UsesRealEpisodeIDsAndOrder(t *testing.T) {
	rg := &ReportGenerator{}
	published := time.Date(2026, 8, 10, 8, 0, 0, 0, time.UTC)
	data := []EpisodeReportData{
		{
			PodcastID:       3,
			PodcastTitle:    "节目甲",
			PodcastCoverURL: "https://example.com/a.jpg",
			Episodes: []EpisodeDetail{
				{EpisodeID: 101, Title: "第一集", ShowNotes: "推荐理由 A", PublishedDate: published, Duration: 1200},
				{EpisodeID: 0, Title: "应被丢弃", ShowNotes: "no id"},
				{EpisodeID: 102, Title: "第二集", ShowNotes: "推荐理由 B", PublishedDate: published.Add(time.Hour), Duration: 600, ImageURL: "https://example.com/ep.jpg"},
			},
		},
	}

	structured := rg.buildStructuredEpisodes(data)
	require.Len(t, structured, 2)
	assert.Equal(t, 1, structured[0].Order)
	assert.Equal(t, uint(101), structured[0].EpisodeID)
	assert.Equal(t, "节目甲", structured[0].PodcastTitle)
	assert.Equal(t, "https://example.com/a.jpg", structured[0].PodcastCoverURL)
	// Show Notes become Context only — never Recommendation (#93).
	assert.Contains(t, structured[0].Context, "推荐理由 A")
	assert.Empty(t, structured[0].Recommendation)
	assert.Equal(t, 2, structured[1].Order)
	assert.Equal(t, uint(102), structured[1].EpisodeID)
	assert.Equal(t, "https://example.com/ep.jpg", structured[1].ImageURL)
}

func TestSanitizeHomepageEpisodeLink(t *testing.T) {
	assert.Equal(t, "https://ok.example/x", sanitizeHomepageEpisodeLink("https://ok.example/x"))
	assert.Equal(t, "", sanitizeHomepageEpisodeLink("javascript:alert(1)"))
	assert.Equal(t, "", sanitizeHomepageEpisodeLink("data:text/html,x"))
}

func TestGenerateForJob_PersistsStructuredHomepageSnapshot(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:structured_homepage_report?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&models.Workflow{},
		&models.Job{},
		&models.JobExecution{},
		&models.Report{},
		&models.Podcast{},
		&models.Episode{},
	))

	workflow := models.Workflow{
		Name:              "首页日报",
		Schedule:          "0 0 8 * * *",
		ScopeType:         models.ScopeTypeAllSubscribed,
		IsEnabled:         true,
		PublishToHomepage: true,
		ReportType:        "daily",
		RulesConfig:       models.RulesConfig{TimeRange: 1},
	}
	require.NoError(t, db.Create(&workflow).Error)

	podcast := models.Podcast{
		Title:        "测试节目",
		Author:       "主播",
		FeedURL:      "https://example.com/feed.xml",
		XYZID:        "structured-podcast",
		CoverURL:     "https://example.com/cover.jpg",
		IsSubscribed: true,
	}
	require.NoError(t, db.Create(&podcast).Error)

	now := time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC)
	episode := models.Episode{
		PodcastID:     podcast.ID,
		GUID:          "structured-ep-1",
		Title:         "结构化单集",
		ShowNotes:     "<p>真实节目上下文</p>",
		PublishedDate: now.Add(-2 * time.Hour),
		Duration:      1800,
	}
	require.NoError(t, db.Create(&episode).Error)

	start := now.Add(-time.Minute)
	end := now
	job := models.Job{
		WorkflowID:        workflow.ID,
		Status:            models.JobStatusCompleted,
		TriggeredBy:       "manual",
		StartTime:         &start,
		EndTime:           &end,
		PodcastsProcessed: 1,
	}
	require.NoError(t, db.Create(&job).Error)
	require.NoError(t, db.Create(&models.JobExecution{
		JobID:          job.ID,
		PodcastID:      &podcast.ID,
		PodcastTitle:   podcast.Title,
		PodcastFeedURL: podcast.FeedURL,
		Status:         models.ExecutionStatusSuccess,
	}).Error)

	rg := NewReportGenerator(db, nil)
	report, err := rg.GenerateForJob(&job)
	require.NoError(t, err)
	require.NotNil(t, report)
	assert.True(t, report.PublishToHomepage)
	assert.Equal(t, "daily", report.ReportType)
	assert.Equal(t, "首页日报", report.WorkflowName)
	require.Len(t, report.StructuredEpisodes, 1)
	assert.Equal(t, episode.ID, report.StructuredEpisodes[0].EpisodeID)
	assert.Equal(t, 1, report.StructuredEpisodes[0].Order)
	assert.Equal(t, "结构化单集", report.StructuredEpisodes[0].EpisodeTitle)
	assert.NotEmpty(t, report.Content)
	assert.Contains(t, report.Content, "结构化单集")
}

func TestGenerateForJob_ZeroEpisodesDoesNotPublishHomepage(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:structured_homepage_zero?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&models.Workflow{},
		&models.Job{},
		&models.JobExecution{},
		&models.Report{},
		&models.Podcast{},
		&models.Episode{},
	))

	workflow := models.Workflow{
		Name:              "空日报",
		Schedule:          "0 0 8 * * *",
		ScopeType:         models.ScopeTypeAllSubscribed,
		IsEnabled:         true,
		PublishToHomepage: true,
		ReportType:        "daily",
		RulesConfig:       models.RulesConfig{TimeRange: 1},
	}
	require.NoError(t, db.Create(&workflow).Error)

	now := time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC)
	start := now
	job := models.Job{
		WorkflowID:  workflow.ID,
		Status:      models.JobStatusCompleted,
		TriggeredBy: "manual",
		StartTime:   &start,
		EndTime:     &start,
	}
	require.NoError(t, db.Create(&job).Error)

	rg := NewReportGenerator(db, nil)
	report, err := rg.GenerateForJob(&job)
	require.NoError(t, err)
	assert.False(t, report.PublishToHomepage)
	assert.Empty(t, report.ReportType)
	assert.Empty(t, report.StructuredEpisodes)
	assert.NotEmpty(t, report.Content) // full body still saved
}

func TestGenerateForJob_UnpublishedWorkflowNeverHomepage(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:structured_homepage_off?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&models.Workflow{},
		&models.Job{},
		&models.JobExecution{},
		&models.Report{},
		&models.Podcast{},
		&models.Episode{},
	))

	workflow := models.Workflow{
		Name:              "未发布",
		Schedule:          "0 0 8 * * *",
		ScopeType:         models.ScopeTypeAllSubscribed,
		IsEnabled:         true,
		PublishToHomepage: false,
		RulesConfig:       models.RulesConfig{TimeRange: 1},
	}
	require.NoError(t, db.Create(&workflow).Error)

	podcast := models.Podcast{
		Title: "P", Author: "A", FeedURL: "https://example.com/u.xml", XYZID: "u-pod", IsSubscribed: true,
	}
	require.NoError(t, db.Create(&podcast).Error)
	now := time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC)
	episode := models.Episode{
		PodcastID: podcast.ID, GUID: "u-ep", Title: "E", PublishedDate: now.Add(-time.Hour),
	}
	require.NoError(t, db.Create(&episode).Error)

	start := now
	job := models.Job{
		WorkflowID: workflow.ID, Status: models.JobStatusCompleted, TriggeredBy: "manual",
		StartTime: &start, EndTime: &start, PodcastsProcessed: 1,
	}
	require.NoError(t, db.Create(&job).Error)
	require.NoError(t, db.Create(&models.JobExecution{
		JobID: job.ID, PodcastID: &podcast.ID, PodcastTitle: podcast.Title,
		PodcastFeedURL: podcast.FeedURL, Status: models.ExecutionStatusSuccess,
	}).Error)

	rg := NewReportGenerator(db, nil)
	report, err := rg.GenerateForJob(&job)
	require.NoError(t, err)
	assert.False(t, report.PublishToHomepage)
	// Structured data may still be stored for future use, but publish flag stays false.
	// Spec: unpublished workflows never enter homepage — listing filters on publish snapshot.
	assert.False(t, report.PublishToHomepage)
}
