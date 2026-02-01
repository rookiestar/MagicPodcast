package main

import (
	"fmt"
	"time"

	"magicpodcast/internal/config"
	"magicpodcast/internal/database"
	"magicpodcast/internal/models"
)

func main() {
	// 加载配置
	if _, err := config.Load("configs/config.yaml"); err != nil {
		panic(fmt.Sprintf("Failed to load config: %v", err))
	}

	// 初始化数据库连接
	db := database.GetDB()
	defer database.Close()

	fmt.Println("开始插入测试数据...")

	// 创建测试播客
	podcasts := []models.Podcast{
		{
			Title:             "科技杂谈",
			Description:       "探讨最新科技趋势",
			Author:            "张三",
			CoverURL:          "https://example.com/cover1.jpg",
			EpisodeCount:      50,
			NewestEpisodeDate: time.Now().AddDate(0, 0, -1),
			AddedDate:         time.Now().AddDate(0, -1, 0),
			IsSubscribed:      true,
		},
		{
			Title:             "商业洞察",
			Description:       "深度商业分析",
			Author:            "李四",
			CoverURL:          "https://example.com/cover2.jpg",
			EpisodeCount:      30,
			NewestEpisodeDate: time.Now().AddDate(0, 0, -2),
			AddedDate:         time.Now().AddDate(0, -2, 0),
			IsSubscribed:      true,
		},
		{
			Title:             "健康生活",
			Description:       "健康生活方式分享",
			Author:            "王五",
			CoverURL:          "https://example.com/cover3.jpg",
			EpisodeCount:      100,
			NewestEpisodeDate: time.Now().AddDate(0, 0, -3),
			AddedDate:         time.Now().AddDate(0, -3, 0),
			IsSubscribed:      true,
		},
	}

	for _, podcast := range podcasts {
		if err := db.Create(&podcast).Error; err != nil {
			fmt.Printf("创建播客失败: %v\n", err)
		} else {
			fmt.Printf("✓ 创建播客: %s (ID: %d)\n", podcast.Title, podcast.ID)
		}
	}

	// 创建测试单集
	episodes := []models.Episode{
		{
			PodcastID:     1,
			EpisodeNo:     "1",
			Title:         "人工智能的未来",
			MediumURL:     "https://example.com/ep1.mp3",
			ShowNotes:     "本期节目讨论了AI的发展趋势...",
			PublishedDate: time.Now().AddDate(0, 0, -1),
		},
		{
			PodcastID:     2,
			EpisodeNo:     "1",
			Title:         "商业模式的创新",
			MediumURL:     "https://example.com/ep2.mp3",
			ShowNotes:     "本期节目讨论了新兴商业模式...",
			PublishedDate: time.Now().AddDate(0, 0, -2),
		},
	}

	for _, episode := range episodes {
		if err := db.Create(&episode).Error; err != nil {
			fmt.Printf("创建单集失败: %v\n", err)
		} else {
			fmt.Printf("✓ 创建单集: %s (ID: %d)\n", episode.Title, episode.ID)
		}
	}

	fmt.Println("\n测试数据插入完成！")
}
