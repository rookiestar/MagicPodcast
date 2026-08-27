package database

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"magicpodcast/internal/feed"
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
	require.True(t, db.Migrator().HasColumn(&models.JobExecution{}, "feed_circuit_state"))
	require.True(t, db.Migrator().HasColumn(&models.JobExecution{}, "feed_source_url"))
	require.True(t, db.Migrator().HasColumn(&models.JobExecution{}, "feed_identity_verification"))
	require.True(t, db.Migrator().HasColumn(&models.Episode{}, "video_availability"))
	require.True(t, db.Migrator().HasTable(&models.SchedulerRun{}))
	require.True(t, db.Migrator().HasTable("feed_snapshots"))
	require.True(t, db.Migrator().HasTable(&models.EpisodeAudioAsset{}))
	require.True(t, db.Migrator().HasColumn(&models.EpisodeAudioAsset{}, "source_digest"))
	require.True(t, db.Migrator().HasColumn(&models.EpisodeAudioAsset{}, "relative_path"))
	var activeJobIndexCount int64
	require.NoError(t, db.Raw(`
		SELECT COUNT(*) FROM sqlite_master
		WHERE type = 'index' AND name = 'idx_jobs_one_active_per_workflow'
	`).Scan(&activeJobIndexCount).Error)
	require.Equal(t, int64(1), activeJobIndexCount)

	var foreignKeys int
	require.NoError(t, db.Raw("PRAGMA foreign_keys").Row().Scan(&foreignKeys))
	require.Equal(t, 1, foreignKeys)
	var busyTimeout int
	require.NoError(t, db.Raw("PRAGMA busy_timeout").Row().Scan(&busyTimeout))
	require.GreaterOrEqual(t, busyTimeout, defaultSQLiteBusyTimeoutMS)
}

func TestRequireSchemaReadyRejectsMissingFeedSnapshotsTable(t *testing.T) {
	db := openMigrationTestDB(t, defaultSQLiteBusyTimeoutMS)
	require.NoError(t, ApplyMigrations(db))
	require.NoError(t, db.Migrator().DropTable("feed_snapshots"))

	status, err := InspectSchema(db)
	require.NoError(t, err)
	require.Contains(t, status.RequiredTablesMissing, "feed_snapshots")
	require.ErrorIs(t, RequireSchemaReady(db), ErrSchemaNotReady)
	require.False(t, db.Migrator().HasTable("feed_snapshots"), "readiness must not recreate the table")
}

func TestRequireSchemaReadyRejectsMissingUserAgentGatesTable(t *testing.T) {
	db := openMigrationTestDB(t, defaultSQLiteBusyTimeoutMS)
	require.NoError(t, ApplyMigrations(db))
	require.NoError(t, db.Migrator().DropTable(feed.FeedUserAgentGatesTableName))

	status, err := InspectSchema(db)
	require.NoError(t, err)
	require.Contains(t, status.RequiredTablesMissing, feed.FeedUserAgentGatesTableName)
	require.ErrorIs(t, RequireSchemaReady(db), ErrSchemaNotReady)
	require.False(t, db.Migrator().HasTable(feed.FeedUserAgentGatesTableName), "readiness must not recreate the gate table")
}

func TestRequireSchemaReadyRejectsMissingUserAgentRecoveryTables(t *testing.T) {
	for _, table := range []string{feed.FeedUserAgentGateAuditsTableName, feed.FeedUserAgentGateRecoveryFeedsTableName} {
		t.Run(table, func(t *testing.T) {
			db := openMigrationTestDB(t, defaultSQLiteBusyTimeoutMS)
			require.NoError(t, ApplyMigrations(db))
			require.NoError(t, db.Migrator().DropTable(table))

			status, err := InspectSchema(db)
			require.NoError(t, err)
			require.Contains(t, status.RequiredTablesMissing, table)
			require.ErrorIs(t, RequireSchemaReady(db), ErrSchemaNotReady)
			require.False(t, db.Migrator().HasTable(table), "readiness must not recreate recovery tables")
		})
	}
}

func TestRequireSchemaReadyRejectsMissingEpisodeAudioAssetsTable(t *testing.T) {
	db := openMigrationTestDB(t, defaultSQLiteBusyTimeoutMS)
	require.NoError(t, ApplyMigrations(db))
	require.NoError(t, db.Migrator().DropTable(&models.EpisodeAudioAsset{}))

	status, err := InspectSchema(db)
	require.NoError(t, err)
	require.Contains(t, status.RequiredTablesMissing, "episode_audio_assets")
	require.ErrorIs(t, RequireSchemaReady(db), ErrSchemaNotReady)
	require.False(t, db.Migrator().HasTable(&models.EpisodeAudioAsset{}))
}

func TestApplyMigrationsUpgradesSchemaV5ToV6WithSchedulerRuns(t *testing.T) {
	db := openMigrationTestDB(t, defaultSQLiteBusyTimeoutMS)

	registry := migrationRegistry()
	require.Len(t, registry, CurrentSchemaVersion)
	require.NoError(t, applyMigrationSet(db, registry[:5]))
	require.NoError(t, db.Migrator().DropTable(&models.SchedulerRun{}))

	status, err := InspectSchema(db)
	require.NoError(t, err)
	require.Equal(t, 5, status.CurrentVersion)
	require.False(t, db.Migrator().HasTable(&models.SchedulerRun{}))

	require.NoError(t, ApplyMigrations(db))
	require.True(t, db.Migrator().HasTable(&models.SchedulerRun{}))
	require.True(t, db.Migrator().HasTable(&models.PodcastAlternativeFeed{}))
	require.Equal(t, CurrentSchemaVersion, mustSchemaStatus(t, db).CurrentVersion)
	require.Empty(t, mustSchemaStatus(t, db).Pending)
}

func TestApplyMigrationsCreatesPersistentUserAgentGate(t *testing.T) {
	db := openMigrationTestDB(t, defaultSQLiteBusyTimeoutMS)
	require.NoError(t, applyMigrationSet(db, migrationRegistry()[:12]))
	require.Equal(t, 12, mustSchemaStatus(t, db).CurrentVersion)

	require.NoError(t, ApplyMigrations(db))
	require.Equal(t, CurrentSchemaVersion, mustSchemaStatus(t, db).CurrentVersion)
	require.True(t, db.Migrator().HasTable(feed.FeedUserAgentGatesTableName))
	require.True(t, db.Migrator().HasTable(feed.FeedUserAgentGateAuditsTableName))
	require.True(t, db.Migrator().HasTable(feed.FeedUserAgentGateRecoveryFeedsTableName))
	for _, column := range []string{
		"domain", "user_agent_fingerprint", "state", "detected_at",
		"probe_eligible_at", "last_probe_result", "recovery_success_count", "approved_by",
		"approved_at", "last_probe_at", "updated_at",
	} {
		require.True(t, db.Migrator().HasColumn(feed.FeedUserAgentGatesTableName, column), "missing gate column %s", column)
	}
}

func TestApplyMigrationsUpgradesSchema13To14UserAgentRecoveryState(t *testing.T) {
	db := openMigrationTestDB(t, defaultSQLiteBusyTimeoutMS)
	require.NoError(t, applyMigrationSet(db, migrationRegistry()[:13]))
	require.Equal(t, 13, mustSchemaStatus(t, db).CurrentVersion)

	require.NoError(t, ApplyMigrations(db))
	require.Equal(t, CurrentSchemaVersion, mustSchemaStatus(t, db).CurrentVersion)
	require.True(t, db.Migrator().HasTable(feed.FeedUserAgentGateAuditsTableName))
	require.True(t, db.Migrator().HasTable(feed.FeedUserAgentGateRecoveryFeedsTableName))
	for _, column := range []string{"approved_by", "approved_at", "last_probe_at"} {
		require.True(t, db.Migrator().HasColumn(feed.FeedUserAgentGatesTableName, column), "missing schema 14 column %s", column)
	}
	for _, column := range []string{
		"feed_user_agent_gate_state", "feed_user_agent_probe_result", "feed_user_agent_approved_by",
		"feed_user_agent_approved_at", "feed_user_agent_last_probe_at",
	} {
		require.True(t, db.Migrator().HasColumn(&models.JobExecution{}, column), "missing JobExecution schema 14 column %s", column)
		require.True(t, db.Migrator().HasColumn(&models.JobFeedAttempt{}, column), "missing JobFeedAttempt schema 14 column %s", column)
	}
}

func TestApplyMigrationsUpgradesSchema14To15EpisodeTriageDecisions(t *testing.T) {
	db := openMigrationTestDB(t, defaultSQLiteBusyTimeoutMS)
	require.NoError(t, applyMigrationSet(db, migrationRegistry()[:14]))
	require.Equal(t, 14, mustSchemaStatus(t, db).CurrentVersion)

	require.NoError(t, ApplyMigrations(db))
	require.Equal(t, CurrentSchemaVersion, mustSchemaStatus(t, db).CurrentVersion)
	require.True(t, db.Migrator().HasTable(&models.EpisodeTriageDecision{}))
	require.True(t, db.Migrator().HasColumn(&models.EpisodeTriageDecision{}, "episode_id"))
	require.True(t, db.Migrator().HasColumn(&models.EpisodeTriageDecision{}, "state"))

	var uniqueIndexCount int64
	require.NoError(t, db.Raw(`
		SELECT COUNT(*) FROM sqlite_master
		WHERE type = 'index' AND name = 'idx_episode_triage_decisions_episode_id'
	`).Scan(&uniqueIndexCount).Error)
	require.Equal(t, int64(1), uniqueIndexCount)
}

func TestApplyMigrationsUpgradesSchema15To16HomepageWorkflowReports(t *testing.T) {
	db := openMigrationTestDB(t, defaultSQLiteBusyTimeoutMS)
	require.NoError(t, applyMigrationSet(db, migrationRegistry()[:15]))
	require.Equal(t, 15, mustSchemaStatus(t, db).CurrentVersion)

	require.NoError(t, ApplyMigrations(db))
	require.Equal(t, CurrentSchemaVersion, mustSchemaStatus(t, db).CurrentVersion)
	require.True(t, db.Migrator().HasColumn(&models.Workflow{}, "publish_to_homepage"))
	require.True(t, db.Migrator().HasColumn(&models.Workflow{}, "report_type"))
	require.True(t, db.Migrator().HasColumn(&models.Report{}, "publish_to_homepage"))
	require.True(t, db.Migrator().HasColumn(&models.Report{}, "report_type"))
	require.True(t, db.Migrator().HasColumn(&models.Report{}, "workflow_name"))
	require.True(t, db.Migrator().HasColumn(&models.Report{}, "structured_episodes"))
}

func TestApplyMigrationsUpgradesSchema16To17ConsumptionStateWithoutLosingHistory(t *testing.T) {
	db := openMigrationTestDB(t, defaultSQLiteBusyTimeoutMS)
	require.NoError(t, applyMigrationSet(db, migrationRegistry()[:16]))
	require.Equal(t, 16, mustSchemaStatus(t, db).CurrentVersion)

	podcast := models.Podcast{
		Title: "迁移测试", FeedURL: "https://example.com/migration.xml", XYZID: "migration-17",
	}
	require.NoError(t, db.Create(&podcast).Error)
	episodes := []models.Episode{
		{PodcastID: podcast.ID, Title: "跨日前已收集", GUID: "migration-shortlisted"},
		{PodcastID: podcast.ID, Title: "不感兴趣", GUID: "migration-discarded"},
		{PodcastID: podcast.ID, Title: "中性状态", GUID: "migration-pending"},
	}
	require.NoError(t, db.Create(&episodes).Error)
	decidedAt := time.Date(2026, 7, 1, 8, 30, 0, 0, time.UTC)
	require.NoError(t, db.Create(&[]models.EpisodeTriageDecision{
		{EpisodeID: episodes[0].ID, State: models.TriageStateShortlisted, DecidedAt: decidedAt},
		{EpisodeID: episodes[1].ID, State: models.TriageStateDiscarded, DecidedAt: decidedAt.Add(time.Hour)},
		{EpisodeID: episodes[2].ID, State: models.TriageStatePending, DecidedAt: decidedAt.Add(2 * time.Hour)},
	}).Error)

	require.NoError(t, ApplyMigrations(db))
	require.Equal(t, CurrentSchemaVersion, mustSchemaStatus(t, db).CurrentVersion)
	for _, column := range []string{
		"queue_state", "dismissed_at", "queue_updated_at", "in_progress_at", "read_at",
	} {
		require.True(t, db.Migrator().HasColumn(&models.EpisodeTriageDecision{}, column), "missing schema 17 column %s", column)
	}

	var migrated []models.EpisodeTriageDecision
	require.NoError(t, db.Order("episode_id ASC").Find(&migrated).Error)
	require.Len(t, migrated, 3)
	require.NotNil(t, migrated[0].QueueState)
	require.Equal(t, models.QueueStateInbox, *migrated[0].QueueState)
	require.NotNil(t, migrated[0].QueueUpdatedAt)
	require.True(t, migrated[0].QueueUpdatedAt.Equal(decidedAt))
	require.Nil(t, migrated[0].DismissedAt)
	require.NotNil(t, migrated[1].DismissedAt)
	require.True(t, migrated[1].DismissedAt.Equal(decidedAt.Add(time.Hour)))
	require.Nil(t, migrated[1].QueueState)
	require.Nil(t, migrated[2].QueueState)
	require.Nil(t, migrated[2].DismissedAt)
	require.Nil(t, migrated[2].QueueUpdatedAt)
	for _, row := range migrated {
		require.Nil(t, row.InProgressAt)
		require.Nil(t, row.ReadAt)
	}
}

func TestEpisodeConsumptionStateMigrationIsIdempotentAndPreservesNewerQueueAction(t *testing.T) {
	db := openMigrationTestDB(t, defaultSQLiteBusyTimeoutMS)
	require.NoError(t, applyMigrationSet(db, migrationRegistry()[:16]))
	podcast := models.Podcast{
		Title: "幂等迁移", FeedURL: "https://example.com/idempotent.xml", XYZID: "migration-17-idempotent",
	}
	require.NoError(t, db.Create(&podcast).Error)
	episode := models.Episode{PodcastID: podcast.ID, Title: "已更新状态", GUID: "migration-idempotent"}
	require.NoError(t, db.Create(&episode).Error)
	legacyAt := time.Date(2026, 6, 1, 1, 0, 0, 0, time.UTC)
	require.NoError(t, db.Create(&models.EpisodeTriageDecision{
		EpisodeID: episode.ID, State: models.TriageStateShortlisted, DecidedAt: legacyAt,
	}).Error)

	require.NoError(t, applyEpisodeConsumptionStateMigration(db))
	focus := models.QueueStateFocus
	newerAt := legacyAt.AddDate(0, 1, 0)
	require.NoError(t, db.Model(&models.EpisodeTriageDecision{}).
		Where("episode_id = ?", episode.ID).
		Updates(map[string]any{"queue_state": focus, "queue_updated_at": newerAt}).Error)
	require.NoError(t, applyEpisodeConsumptionStateMigration(db))

	var row models.EpisodeTriageDecision
	require.NoError(t, db.Where("episode_id = ?", episode.ID).First(&row).Error)
	require.NotNil(t, row.QueueState)
	require.Equal(t, models.QueueStateFocus, *row.QueueState)
	require.NotNil(t, row.QueueUpdatedAt)
	require.True(t, row.QueueUpdatedAt.Equal(newerAt))
}

func TestApplyMigrationsUpgradesSchema17To18WithStableQueueOrder(t *testing.T) {
	db := openMigrationTestDB(t, defaultSQLiteBusyTimeoutMS)
	require.NoError(t, applyMigrationSet(db, migrationRegistry()[:17]))
	require.Equal(t, 17, mustSchemaStatus(t, db).CurrentVersion)
	// The baseline uses the current models, so explicitly remove the schema-18
	// objects to model the persisted shape of an actual version-17 database.
	require.NoError(t, db.Migrator().DropTable(&models.ConsumptionQueueOrder{}))
	require.NoError(t, db.Migrator().DropColumn(&models.EpisodeTriageDecision{}, "queue_position"))
	require.False(t, db.Migrator().HasTable(&models.ConsumptionQueueOrder{}))
	require.False(t, db.Migrator().HasColumn(&models.EpisodeTriageDecision{}, "queue_position"))

	podcast := models.Podcast{
		Title: "队列顺序迁移", FeedURL: "https://example.com/queue-order.xml", XYZID: "migration-18",
	}
	require.NoError(t, db.Create(&podcast).Error)
	episodes := []models.Episode{
		{PodcastID: podcast.ID, Title: "较早", GUID: "migration-18-older"},
		{PodcastID: podcast.ID, Title: "并列先写", GUID: "migration-18-tie-first"},
		{PodcastID: podcast.ID, Title: "并列后写", GUID: "migration-18-tie-second"},
	}
	require.NoError(t, db.Create(&episodes).Error)
	older := time.Date(2026, 8, 1, 8, 0, 0, 0, time.UTC)
	tie := older.Add(time.Hour)
	inbox := models.QueueStateInbox
	require.NoError(t, db.Omit("QueuePosition").Create(&[]models.EpisodeTriageDecision{
		{EpisodeID: episodes[0].ID, State: models.TriageStateShortlisted, DecidedAt: older, QueueState: &inbox, QueueUpdatedAt: &older},
		{EpisodeID: episodes[1].ID, State: models.TriageStateShortlisted, DecidedAt: tie, QueueState: &inbox, QueueUpdatedAt: &tie},
		{EpisodeID: episodes[2].ID, State: models.TriageStateShortlisted, DecidedAt: tie, QueueState: &inbox, QueueUpdatedAt: &tie},
	}).Error)

	require.NoError(t, ApplyMigrations(db))
	require.NoError(t, RequireSchemaReady(db))
	require.Equal(t, CurrentSchemaVersion, mustSchemaStatus(t, db).CurrentVersion)
	require.True(t, db.Migrator().HasColumn(&models.EpisodeTriageDecision{}, "queue_position"))
	require.True(t, db.Migrator().HasTable(&models.ConsumptionQueueOrder{}))

	var ordered []models.EpisodeTriageDecision
	require.NoError(t, db.Where("queue_state = ?", inbox).Order("queue_position ASC").Find(&ordered).Error)
	require.Equal(t, []uint{episodes[2].ID, episodes[1].ID, episodes[0].ID}, []uint{
		ordered[0].EpisodeID,
		ordered[1].EpisodeID,
		ordered[2].EpisodeID,
	})
	for position, row := range ordered {
		require.NotNil(t, row.QueuePosition)
		require.Equal(t, int64(position), *row.QueuePosition)
	}

	var orders []models.ConsumptionQueueOrder
	require.NoError(t, db.Order("queue_state ASC").Find(&orders).Error)
	require.Len(t, orders, 4)
	for _, order := range orders {
		require.Equal(t, int64(1), order.Revision)
	}
	var indexCount int64
	require.NoError(t, db.Raw(`
		SELECT COUNT(*) FROM sqlite_master
		WHERE type = 'index' AND name = 'idx_episode_triage_queue_position'
	`).Scan(&indexCount).Error)
	require.Equal(t, int64(1), indexCount)
}

func TestApplyMigrationsUpgradesSchema18To19WithProvableCompletionFacts(t *testing.T) {
	db := openMigrationTestDB(t, defaultSQLiteBusyTimeoutMS)
	require.NoError(t, applyMigrationSet(db, migrationRegistry()[:18]))
	require.Equal(t, 18, mustSchemaStatus(t, db).CurrentVersion)
	require.NoError(t, db.Migrator().DropTable(&models.EpisodeCompletion{}))

	podcast := models.Podcast{
		Title: "完成事实迁移", FeedURL: "https://example.com/completions.xml", XYZID: "migration-19",
	}
	require.NoError(t, db.Create(&podcast).Error)
	episodes := []models.Episode{
		{PodcastID: podcast.ID, Title: "当前完成", GUID: "migration-19-done"},
		{PodcastID: podcast.ID, Title: "当前行动", GUID: "migration-19-inbox"},
	}
	require.NoError(t, db.Create(&episodes).Error)
	completedAt := time.Date(2026, 8, 20, 8, 30, 0, 0, time.UTC)
	done := models.QueueStateDone
	inbox := models.QueueStateInbox
	positions := []int64{0, 0}
	require.NoError(t, db.Create(&[]models.EpisodeTriageDecision{
		{
			EpisodeID: episodes[0].ID, State: models.TriageStateShortlisted,
			DecidedAt: completedAt, QueueState: &done, QueuePosition: &positions[0],
			QueueUpdatedAt: &completedAt,
		},
		{
			EpisodeID: episodes[1].ID, State: models.TriageStateShortlisted,
			DecidedAt: completedAt, QueueState: &inbox, QueuePosition: &positions[1],
			QueueUpdatedAt: &completedAt,
		},
	}).Error)

	require.NoError(t, ApplyMigrations(db))
	require.NoError(t, RequireSchemaReady(db))
	require.Equal(t, CurrentSchemaVersion, mustSchemaStatus(t, db).CurrentVersion)

	var completions []models.EpisodeCompletion
	require.NoError(t, db.Find(&completions).Error)
	require.Len(t, completions, 1)
	require.Equal(t, episodes[0].ID, completions[0].EpisodeID)
	require.True(t, completions[0].CompletedAt.Equal(completedAt))

	require.NoError(t, applyEpisodeCompletionFactsMigration(db))
	var count int64
	require.NoError(t, db.Model(&models.EpisodeCompletion{}).Count(&count).Error)
	require.Equal(t, int64(1), count)

	require.NoError(t, db.Unscoped().Delete(&episodes[0]).Error)
	require.NoError(t, db.Model(&models.EpisodeCompletion{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestEpisodeCompletionMigrationRejectsDoneWithoutCompletionTime(t *testing.T) {
	db := openMigrationTestDB(t, defaultSQLiteBusyTimeoutMS)
	require.NoError(t, applyMigrationSet(db, migrationRegistry()[:18]))
	require.NoError(t, db.Migrator().DropTable(&models.EpisodeCompletion{}))

	podcast := models.Podcast{
		Title: "完成事实预检", FeedURL: "https://example.com/completion-preflight.xml", XYZID: "migration-19-preflight",
	}
	require.NoError(t, db.Create(&podcast).Error)
	episode := models.Episode{
		PodcastID: podcast.ID, Title: "缺少完成时间", GUID: "migration-19-missing-time",
	}
	require.NoError(t, db.Create(&episode).Error)
	done := models.QueueStateDone
	position := int64(0)
	require.NoError(t, db.Create(&models.EpisodeTriageDecision{
		EpisodeID: episode.ID, State: models.TriageStateShortlisted,
		DecidedAt:  time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC),
		QueueState: &done, QueuePosition: &position,
	}).Error)

	err := ApplyMigrations(db)
	require.ErrorContains(t, err, "have no queue_updated_at")
	require.Equal(t, 18, mustSchemaStatus(t, db).CurrentVersion)
	require.False(t, db.Migrator().HasTable(&models.EpisodeCompletion{}))
}

func TestApplyMigrationsUpgradesSchema19To20WithProcessingInvariants(t *testing.T) {
	db := openMigrationTestDB(t, defaultSQLiteBusyTimeoutMS)
	require.NoError(t, applyMigrationSet(db, migrationRegistry()[:19]))
	require.Equal(t, 19, mustSchemaStatus(t, db).CurrentVersion)
	for _, model := range []any{
		&models.KnowledgeDelivery{},
		&models.EpisodeArtifactSet{},
		&models.ProcessingCheckpoint{},
		&models.EpisodeProcessingRun{},
	} {
		require.NoError(t, db.Migrator().DropTable(model))
	}

	require.NoError(t, ApplyMigrations(db))
	require.NoError(t, RequireSchemaReady(db))
	require.Equal(t, CurrentSchemaVersion, mustSchemaStatus(t, db).CurrentVersion)
	for _, model := range []any{
		&models.EpisodeProcessingRun{},
		&models.ProcessingCheckpoint{},
		&models.EpisodeArtifactSet{},
		&models.KnowledgeDelivery{},
	} {
		require.True(t, db.Migrator().HasTable(model))
	}
	require.True(t, db.Migrator().HasColumn(
		&models.ProcessingCheckpoint{},
		"adapter_version",
	))
	for _, name := range []string{
		"idx_episode_processing_runs_one_active",
		"idx_episode_artifact_sets_one_current",
	} {
		var count int64
		require.NoError(t, db.Raw(
			"SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = ?",
			name,
		).Scan(&count).Error)
		require.Equal(t, int64(1), count)
	}
	var triggerCount int64
	require.NoError(t, db.Raw(`
		SELECT COUNT(*) FROM sqlite_master
		WHERE type = 'trigger' AND name = 'trg_episode_processing_runs_terminal_status'
	`).Scan(&triggerCount).Error)
	require.Equal(t, int64(1), triggerCount)

	podcast := models.Podcast{
		Title: "加工迁移", FeedURL: "https://example.com/processing.xml", XYZID: "migration-20",
	}
	require.NoError(t, db.Create(&podcast).Error)
	episode := models.Episode{
		PodcastID: podcast.ID, Title: "加工单集", GUID: "migration-20-episode",
	}
	require.NoError(t, db.Create(&episode).Error)
	now := time.Date(2026, 8, 24, 8, 0, 0, 0, time.UTC)
	first := models.EpisodeProcessingRun{
		EpisodeID:       episode.ID,
		ProcessingKey:   strings.Repeat("1", 64),
		AudioDigest:     strings.Repeat("a", 64),
		PipelineVersion: "v1",
		TriggerSource:   models.ProcessingTriggerManual,
		Status:          models.ProcessingRunStatusQueued,
		MaxAttempts:     3,
		RetryDeadlineAt: now.Add(time.Hour),
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	require.NoError(t, db.Create(&first).Error)
	second := first
	second.ID = 0
	second.CreatedAt = now.Add(time.Second)
	require.ErrorContains(t, db.Create(&second).Error, "UNIQUE constraint failed")
	invalid := first
	invalid.ID = 0
	invalid.Status = "mystery"
	require.ErrorContains(t, db.Create(&invalid).Error, "CHECK constraint failed")

	require.NoError(t, db.Model(&first).Updates(map[string]any{
		"status":      models.ProcessingRunStatusFailed,
		"finished_at": now,
	}).Error)
	require.ErrorContains(
		t,
		db.Model(&first).Update("status", models.ProcessingRunStatusRunning).Error,
		"terminal processing run status is immutable",
	)
}

func TestApplyMigrationsUpgradesSchema20To21WithManagedAudioInvariants(t *testing.T) {
	db := openMigrationTestDB(t, defaultSQLiteBusyTimeoutMS)
	require.NoError(t, applyMigrationSet(db, migrationRegistry()[:20]))
	require.Equal(t, 20, mustSchemaStatus(t, db).CurrentVersion)
	require.False(t, db.Migrator().HasTable(&models.EpisodeAudioAsset{}))

	require.NoError(t, ApplyMigrations(db))
	require.NoError(t, RequireSchemaReady(db))
	require.Equal(t, CurrentSchemaVersion, mustSchemaStatus(t, db).CurrentVersion)
	require.True(t, db.Migrator().HasTable(&models.EpisodeAudioAsset{}))
	for _, column := range []string{
		"source_digest",
		"status",
		"relative_path",
		"sha256",
		"size_bytes",
		"duration_seconds",
		"media_type",
		"extension",
		"error_code",
		"error_message",
		"claim_token",
		"claim_expires_at",
		"queued_at",
		"downloading_at",
		"ready_at",
		"failed_at",
	} {
		require.True(
			t,
			db.Migrator().HasColumn(&models.EpisodeAudioAsset{}, column),
			"missing episode_audio_assets.%s",
			column,
		)
	}
	for _, name := range []string{
		"idx_episode_audio_assets_one_active",
		"idx_episode_audio_assets_ready_source",
	} {
		var count int64
		require.NoError(t, db.Raw(
			"SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = ?",
			name,
		).Scan(&count).Error)
		require.Equal(t, int64(1), count)
	}

	podcast := models.Podcast{
		Title: "受管音频迁移", FeedURL: "https://example.com/managed-audio.xml", XYZID: "migration-21",
	}
	require.NoError(t, db.Create(&podcast).Error)
	episode := models.Episode{
		PodcastID: podcast.ID, Title: "受管音频单集", GUID: "migration-21-episode",
	}
	require.NoError(t, db.Create(&episode).Error)
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	first := models.EpisodeAudioAsset{
		EpisodeID:       episode.ID,
		SourceDigest:    strings.Repeat("a", 64),
		Status:          models.EpisodeAudioAssetStatusQueued,
		DurationSeconds: 60,
		QueuedAt:        now,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	require.NoError(t, db.Create(&first).Error)
	concurrent := first
	concurrent.ID = 0
	concurrent.SourceDigest = strings.Repeat("b", 64)
	require.ErrorContains(t, db.Create(&concurrent).Error, "UNIQUE constraint failed")

	require.NoError(t, db.Model(&first).Updates(map[string]any{
		"status":        models.EpisodeAudioAssetStatusReady,
		"relative_path": "episodes/1/asset.mp3",
		"sha256":        strings.Repeat("c", 64),
		"size_bytes":    10,
		"media_type":    "audio/mpeg",
		"extension":     "mp3",
		"ready_at":      now,
	}).Error)
	duplicateReady := first
	duplicateReady.ID = 0
	duplicateReady.Status = models.EpisodeAudioAssetStatusReady
	require.ErrorContains(t, db.Create(&duplicateReady).Error, "UNIQUE constraint failed")

	invalid := first
	invalid.ID = 0
	invalid.SourceDigest = strings.Repeat("d", 64)
	invalid.Status = "mystery"
	require.ErrorContains(t, db.Create(&invalid).Error, "CHECK constraint failed")

	require.NoError(t, db.Unscoped().Delete(&episode).Error)
	var count int64
	require.NoError(t, db.Model(&models.EpisodeAudioAsset{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestApplyMigrationsUpgradesSchema21To22WithFocusScheduleHistory(t *testing.T) {
	db := openMigrationTestDB(t, defaultSQLiteBusyTimeoutMS)
	require.NoError(t, applyMigrationSet(db, migrationRegistry()[:21]))
	require.Equal(t, 21, mustSchemaStatus(t, db).CurrentVersion)
	require.False(t, db.Migrator().HasTable(&models.ProcessingScheduleRun{}))
	require.False(t, db.Migrator().HasTable(&models.ProcessingScheduleItem{}))

	require.NoError(t, ApplyMigrations(db))
	require.NoError(t, RequireSchemaReady(db))
	require.Equal(t, CurrentSchemaVersion, mustSchemaStatus(t, db).CurrentVersion)
	require.True(t, db.Migrator().HasTable(&models.ProcessingScheduleRun{}))
	require.True(t, db.Migrator().HasTable(&models.ProcessingScheduleItem{}))
	require.True(t, db.Migrator().HasColumn(&models.EpisodeProcessingRun{}, "schedule_run_id"))
	for _, name := range []string{
		"idx_processing_schedule_runs_trigger_key",
		"idx_processing_schedule_items_run_episode",
	} {
		var count int64
		require.NoError(t, db.Raw(
			"SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = ?",
			name,
		).Scan(&count).Error)
		require.Equal(t, int64(1), count)
	}

	now := time.Date(2026, 8, 25, 6, 0, 0, 0, time.UTC)
	run := models.ProcessingScheduleRun{
		TriggerKey:     strings.Repeat("a", 64),
		ScheduledFor:   now,
		CronExpression: "0 0 3 * * *",
		Timezone:       "Asia/Shanghai",
		BatchSize:      1,
		Status:         models.ProcessingScheduleRunStatusCompleted,
		FinishedAt:     &now,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	require.NoError(t, db.Create(&run).Error)
	duplicate := run
	duplicate.ID = 0
	require.ErrorContains(t, db.Create(&duplicate).Error, "UNIQUE constraint failed")
	invalid := run
	invalid.ID = 0
	invalid.TriggerKey = strings.Repeat("b", 64)
	invalid.Status = "mystery"
	require.ErrorContains(t, db.Create(&invalid).Error, "CHECK constraint failed")

	item := models.ProcessingScheduleItem{
		ScheduleRunID: run.ID,
		EpisodeID:     99,
		QueuePosition: 0,
		Outcome:       models.ProcessingScheduleItemOutcomeSkipped,
		Reason:        "audio_not_ready",
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	require.NoError(t, db.Create(&item).Error)
	duplicateItem := item
	duplicateItem.ID = 0
	require.ErrorContains(t, db.Create(&duplicateItem).Error, "UNIQUE constraint failed")
}

func TestApplyMigrationsUpgradesSchema22To23VideoAvailability(t *testing.T) {
	db := openMigrationTestDB(t, defaultSQLiteBusyTimeoutMS)
	require.NoError(t, applyMigrationSet(db, migrationRegistry()[:22]))
	require.Equal(t, 22, mustSchemaStatus(t, db).CurrentVersion)

	// Baseline AutoMigrate uses current models. Remove the new column to model a
	// real schema-22 library before migration 23 is applied.
	if db.Migrator().HasColumn(&models.Episode{}, "video_availability") {
		require.NoError(t, db.Exec("ALTER TABLE episodes DROP COLUMN video_availability").Error)
	}
	require.False(t, db.Migrator().HasColumn(&models.Episode{}, "video_availability"))

	podcast := models.Podcast{
		Title: "视频三态迁移", FeedURL: "https://example.com/video-availability.xml", XYZID: "migration-23",
	}
	require.NoError(t, db.Create(&podcast).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO episodes (podcast_id, title, guid, published_date) VALUES (?, ?, ?, ?)`,
		podcast.ID, "历史单集", "migration-video", time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
	).Error)

	require.NoError(t, ApplyMigrations(db))
	require.NoError(t, RequireSchemaReady(db))
	require.Equal(t, CurrentSchemaVersion, mustSchemaStatus(t, db).CurrentVersion)
	require.True(t, db.Migrator().HasColumn(&models.Episode{}, "video_availability"))

	var stored models.Episode
	require.NoError(t, db.Where("guid = ?", "migration-video").First(&stored).Error)
	require.Equal(t, "", stored.VideoAvailability)
	require.Equal(t, models.VideoAvailabilityUnknown, models.NormalizeVideoAvailability(stored.VideoAvailability))
}

func TestApplyMigrationsLeavesCurrentManagedAudioSchemaUnchanged(t *testing.T) {
	db := openMigrationTestDB(t, defaultSQLiteBusyTimeoutMS)
	require.NoError(t, ApplyMigrations(db))
	require.NoError(t, ApplyMigrations(db))
	require.NoError(t, RequireSchemaReady(db))

	var count int64
	require.NoError(t, db.Model(&SchemaMigration{}).
		Where("version = ?", CurrentSchemaVersion).
		Count(&count).Error)
	require.Equal(t, int64(1), count)
}

func mustSchemaStatus(t *testing.T, db *gorm.DB) SchemaStatus {
	t.Helper()
	status, err := InspectSchema(db)
	require.NoError(t, err)
	return status
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
		"feed_snapshot_retrieved_at", "feed_circuit_state",
		"feed_source_url", "feed_identity_verification",
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
