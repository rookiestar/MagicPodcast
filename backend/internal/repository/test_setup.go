package repository

import (
	"fmt"
	"magicpodcast/internal/models"
	"testing"
	"time"

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

// generateUniquePodcast 生成唯一播客测试数据
func generateUniquePodcast(id int) *models.Podcast {
	return &models.Podcast{
		Title:        fmt.Sprintf("测试播客_%d", id),
		Author:       "测试作者",
		Description:  fmt.Sprintf("这是一个测试播客_%d", id),
		FeedURL:      fmt.Sprintf("https://example.com/feed%d.xml", id),
		XYZID:        fmt.Sprintf("xyz_test_%d", id),
		PodcastGUID:  fmt.Sprintf("podcast_guid_%d", id),
	}
}

// generateUniqueEpisode 生成唯一单集测试数据
func generateUniqueEpisode(podcastID uint, id int) *models.Episode {
	return &models.Episode{
		PodcastID:     podcastID,
		EpisodeNo:     fmt.Sprintf("%d", id),
		Title:         fmt.Sprintf("测试单集_%d", id),
		MediumURL:     fmt.Sprintf("https://example.com/ep%d.mp3", id),
		ShowNotes:     fmt.Sprintf("节目详情_%d", id),
		PublishedDate: time.Now().Add(time.Duration(-id) * time.Hour),
		GUID:          fmt.Sprintf("episode_guid_%d", id),
	}
}

// generateUniqueTag 生成唯一标签测试数据
func generateUniqueTag(id int) *models.Tag {
	return &models.Tag{
		Name:  fmt.Sprintf("标签%d", id),
		Color: fmt.Sprintf("#%06X", id*0x123456),
	}
}

// generateUniqueWorkflow 生成唯一工作流测试数据
func generateUniqueWorkflow(id int) *models.Workflow {
	return &models.Workflow{
		Name:        fmt.Sprintf("测试工作流_%d", id),
		Description: fmt.Sprintf("这是一个测试工作流_%d", id),
		IsEnabled:   true,
		Schedule:    fmt.Sprintf("0 %d * * *", (id%24)),
	}
}
