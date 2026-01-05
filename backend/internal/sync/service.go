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

// ImportOPMLFromPodcastIndexOnly 仅从PodcastIndex本地数据库导入OPML（不在线抓取）
func (s *Service) ImportOPMLFromPodcastIndexOnly(filePath string, reporter ProgressReporter) (*SyncResult, error) {
	log.Printf("🚀 开始导入OPML（仅本地数据库）: %s", filePath)
	reporter.Report("开始导入OPML文件（仅本地PodcastIndex匹配）: " + filePath)

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

// syncPodcastFromPodcastIndexOnly 仅从PodcastIndex本地数据库查询播客信息（不在线抓取）
func (s *Service) syncPodcastFromPodcastIndexOnly(outline *opml.Outline, feedURL string, reporter ProgressReporter) (*models.Podcast, error) {
	title := ""
	if outline != nil {
		title = outline.GetTitle()
	}

	logPrefix := ""
	if title != "" {
		logPrefix = fmt.Sprintf("[%s]", title)
	}

	log.Printf("%s 🔍 从本地数据库查询: %s", logPrefix, feedURL)

	// 如果PodcastIndex未初始化，返回错误
	if s.podcastIndexQuery == nil {
		err := fmt.Errorf("PodcastIndex未初始化")
		log.Printf("%s ❌ %v", logPrefix, err)
		reporter.Report(fmt.Sprintf("%s - 未在本地数据库找到", title))
		return nil, err
	}

	// 尝试从PodcastIndex查询
	var piInfo *podcastindex.PodcastInfo
	var err error

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
		log.Printf("%s   ✅ 本地数据库匹配成功: %s (作者: %s)", logPrefix, piInfo.Title, piInfo.Author)
		reporter.Report(fmt.Sprintf("%s - 从本地数据库获取", title))
		return s.createEnhancedPodcastFromOPML(piInfo, outline), nil
	}

	// 未找到
	log.Printf("%s   📭 本地数据库未找到", logPrefix)
	reporter.Report(fmt.Sprintf("%s - 未在本地数据库找到（需要在线同步）", title))
	return nil, fmt.Errorf("未在本地PodcastIndex数据库中找到: %s", feedURL)
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
