package database

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"magicpodcast/internal/feed"
	"magicpodcast/internal/logger"
	"magicpodcast/internal/models"

	"gorm.io/gorm"
)

const CurrentSchemaVersion = 25

var ErrSchemaNotReady = errors.New("database schema is not ready")

// SchemaMigration is the durable record of an explicitly applied schema
// change. It deliberately contains metadata only; the recovery source is the
// verified backup recorded by the migration command.
type SchemaMigration struct {
	Version   int       `gorm:"primaryKey"`
	Name      string    `gorm:"size:200;not null"`
	AppliedAt time.Time `gorm:"not null"`
}

func (SchemaMigration) TableName() string { return "schema_migrations" }

type Migration struct {
	Version                     int
	Name                        string
	Description                 string
	Apply                       func(*gorm.DB) error
	RequiresForeignKeysDisabled bool
	Contract                    MigrationContract
}

type SchemaStatus struct {
	MigrationTablePresent bool
	CurrentVersion        int
	RequiredTablesMissing []string
	Pending               []Migration
}

func migrationRegistry() []Migration {
	return []Migration{
		{
			Version:     1,
			Name:        "baseline-current-model",
			Description: "Create the current model tables and indexes, or record an existing complete schema as the baseline.",
			Apply:       applyBaselineMigration,
		},
		{
			Version:     2,
			Name:        "feed-access-observability",
			Description: "Persist bounded Feed access status, timing, source, freshness, and whitelisted response metadata.",
			Apply:       applyFeedAccessObservabilityMigration,
		},
		{
			Version:     3,
			Name:        "feed-snapshot-retrieved-at",
			Description: "Persist the retrieval time of the content used by a Feed execution, including last-good snapshots.",
			Apply:       applyFeedSnapshotRetrievedAtMigration,
		},
		{
			Version:     4,
			Name:        "feed-circuit-state",
			Description: "Persist whether a Feed execution was opened, skipped by, or probing a domain circuit.",
			Apply:       applyFeedCircuitStateMigration,
		},
		{
			Version:     5,
			Name:        "feed-source-verification",
			Description: "Persist the selected Feed source URL and PodcastIndex identity verification result.",
			Apply:       applyFeedSourceVerificationMigration,
		},
		{
			Version:     6,
			Name:        "scheduler-run-history",
			Description: "Create the scheduler run history table used for consecutive-failure observation.",
			Apply:       applySchedulerRunHistoryMigration,
		},
		{
			Version:     7,
			Name:        "feed-snapshots-last-good",
			Description: "Create the bounded feed_snapshots table used to persist last-good Feed snapshots for restart recovery.",
			Apply:       applyFeedSnapshotsMigration,
		},
		{
			Version:     8,
			Name:        "podcast-alternative-feeds",
			Description: "Cache pre-verified alternative Feed URLs keyed by podcast, main feed, and stable identity (#37).",
			Apply:       applyPodcastAlternativeFeedsMigration,
		},
		{
			Version:     9,
			Name:        "job-feed-attempts",
			Description: "Append-only safe Feed attempt metadata per Job for causal history (#39).",
			Apply:       applyJobFeedAttemptsMigration,
		},
		{
			Version:     10,
			Name:        "job-compensation-links",
			Description: "Bidirectional links between partial Jobs and compensation retry Jobs (#40).",
			Apply:       applyJobCompensationLinksMigration,
		},
		{
			Version:     11,
			Name:        "job-execution-failure-phase",
			Description: "Persist Feed failure_phase on JobExecution final projection for attempt history (#39).",
			Apply:       applyJobExecutionFailurePhaseMigration,
		},
		{
			Version:     12,
			Name:        "single-active-workflow-job",
			Description: "Enforce one pending/running/finalizing Job per workflow with a partial unique index (#38).",
			Apply:       applySingleActiveWorkflowJobMigration,
		},
		{
			Version:     13,
			Name:        "feed-user-agent-gates",
			Description: "Persist domain and User-Agent fingerprint blocks across Jobs and restarts (#48).",
			Apply:       applyFeedUserAgentGatesMigration,
		},
		{
			Version:     14,
			Name:        "feed-user-agent-recovery",
			Description: "Add audited human probe approval and distinct-Feed gradual User-Agent recovery state (#49).",
			Apply:       applyFeedUserAgentRecoveryMigration,
		},
		{
			Version:     15,
			Name:        "episode-triage-decisions",
			Description: "Persist one idempotent pending, shortlisted, or discarded decision per library episode (#55).",
			Apply:       applyEpisodeTriageDecisionsMigration,
		},
		{
			Version:     16,
			Name:        "homepage-workflow-reports",
			Description: "Add workflow homepage publish config and structured report episodes for discovery (#89/#90).",
			Apply:       applyHomepageWorkflowReportsMigration,
		},
		{
			Version:     17,
			Name:        "episode-consumption-state",
			Description: "Expand episode triage into one cross-day consumption state for Inbox, queues, reading, and in-progress intent (#101/#102).",
			Apply:       applyEpisodeConsumptionStateMigration,
		},
		{
			Version:     18,
			Name:        "consumption-queue-order",
			Description: "Persist independent queue positions and revisions for precise Inbox ordering (#157).",
			Apply:       applyConsumptionQueueOrderMigration,
		},
		{
			Version:     19,
			Name:        "episode-completion-facts",
			Description: "Persist one durable completion fact per episode and backfill only provable current Done state (#168/#169).",
			Apply:       applyEpisodeCompletionFactsMigration,
		},
		{
			Version:     20,
			Name:        "episode-processing-foundation",
			Description: "Persist episode processing runs, checkpoints, immutable artifact sets, and independent knowledge deliveries (#179).",
			Apply:       applyEpisodeProcessingFoundationMigration,
		},
		{
			Version:     21,
			Name:        "managed-episode-audio-assets",
			Description: "Persist managed episode-audio preparation state without retaining source URLs or exposing local paths (#181).",
			Apply:       applyEpisodeAudioAssetMigration,
		},
		{
			Version:     22,
			Name:        "focus-processing-schedule-history",
			Description: "Persist idempotent Focus schedule triggers and candidate outcomes without reusing Feed workflow scheduling (#182).",
			Apply:       applyFocusProcessingScheduleMigration,
		},
		{
			Version:     23,
			Name:        "episode-video-availability",
			Description: "Persist Xiaoyuzhou episode video tri-state on episodes without storing signed HLS (#199).",
			Apply:       applyEpisodeVideoAvailabilityMigration,
		},
		{
			Version:                     24,
			Name:                        "episode-video-availability-check",
			Description:                 "Constrain persisted Xiaoyuzhou episode video tri-state values (#199).",
			Apply:                       applyEpisodeVideoAvailabilityConstraintMigration,
			RequiresForeignKeysDisabled: true,
		},
		{
			Version:     25,
			Name:        "native-minutes-artifact-integrity",
			Description: "Add forward-compatible audio, Minutes summary, and transcript timeline integrity fields to immutable artifact sets (#206).",
			Apply:       applyNativeMinutesArtifactIntegrityMigration,
			Contract: MigrationContract{
				SchemaChanges: []SchemaChangeRule{
					{Operation: SchemaChangeAddColumn, Table: "episode_artifact_sets", Object: "audio_sha256"},
					{Operation: SchemaChangeAddColumn, Table: "episode_artifact_sets", Object: "minutes_summary_sha256"},
					{Operation: SchemaChangeAddColumn, Table: "episode_artifact_sets", Object: "transcript_timeline_sha256"},
				},
			},
		},
	}
}

var baselineRequiredTables = []string{
	"tags",
	"workflows",
	"sync_configs",
	"podcasts",
	"episodes",
	"jobs",
	"job_executions",
	"reports",
	"podcasts_tags",
	"episodes_tags",
}

var requiredTables = append(append([]string(nil), baselineRequiredTables...), feed.FeedSnapshotsTableName, "podcast_alternative_feeds", "job_feed_attempts", feed.FeedUserAgentGatesTableName, feed.FeedUserAgentGateAuditsTableName, feed.FeedUserAgentGateRecoveryFeedsTableName, "episode_triage_decisions", "consumption_queue_orders", "episode_completions", "episode_processing_runs", "processing_checkpoints", "episode_artifact_sets", "knowledge_deliveries", "episode_audio_assets", "processing_schedule_runs", "processing_schedule_items")

func InspectSchema(db *gorm.DB) (SchemaStatus, error) {
	if db == nil {
		return SchemaStatus{}, fmt.Errorf("%w: database is nil", ErrSchemaNotReady)
	}

	status := SchemaStatus{
		MigrationTablePresent: db.Migrator().HasTable(&SchemaMigration{}),
	}
	if status.MigrationTablePresent {
		if err := db.Model(&SchemaMigration{}).Select("COALESCE(MAX(version), 0)").Scan(&status.CurrentVersion).Error; err != nil {
			return SchemaStatus{}, fmt.Errorf("read schema version: %w", err)
		}
	}
	for _, table := range requiredTables {
		if !db.Migrator().HasTable(table) {
			status.RequiredTablesMissing = append(status.RequiredTablesMissing, table)
		}
	}
	for _, migration := range migrationRegistry() {
		if migration.Version > status.CurrentVersion {
			status.Pending = append(status.Pending, migration)
		}
	}
	sort.Slice(status.Pending, func(i, j int) bool { return status.Pending[i].Version < status.Pending[j].Version })
	return status, nil
}

// RequireSchemaReady is called by the normal API startup path. It is read-only
// and fails closed when an explicit migration has not been applied.
func RequireSchemaReady(db *gorm.DB) error {
	status, err := InspectSchema(db)
	if err != nil {
		return err
	}
	if !status.MigrationTablePresent {
		return fmt.Errorf("%w: schema_migrations is missing; run scripts/migrate-db.sh --dry-run then --apply", ErrSchemaNotReady)
	}
	if len(status.RequiredTablesMissing) > 0 {
		return fmt.Errorf("%w: missing required tables: %v", ErrSchemaNotReady, status.RequiredTablesMissing)
	}
	if status.CurrentVersion != CurrentSchemaVersion || len(status.Pending) > 0 {
		return fmt.Errorf("%w: current=%d expected=%d pending=%v", ErrSchemaNotReady, status.CurrentVersion, CurrentSchemaVersion, migrationNames(status.Pending))
	}
	if err := VerifySQLiteSettings(db); err != nil {
		return fmt.Errorf("%w: %v", ErrSchemaNotReady, err)
	}
	return nil
}

func migrationNames(migrations []Migration) []string {
	names := make([]string, 0, len(migrations))
	for _, migration := range migrations {
		names = append(names, fmt.Sprintf("%d:%s", migration.Version, migration.Name))
	}
	return names
}

// ApplyMigrations is the low-level registry executor used by isolated test and
// bootstrap paths. The production command must use
// ApplyProductionMigrationReport, which requires a bound preflight plan and
// adds the business-data safety contract around this transaction.
func ApplyMigrations(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("database is nil")
	}
	return applyMigrationSet(db, migrationRegistry())
}

func applyMigrationSet(db *gorm.DB, migrations []Migration) error {
	sort.Slice(migrations, func(i, j int) bool { return migrations[i].Version < migrations[j].Version })
	apply := func(rawTx *gorm.DB) error {
		tx := rawTx.Session(&gorm.Session{NewDB: true})
		if err := tx.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
            version INTEGER PRIMARY KEY,
            name TEXT NOT NULL,
            applied_at DATETIME NOT NULL
        )`).Error; err != nil {
			return fmt.Errorf("create schema_migrations: %w", err)
		}

		current, err := currentSchemaVersion(tx)
		if err != nil {
			return err
		}
		for _, migration := range migrations {
			if migration.Version <= current {
				continue
			}
			logger.Infof("🔄 Applying database migration %d: %s", migration.Version, migration.Name)
			if err := migration.Apply(tx); err != nil {
				return fmt.Errorf("migration %d (%s) failed: %w", migration.Version, migration.Name, err)
			}
			if err := tx.Create(&SchemaMigration{
				Version:   migration.Version,
				Name:      migration.Name,
				AppliedAt: time.Now().UTC(),
			}).Error; err != nil {
				return fmt.Errorf("record migration %d (%s): %w", migration.Version, migration.Name, err)
			}
			current = migration.Version
		}
		return nil
	}

	needsForeignKeysDisabled, err := migrationSetNeedsForeignKeysDisabled(db, migrations)
	if err != nil {
		return err
	}
	if !needsForeignKeysDisabled {
		return db.Transaction(apply)
	}

	// SQLite cannot toggle foreign_keys inside an active transaction. Pin one
	// connection, disable enforcement before BEGIN, and restore it after the
	// atomic migration transaction. This is required by migrations that rebuild
	// a referenced table; otherwise DROP TABLE would cascade into child rows.
	return db.Connection(func(conn *gorm.DB) error {
		var foreignKeys int
		if err := conn.Raw("PRAGMA foreign_keys").Row().Scan(&foreignKeys); err != nil {
			return fmt.Errorf("read sqlite foreign_keys pragma: %w", err)
		}
		if foreignKeys != 0 && foreignKeys != 1 {
			return fmt.Errorf("sqlite foreign_keys pragma is %d, want 0 or 1 before migration", foreignKeys)
		}
		if foreignKeys == 1 {
			if err := conn.Exec("PRAGMA foreign_keys = OFF").Error; err != nil {
				return fmt.Errorf("disable sqlite foreign_keys for migration: %w", err)
			}
		}
		migrationErr := conn.Transaction(apply)
		var restoreErr error
		if foreignKeys == 1 {
			restoreErr = conn.Exec("PRAGMA foreign_keys = ON").Error
		}
		if migrationErr != nil {
			return migrationErr
		}
		if restoreErr != nil {
			return fmt.Errorf("restore sqlite foreign_keys after migration: %w", restoreErr)
		}
		return nil
	})
}

func migrationSetNeedsForeignKeysDisabled(db *gorm.DB, migrations []Migration) (bool, error) {
	needs := false
	for _, migration := range migrations {
		if migration.RequiresForeignKeysDisabled {
			needs = true
			break
		}
	}
	if !needs || !db.Migrator().HasTable(&SchemaMigration{}) {
		return needs, nil
	}
	current, err := currentSchemaVersion(db)
	if err != nil {
		return false, err
	}
	for _, migration := range migrations {
		if migration.RequiresForeignKeysDisabled && migration.Version > current {
			return true, nil
		}
	}
	return false, nil
}

func currentSchemaVersion(db *gorm.DB) (int, error) {
	var current int
	if err := db.Session(&gorm.Session{NewDB: true}).Model(&SchemaMigration{}).
		Select("COALESCE(MAX(version), 0)").Scan(&current).Error; err != nil {
		return 0, fmt.Errorf("read current schema version: %w", err)
	}
	return current, nil
}

func applyBaselineMigration(db *gorm.DB) error {
	if len(requiredTablesMissingFrom(db, baselineRequiredTables)) == len(baselineRequiredTables) {
		if err := autoMigrateModels(db); err != nil {
			return err
		}
	}
	missing := requiredTablesMissingFrom(db, baselineRequiredTables)
	if len(missing) > 0 {
		return fmt.Errorf("existing schema is incomplete; missing tables: %v", missing)
	}
	return CreateIndexes(db)
}

func applyFeedAccessObservabilityMigration(db *gorm.DB) error {
	columns := []struct {
		name string
		ddl  string
	}{
		{name: "feed_http_status", ddl: "INTEGER"},
		{name: "feed_error_category", ddl: "TEXT NOT NULL DEFAULT 'not_observed'"},
		{name: "feed_target_domain", ddl: "TEXT NOT NULL DEFAULT ''"},
		{name: "feed_response_time_ms", ddl: "INTEGER NOT NULL DEFAULT 0"},
		{name: "feed_retry_after", ddl: "TEXT NOT NULL DEFAULT ''"},
		{name: "feed_etag", ddl: "TEXT NOT NULL DEFAULT ''"},
		{name: "feed_last_modified", ddl: "TEXT NOT NULL DEFAULT ''"},
		{name: "feed_cache_control", ddl: "TEXT NOT NULL DEFAULT ''"},
		{name: "feed_expires", ddl: "TEXT NOT NULL DEFAULT ''"},
		{name: "feed_age", ddl: "TEXT NOT NULL DEFAULT ''"},
		{name: "feed_response_bytes", ddl: "INTEGER NOT NULL DEFAULT 0"},
		{name: "feed_source_type", ddl: "TEXT NOT NULL DEFAULT 'unknown'"},
		{name: "feed_cache_status", ddl: "TEXT NOT NULL DEFAULT 'not_used'"},
		{name: "feed_freshness", ddl: "TEXT NOT NULL DEFAULT 'unknown'"},
		{name: "feed_egress_id", ddl: "TEXT NOT NULL DEFAULT 'unknown'"},
	}
	for _, column := range columns {
		if db.Migrator().HasColumn(&models.JobExecution{}, column.name) {
			continue
		}
		if err := db.Exec("ALTER TABLE job_executions ADD COLUMN " + column.name + " " + column.ddl).Error; err != nil {
			return fmt.Errorf("add job_executions.%s: %w", column.name, err)
		}
	}
	return nil
}

func applyFeedSnapshotRetrievedAtMigration(db *gorm.DB) error {
	if db.Migrator().HasColumn(&models.JobExecution{}, "feed_snapshot_retrieved_at") {
		return nil
	}
	if err := db.Exec("ALTER TABLE job_executions ADD COLUMN feed_snapshot_retrieved_at DATETIME").Error; err != nil {
		return fmt.Errorf("add job_executions.feed_snapshot_retrieved_at: %w", err)
	}
	return nil
}

func applyFeedCircuitStateMigration(db *gorm.DB) error {
	if db.Migrator().HasColumn(&models.JobExecution{}, "feed_circuit_state") {
		return nil
	}
	if err := db.Exec("ALTER TABLE job_executions ADD COLUMN feed_circuit_state TEXT NOT NULL DEFAULT 'not_used'").Error; err != nil {
		return fmt.Errorf("add job_executions.feed_circuit_state: %w", err)
	}
	return nil
}

func applyFeedSourceVerificationMigration(db *gorm.DB) error {
	columns := []struct {
		name string
		ddl  string
	}{
		{name: "feed_source_url", ddl: "TEXT NOT NULL DEFAULT ''"},
		{name: "feed_identity_verification", ddl: "TEXT NOT NULL DEFAULT 'not_checked'"},
	}
	for _, column := range columns {
		if db.Migrator().HasColumn(&models.JobExecution{}, column.name) {
			continue
		}
		if err := db.Exec("ALTER TABLE job_executions ADD COLUMN " + column.name + " " + column.ddl).Error; err != nil {
			return fmt.Errorf("add job_executions.%s: %w", column.name, err)
		}
	}
	return nil
}

func applySchedulerRunHistoryMigration(db *gorm.DB) error {
	if err := db.AutoMigrate(&models.SchedulerRun{}); err != nil {
		return fmt.Errorf("create scheduler_runs: %w", err)
	}
	return nil
}

// applyFeedSnapshotsMigration creates the durable last-good snapshot table.
// The DDL lives in the feed package so the store and the migration cannot drift.
// It is idempotent: a pre-existing table is left untouched.
func applyFeedSnapshotsMigration(db *gorm.DB) error {
	if db.Migrator().HasTable(feed.FeedSnapshotsTableName) {
		return nil
	}
	if err := db.Exec(feed.FeedSnapshotsCreateTableSQL).Error; err != nil {
		return fmt.Errorf("create %s table: %w", feed.FeedSnapshotsTableName, err)
	}
	if err := db.Exec(feed.FeedSnapshotsCreateIndexSQL).Error; err != nil {
		return fmt.Errorf("create %s eviction index: %w", feed.FeedSnapshotsTableName, err)
	}
	return nil
}

// applyPodcastAlternativeFeedsMigration creates the alternative-Feed verification
// cache used by #37. AutoMigrate is sufficient: the table holds only bounded
// metadata (URLs, identity keys, verification labels) — never Feed bodies.
func applyPodcastAlternativeFeedsMigration(db *gorm.DB) error {
	if err := db.AutoMigrate(&models.PodcastAlternativeFeed{}); err != nil {
		return fmt.Errorf("create podcast_alternative_feeds: %w", err)
	}
	return nil
}

func applyJobFeedAttemptsMigration(db *gorm.DB) error {
	if err := db.AutoMigrate(&models.JobFeedAttempt{}); err != nil {
		return fmt.Errorf("create job_feed_attempts: %w", err)
	}
	return nil
}

func applyJobCompensationLinksMigration(db *gorm.DB) error {
	if !db.Migrator().HasTable(&models.Job{}) {
		// Incomplete fixtures (e.g. partial upgrade tests without jobs) skip
		// column adds; a full baseline always creates jobs first.
		return nil
	}
	columns := []struct {
		name string
		ddl  string
	}{
		{name: "compensation_of_job_id", ddl: "INTEGER"},
		{name: "compensated_by_job_id", ddl: "INTEGER"},
	}
	for _, column := range columns {
		if db.Migrator().HasColumn(&models.Job{}, column.name) {
			continue
		}
		if err := db.Exec("ALTER TABLE jobs ADD COLUMN " + column.name + " " + column.ddl).Error; err != nil {
			return fmt.Errorf("add jobs.%s: %w", column.name, err)
		}
	}
	return nil
}

func applyJobExecutionFailurePhaseMigration(db *gorm.DB) error {
	if !db.Migrator().HasTable(&models.JobExecution{}) {
		return nil
	}
	if db.Migrator().HasColumn(&models.JobExecution{}, "feed_failure_phase") {
		return nil
	}
	if err := db.Exec("ALTER TABLE job_executions ADD COLUMN feed_failure_phase TEXT NOT NULL DEFAULT ''").Error; err != nil {
		return fmt.Errorf("add job_executions.feed_failure_phase: %w", err)
	}
	return nil
}

func applySingleActiveWorkflowJobMigration(db *gorm.DB) error {
	if !db.Migrator().HasTable(&models.Job{}) {
		return nil
	}
	if err := db.Exec(models.ActiveJobUniqueIndexSQL).Error; err != nil {
		return fmt.Errorf("create single-active workflow job index: %w", err)
	}
	return nil
}

func applyFeedUserAgentGatesMigration(db *gorm.DB) error {
	if err := db.Exec(feed.FeedUserAgentGatesCreateTableSQLV13).Error; err != nil {
		return fmt.Errorf("create %s table: %w", feed.FeedUserAgentGatesTableName, err)
	}
	if err := db.Exec(feed.FeedUserAgentGatesCreateIndexSQL).Error; err != nil {
		return fmt.Errorf("create %s state index: %w", feed.FeedUserAgentGatesTableName, err)
	}
	return nil
}

func applyFeedUserAgentRecoveryMigration(db *gorm.DB) error {
	if err := addColumnIfMissing(db, &models.JobExecution{}, "feed_user_agent_gate_state", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := addColumnIfMissing(db, &models.JobExecution{}, "feed_user_agent_probe_result", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := addColumnIfMissing(db, &models.JobExecution{}, "feed_user_agent_approved_by", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := addColumnIfMissing(db, &models.JobExecution{}, "feed_user_agent_approved_at", "DATETIME"); err != nil {
		return err
	}
	if err := addColumnIfMissing(db, &models.JobExecution{}, "feed_user_agent_last_probe_at", "DATETIME"); err != nil {
		return err
	}
	if err := addColumnIfMissing(db, &models.JobFeedAttempt{}, "feed_user_agent_gate_state", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := addColumnIfMissing(db, &models.JobFeedAttempt{}, "feed_user_agent_probe_result", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := addColumnIfMissing(db, &models.JobFeedAttempt{}, "feed_user_agent_approved_by", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := addColumnIfMissing(db, &models.JobFeedAttempt{}, "feed_user_agent_approved_at", "DATETIME"); err != nil {
		return err
	}
	if err := addColumnIfMissing(db, &models.JobFeedAttempt{}, "feed_user_agent_last_probe_at", "DATETIME"); err != nil {
		return err
	}

	for _, column := range []struct {
		name string
		ddl  string
	}{
		{name: "approved_by", ddl: "TEXT NOT NULL DEFAULT ''"},
		{name: "approved_at", ddl: "INTEGER"},
		{name: "last_probe_at", ddl: "INTEGER"},
	} {
		if db.Migrator().HasColumn(feed.FeedUserAgentGatesTableName, column.name) {
			continue
		}
		if err := db.Exec("ALTER TABLE " + feed.FeedUserAgentGatesTableName + " ADD COLUMN " + column.name + " " + column.ddl).Error; err != nil {
			return fmt.Errorf("add %s.%s: %w", feed.FeedUserAgentGatesTableName, column.name, err)
		}
	}

	for _, statement := range []string{
		feed.FeedUserAgentGateAuditsCreateTableSQL,
		feed.FeedUserAgentGateAuditsCreateIndexSQL,
		feed.FeedUserAgentGateRecoveryFeedsCreateTableSQL,
		feed.FeedUserAgentGateRecoveryFeedsCreateIndexSQL,
	} {
		if err := db.Exec(statement).Error; err != nil {
			return fmt.Errorf("apply User-Agent recovery schema: %w", err)
		}
	}
	return nil
}

func applyEpisodeTriageDecisionsMigration(db *gorm.DB) error {
	if err := db.AutoMigrate(&models.EpisodeTriageDecision{}); err != nil {
		return fmt.Errorf("create episode triage decisions: %w", err)
	}
	return nil
}

func applyHomepageWorkflowReportsMigration(db *gorm.DB) error {
	// AutoMigrate adds nullable-safe columns with defaults for existing rows.
	if err := db.AutoMigrate(&models.Workflow{}, &models.Report{}); err != nil {
		return fmt.Errorf("apply homepage workflow report columns: %w", err)
	}
	return nil
}

func applyEpisodeConsumptionStateMigration(db *gorm.DB) error {
	if err := db.AutoMigrate(&models.EpisodeTriageDecision{}); err != nil {
		return fmt.Errorf("expand episode consumption state: %w", err)
	}

	// Backfill only legacy rows that have not already been expanded. The
	// original decided_at remains the first queue/dismissal timestamp, so the
	// migration is repeatable without rewriting newer user actions.
	if err := db.Exec(`
		UPDATE episode_triage_decisions
		SET queue_state = ?,
		    queue_updated_at = decided_at
		WHERE state = ?
		  AND queue_state IS NULL
		  AND dismissed_at IS NULL
		  AND queue_updated_at IS NULL
	`, models.QueueStateInbox, models.TriageStateShortlisted).Error; err != nil {
		return fmt.Errorf("backfill shortlisted episodes into Inbox: %w", err)
	}
	if err := db.Exec(`
		UPDATE episode_triage_decisions
		SET dismissed_at = decided_at
		WHERE state = ?
		  AND queue_state IS NULL
		  AND dismissed_at IS NULL
		  AND queue_updated_at IS NULL
	`, models.TriageStateDiscarded).Error; err != nil {
		return fmt.Errorf("backfill discarded episodes: %w", err)
	}
	return nil
}

func applyConsumptionQueueOrderMigration(db *gorm.DB) error {
	if err := db.AutoMigrate(&models.EpisodeTriageDecision{}, &models.ConsumptionQueueOrder{}); err != nil {
		return fmt.Errorf("create consumption queue order schema: %w", err)
	}
	if err := db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_episode_triage_queue_position
		ON episode_triage_decisions(queue_state, queue_position, episode_id)
	`).Error; err != nil {
		return fmt.Errorf("create consumption queue position index: %w", err)
	}

	for _, queueState := range []string{
		models.QueueStateInbox,
		models.QueueStateFocus,
		models.QueueStateSomeday,
		models.QueueStateDone,
	} {
		var episodeIDs []uint
		if err := db.Model(&models.EpisodeTriageDecision{}).
			Where("queue_state = ?", queueState).
			Order("queue_updated_at DESC").
			Order("episode_id DESC").
			Pluck("episode_id", &episodeIDs).Error; err != nil {
			return fmt.Errorf("read %s queue order: %w", queueState, err)
		}
		for position, episodeID := range episodeIDs {
			if err := db.Exec(
				"UPDATE episode_triage_decisions SET queue_position = ? WHERE episode_id = ?",
				position,
				episodeID,
			).Error; err != nil {
				return fmt.Errorf("backfill %s queue position: %w", queueState, err)
			}
		}

		var existing models.ConsumptionQueueOrder
		err := db.Where("queue_state = ?", queueState).First(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if err := db.Create(&models.ConsumptionQueueOrder{
				QueueState: queueState,
				Revision:   1,
			}).Error; err != nil {
				return fmt.Errorf("create %s queue revision: %w", queueState, err)
			}
		} else if err != nil {
			return fmt.Errorf("read %s queue revision: %w", queueState, err)
		}
	}
	return nil
}

func applyEpisodeCompletionFactsMigration(db *gorm.DB) error {
	var missingCompletionTime int64
	if err := db.Model(&models.EpisodeTriageDecision{}).
		Where("queue_state = ? AND queue_updated_at IS NULL", models.QueueStateDone).
		Count(&missingCompletionTime).Error; err != nil {
		return fmt.Errorf("preflight current Done completion times: %w", err)
	}
	if missingCompletionTime > 0 {
		return fmt.Errorf(
			"preflight current Done completion times: %d episode(s) have no queue_updated_at",
			missingCompletionTime,
		)
	}

	if err := db.AutoMigrate(&models.EpisodeCompletion{}); err != nil {
		return fmt.Errorf("create episode completion facts: %w", err)
	}
	if err := db.Exec(`
		INSERT INTO episode_completions (
			episode_id,
			completed_at,
			created_at,
			updated_at
		)
		SELECT
			episode_id,
			queue_updated_at,
			queue_updated_at,
			queue_updated_at
		FROM episode_triage_decisions
		WHERE queue_state = ?
		  AND queue_updated_at IS NOT NULL
		ON CONFLICT(episode_id) DO NOTHING
	`, models.QueueStateDone).Error; err != nil {
		return fmt.Errorf("backfill current Done completion facts: %w", err)
	}
	return nil
}

func applyEpisodeProcessingFoundationMigration(db *gorm.DB) error {
	if err := db.AutoMigrate(
		&models.EpisodeProcessingRun{},
		&models.ProcessingCheckpoint{},
		&models.EpisodeArtifactSet{},
		&models.KnowledgeDelivery{},
	); err != nil {
		return fmt.Errorf("create episode processing foundation: %w", err)
	}
	for _, statement := range []string{
		models.ActiveProcessingRunUniqueIndexSQL,
		models.CurrentEpisodeArtifactSetUniqueIndexSQL,
		models.ProcessingRunTerminalStatusTriggerSQL,
	} {
		if err := db.Exec(statement).Error; err != nil {
			return fmt.Errorf("apply episode processing invariant: %w", err)
		}
	}
	return nil
}

func applyEpisodeAudioAssetMigration(db *gorm.DB) error {
	if err := db.AutoMigrate(&models.EpisodeAudioAsset{}); err != nil {
		return fmt.Errorf("create managed episode audio assets: %w", err)
	}
	for _, statement := range []string{
		models.ActiveEpisodeAudioAssetUniqueIndexSQL,
		models.ReadyEpisodeAudioAssetUniqueIndexSQL,
	} {
		if err := db.Exec(statement).Error; err != nil {
			return fmt.Errorf("apply managed episode audio invariant: %w", err)
		}
	}
	return nil
}

func applyFocusProcessingScheduleMigration(db *gorm.DB) error {
	if err := db.AutoMigrate(
		&models.EpisodeProcessingRun{},
		&models.ProcessingScheduleRun{},
		&models.ProcessingScheduleItem{},
	); err != nil {
		return fmt.Errorf("create Focus processing schedule history: %w", err)
	}
	for _, statement := range []string{
		models.ProcessingScheduleRunTriggerKeyUniqueIndexSQL,
		models.ProcessingScheduleItemUniqueIndexSQL,
	} {
		if err := db.Exec(statement).Error; err != nil {
			return fmt.Errorf("apply Focus processing schedule invariant: %w", err)
		}
	}
	return nil
}

func applyEpisodeVideoAvailabilityMigration(db *gorm.DB) error {
	if !db.Migrator().HasTable(&models.Episode{}) {
		return nil
	}
	if db.Migrator().HasColumn(&models.Episode{}, "video_availability") {
		return nil
	}
	if err := db.Exec("ALTER TABLE episodes ADD COLUMN video_availability TEXT NOT NULL DEFAULT ''").Error; err != nil {
		return fmt.Errorf("add episodes.video_availability: %w", err)
	}
	return nil
}

func applyEpisodeVideoAvailabilityConstraintMigration(db *gorm.DB) error {
	if !db.Migrator().HasTable(&models.Episode{}) || !db.Migrator().HasColumn(&models.Episode{}, "video_availability") {
		return nil
	}
	if db.Migrator().HasConstraint(&models.Episode{}, models.VideoAvailabilityConstraint) {
		return nil
	}

	// Schema 23 accepted arbitrary text. Preserve known states and map legacy
	// or malformed values to the existing unknown representation before the
	// table is rebuilt with the CHECK constraint.
	if err := db.Exec(`
		UPDATE episodes
		SET video_availability = CASE lower(trim(video_availability))
			WHEN 'unknown' THEN 'unknown'
			WHEN 'unavailable' THEN 'unavailable'
			WHEN 'available' THEN 'available'
			ELSE ''
		END
	`).Error; err != nil {
		return fmt.Errorf("normalize episodes.video_availability: %w", err)
	}
	if err := db.Migrator().CreateConstraint(&models.Episode{}, models.VideoAvailabilityConstraint); err != nil {
		return fmt.Errorf("constrain episodes.video_availability: %w", err)
	}
	return nil
}

func applyNativeMinutesArtifactIntegrityMigration(db *gorm.DB) error {
	// Keep this migration strictly additive. AutoMigrate follows the artifact
	// model's Episode association and may rebuild the referenced episodes table;
	// inside the versioned transaction SQLite cannot disable foreign keys, so
	// dropping that table would cascade-delete episode-owned records.
	columns := []struct {
		name string
		ddl  string
	}{
		{name: "audio_sha256", ddl: "TEXT NOT NULL DEFAULT ''"},
		{name: "minutes_summary_sha256", ddl: "TEXT NOT NULL DEFAULT ''"},
		{name: "transcript_timeline_sha256", ddl: "TEXT NOT NULL DEFAULT ''"},
	}
	for _, column := range columns {
		if err := addColumnIfMissing(db, &models.EpisodeArtifactSet{}, column.name, column.ddl); err != nil {
			return fmt.Errorf("add native Minutes artifact integrity: %w", err)
		}
	}
	return nil
}

func addColumnIfMissing(db *gorm.DB, model any, name, ddl string) error {
	if db.Migrator().HasColumn(model, name) {
		return nil
	}
	table := ""
	switch model.(type) {
	case *models.JobExecution:
		table = (models.JobExecution{}).TableName()
	case *models.JobFeedAttempt:
		table = (models.JobFeedAttempt{}).TableName()
	case *models.EpisodeArtifactSet:
		table = (models.EpisodeArtifactSet{}).TableName()
	default:
		return fmt.Errorf("cannot resolve table for column %s", name)
	}
	if err := db.Exec("ALTER TABLE " + table + " ADD COLUMN " + name + " " + ddl).Error; err != nil {
		return fmt.Errorf("add %s.%s: %w", table, name, err)
	}
	return nil
}

func requiredTablesMissing(db *gorm.DB) []string {
	return requiredTablesMissingFrom(db, requiredTables)
}

func requiredTablesMissingFrom(db *gorm.DB, tables []string) []string {
	missing := make([]string, 0)
	for _, table := range tables {
		if !db.Migrator().HasTable(table) {
			missing = append(missing, table)
		}
	}
	return missing
}

// AutoMigrate 执行数据库自动迁移
func AutoMigrate(db *gorm.DB) error {
	logger.Info("🔄 Running database migrations...")

	// 按顺序迁移所有模型
	for _, model := range models.AllModels {
		if err := db.AutoMigrate(model); err != nil {
			return fmt.Errorf("failed to migrate %T: %w", model, err)
		}
		logger.Infof("   ✅ Migrated %T", model)
	}

	logger.Info("✅ All migrations completed successfully")
	return nil
}

func autoMigrateModels(db *gorm.DB) error {
	for _, model := range models.AllModels {
		if err := db.AutoMigrate(model); err != nil {
			return fmt.Errorf("failed to create %T: %w", model, err)
		}
	}
	return nil
}

// CreateIndexes 创建自定义索引
func CreateIndexes(db *gorm.DB) error {
	logger.Info("🔄 Creating custom indexes...")

	// Podcast 索引
	if err := db.Exec("CREATE INDEX IF NOT EXISTS idx_podcasts_xyz_id ON podcasts(xyz_id)").Error; err != nil {
		return fmt.Errorf("failed to create podcasts index: %w", err)
	}

	// Podcast 复合索引（优化列表查询排序）
	// is_subscribed + newest_episode_date: 用于按最新单集排序
	if err := db.Exec("CREATE INDEX IF NOT EXISTS idx_podcasts_subscribed_date ON podcasts(is_subscribed, newest_episode_date)").Error; err != nil {
		return fmt.Errorf("failed to create podcasts date index: %w", err)
	}
	if err := db.Exec("CREATE INDEX IF NOT EXISTS idx_podcasts_recent_update_desc ON podcasts(COALESCE(newest_episode_date, created_at) DESC, id DESC)").Error; err != nil {
		return fmt.Errorf("failed to create podcasts recent update index: %w", err)
	}
	// is_subscribed + added_date: 用于按添加日期排序
	if err := db.Exec("CREATE INDEX IF NOT EXISTS idx_podcasts_subscribed_added ON podcasts(is_subscribed, added_date)").Error; err != nil {
		return fmt.Errorf("failed to create podcasts added index: %w", err)
	}
	if err := db.Exec("CREATE INDEX IF NOT EXISTS idx_podcasts_added_date_desc ON podcasts(added_date DESC, id DESC)").Error; err != nil {
		return fmt.Errorf("failed to create podcasts added date index: %w", err)
	}
	if err := db.Exec("CREATE INDEX IF NOT EXISTS idx_podcasts_episode_count_desc ON podcasts(episode_count DESC, id DESC)").Error; err != nil {
		return fmt.Errorf("failed to create podcasts episode count index: %w", err)
	}
	if err := db.Exec("CREATE INDEX IF NOT EXISTS idx_podcasts_title_nocase ON podcasts(title COLLATE NOCASE ASC, id ASC)").Error; err != nil {
		return fmt.Errorf("failed to create podcasts title index: %w", err)
	}

	// Episode 索引
	if err := db.Exec("CREATE INDEX IF NOT EXISTS idx_episodes_podcast_id ON episodes(podcast_id)").Error; err != nil {
		return fmt.Errorf("failed to create episodes index: %w", err)
	}
	// Episode 复合索引（优化按播客查询并按发布日期排序）
	if err := db.Exec("CREATE INDEX IF NOT EXISTS idx_episodes_podcast_date ON episodes(podcast_id, published_date)").Error; err != nil {
		return fmt.Errorf("failed to create episodes date index: %w", err)
	}
	if err := db.Exec("CREATE INDEX IF NOT EXISTS idx_episodes_podcast_published_id_desc ON episodes(podcast_id, published_date DESC, id DESC)").Error; err != nil {
		return fmt.Errorf("failed to create episodes stable list index: %w", err)
	}
	// 注意：guid的uniqueIndex由GORM自动创建（通过model标签），这里不再手动创建

	// Workflow 索引
	if err := db.Exec("CREATE INDEX IF NOT EXISTS idx_workflows_enabled ON workflows(is_enabled)").Error; err != nil {
		return fmt.Errorf("failed to create workflows index: %w", err)
	}
	if err := db.Exec("CREATE INDEX IF NOT EXISTS idx_workflows_updated_at_desc ON workflows(updated_at DESC, id DESC)").Error; err != nil {
		return fmt.Errorf("failed to create workflows updated index: %w", err)
	}
	if err := db.Exec("CREATE INDEX IF NOT EXISTS idx_workflows_next_run_at ON workflows(next_run_at ASC, id ASC)").Error; err != nil {
		return fmt.Errorf("failed to create workflows next run index: %w", err)
	}

	// Job 索引
	if err := db.Exec("CREATE INDEX IF NOT EXISTS idx_jobs_workflow_id ON jobs(workflow_id)").Error; err != nil {
		return fmt.Errorf("failed to create jobs index: %w", err)
	}

	// JobExecution 索引
	if err := db.Exec("CREATE INDEX IF NOT EXISTS idx_job_executions_job_id ON job_executions(job_id)").Error; err != nil {
		return fmt.Errorf("failed to create job_executions index: %w", err)
	}
	if err := db.Exec("CREATE INDEX IF NOT EXISTS idx_job_executions_status ON job_executions(status)").Error; err != nil {
		return fmt.Errorf("failed to create job_executions index: %w", err)
	}

	logger.Info("✅ Custom indexes created successfully")
	return nil
}
