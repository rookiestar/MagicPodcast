package database

import (
	"fmt"

	"magicpodcast/internal/models"

	"gorm.io/gorm"
)

// AutoMigrate 执行数据库自动迁移
func AutoMigrate(db *gorm.DB) error {
	fmt.Println("🔄 Running database migrations...")

	// 按顺序迁移所有模型
	for _, model := range models.AllModels {
		if err := db.AutoMigrate(model); err != nil {
			return fmt.Errorf("failed to migrate %T: %w", model, err)
		}
		fmt.Printf("   ✅ Migrated %T\n", model)
	}

	fmt.Println("✅ All migrations completed successfully")
	return nil
}

// CreateIndexes 创建自定义索引
func CreateIndexes(db *gorm.DB) error {
	fmt.Println("🔄 Creating custom indexes...")

	// Podcast 索引
	if err := db.Exec("CREATE INDEX IF NOT EXISTS idx_podcasts_xyz_id ON podcasts(xyz_id)").Error; err != nil {
		return fmt.Errorf("failed to create podcasts index: %w", err)
	}

	// Episode 索引
	if err := db.Exec("CREATE INDEX IF NOT EXISTS idx_episodes_podcast_id ON episodes(podcast_id)").Error; err != nil {
		return fmt.Errorf("failed to create episodes index: %w", err)
	}
	if err := db.Exec("CREATE INDEX IF NOT EXISTS idx_episodes_xyz_id ON episodes(xyz_id)").Error; err != nil {
		return fmt.Errorf("failed to create episodes index: %w", err)
	}

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

	fmt.Println("✅ Custom indexes created successfully")
	return nil
}
