package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"magicpodcast/internal/config"
	"magicpodcast/internal/database"
	"magicpodcast/internal/models"

	"github.com/mmcdole/gofeed"
)

func main() {
	fmt.Println("🔍 Check Empty Podcasts")
	fmt.Println("======================")

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

	// 查询这三个节目
	podcastIDs := []uint{128, 200, 488}
	var podcasts []models.Podcast
	if err := db.Where("id IN ?", podcastIDs).Find(&podcasts).Error; err != nil {
		log.Fatalf("Failed to query podcasts: %v", err)
	}

	fp := gofeed.NewParser()

	for _, podcast := range podcasts {
		fmt.Printf("\n[%d] %s\n", podcast.ID, podcast.Title)
		fmt.Printf("  Feed URL: %s\n", podcast.FeedURL)
		fmt.Printf("  Current episode_count: %d\n", podcast.EpisodeCount)

		// 尝试解析feed
		parsedFeed, err := fp.ParseURL(podcast.FeedURL)
		if err != nil {
			fmt.Printf("  ❌ Failed to parse feed: %v\n", err)
			continue
		}

		fmt.Printf("  ✅ Feed parsed successfully\n")
		fmt.Printf("     Feed title: %s\n", parsedFeed.Title)
		fmt.Printf("     Episodes in feed: %d\n", len(parsedFeed.Items))

		if len(parsedFeed.Items) > 0 {
			fmt.Printf("     Latest episode: %s\n", parsedFeed.Items[0].Title)
		}
	}

	fmt.Println("\n" + "======================")
	fmt.Println("Done!")
}
