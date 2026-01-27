package llm

import (
	"fmt"
	"time"
)

// EpisodeDetail 单集详情（用于LLM摘要）
type EpisodeDetail struct {
	Title         string
	ShowNotes     string
	PublishedDate time.Time
	UpdatedDate   *time.Time
	EpisodeNo     string
	Link          string
	XYZID         string
	QRCode        string
	QRCodeError   bool
}

// EpisodeReportData 报告数据（用于LLM摘要）
type EpisodeReportData struct {
	PodcastID      uint
	PodcastTitle   string
	PodcastFeedURL string
	Episodes       []EpisodeDetail
}

// Summarizer 摘要生成器
type Summarizer struct {
	client     *Client
	tplManager *PromptManager
}

// NewSummarizer 创建摘要生成器
func NewSummarizer(client *Client, tplManager *PromptManager) *Summarizer {
	return &Summarizer{
		client:     client,
		tplManager: tplManager,
	}
}

// GenerateForReport 为工作流报告生成LLM摘要
func (s *Summarizer) GenerateForReport(data []EpisodeReportData, workflowName string, options SummaryOptions) (*SummaryResult, error) {
	// 根据数据量选择策略
	totalEpisodes := 0
	for _, podcast := range data {
		totalEpisodes += len(podcast.Episodes)
	}

	// 如果单集数过多，采样
	maxEpisodes := options.MaxEpisodes
	if maxEpisodes == 0 {
		maxEpisodes = 20 // 默认值
	}

	if totalEpisodes > maxEpisodes {
		data = s.sampleEpisodes(data, maxEpisodes)
		totalEpisodes = 0
		for _, podcast := range data {
			totalEpisodes += len(podcast.Episodes)
		}
	}

	// 准备模板数据
	templateData := struct {
		WorkflowName  string
		TotalEpisodes int
		NumPodcasts   int
		Podcasts      []EpisodeReportData
	}{
		WorkflowName:  workflowName,
		TotalEpisodes: totalEpisodes,
		NumPodcasts:   len(data),
		Podcasts:      data,
	}

	// 渲染模板
	prompt, err := s.tplManager.RenderTemplate("default_summary", templateData)
	if err != nil {
		return nil, fmt.Errorf("渲染prompt模板失败: %w", err)
	}

	// 调用LLM
	result, err := s.client.GenerateSummary(nil, prompt, options)
	if err != nil {
		return nil, fmt.Errorf("LLM摘要生成失败: %w", err)
	}

	return result, nil
}

// sampleEpisodes 采样单集（保留重要内容）
func (s *Summarizer) sampleEpisodes(data []EpisodeReportData, maxEpisodes int) []EpisodeReportData {
	// 简单策略：每个播客最多取N个最新单集
	maxPerPodcast := maxEpisodes / len(data)
	if maxPerPodcast < 1 {
		maxPerPodcast = 1
	}

	var sampled []EpisodeReportData
	for _, podcast := range data {
		sampledEpisodes := podcast.Episodes
		if len(sampledEpisodes) > maxPerPodcast {
			sampledEpisodes = sampledEpisodes[:maxPerPodcast]
		}

		sampled = append(sampled, EpisodeReportData{
			PodcastID:      podcast.PodcastID,
			PodcastTitle:   podcast.PodcastTitle,
			PodcastFeedURL: podcast.PodcastFeedURL,
			Episodes:       sampledEpisodes,
		})
	}

	return sampled
}
