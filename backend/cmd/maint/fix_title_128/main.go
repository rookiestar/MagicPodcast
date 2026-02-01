package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"magicpodcast/internal/config"
	"magicpodcast/internal/database"
	"magicpodcast/internal/models"

	"github.com/mmcdole/gofeed"
)

func main() {
	fmt.Println("🔧 Fix Title for Podcast ID 128")
	fmt.Println("================================")

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

	// 查询节目
	var podcast models.Podcast
	if err := db.Where("id = ?", 128).First(&podcast).Error; err != nil {
		log.Fatalf("Failed to query podcast: %v", err)
	}

	fmt.Printf("\nCurrent podcast info:\n")
	fmt.Printf("  ID: %d\n", podcast.ID)
	fmt.Printf("  Title: %s\n", podcast.Title)
	fmt.Printf("  Title length: %d\n", len(podcast.Title))
	fmt.Printf("  Feed URL: %s\n", podcast.FeedURL)

	// 使用 gofeed 解析 feed
	fp := gofeed.NewParser()
	parsedFeed, err := fp.ParseURL(podcast.FeedURL)
	if err != nil {
		log.Fatalf("Failed to parse feed: %v", err)
	}

	fmt.Printf("\nFeed info:\n")
	fmt.Printf("  Title: %s\n", parsedFeed.Title)
	fmt.Printf("  Title length: %d\n", len(parsedFeed.Title))
	fmt.Printf("  Author: %v\n", parsedFeed.Author)
	fmt.Printf("  Description: %s\n", strings.TrimSpace(parsedFeed.Description))

	// 如果RSS feed有标题，更新数据库
	if parsedFeed.Title != "" && parsedFeed.Title != podcast.Title {
		fmt.Printf("\nUpdating database...\n")

		updates := map[string]interface{}{
			"title": parsedFeed.Title,
		}

		// 如果有其他信息，也一并更新
		if parsedFeed.Author != nil && parsedFeed.Author.Name != "" {
			updates["author"] = parsedFeed.Author.Name
		}
		if parsedFeed.Image != nil && parsedFeed.Image.URL != "" {
			updates["cover_url"] = parsedFeed.Image.URL
		}
		if parsedFeed.Description != "" {
			updates["description"] = parsedFeed.Description
		}
		if parsedFeed.Link != "" {
			updates["link"] = parsedFeed.Link
		}

		if err := db.Model(&podcast).Updates(updates).Error; err != nil {
			log.Fatalf("Failed to update database: %v", err)
		}

		fmt.Printf("✅ Successfully updated podcast!\n")
		fmt.Printf("   Old title: %s\n", podcast.Title)
		fmt.Printf("   New title: %s\n", parsedFeed.Title)
	} else {
		fmt.Printf("\n⚠️  Feed title is the same or empty, no update needed\n")
	}

	fmt.Println("\n" + strings.Repeat("=", 50))
	fmt.Println("Done!")
}
