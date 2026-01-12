package sync

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"magicpodcast/internal/feed"
	"magicpodcast/internal/models"

	"github.com/mmcdole/gofeed"
	"gorm.io/gorm"
)

// SyncPodcastsMetadataSSE 同步所有播客的元数据（SSE流式进度报告）
// 这个函数会在线抓取每个播客的RSS feed，更新元数据字段（episode_count, newest_episode_date等）
func (s *Service) SyncPodcastsMetadataSSE(reporter ProgressReporter) error {
	startTime := time.Now()
	log.Println("🚀 开始同步所有播客的元数据...")
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
		return nil
	}

	log.Printf("📊 共 %d 个播客需要同步元数据", total)
	reporter.Report(fmt.Sprintf("共 %d 个播客需要同步元数据", total))

	// 定义结果类型
	type syncResult struct {
		podcast        *models.Podcast
		err            error
		retries        int
		noUpdate       bool
		episodeResult  *EpisodeSyncResult // 单集同步结果
	}

	// 使用worker pool并发处理
	concurrency := 10
	taskChan := make(chan *models.Podcast, total)
	resultChan := make(chan syncResult, total)

	// 启动worker
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for podcast := range taskChan {
				log.Printf("[Worker %d] 处理: %s", workerID, podcast.Title)

				// 带重试的同步逻辑
				var lastErr error
				var noUpdate bool
				retries := 0

				for retries <= DefaultRetryConfig.MaxRetries {
					if retries > 0 {
						log.Printf("[Worker %d] 重试 %d/%d: %s", workerID, retries, DefaultRetryConfig.MaxRetries, podcast.Title)
						// 指数退避
						delay := DefaultRetryConfig.InitialDelay * time.Duration(1<<uint(retries-1))
						time.Sleep(delay)
					}

					err, noUpdateResult, episodeResult := s.syncPodcastMetadataWithUpdateCheck(podcast, reporter)
					if err == nil {
						// 成功
						resultChan <- syncResult{
							podcast:        podcast,
							err:            nil,
							retries:        retries,
							noUpdate:       noUpdateResult,
							episodeResult:  episodeResult,
						}
						break
					}

					lastErr = err
					noUpdate = noUpdateResult

					// 检查是否是跳过类型的错误（不重试）
					if shouldSkip, _, _ := feed.GetSkipReasonFromError(err); shouldSkip {
						break
					}

					retries++
				}

				// 如果重试后仍然失败
				if lastErr != nil {
					resultChan <- syncResult{
						podcast:        podcast,
						err:            lastErr,
						retries:        retries - 1,
						noUpdate:       noUpdate,
						episodeResult:  nil,
					}
				}
			}
		}(i)
	}

	// 分发任务
	for i := range podcasts {
		taskChan <- &podcasts[i]
	}
	close(taskChan)

	// 收集结果
	successCount := 0
	failedCount := 0
	skippedCount := 0
	noUpdateCount := 0
	current := 0

	// 单集统计
	totalEpisodesCreated := 0
	totalEpisodesUpdated := 0
	totalEpisodesSkipped := 0
	totalEpisodesErrors := 0

	// 等待所有worker完成
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// 处理结果
	for result := range resultChan {
		current++
		podcast := result.podcast

		if result.err != nil {
			// 检查是否是跳过类型的错误
			if shouldSkip, reasonStr, _ := feed.GetSkipReasonFromError(result.err); shouldSkip {
				skippedCount++
				reason := SkipReason(reasonStr)
				reporter.ReportSkip(reason, fmt.Sprintf("[%d/%d] %s - %s", current, total, podcast.Title, result.err.Error()))
			} else {
				failedCount++
				retryMsg := ""
				if result.retries > 0 {
					retryMsg = fmt.Sprintf(" (重试%d次后失败)", result.retries)
				}
				reporter.ReportError(fmt.Sprintf("[%d/%d] %s - %s%s", current, total, podcast.Title, result.err.Error(), retryMsg))
			}
		} else if result.noUpdate {
			noUpdateCount++

			// 构建消息，包含单集统计
			msg := fmt.Sprintf("[%d/%d] %s - 无内容更新", current, total, podcast.Title)
			if result.episodeResult != nil && (result.episodeResult.Created > 0 || result.episodeResult.Updated > 0) {
				msg += fmt.Sprintf(" (单集: +%d, ~%d)", result.episodeResult.Created, result.episodeResult.Updated)
			}
			reporter.ReportSkip(SkipReasonNoUpdate, msg)

			// 即使元数据无更新，也要累加单集统计
			if result.episodeResult != nil {
				totalEpisodesCreated += result.episodeResult.Created
				totalEpisodesUpdated += result.episodeResult.Updated
				totalEpisodesSkipped += result.episodeResult.Skipped
				totalEpisodesErrors += result.episodeResult.Errors
			}
		} else {
			successCount++

			// 构建消息，包含单集统计
			msg := fmt.Sprintf("[%d/%d] 成功同步: %s", current, total, podcast.Title)
			if result.episodeResult != nil && (result.episodeResult.Created > 0 || result.episodeResult.Updated > 0) {
				msg += fmt.Sprintf(" (单集: +%d, ~%d)", result.episodeResult.Created, result.episodeResult.Updated)
			}
			reporter.ReportSuccess(msg)

			// 累加单集统计
			if result.episodeResult != nil {
				totalEpisodesCreated += result.episodeResult.Created
				totalEpisodesUpdated += result.episodeResult.Updated
				totalEpisodesSkipped += result.episodeResult.Skipped
				totalEpisodesErrors += result.episodeResult.Errors
			}
		}

		// 每50个播客报告一次进度（从10改为50）
		if current%50 == 0 || current == total {
			reporter.ReportProgress(current, total, fmt.Sprintf("已处理 %d/%d (成功: %d, 跳过: %d, 无更新: %d)", current, total, successCount, skippedCount, noUpdateCount))
		}
	}

	// 发送汇总信息
	duration := time.Since(startTime)
	summary := &SyncSummary{
		TotalPodcasts:    total,
		SuccessPodcasts:  successCount,
		FailedPodcasts:   failedCount,
		SkippedPodcasts:  skippedCount,
		NoUpdatePodcasts: noUpdateCount,
		TotalEpisodes:    totalEpisodesCreated + totalEpisodesUpdated + totalEpisodesSkipped + totalEpisodesErrors,
		NewEpisodes:      totalEpisodesCreated,
		UpdatedEpisodes:  totalEpisodesUpdated,
		Duration:         duration,
	}
	reporter.ReportSummary(summary)

	log.Printf("✅ 元数据同步完成: 成功=%d, 失败=%d, 跳过=%d, 无更新=%d, 耗时=%s",
		successCount, failedCount, skippedCount, noUpdateCount, formatDuration(duration))
	log.Printf("📝 单集统计: 新增=%d, 更新=%d, 跳过=%d, 错误=%d",
		totalEpisodesCreated, totalEpisodesUpdated, totalEpisodesSkipped, totalEpisodesErrors)

	return nil
}

// syncPodcastMetadataWithUpdateCheck 同步单个播客的元数据，并检测是否有更新
// 返回: 错误, 是否无更新, 单集同步结果
func (s *Service) syncPodcastMetadataWithUpdateCheck(podcast *models.Podcast, reporter ProgressReporter) (error, bool, *EpisodeSyncResult) {
	reporter.Report(fmt.Sprintf("正在抓取: %s", podcast.Title))

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

		// 检测是否有更新
		hasUpdate := false
		var updateReasons []string

		if updatedPodcast.EpisodeCount != podcast.EpisodeCount {
			hasUpdate = true
			updateReasons = append(updateReasons, fmt.Sprintf("episode_count: %d -> %d", podcast.EpisodeCount, updatedPodcast.EpisodeCount))
		}

		// 比较 NewestEpisodeDate - 使用 Equal() 方法而不是 != 操作符
		if !updatedPodcast.NewestEpisodeDate.IsZero() {
			if podcast.NewestEpisodeDate.IsZero() {
				hasUpdate = true
				updateReasons = append(updateReasons, fmt.Sprintf("newest_episode_date: zero -> %s", updatedPodcast.NewestEpisodeDate))
			} else if !updatedPodcast.NewestEpisodeDate.Equal(podcast.NewestEpisodeDate) {
				hasUpdate = true
				updateReasons = append(updateReasons, fmt.Sprintf("newest_episode_date: %s -> %s", podcast.NewestEpisodeDate, updatedPodcast.NewestEpisodeDate))
			}
		}

		if updatedPodcast.NewestEnclosureURL != podcast.NewestEnclosureURL {
			hasUpdate = true
			updateReasons = append(updateReasons, fmt.Sprintf("newest_enclosure_url changed"))
		}

		// Debug日志
		if hasUpdate {
			log.Printf("🔍 检测到更新: %s | 原因: %s", podcast.Title, strings.Join(updateReasons, ", "))
		}

		// 无论元数据是否有更新，都要同步单集
		// 因为单集可能比元数据更频繁地更新
		var episodeResult *EpisodeSyncResult

		// 使用事务保护元数据更新，避免并发修改问题
		err := s.db.Transaction(func(tx *gorm.DB) error {
			if hasUpdate {
				// 有更新，保存更新
				updates := map[string]interface{}{
					"episode_count":             updatedPodcast.EpisodeCount,
					"newest_episode_date":       updatedPodcast.NewestEpisodeDate,
					"newest_enclosure_url":      updatedPodcast.NewestEnclosureURL,
					"newest_enclosure_duration": updatedPodcast.NewestEnclosureDuration,
					"last_fetched_at":           time.Now(),
					"fetch_error_count":         0,
					"feed_url_valid":            true,
				}

				// 使用事务内的更新
				if err := tx.Model(podcast).Updates(updates).Error; err != nil {
					return fmt.Errorf("failed to update podcast: %w", err)
				}

				log.Printf("✅ 成功更新元数据: %s (episode_count=%d)", podcast.Title, updatedPodcast.EpisodeCount)
			} else {
				log.Printf("✓ 元数据无更新: %s", podcast.Title)
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

		// 同步该podcast的单集（使用静默reporter，不输出详细日志）
		silentReporter := NewSilentProgressReporter(reporter)

		// 确定单集同步模式
		var episodeSyncMode EpisodeSyncMode
		var existingEpisodeCount int64
		s.db.Model(&models.Episode{}).Where("podcast_id = ?", podcast.ID).Count(&existingEpisodeCount)

		// RSS feed中的单集数量
		feedEpisodeCount := int64(updatedPodcast.EpisodeCount)

		if existingEpisodeCount == 0 {
			// 如果没有单集，使用全量模式
			episodeSyncMode = SyncModeFull
			log.Printf("   [%s] 无单集，使用全量同步", podcast.Title)
		} else if hasUpdate {
			// 如果元数据有更新，使用全量模式（确保获取所有单集）
			episodeSyncMode = SyncModeFull
			log.Printf("   [%s] 元数据有更新，使用全量同步", podcast.Title)
		} else if existingEpisodeCount != feedEpisodeCount {
			// 如果单集数量不匹配，使用全量模式同步
			episodeSyncMode = SyncModeFull
			log.Printf("   [%s] 单集数量不匹配 (数据库:%d, feed:%d)，使用全量同步", podcast.Title, existingEpisodeCount, feedEpisodeCount)
		} else {
			// 元数据无更新且单集数量匹配，跳过单集同步
			log.Printf("   [%s] 元数据无更新且单集数量匹配(%d)，跳过单集同步", podcast.Title, existingEpisodeCount)
			return nil, true, nil
		}

		config := EpisodeSyncConfig{
			Mode:                 episodeSyncMode,
			UpdateExisting:       true,
			MaxEpisodesPerPodcast: 1000, // 限制每个podcast最多1000个单集
		}

		episodeResult, syncErr := s.SyncPodcastEpisodes(podcast.ID, silentReporter, config)
		if syncErr != nil {
			log.Printf("⚠️  同步单集失败: %s - %v", podcast.Title, syncErr)
			// 单集同步失败不影响元数据同步的成功状态
			episodeResult = nil
		}

		// 返回结果：如果元数据无更新，标记为noUpdate=true
		return nil, !hasUpdate, episodeResult

	case err := <-errChan:
		return fmt.Errorf("fetch feed failed: %w", err), false, nil

	case <-time.After(30 * time.Second):
		return fmt.Errorf("fetch feed timeout after 30s"), false, nil
	}
}

// syncPodcastMetadata 同步单个播客的元数据
func (s *Service) syncPodcastMetadata(podcast *models.Podcast, reporter ProgressReporter) error {
	reporter.Report(fmt.Sprintf("正在抓取: %s", podcast.Title))

	// 添加超时控制：每个播客最多30秒
	resultChan := make(chan *gofeed.Feed, 1)
	errChan := make(chan error, 1)

	go func() {
		// 在线抓取RSS feed（使用feedFetcher）
		gofeed, err := s.feedFetcher.FetchFeed(podcast.FeedURL)
		if err != nil {
			errChan <- err
			return
		}
		resultChan <- gofeed
	}()

	// 等待结果或超时
	select {
	case gofeed := <-resultChan:
		// 使用convertGofeedToModel提取元数据
		updatedPodcast := s.convertGofeedToModel(gofeed, podcast.DataSource, podcast.FeedURL)

		// 只更新元数据字段，保留其他字段
		updates := map[string]interface{}{
			"episode_count":             updatedPodcast.EpisodeCount,
			"newest_episode_date":       updatedPodcast.NewestEpisodeDate,
			"newest_enclosure_url":      updatedPodcast.NewestEnclosureURL,
			"newest_enclosure_duration": updatedPodcast.NewestEnclosureDuration,
			"last_fetched_at":           time.Now(),
			"fetch_error_count":         0,
			"feed_url_valid":            true,
		}

		// 保存更新
		if err := s.db.Model(podcast).Updates(updates).Error; err != nil {
			return fmt.Errorf("failed to update podcast: %w", err)
		}

		log.Printf("✅ 成功更新元数据: %s (episode_count=%d)", podcast.Title, updatedPodcast.EpisodeCount)
		return nil

	case err := <-errChan:
		return fmt.Errorf("fetch feed failed: %w", err)

	case <-time.After(30 * time.Second):
		return fmt.Errorf("fetch feed timeout after 30s")
	}
}
