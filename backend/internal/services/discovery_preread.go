package services

import (
	"html"
	"regexp"
	"strings"
	"time"

	"magicpodcast/internal/models"
)

const (
	PreReadKindSummary    = "summary"
	PreReadKindViewpoints = "viewpoints"
	PreReadKindRelevant   = "relevant"
	PreReadKindChallenge  = "challenge"

	PreReadStatusAvailable    = "available"
	PreReadStatusPending      = "pending"
	PreReadStatusInsufficient = "insufficient"
	PreReadStatusFailed       = "failed"
	PreReadStatusMissing      = "missing"

	discoveryPreReadVersion = "evidence-v1"
)

var prereadHTMLTagPattern = regexp.MustCompile(`<[^>]+>`)

type DiscoveryPreReadSource struct {
	Kind  string `json:"kind"`
	Label string `json:"label"`
	URL   string `json:"url,omitempty"`
}

type DiscoveryPreRead struct {
	Kind             string                   `json:"kind"`
	Label            string                   `json:"label"`
	Status           string                   `json:"status"`
	Content          string                   `json:"content"`
	RelationStrength string                   `json:"relation_strength,omitempty"`
	Sources          []DiscoveryPreReadSource `json:"sources"`
	GeneratedAt      time.Time                `json:"generated_at"`
	Version          string                   `json:"version"`
}

func buildDiscoveryPreReads(episode models.Episode, generatedAt time.Time) []DiscoveryPreRead {
	showNotes := compactPreReadText(episode.ShowNotes, 220)
	contentSources := make([]DiscoveryPreReadSource, 0, 2)
	if showNotes != "" {
		contentSources = append(contentSources, DiscoveryPreReadSource{
			Kind:  "show_notes",
			Label: "Show Notes",
		})
	}
	if episode.Link != "" {
		contentSources = append(contentSources, DiscoveryPreReadSource{
			Kind:  "original_link",
			Label: "原始链接",
			URL:   episode.Link,
		})
	}

	summary := newDiscoveryPreRead(
		PreReadKindSummary,
		"摘要",
		PreReadStatusAvailable,
		showNotes,
		contentSources,
		generatedAt,
	)
	viewpoints := newDiscoveryPreRead(
		PreReadKindViewpoints,
		"观点",
		PreReadStatusInsufficient,
		"当前 Show Notes 未给出可明确归因的观点，保留原始信息供核对。",
		contentSources,
		generatedAt,
	)
	challenge := newDiscoveryPreRead(
		PreReadKindChallenge,
		"值得质疑",
		PreReadStatusAvailable,
		"当前证据仅来自节目方 Show Notes；关键主张仍需回到原始链接或音频核对。",
		contentSources,
		generatedAt,
	)

	if showNotes == "" {
		summary.Status = PreReadStatusMissing
		summary.Content = "Show Notes 暂缺，无法生成摘要；候选身份、时间与原始信息仍可使用。"
		viewpoints.Status = PreReadStatusMissing
		viewpoints.Content = "Show Notes 暂缺，无法提取可归因观点。"
		challenge.Status = PreReadStatusMissing
		challenge.Content = "Show Notes 暂缺，无法基于原文列出质疑点。"
	} else if containsViewpointMarker(showNotes) {
		viewpoints.Status = PreReadStatusAvailable
		viewpoints.Content = "Show Notes 中可核对的表述：" + showNotes
	}

	relevant := buildRelevantPreRead(episode, generatedAt)
	return []DiscoveryPreRead{summary, viewpoints, relevant, challenge}
}

func newDiscoveryPreRead(
	kind string,
	label string,
	status string,
	content string,
	sources []DiscoveryPreReadSource,
	generatedAt time.Time,
) DiscoveryPreRead {
	return DiscoveryPreRead{
		Kind:        kind,
		Label:       label,
		Status:      status,
		Content:     content,
		Sources:     append([]DiscoveryPreReadSource(nil), sources...),
		GeneratedAt: generatedAt,
		Version:     discoveryPreReadVersion,
	}
}

func buildRelevantPreRead(episode models.Episode, generatedAt time.Time) DiscoveryPreRead {
	sources := make([]DiscoveryPreReadSource, 0, len(episode.Tags)+len(episode.Podcast.Tags)+2)
	tagNames := make([]string, 0, len(episode.Tags)+len(episode.Podcast.Tags))
	for _, tag := range episode.Tags {
		tagNames = append(tagNames, tag.Name)
		sources = append(sources, DiscoveryPreReadSource{Kind: "episode_tag", Label: "单集标签：" + tag.Name})
	}
	for _, tag := range episode.Podcast.Tags {
		tagNames = append(tagNames, tag.Name)
		sources = append(sources, DiscoveryPreReadSource{Kind: "podcast_tag", Label: "节目标签：" + tag.Name})
	}
	if strings.TrimSpace(episode.Notes) != "" {
		sources = append(sources, DiscoveryPreReadSource{Kind: "episode_notes", Label: "单集备注"})
	}
	if strings.TrimSpace(episode.Podcast.Notes) != "" {
		sources = append(sources, DiscoveryPreReadSource{Kind: "podcast_notes", Label: "节目备注"})
	}

	preRead := newDiscoveryPreRead(
		PreReadKindRelevant,
		"与我相关",
		PreReadStatusInsufficient,
		"未发现个人标签或备注证据，不生成个人关联。",
		sources,
		generatedAt,
	)
	if len(sources) == 0 {
		return preRead
	}

	searchable := strings.ToLower(episode.Title + " " + compactPreReadText(episode.ShowNotes, 1000))
	matchedTags := make([]string, 0, len(tagNames))
	for _, tagName := range tagNames {
		if normalized := strings.TrimSpace(strings.ToLower(tagName)); normalized != "" &&
			strings.Contains(searchable, normalized) {
			matchedTags = append(matchedTags, tagName)
		}
	}

	preRead.Status = PreReadStatusAvailable
	if len(matchedTags) > 0 {
		preRead.RelationStrength = "明确相关"
		preRead.Content = "个人标签「" + strings.Join(matchedTags, "、") + "」与标题或 Show Notes 直接重合。"
		return preRead
	}

	preRead.RelationStrength = "弱相关"
	preRead.Content = "弱相关：个人库已有标签或备注，但当前原始信息没有明确重合，需自行核对。"
	return preRead
}

func compactPreReadText(raw string, maxRunes int) string {
	text := html.UnescapeString(prereadHTMLTagPattern.ReplaceAllString(raw, " "))
	text = strings.Join(strings.Fields(text), " ")
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	return string(runes[:maxRunes]) + "…"
}

func containsViewpointMarker(value string) bool {
	for _, marker := range []string{"主张", "认为", "建议", "观点", "提出", "强调"} {
		if strings.Contains(value, marker) {
			return true
		}
	}
	return false
}
