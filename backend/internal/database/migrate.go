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

const CurrentSchemaVersion = 13

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
	Version     int
	Name        string
	Description string
	Apply       func(*gorm.DB) error
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

var requiredTables = append(append([]string(nil), baselineRequiredTables...), feed.FeedSnapshotsTableName, "podcast_alternative_feeds", "job_feed_attempts", feed.FeedUserAgentGatesTableName)

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

// ApplyMigrations is the only production schema mutation entry point. Every
// migration and its version record run in one transaction so a failed change
// cannot leave a partially recorded schema version.
func ApplyMigrations(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("database is nil")
	}
	return applyMigrationSet(db, migrationRegistry())
}

func applyMigrationSet(db *gorm.DB, migrations []Migration) error {
	sort.Slice(migrations, func(i, j int) bool { return migrations[i].Version < migrations[j].Version })
	return db.Transaction(func(tx *gorm.DB) error {
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
	})
}

func currentSchemaVersion(db *gorm.DB) (int, error) {
	var current int
	if err := db.Model(&SchemaMigration{}).Select("COALESCE(MAX(version), 0)").Scan(&current).Error; err != nil {
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
	if err := db.Exec(feed.FeedUserAgentGatesCreateTableSQL).Error; err != nil {
		return fmt.Errorf("create %s table: %w", feed.FeedUserAgentGatesTableName, err)
	}
	if err := db.Exec(feed.FeedUserAgentGatesCreateIndexSQL).Error; err != nil {
		return fmt.Errorf("create %s state index: %w", feed.FeedUserAgentGatesTableName, err)
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
