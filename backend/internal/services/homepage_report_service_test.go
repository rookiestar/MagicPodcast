package services

import (
	"testing"
	"time"

	"magicpodcast/internal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func openHomepageReportTestDB(t *testing.T, name string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+name+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&models.Workflow{},
		&models.Job{},
		&models.Report{},
		&models.Podcast{},
		&models.Episode{},
		&models.EpisodeTriageDecision{},
	))
	return db
}

func seedLiveEpisode(t *testing.T, db *gorm.DB, title string) models.Episode {
	t.Helper()
	podcast := models.Podcast{
		Title: "P-" + title, Author: "A", FeedURL: "https://example.com/" + title + ".xml",
		XYZID: "xyz-" + title, IsSubscribed: true,
	}
	require.NoError(t, db.Create(&podcast).Error)
	episode := models.Episode{
		PodcastID: podcast.ID, GUID: "guid-" + title, Title: title,
		PublishedDate: time.Now().UTC(),
	}
	require.NoError(t, db.Create(&episode).Error)
	return episode
}

func seedPublishedReport(
	t *testing.T,
	db *gorm.DB,
	workflowName string,
	reportType string,
	publish bool,
	jobStatus models.JobStatus,
	completedAt time.Time,
	episodes models.ReportEpisodeList,
	schedule ...string,
) models.Report {
	t.Helper()
	cronSchedule := "0 0 8 * * *"
	if len(schedule) > 0 && schedule[0] != "" {
		cronSchedule = schedule[0]
	}
	workflow := models.Workflow{
		Name:              workflowName,
		Schedule:          cronSchedule,
		ScopeType:         models.ScopeTypeAllSubscribed,
		IsEnabled:         true,
		PublishToHomepage: publish,
		ReportType:        reportType,
	}
	require.NoError(t, db.Create(&workflow).Error)

	end := completedAt
	job := models.Job{
		WorkflowID:  workflow.ID,
		Status:      jobStatus,
		TriggeredBy: "cron",
		EndTime:     &end,
	}
	require.NoError(t, db.Create(&job).Error)

	report := models.Report{
		JobID:              job.ID,
		Title:              workflowName + " report",
		Content:            "# body\n\nfull markdown body for " + workflowName,
		Summary:            "summary",
		GeneratedAt:        completedAt,
		PublishToHomepage:  publish,
		ReportType:         reportType,
		WorkflowName:       workflowName,
		StructuredEpisodes: episodes,
		MatchedCount:       len(episodes),
		EpisodesCount:      len(episodes),
	}
	require.NoError(t, db.Create(&report).Error)
	return report
}

func TestHomepageReportService_IncludesPartialJobsAndIgnoresLegacyPublishFlag(t *testing.T) {
	loc := time.FixedZone("CST", 8*3600)
	db := openHomepageReportTestDB(t, "homepage_completed_only")
	svc := NewHomepageReportServiceWithLocation(db, loc)
	svc.now = func() time.Time {
		return time.Date(2026, 8, 10, 15, 0, 0, 0, loc)
	}

	live := seedLiveEpisode(t, db, "live-ep")
	todayNoon := time.Date(2026, 8, 10, 12, 0, 0, 0, loc).UTC()

	seedPublishedReport(t, db, "完成日报", "daily", true, models.JobStatusCompleted, todayNoon, models.ReportEpisodeList{
		{EpisodeID: live.ID, Order: 1, EpisodeTitle: live.Title, PodcastTitle: "P", Recommendation: "", Context: "节目上下文"},
	})
	seedPublishedReport(t, db, "部分日报", "daily", false, models.JobStatusPartial, todayNoon, models.ReportEpisodeList{
		{EpisodeID: live.ID, Order: 1, EpisodeTitle: live.Title, PodcastTitle: "P"},
	})
	seedPublishedReport(t, db, "失败日报", "daily", true, models.JobStatusFailed, todayNoon, models.ReportEpisodeList{
		{EpisodeID: live.ID, Order: 1, EpisodeTitle: live.Title, PodcastTitle: "P"},
	})

	today, err := svc.ListToday()
	require.NoError(t, err)
	require.Len(t, today, 2)
	assert.Equal(t, "部分日报", today[0].WorkflowName)
	assert.Equal(t, "完成日报", today[1].WorkflowName)
	assert.NotEmpty(t, today[0].Content)
}

func TestHomepageReportService_ExcludesMissingAndDeletedEpisodes(t *testing.T) {
	loc := time.UTC
	db := openHomepageReportTestDB(t, "homepage_live_eps")
	svc := NewHomepageReportServiceWithLocation(db, loc)
	svc.now = func() time.Time { return time.Date(2026, 8, 10, 12, 0, 0, 0, loc) }

	live := seedLiveEpisode(t, db, "kept")
	deleted := seedLiveEpisode(t, db, "deleted")
	require.NoError(t, db.Delete(&deleted).Error)

	seedPublishedReport(t, db, "混合单集", "daily", true, models.JobStatusCompleted,
		time.Date(2026, 8, 10, 10, 0, 0, 0, loc),
		models.ReportEpisodeList{
			{EpisodeID: live.ID, Order: 1, EpisodeTitle: "kept", PodcastTitle: "P", Context: "ctx"},
			{EpisodeID: deleted.ID, Order: 2, EpisodeTitle: "deleted", PodcastTitle: "P"},
			{EpisodeID: 99999, Order: 3, EpisodeTitle: "ghost", PodcastTitle: "P"},
		},
	)
	// All invalid => not listed.
	seedPublishedReport(t, db, "全无效", "daily", true, models.JobStatusCompleted,
		time.Date(2026, 8, 10, 11, 0, 0, 0, loc),
		models.ReportEpisodeList{
			{EpisodeID: 0, Order: 1, EpisodeTitle: "zero"},
			{EpisodeID: 88888, Order: 2, EpisodeTitle: "missing"},
		},
	)

	today, err := svc.ListToday()
	require.NoError(t, err)
	require.Len(t, today, 1)
	assert.Equal(t, "混合单集", today[0].WorkflowName)
	require.Len(t, today[0].Episodes, 1)
	assert.Equal(t, live.ID, today[0].Episodes[0].EpisodeID)
	assert.Equal(t, "ctx", today[0].Episodes[0].Context)
	assert.Empty(t, today[0].Episodes[0].Recommendation)
}

func TestHomepageReportService_PreservesRejectedLinksForSharedPlanner(t *testing.T) {
	loc := time.UTC
	db := openHomepageReportTestDB(t, "homepage_links")
	svc := NewHomepageReportServiceWithLocation(db, loc)
	svc.now = func() time.Time { return time.Date(2026, 8, 10, 12, 0, 0, 0, loc) }
	live := seedLiveEpisode(t, db, "link-ep")

	seedPublishedReport(t, db, "链接日报", "daily", true, models.JobStatusCompleted,
		time.Date(2026, 8, 10, 10, 0, 0, 0, loc),
		models.ReportEpisodeList{
			{
				EpisodeID: live.ID, Order: 1, EpisodeTitle: "t", PodcastTitle: "P",
				Link: "javascript:alert(1)", Context: "ctx",
			},
		},
	)

	today, err := svc.ListToday()
	require.NoError(t, err)
	require.Len(t, today, 1)
	assert.Equal(t, "javascript:alert(1)", today[0].Episodes[0].Link)
}

func TestHomepageReportService_HistoryKeepsPublishSnapshotAndMetadataOnly(t *testing.T) {
	loc := time.FixedZone("CST", 8*3600)
	db := openHomepageReportTestDB(t, "homepage_history_snapshot")
	svc := NewHomepageReportServiceWithLocation(db, loc)
	svc.now = func() time.Time {
		return time.Date(2026, 8, 10, 15, 0, 0, 0, loc)
	}

	live := seedLiveEpisode(t, db, "hist-ep")
	yesterday := time.Date(2026, 8, 9, 12, 0, 0, 0, loc).UTC()

	// Snapshot published even if workflow later disables publish.
	report := seedPublishedReport(t, db, "往期快照", "daily", true, models.JobStatusCompleted, yesterday, models.ReportEpisodeList{
		{EpisodeID: live.ID, Order: 1, EpisodeTitle: "hist", PodcastTitle: "P", Context: "往期上下文"},
	}, "0 0 8 * * 1")
	require.NoError(t, db.Model(&models.Workflow{}).Where("name = ?", "往期快照").
		Updates(map[string]interface{}{"publish_to_homepage": false, "report_type": ""}).Error)

	history, err := svc.ListHistory(10)
	require.NoError(t, err)
	require.Len(t, history, 1)
	assert.Equal(t, "往期快照", history[0].WorkflowName)
	assert.Equal(t, "weekly", history[0].ReportType)
	assert.True(t, history[0].MetadataOnly)
	assert.Empty(t, history[0].Content, "history list must not ship full markdown bodies")
	assert.NotEmpty(t, history[0].Episodes)

	full, err := svc.GetPublishedReport(report.ID)
	require.NoError(t, err)
	require.NotNil(t, full)
	assert.Equal(t, "weekly", full.ReportType)
	assert.False(t, full.MetadataOnly)
	assert.Contains(t, full.Content, "full markdown")
}

func TestHomepageReportService_ZeroAndOneAndTwoEpisodes(t *testing.T) {
	loc := time.UTC
	db := openHomepageReportTestDB(t, "homepage_012")
	svc := NewHomepageReportServiceWithLocation(db, loc)
	svc.now = func() time.Time { return time.Date(2026, 8, 10, 12, 0, 0, 0, loc) }
	now := time.Date(2026, 8, 10, 10, 0, 0, 0, loc)

	e1 := seedLiveEpisode(t, db, "one")
	e2 := seedLiveEpisode(t, db, "two")

	seedPublishedReport(t, db, "零条", "daily", true, models.JobStatusCompleted, now, nil)
	seedPublishedReport(t, db, "一条", "daily", true, models.JobStatusCompleted, now.Add(time.Hour), models.ReportEpisodeList{
		{EpisodeID: e1.ID, Order: 1, EpisodeTitle: "one", PodcastTitle: "P"},
	})
	seedPublishedReport(t, db, "两条", "weekly", true, models.JobStatusCompleted, now.Add(2*time.Hour), models.ReportEpisodeList{
		{EpisodeID: e1.ID, Order: 1, EpisodeTitle: "one", PodcastTitle: "P"},
		{EpisodeID: e2.ID, Order: 2, EpisodeTitle: "two", PodcastTitle: "P"},
	}, "0 0 8 * * 1")

	today, err := svc.ListToday()
	require.NoError(t, err)
	require.Len(t, today, 2)
	assert.Equal(t, "两条", today[0].WorkflowName)
	assert.Equal(t, "weekly", today[0].ReportType)
	assert.Equal(t, 2, today[0].EpisodeCount)
	assert.Equal(t, "一条", today[1].WorkflowName)
	assert.Equal(t, 1, today[1].EpisodeCount)
}

func TestHomepageReportService_InfersCustomReportTypeFromCron(t *testing.T) {
	db := openHomepageReportTestDB(t, "homepage_report_types")
	svc := NewHomepageReportServiceWithLocation(db, time.UTC)
	svc.now = func() time.Time { return time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC) }
	episode := seedLiveEpisode(t, db, "cron-type")
	now := time.Date(2026, 8, 10, 10, 0, 0, 0, time.UTC)

	seedPublishedReport(t, db, "日报", "weekly", false, models.JobStatusCompleted, now, models.ReportEpisodeList{
		{EpisodeID: episode.ID, Order: 1, EpisodeTitle: episode.Title, PodcastTitle: "P"},
	}, "0 8 * * *")
	seedPublishedReport(t, db, "周报", "daily", false, models.JobStatusCompleted, now.Add(time.Minute), models.ReportEpisodeList{
		{EpisodeID: episode.ID, Order: 1, EpisodeTitle: episode.Title, PodcastTitle: "P"},
	}, "0 0 8 * * 5")
	seedPublishedReport(t, db, "自定义", "daily", false, models.JobStatusCompleted, now.Add(2*time.Minute), models.ReportEpisodeList{
		{EpisodeID: episode.ID, Order: 1, EpisodeTitle: episode.Title, PodcastTitle: "P"},
	}, "0 0 8 1 * *")

	today, err := svc.ListToday()
	require.NoError(t, err)
	require.Len(t, today, 3)
	got := make(map[string]string, len(today))
	for _, report := range today {
		got[report.WorkflowName] = report.ReportType
	}
	assert.Equal(t, "daily", got["日报"])
	assert.Equal(t, "weekly", got["周报"])
	assert.Equal(t, "custom", got["自定义"])
}

func TestHomepageReportService_HistoryLimitCountsDisplayableReports(t *testing.T) {
	loc := time.UTC
	db := openHomepageReportTestDB(t, "homepage_history_limit")
	svc := NewHomepageReportServiceWithLocation(db, loc)
	svc.now = func() time.Time { return time.Date(2026, 8, 10, 12, 0, 0, 0, loc) }
	live := seedLiveEpisode(t, db, "history-limit")
	yesterday := time.Date(2026, 8, 9, 10, 0, 0, 0, loc)

	seedPublishedReport(t, db, "旧的有效报告", "daily", false, models.JobStatusCompleted, yesterday, models.ReportEpisodeList{
		{EpisodeID: live.ID, Order: 1, EpisodeTitle: live.Title, PodcastTitle: "P"},
	})
	seedPublishedReport(t, db, "更新的空报告", "daily", false, models.JobStatusCompleted, yesterday.Add(time.Hour), nil)

	history, err := svc.ListHistory(1)
	require.NoError(t, err)
	require.Len(t, history, 1)
	assert.Equal(t, "旧的有效报告", history[0].WorkflowName)
}

func TestHomepageReportService_ThemeUsesReportSummaryThenEpisodeFallback(t *testing.T) {
	loc := time.UTC
	db := openHomepageReportTestDB(t, "homepage_theme")
	svc := NewHomepageReportServiceWithLocation(db, loc)
	svc.now = func() time.Time { return time.Date(2026, 8, 10, 12, 0, 0, 0, loc) }
	now := time.Date(2026, 8, 10, 10, 0, 0, 0, loc)
	episode := seedLiveEpisode(t, db, "fallback-topic")

	withSummary := seedPublishedReport(t, db, "AI 日报", "daily", true, models.JobStatusCompleted, now, models.ReportEpisodeList{
		{EpisodeID: episode.ID, Order: 1, EpisodeTitle: "单集标题", PodcastTitle: "P"},
	})
	require.NoError(t, db.Model(&withSummary).Update(
		"llm_summary",
		"## AI 组织变革进入落地期\n\n更长的摘要正文。",
	).Error)

	seedPublishedReport(t, db, "基础日报", "daily", true, models.JobStatusCompleted, now.Add(-time.Hour), models.ReportEpisodeList{
		{EpisodeID: episode.ID, Order: 1, EpisodeTitle: "首条精选主题", PodcastTitle: "P"},
	})

	today, err := svc.ListToday()
	require.NoError(t, err)
	require.Len(t, today, 2)
	assert.Equal(t, "AI 组织变革进入落地期", today[0].Theme)
	assert.Equal(t, "首条精选主题", today[1].Theme)
}
