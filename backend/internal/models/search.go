package models

import "time"

// SearchResultType 搜索结果类型
type SearchResultType string

const (
	SearchResultTypePodcast SearchResultType = "podcast"
	SearchResultTypeEpisode SearchResultType = "episode"
)

// MatchedField 匹配字段信息
type MatchedField struct {
	Field   string  `json:"field"`   // 字段名：title, author, description, show_notes
	Score   float64 `json:"score"`   // 该字段的得分
	Snippet string  `json:"snippet"` // 匹配内容的片段（用于高亮显示）
}

// PodcastSearchResult 播客搜索结果
type PodcastSearchResult struct {
	ID                uint           `json:"id"`
	Title             string         `json:"title"`
	Author            string         `json:"author"`
	Description       string         `json:"description"`
	CoverURL          string         `json:"cover_url"`
	EpisodeCount      int            `json:"episode_count"`
	NewestEpisodeDate time.Time      `json:"newest_episode_date"`
	RelevanceScore    float64        `json:"relevance_score"`
	MatchedFields     []MatchedField `json:"matched_fields"`
	Tags              []Tag          `json:"tags,omitempty"`
}

// EpisodeSearchResult 单集搜索结果
type EpisodeSearchResult struct {
	ID              uint        `json:"id"`
	PodcastID       uint        `json:"podcast_id"`
	PodcastTitle    string      `json:"podcast_title"`
	PodcastCoverURL string      `json:"podcast_cover_url"`
	Title           string      `json:"title"`
	ShowNotes       string      `json:"show_notes"`
	PublishedDate   *time.Time  `json:"published_date"`
	Duration        int         `json:"duration"`
	RelevanceScore  float64     `json:"relevance_score"`
	MatchedFields   []MatchedField `json:"matched_fields"`
}

// PaginationInfo 分页信息
type PaginationInfo struct {
	Page       int `json:"page"`
	PageSize   int `json:"page_size"`
	Total      int `json:"total"`
	TotalPages int `json:"total_pages"`
}

// SearchResponse 搜索响应
type SearchResponse struct {
	Podcasts  []PodcastSearchResult `json:"podcasts"`
	Episodes  []EpisodeSearchResult `json:"episodes"`
}

// SearchPagination 搜索分页信息
type SearchPagination struct {
	Podcasts PaginationInfo `json:"podcasts"`
	Episodes PaginationInfo `json:"episodes"`
}
