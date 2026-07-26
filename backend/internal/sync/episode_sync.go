package sync

import (
	"context"
	"errors"
	"fmt"
	"magicpodcast/internal/cache"
	"magicpodcast/internal/feed"
	"magicpodcast/internal/logger"
	"sync"
	"time"

	"magicpodcast/internal/models"

	"github.com/mmcdole/gofeed"
	"gorm.io/gorm"
)

const episodeGUIDLookupBatchSize = 500

// FullSyncEpoch 是全量同步使用的基准时间
// 2000-01-01 之前RSS/播客格式还不普及,足够早覆盖所有现有节目
var FullSyncEpoch = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

// SyncPodcastEpisodes 同步指定podcast的episodes（智能混合模式）
func (s *Service) SyncPodcastEpisodes(podcastID uint, reporter ProgressReporter, config EpisodeSyncConfig) (*EpisodeSyncResult, error) {
	return s.SyncPodcastEpisodesWithContext(context.Background(), podcastID, reporter, config)
}

// SyncPodcastEpisodesWithContext preserves the caller's cancellation boundary
// and the Feed access outcome for workflow execution history.
func (s *Service) SyncPodcastEpisodesWithContext(ctx context.Context, podcastID uint, reporter ProgressReporter, config EpisodeSyncConfig) (*EpisodeSyncResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	// 1. 获取podcast信息
	var podcast models.Podcast
	if err := s.db.First(&podcast, podcastID).Error; err != nil {
		return nil, fmt.Errorf("failed to find podcast: %w", err)
	}

	logger.Infof("🔄 开始同步podcast episodes: %s (模式: %s)", podcast.Title, config.Mode)
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
	var selectedFeed *gofeed.Feed
	var fetchErr error

	// #35/#36: last-good is NOT used as a success fallback on ordinary primary
	// failure. Matching-validator 304 recovery still happens inside the
	// Coordinator; snapshots remain available for diagnostics only.
	if useIncremental {
		logger.Infof("   📊 增量模式: 基准时间 %v", lastFetchTime)
		fetchResult, err := s.feedFetcher.FetchIncrementalWithContext(ctx, podcast.FeedURL, lastFetchTime)
		if err != nil {
			if alternative, ok := s.fetchVerifiedAlternative(ctx, &podcast, lastFetchTime, true, fetchResult); ok {
				fetchResult = alternative
				err = nil
			}
		}
		if fetchResult != nil {
			selectedFeed = fetchResult.Feed
			result.FeedAccess = &fetchResult.Access
		}
		if err != nil {
			fetchErr = fmt.Errorf("增量抓取失败: %w", err)
		} else {
			items = fetchResult.NewItems
			logger.Infof("   ✅ 增量抓取到 %d 个新episode", len(items))
		}
	} else {
		logger.Infof("   📊 全量模式: 获取所有episodes")
		fetchResult, err := s.feedFetcher.FetchFeedWithContextDetailed(ctx, podcast.FeedURL)
		if err != nil {
			if alternative, ok := s.fetchVerifiedAlternative(ctx, &podcast, lastFetchTime, false, fetchResult); ok {
				fetchResult = alternative
				err = nil
			}
		}
		if fetchResult != nil {
			selectedFeed = fetchResult.Feed
			result.FeedAccess = &fetchResult.Access
		}
		if err != nil {
			fetchErr = fmt.Errorf("全量抓取失败: %w", err)
		} else {
			items = fetchResult.Feed.Items
			logger.Infof("   ✅ 全量抓取到 %d 个episodes", len(items))
		}
	}

	if fetchErr != nil {
		return result, fetchErr
	}
	// Only a primary refresh may establish or change the subscription identity.
	// A verified alternative is temporary content and must never rewrite the
	// primary identity. Prewarming is admitted asynchronously and bounded so
	// optional PodcastIndex work cannot extend the workflow's critical path.
	if result.FeedAccess == nil || result.FeedAccess.SourceType == feed.AccessSourcePrimary || result.FeedAccess.SourceType == feed.AccessSourceSharedCache {
		s.persistPodcastFeedIdentity(&podcast, selectedFeed)
		s.scheduleAlternativePrewarm(&podcast)
	}

	updateLastFetchedAt := result.FeedAccess == nil ||
		(result.FeedAccess.SourceType != feed.AccessSourceLastGood && result.FeedAccess.SourceType != feed.AccessSourceLocalCache)
	episodeResult, err := s.syncPodcastEpisodeItemsWithLastFetchedAt(&podcast, items, config, updateLastFetchedAt)
	episodeResult.FeedAccess = result.FeedAccess
	result = episodeResult
	if err != nil {
		return result, fmt.Errorf("同步单集并写回播客汇总失败: %w", err)
	}

	logger.Infof("✅ 同步完成: %s - 新增: %d, 更新: %d, 跳过: %d, 错误: %d",
		podcast.Title, result.Created, result.Updated, result.Skipped, result.Errors)

	return result, nil
}

func (s *Service) syncPodcastEpisodeItems(podcast *models.Podcast, items []*gofeed.Item, config EpisodeSyncConfig) (*EpisodeSyncResult, error) {
	return s.syncPodcastEpisodeItemsWithLastFetchedAt(podcast, items, config, true)
}

func (s *Service) syncPodcastEpisodeItemsWithLastFetchedAt(podcast *models.Podcast, items []*gofeed.Item, config EpisodeSyncConfig, updateLastFetchedAt bool) (*EpisodeSyncResult, error) {
	result := &EpisodeSyncResult{
		PodcastID:    podcast.ID,
		PodcastTitle: podcast.Title,
	}

	if config.MaxEpisodesPerPodcast > 0 && len(items) > config.MaxEpisodesPerPodcast {
		logger.Infof("   ⚠️  超过最大数量限制，只处理前 %d 个", config.MaxEpisodesPerPodcast)
		items = items[:config.MaxEpisodesPerPodcast]
	}

	episodes := make([]*models.Episode, 0, len(items))
	for _, item := range items {
		episodes = append(episodes, s.convertGofeedItemToEpisode(podcast, item))
	}

	existingByGUID, err := s.loadExistingEpisodesByGUID(collectEpisodeGUIDs(episodes))
	if err != nil {
		logger.Infof("   ❌ 查询已有episodes失败: %v", err)
		result.Errors += len(episodes)
		return result, fmt.Errorf("查询已有单集失败: %w", err)
	}
	existingByIdentity, err := s.loadExistingEpisodesByIdentity(podcast.ID, episodes)
	if err != nil {
		logger.Infof("   ❌ 查询已有单集身份失败: %v", err)
		result.Errors += len(episodes)
		return result, fmt.Errorf("查询已有单集身份失败: %w", err)
	}
	var firstWriteErr error

	for index, episode := range episodes {
		item := items[index]
		existing, exists := existingByGUID[episode.GUID]
		matchedByIdentity := false
		if !exists {
			if identityKey, ok := episodeIdentityKey(episode); ok {
				if identityExisting, identityExists := existingByIdentity[identityKey]; identityExists {
					existing = identityExisting
					exists = true
					matchedByIdentity = true
				}
			}
		}

		if exists && existing.PodcastID != podcast.ID {
			logger.Infof("   ❌ 跳过episode: %s - GUID已属于其他播客", item.Title)
			result.Errors++
			if firstWriteErr == nil {
				firstWriteErr = fmt.Errorf("GUID %q 已属于播客 %d", episode.GUID, existing.PodcastID)
			}
			continue
		}

		if exists && existing.DeletedAt.Valid {
			// Soft-deleted episodes still occupy the unique GUID index. Do not
			// restore them or try to insert a new row with the same GUID.
			logger.Infof("   ⏭️ 跳过已软删除episode: %s", item.Title)
			result.Skipped++
			continue
		}

		if !exists {
			// 新增
			if err := s.db.Create(episode).Error; err != nil {
				logger.Infof("   ❌ 创建episode失败: %s - %v", item.Title, err)
				result.Errors++
				if firstWriteErr == nil {
					firstWriteErr = fmt.Errorf("创建单集 %q 失败: %w", item.Title, err)
				}
			} else {
				logger.Infof("   ✅ 新增episode: %s", item.Title)
				result.Created++
				existingByGUID[episode.GUID] = *episode
				if identityKey, ok := episodeIdentityKey(episode); ok {
					existingByIdentity[identityKey] = *episode
				}
			}
			continue
		}

		if matchedByIdentity {
			// A source-specific GUID may change when the same episode is read from
			// another platform. Keep the first record intact, including its GUID,
			// primary-source fields, and user-owned fields.
			logger.Infof("   🔄 跳过跨源重复episode: %s", item.Title)
			result.Skipped++
			continue
		}

		if exists {
			// 已存在
			if config.UpdateExisting {
				if !episodeNeedsUpdate(&existing, episode) {
					result.Skipped++
					continue
				}

				// 更新（保留用户自定义字段）
				episode.ID = existing.ID
				episode.Notes = existing.Notes
				episode.MyRate = existing.MyRate
				episode.CreatedAt = existing.CreatedAt

				if err := s.db.Save(episode).Error; err != nil {
					logger.Infof("   ❌ 更新episode失败: %s - %v", item.Title, err)
					result.Errors++
					if firstWriteErr == nil {
						firstWriteErr = fmt.Errorf("更新单集 %q 失败: %w", item.Title, err)
					}
				} else {
					logger.Infof("   🔄 更新episode: %s", item.Title)
					result.Updated++
					existingByGUID[episode.GUID] = *episode
					if identityKey, ok := episodeIdentityKey(episode); ok {
						existingByIdentity[identityKey] = *episode
					}
				}
			} else {
				result.Skipped++
			}
		}
	}

	if err := s.refreshPodcastEpisodeSyncFieldsWithLastFetchedAt(podcast, result, updateLastFetchedAt); err != nil {
		return result, fmt.Errorf("刷新播客汇总字段失败: %w", err)
	}
	if firstWriteErr != nil {
		return result, firstWriteErr
	}
	return result, nil
}

func episodeNeedsUpdate(existing *models.Episode, next *models.Episode) bool {
	if existing.PodcastID != next.PodcastID {
		return true
	}
	if existing.EpisodeNo != next.EpisodeNo ||
		existing.Title != next.Title ||
		existing.MediumURL != next.MediumURL ||
		existing.ShowNotes != next.ShowNotes ||
		existing.Duration != next.Duration ||
		existing.Link != next.Link ||
		existing.Content != next.Content ||
		existing.ImageURL != next.ImageURL ||
		existing.EnclosureType != next.EnclosureType ||
		existing.EnclosureLength != next.EnclosureLength ||
		existing.GUID != next.GUID {
		return true
	}

	if !sameTime(existing.PublishedDate, next.PublishedDate) {
		return true
	}

	if !sameOptionalTime(existing.UpdatedDate, next.UpdatedDate) {
		return true
	}

	return false
}

func sameTime(left, right time.Time) bool {
	if left.IsZero() && right.IsZero() {
		return true
	}
	return left.Equal(right)
}

func sameOptionalTime(left, right *time.Time) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return sameTime(*left, *right)
}

func collectEpisodeGUIDs(episodes []*models.Episode) []string {
	seen := make(map[string]struct{}, len(episodes))
	guids := make([]string, 0, len(episodes))

	for _, episode := range episodes {
		if episode.GUID == "" {
			continue
		}
		if _, exists := seen[episode.GUID]; exists {
			continue
		}
		seen[episode.GUID] = struct{}{}
		guids = append(guids, episode.GUID)
	}

	return guids
}

func (s *Service) loadExistingEpisodesByGUID(guids []string) (map[string]models.Episode, error) {
	existingByGUID := make(map[string]models.Episode, len(guids))
	if len(guids) == 0 {
		return existingByGUID, nil
	}

	for start := 0; start < len(guids); start += episodeGUIDLookupBatchSize {
		end := start + episodeGUIDLookupBatchSize
		if end > len(guids) {
			end = len(guids)
		}

		var existingEpisodes []models.Episode
		if err := s.db.Unscoped().Where("guid IN ?", guids[start:end]).Find(&existingEpisodes).Error; err != nil {
			return nil, err
		}

		for _, episode := range existingEpisodes {
			current, exists := existingByGUID[episode.GUID]
			if !exists || (current.DeletedAt.Valid && !episode.DeletedAt.Valid) {
				existingByGUID[episode.GUID] = episode
			}
		}
	}

	return existingByGUID, nil
}

func (s *Service) loadExistingEpisodesByIdentity(podcastID uint, episodes []*models.Episode) (map[string]models.Episode, error) {
	existingByIdentity := make(map[string]models.Episode)
	publishedDates := make([]time.Time, 0, len(episodes))
	seenDates := make(map[int64]struct{}, len(episodes))
	for _, episode := range episodes {
		if episode == nil || episode.PublishedDate.IsZero() {
			continue
		}
		published := episode.PublishedDate.UTC()
		key := published.UnixNano()
		if _, exists := seenDates[key]; exists {
			continue
		}
		seenDates[key] = struct{}{}
		publishedDates = append(publishedDates, published)
	}
	if len(publishedDates) == 0 {
		return existingByIdentity, nil
	}

	var existingEpisodes []models.Episode
	if err := s.db.Where("podcast_id = ? AND published_date IN ?", podcastID, publishedDates).Find(&existingEpisodes).Error; err != nil {
		return nil, err
	}
	for _, episode := range existingEpisodes {
		identityKey, ok := episodeIdentityKey(&episode)
		if !ok {
			continue
		}
		current, exists := existingByIdentity[identityKey]
		if !exists || episode.ID < current.ID {
			existingByIdentity[identityKey] = episode
		}
	}
	return existingByIdentity, nil
}

func (s *Service) refreshPodcastEpisodeSyncFields(podcast *models.Podcast, result *EpisodeSyncResult) error {
	return s.refreshPodcastEpisodeSyncFieldsWithLastFetchedAt(podcast, result, true)
}

func (s *Service) refreshPodcastEpisodeSyncFieldsWithLastFetchedAt(podcast *models.Podcast, result *EpisodeSyncResult, updateLastFetchedAt bool) error {
	now := time.Now()
	updates := map[string]interface{}{}
	if updateLastFetchedAt {
		updates["last_fetched_at"] = now
	}

	// 每次同步都从实际单集重新计算汇总，避免 feed 抓取时间或 RSS
	// updated 字段污染“最近更新”的发布时间语义。
	var newestEpisode models.Episode
	newestErr := s.db.Where("podcast_id = ?", podcast.ID).
		Select("published_date").
		Order("published_date DESC, id DESC").
		First(&newestEpisode).Error
	if newestErr != nil && !errors.Is(newestErr, gorm.ErrRecordNotFound) {
		return fmt.Errorf("查询最新单集失败: %w", newestErr)
	}
	if newestErr == nil && !newestEpisode.PublishedDate.IsZero() {
		podcast.NewestEpisodeDate = newestEpisode.PublishedDate
		updates["newest_episode_date"] = podcast.NewestEpisodeDate
		logger.Infof("   📅 更新newest_episode_date: %s", podcast.NewestEpisodeDate.Format("2006-01-02 15:04:05"))
	} else {
		podcast.NewestEpisodeDate = time.Time{}
		updates["newest_episode_date"] = nil
	}

	var actualEpisodeCount int64
	if err := s.db.Model(&models.Episode{}).Where("podcast_id = ?", podcast.ID).Count(&actualEpisodeCount).Error; err == nil {
		oldEpisodeCount := podcast.EpisodeCount
		podcast.EpisodeCount = int(actualEpisodeCount)
		updates["episode_count"] = podcast.EpisodeCount
		if oldEpisodeCount != podcast.EpisodeCount {
			logger.Infof("   📊 更新 episode_count: %d → %d", oldEpisodeCount, podcast.EpisodeCount)
		}
	} else {
		return fmt.Errorf("统计单集数量失败: %w", err)
	}

	if err := s.db.Model(&models.Podcast{}).Where("id = ?", podcast.ID).Updates(updates).Error; err != nil {
		return fmt.Errorf("写回播客汇总失败: %w", err)
	}
	cache.InvalidatePodcastDetail(podcast.ID)
	return nil
}

// SyncAllPodcastEpisodes 同步所有已订阅podcast的episodes
func (s *Service) SyncAllPodcastEpisodes(reporter ProgressReporter, config EpisodeSyncConfig) error {
	logger.Info("🚀 开始同步所有podcast的episodes...")
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

	logger.Infof("📊 共 %d 个podcast需要同步episodes", total)
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
				logger.Infof("[Worker %d] 处理: %s", workerID, podcast.Title)
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
	logger.Infof("✅ 所有podcast的episodes同步完成: 新增: %d, 更新: %d, 跳过: %d, 错误: %d",
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

	if totalErrors > 0 {
		return fmt.Errorf("单集同步失败: %d 个播客未完成同步", totalErrors)
	}
	return nil
}
