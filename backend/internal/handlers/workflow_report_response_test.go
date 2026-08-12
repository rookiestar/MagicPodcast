package handlers

import (
	"net/http"
	"reflect"
	"testing"
	"time"

	"magicpodcast/internal/models"
)

func TestJobExecutionResponseIncludesFeedAccessSummary(t *testing.T) {
	status := http.StatusForbidden
	retrievedAt := time.Date(2026, 7, 19, 1, 2, 3, 0, time.UTC)
	exec := &models.JobExecution{
		BaseModel:                models.BaseModel{ID: 1, CreatedAt: time.Date(2026, 7, 19, 1, 2, 3, 0, time.UTC)},
		JobID:                    2,
		PodcastTitle:             "Observed Podcast",
		PodcastFeedURL:           "https://user:password@feed.example.test/feed.xml?access_token=super-secret&format=rss",
		Status:                   models.ExecutionStatusFailed,
		FeedHTTPStatus:           &status,
		FeedErrorCategory:        "access_denied",
		FeedTargetDomain:         "feed.example.test",
		FeedResponseTimeMs:       115,
		FeedRetryAfter:           "120",
		FeedETag:                 `"feed-v1"`,
		FeedCacheStatus:          "not_used",
		FeedSourceType:           "primary",
		FeedSourceURL:            "https://feed.example.test/feed.xml",
		FeedIdentityVerification: "not_checked",
		FeedFreshness:            "unknown",
		FeedEgressID:             "direct",
		FeedResponseBytes:        7,
		FeedSnapshotRetrievedAt:  &retrievedAt,
		FeedCircuitState:         "probe",
	}

	response := (&WorkflowHandler{}).toJobExecutionResponse(exec)
	if response.FeedHTTPStatus == nil || *response.FeedHTTPStatus != status {
		t.Fatalf("expected HTTP status %d, got %#v", status, response.FeedHTTPStatus)
	}
	if response.FeedErrorCategory != "access_denied" || response.FeedTargetDomain != "feed.example.test" {
		t.Fatalf("unexpected feed error summary: %#v", response)
	}
	if response.PodcastFeedURL != "https://feed.example.test/feed.xml?access_token=%5BREDACTED%5D&format=rss" {
		t.Fatalf("feed URL credentials were not redacted: %q", response.PodcastFeedURL)
	}
	if response.FeedRetryAfter != "120" || response.FeedETag != `"feed-v1"` || response.FeedResponseBytes != 7 {
		t.Fatalf("whitelisted response metadata missing: %#v", response)
	}
	if response.FeedSnapshotRetrievedAt == nil || !response.FeedSnapshotRetrievedAt.Equal(retrievedAt) {
		t.Fatalf("snapshot retrieval time missing: %#v", response.FeedSnapshotRetrievedAt)
	}
	if response.FeedCircuitState != "probe" {
		t.Fatalf("circuit state missing: %#v", response.FeedCircuitState)
	}
	if response.FeedSourceURL != "https://feed.example.test/feed.xml" || response.FeedIdentityVerification != "not_checked" {
		t.Fatalf("source verification summary missing: %#v", response)
	}
}

func TestWorkflowReportResponseIncludesPublicReportFields(t *testing.T) {
	generatedAt := time.Date(2026, 5, 31, 10, 30, 0, 0, time.UTC)
	rangeStart := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	rangeEnd := time.Date(2026, 5, 31, 23, 59, 59, 0, time.UTC)
	structured := models.ReportEpisodeList{
		{EpisodeID: 7, Order: 1, EpisodeTitle: "单集", PodcastTitle: "节目"},
	}
	report := &models.Report{
		BaseModel:          models.BaseModel{ID: 11},
		JobID:              22,
		Title:              "报告",
		Content:            "# 内容",
		Summary:            "摘要",
		EpisodesCount:      3,
		PodcastsCount:      2,
		MatchedCount:       4,
		TimeRangeStart:     rangeStart,
		TimeRangeEnd:       rangeEnd,
		TimeRangeMode:      "manual",
		GeneratedAt:        generatedAt,
		Format:             "markdown",
		FileSize:           1234,
		PublishToHomepage:  true,
		ReportType:         "daily",
		WorkflowName:       "日报工作流",
		StructuredEpisodes: structured,
		LLMSummary:         "AI 摘要",
		LLMModelUsed:       "test-model",
		LLMTokensUsed:      99,
		LLMError:           "test error",
	}

	response := workflowReportResponse(report)

	expected := map[string]interface{}{
		"id":                  report.ID,
		"job_id":              report.JobID,
		"title":               report.Title,
		"content":             report.Content,
		"summary":             report.Summary,
		"episodes_count":      report.EpisodesCount,
		"podcasts_count":      report.PodcastsCount,
		"matched_count":       report.MatchedCount,
		"time_range_start":    report.TimeRangeStart,
		"time_range_end":      report.TimeRangeEnd,
		"time_range_mode":     report.TimeRangeMode,
		"generated_at":        report.GeneratedAt,
		"format":              report.Format,
		"file_size":           report.FileSize,
		"publish_to_homepage": report.PublishToHomepage,
		"report_type":         report.ReportType,
		"workflow_name":       report.WorkflowName,
		"structured_episodes": report.StructuredEpisodes,
		"llm_summary":         report.LLMSummary,
		"llm_model_used":      report.LLMModelUsed,
		"llm_tokens_used":     report.LLMTokensUsed,
		"llm_error":           report.LLMError,
	}

	if len(response) != len(expected) {
		t.Fatalf("expected %d response fields, got %d", len(expected), len(response))
	}

	for key, want := range expected {
		got := response[key]
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("field %s: expected %#v, got %#v", key, want, got)
		}
	}
}
