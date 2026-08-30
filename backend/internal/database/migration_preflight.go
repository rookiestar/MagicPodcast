package database

import (
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

const MigrationReportVersion = "migration_report.v1"

type PendingMigrationReport struct {
	Version     int               `json:"version"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Contract    MigrationContract `json:"contract"`
}

type MigrationReportResult struct {
	Status        string `json:"status"`
	ApplyEligible bool   `json:"apply_eligible"`
	FailureCode   string `json:"failure_code,omitempty"`
	FailureDetail string `json:"failure_detail,omitempty"`
}

type MigrationReport struct {
	ReportVersion             string                     `json:"report_version"`
	PlanID                    string                     `json:"plan_id"`
	TargetCommit              string                     `json:"target_commit"`
	SourceSchemaVersion       int                        `json:"source_schema_version"`
	TargetSchemaVersion       int                        `json:"target_schema_version"`
	BackupSHA256              string                     `json:"backup_sha256"`
	SourceDatabaseFingerprint string                     `json:"source_database_fingerprint"`
	BackupDatabaseFingerprint string                     `json:"backup_database_fingerprint"`
	PendingMigrations         []PendingMigrationReport   `json:"pending_migrations"`
	SchemaChanges             []SchemaObjectChange       `json:"schema_changes"`
	TableChanges              []TableDataChange          `json:"table_changes"`
	ForeignKeyDependencies    []ForeignKeyEdge           `json:"foreign_key_dependencies"`
	ProtectedTables           []string                   `json:"protected_tables"`
	ProtectedBefore           []TableDataSummary         `json:"protected_before"`
	ProtectedAfter            []TableDataSummary         `json:"protected_after"`
	BackupBaseline            []TableDataSummary         `json:"backup_baseline"`
	Executions                []MigrationExecutionReport `json:"executions"`
	Apply                     *MigrationApplyResult      `json:"apply,omitempty"`
	GeneratedAt               string                     `json:"generated_at"`
	Result                    MigrationReportResult      `json:"result"`
}

type MigrationPreflightOptions struct {
	BackupPath   string
	TargetCommit string
}

var migrationCommitPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

// PreflightProductionMigrations restores a verified backup to an isolated
// SQLite file and runs the same production registry and safety gate there.
func PreflightProductionMigrations(source *gorm.DB, options MigrationPreflightOptions) (MigrationReport, error) {
	return newProductionMigrationRunner().preflight(source, options)
}

// OpenMigrationPreflightSource opens the live source in SQLite read-only mode.
// It never applies runtime WAL or other write-capable connection settings.
func OpenMigrationPreflightSource(path string) (*gorm.DB, func(), error) {
	uri := (&url.URL{Scheme: "file", Path: path}).String()
	dsn := uri + "?mode=ro&_foreign_keys=on&_busy_timeout=5000"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
	if err != nil {
		return nil, nil, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, nil, err
	}
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetMaxOpenConns(1)
	return db, func() { _ = sqlDB.Close() }, nil
}

func (runner productionMigrationRunner) preflight(source *gorm.DB, options MigrationPreflightOptions) (MigrationReport, error) {
	report := MigrationReport{
		ReportVersion: MigrationReportVersion,
		TargetCommit:  strings.ToLower(strings.TrimSpace(options.TargetCommit)),
		GeneratedAt:   runner.now().Format("2006-01-02T15:04:05.000000000Z07:00"),
		Result:        MigrationReportResult{Status: "failed", ApplyEligible: false},
	}
	if source == nil {
		report.Result.FailureCode = "source_database_unavailable"
		report.Result.FailureDetail = "source database is unavailable"
		return report, errors.New(report.Result.FailureDetail)
	}
	if !migrationCommitPattern.MatchString(report.TargetCommit) {
		report.Result.FailureCode = "invalid_target_commit"
		report.Result.FailureDetail = "target commit must be a full lowercase SHA"
		return report, errors.New(report.Result.FailureDetail)
	}

	shadowPath, backupSHA, err := restoreVerifiedMigrationBackup(options.BackupPath)
	if err != nil {
		report.Result.FailureCode = "backup_verification_failed"
		report.Result.FailureDetail = "verified migration backup is required"
		return report, err
	}
	defer removeMigrationShadow(shadowPath)
	report.BackupSHA256 = backupSHA

	shadow, closeShadow, err := openMigrationShadow(shadowPath)
	if err != nil {
		report.Result.FailureCode = "backup_open_failed"
		report.Result.FailureDetail = "verified backup could not be opened as SQLite"
		return report, err
	}
	defer closeShadow()
	if err := verifyMigrationSQLite(shadow); err != nil {
		report.Result.FailureCode = "backup_database_invalid"
		report.Result.FailureDetail = "verified backup failed SQLite integrity or foreign-key checks"
		return report, err
	}

	sourceSnapshot, err := captureMigrationDatabaseSnapshot(source)
	if err != nil {
		report.Result.FailureCode = "source_snapshot_failed"
		report.Result.FailureDetail = "source database fingerprint could not be calculated"
		return report, err
	}
	backupSnapshot, err := captureMigrationDatabaseSnapshot(shadow)
	if err != nil {
		report.Result.FailureCode = "backup_snapshot_failed"
		report.Result.FailureDetail = "backup database fingerprint could not be calculated"
		return report, err
	}
	report.SourceDatabaseFingerprint = migrationDatabaseFingerprint(sourceSnapshot)
	report.BackupDatabaseFingerprint = migrationDatabaseFingerprint(backupSnapshot)
	report.BackupBaseline = migrationSnapshotSummaries(backupSnapshot)
	report.ForeignKeyDependencies = append([]ForeignKeyEdge(nil), backupSnapshot.ForeignKeys...)

	sourceStatus, err := InspectSchema(source)
	if err != nil {
		return report, err
	}
	backupStatus, err := InspectSchema(shadow)
	if err != nil {
		return report, err
	}
	if err := verifyMigrationBackupMetadata(options.BackupPath, report.TargetCommit, backupSHA, backupStatus.CurrentVersion, backupSnapshot); err != nil {
		report.Result.FailureCode = "backup_metadata_invalid"
		report.Result.FailureDetail = "backup metadata does not match the migration input"
		return report, err
	}
	report.SourceSchemaVersion = sourceStatus.CurrentVersion
	report.TargetSchemaVersion = sourceStatus.CurrentVersion
	if sourceStatus.CurrentVersion != backupStatus.CurrentVersion || report.SourceDatabaseFingerprint != report.BackupDatabaseFingerprint {
		report.Result.FailureCode = "source_backup_drift"
		report.Result.FailureDetail = "source database and verified backup do not describe the same migration input"
		return report, errors.New(report.Result.FailureDetail)
	}

	pending := runner.pending(sourceStatus.CurrentVersion)
	for _, migration := range pending {
		report.PendingMigrations = append(report.PendingMigrations, PendingMigrationReport{
			Version: migration.Version, Name: migration.Name,
			Description: migration.Description, Contract: migration.Contract,
		})
		report.TargetSchemaVersion = migration.Version
	}

	executions, migrationErr := runner.run(shadow)
	report.Executions = executions
	afterSnapshot, snapshotErr := captureMigrationDatabaseSnapshot(shadow)
	if snapshotErr != nil {
		return report, snapshotErr
	}
	report.SchemaChanges = migrationSchemaChanges(backupSnapshot, afterSnapshot)
	report.TableChanges = migrationTableChanges(backupSnapshot, afterSnapshot)
	report.ProtectedTables = migrationProtectedTables(backupSnapshot, collectMigrationDDL(executions))
	report.ProtectedBefore = filterMigrationSummaries(backupSnapshot, report.ProtectedTables)
	report.ProtectedAfter = filterMigrationSummaries(afterSnapshot, report.ProtectedTables)

	if migrationErr != nil {
		var gateErr *MigrationGateError
		if errors.As(migrationErr, &gateErr) {
			report.Result.FailureCode = "migration_contract_rejected"
			report.Result.FailureDetail = gateErr.Error()
		} else {
			report.Result.FailureCode = "shadow_migration_failed"
			report.Result.FailureDetail = "shadow migration did not complete"
		}
		report.PlanID = migrationReportPlanID(report)
		return report, migrationErr
	}
	if err := verifyMigrationSQLite(shadow); err != nil {
		report.Result.FailureCode = "shadow_database_invalid"
		report.Result.FailureDetail = "shadow migration failed SQLite integrity or foreign-key checks"
		return report, err
	}
	finalStatus, err := InspectSchema(shadow)
	if err != nil {
		return report, err
	}
	if finalStatus.CurrentVersion != report.TargetSchemaVersion {
		report.Result.FailureCode = "target_schema_mismatch"
		report.Result.FailureDetail = "shadow migration did not reach the target schema"
		return report, errors.New(report.Result.FailureDetail)
	}
	report.Result = MigrationReportResult{Status: "passed", ApplyEligible: true}
	report.PlanID = migrationReportPlanID(report)
	return report, nil
}

func (runner productionMigrationRunner) pending(current int) []Migration {
	pending := make([]Migration, 0)
	for _, migration := range runner.migrations {
		if migration.Version > current {
			pending = append(pending, migration)
		}
	}
	return pending
}

func restoreVerifiedMigrationBackup(backupPath string) (string, string, error) {
	backupPath = strings.TrimSpace(backupPath)
	if backupPath == "" {
		return "", "", errors.New("migration backup is required")
	}
	backup, err := os.Open(backupPath)
	if err != nil {
		return "", "", errors.New("migration backup is unavailable")
	}
	defer backup.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, backup); err != nil {
		return "", "", errors.New("migration backup could not be hashed")
	}
	actualSHA := hex.EncodeToString(hash.Sum(nil))
	sidecar, err := os.ReadFile(backupPath + ".sha256")
	if err != nil {
		return "", "", errors.New("migration backup SHA-256 sidecar is required")
	}
	fields := strings.Fields(string(sidecar))
	if len(fields) == 0 || !strings.EqualFold(fields[0], actualSHA) {
		return "", "", errors.New("migration backup SHA-256 does not match")
	}

	if _, err := backup.Seek(0, io.SeekStart); err != nil {
		return "", "", errors.New("migration backup could not be rewound")
	}
	temporary, err := os.CreateTemp("", "magicpodcast-migration-shadow-*.db")
	if err != nil {
		return "", "", errors.New("migration shadow database could not be created")
	}
	shadowPath := temporary.Name()
	cleanup := func() {
		_ = temporary.Close()
		removeMigrationShadow(shadowPath)
	}
	var reader io.Reader = backup
	var compressed *gzip.Reader
	if strings.HasSuffix(strings.ToLower(backupPath), ".gz") {
		compressed, err = gzip.NewReader(backup)
		if err != nil {
			cleanup()
			return "", "", errors.New("migration backup gzip stream is invalid")
		}
		defer compressed.Close()
		reader = compressed
	}
	if _, err := io.Copy(temporary, reader); err != nil {
		cleanup()
		return "", "", errors.New("migration backup could not be restored to shadow database")
	}
	if err := temporary.Sync(); err != nil {
		cleanup()
		return "", "", errors.New("migration shadow database could not be synchronized")
	}
	if err := temporary.Close(); err != nil {
		removeMigrationShadow(shadowPath)
		return "", "", errors.New("migration shadow database could not be closed")
	}
	return shadowPath, actualSHA, nil
}

func verifyMigrationBackupMetadata(backupPath, targetCommit, backupSHA string, schemaVersion int, snapshot migrationDatabaseSnapshot) error {
	content, err := os.ReadFile(backupPath + ".meta")
	if err != nil {
		return errors.New("migration backup metadata is required")
	}
	metadata := make(map[string]string)
	for _, line := range strings.Split(string(content), "\n") {
		key, value, found := strings.Cut(strings.TrimSpace(line), "=")
		if found && key != "" {
			metadata[key] = value
		}
	}
	for key, want := range map[string]string{
		"source_kind":    "magicpodcast_sqlite",
		"sha256":         strings.ToLower(backupSHA),
		"target_commit":  targetCommit,
		"schema_version": strconv.Itoa(schemaVersion),
	} {
		if strings.ToLower(metadata[key]) != strings.ToLower(want) {
			return fmt.Errorf("backup metadata %s does not match", key)
		}
	}
	countFields := map[string]string{
		"podcasts_count":                 "podcasts",
		"episodes_count":                 "episodes",
		"tags_count":                     "tags",
		"episode_triage_decisions_count": "episode_triage_decisions",
		"episode_completions_count":      "episode_completions",
		"episode_processing_runs_count":  "episode_processing_runs",
		"episode_artifact_sets_count":    "episode_artifact_sets",
		"knowledge_deliveries_count":     "knowledge_deliveries",
		"episode_audio_assets_count":     "episode_audio_assets",
	}
	for field, table := range countFields {
		state, exists := snapshot.Tables[table]
		want := int64(0)
		if exists {
			want = state.Summary.Rows
		}
		if metadata[field] != strconv.FormatInt(want, 10) {
			return fmt.Errorf("backup metadata %s does not match", field)
		}
	}
	queueCounts := map[string]int64{"inbox": 0, "focus": 0, "someday": 0, "done": 0}
	if triage, exists := snapshot.Tables["episode_triage_decisions"]; exists {
		for _, row := range triage.Rows {
			state := strings.TrimPrefix(row.Values["queue_state"], "string:")
			if _, known := queueCounts[state]; known {
				queueCounts[state]++
			}
		}
	}
	for state, count := range queueCounts {
		if metadata["queue_"+state+"_count"] != strconv.FormatInt(count, 10) {
			return fmt.Errorf("backup metadata queue_%s_count does not match", state)
		}
	}
	return nil
}

func openMigrationShadow(path string) (*gorm.DB, func(), error) {
	dsn := path + "?_journal_mode=DELETE&_foreign_keys=on&_busy_timeout=5000"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
	if err != nil {
		return nil, nil, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, nil, err
	}
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetMaxOpenConns(1)
	return db, func() { _ = sqlDB.Close() }, nil
}

func verifyMigrationSQLite(db *gorm.DB) error {
	var integrity string
	if err := db.Raw("PRAGMA integrity_check").Scan(&integrity).Error; err != nil {
		return err
	}
	if integrity != "ok" {
		return errors.New("sqlite integrity check failed")
	}
	type foreignKeyIssue struct {
		Table string
	}
	var issues []foreignKeyIssue
	if err := db.Raw("PRAGMA foreign_key_check").Scan(&issues).Error; err != nil {
		return err
	}
	if len(issues) > 0 {
		return errors.New("sqlite foreign key check failed")
	}
	return nil
}

func collectMigrationDDL(executions []MigrationExecutionReport) []DDLChange {
	var changes []DDLChange
	for _, execution := range executions {
		changes = append(changes, execution.DDL...)
	}
	return changes
}

func filterMigrationSummaries(snapshot migrationDatabaseSnapshot, tables []string) []TableDataSummary {
	result := make([]TableDataSummary, 0, len(tables))
	for _, table := range tables {
		if state, exists := snapshot.Tables[table]; exists {
			result = append(result, state.Summary)
		}
	}
	return result
}

func migrationReportPlanID(report MigrationReport) string {
	binding := report
	binding.PlanID = ""
	binding.GeneratedAt = ""
	binding.Apply = nil
	payload, _ := json.Marshal(binding)
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func WriteMigrationReport(path string, report MigrationReport) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("migration report path is required")
	}
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return errors.New("migration report directory could not be created")
	}
	temporary, err := os.CreateTemp(directory, ".migration-report-*.tmp")
	if err != nil {
		return errors.New("migration report temporary file could not be created")
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	encoder := json.NewEncoder(temporary)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return errors.New("migration report could not be published")
	}
	return nil
}

func ReadMigrationReport(path string) (MigrationReport, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return MigrationReport{}, errors.New("migration report is unavailable")
	}
	var report MigrationReport
	if err := json.Unmarshal(content, &report); err != nil {
		return MigrationReport{}, errors.New("migration report is invalid JSON")
	}
	if report.ReportVersion != MigrationReportVersion || report.PlanID == "" {
		return MigrationReport{}, errors.New("migration report version or plan ID is invalid")
	}
	if report.PlanID != migrationReportPlanID(report) {
		return MigrationReport{}, errors.New("migration report plan binding is invalid")
	}
	return report, nil
}

func removeMigrationShadow(path string) {
	for _, suffix := range []string{"", "-wal", "-shm"} {
		_ = os.Remove(path + suffix)
	}
}

func sortedMigrationProtectedTables() []string {
	tables := make([]string, 0, len(explicitMigrationProtectedTables))
	for table := range explicitMigrationProtectedTables {
		tables = append(tables, table)
	}
	sort.Strings(tables)
	return tables
}
