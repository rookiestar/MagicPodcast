package dataprofile

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"magicpodcast/internal/database"
)

func TestEnsureFixtureCreatesStableCurrentSchemaData(t *testing.T) {
	root := t.TempDir()
	first, err := EnsureFixture(root)
	require.NoError(t, err)
	second, err := EnsureFixture(root)
	require.NoError(t, err)

	require.Equal(t, first.DatabasePath, second.DatabasePath)
	require.Equal(t, CurrentFixtureVersion(), first.Version)
	require.Equal(t, int64(2), first.Manifest.Counts["podcasts"])
	require.Equal(t, int64(3), first.Manifest.Counts["episodes"])
	require.Equal(t, int64(0), first.Manifest.Counts["episode_triage_decisions"])

	db, closeDB, err := openSQLite(first.DatabasePath, true)
	require.NoError(t, err)
	defer closeDB()
	var identities []struct {
		ID   uint
		GUID string
	}
	require.NoError(t, db.Table("episodes").Select("id, guid").Order("id").Scan(&identities).Error)
	require.Equal(t, []struct {
		ID   uint
		GUID string
	}{
		{ID: 2001, GUID: "fixture-episode-2001"},
		{ID: 2002, GUID: "fixture-episode-2002"},
		{ID: 2003, GUID: "fixture-episode-2003"},
	}, identities)

	other, err := EnsureFixture(t.TempDir())
	require.NoError(t, err)
	require.Equal(t, first.Manifest.Counts, other.Manifest.Counts)
	require.Equal(t, first.Manifest.FixtureVersion, other.Manifest.FixtureVersion)
	require.Equal(t, fixtureSemanticFingerprint(t, first.DatabasePath), fixtureSemanticFingerprint(t, other.DatabasePath))
}

func fixtureSemanticFingerprint(t *testing.T, path string) []string {
	t.Helper()
	db, closeDB, err := openSQLite(path, true)
	require.NoError(t, err)
	defer closeDB()

	var schemaVersions []string
	require.NoError(t, db.Raw(`
		SELECT CAST(version AS TEXT) || '|' || name || '|' || CAST(applied_at AS TEXT)
		FROM schema_migrations
		ORDER BY version`).Scan(&schemaVersions).Error)
	var podcasts []string
	require.NoError(t, db.Raw(`
		SELECT printf('%d|%s|%s|%s|%s|%d', id, xyz_id, title, feed_url, data_source, episode_count)
		FROM podcasts
		ORDER BY id`).Scan(&podcasts).Error)
	var episodes []string
	require.NoError(t, db.Raw(`
		SELECT printf('%d|%d|%s|%s|%s|%d', id, podcast_id, episode_no, title, guid, duration)
		FROM episodes
		ORDER BY id`).Scan(&episodes).Error)
	var tags []string
	require.NoError(t, db.Raw(`
		SELECT printf('%d|%s|%s', id, name, color)
		FROM tags
		ORDER BY id`).Scan(&tags).Error)
	var relations []string
	require.NoError(t, db.Raw(`
		SELECT printf('%d|%d', podcast_id, tag_id)
		FROM podcasts_tags
		ORDER BY podcast_id, tag_id`).Scan(&relations).Error)
	var emptyCounts []string
	for _, table := range []string{
		"workflows",
		"sync_configs",
		"jobs",
		"job_executions",
		"job_feed_attempts",
		"reports",
		"episode_triage_decisions",
		"feed_snapshots",
		"feed_user_agent_gates",
		"feed_user_agent_gate_audits",
		"feed_user_agent_gate_recovery_feeds",
	} {
		var count int64
		require.NoError(t, db.Table(table).Count(&count).Error)
		emptyCounts = append(emptyCounts, fmt.Sprintf("%s|%d", table, count))
	}
	fingerprint := append([]string{"schema"}, schemaVersions...)
	fingerprint = append(fingerprint, "podcasts")
	fingerprint = append(fingerprint, podcasts...)
	fingerprint = append(fingerprint, "episodes")
	fingerprint = append(fingerprint, episodes...)
	fingerprint = append(fingerprint, "tags")
	fingerprint = append(fingerprint, tags...)
	fingerprint = append(fingerprint, "relations")
	fingerprint = append(fingerprint, relations...)
	fingerprint = append(fingerprint, "empty-counts")
	fingerprint = append(fingerprint, emptyCounts...)
	return fingerprint
}

func TestEnsureFixturePreservesCorruptExistingFixture(t *testing.T) {
	root := t.TempDir()
	fixtureDir := filepath.Join(root, "fixtures", CurrentFixtureVersion())
	require.NoError(t, os.MkdirAll(fixtureDir, 0o700))
	databasePath := filepath.Join(fixtureDir, "magicpodcast.db")
	original := []byte("not sqlite")
	require.NoError(t, os.WriteFile(databasePath, original, 0o600))

	_, err := EnsureFixture(root)
	require.ErrorContains(t, err, "existing fixture is invalid and was preserved")
	after, readErr := os.ReadFile(databasePath)
	require.NoError(t, readErr)
	require.Equal(t, original, after)
}

func TestValidateDatabaseRejectsMissingTables(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing-tables.db")
	db, closeDB, err := openSQLite(path, false)
	require.NoError(t, err)
	require.NoError(t, db.Exec("CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, name TEXT, applied_at DATETIME)").Error)
	require.NoError(t, db.Exec("INSERT INTO schema_migrations(version, name, applied_at) VALUES (1, 'old', CURRENT_TIMESTAMP)").Error)
	closeDB()

	_, err = ValidateDatabase(path)
	require.ErrorContains(t, err, "required tables missing")
}

func TestValidateDatabaseRejectsOldSchemaWithAllRequiredTablesPresent(t *testing.T) {
	root := t.TempDir()
	fixture, err := EnsureFixture(root)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "old-schema.db")
	require.NoError(t, copyRegularFile(fixture.DatabasePath, path, 0o600))
	db, err := openSQLDatabase(path, false)
	require.NoError(t, err)
	_, err = db.Exec("DELETE FROM schema_migrations WHERE version = ?", database.CurrentSchemaVersion)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	_, err = ValidateDatabase(path)
	require.ErrorContains(t, err, "schema version")
	require.NotContains(t, err.Error(), "required tables missing")
}
