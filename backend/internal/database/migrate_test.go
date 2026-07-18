package database

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"magicpodcast/internal/models"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func openMigrationTestDB(t *testing.T, busyTimeoutMS int) *gorm.DB {
	t.Helper()
	return openMigrationTestDBAt(t, filepath.Join(t.TempDir(), "magicpodcast.db"), busyTimeoutMS)
}

func openMigrationTestDBAt(t *testing.T, path string, busyTimeoutMS int) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("%s?_journal_mode=WAL&_foreign_keys=on&_busy_timeout=%d", path, busyTimeoutMS)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: false,
	})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}

func TestApplyMigrationsCreatesVersionedReadySchema(t *testing.T) {
	db := openMigrationTestDB(t, defaultSQLiteBusyTimeoutMS)

	require.NoError(t, ApplyMigrations(db))
	require.NoError(t, RequireSchemaReady(db))

	var applied SchemaMigration
	require.NoError(t, db.First(&applied, 1).Error)
	require.Equal(t, 1, applied.Version)
	require.Equal(t, "baseline-current-model", applied.Name)

	status, err := InspectSchema(db)
	require.NoError(t, err)
	require.True(t, status.MigrationTablePresent)
	require.Equal(t, CurrentSchemaVersion, status.CurrentVersion)
	require.Empty(t, status.RequiredTablesMissing)
	require.Empty(t, status.Pending)
	require.True(t, db.Migrator().HasColumn(&models.JobExecution{}, "feed_http_status"))
	require.True(t, db.Migrator().HasColumn(&models.JobExecution{}, "feed_error_category"))
	require.True(t, db.Migrator().HasColumn(&models.JobExecution{}, "feed_target_domain"))
	require.True(t, db.Migrator().HasColumn(&models.JobExecution{}, "feed_source_type"))
	require.True(t, db.Migrator().HasColumn(&models.JobExecution{}, "feed_snapshot_retrieved_at"))

	var foreignKeys int
	require.NoError(t, db.Raw("PRAGMA foreign_keys").Row().Scan(&foreignKeys))
	require.Equal(t, 1, foreignKeys)
	var busyTimeout int
	require.NoError(t, db.Raw("PRAGMA busy_timeout").Row().Scan(&busyTimeout))
	require.GreaterOrEqual(t, busyTimeout, defaultSQLiteBusyTimeoutMS)
}

func TestFeedAccessMigrationUpgradesExistingVersionOneSchema(t *testing.T) {
	db := openMigrationTestDB(t, defaultSQLiteBusyTimeoutMS)
	require.NoError(t, db.Exec(`CREATE TABLE job_executions (id INTEGER PRIMARY KEY, job_id INTEGER NOT NULL, status TEXT NOT NULL)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, name TEXT NOT NULL, applied_at DATETIME NOT NULL)`).Error)
	require.NoError(t, db.Create(&SchemaMigration{Version: 1, Name: "baseline-current-model", AppliedAt: time.Now()}).Error)

	require.NoError(t, ApplyMigrations(db))
	for _, column := range []string{
		"feed_http_status", "feed_error_category", "feed_target_domain", "feed_response_time_ms",
		"feed_retry_after", "feed_etag", "feed_last_modified", "feed_cache_control", "feed_expires",
		"feed_age", "feed_response_bytes", "feed_source_type", "feed_cache_status", "feed_freshness", "feed_egress_id",
		"feed_snapshot_retrieved_at",
	} {
		require.True(t, db.Migrator().HasColumn(&models.JobExecution{}, column), "missing upgraded column %s", column)
	}
}

func TestApplyMigrationSetRollsBackSchemaAndVersionOnFailure(t *testing.T) {
	db := openMigrationTestDB(t, defaultSQLiteBusyTimeoutMS)

	err := applyMigrationSet(db, []Migration{{
		Version:     1,
		Name:        "failing-test-migration",
		Description: "test rollback",
		Apply: func(tx *gorm.DB) error {
			if err := tx.Exec("CREATE TABLE partial_migration_table (id INTEGER PRIMARY KEY)").Error; err != nil {
				return err
			}
			return fmt.Errorf("intentional migration failure")
		},
	}})
	require.Error(t, err)
	require.Contains(t, err.Error(), "intentional migration failure")
	require.False(t, db.Migrator().HasTable("partial_migration_table"))
	require.False(t, db.Migrator().HasTable(&SchemaMigration{}))
}

func TestRequireSchemaReadyRejectsCompleteUnversionedSchemaWithoutWriting(t *testing.T) {
	db := openMigrationTestDB(t, defaultSQLiteBusyTimeoutMS)
	require.NoError(t, autoMigrateModels(db))

	err := RequireSchemaReady(db)
	require.ErrorIs(t, err, ErrSchemaNotReady)
	require.Contains(t, err.Error(), "schema_migrations is missing")
	require.False(t, db.Migrator().HasTable(&SchemaMigration{}))
}

func TestForeignKeyConstraintRejectsOrphanEpisode(t *testing.T) {
	db := openMigrationTestDB(t, defaultSQLiteBusyTimeoutMS)
	require.NoError(t, ApplyMigrations(db))

	err := db.Create(&models.Episode{
		PodcastID: 999999,
		Title:     "orphan episode",
	}).Error
	require.Error(t, err)
	require.True(t, strings.Contains(strings.ToLower(err.Error()), "foreign key"), "unexpected error: %v", err)
}

func TestBusyTimeoutAndSingleConnectionAreObservable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "contention.db")
	first := openMigrationTestDBAt(t, path, defaultSQLiteBusyTimeoutMS)
	require.NoError(t, ApplyMigrations(first))
	require.NoError(t, VerifySQLiteSettings(first))

	sqlDB, err := first.DB()
	require.NoError(t, err)
	require.Equal(t, 1, sqlDB.Stats().MaxOpenConnections)
	require.NoError(t, first.Exec("CREATE TABLE contention_test (id INTEGER PRIMARY KEY, value TEXT NOT NULL)").Error)

	second := openMigrationTestDBAt(t, path, 200)
	lock := first.Begin()
	require.NoError(t, lock.Error)
	require.NoError(t, lock.Exec("INSERT INTO contention_test (value) VALUES (?)", "held").Error)

	started := time.Now()
	err = second.Exec("INSERT INTO contention_test (value) VALUES (?)", "waiting").Error
	duration := time.Since(started)
	require.Error(t, err)
	require.GreaterOrEqual(t, duration, 100*time.Millisecond)
	require.NoError(t, lock.Rollback().Error)
}
