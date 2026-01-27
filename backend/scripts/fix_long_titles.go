package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"magicpodcast/internal/config"
	"magicpodcast/internal/database"
	"magicpodcast/internal/feed"
	"magicpodcast/internal/models"
)

func main() {
	fmt.Println("🔧 Fix Long Titles from RSS Feeds")
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
	_ = cfg // 未使用，但需要加载配置来初始化数据库

	// 获取数据库连接
	db := database.GetDB()
	defer database.Close()

	// 查找所有标题异常长的节目（>100字符）
	var podcasts []models.Podcast
	if err := db.Where("LENGTH(title) > 100").Find(&podcasts).Error; err != nil {
		log.Fatalf("Failed to query podcasts: %v", err)
	}

	if len(podcasts) == 0 {
		fmt.Println("✅ No podcasts with long titles found.")
		return
	}

	fmt.Printf("\nFound %d podcasts with long titles:\n", len(podcasts))

	// 创建 feed fetcher（超时30秒）
	fetcher := feed.NewFetcher(30 * time.Second)

	// 统计
	successCount := 0
	failedCount := 0

	for i, podcast := range podcasts {
		fmt.Printf("\n[%d/%d] Processing: %s\n", i+1, len(podcasts), podcast.FeedURL)
		fmt.Printf("  Current title (%d chars): %s\n", len(podcast.Title), truncate(podcast.Title, 80))

		// 从 RSS feed 抓取数据
		feedData, err := fetcher.FetchFeed(podcast.FeedURL)
		if err != nil {
			fmt.Printf("  ❌ Failed to fetch feed: %v\n", err)
			failedCount++
			continue
		}

		if feedData.Title == "" {
			fmt.Printf("  ⚠️  Feed has no title, skipping\n")
			failedCount++
			continue
		}

		// 更新数据库
		oldTitle := podcast.Title
		podcast.Title = feedData.Title
		if feedData.Description != "" {
			podcast.Description = feedData.Description
		}

		if err := db.Save(&podcast).Error; err != nil {
			fmt.Printf("  ❌ Failed to update database: %v\n", err)
			failedCount++
			continue
		}

		fmt.Printf("  ✅ Updated title: %s\n", feedData.Title)
		fmt.Printf("     (was %d chars, now %d chars)\n", len(oldTitle), len(feedData.Title))
		successCount++

		// 避免请求过快
		time.Sleep(500 * time.Millisecond)
	}

	// 输出结果
	fmt.Println("\n" + strings.Repeat("=", 50))
	fmt.Printf("✅ Successfully updated: %d\n", successCount)
	fmt.Printf("❌ Failed: %d\n", failedCount)
	fmt.Println(strings.Repeat("=", 50))
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
