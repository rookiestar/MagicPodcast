package database

import (
	"fmt"

	"magicpodcast/internal/logger"
	"magicpodcast/internal/models"

	"gorm.io/gorm"
)

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
