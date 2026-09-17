package handlers_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"magicpodcast/internal/codexruntime"
	"magicpodcast/internal/handlers"
	"magicpodcast/internal/models"
	"magicpodcast/internal/personidentity"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// scriptedIdentityRuntime replays provider-neutral events so the SSE stream
// carries the observable connection story of a real suggestion run.
type scriptedIdentityRuntime struct {
	mu       sync.Mutex
	events   []codexruntime.Event
	snapshot codexruntime.ExecutionSnapshot
}

func (r *scriptedIdentityRuntime) CreateExecution(_ context.Context, _ codexruntime.ExecutionRequest) (codexruntime.ExecutionSnapshot, error) {
	return codexruntime.ExecutionSnapshot{ID: "exec-sse", Status: codexruntime.StatusStarting, CreatedAt: time.Now().UTC()}, nil
}

func (r *scriptedIdentityRuntime) SubscribeExecution(_ context.Context, _ codexruntime.ExecutionID) (<-chan codexruntime.Event, error) {
	events := make(chan codexruntime.Event, len(r.events))
	for index, event := range r.events {
		event.Sequence = uint64(index + 1)
		events <- event
	}
	close(events)
	return events, nil
}

func (r *scriptedIdentityRuntime) CancelExecution(_ context.Context, id codexruntime.ExecutionID) (codexruntime.CancellationResult, error) {
	return codexruntime.CancellationResult{ExecutionID: id, Status: codexruntime.StatusCancelled}, nil
}

func (r *scriptedIdentityRuntime) GetExecution(_ context.Context, _ codexruntime.ExecutionID) (codexruntime.ExecutionSnapshot, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.snapshot, nil
}

func (r *scriptedIdentityRuntime) Close(context.Context) error { return nil }

func connectionProgress(state codexruntime.ProgressState, metadata map[string]string) codexruntime.Event {
	return codexruntime.Event{
		Type: codexruntime.EventProgress,
		Progress: &codexruntime.Progress{
			ActivityID: "a1",
			Ordinal:    1,
			Category:   codexruntime.CategoryConnection,
			State:      state,
			Metadata:   metadata,
		},
	}
}

type streamingRuntimeFixture struct {
	*personidentity.Service
	source personidentity.EpisodeSources
}

var scriptedIdentityResult = json.RawMessage(`{"people": [{"name": "林言", "kind": "participant", "status": "confirmed", "role": "host", "presence_basis": "self_introduction", "name_evidence": {"source": "show_notes", "fragment": 0, "quote": "主播林言"}, "presence_evidence": {"source": "transcript", "fragment": 1, "quote": "我是林言。"}, "role_evidence": {"source": "show_notes", "fragment": 0, "quote": "主播林言"}, "speech_bindings": [{"speaker_label": "A", "basis": "self_introduction", "evidence": {"source": "transcript", "fragment": 1, "quote": "我是林言。"}, "scope": "stable_speaker", "orders": [], "excluded_orders": []}]}], "speakers": [{"speaker_label": "A", "reason": "明确自我介绍", "candidates": [{"person_index": 0, "level": "direct", "basis": "self_introduction", "reason": "明确自我介绍", "evidence": [{"source": "transcript", "fragment": 1, "quote": "我是林言。"}], "counter_evidence": []}]}]}`)

func (f streamingRuntimeFixture) PrepareCurrent(ctx context.Context, _ uint) (personidentity.EpisodePeople, error) {
	return f.Prepare(ctx, f.source)
}

func newStreamingPersonServer(t *testing.T, runtime codexruntime.Runtime) (*httptest.Server, uint) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := openIsolatedPersonDB(t)
	pod := models.Podcast{XYZID: "runtime-sse", Title: "访谈", FeedURL: "https://example.test/runtime-sse"}
	require.NoError(t, db.Create(&pod).Error)
	ep := models.Episode{PodcastID: pod.ID, GUID: "runtime-sse", Title: "对话", ShowNotes: "本集主播林言"}
	require.NoError(t, db.Create(&ep).Error)
	service, err := personidentity.NewService(db, personidentity.NewRuntimeSuggester(runtime, t.TempDir()))
	require.NoError(t, err)
	fixture := streamingRuntimeFixture{
		Service: service,
		source: personidentity.EpisodeSources{
			EpisodeID:     ep.ID,
			SourceKind:    personidentity.SourceTranscript,
			SourceVersion: "v1",
			ShowNotes:     "本集主播林言",
			Segments:      []personidentity.Segment{{Order: 1, SpeakerLabel: "A", Text: "我是林言。"}},
		},
	}
	router := gin.New()
	router.POST("/episodes/:id/people/prepare", handlers.NewPersonHandler(fixture).Prepare)
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	return server, ep.ID
}

func readPersonSSE(t *testing.T, server *httptest.Server, episodeID uint) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/episodes/%d/people/prepare", server.URL, episodeID), strings.NewReader("{}"))
	require.NoError(t, err)
	request.Header.Set("Accept", "text/event-stream")
	response, err := server.Client().Do(request)
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, "text/event-stream", response.Header.Get("Content-Type"))
	reader := bufio.NewReader(response.Body)
	lines := make([]string, 0, 16)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		if strings.HasPrefix(line, "data:") {
			lines = append(lines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	return strings.Join(lines, "\n")
}

func decodePersonEvents(payload string) []map[string]any {
	events := []map[string]any{}
	for _, line := range strings.Split(payload, "\n") {
		if line == "" {
			continue
		}
		var event map[string]any
		if json.Unmarshal([]byte(line), &event) == nil {
			events = append(events, event)
		}
	}
	return events
}

func TestPersonPreparationStreamsRuntimePhasesThroughSSE(t *testing.T) {
	runtime := &scriptedIdentityRuntime{
		events: []codexruntime.Event{
			{Type: codexruntime.EventStarted},
			connectionProgress(codexruntime.ProgressStarted, map[string]string{"will_retry": "true", "error_class": "response_stream_disconnected", "http_status": "502"}),
			{Type: codexruntime.EventOutputDelta, Text: "{\"people\":"},
			{Type: codexruntime.EventTerminal},
		},
		snapshot: codexruntime.ExecutionSnapshot{
			ID:             "exec-sse",
			Status:         codexruntime.StatusCompleted,
			Result:         scriptedIdentityResult,
			RuntimeVersion: "sdk/0.147.0;runtime/0.147.0",
		},
	}
	server, episodeID := newStreamingPersonServer(t, runtime)
	payload := readPersonSSE(t, server, episodeID)
	events := decodePersonEvents(payload)

	types := make([]string, 0, len(events))
	for _, event := range events {
		types = append(types, event["type"].(string))
	}
	require.Contains(t, types, "runtime")
	require.Contains(t, types, "stage")
	require.Contains(t, types, "complete")

	requestIDs := map[string]bool{}
	for _, event := range events {
		id, _ := event["request_id"].(string)
		require.NotEmpty(t, id, "every event carries the request id")
		requestIDs[id] = true
	}
	require.Len(t, requestIDs, 1, "one request id correlates the whole stream")
	require.Regexp(t, `^[0-9a-f]{16}$`, events[0]["request_id"])

	// Reconnect phases arrive before the terminal event, with provider facts only.
	lastReconnect := -1
	for index, event := range events {
		if event["type"] == "runtime" {
			runtime, ok := event["runtime"].(map[string]any)
			require.True(t, ok, "runtime events carry a nested runtime object")
			if runtime["phase"] == "reconnecting" {
				lastReconnect = index
				require.Equal(t, true, runtime["will_retry"])
			}
		}
	}
	require.Greater(t, lastReconnect, 0, "a reconnect must be observable mid-stream")
	require.Equal(t, "complete", types[len(types)-1], "the stream ends with the saved draft")
	require.NotContains(t, payload, "private", "no provider payload reaches the stream")
}

func TestPersonPreparationClassifiesRuntimeFailureForTheClient(t *testing.T) {
	runtime := &scriptedIdentityRuntime{
		events: []codexruntime.Event{
			{Type: codexruntime.EventStarted},
			connectionProgress(codexruntime.ProgressStarted, map[string]string{"will_retry": "true", "error_class": "http_connection_failed"}),
			{Type: codexruntime.EventTerminal},
		},
		snapshot: codexruntime.ExecutionSnapshot{
			ID:          "exec-sse",
			Status:      codexruntime.StatusFailed,
			ErrorCode:   codexruntime.ErrorRuntimeUnavailable,
			SafeMessage: "runtime preflight failed",
		},
	}
	server, episodeID := newStreamingPersonServer(t, runtime)
	payload := readPersonSSE(t, server, episodeID)
	events := decodePersonEvents(payload)

	require.NotEmpty(t, events)
	last := events[len(events)-1]
	require.Equal(t, "error", last["type"])
	require.Equal(t, "PERSON_RUNTIME_UNAVAILABLE", last["code"])
	require.Equal(t, "runtime_unavailable", last["classification"])
	require.Equal(t, true, last["retryable"])
	require.NotContains(t, payload, "preflight", "internal safe messages stay out of the client stream")
	require.NotContains(t, payload, `"type":"complete"`, "a failed run never announces success")
}

func TestPersonPreparationKeepsTerminalProviderFailureClassification(t *testing.T) {
	for _, code := range []string{"authentication_failed", "quota_exceeded", "upstream_connection_failed"} {
		t.Run(code, func(t *testing.T) {
			r := &scriptedIdentityRuntime{events: []codexruntime.Event{{Type: codexruntime.EventStarted}, {Type: codexruntime.EventTerminal}}, snapshot: codexruntime.ExecutionSnapshot{Status: codexruntime.StatusFailed, ErrorCode: code, SafeMessage: "private-provider-detail"}}
			server, id := newStreamingPersonServer(t, r)
			payload := readPersonSSE(t, server, id)
			events := decodePersonEvents(payload)
			require.Equal(t, code, events[len(events)-1]["classification"])
			require.NotContains(t, payload, "private-provider-detail")
		})
	}
}
