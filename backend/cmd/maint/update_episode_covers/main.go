package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"magicpodcast/internal/config"
	"magicpodcast/internal/database"
	"magicpodcast/internal/models"
)

func main() {
	fmt.Println("🖼️  Update Episode Covers to Podcast Cover")
	fmt.Println("==========================================")

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
	_ = cfg

	// 获取数据库连接
	db := database.GetDB()
	defer database.Close()

	// 查询随机波动播客
	var podcast models.Podcast
	if err := db.Where("id = ?", 79).First(&podcast).Error; err != nil {
		log.Fatalf("Failed to query podcast: %v", err)
	}

	fmt.Printf("\nPodcast: %s\n", podcast.Title)
	fmt.Printf("Podcast cover: %s\n", podcast.CoverURL)

	// 查询所有单集
	var episodes []models.Episode
	if err := db.Where("podcast_id = ?", 79).Find(&episodes).Error; err != nil {
		log.Fatalf("Failed to query episodes: %v", err)
	}

	fmt.Printf("\nFound %d episodes\n", len(episodes))

	// 统计需要更新的单集
	updateCount := 0
	skipCount := 0

	for _, episode := range episodes {
		// 如果单集封面已经是播客封面，跳过
		if episode.ImageURL == podcast.CoverURL {
			skipCount++
			continue
		}

		// 更新单集封面
		if err := db.Model(&episode).Update("image_url", podcast.CoverURL).Error; err != nil {
			fmt.Printf("  ❌ Failed to update episode %d: %v\n", episode.ID, err)
			continue
		}
		updateCount++

		// 每50个输出一次进度
		if updateCount%50 == 0 {
			fmt.Printf("  Updated %d episodes...\n", updateCount)
		}
	}

	fmt.Printf("\n✅ Successfully updated %d episodes\n", updateCount)
	fmt.Printf("⏭️  Skipped %d episodes (already using podcast cover)\n", skipCount)

	// 等待一下，确保数据已写入
	time.Sleep(1 * time.Second)

	fmt.Println("\n" + "==========================================")
	fmt.Println("Done!")
}
