package database

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"strings"

	"gorm.io/gorm"
)

type MigrationApplyResult struct {
	Status                   string                     `json:"status"`
	DatabaseCommitted        bool                       `json:"database_committed"`
	SchemaVersion            int                        `json:"schema_version"`
	FinalDatabaseFingerprint string                     `json:"final_database_fingerprint,omitempty"`
	Executions               []MigrationExecutionReport `json:"executions,omitempty"`
	QueueCounts              map[string]int64           `json:"queue_counts,omitempty"`
	ProcessingStatusCounts   map[string]int64           `json:"processing_status_counts,omitempty"`
	KeyProjectionReadable    bool                       `json:"key_projection_readable"`
	FailureCode              string                     `json:"failure_code,omitempty"`
	FailureDetail            string                     `json:"failure_detail,omitempty"`
}

type MigrationApplyOptions struct {
	BackupPath   string
	TargetCommit string
}

type migrationApplyInvariantError struct {
	Code   string
	Detail string
	Err    error
}

func (e *migrationApplyInvariantError) Error() string { return e.Err.Error() }
func (e *migrationApplyInvariantError) Unwrap() error { return e.Err }

// ApplyProductionMigrationReport consumes one still-valid passing preflight.
// Drift checks happen before the guarded migration transaction. The returned
// DatabaseCommitted bit is authoritative even when post-commit verification
// later fails and the caller must keep services stopped.
func ApplyProductionMigrationReport(target *gorm.DB, report MigrationReport, options MigrationApplyOptions) (MigrationApplyResult, error) {
	return applyMigrationReportWithRunner(newProductionMigrationRunner(), target, report, options)
}

func applyMigrationReportWithRunner(runner productionMigrationRunner, target *gorm.DB, report MigrationReport, options MigrationApplyOptions) (MigrationApplyResult, error) {
	result := MigrationApplyResult{Status: "rejected", DatabaseCommitted: false}
	fail := func(code, detail string, err error) (MigrationApplyResult, error) {
		result.FailureCode = code
		result.FailureDetail = detail
		return result, err
	}
	if target == nil {
		return fail("target_database_unavailable", "target database is unavailable", errors.New("target database is unavailable"))
	}
	if report.ReportVersion != MigrationReportVersion || report.PlanID == "" || report.PlanID != migrationReportPlanID(report) {
		return fail("migration_plan_invalid", "Migration Report binding is invalid", errors.New("migration report binding is invalid"))
	}
	if !report.Result.ApplyEligible || report.Result.Status != "passed" {
		return fail("migration_plan_not_eligible", "Migration Report did not pass preflight", errors.New("migration report is not apply eligible"))
	}
	targetCommit := strings.ToLower(strings.TrimSpace(options.TargetCommit))
	if targetCommit != report.TargetCommit {
		return fail("target_commit_drift", "target commit changed after preflight", errors.New("target commit drift"))
	}

	shadowPath, backupSHA, err := restoreVerifiedMigrationBackup(options.BackupPath)
	if err != nil {
		return fail("backup_verification_failed", "verified backup is unavailable or changed", err)
	}
	defer removeMigrationShadow(shadowPath)
	if backupSHA != report.BackupSHA256 {
		return fail("backup_digest_drift", "backup SHA-256 changed after preflight", errors.New("backup digest drift"))
	}
	shadow, closeShadow, err := openMigrationShadow(shadowPath)
	if err != nil {
		return fail("backup_open_failed", "verified backup could not be reopened", err)
	}
	defer closeShadow()
	backupSnapshot, err := captureMigrationDatabaseSnapshot(shadow)
	if err != nil {
		return fail("backup_snapshot_failed", "backup fingerprint could not be recalculated", err)
	}
	backupStatus, err := InspectSchema(shadow)
	if err != nil {
		return fail("backup_schema_failed", "backup schema could not be inspected", err)
	}
	if err := verifyMigrationBackupMetadata(options.BackupPath, report.TargetCommit, backupSHA, backupStatus.CurrentVersion, backupSnapshot); err != nil {
		return fail("backup_metadata_drift", "backup metadata changed after preflight", err)
	}
	if migrationDatabaseFingerprint(backupSnapshot) != report.BackupDatabaseFingerprint {
		return fail("backup_fingerprint_drift", "backup database changed after preflight", errors.New("backup database fingerprint drift"))
	}

	before, err := captureMigrationDatabaseSnapshot(target)
	if err != nil {
		return fail("source_snapshot_failed", "target database fingerprint could not be calculated", err)
	}
	status, err := InspectSchema(target)
	if err != nil {
		return fail("source_schema_failed", "target schema could not be inspected", err)
	}
	if status.CurrentVersion == report.TargetSchemaVersion && report.SourceSchemaVersion != report.TargetSchemaVersion && len(runner.pending(status.CurrentVersion)) == 0 {
		if err := verifyMigrationSQLite(target); err != nil ||
			!reflect.DeepEqual(migrationSchemaChanges(backupSnapshot, before), report.SchemaChanges) ||
			!reflect.DeepEqual(migrationTableChanges(backupSnapshot, before), report.TableChanges) ||
			!reflect.DeepEqual(filterMigrationSummaries(before, report.ProtectedTables), report.ProtectedAfter) {
			return fail("already_applied_state_mismatch", "target schema is applied but protected data does not match preflight", errors.New("already-applied state mismatch"))
		}
		queueCounts, processingCounts, err := readMigrationKeyProjections(target)
		if err != nil {
			return fail("already_applied_projection_unreadable", "target schema is applied but key projections are unreadable", err)
		}
		result.Status = "already_applied"
		result.DatabaseCommitted = true
		result.SchemaVersion = status.CurrentVersion
		result.FinalDatabaseFingerprint = migrationDatabaseFingerprint(before)
		result.QueueCounts = queueCounts
		result.ProcessingStatusCounts = processingCounts
		result.KeyProjectionReadable = true
		return result, nil
	}
	if migrationDatabaseFingerprint(before) != report.SourceDatabaseFingerprint {
		return fail("source_database_drift", "target database changed after preflight", errors.New("source database drift"))
	}
	if status.CurrentVersion != report.SourceSchemaVersion {
		return fail("source_schema_drift", "target schema changed after preflight", errors.New("source schema drift"))
	}
	pending := runner.pending(status.CurrentVersion)
	pendingReport := make([]PendingMigrationReport, 0, len(pending))
	for _, migration := range pending {
		pendingReport = append(pendingReport, PendingMigrationReport{
			Version: migration.Version, Name: migration.Name,
			Description: migration.Description, Contract: migration.Contract,
		})
	}
	if !reflect.DeepEqual(pendingReport, report.PendingMigrations) {
		return fail("pending_migration_drift", "pending migrations changed after preflight", errors.New("pending migration drift"))
	}

	executions, err := runner.runWithValidation(target, func(tx *gorm.DB, executions []MigrationExecutionReport) error {
		invariantFailure := func(code, detail string, err error) error {
			return &migrationApplyInvariantError{Code: code, Detail: detail, Err: err}
		}
		if !migrationEvidenceEqual(executions, report.Executions) {
			return invariantFailure("migration_replay_mismatch", "apply DDL or data differences did not match preflight", errors.New("migration replay mismatch"))
		}
		if err := verifyMigrationSQLite(tx); err != nil {
			return invariantFailure("transaction_sqlite_invalid", "migration transaction failed integrity or foreign-key checks", err)
		}
		finalStatus, err := InspectSchema(tx)
		if err != nil {
			return invariantFailure("transaction_schema_unreadable", "migration transaction schema could not be inspected", err)
		}
		if finalStatus.CurrentVersion != report.TargetSchemaVersion {
			return invariantFailure("transaction_schema_mismatch", "migration transaction does not match the Migration Report", errors.New("transaction schema mismatch"))
		}
		after, err := captureMigrationDatabaseSnapshot(tx)
		if err != nil {
			return invariantFailure("transaction_snapshot_failed", "migration transaction fingerprint could not be calculated", err)
		}
		if !reflect.DeepEqual(migrationSchemaChanges(before, after), report.SchemaChanges) ||
			!reflect.DeepEqual(migrationTableChanges(before, after), report.TableChanges) ||
			!reflect.DeepEqual(filterMigrationSummaries(after, report.ProtectedTables), report.ProtectedAfter) {
			return invariantFailure("transaction_report_mismatch", "migration transaction does not match preflight data summaries", errors.New("transaction report mismatch"))
		}
		if _, _, err := readMigrationKeyProjections(tx); err != nil {
			return invariantFailure("transaction_projection_unreadable", "action queue or processing projection is unreadable", err)
		}
		return nil
	})
	result.Executions = executions
	if err != nil {
		var committedErr *MigrationPostCommitError
		if errors.As(err, &committedErr) {
			result.Status = "committed_verification_failed"
			result.DatabaseCommitted = true
			result.FailureCode = "post_commit_connection_safety_failed"
			result.FailureDetail = "migration committed but SQLite connection safety could not be restored"
			return result, err
		}
		var invariantErr *migrationApplyInvariantError
		if errors.As(err, &invariantErr) {
			result.Status = "rolled_back"
			result.FailureCode = invariantErr.Code
			result.FailureDetail = invariantErr.Detail
			return result, err
		}
		result.Status = "rolled_back"
		result.FailureCode = "migration_transaction_rolled_back"
		result.FailureDetail = "migration transaction failed and was rolled back"
		return result, err
	}
	result.DatabaseCommitted = true
	result.Status = "committed_verification_failed"
	postFail := func(code, detail string, err error) (MigrationApplyResult, error) {
		result.FailureCode = code
		result.FailureDetail = detail
		return result, err
	}
	if !migrationEvidenceEqual(executions, report.Executions) {
		return postFail("migration_replay_mismatch", "apply DDL or data differences did not match preflight", errors.New("migration replay mismatch"))
	}
	if err := verifyMigrationSQLite(target); err != nil {
		return postFail("post_commit_sqlite_invalid", "committed database failed integrity or foreign-key checks", err)
	}
	finalStatus, err := InspectSchema(target)
	if err != nil {
		return postFail("post_commit_schema_unreadable", "committed schema could not be inspected", err)
	}
	result.SchemaVersion = finalStatus.CurrentVersion
	if finalStatus.CurrentVersion != report.TargetSchemaVersion {
		return postFail("post_commit_schema_mismatch", "committed schema does not match the Migration Report", errors.New("post-commit schema mismatch"))
	}
	after, err := captureMigrationDatabaseSnapshot(target)
	if err != nil {
		return postFail("post_commit_snapshot_failed", "committed database fingerprint could not be calculated", err)
	}
	result.FinalDatabaseFingerprint = migrationDatabaseFingerprint(after)
	if !reflect.DeepEqual(migrationSchemaChanges(before, after), report.SchemaChanges) ||
		!reflect.DeepEqual(migrationTableChanges(before, after), report.TableChanges) ||
		!reflect.DeepEqual(filterMigrationSummaries(after, report.ProtectedTables), report.ProtectedAfter) {
		return postFail("post_commit_report_mismatch", "committed database does not match preflight data summaries", errors.New("post-commit report mismatch"))
	}

	queueCounts, processingCounts, err := readMigrationKeyProjections(target)
	if err != nil {
		return postFail("post_commit_projection_unreadable", "action queue or processing projection is unreadable", err)
	}
	result.QueueCounts = queueCounts
	result.ProcessingStatusCounts = processingCounts
	result.KeyProjectionReadable = true
	result.Status = "committed"
	result.FailureCode = ""
	result.FailureDetail = ""
	return result, nil
}

func migrationEvidenceEqual(left, right any) bool {
	leftJSON, leftErr := json.Marshal(left)
	rightJSON, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftJSON, rightJSON)
}

func readMigrationKeyProjections(db *gorm.DB) (map[string]int64, map[string]int64, error) {
	queueCounts := map[string]int64{"inbox": 0, "focus": 0, "someday": 0, "done": 0}
	type countRow struct {
		State string
		Count int64
	}
	var queues []countRow
	if err := db.Raw(`
		SELECT queue_state AS state, COUNT(*) AS count
		FROM episode_triage_decisions
		WHERE queue_state IN ('inbox', 'focus', 'someday', 'done')
		GROUP BY queue_state
	`).Scan(&queues).Error; err != nil {
		return nil, nil, err
	}
	for _, row := range queues {
		queueCounts[row.State] = row.Count
	}
	processingCounts := make(map[string]int64)
	var processing []countRow
	if err := db.Raw(`
		SELECT status AS state, COUNT(*) AS count
		FROM episode_processing_runs
		GROUP BY status
	`).Scan(&processing).Error; err != nil {
		return nil, nil, err
	}
	for _, row := range processing {
		processingCounts[row.State] = row.Count
	}
	return queueCounts, processingCounts, nil
}
