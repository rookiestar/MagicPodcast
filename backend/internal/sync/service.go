package sync

import (
	"fmt"
	"log"
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
	return s.ImportOPMLWithProgress(filePath, NewLogProgressReporter())
}

// ImportOPMLWithProgress 导入OPML文件（带进度报告）
func (s *Service) ImportOPMLWithProgress(filePath string, reporter ProgressReporter) (*SyncResult, error) {
	log.Printf("🚀 开始导入OPML: %s", filePath)
	reporter.Report("开始导入OPML文件: " + filePath)

	// 1. 解析OPML文件
	outlines, err := s.opmlParser.ParseFile(filePath)
	if err != nil {
		reporter.ReportError("解析OPML文件失败: " + err.Error())
		return nil, fmt.Errorf("failed to parse OPML: %w", err)
	}

	log.Printf("📋 解析到 %d 个 RSS feed", len(outlines))
	reporter.ReportSuccess(fmt.Sprintf("解析到 %d 个RSS feed", len(outlines)))

	result := &SyncResult{
		TotalPodcasts: len(outlines),
		Errors:       []string{},
	}

	// 统计信息
	fromPodcastIndex := 0
	fromOnlineFetch := 0
	skippedCount := 0

	startTime := time.Now()

	// 2. 对每个RSS feed进行数据获取
	for i, outline := range outlines {
		log.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ [%d/%d] %s", i+1, len(outlines), outline.Title)

		reporter.ReportProgress(i+1, len(outlines),
			fmt.Sprintf("正在处理: %s", outline.Title))

		podcast, err := s.syncPodcastFromFeedWithRetry(outline.XMLURL, outline.Title, reporter, DefaultRetryConfig)
		if err != nil {
			// 检查是否应该跳过（不报错）
			if shouldSkip, reasonStr, description := feed.GetSkipReasonFromError(err); shouldSkip {
				reason := SkipReason(reasonStr)
				skippedCount++
				reporter.ReportSkip(reason, fmt.Sprintf("%s - %s", outline.Title, description))
				continue
			}

			// 其他错误
			result.FailedPodcasts++
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", outline.Title, err))
			reporter.ReportError(fmt.Sprintf("%s - %v", outline.Title, err))
			continue
		}

		// 统计数据来源
		if podcast.DataSource == "podcastindex" {
			fromPodcastIndex++
		} else if podcast.DataSource == "rss" {
			fromOnlineFetch++
		}

		// 保存到数据库
		if err := s.saveOrUpdatePodcast(podcast); err != nil {
			result.FailedPodcasts++
			result.Errors = append(result.Errors, fmt.Sprintf("%s: save failed - %v", outline.Title, err))
			reporter.ReportError(fmt.Sprintf("保存失败: %s - %v", outline.Title, err))
			continue
		}

		result.SuccessPodcasts++
		reporter.ReportSuccess(fmt.Sprintf("成功导入: %s", outline.Title))
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
	log.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	reporter.ReportSuccess(fmt.Sprintf("导入完成！成功: %d, 失败: %d, 跳过: %d, 耗时: %v",
		result.SuccessPodcasts, result.FailedPodcasts, skippedCount, duration))

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
	return s.syncPodcastFromFeedWithRetry(feedURL, "", NewLogProgressReporter(), DefaultRetryConfig)
}

// syncPodcastFromFeedWithRetry 从RSS feed同步播客信息（带重试）
func (s *Service) syncPodcastFromFeedWithRetry(feedURL string, title string, reporter ProgressReporter, config RetryConfig) (*models.Podcast, error) {
	var lastErr error
	var delay time.Duration

	logPrefix := ""
	if title != "" {
		logPrefix = fmt.Sprintf("[%s]", title)
	}

	log.Printf("%s 🔍 开始同步 feed: %s", logPrefix, feedURL)

	// 尝试从PodcastIndex查询（不重试，因为它是本地数据库）
	if s.podcastIndexQuery != nil {
		log.Printf("%s 📚 尝试从 PodcastIndex 查询...", logPrefix)
		piInfo, err := s.podcastIndexQuery.FindByFeedURL(feedURL)
		if err != nil {
			log.Printf("%s ⚠️  PodcastIndex 查询出错: %v", logPrefix, err)
		} else if piInfo != nil {
			log.Printf("%s ✅ 从 PodcastIndex 找到: %s (作者: %s)", logPrefix, piInfo.Title, piInfo.Author)
			reporter.Report(fmt.Sprintf("%s - 从本地数据库快速获取", title))
			return s.convertPodcastIndexToModel(piInfo), nil
		} else {
			log.Printf("%s 📭 PodcastIndex 中未找到，准备在线抓取", logPrefix)
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

// convertPodcastIndexToModel 将PodcastIndex信息转换为模型
func (s *Service) convertPodcastIndexToModel(info *podcastindex.PodcastInfo) *models.Podcast {
	return &models.Podcast{
		Title:       info.Title,
		Author:      info.Author,
		Description: info.Description,
		CoverURL:    info.CoverURL,
		FeedURL:     info.FeedURL,
		ITunesID:    fmt.Sprintf("%d", info.ITunesID),
		IsSubscribed: true,
		DataSource:  "podcastindex",
	}
}

// convertGofeedToModel 将gofeed转换为模型
func (s *Service) convertGofeedToModel(feed *gofeed.Feed, dataSource string, feedURL string) *models.Podcast {
	podcast := &models.Podcast{
		Title:       feed.Title,
		Description: feed.Description,
		FeedURL:     feedURL, // 使用传入的feedURL
		Author:      feed.Author.Name,
		IsSubscribed: true,
		DataSource:  dataSource,
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

	return podcast
}
