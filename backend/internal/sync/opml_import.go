package sync

import (
	"fmt"
	"magicpodcast/internal/logger"
	"strings"
	"sync"
	"time"

	"magicpodcast/internal/feed"
	"magicpodcast/internal/models"
	"magicpodcast/internal/opml"
	"magicpodcast/internal/podcastindex"
)

// ImportOPML 导入OPML文件（使用默认配置）
func (s *Service) ImportOPML(filePath string) (*SyncResult, error) {
	return s.ImportOPMLWithProgressAndConfig(filePath, NewLogProgressReporter(), DefaultImportConfig)
}

// ImportOPMLWithProgress 导入OPML文件（带进度报告，使用默认并发配置）
func (s *Service) ImportOPMLWithProgress(filePath string, reporter ProgressReporter) (*SyncResult, error) {
	return s.ImportOPMLWithProgressAndConfig(filePath, reporter, DefaultImportConfig)
}

// ImportOPMLWithProgressAndConfig 导入OPML文件（带进度报告和自定义并发配置）
func (s *Service) ImportOPMLWithProgressAndConfig(filePath string, reporter ProgressReporter, config ImportConfig) (*SyncResult, error) {
	logger.Infof("🚀 开始导入OPML (并发度: %d): %s", config.Concurrency, filePath)
	reporter.Report("开始导入OPML文件: " + filePath)

	// 1. 解析OPML文件
	outlines, err := s.opmlParser.ParseFile(filePath)
	if err != nil {
		reporter.ReportError("解析OPML文件失败: " + err.Error())
		return nil, fmt.Errorf("failed to parse OPML: %w", err)
	}

	logger.Infof("📋 解析到 %d 个 RSS feed", len(outlines))
	reporter.ReportSuccess(fmt.Sprintf("解析到 %d 个RSS feed", len(outlines)))

	// 准备并发处理
	result := &SyncResult{
		TotalPodcasts: len(outlines),
		Errors:        []string{},
	}

	// 任务通道
	taskChan := make(chan opml.Outline, len(outlines))
	// 结果通道
	type importResult struct {
		podcast *models.Podcast
		err     error
		title   string
	}
	resultChan := make(chan importResult, len(outlines))

	// 统计（使用 mutex 保护并发安全）
	var mu sync.Mutex
	fromPodcastIndex := 0
	fromOnlineFetch := 0
	skippedCount := 0
	processedCount := 0

	startTime := time.Now()

	// 启动 worker pool
	var wg sync.WaitGroup
	for i := 0; i < config.Concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for outline := range taskChan {
				title := outline.GetTitle()
				logger.Infof("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ [Worker %d] %s", workerID, title)

				// 同步播客
				podcast, err := s.syncPodcastFromFeedWithRetry(&outline, outline.XMLURL, reporter)

				resultChan <- importResult{
					podcast: podcast,
					err:     err,
					title:   title,
				}
			}
		}(i)
	}

	// 发送任务到通道
	for _, outline := range outlines {
		taskChan <- outline
	}
	close(taskChan)

	// 等待所有 worker 完成
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// 处理结果
	for res := range resultChan {
		func() {
			// 使用匿名函数确保在panic时也能释放锁
			mu.Lock()
			defer mu.Unlock()

			processedCount++

			// 更新进度
			reporter.ReportProgress(processedCount, len(outlines),
				fmt.Sprintf("正在处理: %s", res.title))

			if res.err != nil {
				// 检查是否应该跳过（不报错）
				if shouldSkip, reasonStr, description := feed.GetSkipReasonFromError(res.err); shouldSkip {
					reason := SkipReason(reasonStr)
					skippedCount++
					reporter.ReportSkip(reason, fmt.Sprintf("%s - %s", res.title, description))
					return
				}

				// 其他错误
				result.FailedPodcasts++
				result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", res.title, res.err))
				reporter.ReportError(fmt.Sprintf("%s - %v", res.title, res.err))
				return
			}

			// 统计数据来源
			if res.podcast.DataSource == "podcastindex" {
				fromPodcastIndex++
			} else if res.podcast.DataSource == "rss" {
				fromOnlineFetch++
			}

			// 保存到数据库
			if err := s.saveOrUpdatePodcast(res.podcast); err != nil {
				result.FailedPodcasts++
				result.Errors = append(result.Errors, fmt.Sprintf("%s: save failed - %v", res.title, err))
				reporter.ReportError(fmt.Sprintf("保存失败: %s - %v", res.title, err))
				return
			}

			result.SuccessPodcasts++
			reporter.ReportSuccess(fmt.Sprintf("成功导入: %s", res.title))
		}() // 立即执行匿名函数
	}

	duration := time.Since(startTime)

	// 打印统计信息
	logger.Infof("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ 📊 导入统计")
	logger.Infof("✅ 成功: %d", result.SuccessPodcasts)
	logger.Infof("❌ 失败: %d", result.FailedPodcasts)
	logger.Infof("⏭️  跳过: %d", skippedCount)
	logger.Infof("📚 来自 PodcastIndex: %d (%.1f%%)", fromPodcastIndex, float64(fromPodcastIndex)*100/float64(len(outlines)))
	logger.Infof("🌐 来自在线抓取: %d (%.1f%%)", fromOnlineFetch, float64(fromOnlineFetch)*100/float64(len(outlines)))
	logger.Infof("⏱️  总耗时: %v", duration)
	logger.Infof("🚀 平均速度: %.2f 个/秒", float64(len(outlines))/duration.Seconds())
	logger.Infof("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	reporter.ReportSuccess(fmt.Sprintf("导入完成！成功: %d, 失败: %d, 跳过: %d, 耗时: %v",
		result.SuccessPodcasts, result.FailedPodcasts, skippedCount, duration))

	return result, nil
}

// ImportOPMLFromPodcastIndexOnly 从PodcastIndex导入并在线同步元数据
func (s *Service) ImportOPMLFromPodcastIndexOnly(filePath string, reporter ProgressReporter) (*SyncResult, error) {
	logger.Infof("🚀 开始导入OPML（智能模式）: %s", filePath)
	reporter.Report("开始导入OPML文件（智能模式：本地匹配+在线同步）: " + filePath)

	// 1. 解析OPML文件
	outlines, err := s.opmlParser.ParseFile(filePath)
	if err != nil {
		reporter.ReportError("解析OPML文件失败: " + err.Error())
		return nil, fmt.Errorf("failed to parse OPML: %w", err)
	}

	logger.Infof("📋 解析到 %d 个 RSS feed", len(outlines))
	reporter.ReportSuccess(fmt.Sprintf("解析到 %d 个RSS feed", len(outlines)))

	// 准备结果
	result := &SyncResult{
		TotalPodcasts: len(outlines),
		Errors:        []string{},
	}

	// 任务通道
	taskChan := make(chan opml.Outline, len(outlines))
	// 结果通道
	type importResult struct {
		podcast *models.Podcast
		err     error
		title   string
	}
	resultChan := make(chan importResult, len(outlines))

	// 统计（使用 mutex 保护并发安全）
	var mu sync.Mutex
	matchedCount := 0
	notFoundCount := 0
	processedCount := 0

	startTime := time.Now()

	// 启动 worker pool（使用较少的并发，因为是本地数据库查询）
	concurrency := 10
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for outline := range taskChan {
				title := outline.GetTitle()
				logger.Infof("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ [Worker %d] %s", workerID, title)

				// 仅从PodcastIndex查询，不在线抓取
				podcast, err := s.syncPodcastFromPodcastIndexOnly(&outline, outline.XMLURL, reporter)

				resultChan <- importResult{
					podcast: podcast,
					err:     err,
					title:   title,
				}
			}
		}(i)
	}

	// 发送任务到通道
	for _, outline := range outlines {
		taskChan <- outline
	}
	close(taskChan)

	// 等待所有 worker 完成
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// 处理结果
	for res := range resultChan {
		mu.Lock()
		processedCount++
		reporter.ReportProgress(processedCount, len(outlines),
			fmt.Sprintf("处理中: %d/%d", processedCount, len(outlines)))

		if res.err != nil {
			// 检查是否为跳过类型
			if shouldSkip, reasonStr, _ := feed.GetSkipReasonFromError(res.err); shouldSkip {
				notFoundCount++
				reason := SkipReason(reasonStr)
				reporter.ReportSkip(reason, fmt.Sprintf("%s - %s", res.title, res.err.Error()))
				logger.Infof("⏭️  [%d/%d] %s - 跳过: %v", processedCount, len(outlines), res.title, res.err)
			} else {
				result.FailedPodcasts++
				result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", res.title, res.err))
				reporter.ReportError(fmt.Sprintf("%s - %v", res.title, res.err))
				logger.Infof("❌ [%d/%d] %s - 失败: %v", processedCount, len(outlines), res.title, res.err)
			}
		} else if res.podcast != nil {
			// 保存播客
			if err := s.saveOrUpdatePodcast(res.podcast); err != nil {
				result.FailedPodcasts++
				result.Errors = append(result.Errors, fmt.Sprintf("%s: save failed - %v", res.title, err))
				reporter.ReportError(fmt.Sprintf("保存失败: %s - %v", res.title, err))
				logger.Infof("❌ [%d/%d] %s - 保存失败: %v", processedCount, len(outlines), res.title, err)
			} else {
				matchedCount++
				reporter.ReportSuccess(fmt.Sprintf("成功导入: %s", res.title))
				logger.Infof("✅ [%d/%d] %s - 成功", processedCount, len(outlines), res.title)
			}
		}
		mu.Unlock()
	}

	duration := time.Since(startTime)

	logger.Infof("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ 📊 导入汇总")
	logger.Infof("✅ 匹配成功: %d", matchedCount)
	logger.Infof("📭 未找到: %d", notFoundCount)
	logger.Infof("❌ 失败: %d", result.FailedPodcasts)
	logger.Infof("⏱️  总耗时: %v", duration)
	logger.Infof("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	reporter.ReportSuccess(fmt.Sprintf("导入完成！成功: %d, 未找到: %d, 失败: %d, 耗时: %v",
		matchedCount, notFoundCount, result.FailedPodcasts, duration))

	result.SuccessPodcasts = matchedCount
	return result, nil
}

// syncPodcastFromFeed 从RSS feed同步播客信息
func (s *Service) syncPodcastFromFeed(feedURL string) (*models.Podcast, error) {
	return s.syncPodcastFromFeedWithRetry(nil, feedURL, NewLogProgressReporter())
}

// syncPodcastFromFeedWithRetry 从RSS feed同步播客信息（带重试）。重试行为完全由
// feed.RetryPolicy 提供：单一可重试分类、Retry-After 与有界 full-jitter 退避；每次
// 重试都经 Fetcher/Coordinator，断路、按域并发、去重与 fallback 语义不被旁路。
func (s *Service) syncPodcastFromFeedWithRetry(outline *opml.Outline, feedURL string, reporter ProgressReporter) (*models.Podcast, error) {
	var lastErr error

	title := ""
	if outline != nil {
		title = outline.GetTitle()
	}

	logPrefix := ""
	if title != "" {
		logPrefix = fmt.Sprintf("[%s]", title)
	}

	logger.Infof("%s 🔍 开始同步 feed: %s", logPrefix, feedURL)

	// 尝试从PodcastIndex查询（不重试，因为它是本地数据库）
	if s.podcastIndexQuery != nil {
		logger.Infof("%s 📚 尝试从 PodcastIndex 查询...", logPrefix)

		var piInfo *podcastindex.PodcastInfo
		var err error

		// 策略: 使用 feed_url 匹配（带 http/https 转换）
		logger.Infof("%s   📌 尝试使用 feed_url 匹配: %s", logPrefix, feedURL)

		// 1. 先尝试原始URL
		piInfo, err = s.podcastIndexQuery.FindByFeedURL(feedURL)
		if err != nil {
			logger.Infof("%s   ⚠️  feed_url 查询出错: %v", logPrefix, err)
		}

		// 2. 如果原始URL未匹配，尝试 http/https 互换
		if piInfo == nil && err == nil {
			var altURL string
			if strings.HasPrefix(feedURL, "http://") {
				altURL = strings.Replace(feedURL, "http://", "https://", 1)
				logger.Infof("%s   🔄 尝试 https 转换: %s", logPrefix, altURL)
			} else if strings.HasPrefix(feedURL, "https://") {
				altURL = strings.Replace(feedURL, "https://", "http://", 1)
				logger.Infof("%s   🔄 尝试 http 转换: %s", logPrefix, altURL)
			}

			if altURL != "" {
				piInfo, err = s.podcastIndexQuery.FindByFeedURL(altURL)
				if err != nil {
					logger.Infof("%s   ⚠️  转换URL查询出错: %v", logPrefix, err)
				}
			}
		}

		// 3. 检查匹配结果
		if piInfo != nil {
			logger.Infof("%s   ✅ feed_url 匹配成功: %s (作者: %s)", logPrefix, piInfo.Title, piInfo.Author)
			reporter.Report(fmt.Sprintf("%s - 从本地数据库快速获取（feed_url匹配）", title))
			return s.createEnhancedPodcastFromOPML(piInfo, outline), nil
		} else {
			logger.Infof("%s   📭 feed_url 未找到，准备在线抓取", logPrefix)
		}
	} else {
		logger.Infof("%s ⚠️  PodcastIndex 未初始化，直接在线抓取", logPrefix)
	}

	// 对RSS feed抓取进行有限重试（预算、分类、Retry-After 与 full-jitter 退避均来自
	// feed.RetryPolicy，不在此另建旁路规则）。
	policy := s.retryPolicy
	logger.Infof("%s 🌐 开始在线抓取 RSS feed (最多重试 %d 次)", logPrefix, policy.Budget)

	for attempt := 0; attempt <= policy.Budget; attempt++ {
		if attempt > 0 {
			delay, _ := policy.NextDelay(lastErr, attempt-1)
			category := feed.CategoryOf(lastErr)
			retryAfter := feed.RetryAfterOf(lastErr)
			if title != "" {
				reporter.Report(fmt.Sprintf("%s - 第 %d 次重试中...", title, attempt))
			}
			logger.Infof("%s ⏳ 等待 %.0f 秒后重试 (category=%s retry_after=%q)...", logPrefix, delay.Seconds(), category, retryAfter)
			policy.Sleep(delay)
		}

		logger.Infof("%s 📡 正在抓取 (第 %d 次尝试)...", logPrefix, attempt+1)
		feedData, err := s.feedFetcher.FetchFeed(feedURL)
		if err == nil {
			logger.Infof("%s ✅ 抓取成功: %s", logPrefix, feedData.Title)
			// 成功
			if attempt > 0 && title != "" {
				reporter.ReportSuccess(fmt.Sprintf("%s - 重试成功", title))
			}
			return s.convertGofeedToModel(feedData, "rss", feedURL), nil
		}

		lastErr = err
		logger.Infof("%s ❌ 抓取失败 (第 %d 次尝试): %v", logPrefix, attempt+1, err)

		// 不可重试的错误（403/401/404/402/parse/重定向策略）立即返回，不消耗重试预算，
		// 也不会把已断路上游再次打向访问拒绝源。
		if !policy.ShouldRetry(err) {
			logger.Infof("%s ⛔ 不可重试的错误，停止重试: %v", logPrefix, err)
			return nil, err
		}
	}

	if title != "" {
		reporter.ReportError(fmt.Sprintf("%s - 重试 %d 次后仍然失败", title, policy.Budget))
	}
	logger.Infof("%s 💥 达到最大重试次数，放弃", logPrefix)
	return nil, fmt.Errorf("failed after %d retries: %w", policy.Budget, lastErr)
}

// syncPodcastFromPodcastIndexOnly 智能同步：先本地匹配，再在线抓取
func (s *Service) syncPodcastFromPodcastIndexOnly(outline *opml.Outline, feedURL string, reporter ProgressReporter) (*models.Podcast, error) {
	title := ""
	if outline != nil {
		title = outline.GetTitle()
	}

	logPrefix := ""
	if title != "" {
		logPrefix = fmt.Sprintf("[%s]", title)
	}

	logger.Infof("%s 🔍 智能同步模式: %s", logPrefix, feedURL)

	// 步骤1: 尝试从PodcastIndex本地数据库匹配
	var piInfo *podcastindex.PodcastInfo
	var err error

	if s.podcastIndexQuery != nil {
		// 1. 先尝试原始URL
		piInfo, err = s.podcastIndexQuery.FindByFeedURL(feedURL)
		if err != nil {
			logger.Infof("%s   ⚠️  feed_url 查询出错: %v", logPrefix, err)
		}

		// 2. 如果原始URL未匹配，尝试 http/https 互换
		if piInfo == nil && err == nil {
			var altURL string
			if strings.HasPrefix(feedURL, "http://") {
				altURL = strings.Replace(feedURL, "http://", "https://", 1)
				logger.Infof("%s   🔄 尝试 https 转换: %s", logPrefix, altURL)
			} else if strings.HasPrefix(feedURL, "https://") {
				altURL = strings.Replace(feedURL, "https://", "http://", 1)
				logger.Infof("%s   🔄 尝试 http 转换: %s", logPrefix, altURL)
			}

			if altURL != "" {
				piInfo, err = s.podcastIndexQuery.FindByFeedURL(altURL)
				if err != nil {
					logger.Infof("%s   ⚠️  转换URL查询出错: %v", logPrefix, err)
				}
			}
		}
	}

	// 步骤2: 根据匹配结果采取不同策略
	if piInfo != nil {
		// 情况A: 本地数据库匹配成功 - 在线抓取4个字段
		logger.Infof("%s   ✅ 本地数据库匹配成功: %s (作者: %s)", logPrefix, piInfo.Title, piInfo.Author)
		reporter.Report(fmt.Sprintf("%s - 本地匹配成功，正在更新元数据...", title))

		// 创建基础播客对象（从本地数据库）
		podcast := s.createEnhancedPodcastFromOPML(piInfo, outline)

		// 在线抓取4个关键字段
		logger.Infof("%s   🌐 在线更新元数据字段", logPrefix)
		updatedPodcast, updateErr := s.updatePodcastMetadataOnline(podcast, reporter)
		if updateErr != nil {
			logger.Infof("%s   ⚠️  在线更新失败: %v，使用本地数据库数据", logPrefix, updateErr)
			reporter.Report(fmt.Sprintf("%s - 在线更新失败，使用本地数据", title))
			// 即使在线更新失败，也返回本地数据
			return podcast, nil
		}

		logger.Infof("%s   ✅ 同步完成: %s", logPrefix, updatedPodcast.Title)
		reporter.Report(fmt.Sprintf("%s - 同步完成", title))
		return updatedPodcast, nil
	}

	// 情况B: 本地数据库未匹配 - 在线抓取完整信息
	logger.Infof("%s   📭 本地数据库未找到，尝试在线抓取...", logPrefix)
	reporter.Report(fmt.Sprintf("%s - 未在本地数据库找到，正在在线抓取...", title))

	podcast, fetchErr := s.fetchPodcastOnline(outline, feedURL, reporter)
	if fetchErr != nil {
		// 检查是否为永久性错误（402付费、SSL过期、403/404等）
		// 对于永久性错误，直接跳过，不创建数据库记录
		if shouldSkip, reasonStr, description := feed.GetSkipReasonFromError(fetchErr); shouldSkip {
			reason := SkipReason(reasonStr)
			logger.Infof("%s   ⏭️  永久性错误，跳过此feed: %s - %s", logPrefix, reasonStr, description)
			reporter.ReportSkip(reason, fmt.Sprintf("%s - %s", title, description))
			return nil, fetchErr
		}

		// 对于临时性错误（网络故障、超时等），创建基础记录以便稍后重试
		logger.Infof("%s   ⚠️  临时性错误，创建基础记录: %v", logPrefix, fetchErr)
		reporter.Report(fmt.Sprintf("%s - 临时错误，已创建基础记录（可稍后同步）", title))

		// 创建基础播客对象
		basePodcast := &models.Podcast{
			Title:        title,
			FeedURL:      feedURL,
			IsSubscribed: true,
			DataSource:   "rss",
			AddedDate:    time.Now(),
			FeedURLValid: false, // 标记为未验证，稍后可以通过同步功能重试
		}

		// 设置一些默认值
		basePodcast.EpisodeCount = 0
		basePodcast.Priority = 5
		basePodcast.PopularityScore = 0
		basePodcast.UpdateFrequency = 0

		// 尝试从outline获取更多信息
		if outline != nil {
			if outline.Text != "" {
				basePodcast.Title = outline.Text
			}
			if outline.HTMLURL != "" {
				basePodcast.Link = outline.HTMLURL
			}
		}

		return basePodcast, nil
	}

	logger.Infof("%s   ✅ 在线抓取成功: %s", logPrefix, podcast.Title)
	reporter.Report(fmt.Sprintf("%s - 在线抓取成功", title))
	return podcast, nil
}

// updatePodcastMetadataOnline 在线更新播客的4个关键字段
func (s *Service) updatePodcastMetadataOnline(podcast *models.Podcast, reporter ProgressReporter) (*models.Podcast, error) {
	logger.Infof("   🌐 抓取元数据: %s", podcast.FeedURL)

	// 在线抓取RSS feed
	gofeed, err := s.feedFetcher.FetchFeed(podcast.FeedURL)
	if err != nil {
		return nil, fmt.Errorf("抓取feed失败: %w", err)
	}

	// 提取4个关键字段
	updated := s.convertGofeedToModel(gofeed, podcast.DataSource, podcast.FeedURL)

	// 只更新这4个字段，保留其他字段
	podcast.EpisodeCount = updated.EpisodeCount
	podcast.NewestEpisodeDate = updated.NewestEpisodeDate
	podcast.NewestEnclosureURL = updated.NewestEnclosureURL
	podcast.NewestEnclosureDuration = updated.NewestEnclosureDuration

	now := time.Now()
	podcast.LastFetchedAt = &now
	podcast.FeedURLValid = true
	podcast.FetchErrorCount = 0

	logger.Infof("   ✅ 元数据更新成功: episode_count=%d, newest_episode_date=%v",
		podcast.EpisodeCount, podcast.NewestEpisodeDate)

	return podcast, nil
}

// fetchPodcastOnline 在线抓取完整播客信息
func (s *Service) fetchPodcastOnline(outline *opml.Outline, feedURL string, reporter ProgressReporter) (*models.Podcast, error) {
	logger.Infof("   🌐 在线抓取完整播客信息: %s", feedURL)

	// 在线抓取RSS feed
	gofeed, err := s.feedFetcher.FetchFeed(feedURL)
	if err != nil {
		// 分类错误类型
		classifiedErr := feed.ClassifyError(feedURL, err)
		return nil, classifiedErr
	}

	// 转换为播客模型
	podcast := s.convertGofeedToModel(gofeed, "rss", feedURL)

	// 设置额外字段
	if outline != nil {
		if podcast.Title == "" {
			podcast.Title = outline.GetTitle()
		}
		podcast.AddedDate = time.Now()
	}

	podcast.IsSubscribed = true

	now := time.Now()
	podcast.LastFetchedAt = &now
	podcast.FeedURLValid = true
	podcast.FetchErrorCount = 0

	logger.Infof("   ✅ 在线抓取完成: %s (episode_count=%d)", podcast.Title, podcast.EpisodeCount)

	return podcast, nil
}
