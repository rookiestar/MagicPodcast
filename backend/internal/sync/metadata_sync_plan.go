package sync

import (
	"fmt"
	"time"

	"magicpodcast/internal/logger"
	"magicpodcast/internal/models"
)

type podcastMetadataUpdateCheck struct {
	hasUpdate bool
	reasons   []string
}

type episodeSyncPlan struct {
	mode       EpisodeSyncMode
	shouldSync bool
}

func detectPodcastMetadataUpdate(current *models.Podcast, updated *models.Podcast) podcastMetadataUpdateCheck {
	check := podcastMetadataUpdateCheck{}

	if updated.Title != "" && updated.Title != current.Title {
		check.hasUpdate = true
		check.reasons = append(check.reasons, fmt.Sprintf("title: %s -> %s", current.Title, updated.Title))
	}
	if updated.Description != "" && updated.Description != current.Description {
		check.hasUpdate = true
		check.reasons = append(check.reasons, "description changed")
	}
	if updated.Author != "" && updated.Author != current.Author {
		check.hasUpdate = true
		check.reasons = append(check.reasons, "author changed")
	}
	if updated.EpisodeCount != current.EpisodeCount {
		check.hasUpdate = true
		check.reasons = append(check.reasons, fmt.Sprintf("episode_count: %d -> %d", current.EpisodeCount, updated.EpisodeCount))
	}

	if !updated.NewestEpisodeDate.IsZero() {
		if current.NewestEpisodeDate.IsZero() {
			check.hasUpdate = true
			check.reasons = append(check.reasons, fmt.Sprintf("newest_episode_date: zero -> %s", updated.NewestEpisodeDate))
		} else if !updated.NewestEpisodeDate.Equal(current.NewestEpisodeDate) {
			check.hasUpdate = true
			check.reasons = append(check.reasons, fmt.Sprintf("newest_episode_date: %s -> %s", current.NewestEpisodeDate, updated.NewestEpisodeDate))
		}
	}

	if updated.NewestEnclosureURL != current.NewestEnclosureURL {
		check.hasUpdate = true
		check.reasons = append(check.reasons, "newest_enclosure_url changed")
	}

	if updated.CoverURL != current.CoverURL {
		check.hasUpdate = true
		check.reasons = append(check.reasons, "cover_url changed")
	}

	if updated.ITunesID != "" && updated.ITunesID != current.ITunesID {
		check.hasUpdate = true
		check.reasons = append(check.reasons, "itunes_id discovered")
	}
	if updated.PodcastGUID != "" && updated.PodcastGUID != current.PodcastGUID {
		check.hasUpdate = true
		check.reasons = append(check.reasons, "podcast_guid discovered")
	}

	return check
}

// podcastMetadataUpdates 是元数据检查的写入白名单。标题/作者/简介/网站
// 属于源站管理字段，随检查更新；缺失字段不得清空已有可信信息（#398 R6）。
func podcastMetadataUpdates(updated *models.Podcast) map[string]interface{} {
	updates := map[string]interface{}{
		"cover_url":                 updated.CoverURL,
		"episode_count":             updated.EpisodeCount,
		"newest_episode_date":       updated.NewestEpisodeDate,
		"newest_enclosure_url":      updated.NewestEnclosureURL,
		"newest_enclosure_duration": updated.NewestEnclosureDuration,
		"last_fetched_at":           time.Now(),
		"fetch_error_count":         0,
		"feed_url_valid":            true,
	}
	if updated.Title != "" {
		updates["title"] = updated.Title
	}
	if updated.Description != "" {
		updates["description"] = updated.Description
	}
	if updated.Author != "" {
		updates["author"] = updated.Author
	}
	if updated.Link != "" {
		updates["link"] = updated.Link
	}
	if updated.ITunesID != "" {
		updates["i_tunes_id"] = updated.ITunesID
	}
	if updated.PodcastGUID != "" {
		updates["podcast_guid"] = updated.PodcastGUID
	}
	return updates
}

// planEpisodeSync 决定元数据检查后是否执行单集同步。只要成功读取到 Feed
// 内容就同步：集数、最新日期和封面不变不能证明旧单集内容相同，数量相等
// 时同样可能存在 Show Notes/标题的实质修订（#398 R7）。实际写入仍由
// episodeNeedsUpdate 控制，未变化的单集不会产生写放大。
func planEpisodeSync(podcastTitle string, hasMetadataUpdate bool, existingEpisodeCount int64, feedEpisodeCount int64) episodeSyncPlan {
	if existingEpisodeCount == 0 {
		logger.Infof("   [%s] 无单集，使用全量同步", podcastTitle)
		return episodeSyncPlan{mode: SyncModeFull, shouldSync: true}
	}

	if hasMetadataUpdate {
		logger.Infof("   [%s] 元数据有更新，使用全量同步", podcastTitle)
		return episodeSyncPlan{mode: SyncModeFull, shouldSync: true}
	}

	logger.Infof("   [%s] 元数据无更新，仍检查已有单集内容修订 (数据库:%d, feed:%d)",
		podcastTitle, existingEpisodeCount, feedEpisodeCount)
	return episodeSyncPlan{mode: SyncModeFull, shouldSync: true}
}
