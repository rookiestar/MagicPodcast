package llm

import (
	"bytes"
	"context"
	"fmt"
	"text/template"
	"time"
)

const (
	defaultMaxSummaryEpisodes = 20
	fallbackSystemPrompt      = "你是播客内容分析专家。请基于提供的数据进行分析，不编造信息，保持客观中立。"
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

type summaryTemplateData struct {
	WorkflowName  string
	TotalEpisodes int
	NumPodcasts   int
	Podcasts      []EpisodeReportData
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
func (s *Summarizer) GenerateForReport(ctx context.Context, data []EpisodeReportData, workflowName string, userPrompt string, options SummaryOptions) (*SummaryResult, error) {
	// 数据量采样策略
	totalEpisodes := countEpisodes(data)

	// 如果单集数过多，采样
	maxEpisodes := options.MaxEpisodes
	if maxEpisodes == 0 {
		maxEpisodes = defaultMaxSummaryEpisodes
	}

	if totalEpisodes > maxEpisodes {
		data = s.sampleEpisodes(data, maxEpisodes)
		totalEpisodes = countEpisodes(data)
	}

	// System Prompt：从全局config获取（不在workflow中存储）
	systemPrompt := s.client.GetSystemPrompt()
	if systemPrompt == "" {
		systemPrompt = fallbackSystemPrompt
	}

	templateData := buildSummaryTemplateData(workflowName, totalEpisodes, data)
	userPrompt, err := s.renderReportUserPrompt(userPrompt, templateData)
	if err != nil {
		return nil, err
	}

	// 调用LLM（传入system和user prompt）
	if ctx == nil {
		ctx = context.Background()
	}
	result, err := s.client.GenerateSummary(ctx, systemPrompt, userPrompt, options)
	if err != nil {
		return nil, fmt.Errorf("LLM摘要生成失败: %w", err)
	}

	return result, nil
}

func countEpisodes(data []EpisodeReportData) int {
	totalEpisodes := 0
	for _, podcast := range data {
		totalEpisodes += len(podcast.Episodes)
	}
	return totalEpisodes
}

func buildSummaryTemplateData(workflowName string, totalEpisodes int, data []EpisodeReportData) summaryTemplateData {
	return summaryTemplateData{
		WorkflowName:  workflowName,
		TotalEpisodes: totalEpisodes,
		NumPodcasts:   len(data),
		Podcasts:      data,
	}
}

func (s *Summarizer) renderReportUserPrompt(userPrompt string, templateData summaryTemplateData) (string, error) {
	if userPrompt == "" {
		renderedPrompt, err := s.tplManager.RenderTemplate("default_summary", templateData)
		if err != nil {
			return "", fmt.Errorf("渲染prompt模板失败: %w", err)
		}
		return renderedPrompt, nil
	}

	tpl, err := template.New("user_custom").Parse(userPrompt)
	if err != nil {
		return "", fmt.Errorf("解析用户自定义模板失败: %w", err)
	}
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, templateData); err != nil {
		return "", fmt.Errorf("渲染用户自定义模板失败: %w", err)
	}
	return buf.String(), nil
}

// sampleEpisodes 采样单集（保留重要内容）
func (s *Summarizer) sampleEpisodes(data []EpisodeReportData, maxEpisodes int) []EpisodeReportData {
	// 采样策略：遍历所有播客，依次添加单集，直到达到maxEpisodes上限
	// 不按平均分配，而是优先处理前面的播客

	var sampled []EpisodeReportData
	totalCount := 0

	for _, podcast := range data {
		// 计算这个播客还能添加多少个
		remaining := maxEpisodes - totalCount

		// 如果没有剩余配额，跳过
		if remaining <= 0 {
			continue
		}

		// 决定取多少个（取剩余和该播客单集数的较小值）
		takeCount := remaining
		if len(podcast.Episodes) < takeCount {
			takeCount = len(podcast.Episodes)
		}

		sampledEpisodes := podcast.Episodes[:takeCount]
		totalCount += len(sampledEpisodes)

		sampled = append(sampled, EpisodeReportData{
			PodcastID:      podcast.PodcastID,
			PodcastTitle:   podcast.PodcastTitle,
			PodcastFeedURL: podcast.PodcastFeedURL,
			Episodes:       sampledEpisodes,
		})
	}

	return sampled
}
