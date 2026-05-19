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

func TestSearchServiceUsesFTSWhenAvailable(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:search_fts_test?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.Podcast{}, &models.Episode{}))

	podcast := models.Podcast{
		Title:       "Tech Daily",
		FeedURL:     "https://example.com/feed.xml",
		Description: "Daily technology updates",
		Author:      "Example",
	}
	require.NoError(t, db.Create(&podcast).Error)

	require.NoError(t, db.Create(&models.Episode{
		PodcastID:     podcast.ID,
		Title:         "Morning briefing",
		ShowNotes:     "A compact podcast update about infrastructure.",
		PublishedDate: time.Now(),
		GUID:          "episode-fts-match",
	}).Error)
	require.NoError(t, db.Create(&models.Episode{
		PodcastID:     podcast.ID,
		Title:         "Unrelated episode",
		ShowNotes:     "No matching keyword here.",
		PublishedDate: time.Now().Add(-time.Hour),
		GUID:          "episode-fts-miss",
	}).Error)

	createSearchFTSTablesForTest(t, db)

	service := NewSearchServiceWithDB(db, defaultSearchConfig())
	response, err := service.Search(SearchRequest{
		Query:           "podcast",
		Type:            "episodes",
		Page:            1,
		PageSize:        10,
		EpisodePage:     1,
		EpisodePageSize: 10,
	})

	require.NoError(t, err)
	require.Len(t, response.Episodes, 1)
	assert.Equal(t, "Morning briefing", response.Episodes[0].Title)
	assert.Equal(t, 1, response.Pagination.Episodes.Total)
}

func TestCanUseSearchFTSRequiresIndexAndTokenFriendlyQuery(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:search_fts_gate_test?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)

	assert.False(t, canUseSearchFTS(db, episodeSearchFTSTable, "AI"))
	assert.False(t, canUseSearchFTS(db, episodeSearchFTSTable, "科技"))
	assert.False(t, canUseSearchFTS(db, episodeSearchFTSTable, "podcast"))

	require.NoError(t, db.Exec(`
		CREATE VIRTUAL TABLE episode_search_fts
		USING fts4(title, show_notes, tokenize=unicode61)
	`).Error)

	assert.True(t, canUseSearchFTS(db, episodeSearchFTSTable, "podcast"))
}

func createSearchFTSTablesForTest(t *testing.T, db *gorm.DB) {
	t.Helper()

	require.NoError(t, db.Exec(`
		CREATE VIRTUAL TABLE podcast_search_fts
		USING fts4(
			title,
			author,
			description,
			content='podcasts',
			tokenize=unicode61
		)
	`).Error)

	require.NoError(t, db.Exec(`
		CREATE VIRTUAL TABLE episode_search_fts
		USING fts4(
			title,
			show_notes,
			content='episodes',
			tokenize=unicode61
		)
	`).Error)

	require.NoError(t, db.Exec("INSERT INTO podcast_search_fts(podcast_search_fts) VALUES('rebuild')").Error)
	require.NoError(t, db.Exec("INSERT INTO episode_search_fts(episode_search_fts) VALUES('rebuild')").Error)
}
