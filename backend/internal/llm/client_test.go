package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
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
