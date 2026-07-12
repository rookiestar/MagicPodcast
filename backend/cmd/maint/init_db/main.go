package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"magicpodcast/internal/config"
	"magicpodcast/internal/database"
	"magicpodcast/internal/models"

	"gorm.io/gorm"
)

func main() {
	fmt.Println("🚀 MagicPodcast Database Initialization")
	fmt.Println("======================================")

	// 获取配置文件路径
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "./configs/config.yaml"
	}

	// 转换为绝对路径
	absPath, err := filepath.Abs(configPath)
	if err != nil {
		log.Fatalf("Failed to get absolute path for config: %v", err)
	}

	// 加载配置
	cfg, err := config.Load(absPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	fmt.Printf("✅ Config loaded from: %s\n", absPath)
	fmt.Printf("   Database: %s\n\n", cfg.Database.Path)

	// 初始化数据库
	db := database.GetDB()
	defer database.Close()

	sqlDB, err := db.DB()
	if err != nil {
		log.Fatalf("Failed to get database instance: %v", err)
	}

	// 测试连接
	if err := sqlDB.Ping(); err != nil {
		log.Fatalf("Failed to ping database: %v", err)
	}

	fmt.Println("\n📊 Running explicit versioned migrations...")
	if err := database.ApplyMigrations(db); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}

	if err := database.RequireSchemaReady(db); err != nil {
		log.Fatalf("Database schema is not ready after migrations: %v", err)
	}

	fmt.Println("\n🌱 Seeding initial data...")
	if err := seedData(db); err != nil {
		log.Printf("Warning: Failed to seed data: %v", err)
	}

	fmt.Println("\n✅ Database initialization completed successfully!")
	fmt.Println("   You can now start the API server with:")
	fmt.Println("   go run cmd/api/main.go")
}

// seedData 插入种子数据
func seedData(db *gorm.DB) error {
	// 检查是否已有数据
	var count int64
	db.Model(&models.Tag{}).Count(&count)
	if count > 0 {
		fmt.Println("   ⏭️  Skipping seed data (already exists)")
		return nil
	}

	// 创建示例标签
	tags := []models.Tag{
		{
			Name:  "科技",
			Color: "#3B82F6",
		},
		{
			Name:  "教育",
			Color: "#10B981",
		},
		{
			Name:  "娱乐",
			Color: "#F59E0B",
		},
		{
			Name:  "商业",
			Color: "#8B5CF6",
		},
		{
			Name:  "健康",
			Color: "#EC4899",
		},
	}

	for _, tag := range tags {
		if err := db.Create(&tag).Error; err != nil {
			return fmt.Errorf("failed to create tag %s: %w", tag.Name, err)
		}
		fmt.Printf("   ✅ Created tag: %s\n", tag.Name)
	}

	return nil
}
