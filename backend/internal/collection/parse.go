// Package collection 实现外部单集清单的读取、解析、预览与保存。
// 首版仅支持小宇宙公开单集清单的确定性解析；来源内容不是产品指令。
package collection

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"magicpodcast/internal/models"
)

// 解析失败分类：调用方据此区分来源受限、格式不支持、结构缺失与真正空清单。
var (
	ErrIncompleteSource      = errors.New("collection page is missing required structure")
	ErrUnsupportedTargetType = errors.New("only episode collections are supported")
	ErrEmptyCollection       = errors.New("collection contains no episode entries")
	ErrDuplicateItems        = errors.New("collection contains duplicate episode identities")
)

// PlatformXiaoyuzhoufm 当前唯一支持的清单来源平台。
const PlatformXiaoyuzhoufm = models.SourcePlatformXiaoyuzhoufm

// PayTypeFree 小宇宙免费单集标记；其他取值（如 PAY_EPISODE_PODCAST）代表付费受限。
const PayTypeFree = "FREE"

// imagePayload 源站图片字段。
type imagePayload struct {
	PicURL string `json:"picUrl"`
}

// podcastRefPayload 条目内嵌的节目标识与元数据。
type podcastRefPayload struct {
	PID          string        `json:"pid"`
	Title        string        `json:"title"`
	Author       string        `json:"author"`
	Image        *imagePayload `json:"image"`
	EpisodeCount json.Number   `json:"episodeCount"`
}

// itemDraft 是解析输出的一条清单条目快照，不携带任何个人库状态。
type ItemDraft struct {
	ExternalEpisodeID   string
	ExternalPodcastID   string
	PodcastTitle        string
	PodcastAuthor       string
	PodcastCoverURL     string
	EpisodeTitle        string
	Recommendation      string
	Shownotes           string
	Duration            int
	PublishedAt         *time.Time
	ImageURL            string
	EpisodeURL          string
	PayType             string
	IsPrivateMedia      bool
	AudioURL            string
	AudioMimeType       string
	AudioSize           int64
	PodcastEpisodeCount int
}

// Draft 是一份清单的确定性解析结果。
type Draft struct {
	Platform        string
	ExternalID      string
	Title           string
	Description     string
	Author          string
	SourceURL       string
	SourceCreatedAt *time.Time
	TotalKnown      bool // 当前源格式没有总数或分页标记，恒为 false；保留字段避免语义含混
	Items           []ItemDraft
}

// nextData 只解码需要的字段；未识别字段忽略。
type nextData struct {
	Props struct {
		PageProps struct {
			Collection *collectionPayload `json:"collection"`
		} `json:"pageProps"`
	} `json:"props"`
}

type collectionPayload struct {
	ID          string            `json:"id"`
	Title       string            `json:"title"`
	Description string            `json:"description"`
	TargetType  string            `json:"targetType"`
	Target      []json.RawMessage `json:"target"`
	Author      *authorRefPayload `json:"author"`
	CreatedAt   string            `json:"createdAt"`
	HasMore     bool              `json:"hasMore"`
	TotalCount  *int              `json:"totalCount"`
	LoadMoreKey json.RawMessage   `json:"loadMoreKey"`
}

type authorRefPayload struct {
	Nickname string `json:"nickname"`
}

type enclosurePayload struct {
	URL  string `json:"url"`
	Type string `json:"type"`
}

type mediaPayload struct {
	ID       string      `json:"id"`
	Size     json.Number `json:"size"`
	MIMEType string      `json:"mimeType"`
}

type itemPayload struct {
	Type      string             `json:"type"`
	EID       string             `json:"eid"`
	PID       string             `json:"pid"`
	Title     string             `json:"title"`
	Shownotes string             `json:"shownotes"`
	Image     *imagePayload      `json:"image"`
	Podcast   *podcastRefPayload `json:"podcast"`
	Enclosure *enclosurePayload  `json:"enclosure"`
	Media     *mediaPayload      `json:"media"`

	IsPrivateMedia bool   `json:"isPrivateMedia"`
	PubDate        string `json:"pubDate"`
	Duration       *int   `json:"duration"`
	PayType        string `json:"payType"`

	Recommendation string `json:"recommendation"`
}

var nextDataScriptPattern = regexp.MustCompile(
	`(?is)<script[^>]*\bid="__NEXT_DATA__"[^>]*>(.*?)</script>`)

// ParsePageHTML 从清单页 HTML 中确定性解析清单元数据与条目。
// expectedExternalID 是从用户提交 URL 中识别出的清单 ID，与页面数据不一致时视为
// 结构不可信并拒绝导入；传空则跳过该核对（仅测试使用）。
func ParsePageHTML(html string, expectedExternalID string) (*Draft, error) {
	script := nextDataScriptPattern.FindStringSubmatch(html)
	if len(script) != 2 {
		return nil, fmt.Errorf("%w: __NEXT_DATA__ script not found", ErrIncompleteSource)
	}

	var data nextData
	if err := json.Unmarshal([]byte(script[1]), &data); err != nil {
		return nil, fmt.Errorf("%w: decode __NEXT_DATA__: %v", ErrIncompleteSource, err)
	}
	payload := data.Props.PageProps.Collection
	if payload == nil {
		return nil, fmt.Errorf("%w: collection object missing", ErrIncompleteSource)
	}
	if strings.TrimSpace(payload.ID) == "" || strings.TrimSpace(payload.Title) == "" {
		return nil, fmt.Errorf("%w: collection id/title missing", ErrIncompleteSource)
	}
	if expectedExternalID != "" && payload.ID != expectedExternalID {
		return nil, fmt.Errorf("%w: collection id %q does not match requested %q",
			ErrIncompleteSource, payload.ID, expectedExternalID)
	}
	// 首版仅支持单集清单；节目型清单明确拒绝而不是猜测条目。
	if payload.TargetType != "EPISODE" {
		return nil, fmt.Errorf("%w: targetType=%q", ErrUnsupportedTargetType, payload.TargetType)
	}
	if payload.Target == nil {
		return nil, fmt.Errorf("%w: collection target missing", ErrIncompleteSource)
	}

	cursor := strings.TrimSpace(string(payload.LoadMoreKey))
	if payload.HasMore || (payload.TotalCount != nil && *payload.TotalCount != len(payload.Target)) || (cursor != "" && cursor != "null" && cursor != `""` && cursor != `{}`) {
		return nil, fmt.Errorf("%w: remaining page or total mismatch", ErrIncompleteSource)
	}

	draft := &Draft{
		Platform:        PlatformXiaoyuzhoufm,
		ExternalID:      payload.ID,
		Title:           strings.TrimSpace(payload.Title),
		Description:     strings.TrimSpace(payload.Description),
		SourceCreatedAt: parseOptionalTime(payload.CreatedAt),
		TotalKnown:      false,
		Items:           make([]ItemDraft, 0, len(payload.Target)),
	}
	if payload.Author != nil {
		draft.Author = strings.TrimSpace(payload.Author.Nickname)
	}

	seen := make(map[string]struct{}, len(payload.Target))
	for index, raw := range payload.Target {
		var item itemPayload
		if err := json.Unmarshal(raw, &item); err != nil {
			return nil, fmt.Errorf("%w: decode target item %d: %v", ErrIncompleteSource, index, err)
		}
		if strings.TrimSpace(item.EID) == "" || strings.TrimSpace(item.Title) == "" {
			return nil, fmt.Errorf("%w: target item %d missing eid/title", ErrIncompleteSource, index)
		}
		if item.Podcast == nil || strings.TrimSpace(item.Podcast.Title) == "" {
			return nil, fmt.Errorf("%w: target item %d missing podcast title", ErrIncompleteSource, index)
		}
		if _, exists := seen[item.EID]; exists {
			return nil, fmt.Errorf("%w: duplicate eid %q", ErrDuplicateItems, item.EID)
		}
		seen[item.EID] = struct{}{}

		draft.Items = append(draft.Items, ItemDraft{
			ExternalEpisodeID: item.EID,
			ExternalPodcastID: firstNonEmpty(item.PID, item.Podcast.PID),
			PodcastTitle:      strings.TrimSpace(item.Podcast.Title),
			PodcastAuthor:     strings.TrimSpace(item.Podcast.Author),
			PodcastCoverURL:   optionalString(item.Podcast.Image),
			EpisodeTitle:      strings.TrimSpace(item.Title),
			// 推荐语缺失时保留空值，不生成替代文案。
			Recommendation:      strings.TrimSpace(item.Recommendation),
			Shownotes:           item.Shownotes,
			Duration:            normalizedDuration(item.Duration),
			PublishedAt:         parseOptionalTime(item.PubDate),
			ImageURL:            optionalString(item.Image),
			EpisodeURL:          EpisodeURLForEID(item.EID),
			PayType:             strings.TrimSpace(item.PayType),
			IsPrivateMedia:      item.IsPrivateMedia,
			AudioURL:            audioURL(item),
			AudioMimeType:       audioMIME(item),
			AudioSize:           audioSize(item),
			PodcastEpisodeCount: podcastEpisodeCount(item.Podcast),
		})
	}
	if len(draft.Items) == 0 {
		return draft, ErrEmptyCollection
	}
	return draft, nil
}

// EpisodeURLForEID 构造源平台单集链接；清单保存的条目链接由 ID 确定性生成。
func EpisodeURLForEID(eid string) string {
	return "https://www.xiaoyuzhoufm.com/episode/" + eid
}

func optionalString(image *imagePayload) string {
	if image == nil {
		return ""
	}
	return strings.TrimSpace(image.PicURL)
}

func podcastEpisodeCount(podcast *podcastRefPayload) int {
	if podcast == nil || podcast.EpisodeCount == "" {
		return 0
	}
	parsed, err := podcast.EpisodeCount.Int64()
	if err != nil || parsed < 0 || parsed > int64(maxInt) {
		return 0
	}
	return int(parsed)
}

const maxInt = int(^uint(0) >> 1)

// audioURL 仅在公开免费单集上保留音频地址快照；私密或付费留空。
func audioURL(item itemPayload) string {
	if item.IsPrivateMedia || (strings.TrimSpace(item.PayType) != "" && strings.TrimSpace(item.PayType) != PayTypeFree) {
		return ""
	}
	if item.Enclosure == nil {
		return ""
	}
	return strings.TrimSpace(item.Enclosure.URL)
}

func audioMIME(item itemPayload) string {
	if item.Enclosure != nil && strings.TrimSpace(item.Enclosure.Type) != "" {
		return strings.TrimSpace(item.Enclosure.Type)
	}
	if item.Media != nil {
		return strings.TrimSpace(item.Media.MIMEType)
	}
	return ""
}

func audioSize(item itemPayload) int64 {
	if item.Media == nil || item.Media.Size == "" {
		return 0
	}
	parsed, err := item.Media.Size.Int64()
	if err != nil || parsed < 0 {
		return 0
	}
	return parsed
}

func normalizedDuration(duration *int) int {
	if duration == nil || *duration < 0 {
		return 0
	}
	return *duration
}

func parseOptionalTime(raw string) *time.Time {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	parsed, err := time.Parse(time.RFC3339, trimmed)
	if err != nil {
		// 时间只是展示元数据；解析失败保留空值，不编造日期。
		return nil
	}
	return &parsed
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
