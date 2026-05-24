package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

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
