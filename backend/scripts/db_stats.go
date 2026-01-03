package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"magicpodcast/internal/config"
	"magicpodcast/internal/database"
	"magicpodcast/internal/models"
)

func main() {
	fmt.Println("📊 MagicPodcast Database Statistics")
	fmt.Println("===================================")

	// 加载配置
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "./configs/config.yaml"
	}

	absPath, err := filepath.Abs(configPath)
	if err != nil {
		log.Fatalf("Failed to get absolute path: %v", err)
	}

	cfg, err := config.Load(absPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	fmt.Printf("Database: %s\n\n", cfg.Database.Path)

	// 获取数据库连接
	db := database.GetDB()
	defer database.Close()

	// 统计各表的记录数
	tables := map[string]interface{}{
		"Podcasts":  &models.Podcast{},
		"Episodes":  &models.Episode{},
		"Tags":      &models.Tag{},
		"Workflows": &models.Workflow{},
		"Jobs":      &models.Job{},
		"Executions": &models.JobExecution{},
		"Reports":   &models.Report{},
	}

	totalRecords := int64(0)

	for name, model := range tables {
		var count int64
		if err := db.Model(model).Count(&count).Error; err != nil {
			log.Printf("❌ Failed to count %s: %v\n", name, err)
		} else {
			fmt.Printf("   %-12s: %d records\n", name, count)
			totalRecords += count
		}
	}

	fmt.Printf("\n   %-12s: %d records\n", "Total", totalRecords)

	// 检查数据库文件大小
	info, err := os.Stat(cfg.Database.Path)
	if err != nil {
		log.Printf("❌ Failed to get database file info: %v\n", err)
	} else {
		sizeKB := float64(info.Size()) / 1024
		fmt.Printf("\n📁 Database file size: %.2f KB\n", sizeKB)
	}

	fmt.Println("\n✅ Statistics completed!")
}
