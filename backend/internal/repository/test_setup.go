package repository

import (
	"magicpodcast/internal/models"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// setupTestDB 设置测试数据库（使用内存数据库）
func setupTestDB(t *testing.T) (*gorm.DB, func()) {
	// 创建内存数据库
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}

	// 自动迁移所有核心表
	err = db.AutoMigrate(
		&models.Podcast{},
		&models.Episode{},
		&models.Tag{},
		&models.Workflow{},
		&models.Job{},
		&models.JobExecution{},
		&models.SyncConfig{},
		&models.SchedulerRun{},
		&models.Report{},
	)
	if err != nil {
		t.Fatalf("failed to migrate test database: %v", err)
	}

	cleanup := func() {
		// 清理数据库连接
		sqlDB, _ := db.DB()
		sqlDB.Close()
	}

	return db, cleanup
}
