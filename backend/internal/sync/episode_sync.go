package sync

import (
	"fmt"
	"log"
	"sync"
	"time"

	"magicpodcast/internal/models"

	"github.com/mmcdole/gofeed"
	"gorm.io/gorm"
)

// FullSyncEpoch 是全量同步使用的基准时间
// 2000-01-01 之前RSS/播客格式还不普及,足够早覆盖所有现有节目
var FullSyncEpoch = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

// SyncPodcastEpisodes 同步指定podcast的episodes（智能混合模式）
func (s *Service) SyncPodcastEpisodes(podcastID uint, reporter ProgressReporter, config EpisodeSyncConfig) (*EpisodeSyncResult, error) {
	// 1. 获取podcast信息
	var podcast models.Podcast
	if err := s.db.First(&podcast, podcastID).Error; err != nil {
		return nil, fmt.Errorf("failed to find podcast: %w", err)
	}

	log.Printf("🔄 开始同步podcast episodes: %s (模式: %s)", podcast.Title, config.Mode)
	reporter.Report(fmt.Sprintf("正在同步: %s", podcast.Title))

	result := &EpisodeSyncResult{
		PodcastID:    podcast.ID,
		PodcastTitle: podcast.Title,
	}

	// 2. 确定同步模式和基准时间
	var lastFetchTime time.Time
	useIncremental := false

	switch config.Mode {
	case SyncModeIncremental:
		useIncremental = true
		if podcast.LastFetchedAt != nil {
			lastFetchTime = *podcast.LastFetchedAt
		} else {
			// 如果没有抓取过，使用7天前
			lastFetchTime = time.Now().AddDate(0, 0, -7)
		}

	case SyncModeFull:
		useIncremental = false
		// 全量模式，使用很早的时间确保获取所有内容
		lastFetchTime = FullSyncEpoch

	case SyncModeSmart:
		// 智能模式：根据最后抓取时间自动选择
		if podcast.LastFetchedAt != nil {
			// 如果最后抓取时间在7天内，使用增量同步
			daysSinceLastFetch := time.Since(*podcast.LastFetchedAt).Hours() / 24
			if daysSinceLastFetch <= 7 {
				useIncremental = true
				lastFetchTime = *podcast.LastFetchedAt
			} else {
				// 超过7天，使用全量同步
				useIncremental = false
				lastFetchTime = FullSyncEpoch
			}
		} else {
			// 从未抓取过，使用全量模式
			useIncremental = false
			lastFetchTime = FullSyncEpoch
		}
	}

	// 3. 抓取feed
	var items []*gofeed.Item
	var fetchErr error

	if useIncremental {
		log.Printf("   📊 增量模式: 基准时间 %v", lastFetchTime)
		fetchResult, err := s.feedFetcher.FetchIncremental(podcast.FeedURL, lastFetchTime)
		if err != nil {
			fetchErr = fmt.Errorf("增量抓取失败: %w", err)
		} else {
			items = fetchResult.NewItems
			log.Printf("   ✅ 增量抓取到 %d 个新episode", len(items))
		}
	} else {
		log.Printf("   📊 全量模式: 获取所有episodes")
		feedData, err := s.feedFetcher.FetchFeed(podcast.FeedURL)
		if err != nil {
			fetchErr = fmt.Errorf("全量抓取失败: %w", err)
		} else {
			items = feedData.Items
			log.Printf("   ✅ 全量抓取到 %d 个episodes", len(items))
		}
	}

	if fetchErr != nil {
		return nil, fetchErr
	}

	// 4. 限制处理数量（防止单个podcast过大）
	if config.MaxEpisodesPerPodcast > 0 && len(items) > config.MaxEpisodesPerPodcast {
		log.Printf("   ⚠️  超过最大数量限制，只处理前 %d 个", config.MaxEpisodesPerPodcast)
		items = items[:config.MaxEpisodesPerPodcast]
	}

	// 5. 处理每个episode
	for _, item := range items {
		episode := s.convertGofeedItemToEpisode(&podcast, item)

		// 检查是否已存在
		var existing models.Episode
		err := s.db.Where("guid = ?", episode.GUID).First(&existing).Error

		if err == gorm.ErrRecordNotFound {
			// 新增
			if err := s.db.Create(episode).Error; err != nil {
				log.Printf("   ❌ 创建episode失败: %s - %v", item.Title, err)
				result.Errors++
			} else {
				log.Printf("   ✅ 新增episode: %s", item.Title)
				result.Created++
			}
		} else if err == nil {
			// 已存在
			if config.UpdateExisting {
				// 更新（保留用户自定义字段）
				episode.ID = existing.ID
				episode.Notes = existing.Notes
				episode.MyRate = existing.MyRate
				episode.CreatedAt = existing.CreatedAt

				if err := s.db.Save(episode).Error; err != nil {
					log.Printf("   ❌ 更新episode失败: %s - %v", item.Title, err)
					result.Errors++
				} else {
					log.Printf("   🔄 更新episode: %s", item.Title)
					result.Updated++
				}
			} else {
				result.Skipped++
			}
		} else {
			// 查询错误
			log.Printf("   ❌ 查询episode失败: %v", err)
			result.Errors++
		}
	}

	// 6. 更新podcast的最后抓取时间
	now := time.Now()
	podcast.LastFetchedAt = &now
	if err := s.db.Save(&podcast).Error; err != nil {
		log.Printf("   ⚠️  更新podcast最后抓取时间失败: %v", err)
	}

	log.Printf("✅ 同步完成: %s - 新增: %d, 更新: %d, 跳过: %d, 错误: %d",
		podcast.Title, result.Created, result.Updated, result.Skipped, result.Errors)

	return result, nil
}

// SyncAllPodcastEpisodes 同步所有已订阅podcast的episodes
func (s *Service) SyncAllPodcastEpisodes(reporter ProgressReporter, config EpisodeSyncConfig) error {
	log.Println("🚀 开始同步所有podcast的episodes...")
	reporter.Report("开始同步所有podcast的episodes...")

	// 1. 获取所有已订阅的podcast
	var podcasts []models.Podcast
	if err := s.db.Where("is_subscribed = ?", true).Find(&podcasts).Error; err != nil {
		errMsg := fmt.Sprintf("获取podcast列表失败: %v", err)
		reporter.ReportError(errMsg)
		return fmt.Errorf("failed to fetch podcasts: %w", err)
	}

	total := len(podcasts)
	if total == 0 {
		reporter.Report("没有已订阅的podcast")
		return nil
	}

	log.Printf("📊 共 %d 个podcast需要同步episodes", total)
	reporter.Report(fmt.Sprintf("共 %d 个podcast需要同步episodes", total))

	// 2. 准备并发处理
	concurrency := 10
	taskChan := make(chan *models.Podcast, total)
	resultChan := make(chan *EpisodeSyncResult, total)

	var wg sync.WaitGroup

	// 3. 启动worker pool
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for podcast := range taskChan {
				log.Printf("[Worker %d] 处理: %s", workerID, podcast.Title)
				result, err := s.SyncPodcastEpisodes(podcast.ID, reporter, config)
				if err != nil {
					reporter.ReportError(fmt.Sprintf("%s - %v", podcast.Title, err))
					// 返回一个包含错误的结果
					result = &EpisodeSyncResult{
						PodcastID:    podcast.ID,
						PodcastTitle: podcast.Title,
						Errors:       1,
					}
				}
				resultChan <- result
			}
		}(i)
	}

	// 4. 分发任务
	for i := range podcasts {
		taskChan <- &podcasts[i]
	}
	close(taskChan)

	// 5. 收集结果
	totalCreated := 0
	totalUpdated := 0
	totalSkipped := 0
	totalErrors := 0
	processedCount := 0

	go func() {
		wg.Wait()
		close(resultChan)
	}()

	for result := range resultChan {
		processedCount++
		totalCreated += result.Created
		totalUpdated += result.Updated
		totalSkipped += result.Skipped
		totalErrors += result.Errors

		// 报告进度 - 只在有大量新增或更新时才报告具体播客
		if result.Created > 10 || result.Updated > 50 {
			reporter.ReportSuccess(fmt.Sprintf("[%d/%d] %s - 新增: %d, 更新: %d",
				processedCount, total, result.PodcastTitle, result.Created, result.Updated))
		}

		// 每50个podcast报告一次进度（从10改为50）
		if processedCount%50 == 0 || processedCount == total {
			reporter.ReportProgress(processedCount, total, fmt.Sprintf("已处理 %d/%d (新增: %d, 更新: %d)", processedCount, total, totalCreated, totalUpdated))
		}
	}

	// 6. 发送最终结果
	log.Printf("✅ 所有podcast的episodes同步完成: 新增: %d, 更新: %d, 跳过: %d, 错误: %d",
		totalCreated, totalUpdated, totalSkipped, totalErrors)

	// 发送汇总统计
	totalSuccess := processedCount - totalErrors
	summary := &SyncSummary{
		TotalPodcasts:    total,
		SuccessPodcasts:  totalSuccess,
		FailedPodcasts:   totalErrors,
		SkippedPodcasts:  0, // 单集同步不会跳过podcast
		NoUpdatePodcasts: 0,
		TotalEpisodes:    totalCreated + totalUpdated,
		NewEpisodes:      totalCreated,
		UpdatedEpisodes:  totalUpdated,
	}
	reporter.ReportSummary(summary)

	return nil
}
