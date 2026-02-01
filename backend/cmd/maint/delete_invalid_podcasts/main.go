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
	fmt.Println("🗑️  Delete Invalid Podcasts")
	fmt.Println("============================")

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

	// 需要删除的播客ID列表
	podcastIDs := []uint{128, 200}

	var podcasts []models.Podcast
	if err := db.Where("id IN ?", podcastIDs).Find(&podcasts).Error; err != nil {
		log.Fatalf("Failed to query podcasts: %v", err)
	}

	fmt.Printf("\nFound %d podcasts to delete:\n", len(podcasts))

	// 统计
	successCount := 0
	failedCount := 0

	for _, podcast := range podcasts {
		fmt.Printf("\n[%d] %s\n", podcast.ID, podcast.Title)
		fmt.Printf("  Feed URL: %s\n", podcast.FeedURL)
		fmt.Printf("  Episodes: %d\n", podcast.EpisodeCount)

		// 确认删除（实际删除，不使用软删除）
		if err := db.Unscoped().Delete(&podcast).Error; err != nil {
			fmt.Printf("  ❌ Failed to delete: %v\n", err)
			failedCount++
			continue
		}

		fmt.Printf("  ✅ Deleted successfully\n")
		successCount++
	}

	// 输出结果
	fmt.Println("\n" + "============================")
	fmt.Printf("✅ Successfully deleted: %d\n", successCount)
	fmt.Printf("❌ Failed: %d\n", failedCount)
	fmt.Println("============================")

	if successCount > 0 {
		fmt.Println("\n⚠️  Note: These podcasts have been permanently deleted from the database.")
		fmt.Println("If you want to re-import them later, you'll need to fix the feed issues first:")
		fmt.Println("  - ID 128 (动次打次音乐星球): 402 Payment Required - requires paid subscription")
		fmt.Println("  - ID 200 (青腾树下): SSL certificate expired - wait for fix or use alternative source")
	}
}
