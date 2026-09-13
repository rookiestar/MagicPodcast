package sync

import (
	"net/url"
	"strconv"
	"strings"

	"magicpodcast/internal/models"

	"gorm.io/gorm"
)

// identityMatchKind 描述导入条目与本地节目记录的匹配方式。只有精确
// feed_url 与已验证的外部身份（小宇宙节目 ID）参与复用；标题或协议替换
// 本身不构成同档证据（#398/#401）。
type identityMatchKind string

const (
	identityNone      identityMatchKind = ""
	identityByFeedURL identityMatchKind = "feed_url"
	identityByXyzID   identityMatchKind = "xyz_id"
	identityDeleted   identityMatchKind = "deleted"
)

// resolvedPodcastIdentity 是写入前的身份核对结果。
type resolvedPodcastIdentity struct {
	kind    identityMatchKind
	podcast *models.Podcast
}

// xiaoyuzhouPodcastPathSegment 从 URL 路径中识别小宇宙节目 ID。
// 覆盖 /podcast/<id> 与 /podcast/<id>.rss 两种形态；与清单收录的 xyz_id
// 共享同一外部身份键，仅用于导入前的身份核对。
func xiaoyuzhouPodcastPathSegment(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	host, segments := splitXiaoyuzhouPath(rawURL)
	if host == "" || len(segments) != 2 || segments[0] != "podcast" {
		return ""
	}
	id := segments[1]
	if idx := strings.Index(id, "."); idx > 0 {
		id = id[:idx]
	}
	if !isXiaoyuzhouEpisodeID(id) {
		return ""
	}
	return id
}

func splitXiaoyuzhouPath(rawURL string) (host string, segments []string) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.User != nil || parsed.Port() != "" {
		return "", nil
	}
	host = strings.ToLower(parsed.Hostname())
	if host != "www.xiaoyuzhoufm.com" && host != "xiaoyuzhoufm.com" && host != "web.xiaoyuzhoufm.com" {
		return "", nil
	}
	return host, strings.Split(strings.Trim(parsed.Path, "/"), "/")
}

// resolveImportIdentity 在写入前核对导入条目的本地身份。查询使用 Unscoped
// 以便识别软删除记录：已删除的记录不静默复活，只作为跳过原因上报。
func (s *Service) resolveImportIdentity(feedURL string) resolvedPodcastIdentity {
	if feedURL == "" {
		return resolvedPodcastIdentity{kind: identityNone}
	}

	var podcast models.Podcast
	err := s.db.Unscoped().Where("feed_url = ?", feedURL).First(&podcast).Error
	if err == nil {
		if podcast.DeletedAt.Valid {
			return resolvedPodcastIdentity{kind: identityDeleted, podcast: &podcast}
		}
		return resolvedPodcastIdentity{kind: identityByFeedURL, podcast: &podcast}
	}
	if err != gorm.ErrRecordNotFound {
		return resolvedPodcastIdentity{kind: identityNone}
	}

	// 小宇宙节目页/订阅地址携带外部节目 ID：命中清单收录创建的无 RSS
	// 记录时，构成可确认的同档证据（xyz_id 一致）。
	if xyzID := xiaoyuzhouPodcastPathSegment(feedURL); xyzID != "" {
		var byXyz models.Podcast
		if err := s.db.Unscoped().Where("xyz_id = ?", xyzID).First(&byXyz).Error; err == nil {
			if byXyz.DeletedAt.Valid {
				return resolvedPodcastIdentity{kind: identityDeleted, podcast: &byXyz}
			}
			return resolvedPodcastIdentity{kind: identityByXyzID, podcast: &byXyz}
		}
	}

	return resolvedPodcastIdentity{kind: identityNone}
}

// identityConflictCandidate 是导入期（抓取到 Feed 后）发现的稳定身份冲突：
// 抓取到的节目身份已被另一条本地记录占用。只有单一候选且用户确认时才
// 允许合并；多候选一律跳过并展示证据。
type identityConflictCandidate struct {
	PodcastID   uint   `json:"podcast_id"`
	Title       string `json:"title"`
	FeedURL     string `json:"feed_url"`
	Evidence    string `json:"evidence"`
	Subscribed  bool   `json:"subscribed"`
	FeedMissing bool   `json:"feed_missing"`
	Deleted     bool   `json:"deleted"`
}

// findIdentityConflictCandidates 返回与抓取身份相同的本地记录（排除
// excludeID）。evidence 标注命中的身份字段。
func (s *Service) findIdentityConflictCandidates(fetched *models.Podcast, excludeID uint) []identityConflictCandidate {
	candidates := make([]identityConflictCandidate, 0)
	seen := make(map[uint]struct{})

	appendCandidate := func(podcast models.Podcast, evidence string) {
		if podcast.ID == 0 || podcast.ID == excludeID {
			return
		}
		if _, exists := seen[podcast.ID]; exists {
			// 同一记录同时命中两个身份字段时合并证据。
			for i := range candidates {
				if candidates[i].PodcastID == podcast.ID {
					candidates[i].Evidence += "+" + evidence
				}
			}
			return
		}
		seen[podcast.ID] = struct{}{}
		candidates = append(candidates, identityConflictCandidate{
			PodcastID:   podcast.ID,
			Title:       podcast.Title,
			FeedURL:     podcast.FeedURL,
			Evidence:    evidence,
			Subscribed:  podcast.IsSubscribed,
			FeedMissing: podcast.FeedURL == "",
			Deleted:     podcast.DeletedAt.Valid,
		})
	}

	if guid := normalizeIdentity(fetched.PodcastGUID); guid != "" {
		var matches []models.Podcast
		if err := s.db.Unscoped().Where("podcast_guid = ? AND id <> ?", guid, excludeID).Find(&matches).Error; err == nil {
			for _, match := range matches {
				appendCandidate(match, "podcast_guid")
			}
		}
	}
	if itunesID := parseITunesID(fetched.ITunesID); itunesID > 0 {
		var matches []models.Podcast
		if err := s.db.Unscoped().Where("i_tunes_id = ? AND id <> ?", strconv.Itoa(itunesID), excludeID).Find(&matches).Error; err == nil {
			for _, match := range matches {
				appendCandidate(match, "itunes_id")
			}
		}
	}
	return candidates
}
