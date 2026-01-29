package llm

import (
	"bytes"
	"context"
	"fmt"
	"magicpodcast/internal/logger"
	"text/template"
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
func (s *Summarizer) GenerateForReport(data []EpisodeReportData, workflowName string, userPrompt string, options SummaryOptions) (*SummaryResult, error) {
	// 添加调试日志
	logger.Infof("[Summarizer.GenerateForReport] Called with workflow=%s, num_podcasts=%d", workflowName, len(data))
	logger.Infof("  - Client is nil: %v", s.client == nil)
	if s.client != nil {
		logger.Infof("  - Client config enabled: %v", s.client.config.Enabled)
	}

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

	// System Prompt：从全局config获取（不在workflow中存储）
	systemPrompt := s.client.GetSystemPrompt()
	if systemPrompt == "" {
		// Fallback默认值（如果config未配置）
		systemPrompt = "你是播客内容分析专家。请基于提供的数据进行分析，不编造信息，保持客观中立。"
	}

	// User Prompt：使用workflow自定义或默认模板
	if userPrompt == "" {
		// 准备模板数据（使用默认模板）
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

		// 渲染默认模板
		renderedPrompt, err := s.tplManager.RenderTemplate("default_summary", templateData)
		if err != nil {
			return nil, fmt.Errorf("渲染prompt模板失败: %w", err)
		}
		userPrompt = renderedPrompt
	} else {
		// 用户自定义模板，需要渲染
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

		// 解析并渲染用户自定义模板
		tpl, err := template.New("user_custom").Parse(userPrompt)
		if err != nil {
			return nil, fmt.Errorf("解析用户自定义模板失败: %w", err)
		}

		var buf bytes.Buffer
		if err := tpl.Execute(&buf, templateData); err != nil {
			return nil, fmt.Errorf("渲染用户自定义模板失败: %w", err)
		}
		userPrompt = buf.String()
	}

	// 调用LLM（传入system和user prompt）
	result, err := s.client.GenerateSummary(context.Background(), systemPrompt, userPrompt, options)
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
