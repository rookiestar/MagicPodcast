package handlers

import (
	"testing"
	"time"

	"magicpodcast/internal/models"
)

func TestWorkflowReportResponseIncludesPublicReportFields(t *testing.T) {
	generatedAt := time.Date(2026, 5, 31, 10, 30, 0, 0, time.UTC)
	rangeStart := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	rangeEnd := time.Date(2026, 5, 31, 23, 59, 59, 0, time.UTC)
	report := &models.Report{
		BaseModel:      models.BaseModel{ID: 11},
		JobID:          22,
		Title:          "报告",
		Content:        "# 内容",
		Summary:        "摘要",
		EpisodesCount:  3,
		PodcastsCount:  2,
		MatchedCount:   4,
		TimeRangeStart: rangeStart,
		TimeRangeEnd:   rangeEnd,
		TimeRangeMode:  "manual",
		GeneratedAt:    generatedAt,
		Format:         "markdown",
		FileSize:       1234,
		LLMSummary:     "AI 摘要",
		LLMModelUsed:   "test-model",
		LLMTokensUsed:  99,
		LLMError:       "test error",
	}

	response := workflowReportResponse(report)

	expected := map[string]interface{}{
		"id":               report.ID,
		"job_id":           report.JobID,
		"title":            report.Title,
		"content":          report.Content,
		"summary":          report.Summary,
		"episodes_count":   report.EpisodesCount,
		"podcasts_count":   report.PodcastsCount,
		"matched_count":    report.MatchedCount,
		"time_range_start": report.TimeRangeStart,
		"time_range_end":   report.TimeRangeEnd,
		"time_range_mode":  report.TimeRangeMode,
		"generated_at":     report.GeneratedAt,
		"format":           report.Format,
		"file_size":        report.FileSize,
		"llm_summary":      report.LLMSummary,
		"llm_model_used":   report.LLMModelUsed,
		"llm_tokens_used":  report.LLMTokensUsed,
		"llm_error":        report.LLMError,
	}

	if len(response) != len(expected) {
		t.Fatalf("expected %d response fields, got %d", len(expected), len(response))
	}

	for key, want := range expected {
		if got := response[key]; got != want {
			t.Fatalf("field %s: expected %#v, got %#v", key, want, got)
		}
	}
}
