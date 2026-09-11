package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"magicpodcast/internal/config"
)

func newSummarizerHTTPTestClient(t *testing.T, serverURL string) *Client {
	t.Helper()
	return NewClient(&config.LLMConfig{
		Enabled:            true,
		Provider:           config.LLMProviderDeepSeek,
		APIKey:             "test-key",
		BaseURL:            serverURL,
		DefaultModel:       "deepseek-v4-flash",
		Timeout:            5,
		MaxRetries:         0,
		RateLimitPerMinute: 60,
	})
}

func newSummarizerPromptManager(t *testing.T) *PromptManager {
	t.Helper()
	return NewPromptManager(t.TempDir())
}

func summarizerRetryData() []EpisodeReportData {
	return []EpisodeReportData{{
		PodcastID:      1,
		PodcastTitle:   "测试节目",
		PodcastFeedURL: "https://example.com/feed.xml",
		Episodes: []EpisodeDetail{{
			Title:     "测试单集",
			ShowNotes: strings.Repeat("这是需要在恢复请求中压缩的节目详情。", 100),
		}},
	}}
}

func TestGenerateForReportRetriesEmptyTruncatedCompletionWithCompactPrompt(t *testing.T) {
	var attempts atomic.Int32
	var prompts []string
	var maxTokens []int
	var models []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request ChatCompletionRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		prompts = append(prompts, request.Messages[len(request.Messages)-1].Content)
		maxTokens = append(maxTokens, request.MaxTokens)
		models = append(models, request.Model)

		n := attempts.Add(1)
		if n == 1 {
			require.Nil(t, request.Thinking)
		} else {
			require.NotNil(t, request.Thinking)
			require.Equal(t, "disabled", request.Thinking.Type)
		}
		w.Header().Set("Content-Type", "application/json")
		if n == 1 {
			_, _ = w.Write(chatCompletionBody("deepseek-flash", "", "length", 4000))
			return
		}
		_, _ = w.Write(chatCompletionBody("deepseek-flash", "恢复后的摘要", "stop", 600))
	}))
	defer server.Close()

	summarizer := NewSummarizer(newSummarizerHTTPTestClient(t, server.URL), newSummarizerPromptManager(t))
	result, err := summarizer.GenerateForReport(
		context.Background(),
		summarizerRetryData(),
		"测试工作流",
		"请总结：{{range .Podcasts}}{{range .Episodes}}{{.ShowNotes}}{{end}}{{end}}",
		SummaryOptions{MaxTokens: 4000},
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "恢复后的摘要", result.Summary)
	require.Equal(t, 4600, result.TokensUsed)
	require.Equal(t, int32(2), attempts.Load())
	require.Len(t, prompts, 2)
	require.Len(t, maxTokens, 2)
	require.Len(t, models, 2)
	require.Equal(t, "deepseek-flash", models[1])
	require.Less(t, len(prompts[1]), len(prompts[0]))
	require.Less(t, maxTokens[1], maxTokens[0])
	require.Contains(t, prompts[1], "不要输出思考过程")
}

func TestGenerateForReportKeepsNonEmptyTruncatedCompletionWithoutRetry(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(chatCompletionBody("deepseek-flash", "半份摘要", "length", 4000))
	}))
	defer server.Close()

	summarizer := NewSummarizer(newSummarizerHTTPTestClient(t, server.URL), newSummarizerPromptManager(t))
	result, err := summarizer.GenerateForReport(
		context.Background(),
		summarizerRetryData(),
		"测试工作流",
		"请总结：{{range .Podcasts}}{{range .Episodes}}{{.ShowNotes}}{{end}}{{end}}",
		SummaryOptions{MaxTokens: 4000},
	)

	require.Error(t, err)
	require.ErrorIs(t, err, ErrIncompleteCompletion)
	require.NotNil(t, result)
	require.Equal(t, "半份摘要", result.Summary)
	require.True(t, result.Incomplete)
	require.Equal(t, int32(1), attempts.Load())
}

func TestGenerateForReportKeepsEmptyTruncatedStateWhenCompactRetryFails(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if n == 1 {
			_, _ = w.Write(chatCompletionBody("deepseek-flash", "", "length", 4000))
			return
		}
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":{"message":"temporary upstream failure","code":"bad_gateway"}}`))
	}))
	defer server.Close()

	summarizer := NewSummarizer(newSummarizerHTTPTestClient(t, server.URL), newSummarizerPromptManager(t))
	result, err := summarizer.GenerateForReport(
		context.Background(),
		summarizerRetryData(),
		"测试工作流",
		"请总结：{{range .Podcasts}}{{range .Episodes}}{{.ShowNotes}}{{end}}{{end}}",
		SummaryOptions{MaxTokens: 4000},
	)

	require.Error(t, err)
	require.ErrorIs(t, err, ErrIncompleteCompletion)
	require.Contains(t, err.Error(), "精简重试失败")
	require.NotNil(t, result)
	require.Empty(t, result.Summary)
	require.True(t, result.Incomplete)
	require.Equal(t, int32(2), attempts.Load())
}
