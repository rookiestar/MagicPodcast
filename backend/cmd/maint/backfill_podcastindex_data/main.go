package main

import (
	"log"
	"time"

	"magicpodcast/internal/config"
	"magicpodcast/internal/database"
	"magicpodcast/internal/models"
	"magicpodcast/internal/podcastindex"
)

func main() {
	log.Println("🚀 开始从 PodcastIndex 回填播客数据")
	log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// 加载配置
	if _, err := config.Load("configs/config.yaml"); err != nil {
		log.Fatalf("❌ 加载配置失败: %v", err)
	}
	log.Println("✅ 配置加载成功")

	// 获取数据库连接
	db := database.GetDB()
	log.Println("✅ 数据库连接成功")

	// 初始化 PodcastIndex 查询器
	podcastIndexPath := config.Get().PodcastIndex.Path
	podcastIndexQuery, err := podcastindex.NewQuery(podcastIndexPath)
	if err != nil {
		log.Fatalf("❌ 初始化 PodcastIndex 查询器失败: %v", err)
	}
	defer podcastIndexQuery.Close()
	log.Println("✅ PodcastIndex 查询器初始化成功")

	// 获取所有播客
	var podcasts []models.Podcast
	if err := db.Find(&podcasts).Error; err != nil {
		log.Fatalf("❌ 获取播客列表失败: %v", err)
	}

	log.Printf("📊 找到 %d 个播客需要处理", len(podcasts))
	log.Println("")

	startTime := time.Now()

	// 统计信息
	totalCount := len(podcasts)
	successCount := 0
	skippedCount := 0
	errorCount := 0

	// 处理每个播客
	for i, podcast := range podcasts {
		log.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ [%d/%d] %s", i+1, totalCount, podcast.Title)

		// 检查 feed_url 是否存在
		if podcast.FeedURL == "" {
			log.Printf("  ⏭️  跳过：没有 feed_url")
			skippedCount++
			continue
		}

		// 从 PodcastIndex 查询
		log.Printf("  💾 从 PodcastIndex 查询...")
		piInfo, err := podcastIndexQuery.FindByFeedURL(podcast.FeedURL)
		if err != nil {
			log.Printf("  ❌ PodcastIndex 查询错误: %v", err)
			errorCount++
			continue
		}

		if piInfo == nil {
			log.Printf("  📭 PodcastIndex 中未找到")
			skippedCount++
			continue
		}

		// 更新字段
		updates := map[string]interface{}{
			"link":                      piInfo.WebsiteURL,
			"newest_enclosure_url":      piInfo.NewestEnclosureURL,
			"newest_enclosure_duration": piInfo.NewestEnclosureDuration,
			"popularity_score":          piInfo.PopularityScore,
			"priority":                  piInfo.Priority,
			"update_frequency":          piInfo.UpdateFrequency,
			"episode_count":             piInfo.EpisodeCount,
		}

		// 处理时间戳字段
		if piInfo.NewestItemPubdate > 0 {
			t := time.Unix(piInfo.NewestItemPubdate, 0)
			updates["newest_episode_date"] = t
		}

		if piInfo.LastUpdate > 0 {
			t := time.Unix(piInfo.LastUpdate, 0)
			updates["last_update"] = &t
		}

		if piInfo.OldestItemPubdate > 0 {
			t := time.Unix(piInfo.OldestItemPubdate, 0)
			updates["oldest_episode_date"] = &t
		}

		// 执行更新
		if err := db.Model(&podcast).Updates(updates).Error; err != nil {
			log.Printf("  ❌ 更新失败: %v", err)
			errorCount++
			continue
		}

		log.Printf("  ✅ 更新成功")
		successCount++
		log.Printf("     - 官网: %s", piInfo.WebsiteURL)
		log.Printf("     - 热度: %d/10", piInfo.PopularityScore)
		log.Printf("     - 单集数: %d", piInfo.EpisodeCount)
		log.Printf("     - 优先级: %d", piInfo.Priority)

		// 避免过快查询
		time.Sleep(10 * time.Millisecond)
	}

	duration := time.Since(startTime)

	// 打印统计信息
	log.Println("")
	log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ 📊 回填统计")
	log.Printf("✅ 成功: %d (%.1f%%)", successCount, float64(successCount)*100/float64(totalCount))
	log.Printf("⏭️  跳过: %d (%.1f%%)", skippedCount, float64(skippedCount)*100/float64(totalCount))
	log.Printf("❌ 失败: %d (%.1f%%)", errorCount, float64(errorCount)*100/float64(totalCount))
	log.Printf("⏱️  总耗时: %v", duration)
	log.Printf("📈 平均速度: %.2f 秒/个", duration.Seconds()/float64(totalCount))
	log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	log.Println("🎉 回填完成！")
}
