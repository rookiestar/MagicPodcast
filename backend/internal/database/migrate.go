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
	// is_subscribed + added_date: 用于按添加日期排序
	if err := db.Exec("CREATE INDEX IF NOT EXISTS idx_podcasts_subscribed_added ON podcasts(is_subscribed, added_date)").Error; err != nil {
		return fmt.Errorf("failed to create podcasts added index: %w", err)
	}

	// Episode 索引
	if err := db.Exec("CREATE INDEX IF NOT EXISTS idx_episodes_podcast_id ON episodes(podcast_id)").Error; err != nil {
		return fmt.Errorf("failed to create episodes index: %w", err)
	}
	// Episode 复合索引（优化按播客查询并按发布日期排序）
	if err := db.Exec("CREATE INDEX IF NOT EXISTS idx_episodes_podcast_date ON episodes(podcast_id, published_date)").Error; err != nil {
		return fmt.Errorf("failed to create episodes date index: %w", err)
	}
	// 注意：guid的uniqueIndex由GORM自动创建（通过model标签），这里不再手动创建

	// Workflow 索引
	if err := db.Exec("CREATE INDEX IF NOT EXISTS idx_workflows_enabled ON workflows(is_enabled)").Error; err != nil {
		return fmt.Errorf("failed to create workflows index: %w", err)
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
