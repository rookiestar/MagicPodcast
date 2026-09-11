package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"magicpodcast/internal/config"
	"magicpodcast/internal/llm"
	"magicpodcast/internal/models"
	"magicpodcast/internal/workflow"
)

func TestGenerateThenReadReportRecoversEmptyTruncatedSummary(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if n == 1 {
			_, _ = w.Write(handlerChatCompletionBody("deepseek-flash", "", "length", 1000))
			return
		}
		_, _ = w.Write(handlerChatCompletionBody("deepseek-flash", "恢复后的摘要", "stop", 300))
	}))
	defer server.Close()

	client := llm.NewClient(&config.LLMConfig{
		Enabled:            true,
		Provider:           config.LLMProviderDeepSeek,
		APIKey:             "test-key",
		BaseURL:            server.URL,
		DefaultModel:       "deepseek-flash",
		Timeout:            5,
		MaxRetries:         0,
		RateLimitPerMinute: 60,
	})
	promptManager := llm.NewPromptManager(t.TempDir())
	require.NoError(t, promptManager.ResetToDefault("default_summary"))
	summarizer := llm.NewSummarizer(client, promptManager)
	db, router := setupReportAPI(t, summarizer)
	job := seedReportJob(t, db, true)

	report, err := workflow.NewReportGenerator(db, summarizer).GenerateForJob(context.Background(), &job)
	require.NoError(t, err)
	require.Equal(t, "恢复后的摘要", report.LLMSummary)
	require.Equal(t, 1300, report.LLMTokensUsed)
	require.Equal(t, models.AIStatusGenerated, report.DeriveAIStatus())

	body := getReportJSON(t, router, job.ID)
	require.Equal(t, "恢复后的摘要", body["llm_summary"])
	stats, ok := body["report_stats"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, models.AIStatusGenerated, stats["ai_status"])
	require.Contains(t, stats["line"], "AI 已生成 · deepseek-flash · 1.3K Token")
	require.Equal(t, int32(2), attempts.Load())
}

func handlerChatCompletionBody(model, content, finishReason string, totalTokens int) []byte {
	payload, _ := json.Marshal(llm.ChatCompletionResponse{
		ID:      "test",
		Object:  "chat.completion",
		Created: 1,
		Model:   model,
		Choices: []llm.Choice{{
			Index:        0,
			Message:      llm.Message{Role: "assistant", Content: content},
			FinishReason: finishReason,
		}},
		Usage: llm.Usage{PromptTokens: 1, CompletionTokens: 1, TotalTokens: totalTokens},
	})
	return payload
}
