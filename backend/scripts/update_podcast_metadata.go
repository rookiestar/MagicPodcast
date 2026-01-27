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
	"magicpodcast/internal/models"

	"github.com/mmcdole/gofeed"
)

func main() {
	fmt.Println("🔧 Update Podcast Metadata from RSS Feeds")
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
	_ = cfg // 未使用，但需要加载配置来初始化数据库

	// 获取数据库连接
	db := database.GetDB()
	defer database.Close()

	// 需要更新的节目ID列表
	podcastIDs := []uint{200, 483, 485, 486, 488, 489}

	var podcasts []models.Podcast
	if err := db.Where("id IN ?", podcastIDs).Find(&podcasts).Error; err != nil {
		log.Fatalf("Failed to query podcasts: %v", err)
	}

	fmt.Printf("\nFound %d podcasts to update:\n", len(podcasts))

	// 统计
	successCount := 0
	failedCount := 0

	for i, podcast := range podcasts {
		fmt.Printf("\n[%d/%d] Processing: %s\n", i+1, len(podcasts), podcast.Title)
		fmt.Printf("  Feed URL: %s\n", podcast.FeedURL)
		fmt.Printf("  Current cover: %s\n", podcast.CoverURL)
		fmt.Printf("  Current author: %s\n", podcast.Author)

		// 使用 gofeed 直接从 URL 解析 feed
		fp := gofeed.NewParser()
		parsedFeed, err := fp.ParseURL(podcast.FeedURL)
		if err != nil {
			fmt.Printf("  ❌ Failed to parse feed: %v\n", err)
			failedCount++
			continue
		}

		// 更新数据库（完整更新所有元数据）
		updates := make(map[string]interface{})

		// 作者信息
		if parsedFeed.Author != nil && parsedFeed.Author.Name != "" {
			updates["author"] = parsedFeed.Author.Name
		} else if parsedFeed.ITunesExt != nil && parsedFeed.ITunesExt.Author != "" {
			updates["author"] = parsedFeed.ITunesExt.Author
		}

		// 封面图片
		if parsedFeed.Image != nil && parsedFeed.Image.URL != "" {
			updates["cover_url"] = parsedFeed.Image.URL
		} else if parsedFeed.ITunesExt != nil && parsedFeed.ITunesExt.Image != "" {
			updates["cover_url"] = parsedFeed.ITunesExt.Image
		}

		// 网站链接
		if parsedFeed.Link != "" {
			updates["link"] = parsedFeed.Link
		}

		// 如果没有更新内容，跳过
		if len(updates) == 0 {
			fmt.Printf("  ⚠️  No new metadata found, skipping\n")
			continue
		}

		// 执行更新
		if err := db.Model(&podcast).Updates(updates).Error; err != nil {
			fmt.Printf("  ❌ Failed to update database: %v\n", err)
			failedCount++
			continue
		}

		fmt.Printf("  ✅ Updated metadata:\n")
		if val, ok := updates["cover_url"]; ok && val != nil {
			fmt.Printf("     - Cover: %s\n", val)
		}
		if val, ok := updates["author"]; ok && val != nil && val != "" {
			fmt.Printf("     - Author: %s\n", val)
		}
		if val, ok := updates["link"]; ok && val != nil && val != "" {
			fmt.Printf("     - Link: %s\n", val)
		}

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
