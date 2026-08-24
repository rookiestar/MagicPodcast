package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"magicpodcast/internal/episodecopilot"
	"magicpodcast/internal/handlers"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestEpisodeCopilotHandlerExposesScopeAndStreamsAnswers(t *testing.T) {
	module := &fakeEpisodeCopilotModule{
		scope: episodecopilot.ContextScope{
			EpisodeID:            71,
			ShowNotesAvailable:   true,
			TranscriptAvailable:  false,
			PrivateNoteAvailable: true,
		},
		events: []episodecopilot.StreamEvent{
			{
				Type:           episodecopilot.EventTypeContext,
				Message:        "未使用逐字稿",
				TranscriptUsed: false,
			},
			{
				Type:    episodecopilot.EventTypeAnswerDelta,
				Message: "## 回答\n\n基于 Show Notes。",
			},
			{
				Type:           episodecopilot.EventTypeComplete,
				Message:        "回答完成",
				FirstContentMS: 120,
				TotalMS:        420,
			},
		},
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := handlers.NewEpisodeCopilotHandler(module)
	router.GET("/api/v1/episodes/:id/copilot/context", handler.Context)
	router.POST("/api/v1/episodes/:id/copilot/questions", handler.Ask)

	scopeResponse := performEpisodeCopilotRequest(
		router,
		http.MethodGet,
		"/api/v1/episodes/71/copilot/context",
		"",
	)
	require.Equal(t, http.StatusOK, scopeResponse.Code)
	require.Contains(t, scopeResponse.Body.String(), `"transcript_available":false`)
	require.NotContains(t, scopeResponse.Body.String(), "private launch note")

	answerResponse := performEpisodeCopilotRequest(
		router,
		http.MethodPost,
		"/api/v1/episodes/71/copilot/questions",
		`{
			"question":"解释这段内容",
			"selection":"selected text",
			"selection_source":"show_notes",
			"include_private_note":true
		}`,
	)
	require.Equal(t, http.StatusOK, answerResponse.Code)
	require.Equal(
		t,
		"text/event-stream",
		answerResponse.Header().Get("Content-Type"),
	)
	require.Contains(t, answerResponse.Body.String(), `"type":"answer_delta"`)
	require.Contains(t, answerResponse.Body.String(), `"first_content_ms":120`)
	require.Equal(t, episodecopilot.QuestionRequest{
		EpisodeID:          71,
		Question:           "解释这段内容",
		Selection:          "selected text",
		SelectionSource:    episodecopilot.SelectionSourceShowNotes,
		IncludePrivateNote: true,
	}, module.request)
}

func TestEpisodeCopilotHandlerRejectsUnknownFieldsBeforeRuntime(t *testing.T) {
	module := &fakeEpisodeCopilotModule{}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := handlers.NewEpisodeCopilotHandler(module)
	router.POST("/api/v1/episodes/:id/copilot/questions", handler.Ask)

	response := performEpisodeCopilotRequest(
		router,
		http.MethodPost,
		"/api/v1/episodes/71/copilot/questions",
		`{"question":"hello","runtime_url":"http://127.0.0.1:9999"}`,
	)
	require.Equal(t, http.StatusBadRequest, response.Code)
	require.Contains(t, response.Body.String(), "INVALID_COPILOT_REQUEST")
	require.Zero(t, module.askCalls)
}

func TestEpisodeCopilotHandlerAcceptsMaximumMultibyteSelection(t *testing.T) {
	module := &fakeEpisodeCopilotModule{}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := handlers.NewEpisodeCopilotHandler(module)
	router.POST("/api/v1/episodes/:id/copilot/questions", handler.Ask)
	body, err := json.Marshal(map[string]any{
		"question":             "解释选区",
		"selection":            strings.Repeat("播", 12_000),
		"selection_source":     "show_notes",
		"include_private_note": false,
	})
	require.NoError(t, err)
	require.Less(t, len(body), 256<<10)

	response := performEpisodeCopilotRequest(
		router,
		http.MethodPost,
		"/api/v1/episodes/71/copilot/questions",
		string(body),
	)
	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, 1, module.askCalls)
	require.Len(t, []rune(module.request.Selection), 12_000)
}

func TestEpisodeCopilotHandlerRejectsOversizedBodyBeforeRuntime(t *testing.T) {
	module := &fakeEpisodeCopilotModule{}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := handlers.NewEpisodeCopilotHandler(module)
	router.POST("/api/v1/episodes/:id/copilot/questions", handler.Ask)

	response := performEpisodeCopilotRequest(
		router,
		http.MethodPost,
		"/api/v1/episodes/71/copilot/questions",
		`{"question":"hello","selection":"`+
			strings.Repeat("a", (256<<10)+1)+
			`","selection_source":"show_notes","include_private_note":false}`,
	)
	require.Equal(t, http.StatusBadRequest, response.Code)
	require.Contains(t, response.Body.String(), "INVALID_COPILOT_REQUEST")
	require.Zero(t, module.askCalls)
}

func performEpisodeCopilotRequest(
	router *gin.Engine,
	method string,
	path string,
	body string,
) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	router.ServeHTTP(recorder, request)
	return recorder
}

type fakeEpisodeCopilotModule struct {
	scope    episodecopilot.ContextScope
	events   []episodecopilot.StreamEvent
	request  episodecopilot.QuestionRequest
	askCalls int
	err      error
}

func (f *fakeEpisodeCopilotModule) ContextScope(
	context.Context,
	uint,
) (episodecopilot.ContextScope, error) {
	return f.scope, f.err
}

func (f *fakeEpisodeCopilotModule) Ask(
	_ context.Context,
	request episodecopilot.QuestionRequest,
) (<-chan episodecopilot.StreamEvent, error) {
	f.askCalls++
	f.request = request
	if f.err != nil {
		return nil, f.err
	}
	events := make(chan episodecopilot.StreamEvent, len(f.events))
	for _, event := range f.events {
		events <- event
	}
	close(events)
	return events, nil
}
