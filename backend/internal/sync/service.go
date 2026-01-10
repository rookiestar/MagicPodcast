package sync

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"magicpodcast/internal/feed"
	"magicpodcast/internal/models"
	"magicpodcast/internal/opml"
	"magicpodcast/internal/podcastindex"
	"magicpodcast/internal/scraper"

	"github.com/mmcdole/gofeed"
	"gorm.io/gorm"
)

// RetryConfig 重试配置
type RetryConfig struct {
	MaxRetries    int           // 最大重试次数
	InitialDelay  time.Duration // 初始延迟
	MaxDelay      time.Duration // 最大延迟
	BackoffFactor float64       // 退避因子
}

// DefaultRetryConfig 默认重试配置
var DefaultRetryConfig = RetryConfig{
	MaxRetries:    3,
	InitialDelay:  1 * time.Second,
	MaxDelay:      8 * time.Second,
	BackoffFactor: 2.0,
}

// ImportConfig 导入配置
type ImportConfig struct {
	Concurrency int // 并发数（默认 10）
}

// DefaultImportConfig 默认导入配置
var DefaultImportConfig = ImportConfig{
	Concurrency: 10,
}

// Service 同步服务
type Service struct {
	db                *gorm.DB
	opmlParser        *opml.Parser
	feedFetcher       *feed.Fetcher
	podcastIndexQuery *podcastindex.Query
	scraper           *scraper.Scraper
}

// SyncResult 同步结果
type SyncResult struct {
	TotalPodcasts   int     `json:"total_podcasts"`
	SuccessPodcasts int     `json:"success_podcasts"`
	FailedPodcasts  int     `json:"failed_podcasts"`
	NewEpisodes     int     `json:"new_episodes"`
	Errors          []string `json:"errors,omitempty"`
}

// EpisodeSyncMode Episode同步模式
type EpisodeSyncMode string

const (
	SyncModeIncremental EpisodeSyncMode = "incremental" // 增量同步
	SyncModeFull        EpisodeSyncMode = "full"        // 全量同步
	SyncModeSmart       EpisodeSyncMode = "smart"       // 智能模式
)

// EpisodeSyncConfig Episode同步配置
type EpisodeSyncConfig struct {
	Mode                EpisodeSyncMode // 同步模式
	MaxEpisodesPerPodcast int           // 单次同步最大episode数（防止单个podcast过大）
	UpdateExisting      bool           // 是否更新已存在的episode
	DeleteMissing       bool           // 是否删除feed中不存在的episode（谨慎使用）
}

// DefaultEpisodeSyncConfig 默认Episode同步配置
var DefaultEpisodeSyncConfig = EpisodeSyncConfig{
	Mode:                SyncModeSmart,
	MaxEpisodesPerPodcast: 1000, // 默认最多同步1000个episode
	UpdateExisting:      true,  // 更新已存在的episode
	DeleteMissing:       false, // 不自动删除（安全考虑）
}

// EpisodeSyncResult Episode同步结果
type EpisodeSyncResult struct {
	PodcastID    uint   `json:"podcast_id"`
	PodcastTitle string `json:"podcast_title"`
	Created      int    `json:"created"`   // 新增数量
	Updated      int    `json:"updated"`   // 更新数量
	Skipped      int    `json:"skipped"`   // 跳过数量
	Deleted      int    `json:"deleted"`   // 删除数量
	Errors       int    `json:"errors"`    // 错误数量
}

// NewService 创建同步服务
func NewService(db *gorm.DB, podcastIndexPath string) (*Service, error) {
	// 初始化PodcastIndex查询器
	podcastIndexQuery, err := podcastindex.NewQuery(podcastIndexPath)
	if err != nil {
		log.Printf("Warning: Failed to initialize PodcastIndex query: %v", err)
		// 不返回错误，继续创建服务（PodcastIndex是可选的）
		podcastIndexQuery = nil
	}

	return &Service{
		db:                db,
		opmlParser:        opml.NewParser(),
		feedFetcher:       feed.NewFetcher(30 * time.Second),
		podcastIndexQuery: podcastIndexQuery,
		scraper:           scraper.NewScraper(),
	}, nil
}

// Close 关闭服务，释放资源
func (s *Service) Close() error {
	if s.podcastIndexQuery != nil {
		return s.podcastIndexQuery.Close()
	}
	return nil
}

// ImportOPML 导入OPML文件
func (s *Service) ImportOPML(filePath string) (*SyncResult, error) {
	return s.ImportOPMLWithProgressAndConfig(filePath, NewLogProgressReporter(), DefaultImportConfig)
}

// ImportOPMLWithProgress 导入OPML文件（带进度报告，使用默认并发配置）
func (s *Service) ImportOPMLWithProgress(filePath string, reporter ProgressReporter) (*SyncResult, error) {
	return s.ImportOPMLWithProgressAndConfig(filePath, reporter, DefaultImportConfig)
}

// ImportOPMLWithProgressAndConfig 导入OPML文件（带进度报告和自定义并发配置）
func (s *Service) ImportOPMLWithProgressAndConfig(filePath string, reporter ProgressReporter, config ImportConfig) (*SyncResult, error) {
	log.Printf("🚀 开始导入OPML (并发度: %d): %s", config.Concurrency, filePath)
	reporter.Report("开始导入OPML文件: " + filePath)

	// 1. 解析OPML文件
	outlines, err := s.opmlParser.ParseFile(filePath)
	if err != nil {
		reporter.ReportError("解析OPML文件失败: " + err.Error())
		return nil, fmt.Errorf("failed to parse OPML: %w", err)
	}

	log.Printf("📋 解析到 %d 个 RSS feed", len(outlines))
	reporter.ReportSuccess(fmt.Sprintf("解析到 %d 个RSS feed", len(outlines)))

	// 准备并发处理
	result := &SyncResult{
		TotalPodcasts: len(outlines),
		Errors:       []string{},
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
				log.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ [Worker %d] %s", workerID, title)

				// 同步播客
				podcast, err := s.syncPodcastFromFeedWithRetry(&outline, outline.XMLURL, reporter, DefaultRetryConfig)

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

		// 更新进度
		reporter.ReportProgress(processedCount, len(outlines),
			fmt.Sprintf("正在处理: %s", res.title))

		if res.err != nil {
			// 检查是否应该跳过（不报错）
			if shouldSkip, reasonStr, description := feed.GetSkipReasonFromError(res.err); shouldSkip {
				reason := SkipReason(reasonStr)
				skippedCount++
				reporter.ReportSkip(reason, fmt.Sprintf("%s - %s", res.title, description))
				mu.Unlock()
				continue
			}

			// 其他错误
			result.FailedPodcasts++
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", res.title, res.err))
			reporter.ReportError(fmt.Sprintf("%s - %v", res.title, res.err))
			mu.Unlock()
			continue
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
			mu.Unlock()
			continue
		}

		result.SuccessPodcasts++
		reporter.ReportSuccess(fmt.Sprintf("成功导入: %s", res.title))
		mu.Unlock()
	}

	duration := time.Since(startTime)

	// 打印统计信息
	log.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ 📊 导入统计")
	log.Printf("✅ 成功: %d", result.SuccessPodcasts)
	log.Printf("❌ 失败: %d", result.FailedPodcasts)
	log.Printf("⏭️  跳过: %d", skippedCount)
	log.Printf("📚 来自 PodcastIndex: %d (%.1f%%)", fromPodcastIndex, float64(fromPodcastIndex)*100/float64(len(outlines)))
	log.Printf("🌐 来自在线抓取: %d (%.1f%%)", fromOnlineFetch, float64(fromOnlineFetch)*100/float64(len(outlines)))
	log.Printf("⏱️  总耗时: %v", duration)
	log.Printf("🚀 平均速度: %.2f 个/秒", float64(len(outlines))/duration.Seconds())
	log.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	reporter.ReportSuccess(fmt.Sprintf("导入完成！成功: %d, 失败: %d, 跳过: %d, 耗时: %v",
		result.SuccessPodcasts, result.FailedPodcasts, skippedCount, duration))

	return result, nil
}

// ImportOPMLFromPodcastIndexOnly 从PodcastIndex导入并在线同步元数据
func (s *Service) ImportOPMLFromPodcastIndexOnly(filePath string, reporter ProgressReporter) (*SyncResult, error) {
	log.Printf("🚀 开始导入OPML（智能模式）: %s", filePath)
	reporter.Report("开始导入OPML文件（智能模式：本地匹配+在线同步）: " + filePath)

	// 1. 解析OPML文件
	outlines, err := s.opmlParser.ParseFile(filePath)
	if err != nil {
		reporter.ReportError("解析OPML文件失败: " + err.Error())
		return nil, fmt.Errorf("failed to parse OPML: %w", err)
	}

	log.Printf("📋 解析到 %d 个 RSS feed", len(outlines))
	reporter.ReportSuccess(fmt.Sprintf("解析到 %d 个RSS feed", len(outlines)))

	// 准备结果
	result := &SyncResult{
		TotalPodcasts: len(outlines),
		Errors:       []string{},
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
				log.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ [Worker %d] %s", workerID, title)

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
				log.Printf("⏭️  [%d/%d] %s - 跳过: %v", processedCount, len(outlines), res.title, res.err)
			} else {
				result.FailedPodcasts++
				result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", res.title, res.err))
				reporter.ReportError(fmt.Sprintf("%s - %v", res.title, res.err))
				log.Printf("❌ [%d/%d] %s - 失败: %v", processedCount, len(outlines), res.title, res.err)
			}
		} else if res.podcast != nil {
			// 保存播客
			if err := s.saveOrUpdatePodcast(res.podcast); err != nil {
				result.FailedPodcasts++
				result.Errors = append(result.Errors, fmt.Sprintf("%s: save failed - %v", res.title, err))
				reporter.ReportError(fmt.Sprintf("保存失败: %s - %v", res.title, err))
				log.Printf("❌ [%d/%d] %s - 保存失败: %v", processedCount, len(outlines), res.title, err)
			} else {
				matchedCount++
				reporter.ReportSuccess(fmt.Sprintf("成功导入: %s", res.title))
				log.Printf("✅ [%d/%d] %s - 成功", processedCount, len(outlines), res.title)
			}
		}
		mu.Unlock()
	}

	duration := time.Since(startTime)

	log.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ 📊 导入汇总")
	log.Printf("✅ 匹配成功: %d", matchedCount)
	log.Printf("📭 未找到: %d", notFoundCount)
	log.Printf("❌ 失败: %d", result.FailedPodcasts)
	log.Printf("⏱️  总耗时: %v", duration)
	log.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	reporter.ReportSuccess(fmt.Sprintf("导入完成！成功: %d, 未找到: %d, 失败: %d, 耗时: %v",
		matchedCount, notFoundCount, result.FailedPodcasts, duration))

	result.SuccessPodcasts = matchedCount
	return result, nil
}

// SyncAllPodcasts 同步所有订阅的播客（定时任务）
func (s *Service) SyncAllPodcasts() (*SyncResult, error) {
	log.Println("开始同步所有播客...")

	var podcasts []models.Podcast
	if err := s.db.Where("is_subscribed = ?", true).Find(&podcasts).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch podcasts: %w", err)
	}

	result := &SyncResult{
		TotalPodcasts: len(podcasts),
		Errors:       []string{},
	}

	for _, podcast := range podcasts {
		// 检查feed是否有效
		if !podcast.FeedURLValid {
			log.Printf("跳过无效feed: %s", podcast.Title)
			continue
		}

		// 增量抓取RSS
		newEpisodes, err := s.fetchNewEpisodes(&podcast)
		if err != nil {
			result.FailedPodcasts++
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", podcast.Title, err))

			// 更新错误计数
			podcast.FetchErrorCount++
			if podcast.FetchErrorCount >= 3 {
				podcast.FeedURLValid = false
			}
			s.db.Save(&podcast)

			log.Printf("❌ 同步失败: %s - %v", podcast.Title, err)
			continue
		}

		// 保存新单集
		for _, episode := range newEpisodes {
			if err := s.saveEpisode(&podcast, episode); err != nil {
				log.Printf("❌ 保存单集失败: %v", err)
			} else {
				result.NewEpisodes++
			}
		}

		// 更新最后抓取时间，重置错误计数
		now := time.Now()
		podcast.LastFetchedAt = &now
		podcast.FetchErrorCount = 0
		podcast.FeedURLValid = true
		s.db.Save(&podcast)

		result.SuccessPodcasts++
	}

	return result, nil
}

// syncPodcastFromFeed 从RSS feed同步播客信息
func (s *Service) syncPodcastFromFeed(feedURL string) (*models.Podcast, error) {
	return s.syncPodcastFromFeedWithRetry(nil, feedURL, NewLogProgressReporter(), DefaultRetryConfig)
}

// syncPodcastFromFeedWithRetry 从RSS feed同步播客信息（带重试）
func (s *Service) syncPodcastFromFeedWithRetry(outline *opml.Outline, feedURL string, reporter ProgressReporter, config RetryConfig) (*models.Podcast, error) {
	var lastErr error
	var delay time.Duration

	title := ""
	if outline != nil {
		title = outline.GetTitle()
	}

	logPrefix := ""
	if title != "" {
		logPrefix = fmt.Sprintf("[%s]", title)
	}

	log.Printf("%s 🔍 开始同步 feed: %s", logPrefix, feedURL)

	// 尝试从PodcastIndex查询（不重试，因为它是本地数据库）
	if s.podcastIndexQuery != nil {
		log.Printf("%s 📚 尝试从 PodcastIndex 查询...", logPrefix)

		var piInfo *podcastindex.PodcastInfo
		var err error

		// 策略: 使用 feed_url 匹配（带 http/https 转换）
		log.Printf("%s   📌 尝试使用 feed_url 匹配: %s", logPrefix, feedURL)

		// 1. 先尝试原始URL
		piInfo, err = s.podcastIndexQuery.FindByFeedURL(feedURL)
		if err != nil {
			log.Printf("%s   ⚠️  feed_url 查询出错: %v", logPrefix, err)
		}

		// 2. 如果原始URL未匹配，尝试 http/https 互换
		if piInfo == nil && err == nil {
			var altURL string
			if strings.HasPrefix(feedURL, "http://") {
				altURL = strings.Replace(feedURL, "http://", "https://", 1)
				log.Printf("%s   🔄 尝试 https 转换: %s", logPrefix, altURL)
			} else if strings.HasPrefix(feedURL, "https://") {
				altURL = strings.Replace(feedURL, "https://", "http://", 1)
				log.Printf("%s   🔄 尝试 http 转换: %s", logPrefix, altURL)
			}

			if altURL != "" {
				piInfo, err = s.podcastIndexQuery.FindByFeedURL(altURL)
				if err != nil {
					log.Printf("%s   ⚠️  转换URL查询出错: %v", logPrefix, err)
				}
			}
		}

		// 3. 检查匹配结果
		if piInfo != nil {
			log.Printf("%s   ✅ feed_url 匹配成功: %s (作者: %s)", logPrefix, piInfo.Title, piInfo.Author)
			reporter.Report(fmt.Sprintf("%s - 从本地数据库快速获取（feed_url匹配）", title))
			return s.createEnhancedPodcastFromOPML(piInfo, outline), nil
		} else {
			log.Printf("%s   📭 feed_url 未找到，准备在线抓取", logPrefix)
		}
	} else {
		log.Printf("%s ⚠️  PodcastIndex 未初始化，直接在线抓取", logPrefix)
	}

	// 对RSS feed抓取进行重试
	log.Printf("%s 🌐 开始在线抓取 RSS feed (最多重试 %d 次)", logPrefix, config.MaxRetries)

	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		if attempt > 0 {
			// 计算延迟时间（指数退避）
			delay = time.Duration(float64(config.InitialDelay) * float64(int(1)<<uint(attempt-1)))
			if delay > config.MaxDelay {
				delay = config.MaxDelay
			}

			if title != "" {
				reporter.Report(fmt.Sprintf("%s - 第 %d 次重试中...", title, attempt))
			}
			log.Printf("%s ⏳ 等待 %.0f 秒后重试...", logPrefix, delay.Seconds())
			time.Sleep(delay)
		}

		log.Printf("%s 📡 正在抓取 (第 %d 次尝试)...", logPrefix, attempt+1)
		feedData, err := s.feedFetcher.FetchFeed(feedURL)
		if err == nil {
			log.Printf("%s ✅ 抓取成功: %s", logPrefix, feedData.Title)
			// 成功
			if attempt > 0 && title != "" {
				reporter.ReportSuccess(fmt.Sprintf("%s - 重试成功", title))
			}
			return s.convertGofeedToModel(feedData, "rss", feedURL), nil
		}

		lastErr = err
		log.Printf("%s ❌ 抓取失败 (第 %d 次尝试): %v", logPrefix, attempt+1, err)

		// 检查是否为可重试错误
		if !feed.IsRetryable(err) {
			// 不可重试的错误，直接返回
			log.Printf("%s ⛔ 不可重试的错误，停止重试: %v", logPrefix, err)
			return nil, err
		}

		// 如果是最后一次尝试，不再重试
		if attempt >= config.MaxRetries {
			if title != "" {
				reporter.ReportError(fmt.Sprintf("%s - 重试 %d 次后仍然失败", title, config.MaxRetries))
			}
			log.Printf("%s 💥 达到最大重试次数，放弃", logPrefix)
			return nil, fmt.Errorf("failed after %d retries: %w", config.MaxRetries, lastErr)
		}

		// 记录重试信息
		if title != "" {
			reporter.Report(fmt.Sprintf("%s - 网络错误，将在 %.0f 秒后重试...", title, delay.Seconds()))
		}
	}

	return nil, lastErr
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

	log.Printf("%s 🔍 智能同步模式: %s", logPrefix, feedURL)

	// 步骤1: 尝试从PodcastIndex本地数据库匹配
	var piInfo *podcastindex.PodcastInfo
	var err error

	if s.podcastIndexQuery != nil {
		// 1. 先尝试原始URL
		piInfo, err = s.podcastIndexQuery.FindByFeedURL(feedURL)
		if err != nil {
			log.Printf("%s   ⚠️  feed_url 查询出错: %v", logPrefix, err)
		}

		// 2. 如果原始URL未匹配，尝试 http/https 互换
		if piInfo == nil && err == nil {
			var altURL string
			if strings.HasPrefix(feedURL, "http://") {
				altURL = strings.Replace(feedURL, "http://", "https://", 1)
				log.Printf("%s   🔄 尝试 https 转换: %s", logPrefix, altURL)
			} else if strings.HasPrefix(feedURL, "https://") {
				altURL = strings.Replace(feedURL, "https://", "http://", 1)
				log.Printf("%s   🔄 尝试 http 转换: %s", logPrefix, altURL)
			}

			if altURL != "" {
				piInfo, err = s.podcastIndexQuery.FindByFeedURL(altURL)
				if err != nil {
					log.Printf("%s   ⚠️  转换URL查询出错: %v", logPrefix, err)
				}
			}
		}
	}

	// 步骤2: 根据匹配结果采取不同策略
	if piInfo != nil {
		// 情况A: 本地数据库匹配成功 - 在线抓取4个字段
		log.Printf("%s   ✅ 本地数据库匹配成功: %s (作者: %s)", logPrefix, piInfo.Title, piInfo.Author)
		reporter.Report(fmt.Sprintf("%s - 本地匹配成功，正在更新元数据...", title))

		// 创建基础播客对象（从本地数据库）
		podcast := s.createEnhancedPodcastFromOPML(piInfo, outline)

		// 在线抓取4个关键字段
		log.Printf("%s   🌐 在线更新元数据字段", logPrefix)
		updatedPodcast, updateErr := s.updatePodcastMetadataOnline(podcast, reporter)
		if updateErr != nil {
			log.Printf("%s   ⚠️  在线更新失败: %v，使用本地数据库数据", logPrefix, updateErr)
			reporter.Report(fmt.Sprintf("%s - 在线更新失败，使用本地数据", title))
			// 即使在线更新失败，也返回本地数据
			return podcast, nil
		}

		log.Printf("%s   ✅ 同步完成: %s", logPrefix, updatedPodcast.Title)
		reporter.Report(fmt.Sprintf("%s - 同步完成", title))
		return updatedPodcast, nil
	}

	// 情况B: 本地数据库未匹配 - 在线抓取完整信息
	log.Printf("%s   📭 本地数据库未找到，尝试在线抓取...", logPrefix)
	reporter.Report(fmt.Sprintf("%s - 未在本地数据库找到，正在在线抓取...", title))

	podcast, fetchErr := s.fetchPodcastOnline(outline, feedURL, reporter)
	if fetchErr != nil {
		// 在线抓取失败 - 创建一个基础播客记录（至少保存title和feedURL）
		log.Printf("%s   ⚠️  在线抓取失败: %v，创建基础记录", logPrefix, fetchErr)
		reporter.Report(fmt.Sprintf("%s - 在线抓取失败，已创建基础记录（可稍后同步）", title))

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

	log.Printf("%s   ✅ 在线抓取成功: %s", logPrefix, podcast.Title)
	reporter.Report(fmt.Sprintf("%s - 在线抓取成功", title))
	return podcast, nil
}

// updatePodcastMetadataOnline 在线更新播客的4个关键字段
func (s *Service) updatePodcastMetadataOnline(podcast *models.Podcast, reporter ProgressReporter) (*models.Podcast, error) {
	log.Printf("   🌐 抓取元数据: %s", podcast.FeedURL)

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

	log.Printf("   ✅ 元数据更新成功: episode_count=%d, newest_episode_date=%v",
		podcast.EpisodeCount, podcast.NewestEpisodeDate)

	return podcast, nil
}

// fetchPodcastOnline 在线抓取完整播客信息
func (s *Service) fetchPodcastOnline(outline *opml.Outline, feedURL string, reporter ProgressReporter) (*models.Podcast, error) {
	log.Printf("   🌐 在线抓取完整播客信息: %s", feedURL)

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

	log.Printf("   ✅ 在线抓取完成: %s (episode_count=%d)", podcast.Title, podcast.EpisodeCount)

	return podcast, nil
}

// fetchNewEpisodes 获取新单集（增量）
func (s *Service) fetchNewEpisodes(podcast *models.Podcast) ([]*gofeed.Item, error) {
	var lastFetchTime time.Time
	if podcast.LastFetchedAt != nil {
		lastFetchTime = *podcast.LastFetchedAt
	} else {
		// 如果没有抓取过，使用7天前
		lastFetchTime = time.Now().AddDate(0, 0, -7)
	}

	result, err := s.feedFetcher.FetchIncremental(podcast.FeedURL, lastFetchTime)
	if err != nil {
		return nil, err
	}

	return result.NewItems, nil
}

// saveOrUpdatePodcast 保存或更新播客
func (s *Service) saveOrUpdatePodcast(podcast *models.Podcast) error {
	var existing models.Podcast

	// 尝试通过feed_url查找现有播客
	err := s.db.Where("feed_url = ?", podcast.FeedURL).First(&existing).Error

	if err == gorm.ErrRecordNotFound {
		// 新播客，检查xyz_id是否为空
		if podcast.XYZID == "" {
			// 如果xyz_id为空，生成一个临时的唯一ID
			podcast.XYZID = "temp_" + feedURLToID(podcast.FeedURL)
		}
		podcast.AddedDate = time.Now()
		return s.db.Create(podcast).Error
	} else if err != nil {
		return err
	}

	// 更新现有播客（保留用户自定义数据）
	podcast.ID = existing.ID
	podcast.XYZID = existing.XYZID // 保留原有的xyz_id
	podcast.Notes = existing.Notes
	podcast.MyRate = existing.MyRate
	podcast.CreatedAt = existing.CreatedAt
	podcast.AddedDate = existing.AddedDate // 保留原有的添加日期
	return s.db.Save(podcast).Error
}

// feedURLToID 从feed URL生成唯一ID
func feedURLToID(feedURL string) string {
	// 使用简单的hash算法生成唯一ID
	hash := 5381
	for _, c := range feedURL {
		hash = ((hash << 5) + hash) + int(c)
	}
	return fmt.Sprintf("opml_%d", hash)
}

// saveEpisode 保存单集
func (s *Service) saveEpisode(podcast *models.Podcast, item *gofeed.Item) error {
	// 检查是否已存在（通过GUID）
	if item.GUID != "" {
		var count int64
		s.db.Model(&models.Episode{}).Where("guid = ?", item.GUID).Count(&count)
		if count > 0 {
			return nil // 已存在，跳过
		}
	}

	episode := &models.Episode{
		PodcastID:    podcast.ID,
		Title:        item.Title,
		ShowNotes:    item.Description,
		GUID:         item.GUID,
	}

	// 获取音频URL（从enclosures数组）
	if len(item.Enclosures) > 0 {
		episode.MediumURL = item.Enclosures[0].URL
	}

	if item.PublishedParsed != nil {
		episode.PublishedDate = *item.PublishedParsed
	}

	now := time.Now()
	episode.FetchedAt = &now

	return s.db.Create(episode).Error
}
// createEnhancedPodcastFromOPML 从 PodcastIndex 和 OPML 创建播客
// 策略：只保留 OPML 的核心字段（title, description, feed_url）
//       所有其他元数据从 PodcastIndex 获取
func (s *Service) createEnhancedPodcastFromOPML(
	piInfo *podcastindex.PodcastInfo,
	outline *opml.Outline,
) *models.Podcast {
	podcast := &models.Podcast{}

	// === 从 OPML 保留（仅核心字段） ===
	podcast.Title = outline.GetTitle()
	podcast.FeedURL = outline.XMLURL

	// description：保留 OPML 的（来自 text），如果为空则用 PodcastIndex 的
	if outline.GetDescription() != "" {
		podcast.Description = outline.GetDescription()
	} else {
		podcast.Description = piInfo.Description
	}

	// === 从 PodcastIndex 获取（所有其他字段） ===
	// link（即使 OPML 有 htmlUrl，也用 PodcastIndex 的）
	podcast.Link = piInfo.WebsiteURL

	// author
	podcast.Author = piInfo.Author

	// cover_url
	podcast.CoverURL = piInfo.CoverURL

	// iTunes ID
	if piInfo.ITunesID > 0 {
		podcast.ITunesID = fmt.Sprintf("%d", piInfo.ITunesID)
	}

	// PodcastIndex 特有字段
	podcast.NewestEnclosureURL = piInfo.NewestEnclosureURL
	podcast.NewestEnclosureDuration = piInfo.NewestEnclosureDuration
	podcast.EpisodeCount = piInfo.EpisodeCount

	// 时间戳字段
	if piInfo.NewestItemPubdate > 0 {
		t := time.Unix(piInfo.NewestItemPubdate, 0)
		podcast.NewestEpisodeDate = t
	}

	if piInfo.LastUpdate > 0 {
		t := time.Unix(piInfo.LastUpdate, 0)
		podcast.LastUpdate = &t
	}

	if piInfo.OldestItemPubdate > 0 {
		t := time.Unix(piInfo.OldestItemPubdate, 0)
		podcast.OldestEpisodeDate = &t
	}

	// 评分字段
	if piInfo.PopularityScore > 0 {
		podcast.PopularityScore = piInfo.PopularityScore
	}

	if piInfo.Priority >= -1 {
		podcast.Priority = piInfo.Priority
	}

	if piInfo.UpdateFrequency >= 0 {
		podcast.UpdateFrequency = piInfo.UpdateFrequency
	}

	// 元数据
	podcast.DataSource = "podcastindex"
	podcast.IsSubscribed = true
	podcast.FeedURLValid = true

	return podcast
}

// convertPodcastIndexToModel 将PodcastIndex信息转换为模型
func (s *Service) convertPodcastIndexToModel(info *podcastindex.PodcastInfo) *models.Podcast {
	podcast := &models.Podcast{
		Title:           info.Title,
		Author:          info.Author,
		Description:     info.Description,
		CoverURL:        info.CoverURL,
		FeedURL:         info.FeedURL,
		ITunesID:        fmt.Sprintf("%d", info.ITunesID),
		Link:            info.WebsiteURL,              // 🆕 播客网站链接
		NewestEnclosureURL: info.NewestEnclosureURL,  // 🆕 最新单集音频URL
		EpisodeCount:     info.EpisodeCount,          // 🆕 单集总数
		IsSubscribed:     true,
		DataSource:       "podcastindex",
	}

	// 🆕 处理时间戳字段
	if info.NewestItemPubdate > 0 {
		t := time.Unix(info.NewestItemPubdate, 0)
		podcast.NewestEpisodeDate = t
	}

	if info.LastUpdate > 0 {
		t := time.Unix(info.LastUpdate, 0)
		podcast.LastUpdate = &t
	}

	if info.OldestItemPubdate > 0 {
		t := time.Unix(info.OldestItemPubdate, 0)
		podcast.OldestEpisodeDate = &t
	}

	// 🆕 处理评分字段
	if info.PopularityScore > 0 {
		podcast.PopularityScore = info.PopularityScore
	}

	// 🆕 处理优先级（允许 -1 表示暂停）
	if info.Priority >= -1 {
		podcast.Priority = info.Priority
	}

	// 🆕 处理更新频率
	if info.UpdateFrequency >= 0 {
		podcast.UpdateFrequency = info.UpdateFrequency
	}

	// 🆕 处理最新单集时长
	if info.NewestEnclosureDuration > 0 {
		podcast.NewestEnclosureDuration = info.NewestEnclosureDuration
	}

	return podcast
}

// convertGofeedToModel 将gofeed转换为模型
func (s *Service) convertGofeedToModel(feed *gofeed.Feed, dataSource string, feedURL string) *models.Podcast {
	podcast := &models.Podcast{
		Title:        feed.Title,
		Description:  feed.Description,
		FeedURL:      feedURL, // 使用传入的feedURL
		Author:       feed.Author.Name,
		IsSubscribed: true,
		DataSource:   dataSource,
	}

	if feed.Image != nil {
		podcast.CoverURL = feed.Image.URL
	}

	if feed.ITunesExt != nil {
		if feed.ITunesExt.Author != "" {
			podcast.Author = feed.ITunesExt.Author
		}
		if feed.ITunesExt.Image != "" {
			podcast.CoverURL = feed.ITunesExt.Image
		}
	}

	// 提取单集统计信息和最新单集数据
	if len(feed.Items) > 0 {
		podcast.EpisodeCount = len(feed.Items)

		// 查找最新单集（按发布时间排序）
		var newestItem *gofeed.Item
		var newestTime time.Time

		for i, item := range feed.Items {
			var itemTime time.Time
			if item.UpdatedParsed != nil {
				itemTime = *item.UpdatedParsed
			} else if item.PublishedParsed != nil {
				itemTime = *item.PublishedParsed
			}

			if i == 0 || itemTime.After(newestTime) {
				newestTime = itemTime
				newestItem = item
			}
		}

		// 设置最新单集信息
		if newestItem != nil {
			if !newestTime.IsZero() {
				podcast.NewestEpisodeDate = newestTime
			}

			// 获取最新单集的音频URL
			if len(newestItem.Enclosures) > 0 {
				podcast.NewestEnclosureURL = newestItem.Enclosures[0].URL
			}

			// 尝试从iTunes扩展获取时长（格式为HH:MM:SS或MM:SS或秒数）
			if newestItem.ITunesExt != nil && newestItem.ITunesExt.Duration != "" {
				podcast.NewestEnclosureDuration = parseITunesDuration(newestItem.ITunesExt.Duration)
			}
		}
	}

	return podcast
}

// parseITunesDuration 解析iTunes时长格式为秒数
// 支持格式：HH:MM:SS, MM:SS, 或纯数字秒数
func parseITunesDuration(duration string) int {
	if duration == "" {
		return 0
	}

	// 尝试按冒号分割
	parts := strings.Split(duration, ":")
	seconds := 0

	switch len(parts) {
	case 3: // HH:MM:SS
		if h, err := strconv.Atoi(parts[0]); err == nil {
			seconds += h * 3600
		}
		if m, err := strconv.Atoi(parts[1]); err == nil {
			seconds += m * 60
		}
		if s, err := strconv.Atoi(parts[2]); err == nil {
			seconds += s
		}
	case 2: // MM:SS
		if m, err := strconv.Atoi(parts[0]); err == nil {
			seconds += m * 60
		}
		if s, err := strconv.Atoi(parts[1]); err == nil {
			seconds += s
		}
	case 1: // 纯数字秒数
		if s, err := strconv.Atoi(parts[0]); err == nil {
			seconds = s
		}
	}

	return seconds
}

// SyncPodcastsMetadataSSE 同步所有播客的元数据（SSE流式进度报告）
// 这个函数会在线抓取每个播客的RSS feed，更新元数据字段（episode_count, newest_episode_date等）
func (s *Service) SyncPodcastsMetadataSSE(reporter ProgressReporter) error {
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
		podcast *models.Podcast
		err     error
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
				err := s.syncPodcastMetadata(podcast, reporter)
				resultChan <- syncResult{
					podcast: podcast,
					err:     err,
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
	current := 0

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
				reporter.ReportError(fmt.Sprintf("[%d/%d] %s - %s", current, total, podcast.Title, result.err.Error()))
			}
		} else {
			successCount++
			reporter.ReportSuccess(fmt.Sprintf("[%d/%d] 成功同步: %s", current, total, podcast.Title))
		}

		// 每10个播客报告一次进度
		if current%10 == 0 || current == total {
			reporter.ReportProgress(current, total, fmt.Sprintf("已处理 %d/%d", current, total))
		}
	}

	// 发送最终结果
	log.Printf("✅ 元数据同步完成: 成功=%d, 失败=%d, 跳过=%d", successCount, failedCount, skippedCount)
	reporter.ReportSuccess(fmt.Sprintf("同步完成！成功: %d, 失败: %d, 跳过: %d", successCount, failedCount, skippedCount))

	return nil
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

// convertGofeedItemToEpisode 将gofeed.Item转换为Episode模型
func (s *Service) convertGofeedItemToEpisode(podcast *models.Podcast, item *gofeed.Item) *models.Episode {
	episode := &models.Episode{
		PodcastID: podcast.ID,
		Title:     item.Title,
		ShowNotes: item.Description,
		Link:      item.Link,
		Content:   item.Content,
	}

	// 设置GUID：优先使用RSS GUID，其次Link，最后EnclosureURL或Hash
	// GUID用于唯一标识和去重
	if item.GUID != "" {
		episode.GUID = item.GUID
	} else if item.Link != "" {
		episode.GUID = item.Link
	} else if len(item.Enclosures) > 0 && item.Enclosures[0].URL != "" {
		episode.GUID = item.Enclosures[0].URL
	} else {
		// 极少数情况：都没有，使用hash生成唯一ID
		episode.GUID = generateHashID(fmt.Sprintf("%d-%s", podcast.ID, item.Title))
	}

	// 音频信息
	if len(item.Enclosures) > 0 {
		episode.MediumURL = item.Enclosures[0].URL
		episode.EnclosureType = item.Enclosures[0].Type
		if item.Enclosures[0].Length != "" {
			if length, err := strconv.ParseInt(item.Enclosures[0].Length, 10, 64); err == nil {
				episode.EnclosureLength = length
			}
		}
	}

	// 时间信息
	if item.PublishedParsed != nil {
		episode.PublishedDate = *item.PublishedParsed
	}
	if item.UpdatedParsed != nil {
		episode.UpdatedDate = item.UpdatedParsed
	}

	// iTunes扩展 - 解析时长
	if item.ITunesExt != nil && item.ITunesExt.Duration != "" {
		episode.Duration = parseITunesDuration(item.ITunesExt.Duration)
	}

	// 图片
	if item.Image != nil {
		episode.ImageURL = item.Image.URL
	}

	now := time.Now()
	episode.FetchedAt = &now

	return episode
}

// generateHashID 生成基于字符串的hash ID（用于GUID的兜底方案）
func generateHashID(input string) string {
	hash := 5381
	for _, c := range input {
		hash = ((hash << 5) + hash) + int(c)
	}
	return fmt.Sprintf("ep_%d", hash)
}

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
		// 全量模式，使用很久以前的时间确保获取所有内容
		lastFetchTime = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

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
				lastFetchTime = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
			}
		} else {
			// 从未抓取过，使用全量模式
			useIncremental = false
			lastFetchTime = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
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

		// 报告进度
		if result.Created > 0 || result.Updated > 0 {
			reporter.ReportSuccess(fmt.Sprintf("[%d/%d] %s - 新增: %d, 更新: %d",
				processedCount, total, result.PodcastTitle, result.Created, result.Updated))
		}

		// 每10个podcast报告一次进度
		if processedCount%10 == 0 || processedCount == total {
			reporter.ReportProgress(processedCount, total, fmt.Sprintf("已处理 %d/%d", processedCount, total))
		}
	}

	// 6. 发送最终结果
	log.Printf("✅ 所有podcast的episodes同步完成: 新增: %d, 更新: %d, 跳过: %d, 错误: %d",
		totalCreated, totalUpdated, totalSkipped, totalErrors)
	reporter.ReportSuccess(fmt.Sprintf("同步完成！新增: %d, 更新: %d, 跳过: %d, 错误: %d",
		totalCreated, totalUpdated, totalSkipped, totalErrors))

	return nil
}
