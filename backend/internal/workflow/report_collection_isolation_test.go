package workflow

import (
	"context"
	"testing"
	"time"

	"magicpodcast/internal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// 清单独有、尚未被普通同步识别的单集不进入日报候选；父节目已关注也一样（#378）。
func TestReportGeneratorExcludesCollectionOnlyEpisodes(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:report_collection_isolation?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&models.Workflow{},
		&models.Job{},
		&models.JobExecution{},
		&models.Report{},
		&models.Podcast{},
		&models.Episode{},
	))

	podcast := models.Podcast{
		Title: "已关注节目", XYZID: "report-pid",
		FeedURL: "https://example.com/feed.xml", IsSubscribed: true,
	}
	require.NoError(t, db.Create(&podcast).Error)

	now := time.Now().UTC().Truncate(time.Second)
	inWindow := now.Add(-30 * time.Second)
	synced := models.Episode{
		PodcastID:     podcast.ID,
		Title:         "普通同步单集",
		GUID:          "guid-synced",
		PublishedDate: inWindow,
	}
	require.NoError(t, db.Create(&synced).Error)
	collectionOnly := models.Episode{
		PodcastID:      podcast.ID,
		Title:          "清单独有单集",
		GUID:           "xiaoyuzhoufm:episode:report-isolated",
		PublishedDate:  inWindow,
		CollectionOnly: true,
	}
	require.NoError(t, db.Create(&collectionOnly).Error)

	workflow := models.Workflow{
		Name: "隔离日报", Schedule: "0 0 8 * * *",
		ScopeType: models.ScopeTypeAllSubscribed, IsEnabled: true,
		PublishToHomepage: true, ReportType: "daily",
		RulesConfig: models.RulesConfig{TimeRange: 1},
	}
	require.NoError(t, db.Create(&workflow).Error)
	start := now.Add(-time.Minute)
	end := now
	job := models.Job{
		WorkflowID: workflow.ID, Status: models.JobStatusCompleted,
		TriggeredBy: "manual", StartTime: &start, EndTime: &end,
	}
	require.NoError(t, db.Create(&job).Error)
	require.NoError(t, db.Create(&models.JobExecution{
		JobID: job.ID, PodcastID: &podcast.ID, PodcastTitle: podcast.Title,
		PodcastFeedURL: podcast.FeedURL, Status: models.ExecutionStatusSuccess,
	}).Error)

	rg := NewReportGenerator(db, nil)
	report, err := rg.GenerateForJob(context.Background(), &job)
	require.NoError(t, err)
	require.NotNil(t, report)

	titles := make([]string, 0, len(report.StructuredEpisodes))
	for _, episode := range report.StructuredEpisodes {
		titles = append(titles, episode.EpisodeTitle)
	}
	assert.Contains(t, titles, "普通同步单集", "正常同步单集保留既有报告行为")
	assert.NotContains(t, titles, "清单独有单集", "清单独有单集不进入日报")
	assert.NotContains(t, report.Content, "清单独有单集")
}
