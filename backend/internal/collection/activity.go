package collection

import (
	"encoding/json"
	"fmt"
	"strings"
)

// 小宇宙活动页（h5.xiaoyuzhoufm.com/xyz-activity/{code}）页面本身是客户端
// 渲染壳，单集数据来自其公开的活动接口（POST get-by-code）。页面由图片模块
// 与若干单集列表模块组成；导入时按页面顺序展平为一份有序清单，只读取
// 已核实的响应结构。单集载荷比清单/专题更薄：节目只有名称，无 Show Notes
// 与发布时间，对应字段保留空值，不编造。

type activityEnvelope struct {
	Data *activityPayload `json:"data"`
}

type activityPayload struct {
	Code    string                  `json:"code"`
	Title   string                  `json:"title"`
	Modules []activityModulePayload `json:"modules"`
}

type activityModulePayload struct {
	Type     string                   `json:"type"`
	Episodes []activityEpisodePayload `json:"episodes"`
}

type activityMediaSourcePayload struct {
	Mode string `json:"mode"`
	URL  string `json:"url"`
}

type activityMediaPayload struct {
	Size     json.Number                 `json:"size"`
	MIMEType string                      `json:"mimeType"`
	Source   *activityMediaSourcePayload `json:"source"`
}

type activityEpisodePayload struct {
	ID             string                `json:"id"`
	PID            string                `json:"pid"`
	Title          string                `json:"title"`
	PodcastTitle   string                `json:"podcastTitle"`
	Image          *imagePayload         `json:"image"`
	Duration       *int                  `json:"duration"`
	PayType        string                `json:"payType"`
	Media          *activityMediaPayload `json:"media"`
	Recommendation string                `json:"recommendation"`
}

// activityMediaPublic 公开可直连的音频来源标记；其他取值代表受限访问。
const activityMediaPublic = "PUBLIC"

// ParseActivityJSON 从活动接口响应中确定性解析清单元数据与单集条目。
// expectedCode 是从用户提交 URL 中识别出的活动 code，与响应不一致时视为
// 结构不可信并拒绝导入；未知 code 的响应 data 为 null，同样拒绝。
func ParseActivityJSON(body string, expectedCode string) (*Draft, error) {
	var envelope activityEnvelope
	if err := json.Unmarshal([]byte(body), &envelope); err != nil {
		return nil, fmt.Errorf("%w: decode activity response: %v", ErrIncompleteSource, err)
	}
	payload := envelope.Data
	if payload == nil {
		return nil, fmt.Errorf("%w: activity data missing", ErrIncompleteSource)
	}
	if strings.TrimSpace(payload.Code) != expectedCode {
		return nil, fmt.Errorf("%w: activity code %q does not match requested %q",
			ErrIncompleteSource, payload.Code, expectedCode)
	}
	title := strings.TrimSpace(payload.Title)
	if title == "" {
		return nil, fmt.Errorf("%w: activity title missing", ErrIncompleteSource)
	}

	draft := &Draft{
		Platform:   PlatformXiaoyuzhoufm,
		ExternalID: expectedCode,
		Title:      title,
		TotalKnown: false,
		Items:      make([]ItemDraft, 0),
	}
	seen := make(map[string]struct{})
	hasEpisodeList := false
	for moduleIndex, module := range payload.Modules {
		if module.Type != activityListKind {
			continue
		}
		hasEpisodeList = true
		for index, episode := range module.Episodes {
			if strings.TrimSpace(episode.ID) == "" || strings.TrimSpace(episode.Title) == "" {
				return nil, fmt.Errorf("%w: activity item %d/%d missing id/title",
					ErrIncompleteSource, moduleIndex, index)
			}
			if strings.TrimSpace(episode.PodcastTitle) == "" {
				return nil, fmt.Errorf("%w: activity item %d/%d missing podcast title",
					ErrIncompleteSource, moduleIndex, index)
			}
			if _, exists := seen[episode.ID]; exists {
				return nil, fmt.Errorf("%w: duplicate eid %q", ErrDuplicateItems, episode.ID)
			}
			seen[episode.ID] = struct{}{}

			draft.Items = append(draft.Items, ItemDraft{
				ExternalEpisodeID: episode.ID,
				ExternalPodcastID: strings.TrimSpace(episode.PID),
				PodcastTitle:      strings.TrimSpace(episode.PodcastTitle),
				// 活动载荷没有节目封面；条目图片是单集封面，不得冒充节目封面。
				PodcastCoverURL: "",
				EpisodeTitle:    strings.TrimSpace(episode.Title),
				// 推荐语缺失时保留空值，不生成替代文案。
				Recommendation: strings.TrimSpace(episode.Recommendation),
				Duration:       normalizedDuration(episode.Duration),
				ImageURL:       optionalString(episode.Image),
				EpisodeURL:     EpisodeURLForEID(episode.ID),
				PayType:        strings.TrimSpace(episode.PayType),
				AudioURL:       activityAudioURL(episode),
				AudioMimeType:  activityAudioMIME(episode),
				AudioSize:      activityAudioSize(episode),
			})
		}
	}
	if !hasEpisodeList {
		return nil, fmt.Errorf("%w: activity has no episode-list module", ErrUnsupportedTargetType)
	}
	if len(draft.Items) == 0 {
		return draft, ErrEmptyCollection
	}
	return draft, nil
}

// activityAudioURL 仅在显式 FREE 且公开来源的单集上保留音频地址快照；
// 付费、私密与付费状态未知（缺失或空白）一律留空，fail closed。
func activityAudioURL(episode activityEpisodePayload) string {
	if strings.TrimSpace(episode.PayType) != PayTypeFree {
		return ""
	}
	if episode.Media == nil || episode.Media.Source == nil ||
		strings.TrimSpace(episode.Media.Source.Mode) != activityMediaPublic {
		return ""
	}
	return strings.TrimSpace(episode.Media.Source.URL)
}

func activityAudioMIME(episode activityEpisodePayload) string {
	if episode.Media == nil {
		return ""
	}
	return strings.TrimSpace(episode.Media.MIMEType)
}

func activityAudioSize(episode activityEpisodePayload) int64 {
	if episode.Media == nil || episode.Media.Size == "" {
		return 0
	}
	parsed, err := episode.Media.Size.Int64()
	if err != nil || parsed < 0 {
		return 0
	}
	return parsed
}
