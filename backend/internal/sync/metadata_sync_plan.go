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
	if updated.ITunesID != "" {
		updates["i_tunes_id"] = updated.ITunesID
	}
	if updated.PodcastGUID != "" {
		updates["podcast_guid"] = updated.PodcastGUID
	}
	return updates
}

func planEpisodeSync(podcastTitle string, hasMetadataUpdate bool, existingEpisodeCount int64, feedEpisodeCount int64) episodeSyncPlan {
	if existingEpisodeCount == 0 {
		logger.Infof("   [%s] 无单集，使用全量同步", podcastTitle)
		return episodeSyncPlan{mode: SyncModeFull, shouldSync: true}
	}

	if hasMetadataUpdate {
		logger.Infof("   [%s] 元数据有更新，使用全量同步", podcastTitle)
		return episodeSyncPlan{mode: SyncModeFull, shouldSync: true}
	}

	if existingEpisodeCount != feedEpisodeCount {
		logger.Infof("   [%s] 单集数量不匹配 (数据库:%d, feed:%d)，使用全量同步", podcastTitle, existingEpisodeCount, feedEpisodeCount)
		return episodeSyncPlan{mode: SyncModeFull, shouldSync: true}
	}

	logger.Infof("   [%s] 元数据无更新且单集数量匹配(%d)，跳过单集同步", podcastTitle, existingEpisodeCount)
	return episodeSyncPlan{shouldSync: false}
}
