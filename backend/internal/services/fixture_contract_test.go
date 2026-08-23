package services_test

import (
	"net/url"
	"testing"
	"time"

	"magicpodcast/internal/dataprofile"
	"magicpodcast/internal/models"
	"magicpodcast/internal/services"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestJourneyFixtureCoversDiscoveryConsumptionAndReportContracts(t *testing.T) {
	fixture, err := dataprofile.EnsureFixture(t.TempDir())
	require.NoError(t, err)
	db := openFixtureDatabase(t, fixture.DatabasePath)

	discovery := services.NewDiscoveryService(db)
	candidates, err := discovery.ListRecentCandidateSummaries(100)
	require.NoError(t, err)
	candidateIDs := make([]uint, 0, len(candidates))
	for _, candidate := range candidates {
		candidateIDs = append(candidateIDs, candidate.EpisodeID)
	}
	require.Contains(t, candidateIDs, uint(2013), "13-day candidate must remain visible")
	require.NotContains(t, candidateIDs, uint(2014), "15-day candidate must stay outside the 14-day window")

	consumption := services.NewConsumptionService(db)
	summary, err := consumption.QueueSummary()
	require.NoError(t, err)
	require.Equal(t, int64(2), summary.Counts[models.QueueStateInbox])
	require.Equal(t, int64(6), summary.Counts[models.QueueStateFocus])
	require.Equal(t, int64(2), summary.Counts[models.QueueStateSomeday])
	require.Equal(t, int64(1), summary.Counts[models.QueueStateDone])
	require.False(t, summary.FocusOverLimit)

	recent, err := consumption.ListQueue(models.QueueStateDone)
	require.NoError(t, err)
	require.Equal(t, []uint{2012}, fixtureConsumptionItemIDs(recent.Items))
	require.False(t, recent.HasMore)
	require.NotNil(t, recent.Items[0].CompletedAt)
	require.True(t, recent.Items[0].CompletedAt.Equal(fixture.AnchorAt.Add(-3*24*time.Hour)))

	focus, err := consumption.ListQueue(models.QueueStateFocus)
	require.NoError(t, err)
	attention := map[uint]string{}
	for _, item := range focus.Items {
		attention[item.EpisodeID] = item.Attention
	}
	require.Equal(t, services.AttentionNone, attention[2006])
	require.Equal(t, services.AttentionStale, attention[2007])
	require.Equal(t, services.AttentionStale, attention[2008])

	someday, err := consumption.ListQueue(models.QueueStateSomeday)
	require.NoError(t, err)
	attention = map[uint]string{}
	for _, item := range someday.Items {
		attention[item.EpisodeID] = item.Attention
	}
	require.Equal(t, services.AttentionStale, attention[2011])
	require.Equal(t, services.AttentionReview, attention[2009])

	location, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	reportService := services.NewHomepageReportServiceWithLocation(db, location)
	reports, err := reportService.ListTodayAndHistory(30)
	require.NoError(t, err)
	require.Len(t, reports.Today, 2)
	require.Len(t, reports.History, 1)
	require.True(t, reports.History[0].MetadataOnly)
	require.Empty(t, reports.History[0].Content)
	require.Equal(t, uint(6002), reports.Today[0].ID)
	require.Equal(t, uint(6001), reports.Today[1].ID)
	require.Empty(t, reports.Today[1].Episodes[1].Link, "dangerous report link must be stripped")

	var canonicalDecisions []models.EpisodeTriageDecision
	require.NoError(t, db.Where("episode_id IN ?", []uint{2002, 2015}).
		Order("episode_id").Find(&canonicalDecisions).Error)
	require.Len(t, canonicalDecisions, 1)
	require.Equal(t, uint(2002), canonicalDecisions[0].EpisodeID)
	require.Equal(t, models.QueueStateInbox, *canonicalDecisions[0].QueueState)
}

func fixtureConsumptionItemIDs(items []services.ConsumptionItem) []uint {
	ids := make([]uint, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.EpisodeID)
	}
	return ids
}

func TestFixtureReportScenariosExposeZeroOneAndMultipleTodayReports(t *testing.T) {
	tests := []struct {
		scenario string
		today    int
		history  int
	}{
		{dataprofile.FixtureScenarioReportEmpty, 0, 0},
		{dataprofile.FixtureScenarioReportSingle, 1, 1},
		{dataprofile.DefaultFixtureScenario, 2, 1},
	}
	location, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	for _, test := range tests {
		t.Run(test.scenario, func(t *testing.T) {
			fixture, err := dataprofile.EnsureFixtureScenario(t.TempDir(), test.scenario)
			require.NoError(t, err)
			service := services.NewHomepageReportServiceWithLocation(
				openFixtureDatabase(t, fixture.DatabasePath),
				location,
			)
			payload, err := service.ListTodayAndHistory(30)
			require.NoError(t, err)
			require.Len(t, payload.Today, test.today)
			require.Len(t, payload.History, test.history)
		})
	}
}

func openFixtureDatabase(t *testing.T, path string) *gorm.DB {
	t.Helper()
	databaseURL := (&url.URL{Scheme: "file", Path: path}).String()
	db, err := gorm.Open(
		sqlite.Open(databaseURL+"?mode=ro&_query_only=1&_foreign_keys=on"),
		&gorm.Config{},
	)
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}
