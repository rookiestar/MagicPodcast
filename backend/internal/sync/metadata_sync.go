package sync

import (
	"context"
	"fmt"
	"magicpodcast/internal/logger"
	"strings"
	"sync"
	"time"

	"magicpodcast/internal/models"

	"github.com/mmcdole/gofeed"
	"gorm.io/gorm"
)

type metadataSyncResult struct {
	podcast       *models.Podcast
	err           error
	retries       int
	noUpdate      bool
	episodeResult *EpisodeSyncResult
}

// SyncPodcastsMetadataSSE 同步所有播客的元数据（SSE流式进度报告）
// 这个函数会在线抓取每个播客的RSS feed，更新元数据字段（episode_count, newest_episode_date等）
func (s *Service) SyncPodcastsMetadataSSE(reporter ProgressReporter) error {
	startTime := time.Now()
	logger.Info("🚀 开始同步所有播客的元数据...")
	reporter.Report("开始同步所有播客的元数据...")

	// 查询所有已订阅的播客
	var podcasts []models.Podcast
	if err := s.db.Where("is_subscribed = ?", true).Find(&podcasts).Error; err != nil {
		errMsg := fmt.Sprintf("获取播客列表失败: %v", err)
		reporter.ReportError(errMsg)
		return fmt.Errorf("failed to fetch podcasts: %w", err)
	}

	total := len(podcasts)
	if total == 0 {
		reporter.Report("没有已订阅的播客")
		reporter.ReportSummary(&SyncSummary{
			Operation: "sync",
			Duration:  time.Since(startTime),
		})
		return nil
	}

	logger.Infof("📊 共 %d 个播客需要同步元数据", total)
	reporter.Report(fmt.Sprintf("共 %d 个播客需要同步元数据", total))

	// 使用worker pool并发处理
	concurrency := 10
	taskChan := make(chan *models.Podcast, total)
	resultChan := make(chan metadataSyncResult, total)

	// 启动worker
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for podcast := range taskChan {
				logger.Infof("[Worker %d] 处理: %s", workerID, podcast.Title)
				resultChan <- s.syncPodcastMetadataWithRetry(podcast, workerID)
			}
		}(i)
	}

	// 分发任务
	for i := range podcasts {
		taskChan <- &podcasts[i]
	}
	close(taskChan)

	// 收集结果
	stats := metadataSyncStats{}
	current := 0

	// 等待所有worker完成
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// 处理结果
	for result := range resultChan {
		current++
		stats.record(result, current, total, reporter)

		// 每50个播客报告一次进度（从10改为50）
		if current%50 == 0 || current == total {
			stats.reportProgress(current, total, reporter)
		}
	}

	// 发送汇总信息
	duration := time.Since(startTime)
	reporter.ReportSummary(stats.summary(total, duration))
	stats.logSummary(duration)

	if stats.failedCount > 0 {
		return fmt.Errorf("元数据同步失败: %d/%d 个播客未完成同步", stats.failedCount, total)
	}
	return nil
}

// syncPodcastMetadataWithUpdateCheck 同步单个播客的元数据，并检测是否有更新
// 返回: 错误, 是否无更新, 单集同步结果
func (s *Service) syncPodcastMetadataWithUpdateCheck(podcast *models.Podcast) (error, bool, *EpisodeSyncResult) {
	logger.Infof("正在抓取: %s", podcast.Title)

	// 添加context超时控制：每个播客最多30秒
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel() // 确保context被取消，释放资源

	resultChan := make(chan *gofeed.Feed, 1)
	errChan := make(chan error, 1)

	// 使用带context的goroutine，确保可以被取消
	go func() {
		defer func() {
			if r := recover(); r != nil {
				errChan <- fmt.Errorf("panic in feed fetcher: %v", r)
			}
		}()

		// 在线抓取RSS feed（使用feedFetcher，支持context）
		gofeed, err := s.feedFetcher.FetchFeedWithContext(ctx, podcast.FeedURL)
		if err != nil {
			errChan <- err
			return
		}
		resultChan <- gofeed
	}()

	// 等待结果或超时
	select {
	case <-ctx.Done():
		// 超时或context取消
		return fmt.Errorf("fetch feed timeout after 30s"), false, nil
	case gofeed := <-resultChan:
		// 使用convertGofeedToModel提取元数据
		updatedPodcast := s.convertGofeedToModel(gofeed, podcast.DataSource, podcast.FeedURL)
		updateCheck := detectPodcastMetadataUpdate(podcast, updatedPodcast)

		// Debug日志
		if updateCheck.hasUpdate {
			logger.Infof("🔍 检测到更新: %s | 原因: %s", podcast.Title, strings.Join(updateCheck.reasons, ", "))
		}

		// 无论元数据是否有更新，都要同步单集
		// 因为单集可能比元数据更频繁地更新
		var episodeResult *EpisodeSyncResult

		// 使用事务保护元数据更新，避免并发修改问题
		err := s.db.Transaction(func(tx *gorm.DB) error {
			if updateCheck.hasUpdate {
				// 有更新，保存更新
				// 使用事务内的更新
				if err := tx.Model(podcast).Updates(podcastMetadataUpdates(updatedPodcast)).Error; err != nil {
					return fmt.Errorf("failed to update podcast: %w", err)
				}

				logger.Infof("✅ 成功更新元数据: %s (episode_count=%d)", podcast.Title, updatedPodcast.EpisodeCount)
			} else {
				logger.Infof("✓ 元数据无更新: %s", podcast.Title)
				// 更新last_fetched_at，表示已经检查过
				now := time.Now()
				if err := tx.Model(podcast).Update("last_fetched_at", now).Error; err != nil {
					return fmt.Errorf("failed to update last_fetched_at: %w", err)
				}
			}
			return nil
		})

		if err != nil {
			return fmt.Errorf("transaction failed: %w", err), false, nil
		}
		// Metadata refresh is another healthy-primary seam. Persist any stable
		// identity learned from the live body, invalidate stale bindings when it
		// changes, and prepare the alternative outside the sync critical path.
		s.persistPodcastFeedIdentity(podcast, gofeed)
		s.scheduleAlternativePrewarm(podcast)

		// 确定单集同步模式
		var existingEpisodeCount int64
		s.db.Model(&models.Episode{}).Where("podcast_id = ?", podcast.ID).Count(&existingEpisodeCount)

		// RSS feed中的单集数量
		feedEpisodeCount := int64(updatedPodcast.EpisodeCount)
		syncPlan := planEpisodeSync(podcast.Title, updateCheck.hasUpdate, existingEpisodeCount, feedEpisodeCount)

		if !syncPlan.shouldSync {
			return nil, true, nil
		}

		config := EpisodeSyncConfig{
			Mode:                  syncPlan.mode,
			UpdateExisting:        true,
			MaxEpisodesPerPodcast: 1000, // 限制每个podcast最多1000个单集
		}

		episodeResult, err = s.syncPodcastEpisodeItemsWithLastFetchedAt(ctx, podcast, gofeed.Items, config, true)
		if err != nil {
			return fmt.Errorf("同步单集并写回播客汇总失败: %w", err), false, episodeResult
		}

		// 返回结果：如果元数据无更新，标记为noUpdate=true
		return nil, !updateCheck.hasUpdate, episodeResult

	case err := <-errChan:
		return fmt.Errorf("fetch feed failed: %w", err), false, nil

	case <-time.After(30 * time.Second):
		return fmt.Errorf("fetch feed timeout after 30s"), false, nil
	}
}
