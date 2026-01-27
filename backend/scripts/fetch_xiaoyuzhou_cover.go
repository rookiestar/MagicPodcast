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
	"magicpodcast/internal/scraper"
)

func main() {
	fmt.Println("🎨 Fetch Cover from Xiaoyuzhou")
	fmt.Println("===============================")

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

	// 查询随机波动播客
	var podcast models.Podcast
	if err := db.Where("id = ?", 79).First(&podcast).Error; err != nil {
		log.Fatalf("Failed to query podcast: %v", err)
	}

	fmt.Printf("\nCurrent podcast: %s\n", podcast.Title)
	fmt.Printf("Current cover: %s\n", podcast.CoverURL)

	// 小宇宙网页URL
	xiaoyuzhouURL := "https://www.xiaoyuzhoufm.com/podcast/5e7cc741418a84a046b0c2bd"

	// 创建scraper
	s := scraper.NewScraper()

	// 抓取小宇宙网页
	fmt.Printf("\nFetching from: %s\n", xiaoyuzhouURL)
	scraped, err := s.ScrapeXiaoyuzhou(xiaoyuzhouURL)
	if err != nil {
		log.Fatalf("Failed to scrape xiaoyuzhou: %v", err)
	}

	if scraped == nil {
		log.Fatalf("Scraped data is nil")
	}

	fmt.Printf("\nScraped data:\n")
	fmt.Printf("  Title: %s\n", scraped.Title)
	fmt.Printf("  Author: %s\n", scraped.Author)
	fmt.Printf("  Cover URL: %s\n", scraped.CoverURL)
	fmt.Printf("  Description: %s\n", scraped.Description)

	// 如果获取到封面，更新数据库
	if scraped.CoverURL != "" {
		fmt.Printf("\nUpdating database...\n")

		updates := map[string]interface{}{
			"cover_url": scraped.CoverURL,
		}

		// 如果有其他信息，也一并更新
		if scraped.Author != "" {
			updates["author"] = scraped.Author
		}
		if scraped.Description != "" {
			updates["description"] = scraped.Description
		}

		if err := db.Model(&podcast).Updates(updates).Error; err != nil {
			log.Fatalf("Failed to update database: %v", err)
		}

		fmt.Printf("✅ Successfully updated podcast cover!\n")
		fmt.Printf("   Old cover: %s\n", podcast.CoverURL)
		fmt.Printf("   New cover: %s\n", scraped.CoverURL)
	} else {
		fmt.Printf("⚠️  No cover URL found in scraped data\n")
	}

	// 等待一下，确保数据已写入
	time.Sleep(1 * time.Second)

	fmt.Println("\n" + strings.Repeat("=", 50))
	fmt.Println("Done!")
}
