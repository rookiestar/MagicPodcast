package main

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
)

func openRepairTestDB(t *testing.T) *sql.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "magicpodcast.db")
	db, err := sql.Open("sqlite3", dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	_, err = db.Exec(`
		CREATE TABLE podcasts (
			id INTEGER PRIMARY KEY,
			title TEXT NOT NULL,
			episode_count INTEGER NOT NULL DEFAULT 0,
			newest_episode_date DATETIME,
			updated_at DATETIME,
			deleted_at DATETIME
		);
		CREATE TABLE episodes (
			id INTEGER PRIMARY KEY,
			podcast_id INTEGER NOT NULL,
			published_date DATETIME NOT NULL,
			deleted_at DATETIME
		);`)
	require.NoError(t, err)
	return db
}

func TestLoadRepairPlanUsesPublishedDateAndIgnoresDeletedEpisodes(t *testing.T) {
	db := openRepairTestDB(t)
	_, err := db.Exec(`
		INSERT INTO podcasts(id, title, episode_count, newest_episode_date)
		VALUES (1, 'Needs repair', 0, '2026-07-01 08:00:00+00:00');
		INSERT INTO episodes(id, podcast_id, published_date)
		VALUES (1, 1, '2026-07-03 08:00:00+00:00');
		INSERT INTO episodes(id, podcast_id, published_date, deleted_at)
		VALUES (2, 1, '2026-07-10 08:00:00+00:00', '2026-07-11 00:00:00+00:00');`)
	require.NoError(t, err)

	plan, audit, err := loadRepairPlan(db)
	require.NoError(t, err)
	require.Len(t, plan, 1)
	require.Equal(t, 1, audit.MismatchedPodcasts)
	require.Equal(t, 1, audit.EpisodeCountMismatches)
	require.Equal(t, 1, audit.NewestEpisodeDateMismatches)
	require.Equal(t, int64(1), plan[0].ExpectedEpisodeCount)
	require.Equal(t, "2026-07-03T08:00:00Z", *plan[0].ExpectedNewestEpisodeDate)
}

func TestApplyRepairsPodcastSummariesAndLeavesEpisodeDataUntouched(t *testing.T) {
	db := openRepairTestDB(t)
	_, err := db.Exec(`
		INSERT INTO podcasts(id, title, episode_count, newest_episode_date)
		VALUES (1, 'Needs repair', 0, NULL);
		INSERT INTO episodes(id, podcast_id, published_date)
		VALUES (1, 1, '2026-07-14 08:00:00+00:00');`)
	require.NoError(t, err)

	plan, _, err := loadRepairPlan(db)
	require.NoError(t, err)
	require.Len(t, plan, 1)

	applied, err := applyRepairs(db, plan)
	require.NoError(t, err)
	require.Equal(t, 1, applied)

	var episodeCount int64
	var newest string
	require.NoError(t, db.QueryRow(`SELECT episode_count, newest_episode_date FROM podcasts WHERE id = 1`).Scan(&episodeCount, &newest))
	require.Equal(t, int64(1), episodeCount)
	require.Equal(t, "2026-07-14T08:00:00Z", newest)

	var episodeDate string
	require.NoError(t, db.QueryRow(`SELECT published_date FROM episodes WHERE id = 1`).Scan(&episodeDate))
	require.Equal(t, "2026-07-14T08:00:00Z", canonicalDate(episodeDate))

	_, after, err := loadRepairPlan(db)
	require.NoError(t, err)
	require.Equal(t, 0, after.MismatchedPodcasts)
}

func TestBackupDatabaseCreatesVerifiableCopy(t *testing.T) {
	db := openRepairTestDB(t)
	_, err := db.Exec(`INSERT INTO podcasts(id, title) VALUES (1, 'Backup me')`)
	require.NoError(t, err)

	backupPath := filepath.Join(t.TempDir(), "backup.db")
	require.NoError(t, backupDatabase(db, backupPath))

	backup, err := sql.Open("sqlite3", backupPath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, backup.Close()) })
	var title string
	require.NoError(t, backup.QueryRow(`SELECT title FROM podcasts WHERE id = 1`).Scan(&title))
	require.Equal(t, "Backup me", title)
}

func TestValidateApplyRequiresExplicitConfirmation(t *testing.T) {
	require.Error(t, validateApply(true, ""))
	require.Error(t, validateApply(true, "I_UNDERSTAND_THIS_WRITES_DATA"))
	require.NoError(t, validateApply(true, applyConfirmation))
	require.NoError(t, validateApply(false, ""))
}
