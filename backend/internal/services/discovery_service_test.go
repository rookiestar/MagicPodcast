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
	fetchedAt := published
	if fetchedAt.IsZero() && updated != nil {
		fetchedAt = *updated
	}
	if !fetchedAt.IsZero() {
		episode.FetchedAt = &fetchedAt
	}
	require.NoError(t, db.Create(&episode).Error)
	return episode
}

func setDiscoveryEpisodeSystemTime(
	t *testing.T,
	db *gorm.DB,
	episode models.Episode,
	fetchedAt *time.Time,
	createdAt time.Time,
) {
	t.Helper()
	require.NoError(t, db.Model(&episode).UpdateColumns(map[string]interface{}{
		"fetched_at": fetchedAt,
		"created_at": createdAt,
	}).Error)
}

func TestDiscoveryService_ListRecentCandidates_UsesSystemSyncRecency(t *testing.T) {
	db := setupDiscoveryTestDB(t)
	podcast := createDiscoveryPodcast(t, db, "个人播客")
	service := NewDiscoveryService(db)
	service.now = func() time.Time {
		return time.Date(2026, 7, 30, 8, 30, 0, 0, time.UTC)
	}

	oldSourceDate := time.Date(2025, 1, 1, 8, 0, 0, 0, time.UTC)
	recentSourceDate := time.Date(2026, 7, 30, 8, 0, 0, 0, time.UTC)
	latestSync := time.Date(2026, 7, 29, 8, 30, 0, 0, time.UTC)
	sameSync := time.Date(2026, 7, 28, 9, 0, 0, 0, time.UTC)
	legacyCreated := time.Date(2026, 7, 27, 9, 0, 0, 0, time.UTC)
	outsideWindow := time.Date(2026, 7, 10, 9, 0, 0, 0, time.UTC)

	latest := createDiscoveryEpisode(t, db, podcast.ID, "最近同步", oldSourceDate, nil)
	firstTie := createDiscoveryEpisode(t, db, podcast.ID, "同时间先写入", oldSourceDate, nil)
	secondTie := createDiscoveryEpisode(t, db, podcast.ID, "同时间后写入", oldSourceDate, nil)
	legacy := createDiscoveryEpisode(t, db, podcast.ID, "历史记录回退", oldSourceDate, nil)
	stale := createDiscoveryEpisode(t, db, podcast.ID, "源站新但同步早", recentSourceDate, nil)
	setDiscoveryEpisodeSystemTime(t, db, latest, &latestSync, latestSync)
	setDiscoveryEpisodeSystemTime(t, db, firstTie, &sameSync, sameSync)
	setDiscoveryEpisodeSystemTime(t, db, secondTie, &sameSync, sameSync)
	setDiscoveryEpisodeSystemTime(t, db, legacy, nil, legacyCreated)
	setDiscoveryEpisodeSystemTime(t, db, stale, &outsideWindow, outsideWindow)

	candidates, err := service.ListRecentCandidates(20)

	require.NoError(t, err)
	require.Len(t, candidates, 4)
	assert.Equal(t, latest.ID, candidates[0].EpisodeID)
	assert.Equal(t, secondTie.ID, candidates[1].EpisodeID)
	assert.Equal(t, firstTie.ID, candidates[2].EpisodeID)
	assert.Equal(t, legacy.ID, candidates[3].EpisodeID)
	assert.Equal(t, latestSync, candidates[0].CandidateTime)
	assert.Equal(t, "fetched_at", candidates[0].TimeBasis)
	assert.Equal(t, legacyCreated, candidates[3].CandidateTime)
	assert.Equal(t, "created_at", candidates[3].TimeBasis)
	assert.NotContains(t, []uint{
		candidates[0].EpisodeID,
		candidates[1].EpisodeID,
		candidates[2].EpisodeID,
		candidates[3].EpisodeID,
	}, stale.ID)
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

	service := NewDiscoveryService(db)
	service.now = func() time.Time {
		return time.Date(2026, 7, 30, 8, 0, 0, 0, time.UTC)
	}
	candidates, err := service.ListRecentCandidates(2)

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

	service := NewDiscoveryService(db)
	service.now = func() time.Time {
		return time.Date(2026, 7, 30, 8, 0, 0, 0, time.UTC)
	}
	candidates, err := service.ListRecentCandidates(1)

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

	service := NewDiscoveryService(db)
	service.now = func() time.Time {
		return time.Date(2026, 7, 30, 8, 0, 0, 0, time.UTC)
	}
	candidates, err := service.ListRecentCandidates(1)

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

func TestDiscoveryService_ListRecentCandidates_AllowsFullSevenDayWindow(t *testing.T) {
	db := setupDiscoveryTestDB(t)
	podcast := createDiscoveryPodcast(t, db, "个人播客")
	service := NewDiscoveryService(db)
	service.now = func() time.Time {
		return time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)
	}

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
	assert.Len(t, candidates, 105)
}

func TestDiscoveryService_ListRecentCandidateSummaries_OmitsHeavyContent(t *testing.T) {
	db := setupDiscoveryTestDB(t)
	podcast := createDiscoveryPodcast(t, db, "轻量候选节目")
	episode := createDiscoveryEpisode(
		t,
		db,
		podcast.ID,
		"轻量候选单集",
		time.Date(2026, 7, 29, 8, 0, 0, 0, time.UTC),
		nil,
	)
	episode.ShowNotes = "<p>这是一段用于列表摘要、但不应完整返回的 Show Notes。</p>"
	require.NoError(t, db.Save(&episode).Error)

	service := NewDiscoveryService(db)
	service.now = func() time.Time {
		return time.Date(2026, 7, 30, 8, 0, 0, 0, time.UTC)
	}
	candidates, err := service.ListRecentCandidateSummaries(10)

	require.NoError(t, err)
	require.Len(t, candidates, 1)
	assert.True(t, candidates[0].MetadataOnly)
	assert.Contains(t, candidates[0].Excerpt, "这是一段用于列表摘要")
	assert.Empty(t, candidates[0].ShowNotes)
	assert.Empty(t, candidates[0].PreReads)
	assert.Equal(t, "available", candidates[0].ShowNotesStatus)
}

func TestDiscoveryService_GetCandidate_ReturnsFullOnDemandContent(t *testing.T) {
	db := setupDiscoveryTestDB(t)
	podcast := createDiscoveryPodcast(t, db, "详情候选节目")
	episode := createDiscoveryEpisode(
		t,
		db,
		podcast.ID,
		"详情候选单集",
		time.Date(2026, 7, 29, 8, 0, 0, 0, time.UTC),
		nil,
	)
	episode.ShowNotes = "<p>按需返回的完整 Show Notes</p>"
	require.NoError(t, db.Save(&episode).Error)

	candidate, err := NewDiscoveryService(db).GetCandidate(episode.ID)

	require.NoError(t, err)
	require.NotNil(t, candidate)
	assert.False(t, candidate.MetadataOnly)
	assert.Equal(t, episode.ShowNotes, candidate.ShowNotes)
	require.Len(t, candidate.PreReads, 4)
}

func TestDiscoveryService_ListRecentCandidates_UsesSevenDayRollingBoundary(t *testing.T) {
	db := setupDiscoveryTestDB(t)
	podcast := createDiscoveryPodcast(t, db, "七天边界")
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	includedAtBoundary := createDiscoveryEpisode(
		t, db, podcast.ID, "刚好七天", now.Add(-7*24*time.Hour), nil,
	)
	includedNewer := createDiscoveryEpisode(
		t, db, podcast.ID, "窗口内", now.Add(-7*24*time.Hour+time.Second), nil,
	)
	createDiscoveryEpisode(
		t, db, podcast.ID, "窗口外", now.Add(-7*24*time.Hour-time.Second), nil,
	)
	service := NewDiscoveryService(db)
	service.now = func() time.Time { return now }

	candidates, err := service.ListRecentCandidates(20)

	require.NoError(t, err)
	require.Len(t, candidates, 2)
	assert.Equal(t, includedNewer.ID, candidates[0].EpisodeID)
	assert.Equal(t, includedAtBoundary.ID, candidates[1].EpisodeID)
}

func TestDiscoveryService_ListRecentCandidates_UsesAbsoluteTimeAcrossTimeZones(t *testing.T) {
	db := setupDiscoveryTestDB(t)
	podcast := createDiscoveryPodcast(t, db, "跨时区七天边界")
	shanghai := time.FixedZone("Asia/Shanghai", 8*60*60)
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, shanghai)
	includedAtBoundary := createDiscoveryEpisode(
		t, db, podcast.ID, "本地时间刚好七天", now.Add(-7*24*time.Hour), nil,
	)
	createDiscoveryEpisode(
		t, db, podcast.ID, "本地时间窗口外", now.Add(-7*24*time.Hour-time.Second), nil,
	)
	olderShanghai := createDiscoveryEpisode(
		t,
		db,
		podcast.ID,
		"上海时区较早",
		time.Date(2026, 8, 5, 10, 0, 0, 0, shanghai),
		nil,
	)
	newerUTC := createDiscoveryEpisode(
		t,
		db,
		podcast.ID,
		"UTC 时区较晚",
		time.Date(2026, 8, 5, 3, 0, 0, 0, time.UTC),
		nil,
	)
	service := NewDiscoveryService(db)
	service.now = func() time.Time { return now }

	candidates, err := service.ListRecentCandidates(20)

	require.NoError(t, err)
	require.Len(t, candidates, 3)
	assert.Equal(t, newerUTC.ID, candidates[0].EpisodeID)
	assert.Equal(t, olderShanghai.ID, candidates[1].EpisodeID)
	assert.Equal(t, includedAtBoundary.ID, candidates[2].EpisodeID)
}
