package collection

import (
	"encoding/json"
	"fmt"
	"strings"
)

// 小宇宙专题（collection.xiaoyuzhoufm.com/{slug}）页面本身是客户端渲染壳，
// 单集数据来自其公开的 campaign 接口；本解析器只读取已核实的响应结构。
// 专题中的未发布条目没有 episode 对象，属于正常预告位，跳过而不是报错。

type campaignEnvelope struct {
	Data *campaignPayload `json:"data"`
}

type campaignPayload struct {
	Slug       string                     `json:"slug"`
	Config     campaignConfigPayload      `json:"config"`
	Components []campaignComponentPayload `json:"components"`
}

type campaignConfigPayload struct {
	Share *campaignSharePayload `json:"share"`
}

type campaignSharePayload struct {
	Title string `json:"title"`
	Desc  string `json:"desc"`
}

type campaignComponentPayload struct {
	Kind  string            `json:"kind"`
	Items []json.RawMessage `json:"items"`
}

type campaignItemPayload struct {
	Kind    string       `json:"kind"`
	Episode *itemPayload `json:"episode"`
	Quote   string       `json:"quote"`
}

// ParseCampaignJSON 从专题接口响应中确定性解析清单元数据与已发布条目。
// expectedSlug 是从用户提交 URL 中识别出的专题 slug，与响应不一致时视为
// 结构不可信并拒绝导入。
func ParseCampaignJSON(body string, expectedSlug string) (*Draft, error) {
	var envelope campaignEnvelope
	if err := json.Unmarshal([]byte(body), &envelope); err != nil {
		return nil, fmt.Errorf("%w: decode campaign response: %v", ErrIncompleteSource, err)
	}
	payload := envelope.Data
	if payload == nil {
		return nil, fmt.Errorf("%w: campaign data missing", ErrIncompleteSource)
	}
	if strings.TrimSpace(payload.Slug) != expectedSlug {
		return nil, fmt.Errorf("%w: campaign slug %q does not match requested %q",
			ErrIncompleteSource, payload.Slug, expectedSlug)
	}

	title := ""
	description := ""
	if payload.Config.Share != nil {
		title = strings.TrimSpace(payload.Config.Share.Title)
		description = strings.TrimSpace(payload.Config.Share.Desc)
	}
	if title == "" {
		return nil, fmt.Errorf("%w: campaign title missing", ErrIncompleteSource)
	}

	draft := &Draft{
		Platform:    PlatformXiaoyuzhoufm,
		ExternalID:  payload.Slug,
		Title:       title,
		Description: description,
		TotalKnown:  false,
		Items:       make([]ItemDraft, 0),
	}
	seen := make(map[string]struct{})
	hasEpisodeList := false
	for componentIndex, component := range payload.Components {
		if component.Kind != campaignComponentKind {
			continue
		}
		hasEpisodeList = true
		for index, raw := range component.Items {
			var item campaignItemPayload
			if err := json.Unmarshal(raw, &item); err != nil {
				return nil, fmt.Errorf("%w: decode campaign item %d/%d: %v",
					ErrIncompleteSource, componentIndex, index, err)
			}
			// 未发布条目没有 episode 对象或 eid，是专题页的预告位；跳过。
			if item.Episode == nil || strings.TrimSpace(item.Episode.EID) == "" {
				continue
			}
			if strings.TrimSpace(item.Episode.Title) == "" {
				return nil, fmt.Errorf("%w: campaign item %d/%d missing title",
					ErrIncompleteSource, componentIndex, index)
			}
			if item.Episode.Podcast == nil || strings.TrimSpace(item.Episode.Podcast.Title) == "" {
				return nil, fmt.Errorf("%w: campaign item %d/%d missing podcast title",
					ErrIncompleteSource, componentIndex, index)
			}
			if _, exists := seen[item.Episode.EID]; exists {
				return nil, fmt.Errorf("%w: duplicate eid %q", ErrDuplicateItems, item.Episode.EID)
			}
			seen[item.Episode.EID] = struct{}{}

			entry := itemDraftFromPayload(*item.Episode)
			// 专题条目的推荐语来自编辑写在条目上的 quote；缺失时保留空值。
			entry.Recommendation = strings.TrimSpace(item.Quote)
			draft.Items = append(draft.Items, entry)
		}
	}
	if !hasEpisodeList {
		return nil, fmt.Errorf("%w: campaign has no episode-list component", ErrUnsupportedTargetType)
	}
	if len(draft.Items) == 0 {
		return draft, ErrEmptyCollection
	}
	return draft, nil
}
