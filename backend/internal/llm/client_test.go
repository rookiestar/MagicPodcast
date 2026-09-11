package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"magicpodcast/internal/config"
)

func TestDeepSeekProviderNormalizesLegacyGLMModel(t *testing.T) {
	var requestedModel string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/chat/completions", r.URL.Path)

		var req ChatCompletionRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		requestedModel = req.Model

		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(`{
			"id": "test",
			"object": "chat.completion",
			"created": 1,
			"model": "deepseek-v4-flash",
			"choices": [{
				"index": 0,
				"message": {"role": "assistant", "content": "OK"},
				"finish_reason": "stop"
			}],
			"usage": {"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2}
		}`))
		require.NoError(t, err)
	}))
	defer server.Close()

	client := NewClient(&config.LLMConfig{
		Enabled:            true,
		Provider:           config.LLMProviderDeepSeek,
		APIKey:             "test-key",
		BaseURL:            server.URL,
		DefaultModel:       "deepseek-v4-flash",
		Timeout:            5,
		RateLimitPerMinute: 60,
	})

	result, err := client.GenerateSummary(
		context.Background(),
		"",
		"test",
		SummaryOptions{Model: "glm-4.5-air", MaxTokens: 8},
	)

	require.NoError(t, err)
	require.Equal(t, "deepseek-v4-flash", requestedModel)
	require.Equal(t, "deepseek-v4-flash", result.ModelUsed)
}

func TestGenerateSummaryUsesConfiguredMaxTokensWhenUnset(t *testing.T) {
	var requestedMaxTokens int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req ChatCompletionRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		requestedMaxTokens = req.MaxTokens
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write(successChatCompletionBody("deepseek-v4-flash", "OK"))
		require.NoError(t, err)
	}))
	defer server.Close()

	client := NewClient(&config.LLMConfig{
		Enabled:             true,
		Provider:            config.LLMProviderDeepSeek,
		APIKey:              "test-key",
		BaseURL:             server.URL,
		DefaultModel:        "deepseek-v4-flash",
		MaxTokensPerRequest: 1234,
		Timeout:             5,
		RateLimitPerMinute:  60,
	})

	result, err := client.GenerateSummary(context.Background(), "", "test", SummaryOptions{})
	require.NoError(t, err)
	require.Equal(t, "OK", result.Summary)
	require.Equal(t, 1234, requestedMaxTokens)
}

func TestGenerateSummaryRejectsMaxTokensAboveConfiguredLimit(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
	}))
	defer server.Close()

	client := NewClient(&config.LLMConfig{
		Enabled:             true,
		Provider:            config.LLMProviderDeepSeek,
		APIKey:              "test-key",
		BaseURL:             server.URL,
		DefaultModel:        "deepseek-v4-flash",
		MaxTokensPerRequest: 1234,
		Timeout:             5,
		RateLimitPerMinute:  60,
	})

	_, err := client.GenerateSummary(context.Background(), "", "test", SummaryOptions{MaxTokens: 1235})
	require.EqualError(t, err, "llm max_tokens 1235 exceeds configured limit 1234")
	require.Zero(t, requests)
}

func successChatCompletionBody(model, content string) []byte {
	payload, _ := json.Marshal(ChatCompletionResponse{
		ID:      "test",
		Object:  "chat.completion",
		Created: 1,
		Model:   model,
		Choices: []Choice{{
			Index:        0,
			Message:      Message{Role: "assistant", Content: content},
			FinishReason: "stop",
		}},
		Usage: Usage{PromptTokens: 1, CompletionTokens: 1, TotalTokens: 2},
	})
	return payload
}

func TestGenerateSummaryRetriesAfterBodyReadTimeout(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NotEmpty(t, body)
		var req ChatCompletionRequest
		require.NoError(t, json.Unmarshal(body, &req))
		require.Equal(t, "test-prompt", req.Messages[len(req.Messages)-1].Content)

		n := attempts.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		if n == 1 {
			time.Sleep(250 * time.Millisecond)
			return
		}
		_, err = w.Write(successChatCompletionBody("deepseek-v4-flash", "OK"))
		require.NoError(t, err)
	}))
	defer server.Close()

	client := NewClient(&config.LLMConfig{
		Enabled:            true,
		Provider:           config.LLMProviderDeepSeek,
		APIKey:             "test-key",
		BaseURL:            server.URL,
		DefaultModel:       "deepseek-v4-flash",
		Timeout:            1,
		MaxRetries:         1,
		RetryInterval:      1,
		RateLimitPerMinute: 60,
	})
	client.httpClient.Timeout = 80 * time.Millisecond

	result, err := client.GenerateSummary(
		context.Background(),
		"",
		"test-prompt",
		SummaryOptions{MaxTokens: 8},
	)
	require.NoError(t, err)
	require.Equal(t, "OK", result.Summary)
	require.Equal(t, int32(2), attempts.Load())
}

func TestGenerateSummaryDoesNotRetryHTTP4xx(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"bad request","code":"invalid"}}`))
	}))
	defer server.Close()

	client := NewClient(&config.LLMConfig{
		Enabled:            true,
		Provider:           config.LLMProviderDeepSeek,
		APIKey:             "test-key",
		BaseURL:            server.URL,
		DefaultModel:       "deepseek-v4-flash",
		Timeout:            5,
		MaxRetries:         2,
		RetryInterval:      1,
		RateLimitPerMinute: 60,
	})

	_, err := client.GenerateSummary(context.Background(), "", "test", SummaryOptions{MaxTokens: 8})
	require.Error(t, err)
	require.Contains(t, err.Error(), "LLM API错误")
	require.Equal(t, int32(1), attempts.Load())
}

func TestGenerateSummaryDoesNotRetryHTTP5xx(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"unavailable"}`))
	}))
	defer server.Close()

	client := NewClient(&config.LLMConfig{
		Enabled:            true,
		Provider:           config.LLMProviderDeepSeek,
		APIKey:             "test-key",
		BaseURL:            server.URL,
		DefaultModel:       "deepseek-v4-flash",
		Timeout:            5,
		MaxRetries:         2,
		RetryInterval:      1,
		RateLimitPerMinute: 60,
	})

	_, err := client.GenerateSummary(context.Background(), "", "test", SummaryOptions{MaxTokens: 8})
	require.Error(t, err)
	require.Contains(t, err.Error(), "LLM API返回错误状态: 500")
	require.Equal(t, int32(1), attempts.Load())
}

func chatCompletionBody(model, content, finishReason string, totalTokens int) []byte {
	payload, _ := json.Marshal(ChatCompletionResponse{
		ID:      "test",
		Object:  "chat.completion",
		Created: 1,
		Model:   model,
		Choices: []Choice{{
			Index:        0,
			Message:      Message{Role: "assistant", Content: content},
			FinishReason: finishReason,
		}},
		Usage: Usage{PromptTokens: 1, CompletionTokens: 1, TotalTokens: totalTokens},
	})
	return payload
}

func testLLMClient(t *testing.T, serverURL string, maxRetries int) *Client {
	t.Helper()
	return NewClient(&config.LLMConfig{
		Enabled:            true,
		Provider:           config.LLMProviderDeepSeek,
		APIKey:             "test-key",
		BaseURL:            serverURL,
		DefaultModel:       "deepseek-v4-flash",
		Timeout:            5,
		MaxRetries:         maxRetries,
		RetryInterval:      1,
		RateLimitPerMinute: 60,
	})
}

func TestGenerateSummarySucceedsOnStopWithNonEmptyBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write(chatCompletionBody("deepseek-v4-flash", "本周有三档教育节目值得听。", "stop", 42))
		require.NoError(t, err)
	}))
	defer server.Close()

	result, err := testLLMClient(t, server.URL, 2).GenerateSummary(
		context.Background(), "", "test", SummaryOptions{MaxTokens: 8},
	)
	require.NoError(t, err)
	require.Contains(t, result.Summary, "教育节目")
	require.Equal(t, "stop", result.FinishReason)
	require.False(t, result.Incomplete)
	require.Equal(t, 42, result.TokensUsed)
}

func TestGenerateSummaryRetriesEmptyAndWhitespaceBodies(t *testing.T) {
	cases := []struct {
		name    string
		content string
	}{
		{name: "empty string", content: ""},
		{name: "whitespace", content: "  \n\t  \n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var attempts atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				n := attempts.Add(1)
				w.Header().Set("Content-Type", "application/json")
				content := tc.content
				if n > 1 {
					content = "有效摘要"
				}
				_, err := w.Write(chatCompletionBody("deepseek-v4-flash", content, "stop", 88))
				require.NoError(t, err)
			}))
			defer server.Close()

			result, err := testLLMClient(t, server.URL, 2).GenerateSummary(
				context.Background(), "", "test", SummaryOptions{MaxTokens: 8},
			)
			require.NoError(t, err)
			require.Equal(t, "有效摘要", result.Summary)
			require.Equal(t, int32(2), attempts.Load())
		})
	}
}

func TestGenerateSummaryEmptyBodyDoesNotExceedMaxRetries(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write(chatCompletionBody("deepseek-v4-flash", "", "stop", 7245))
		require.NoError(t, err)
	}))
	defer server.Close()

	result, err := testLLMClient(t, server.URL, 2).GenerateSummary(
		context.Background(), "", "test", SummaryOptions{MaxTokens: 8},
	)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrEmptyCompletion)
	require.NotNil(t, result)
	require.Empty(t, strings.TrimSpace(result.Summary))
	require.Equal(t, 7245, result.TokensUsed)
	require.Equal(t, int32(3), attempts.Load())
}

func TestGenerateSummaryTruncationIsIncompleteAndNotRetried(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write(chatCompletionBody("deepseek-v4-flash", "只写了一半", "length", 120))
		require.NoError(t, err)
	}))
	defer server.Close()

	result, err := testLLMClient(t, server.URL, 2).GenerateSummary(
		context.Background(), "", "test", SummaryOptions{MaxTokens: 8},
	)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrIncompleteCompletion)
	require.NotNil(t, result)
	require.True(t, result.Incomplete)
	require.Equal(t, "只写了一半", result.Summary)
	require.Equal(t, int32(1), attempts.Load())
}

func TestGenerateSummaryMissingFinishReasonIsNotSuccess(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write(chatCompletionBody("deepseek-v4-flash", "看起来像摘要", "", 50))
		require.NoError(t, err)
	}))
	defer server.Close()

	result, err := testLLMClient(t, server.URL, 2).GenerateSummary(
		context.Background(), "", "test", SummaryOptions{MaxTokens: 8},
	)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrInvalidFinishReason)
	require.NotNil(t, result)
	require.False(t, result.Incomplete)
	require.Equal(t, int32(1), attempts.Load())
}

func TestGenerateSummaryRetriesTransportFailureWithinBudget(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n == 1 {
			hj, ok := w.(http.Hijacker)
			require.True(t, ok)
			conn, _, err := hj.Hijack()
			require.NoError(t, err)
			require.NoError(t, conn.Close())
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write(chatCompletionBody("deepseek-v4-flash", "恢复后的摘要", "stop", 10))
		require.NoError(t, err)
	}))
	defer server.Close()

	result, err := testLLMClient(t, server.URL, 1).GenerateSummary(
		context.Background(), "", "test", SummaryOptions{MaxTokens: 8},
	)
	require.NoError(t, err)
	require.Equal(t, "恢复后的摘要", result.Summary)
	require.Equal(t, int32(2), attempts.Load())
}

func TestGenerateSummaryDoesNotRetryWhenDisabled(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(&config.LLMConfig{
		Enabled:            false,
		Provider:           config.LLMProviderDeepSeek,
		APIKey:             "test-key",
		BaseURL:            server.URL,
		DefaultModel:       "deepseek-v4-flash",
		Timeout:            5,
		MaxRetries:         2,
		RateLimitPerMinute: 60,
	})
	_, err := client.GenerateSummary(context.Background(), "", "test", SummaryOptions{MaxTokens: 8})
	require.Error(t, err)
	require.Contains(t, err.Error(), "LLM功能未启用")
	require.Equal(t, int32(0), attempts.Load())
}
