package main

import (
	"fmt"
	"log"

	"magicpodcast/internal/config"
	"magicpodcast/internal/database"
	"magicpodcast/internal/models"
	"magicpodcast/internal/services"

	"gorm.io/gorm"
)

// migrateLLMRetryTable 创建LLM重试任务表
func migrateLLMRetryTable(db *gorm.DB) error {
	log.Println("📊 开始迁移LLM重试任务表...")

	// 自动迁移
	err := db.AutoMigrate(&services.LLMRetryJob{})
	if err != nil {
		return fmt.Errorf("迁移LLM重试任务表失败: %w", err)
	}

	// 创建索引
	if err := db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_llm_retry_jobs_status_next_retry
		ON llm_retry_jobs (status, next_retry_at);
	`).Error; err != nil {
		log.Printf("创建索引失败: %v", err)
	}

	// 创建复合索引
	if err := db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_llm_retry_jobs_workflow_status
		ON llm_retry_jobs (workflow_id, status);
	`).Error; err != nil {
		log.Printf("创建索引失败: %v", err)
	}

	log.Println("✅ LLM重试任务表迁移完成")
	return nil
}

func main() {
	// 加载配置
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	// 初始化数据库
	db, err := database.Init(cfg)
	if err != nil {
		log.Fatalf("初始化数据库失败: %v", err)
	}

	// 执行迁移
	if err := migrateLLMRetryTable(db); err != nil {
		log.Fatalf("迁移失败: %v", err)
	}

	log.Println("✅ 所有迁移完成")
}
