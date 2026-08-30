package database

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"magicpodcast/internal/models"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const schema24FixtureSourceCommit = "d3e5b81bf193cd1448fe83ed193576f66a5a206a"

func openSchema24MigrationFixture(t *testing.T) *gorm.DB {
	t.Helper()
	fixtureSQL, err := os.ReadFile(filepath.Join("testdata", "schema24_fixture.sql"))
	require.NoError(t, err)
	require.Contains(t, string(fixtureSQL), schema24FixtureSourceCommit)
	require.NotContains(t, string(fixtureSQL), "https://")
	require.NotContains(t, string(fixtureSQL), "http://")
	require.NotContains(t, string(fixtureSQL), "/Users/")

	path := filepath.Join(t.TempDir(), "schema24-fixture.db")
	dsn := path + "?_foreign_keys=on&_busy_timeout=5000"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.Exec(string(fixtureSQL)).Error)
	require.NoError(t, db.Exec("PRAGMA foreign_keys = ON").Error)
	return db
}

func TestSchema24FixtureIsHistoricalSanitizedAndComplete(t *testing.T) {
	db := openSchema24MigrationFixture(t)
	status, err := InspectSchema(db)
	require.NoError(t, err)
	require.Equal(t, 24, status.CurrentVersion)
	require.Equal(t, []string{"25:native-minutes-artifact-integrity"}, migrationNames(status.Pending))

	for table, want := range map[string]int64{
		"podcasts":                  1,
		"podcasts_tags":             2,
		"episodes":                  13,
		"episodes_tags":             4,
		"episode_triage_decisions":  13,
		"consumption_queue_orders":  4,
		"episode_completions":       3,
		"episode_processing_runs":   1,
		"processing_checkpoints":    1,
		"episode_artifact_sets":     1,
		"knowledge_deliveries":      1,
		"episode_audio_assets":      1,
		"processing_schedule_runs":  1,
		"processing_schedule_items": 1,
	} {
		var count int64
		require.NoError(t, db.Table(table).Count(&count).Error, table)
		require.Equal(t, want, count, table)
	}

	var queueStates []string
	require.NoError(t, db.Table("episode_triage_decisions").Distinct("queue_state").Order("queue_state").Pluck("queue_state", &queueStates).Error)
	require.Equal(t, []string{"done", "focus", "inbox", "someday"}, queueStates)

	snapshot, err := captureMigrationDatabaseSnapshot(db)
	require.NoError(t, err)
	require.Contains(t, snapshot.ForeignKeys, ForeignKeyEdge{Parent: "episodes", Child: "episode_triage_decisions"})
	require.Contains(t, snapshot.ForeignKeys, ForeignKeyEdge{Parent: "episodes", Child: "episode_processing_runs"})
	require.Contains(t, snapshot.ForeignKeys, ForeignKeyEdge{Parent: "episode_processing_runs", Child: "processing_checkpoints"})
	require.Contains(t, snapshot.ForeignKeys, ForeignKeyEdge{Parent: "episode_artifact_sets", Child: "knowledge_deliveries"})
	require.Contains(t, explicitMigrationProtectedTables, "processing_schedule_items")
}

func TestMigrationSnapshotPreservesDuplicateRowsWithoutPrimaryKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "duplicate-rows.db")
	db, err := gorm.Open(sqlite.Open(path+"?_foreign_keys=on"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })

	require.NoError(t, db.Exec("CREATE TABLE duplicate_rows (value TEXT NOT NULL)").Error)
	require.NoError(t, db.Exec("INSERT INTO duplicate_rows (value) VALUES (?)", "same").Error)
	require.NoError(t, db.Exec("INSERT INTO duplicate_rows (value) VALUES (?)", "same").Error)
	require.NoError(t, db.Exec("CREATE VIRTUAL TABLE duplicate_search USING fts4(title, show_notes)").Error)
	require.NoError(t, db.Exec("INSERT INTO duplicate_search (title, show_notes) VALUES (?, ?)", "same", "same").Error)
	require.NoError(t, db.Exec("INSERT INTO duplicate_search (title, show_notes) VALUES (?, ?)", "same", "same").Error)

	snapshot, err := captureMigrationDatabaseSnapshot(db)
	require.NoError(t, err)
	for _, tableName := range []string{"duplicate_rows", "duplicate_search"} {
		table, ok := snapshot.Tables[tableName]
		require.True(t, ok, tableName)
		require.Equal(t, int64(2), table.Summary.Rows, tableName)
		require.Len(t, table.Rows, 2, tableName)
	}
}

func TestMigrationProtectionFollowsAllForeignKeyDescendants(t *testing.T) {
	snapshot := migrationDatabaseSnapshot{
		Tables: map[string]migrationTableSnapshot{
			"episodes": {}, "future_episode_child": {}, "future_grandchild": {},
		},
		ForeignKeys: []ForeignKeyEdge{
			{Parent: "episodes", Child: "future_episode_child"},
			{Parent: "future_episode_child", Child: "future_grandchild"},
		},
	}
	protected := migrationProtectedTables(snapshot, []DDLChange{{Operation: SchemaChangeRebuildTable, Table: "episodes"}})
	require.Contains(t, protected, "future_episode_child")
	require.Contains(t, protected, "future_grandchild")
}

func TestProductionMigrationRunnerPreservesSchema24ProtectedDataAndIsIdempotent(t *testing.T) {
	db := openSchema24MigrationFixture(t)
	before, err := captureMigrationDatabaseSnapshot(db)
	require.NoError(t, err)

	reports, err := newProductionMigrationRunner().run(db)
	require.NoError(t, err)
	require.Len(t, reports, 1)
	require.Equal(t, 25, reports[0].Version)
	require.Empty(t, reports[0].Violations)
	require.Equal(t, []DDLChange{
		{Operation: SchemaChangeAddColumn, Table: "episode_artifact_sets", Object: "audio_sha256"},
		{Operation: SchemaChangeAddColumn, Table: "episode_artifact_sets", Object: "minutes_summary_sha256"},
		{Operation: SchemaChangeAddColumn, Table: "episode_artifact_sets", Object: "transcript_timeline_sha256"},
	}, reports[0].DDL)

	after, err := captureMigrationDatabaseSnapshot(db)
	require.NoError(t, err)
	for table := range explicitMigrationProtectedTables {
		oldTable, hadOld := before.Tables[table]
		newTable, hasNew := after.Tables[table]
		if !hadOld {
			continue
		}
		require.True(t, hasNew, table)
		identitiesKept, contentKept := migrationExistingRowsKept(oldTable, newTable, nil)
		require.True(t, identitiesKept, table)
		require.True(t, contentKept, table)
		require.Equal(t, oldTable.Summary.Rows, newTable.Summary.Rows, table)
	}
	require.Equal(t, int64(13), after.Tables["episode_triage_decisions"].Summary.Rows)
	require.Equal(t, CurrentSchemaVersion, mustSchemaStatus(t, db).CurrentVersion)
	assertMigrationDatabaseHealthy(t, db)

	stableFingerprint := migrationDatabaseFingerprint(after)
	reports, err = newProductionMigrationRunner().run(db)
	require.NoError(t, err)
	require.Empty(t, reports)
	idempotent, err := captureMigrationDatabaseSnapshot(db)
	require.NoError(t, err)
	require.Equal(t, stableFingerprint, migrationDatabaseFingerprint(idempotent))
	var versionRows int64
	require.NoError(t, db.Model(&SchemaMigration{}).Where("version = ?", 25).Count(&versionRows).Error)
	require.Equal(t, int64(1), versionRows)
}

func TestProductionMigrationRunnerRejectsEpisodeRebuildAndRollsBack(t *testing.T) {
	db := openSchema24MigrationFixture(t)
	dangerous := dangerousEpisodeRebuildMigration()

	reports, err := newMigrationRunner([]Migration{dangerous}).run(db)
	require.Error(t, err)
	require.Len(t, reports, 1)
	var gateErr *MigrationGateError
	require.True(t, errors.As(err, &gateErr))
	require.Contains(t, err.Error(), "episodes")
	require.True(t, strings.Contains(err.Error(), "undeclared") || strings.Contains(err.Error(), "13->0"), err.Error())
	require.Contains(t, gateErr.Report.Violations, MigrationViolation{
		Code: "protected_data_decreased", Table: "episode_triage_decisions", Operation: DataChangeDelete,
		Detail: "table episode_triage_decisions protected rows decreased 13->0",
	})

	require.Equal(t, 24, mustSchemaStatus(t, db).CurrentVersion)
	var triageCount int64
	require.NoError(t, db.Table("episode_triage_decisions").Count(&triageCount).Error)
	require.Equal(t, int64(13), triageCount)
	require.False(t, db.Migrator().HasColumn(&models.EpisodeArtifactSet{}, "audio_sha256"))
	assertMigrationDatabaseHealthy(t, db)
}

func TestMigrationPreflightFailureReportCapturesDestructiveDDLAndDataLoss(t *testing.T) {
	source := openSchema24MigrationFixture(t)
	targetCommit := strings.Repeat("d", 40)
	backup := createVerifiedMigrationBackup(t, source, targetCommit)
	report, err := newMigrationRunner([]Migration{dangerousEpisodeRebuildMigration()}).preflight(source, MigrationPreflightOptions{
		BackupPath: backup, TargetCommit: targetCommit,
	})
	require.Error(t, err)
	require.False(t, report.Result.ApplyEligible)
	require.Equal(t, "migration_contract_rejected", report.Result.FailureCode)
	require.Len(t, report.Executions, 1)
	require.Contains(t, report.Executions[0].Violations, MigrationViolation{
		Code: "protected_data_decreased", Table: "episode_triage_decisions", Operation: DataChangeDelete,
		Detail: "table episode_triage_decisions protected rows decreased 13->0",
	})
	require.Contains(t, report.Executions[0].DDL, DDLChange{Operation: SchemaChangeDropTable, Table: "episodes"})
	require.Equal(t, 24, mustSchemaStatus(t, source).CurrentVersion)
	var triageCount int64
	require.NoError(t, source.Table("episode_triage_decisions").Count(&triageCount).Error)
	require.Equal(t, int64(13), triageCount)
}

func TestMigrationPreflightRejectsUnrecognizedColumnDropOnEmptyTable(t *testing.T) {
	source := openSchema24MigrationFixture(t)
	require.NoError(t, source.Exec("CREATE TABLE empty_guard (id INTEGER PRIMARY KEY, protected_note TEXT)").Error)
	targetCommit := strings.Repeat("6", 40)
	backup := createVerifiedMigrationBackup(t, source, targetCommit)
	migration := migrationRegistry()[24]
	migration.Apply = func(tx *gorm.DB) error {
		if err := applyNativeMinutesArtifactIntegrityMigration(tx); err != nil {
			return err
		}
		return tx.Exec("ALTER TABLE empty_guard DROP COLUMN protected_note").Error
	}

	report, err := newMigrationRunner([]Migration{migration}).preflight(source, MigrationPreflightOptions{
		BackupPath: backup, TargetCommit: targetCommit,
	})
	require.Error(t, err)
	require.False(t, report.Result.ApplyEligible)
	require.Equal(t, "migration_contract_rejected", report.Result.FailureCode)
	require.Contains(t, report.Executions[0].DDL, DDLChange{Operation: SchemaChangeUnsupported})
	require.Contains(t, report.Executions[0].Violations, MigrationViolation{
		Code: "undeclared_schema_diff", Table: "empty_guard", Operation: "drop_column",
		Detail: "migration produced undeclared drop_column for column protected_note",
	})
	require.True(t, source.Migrator().HasColumn("empty_guard", "protected_note"))
}

func TestMigrationPreflightRejectsDeclaredSchemaChangeThatDidNotHappen(t *testing.T) {
	source := openSchema24MigrationFixture(t)
	targetCommit := strings.Repeat("7", 40)
	backup := createVerifiedMigrationBackup(t, source, targetCommit)
	migration := migrationRegistry()[24]
	migration.Apply = func(*gorm.DB) error { return nil }

	report, err := newMigrationRunner([]Migration{migration}).preflight(source, MigrationPreflightOptions{
		BackupPath: backup, TargetCommit: targetCommit,
	})
	require.Error(t, err)
	require.False(t, report.Result.ApplyEligible)
	require.Equal(t, "migration_contract_rejected", report.Result.FailureCode)
	require.Contains(t, report.Executions[0].Violations, MigrationViolation{
		Code: "declared_schema_change_missing", Table: "episode_artifact_sets", Operation: SchemaChangeAddColumn,
		Detail: "migration did not perform declared add_column on table episode_artifact_sets",
	})
	require.Equal(t, 24, mustSchemaStatus(t, source).CurrentVersion)
}

func dangerousEpisodeRebuildMigration() Migration {
	dangerous := migrationRegistry()[24]
	dangerous.Apply = func(tx *gorm.DB) error {
		if err := applyNativeMinutesArtifactIntegrityMigration(tx); err != nil {
			return err
		}
		// Reproduce the incident class: a migration that declares only an
		// artifact-table change silently rebuilds the Episode parent table.
		// DROP TABLE performs the same ON DELETE CASCADE that erased the
		// episode-owned rows in production.
		for _, statement := range []string{
			"CREATE TABLE episodes__temp AS SELECT * FROM episodes",
			"DROP TABLE episodes",
			"ALTER TABLE episodes__temp RENAME TO episodes",
		} {
			if err := tx.Exec(statement).Error; err != nil {
				return err
			}
		}
		return nil
	}
	return dangerous
}

func TestProductionMigrationPreflightBuildsBoundSanitizedReportWithoutWritingSource(t *testing.T) {
	source := openSchema24MigrationFixture(t)
	require.NoError(t, source.Exec(`
		UPDATE podcasts
		SET title = 'SENSITIVE_FIXTURE_TITLE',
		    notes = 'SENSITIVE_FIXTURE_NOTE',
		    feed_url = 'https://sensitive.invalid/feed'
		WHERE id = 1
	`).Error)
	targetCommit := strings.Repeat("a", 40)
	backup := createVerifiedMigrationBackup(t, source, targetCommit)

	report, err := PreflightProductionMigrations(source, MigrationPreflightOptions{
		BackupPath: backup, TargetCommit: targetCommit,
	})
	require.NoError(t, err)
	require.Equal(t, MigrationReportVersion, report.ReportVersion)
	require.Equal(t, targetCommit, report.TargetCommit)
	require.Equal(t, 24, report.SourceSchemaVersion)
	require.Equal(t, 25, report.TargetSchemaVersion)
	require.True(t, report.Result.ApplyEligible)
	require.Equal(t, "passed", report.Result.Status)
	require.Len(t, report.PendingMigrations, 1)
	require.Equal(t, "native-minutes-artifact-integrity", report.PendingMigrations[0].Name)
	require.Len(t, report.Executions, 1)
	require.Contains(t, report.ForeignKeyDependencies, ForeignKeyEdge{Parent: "episodes", Child: "episode_triage_decisions"})
	require.Contains(t, report.ProtectedTables, "episode_triage_decisions")
	require.Equal(t, int64(13), migrationSummaryByTable(t, report.ProtectedBefore, "episode_triage_decisions").Rows)
	require.Equal(t, int64(13), migrationSummaryByTable(t, report.ProtectedAfter, "episode_triage_decisions").Rows)
	require.Contains(t, report.SchemaChanges, SchemaObjectChange{
		Operation: SchemaChangeAddColumn, Type: "column", Table: "episode_artifact_sets", Object: "audio_sha256",
		AfterSHA: migrationSchemaChangeByObject(t, report.SchemaChanges, "audio_sha256").AfterSHA,
	})

	encoded, err := jsonMarshalMigrationReport(report)
	require.NoError(t, err)
	for _, forbidden := range []string{
		"SENSITIVE_FIXTURE_TITLE", "SENSITIVE_FIXTURE_NOTE", "sensitive.invalid",
		backup, filepath.Dir(backup), "fixture-note-05", "Fixture Episode 05",
	} {
		require.NotContains(t, encoded, forbidden)
	}

	reportPath := filepath.Join(t.TempDir(), "migration-report.json")
	require.NoError(t, WriteMigrationReport(reportPath, report))
	stored, err := ReadMigrationReport(reportPath)
	require.NoError(t, err)
	require.Equal(t, report.PlanID, stored.PlanID)

	status, err := InspectSchema(source)
	require.NoError(t, err)
	require.Equal(t, 24, status.CurrentVersion)
	require.False(t, source.Migrator().HasColumn(&models.EpisodeArtifactSet{}, "audio_sha256"))

	repeated, err := PreflightProductionMigrations(source, MigrationPreflightOptions{
		BackupPath: backup, TargetCommit: targetCommit,
	})
	require.NoError(t, err)
	require.Equal(t, report.PlanID, repeated.PlanID)
	require.True(t, reflect.DeepEqual(report.SchemaChanges, repeated.SchemaChanges))
	require.True(t, reflect.DeepEqual(report.TableChanges, repeated.TableChanges))
	require.True(t, repeated.Result.ApplyEligible)
}

func TestMigrationPreflightSourceConnectionIsReadOnly(t *testing.T) {
	source := openSchema24MigrationFixture(t)
	path := migrationFixtureDatabasePath(t, source)
	readOnly, closeReadOnly, err := OpenMigrationPreflightSource(path)
	require.NoError(t, err)
	defer closeReadOnly()
	require.Error(t, readOnly.Exec("UPDATE episodes SET notes = 'must-not-write' WHERE id = 1").Error)
	var notes string
	require.NoError(t, source.Table("episodes").Select("notes").Where("id = 1").Scan(&notes).Error)
	require.Equal(t, "fixture-note-01", notes)
}

func TestMigrationPreflightAllowsDeclaredBackfillAndNormalizationOnly(t *testing.T) {
	source := openSchema24MigrationFixture(t)
	targetCommit := strings.Repeat("b", 40)
	backup := createVerifiedMigrationBackup(t, source, targetCommit)
	migration := migrationRegistry()[24]
	migration.Contract.DataChanges = []DataChangeRule{
		{
			Operation: DataChangeBackfill, Table: "episode_triage_decisions",
			Columns: []string{"read_at"}, MaxRows: 1,
			Condition: DataChangeCondition{Type: DataConditionNonEmpty},
		},
		{
			Operation: DataChangeNormalize, Table: "episodes",
			Columns: []string{"video_availability"}, MaxRows: 1,
			Condition: DataChangeCondition{Type: DataConditionAllowedValues, Values: []string{"unknown", "unavailable", "available"}},
		},
	}
	migration.Apply = func(tx *gorm.DB) error {
		if err := applyNativeMinutesArtifactIntegrityMigration(tx); err != nil {
			return err
		}
		if err := tx.Exec("UPDATE episode_triage_decisions SET read_at = ? WHERE id = 1", "2026-08-30 09:00:00+00:00").Error; err != nil {
			return err
		}
		return tx.Exec("UPDATE episodes SET video_availability = 'unknown' WHERE id = 1").Error
	}
	report, err := newMigrationRunner([]Migration{migration}).preflight(source, MigrationPreflightOptions{
		BackupPath: backup, TargetCommit: targetCommit,
	})
	require.NoError(t, err)
	require.True(t, report.Result.ApplyEligible)
	require.Empty(t, report.Executions[0].Violations)

	unexpected := migration
	unexpected.Apply = func(tx *gorm.DB) error {
		if err := migration.Apply(tx); err != nil {
			return err
		}
		return tx.Exec("UPDATE episodes SET notes = 'undeclared-change' WHERE id = 2").Error
	}
	failed, err := newMigrationRunner([]Migration{unexpected}).preflight(source, MigrationPreflightOptions{
		BackupPath: backup, TargetCommit: targetCommit,
	})
	require.Error(t, err)
	require.False(t, failed.Result.ApplyEligible)
	require.Equal(t, "migration_contract_rejected", failed.Result.FailureCode)
	require.Contains(t, failed.Executions[0].Violations, MigrationViolation{
		Code: "undeclared_data_update", Table: "episodes", Operation: "update",
		Detail: "table episodes changed 1 protected value(s)",
	})
}

func TestMigrationPreflightEnforcesBackfillDirectionAndDeleteCondition(t *testing.T) {
	for _, testCase := range []struct {
		name      string
		prepare   func(*testing.T, *gorm.DB)
		contract  DataChangeRule
		statement string
	}{
		{
			name: "backfill cannot overwrite existing content",
			contract: DataChangeRule{
				Operation: DataChangeBackfill, Table: "episodes", Columns: []string{"notes"}, MaxRows: 1,
				Condition: DataChangeCondition{Type: DataConditionNonEmpty},
			},
			statement: "UPDATE episodes SET notes = 'replacement' WHERE id = 1",
		},
		{
			name: "delete must satisfy row condition",
			prepare: func(t *testing.T, db *gorm.DB) {
				require.NoError(t, db.Exec("CREATE TABLE cleanup_items (id INTEGER PRIMARY KEY, kind TEXT NOT NULL)").Error)
				require.NoError(t, db.Exec("INSERT INTO cleanup_items (id, kind) VALUES (1, 'blocked')").Error)
			},
			contract: DataChangeRule{
				Operation: DataChangeDelete, Table: "cleanup_items", Columns: []string{"kind"}, MaxRows: 1,
				Condition: DataChangeCondition{Type: DataConditionAllowedValues, Values: []string{"allowed"}},
			},
			statement: "DELETE FROM cleanup_items WHERE id = 1",
		},
		{
			name: "insert must satisfy row condition",
			prepare: func(t *testing.T, db *gorm.DB) {
				require.NoError(t, db.Exec("CREATE TABLE cleanup_items (id INTEGER PRIMARY KEY, kind TEXT NOT NULL)").Error)
			},
			contract: DataChangeRule{
				Operation: DataChangeInsert, Table: "cleanup_items", Columns: []string{"kind"}, MaxRows: 1,
				Condition: DataChangeCondition{Type: DataConditionAllowedValues, Values: []string{"allowed"}},
			},
			statement: "INSERT INTO cleanup_items (id, kind) VALUES (1, 'blocked')",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			source := openSchema24MigrationFixture(t)
			if testCase.prepare != nil {
				testCase.prepare(t, source)
			}
			targetCommit := strings.Repeat("8", 40)
			backup := createVerifiedMigrationBackup(t, source, targetCommit)
			migration := migrationRegistry()[24]
			migration.Contract.DataChanges = []DataChangeRule{testCase.contract}
			migration.Apply = func(tx *gorm.DB) error {
				if err := applyNativeMinutesArtifactIntegrityMigration(tx); err != nil {
					return err
				}
				return tx.Exec(testCase.statement).Error
			}
			report, err := newMigrationRunner([]Migration{migration}).preflight(source, MigrationPreflightOptions{
				BackupPath: backup, TargetCommit: targetCommit,
			})
			require.Error(t, err)
			require.False(t, report.Result.ApplyEligible)
			require.Equal(t, "migration_contract_rejected", report.Result.FailureCode)
			require.NotEmpty(t, report.Executions[0].Violations)
			require.Equal(t, 24, mustSchemaStatus(t, source).CurrentVersion)
		})
	}
}

func TestMigrationPreflightRejectsBackupOrSourceDrift(t *testing.T) {
	source := openSchema24MigrationFixture(t)
	targetCommit := strings.Repeat("c", 40)
	backup := createVerifiedMigrationBackup(t, source, targetCommit)
	require.NoError(t, source.Exec("UPDATE episodes SET notes = 'post-backup-write' WHERE id = 1").Error)
	report, err := PreflightProductionMigrations(source, MigrationPreflightOptions{
		BackupPath: backup, TargetCommit: targetCommit,
	})
	require.Error(t, err)
	require.Equal(t, "source_backup_drift", report.Result.FailureCode)
	require.False(t, report.Result.ApplyEligible)

	require.NoError(t, os.WriteFile(backup+".sha256", []byte(strings.Repeat("0", 64)+"  fixture.db\n"), 0o600))
	report, err = PreflightProductionMigrations(source, MigrationPreflightOptions{
		BackupPath: backup, TargetCommit: targetCommit,
	})
	require.Error(t, err)
	require.Equal(t, "backup_verification_failed", report.Result.FailureCode)
}

func TestAuthorizedBackupDestructiveMigrationDrill(t *testing.T) {
	backup := strings.TrimSpace(os.Getenv("MAGICPODCAST_AUTHORIZED_DRILL_BACKUP"))
	if backup == "" {
		t.Skip("requires separately authorized production-backup drill")
	}
	targetCommit := strings.TrimSpace(os.Getenv("MAGICPODCAST_AUTHORIZED_DRILL_TARGET_COMMIT"))
	shadowPath, backupSHA, err := restoreVerifiedMigrationBackup(backup)
	require.NoError(t, err)
	defer removeMigrationShadow(shadowPath)
	source, closeSource, err := openMigrationShadow(shadowPath)
	require.NoError(t, err)
	defer closeSource()
	status, err := InspectSchema(source)
	require.NoError(t, err)
	var triageCountBefore int64
	require.NoError(t, source.Table("episode_triage_decisions").Count(&triageCountBefore).Error)
	dangerous := dangerousEpisodeRebuildMigration()
	dangerous.Version = status.CurrentVersion + 1
	dangerous.Name = "authorized-drill-episode-parent-rebuild"

	executions, err := newMigrationRunner([]Migration{dangerous}).run(source)
	require.Error(t, err)
	require.NotEmpty(t, executions)
	require.NotEmpty(t, executions[0].Violations)
	report := MigrationReport{
		ReportVersion:       MigrationReportVersion,
		TargetCommit:        targetCommit,
		SourceSchemaVersion: status.CurrentVersion,
		TargetSchemaVersion: dangerous.Version,
		BackupSHA256:        backupSHA,
		Executions:          executions,
		Result: MigrationReportResult{
			Status:        "failed",
			ApplyEligible: false,
			FailureCode:   "migration_contract_rejected",
			FailureDetail: err.Error(),
		},
	}
	report.PlanID = migrationReportPlanID(report)
	finalStatus, statusErr := InspectSchema(source)
	require.NoError(t, statusErr)
	require.Equal(t, status.CurrentVersion, finalStatus.CurrentVersion)
	var triageCountAfter int64
	require.NoError(t, source.Table("episode_triage_decisions").Count(&triageCountAfter).Error)
	require.Equal(t, triageCountBefore, triageCountAfter)
	if output := strings.TrimSpace(os.Getenv("MAGICPODCAST_AUTHORIZED_DRILL_REPORT")); output != "" {
		require.NoError(t, WriteMigrationReport(output, report))
	}
	t.Logf("destructive_drill=blocked plan_id=%s source_schema=%d violation_count=%d", report.PlanID, report.SourceSchemaVersion, len(report.Executions[0].Violations))
}

func TestDestructiveDrillRemainsRedAtCurrentSchema(t *testing.T) {
	source := openSchema24MigrationFixture(t)
	_, err := newProductionMigrationRunner().run(source)
	require.NoError(t, err)
	targetCommit := strings.Repeat("4", 40)
	backup := createVerifiedMigrationBackup(t, source, targetCommit)
	dangerous := dangerousEpisodeRebuildMigration()
	dangerous.Version = CurrentSchemaVersion + 1
	dangerous.Name = "test-current-schema-episode-parent-rebuild"
	report, err := newMigrationRunner([]Migration{dangerous}).preflight(source, MigrationPreflightOptions{
		BackupPath: backup, TargetCommit: targetCommit,
	})
	require.Error(t, err)
	require.False(t, report.Result.ApplyEligible)
	require.Equal(t, CurrentSchemaVersion, report.SourceSchemaVersion)
	require.Contains(t, report.Executions[0].Violations, MigrationViolation{
		Code: "protected_data_decreased", Table: "episode_triage_decisions", Operation: DataChangeDelete,
		Detail: "table episode_triage_decisions protected rows decreased 13->0",
	})
}

func TestApplyProductionMigrationReportCommitsMatchingPlanAndProjections(t *testing.T) {
	target := openSchema24MigrationFixture(t)
	targetCommit := strings.Repeat("e", 40)
	backup := createVerifiedMigrationBackup(t, target, targetCommit)
	report, err := PreflightProductionMigrations(target, MigrationPreflightOptions{BackupPath: backup, TargetCommit: targetCommit})
	require.NoError(t, err)
	preflightPath := filepath.Join(t.TempDir(), "preflight-migration-report.json")
	require.NoError(t, WriteMigrationReport(preflightPath, report))
	storedPlan, err := ReadMigrationReport(preflightPath)
	require.NoError(t, err)

	result, err := ApplyProductionMigrationReport(target, storedPlan, MigrationApplyOptions{BackupPath: backup, TargetCommit: targetCommit})
	require.NoError(t, err)
	require.Equal(t, "committed", result.Status)
	require.True(t, result.DatabaseCommitted)
	require.Equal(t, 25, result.SchemaVersion)
	require.True(t, result.KeyProjectionReadable)
	require.Equal(t, map[string]int64{"inbox": 4, "focus": 3, "someday": 3, "done": 3}, result.QueueCounts)
	require.Equal(t, int64(1), result.ProcessingStatusCounts[models.ProcessingRunStatusCompleted])
	require.Equal(t, 25, mustSchemaStatus(t, target).CurrentVersion)
	assertMigrationDatabaseHealthy(t, target)
	storedPlan.Apply = &result
	reportPath := filepath.Join(t.TempDir(), "applied-migration-report.json")
	require.NoError(t, WriteMigrationReport(reportPath, storedPlan))
	stored, err := ReadMigrationReport(reportPath)
	require.NoError(t, err)
	require.NotNil(t, stored.Apply)
	require.True(t, stored.Apply.DatabaseCommitted)
	require.Equal(t, "committed", stored.Apply.Status)

	repeated, err := ApplyProductionMigrationReport(target, storedPlan, MigrationApplyOptions{BackupPath: backup, TargetCommit: targetCommit})
	require.NoError(t, err)
	require.Equal(t, "already_applied", repeated.Status)
	require.True(t, repeated.DatabaseCommitted)
	require.Equal(t, 25, mustSchemaStatus(t, target).CurrentVersion)
}

func TestApplyProductionMigrationReportRejectsEveryPlanDriftBeforeWriting(t *testing.T) {
	for _, testCase := range []struct {
		name        string
		mutate      func(*testing.T, *gorm.DB, *MigrationReport, string)
		commit      string
		failureCode string
	}{
		{
			name: "source database",
			mutate: func(t *testing.T, db *gorm.DB, _ *MigrationReport, _ string) {
				require.NoError(t, db.Exec("UPDATE episodes SET notes = 'drift' WHERE id = 1").Error)
			},
			failureCode: "source_database_drift",
		},
		{
			name:   "target commit",
			mutate: func(_ *testing.T, _ *gorm.DB, _ *MigrationReport, _ string) {},
			commit: strings.Repeat("f", 40), failureCode: "target_commit_drift",
		},
		{
			name: "pending migrations",
			mutate: func(_ *testing.T, _ *gorm.DB, report *MigrationReport, _ string) {
				report.PendingMigrations = nil
				report.PlanID = migrationReportPlanID(*report)
			},
			failureCode: "pending_migration_drift",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			target := openSchema24MigrationFixture(t)
			targetCommit := strings.Repeat("1", 40)
			backup := createVerifiedMigrationBackup(t, target, targetCommit)
			report, err := PreflightProductionMigrations(target, MigrationPreflightOptions{BackupPath: backup, TargetCommit: targetCommit})
			require.NoError(t, err)
			testCase.mutate(t, target, &report, backup)
			commit := testCase.commit
			if commit == "" {
				commit = targetCommit
			}
			result, err := ApplyProductionMigrationReport(target, report, MigrationApplyOptions{BackupPath: backup, TargetCommit: commit})
			require.Error(t, err)
			require.False(t, result.DatabaseCommitted)
			require.Equal(t, testCase.failureCode, result.FailureCode)
			require.Equal(t, 24, mustSchemaStatus(t, target).CurrentVersion)
		})
	}
}

func TestApplyMigrationReportRollsBackInterruptedTransaction(t *testing.T) {
	target := openSchema24MigrationFixture(t)
	targetCommit := strings.Repeat("2", 40)
	backup := createVerifiedMigrationBackup(t, target, targetCommit)
	migration := migrationRegistry()[24]
	applyCalls := 0
	migration.Apply = func(tx *gorm.DB) error {
		applyCalls++
		if err := applyNativeMinutesArtifactIntegrityMigration(tx); err != nil {
			return err
		}
		if err := tx.Exec("CREATE TABLE interrupted_migration_write (id INTEGER PRIMARY KEY)").Error; err != nil {
			return err
		}
		if applyCalls == 1 {
			return nil
		}
		return errors.New("injected transaction interruption")
	}
	migration.Contract.SchemaChanges = append(migration.Contract.SchemaChanges,
		SchemaChangeRule{Operation: SchemaChangeCreateTable, Table: "interrupted_migration_write"})
	runner := newMigrationRunner([]Migration{migration})
	report, err := runner.preflight(target, MigrationPreflightOptions{BackupPath: backup, TargetCommit: targetCommit})
	require.NoError(t, err)
	result, err := applyMigrationReportWithRunner(runner, target, report, MigrationApplyOptions{BackupPath: backup, TargetCommit: targetCommit})
	require.Error(t, err)
	require.Equal(t, "rolled_back", result.Status)
	require.False(t, result.DatabaseCommitted)
	require.Equal(t, 24, mustSchemaStatus(t, target).CurrentVersion)
	require.False(t, target.Migrator().HasTable("interrupted_migration_write"))
	require.False(t, target.Migrator().HasColumn(&models.EpisodeArtifactSet{}, "audio_sha256"))
}

func TestApplyMigrationReportRollsBackReplayMismatchBeforeCommit(t *testing.T) {
	target := openSchema24MigrationFixture(t)
	targetCommit := strings.Repeat("3", 40)
	backup := createVerifiedMigrationBackup(t, target, targetCommit)
	migration := migrationRegistry()[24]
	migration.Contract.DataChanges = []DataChangeRule{{
		Operation: DataChangeNormalize, Table: "episodes", Columns: []string{"notes"}, MaxRows: 1,
		Condition: DataChangeCondition{Type: DataConditionAny},
	}}
	applyCalls := 0
	migration.Apply = func(tx *gorm.DB) error {
		applyCalls++
		if err := applyNativeMinutesArtifactIntegrityMigration(tx); err != nil {
			return err
		}
		if applyCalls > 1 {
			return tx.Exec("UPDATE episodes SET notes = 'apply-only-change' WHERE id = 1").Error
		}
		return nil
	}
	runner := newMigrationRunner([]Migration{migration})
	report, err := runner.preflight(target, MigrationPreflightOptions{BackupPath: backup, TargetCommit: targetCommit})
	require.NoError(t, err)

	result, err := applyMigrationReportWithRunner(runner, target, report, MigrationApplyOptions{BackupPath: backup, TargetCommit: targetCommit})
	require.Error(t, err)
	require.False(t, result.DatabaseCommitted)
	require.Equal(t, "rolled_back", result.Status)
	require.Equal(t, "migration_replay_mismatch", result.FailureCode)
	require.Equal(t, 24, mustSchemaStatus(t, target).CurrentVersion)
}

func TestApplyMigrationReportDoesNotMisreportPostCommitConnectionFailureAsRollback(t *testing.T) {
	target := openSchema24MigrationFixture(t)
	targetCommit := strings.Repeat("9", 40)
	backup := createVerifiedMigrationBackup(t, target, targetCommit)
	migration := migrationRegistry()[24]
	migration.RequiresForeignKeysDisabled = true
	runner := newMigrationRunner([]Migration{migration})
	restoreCalls := 0
	runner.restoreForeignKeys = func(db *gorm.DB) error {
		restoreCalls++
		require.NoError(t, db.Exec("PRAGMA foreign_keys = ON").Error)
		if restoreCalls > 1 {
			return errors.New("injected post-commit foreign-key restore failure")
		}
		return nil
	}
	report, err := runner.preflight(target, MigrationPreflightOptions{BackupPath: backup, TargetCommit: targetCommit})
	require.NoError(t, err)

	result, err := applyMigrationReportWithRunner(runner, target, report, MigrationApplyOptions{
		BackupPath: backup, TargetCommit: targetCommit,
	})
	require.Error(t, err)
	require.True(t, result.DatabaseCommitted)
	require.Equal(t, "committed_verification_failed", result.Status)
	require.Equal(t, "post_commit_connection_safety_failed", result.FailureCode)
	require.Equal(t, 25, mustSchemaStatus(t, target).CurrentVersion)
}

func createVerifiedMigrationBackup(t *testing.T, db *gorm.DB, targetCommit string) string {
	t.Helper()
	sourcePath := migrationFixtureDatabasePath(t, db)
	content, err := os.ReadFile(sourcePath)
	require.NoError(t, err)
	backup := filepath.Join(t.TempDir(), "fixture-backup.db")
	require.NoError(t, os.WriteFile(backup, content, 0o600))
	sum := sha256.Sum256(content)
	require.NoError(t, os.WriteFile(backup+".sha256", []byte(fmt.Sprintf("%x  fixture-backup.db\n", sum)), 0o600))
	snapshot, err := captureMigrationDatabaseSnapshot(db)
	require.NoError(t, err)
	status, err := InspectSchema(db)
	require.NoError(t, err)
	count := func(table string) int64 {
		if state, exists := snapshot.Tables[table]; exists {
			return state.Summary.Rows
		}
		return 0
	}
	queueCounts := map[string]int64{"inbox": 0, "focus": 0, "someday": 0, "done": 0}
	for _, row := range snapshot.Tables["episode_triage_decisions"].Rows {
		state := strings.TrimPrefix(row.Values["queue_state"], "string:")
		if _, exists := queueCounts[state]; exists {
			queueCounts[state]++
		}
	}
	metadata := fmt.Sprintf(`source_kind=magicpodcast_sqlite
sha256=%x
schema_version=%d
target_commit=%s
podcasts_count=%d
episodes_count=%d
tags_count=%d
episode_triage_decisions_count=%d
episode_completions_count=%d
episode_processing_runs_count=%d
episode_artifact_sets_count=%d
knowledge_deliveries_count=%d
episode_audio_assets_count=%d
queue_inbox_count=%d
queue_focus_count=%d
queue_someday_count=%d
queue_done_count=%d
`, sum, status.CurrentVersion, targetCommit,
		count("podcasts"), count("episodes"), count("tags"), count("episode_triage_decisions"),
		count("episode_completions"), count("episode_processing_runs"), count("episode_artifact_sets"),
		count("knowledge_deliveries"), count("episode_audio_assets"),
		queueCounts["inbox"], queueCounts["focus"], queueCounts["someday"], queueCounts["done"])
	require.NoError(t, os.WriteFile(backup+".meta", []byte(metadata), 0o600))
	return backup
}

func migrationFixtureDatabasePath(t *testing.T, db *gorm.DB) string {
	t.Helper()
	type databaseRow struct {
		Name string
		File string
	}
	var databases []databaseRow
	require.NoError(t, db.Raw("PRAGMA database_list").Scan(&databases).Error)
	var sourcePath string
	for _, database := range databases {
		if database.Name == "main" {
			sourcePath = database.File
			break
		}
	}
	require.NotEmpty(t, sourcePath)
	return sourcePath
}

func migrationSummaryByTable(t *testing.T, summaries []TableDataSummary, table string) TableDataSummary {
	t.Helper()
	for _, summary := range summaries {
		if summary.Table == table {
			return summary
		}
	}
	t.Fatalf("missing migration summary for table %s", table)
	return TableDataSummary{}
}

func migrationSchemaChangeByObject(t *testing.T, changes []SchemaObjectChange, object string) SchemaObjectChange {
	t.Helper()
	for _, change := range changes {
		if change.Object == object {
			return change
		}
	}
	t.Fatalf("missing migration schema change for object %s", object)
	return SchemaObjectChange{}
}

func jsonMarshalMigrationReport(report MigrationReport) (string, error) {
	encoded, err := json.Marshal(report)
	return string(encoded), err
}

func assertMigrationDatabaseHealthy(t *testing.T, db *gorm.DB) {
	t.Helper()
	var integrity string
	require.NoError(t, db.Raw("PRAGMA integrity_check").Scan(&integrity).Error)
	require.Equal(t, "ok", integrity)
	type foreignKeyIssue struct {
		Table string
	}
	var issues []foreignKeyIssue
	require.NoError(t, db.Raw("PRAGMA foreign_key_check").Scan(&issues).Error)
	require.Empty(t, issues)
}
