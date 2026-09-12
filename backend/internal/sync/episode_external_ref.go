package sync

import (
	"errors"
	"net/url"
	"strings"

	"magicpodcast/internal/models"

	"github.com/mmcdole/gofeed"
)

// errExternalRefPodcastMismatch 表示外部身份映射归属其他节目，拒绝合并。
var errExternalRefPodcastMismatch = errors.New("external ref belongs to another podcast")

// xiaoyuzhouEpisodePathSegment 从 URL 路径中识别小宇宙单集 ID。
// 与清单收录共享同一外部身份键；仅用于同步前的身份核对，不触发写库。
func xiaoyuzhouEpisodePathSegment(rawURL string) string {
	if rawURL == "" {
		return ""
	}

	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.User != nil || parsed.Port() != "" {
		return ""
	}
	host := strings.ToLower(parsed.Hostname())
	if host != "www.xiaoyuzhoufm.com" && host != "xiaoyuzhoufm.com" && host != "web.xiaoyuzhoufm.com" {
		return ""
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) != 2 || parts[0] != "episode" || !isXiaoyuzhouEpisodeID(parts[1]) {
		return ""
	}
	rest := parts[1]
	return rest
}

func isXiaoyuzhouEpisodeID(value string) bool {
	if len(value) != 24 {
		return false
	}
	for _, r := range value {
		switch {
		case r >= '0' && r <= '9':
		case r >= 'a' && r <= 'f':
		default:
			return false
		}
	}
	return true
}

// findExistingEpisodeByExternalRef 在创建前复用清单收录留下的外部身份映射：
// RSS 条目链接指向同一小宇宙单集且映射归属同一节目时，复用同一本地单集。
// 命中归属其他节目的映射时返回冲突，保持既有跨节目拒绝行为。
func (s *Service) findExistingEpisodeByExternalRef(podcast *models.Podcast, item *gofeed.Item) (*models.Episode, error) {
	if item == nil {
		return nil, nil
	}
	externalID := ""
	for _, candidate := range []string{item.Link, item.GUID} {
		if externalID = xiaoyuzhouEpisodePathSegment(candidate); externalID != "" {
			break
		}
	}
	if externalID == "" && isXiaoyuzhouEpisodeID(strings.TrimSpace(item.GUID)) {
		externalID = strings.TrimSpace(item.GUID)
	}
	if externalID == "" {
		return nil, nil
	}

	var refs []models.EpisodeExternalRef
	if err := s.db.Where("source_platform = ? AND external_episode_id = ?",
		models.SourcePlatformXiaoyuzhoufm, externalID).Find(&refs).Error; err != nil {
		return nil, err
	}
	for _, ref := range refs {
		var episode models.Episode
		if err := s.db.Unscoped().First(&episode, ref.EpisodeID).Error; err != nil {
			return nil, err
		}
		if episode.PodcastID != podcast.ID {
			// 映射归属其他节目：不跨节目合并，交由调用方按冲突处理。
			return &episode, errExternalRefPodcastMismatch
		}
		return &episode, nil
	}
	return nil, nil
}
