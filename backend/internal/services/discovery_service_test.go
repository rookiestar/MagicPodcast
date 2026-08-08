package services

import (
	"fmt"
	"testing"
	"time"

	"magicpodcast/internal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupDiscoveryTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:discovery_service_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.Podcast{}, &models.Episode{}, &models.Tag{}))

	t.Cleanup(func() {
		sqlDB, _ := db.DB()
		_ = sqlDB.Close()
	})
	return db
}

func createDiscoveryPodcast(t *testing.T, db *gorm.DB, title string) models.Podcast {
	t.Helper()

	podcast := models.Podcast{
		Title:        title,
		Author:       "测试作者",
		FeedURL:      fmt.Sprintf("https://example.com/%d.xml", time.Now().UnixNano()),
		XYZID:        fmt.Sprintf("xyz-%d", time.Now().UnixNano()),
		CoverURL:     "https://example.com/cover.jpg",
		IsSubscribed: true,
	}
	require.NoError(t, db.Create(&podcast).Error)
	return podcast
}

func createDiscoveryEpisode(
	t *testing.T,
	db *gorm.DB,
	podcastID uint,
	title string,
	published time.Time,
	updated *time.Time,
) models.Episode {
	t.Helper()

	episode := models.Episode{
		PodcastID:     podcastID,
		Title:         title,
		GUID:          fmt.Sprintf("guid-%d", time.Now().UnixNano()),
		PublishedDate: published,
		UpdatedDate:   updated,
		Duration:      3720,
		ShowNotes:     "<p>可核对的 Show Notes</p>",
		Link:          "https://example.com/episode",
	}
	require.NoError(t, db.Create(&episode).Error)
	return episode
}

func TestDiscoveryService_ListRecentCandidates_UsesStableVerifiableRecency(t *testing.T) {
	db := setupDiscoveryTestDB(t)
	podcast := createDiscoveryPodcast(t, db, "个人播客")
	service := NewDiscoveryService(db)

	samePublished := time.Date(2026, 7, 28, 9, 0, 0, 0, time.UTC)
	olderPublished := time.Date(2026, 7, 27, 9, 0, 0, 0, time.UTC)
	fallbackUpdated := time.Date(2026, 7, 29, 8, 30, 0, 0, time.UTC)

	older := createDiscoveryEpisode(t, db, podcast.ID, "较早发布", olderPublished, nil)
	firstTie := createDiscoveryEpisode(t, db, podcast.ID, "同时间先写入", samePublished, nil)
	secondTie := createDiscoveryEpisode(t, db, podcast.ID, "同时间后写入", samePublished, nil)
	fallback := createDiscoveryEpisode(t, db, podcast.ID, "缺少发布时间", time.Time{}, &fallbackUpdated)

	candidates, err := service.ListRecentCandidates(20)

	require.NoError(t, err)
	require.Len(t, candidates, 4)
	assert.Equal(t, fallback.ID, candidates[0].EpisodeID)
	assert.Equal(t, secondTie.ID, candidates[1].EpisodeID)
	assert.Equal(t, firstTie.ID, candidates[2].EpisodeID)
	assert.Equal(t, older.ID, candidates[3].EpisodeID)
	assert.Equal(t, fallbackUpdated, candidates[0].CandidateTime)
	assert.Equal(t, "updated_date", candidates[0].TimeBasis)
	assert.Equal(t, "published_date", candidates[1].TimeBasis)
	assert.Equal(t, "最近更新", candidates[0].Source)
	assert.Equal(t, "个人播客", candidates[0].PodcastTitle)
	assert.NotEmpty(t, candidates[0].ShowNotes)
	assert.NotEmpty(t, candidates[0].OriginalURL)
}

func TestDiscoveryService_ListRecentCandidates_NormalizesEpisodeNoForDisplay(t *testing.T) {
	db := setupDiscoveryTestDB(t)
	podcast := createDiscoveryPodcast(t, db, "期号口径节目")

	withTitleLabel := createDiscoveryEpisode(
		t,
		db,
		podcast.ID,
		"节目 S10E24 特别篇",
		time.Date(2026, 7, 29, 8, 0, 0, 0, time.UTC),
		nil,
	)
	withTitleLabel.EpisodeNo = "20240438"
	require.NoError(t, db.Save(&withTitleLabel).Error)

	withoutReliableLabel := createDiscoveryEpisode(
		t,
		db,
		podcast.ID,
		"无期号标题",
		time.Date(2026, 7, 28, 8, 0, 0, 0, time.UTC),
		nil,
	)
	withoutReliableLabel.EpisodeNo = "20240439"
	require.NoError(t, db.Save(&withoutReliableLabel).Error)

	candidates, err := NewDiscoveryService(db).ListRecentCandidates(2)

	require.NoError(t, err)
	require.Len(t, candidates, 2)
	assert.Equal(t, "S10E24", candidates[0].EpisodeNo)
	assert.Empty(t, candidates[1].EpisodeNo)
}

func TestDiscoveryService_ListRecentCandidates_ReturnsFourEvidenceBoundPreReads(t *testing.T) {
	db := setupDiscoveryTestDB(t)
	podcast := createDiscoveryPodcast(t, db, "个人播客")
	podcast.Notes = "持续关注离线优先工具"
	require.NoError(t, db.Save(&podcast).Error)
	episode := createDiscoveryEpisode(
		t,
		db,
		podcast.ID,
		"离线优先的 AI 工具",
		time.Date(2026, 7, 29, 8, 0, 0, 0, time.UTC),
		nil,
	)
	episode.Notes = "想核对隐私与同步边界"
	episode.ShowNotes = "<p>节目主张离线优先的 AI 工具应明确说明隐私与同步边界。</p>"
	require.NoError(t, db.Save(&episode).Error)
	tag := models.Tag{Name: "AI 工具", Color: "#7A4B2A"}
	require.NoError(t, db.Create(&tag).Error)
	require.NoError(t, db.Model(&episode).Association("Tags").Append(&tag))

	candidates, err := NewDiscoveryService(db).ListRecentCandidates(1)

	require.NoError(t, err)
	require.Len(t, candidates, 1)
	require.Len(t, candidates[0].PreReads, 4)

	preReads := make(map[string]DiscoveryPreRead, len(candidates[0].PreReads))
	for _, preRead := range candidates[0].PreReads {
		preReads[preRead.Kind] = preRead
		assert.NotEmpty(t, preRead.Content)
		assert.NotEmpty(t, preRead.Version)
		assert.False(t, preRead.GeneratedAt.IsZero())
	}

	assert.Equal(t, PreReadStatusAvailable, preReads[PreReadKindSummary].Status)
	assert.Equal(t, PreReadStatusAvailable, preReads[PreReadKindViewpoints].Status)
	assert.Equal(t, PreReadStatusAvailable, preReads[PreReadKindRelevant].Status)
	assert.Equal(t, "明确相关", preReads[PreReadKindRelevant].RelationStrength)
	assert.Contains(t, preReads[PreReadKindRelevant].Content, "AI 工具")
	assert.Equal(t, PreReadStatusAvailable, preReads[PreReadKindChallenge].Status)
	assert.Contains(t, preReads[PreReadKindSummary].Sources, DiscoveryPreReadSource{
		Kind:  "show_notes",
		Label: "Show Notes",
	})
	assert.Contains(t, preReads[PreReadKindSummary].Sources, DiscoveryPreReadSource{
		Kind:  "original_link",
		Label: "原始链接",
		URL:   "https://example.com/episode",
	})
}

func TestDiscoveryService_ListRecentCandidates_DoesNotInventPreReadsWithoutEvidence(t *testing.T) {
	db := setupDiscoveryTestDB(t)
	podcast := createDiscoveryPodcast(t, db, "无个人信号节目")
	episode := createDiscoveryEpisode(
		t,
		db,
		podcast.ID,
		"原始信息不足",
		time.Date(2026, 7, 29, 8, 0, 0, 0, time.UTC),
		nil,
	)
	require.NoError(t, db.Model(&episode).Update("show_notes", "").Error)

	candidates, err := NewDiscoveryService(db).ListRecentCandidates(1)

	require.NoError(t, err)
	require.Len(t, candidates, 1)
	preReads := make(map[string]DiscoveryPreRead, len(candidates[0].PreReads))
	for _, preRead := range candidates[0].PreReads {
		preReads[preRead.Kind] = preRead
	}
	assert.Equal(t, PreReadStatusMissing, preReads[PreReadKindSummary].Status)
	assert.Equal(t, PreReadStatusMissing, preReads[PreReadKindViewpoints].Status)
	assert.Equal(t, PreReadStatusMissing, preReads[PreReadKindChallenge].Status)
	assert.Equal(t, PreReadStatusInsufficient, preReads[PreReadKindRelevant].Status)
	assert.Empty(t, preReads[PreReadKindRelevant].RelationStrength)
	assert.Contains(t, preReads[PreReadKindRelevant].Content, "不生成个人关联")
}

func TestDiscoveryService_ListTodayShortlisted_UsesConfiguredTimezoneAndStableOrder(t *testing.T) {
	db := setupDiscoveryTestDB(t)
	require.NoError(t, db.AutoMigrate(&models.EpisodeTriageDecision{}))
	podcast := createDiscoveryPodcast(t, db, "今日备选节目")
	first := createDiscoveryEpisode(
		t,
		db,
		podcast.ID,
		"今日较早写入",
		time.Date(2026, 7, 28, 8, 0, 0, 0, time.UTC),
		nil,
	)
	second := createDiscoveryEpisode(
		t,
		db,
		podcast.ID,
		"今日较晚写入",
		time.Date(2026, 7, 29, 8, 0, 0, 0, time.UTC),
		nil,
	)
	previousDay := createDiscoveryEpisode(
		t,
		db,
		podcast.ID,
		"昨日备选",
		time.Date(2026, 7, 27, 8, 0, 0, 0, time.UTC),
		nil,
	)
	sameDecisionTime := time.Date(2026, 7, 28, 16, 30, 0, 0, time.UTC)
	require.NoError(t, db.Create(&models.EpisodeTriageDecision{
		EpisodeID: first.ID,
		State:     models.TriageStateShortlisted,
		DecidedAt: sameDecisionTime,
	}).Error)
	require.NoError(t, db.Create(&models.EpisodeTriageDecision{
		EpisodeID: second.ID,
		State:     models.TriageStateShortlisted,
		DecidedAt: sameDecisionTime,
	}).Error)
	require.NoError(t, db.Create(&models.EpisodeTriageDecision{
		EpisodeID: previousDay.ID,
		State:     models.TriageStateShortlisted,
		DecidedAt: time.Date(2026, 7, 28, 15, 59, 0, 0, time.UTC),
	}).Error)
	location, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	service := NewDiscoveryServiceWithLocation(db, location)
	service.now = func() time.Time {
		return time.Date(2026, 7, 29, 10, 0, 0, 0, location)
	}

	shortlist, err := service.ListTodayShortlisted()

	require.NoError(t, err)
	assert.Equal(t, "2026-07-29", shortlist.Date)
	assert.Equal(t, "Asia/Shanghai", shortlist.Timezone)
	require.Len(t, shortlist.Candidates, 2)
	assert.Equal(t, second.ID, shortlist.Candidates[0].EpisodeID)
	assert.Equal(t, first.ID, shortlist.Candidates[1].EpisodeID)
	assert.Equal(t, models.TriageStateShortlisted, shortlist.Candidates[0].DecisionState)
	assert.NotEmpty(t, shortlist.Candidates[0].PreReads[0].Content)
}

func TestDiscoveryService_ListRecentCandidates_ClampsLimit(t *testing.T) {
	db := setupDiscoveryTestDB(t)
	podcast := createDiscoveryPodcast(t, db, "个人播客")
	service := NewDiscoveryService(db)

	for index := 0; index < 105; index++ {
		createDiscoveryEpisode(
			t,
			db,
			podcast.ID,
			fmt.Sprintf("单集 %03d", index),
			time.Date(2026, 7, 28, 9, index, 0, 0, time.UTC),
			nil,
		)
	}

	candidates, err := service.ListRecentCandidates(1000)

	require.NoError(t, err)
	assert.Len(t, candidates, 100)
}
