package sync

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"magicpodcast/internal/models"
	"magicpodcast/internal/opml"
	"magicpodcast/internal/podcastindex"

	"github.com/mmcdole/gofeed"
	"gorm.io/gorm"
)

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
		PodcastID: podcast.ID,
		Title:     item.Title,
		ShowNotes: item.Description,
		GUID:      item.GUID,
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
//
//	所有其他元数据从 PodcastIndex 获取
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
		Title:              info.Title,
		Author:             info.Author,
		Description:        info.Description,
		CoverURL:           info.CoverURL,
		FeedURL:            info.FeedURL,
		ITunesID:           fmt.Sprintf("%d", info.ITunesID),
		Link:               info.WebsiteURL,         // 🆕 播客网站链接
		NewestEnclosureURL: info.NewestEnclosureURL, // 🆕 最新单集音频URL
		EpisodeCount:       info.EpisodeCount,       // 🆕 单集总数
		IsSubscribed:       true,
		DataSource:         "podcastindex",
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
		ITunesID:     extractITunesID(feed),
		PodcastGUID:  extractPodcastGUID(feed),
		IsSubscribed: true,
		DataSource:   dataSource,
	}

	if feed.Author != nil {
		podcast.Author = feed.Author.Name
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

		// 查找最新单集（按发布时间排序；不使用 RSS 更新时间或抓取时间）
		var newestItem *gofeed.Item
		var newestTime time.Time

		for i, item := range feed.Items {
			var itemTime time.Time
			if item.PublishedParsed != nil {
				itemTime = *item.PublishedParsed
			} else if item.UpdatedParsed != nil {
				// 少数 feed 缺少发布时间时保留可用的时间兜底，但正常排序
				// 永远以发布时间为准。
				itemTime = *item.UpdatedParsed
			}

			if newestItem == nil || (i == 0 && newestTime.IsZero()) || itemTime.After(newestTime) {
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

// extractPodcastGUID reads the standard Podcast Namespace GUID when the RSS
// publisher provides it. gofeed keeps less common namespace fields in the
// generic extension map, so this helper also tolerates common capitalization
// variants without treating a title or URL as an identity.
func extractPodcastGUID(parsedFeed *gofeed.Feed) string {
	if parsedFeed == nil {
		return ""
	}
	for namespace, fields := range parsedFeed.Extensions {
		if !strings.EqualFold(namespace, "podcast") {
			continue
		}
		for field, values := range fields {
			if !strings.EqualFold(field, "guid") && !strings.EqualFold(field, "podcastGuid") {
				continue
			}
			if len(values) > 0 {
				return strings.TrimSpace(values[0].Value)
			}
		}
	}
	return ""
}

func extractITunesID(parsedFeed *gofeed.Feed) string {
	if parsedFeed == nil {
		return ""
	}
	for namespace, fields := range parsedFeed.Extensions {
		if !strings.EqualFold(namespace, "itunes") {
			continue
		}
		for field, values := range fields {
			if !strings.EqualFold(field, "id") || len(values) == 0 {
				continue
			}
			value := strings.TrimSpace(values[0].Value)
			if parseITunesID(value) > 0 {
				return strconv.Itoa(parseITunesID(value))
			}
		}
	}
	return ""
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
