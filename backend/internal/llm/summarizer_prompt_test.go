package llm

import (
	"strings"
	"testing"
)

func TestBuildSummaryTemplateData(t *testing.T) {
	data := []EpisodeReportData{
		{
			PodcastID:    1,
			PodcastTitle: "播客 A",
			Episodes: []EpisodeDetail{
				{Title: "A1"},
				{Title: "A2"},
			},
		},
		{
			PodcastID:    2,
			PodcastTitle: "播客 B",
			Episodes: []EpisodeDetail{
				{Title: "B1"},
			},
		},
	}

	templateData := buildSummaryTemplateData("工作流", countEpisodes(data), data)

	if templateData.WorkflowName != "工作流" {
		t.Fatalf("expected workflow name to be preserved")
	}
	if templateData.TotalEpisodes != 3 {
		t.Fatalf("expected 3 episodes, got %d", templateData.TotalEpisodes)
	}
	if templateData.NumPodcasts != 2 {
		t.Fatalf("expected 2 podcasts, got %d", templateData.NumPodcasts)
	}
	if len(templateData.Podcasts) != len(data) {
		t.Fatalf("expected podcasts to be preserved")
	}
}

func TestRenderReportUserPromptWithCustomTemplate(t *testing.T) {
	summarizer := &Summarizer{}
	templateData := summaryTemplateData{
		WorkflowName:  "每日报告",
		TotalEpisodes: 3,
		NumPodcasts:   2,
		Podcasts: []EpisodeReportData{
			{PodcastTitle: "播客 A"},
		},
	}

	rendered, err := summarizer.renderReportUserPrompt(
		"{{.WorkflowName}}: {{.TotalEpisodes}} episodes from {{.NumPodcasts}} podcasts; first={{(index .Podcasts 0).PodcastTitle}}",
		templateData,
	)

	if err != nil {
		t.Fatalf("expected custom prompt to render, got %v", err)
	}
	if rendered != "每日报告: 3 episodes from 2 podcasts; first=播客 A" {
		t.Fatalf("unexpected rendered prompt: %s", rendered)
	}
}

func TestRenderReportUserPromptReturnsTemplateParseError(t *testing.T) {
	summarizer := &Summarizer{}

	_, err := summarizer.renderReportUserPrompt("{{", summaryTemplateData{})

	if err == nil || !strings.Contains(err.Error(), "解析用户自定义模板失败") {
		t.Fatalf("expected parse error, got %v", err)
	}
}
