package dataprofile

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"magicpodcast/internal/database"
	"magicpodcast/internal/models"
)

func TestEnsureFixtureCreatesStableCurrentSchemaData(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, time.August, 14, 2, 0, 0, 0, time.FixedZone("UTC+8", 8*60*60))
	first, err := ensureFixtureScenarioAt(root, DefaultFixtureScenario, now)
	require.NoError(t, err)
	second, err := ensureFixtureScenarioAt(root, DefaultFixtureScenario, now)
	require.NoError(t, err)

	require.Equal(t, first.DatabasePath, second.DatabasePath)
	require.Equal(t, "complete-v2-journey-20260814T02-schema-23", first.Version)
	require.Equal(t, DefaultFixtureScenario, first.Scenario)
	require.Equal(t, "2026-08-14T02:00:00+08:00", first.Manifest.FixtureAnchorAt)
	require.Equal(t, int64(3), first.Manifest.Counts["podcasts"])
	require.Equal(t, int64(20), first.Manifest.Counts["episodes"])
	require.Equal(t, int64(11), first.Manifest.Counts["episode_triage_decisions"])
	require.Equal(t, int64(4), first.Manifest.Counts["consumption_queue_orders"])
	require.Equal(t, int64(1), first.Manifest.Counts["episode_completions"])
	require.Equal(t, int64(4), first.Manifest.Counts["reports"])

	db, closeDB, err := openSQLite(first.DatabasePath, true)
	require.NoError(t, err)
	defer closeDB()
	var identities []struct {
		ID   uint
		GUID string
	}
	require.NoError(t, db.Table("episodes").Select("id, guid").Order("id").Scan(&identities).Error)
	require.Len(t, identities, 20)
	require.Equal(t, struct {
		ID   uint
		GUID string
	}{ID: 2001, GUID: "fixture-episode-2001"}, identities[0])
	require.Equal(t, struct {
		ID   uint
		GUID string
	}{ID: 2020, GUID: "fixture-episode-2020"}, identities[len(identities)-1])
	var videoStates []struct {
		ID                uint
		VideoAvailability string
		Link              string
	}
	require.NoError(t, db.Table("episodes").Select("id, video_availability, link").
		Where("id IN ?", []uint{2001, 2013, 2014}).Order("id").Scan(&videoStates).Error)
	require.Equal(t, "", videoStates[0].VideoAvailability)
	require.Equal(t, models.VideoAvailabilityAvailable, videoStates[1].VideoAvailability)
	require.Contains(t, videoStates[1].Link, "6a734c29ab3a91c24a1067fa")
	require.Equal(t, models.VideoAvailabilityUnavailable, videoStates[2].VideoAvailability)
	require.Contains(t, videoStates[2].Link, "6a8cf80a1352af56ff3b7e2d")
	var covers []string
	require.NoError(t, db.Table("podcasts").Order("id").Pluck("cover_url", &covers).Error)
	require.Equal(t, []string{fixtureInlinePNG, "", fixtureInlinePNG}, covers)

	other, err := ensureFixtureScenarioAt(t.TempDir(), DefaultFixtureScenario, now)
	require.NoError(t, err)
	require.Equal(t, first.Manifest.Counts, other.Manifest.Counts)
	require.Equal(t, first.Manifest.FixtureVersion, other.Manifest.FixtureVersion)
	require.Equal(t, fixtureSemanticFingerprint(t, first.DatabasePath), fixtureSemanticFingerprint(t, other.DatabasePath))
}

func TestFixtureScenariosCoverQueueAndReportBoundaries(t *testing.T) {
	now := time.Date(2026, time.August, 14, 2, 0, 0, 0, time.FixedZone("UTC+8", 8*60*60))
	tests := []struct {
		scenario        string
		focusCount      int64
		reportCount     int64
		episodeCount    int64
		completionCount int64
	}{
		{DefaultFixtureScenario, 6, 4, 20, 1},
		{FixtureScenarioEmpty, 0, 0, 0, 0},
		{FixtureScenarioFocusZero, 0, 4, 20, 1},
		{FixtureScenarioFocusSeven, 7, 4, 20, 1},
		{FixtureScenarioFocusOverLimit, 8, 4, 20, 1},
		{FixtureScenarioCompletionHistory, 6, 4, 75, 59},
		{FixtureScenarioReportEmpty, 6, 0, 20, 1},
		{FixtureScenarioReportSingle, 6, 3, 20, 1},
	}
	for _, test := range tests {
		t.Run(test.scenario, func(t *testing.T) {
			fixture, err := ensureFixtureScenarioAt(t.TempDir(), test.scenario, now)
			require.NoError(t, err)
			db, closeDB, err := openSQLite(fixture.DatabasePath, true)
			require.NoError(t, err)
			defer closeDB()

			var focusCount int64
			require.NoError(t, db.Model(&models.EpisodeTriageDecision{}).
				Where("queue_state = ?", models.QueueStateFocus).
				Count(&focusCount).Error)
			require.Equal(t, test.focusCount, focusCount)
			require.Equal(t, test.reportCount, fixture.Manifest.Counts["reports"])
			require.Equal(t, test.episodeCount, fixture.Manifest.Counts["episodes"])
			require.Equal(
				t,
				test.completionCount,
				fixture.Manifest.Counts["episode_completions"],
			)
		})
	}
}

func TestEnsureFixtureRejectsUnknownScenario(t *testing.T) {
	_, err := EnsureFixtureScenario(t.TempDir(), "system-recommendation")
	require.ErrorContains(t, err, "unsupported fixture scenario")
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
	var queueOrders []string
	require.NoError(t, db.Raw(`
		SELECT queue_state || '|' || CAST(revision AS TEXT) || '|' || CAST(updated_at AS TEXT)
		FROM consumption_queue_orders
		ORDER BY queue_state`).Scan(&queueOrders).Error)
	var tableCounts []string
	for _, table := range []string{
		"workflows",
		"sync_configs",
		"jobs",
		"job_executions",
		"job_feed_attempts",
		"reports",
		"episode_triage_decisions",
		"consumption_queue_orders",
		"episode_completions",
		"feed_snapshots",
		"feed_user_agent_gates",
		"feed_user_agent_gate_audits",
		"feed_user_agent_gate_recovery_feeds",
	} {
		var count int64
		require.NoError(t, db.Table(table).Count(&count).Error)
		tableCounts = append(tableCounts, fmt.Sprintf("%s|%d", table, count))
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
	fingerprint = append(fingerprint, "queue-orders")
	fingerprint = append(fingerprint, queueOrders...)
	fingerprint = append(fingerprint, "table-counts")
	fingerprint = append(fingerprint, tableCounts...)
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
