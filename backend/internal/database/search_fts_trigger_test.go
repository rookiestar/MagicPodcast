package database

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"magicpodcast/internal/models"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestSearchFTSTriggersKeepExternalContentIndexesInSync(t *testing.T) {
	db := openMigrationTestDB(t, defaultSQLiteBusyTimeoutMS)
	require.NoError(t, ApplyMigrations(db))
	installSearchFTSMigrationForTest(t, db)

	podcast := &models.Podcast{
		XYZID:       "fts-trigger-podcast",
		Title:       "Original Podcast Title",
		Author:      "Original Author",
		Description: "Original Podcast Description",
		FeedURL:     "https://example.com/fts-trigger.xml",
	}
	require.NoError(t, db.Create(podcast).Error)

	episode := &models.Episode{
		PodcastID: podcast.ID,
		GUID:      "fts-trigger-episode",
		Title:     "Original Episode Title",
		ShowNotes: "Original Episode Notes",
	}
	require.NoError(t, db.Create(episode).Error)

	require.NoError(t, db.Model(&models.Podcast{}).
		Where("id = ?", podcast.ID).
		Updates(map[string]interface{}{
			"title":       "Updated Podcast Title",
			"author":      "Updated Author",
			"description": "Updated Podcast Description",
		}).Error)
	require.NoError(t, db.Model(&models.Episode{}).
		Where("id = ?", episode.ID).
		Updates(map[string]interface{}{
			"title":      "Updated Episode Title",
			"show_notes": "Updated Episode Notes",
		}).Error)

	assertFTSMatchCount(t, db, "podcast_search_fts", "Updated", 1)
	assertFTSMatchCount(t, db, "podcast_search_fts", "Original", 0)
	assertFTSMatchCount(t, db, "episode_search_fts", "Updated", 1)
	assertFTSMatchCount(t, db, "episode_search_fts", "Original", 0)

	require.NoError(t, db.Delete(&models.Episode{}, episode.ID).Error)
	require.NoError(t, db.Delete(&models.Podcast{}, podcast.ID).Error)
	assertFTSMatchCount(t, db, "episode_search_fts", "Updated", 0)
	assertFTSMatchCount(t, db, "podcast_search_fts", "Updated", 0)
}

func installSearchFTSMigrationForTest(t *testing.T, db *gorm.DB) {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok)
	sqlPath := filepath.Join(filepath.Dir(filename), "../../scripts/migrations/add_search_fts.sql")
	sql, err := os.ReadFile(sqlPath)
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	_, err = sqlDB.Exec(string(sql))
	require.NoError(t, err)
}

func assertFTSMatchCount(t *testing.T, db *gorm.DB, tableName, query string, want int) {
	t.Helper()
	var got int
	require.NoError(t, db.Raw("SELECT COUNT(*) FROM "+tableName+" WHERE "+tableName+" MATCH ?", query).Scan(&got).Error)
	require.Equal(t, want, got, "FTS table %s query %q", tableName, query)
}
