package episodecopilot

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"magicpodcast/internal/codexruntime"

	"github.com/stretchr/testify/require"
)

func TestServiceAnswersThroughSeparatedSearchAndReadOnlyFinalExecutions(
	t *testing.T,
) {
	loader := &fakeContextLoader{
		context: EpisodeContext{
			EpisodeID:    41,
			EpisodeTitle: "Runtime contracts",
			PodcastTitle: "System Notes",
			ShowNotes:    "The host uses a restricted runtime contract.",
			Transcript:   "[00:12] Runtime permissions are reduced per turn.",
			PrivateNotes: "Only compare this with my unpublished launch plan.",
		},
	}
	runtime := newFakeRuntime(
		fakeExecution{
			result: json.RawMessage(`{
				"resources":[
					{
						"title":"Codex SDK guide",
						"url":"https://developers.openai.com/codex/sdk/",
						"relevance":"Official SDK behavior"
					},
					{
						"title":"Local secret",
						"url":"http://127.0.0.1:8080/private",
						"relevance":"Must be rejected"
					}
				],
				"conflicts":[],
				"limitations":[]
			}`),
		},
		fakeExecution{
			deltas: []string{"权限应", "按次收窄 [逐字稿 L1-L1]。"},
			result: json.RawMessage(`{"text":"权限应按次收窄。"}`),
		},
	)
	service, err := NewService(
		loader,
		runtime,
		t.TempDir(),
		WithClock(func() time.Time {
			return time.Date(2026, 8, 24, 20, 0, 0, 0, time.UTC)
		}),
	)
	require.NoError(t, err)

	events, err := service.Ask(context.Background(), QuestionRequest{
		EpisodeID:          41,
		Question:           "为什么需要按次收窄权限？",
		Selection:          "Runtime permissions are reduced per turn.",
		SelectionSource:    SelectionSourceTranscript,
		IncludePrivateNote: true,
	})
	require.NoError(t, err)

	var answer strings.Builder
	var completed *StreamEvent
	for event := range events {
		if event.Type == EventTypeAnswerDelta {
			answer.WriteString(event.Message)
		}
		if event.Type == EventTypeComplete {
			copied := event
			completed = &copied
		}
	}

	requests := runtime.Requests()
	require.Len(t, requests, 2)
	require.Equal(t, codexruntime.ExecutionKindAssistant, requests[0].Kind)
	require.Equal(
		t,
		[]codexruntime.ToolCapability{codexruntime.ToolWebSearch},
		requests[0].ToolRestriction.Allowed,
	)
	require.NotContains(t, requests[0].Prompt, loader.context.PrivateNotes)
	require.Equal(t, codexruntime.ExecutionKindAssistant, requests[1].Kind)
	require.Empty(t, requests[1].ToolRestriction.Allowed)
	require.Contains(t, requests[1].Prompt, loader.context.PrivateNotes)
	require.NotContains(
		t,
		requests[1].Prompt,
		"https://developers.openai.com/codex/sdk/",
	)

	rendered := answer.String()
	require.Contains(t, rendered, "权限应按次收窄")
	require.Contains(t, rendered, "[逐字稿 L1-L1]")
	require.Contains(t, rendered, "单集内部来源")
	require.Contains(t, rendered, "逐字稿")
	require.Contains(t, rendered, "公开外部来源")
	require.Contains(
		t,
		rendered,
		"https://developers.openai.com/codex/sdk/",
	)
	require.NotContains(t, rendered, "127.0.0.1")
	require.Contains(t, rendered, "2026-08-24")
	require.NotNil(t, completed)
	require.True(t, completed.TranscriptUsed)
	require.True(t, completed.PrivateNoteIncluded)
	require.GreaterOrEqual(t, completed.FirstContentMS, int64(0))
	require.GreaterOrEqual(t, completed.TotalMS, completed.FirstContentMS)
}

func TestServiceDegradesWithoutTranscriptAndExcludesPrivateNoteByDefault(
	t *testing.T,
) {
	loader := &fakeContextLoader{
		context: EpisodeContext{
			EpisodeID:    42,
			EpisodeTitle: "Show Notes only",
			PodcastTitle: "System Notes",
			ShowNotes:    "A verified statement in Show Notes.",
			PrivateNotes: "Do not send this note.",
		},
	}
	runtime := newFakeRuntime(
		fakeExecution{
			result: json.RawMessage(
				`{"resources":[],"conflicts":[],"limitations":["没有外部来源"]}`,
			),
		},
		fakeExecution{
			deltas: []string{"只能依据 Show Notes 回答 [Show Notes L1-L1]。"},
			result: json.RawMessage(`{"text":"只能依据 Show Notes 回答。"}`),
		},
	)
	service, err := NewService(loader, runtime, t.TempDir())
	require.NoError(t, err)

	events, err := service.Ask(context.Background(), QuestionRequest{
		EpisodeID:       42,
		Question:        "这句话是否有依据？",
		Selection:       "verified statement",
		SelectionSource: SelectionSourceShowNotes,
	})
	require.NoError(t, err)

	var contextEvent StreamEvent
	var answer strings.Builder
	for event := range events {
		if event.Type == EventTypeContext {
			contextEvent = event
		}
		if event.Type == EventTypeAnswerDelta {
			answer.WriteString(event.Message)
		}
	}

	require.False(t, contextEvent.TranscriptUsed)
	require.Contains(t, contextEvent.Message, "未使用逐字稿")
	require.Contains(t, answer.String(), "未使用逐字稿")
	for _, request := range runtime.Requests() {
		require.NotContains(t, request.Prompt, loader.context.PrivateNotes)
	}
}

func TestServiceKeepsTheSelectedExcerptWhenCroppingLargeContext(t *testing.T) {
	const selected = "[02:14:00] The selected claim is near the end."
	loader := &fakeContextLoader{
		context: EpisodeContext{
			EpisodeID:    43,
			EpisodeTitle: "Long transcript",
			PodcastTitle: "System Notes",
			ShowNotes:    "Short notes",
			Transcript:   strings.Repeat("Earlier transcript line.\n", 9_000) + selected,
		},
	}
	runtime := newFakeRuntime(
		fakeExecution{
			result: json.RawMessage(
				`{"resources":[],"conflicts":[],"limitations":[]}`,
			),
		},
		fakeExecution{
			deltas: []string{"已定位选区 [逐字稿 L1-L1]。"},
			result: json.RawMessage(`{"text":"已定位选区。"}`),
		},
	)
	service, err := NewService(loader, runtime, t.TempDir())
	require.NoError(t, err)

	events, err := service.Ask(context.Background(), QuestionRequest{
		EpisodeID:       43,
		Question:        "解释选区",
		Selection:       selected,
		SelectionSource: SelectionSourceTranscript,
	})
	require.NoError(t, err)
	for range events {
	}

	requests := runtime.Requests()
	require.Len(t, requests, 2)
	require.GreaterOrEqual(t, strings.Count(requests[0].Prompt, selected), 2)
	require.GreaterOrEqual(t, strings.Count(requests[1].Prompt, selected), 2)
	require.Less(t, utf8.RuneCountInString(requests[0].Prompt), 220_000)
}

func TestServiceKeepsWhitespaceNormalizedSelectionWhenCropping(t *testing.T) {
	selectedInSource := "The SELECTED\n\t claim remains visible."
	source := strings.Repeat("Earlier transcript line.\n", 9_000) +
		selectedInSource

	start, end, matched := normalizedSelectionRange(
		source,
		"the selected claim remains visible.",
	)
	require.True(t, matched)
	require.Equal(t, selectedInSource, string([]rune(source)[start:end]))

	clipped := clipAroundSelection(
		source,
		"the selected claim remains visible.",
		500,
	)
	require.Contains(t, clipped, selectedInSource)
	require.Contains(t, clipped, "[内容前部已裁剪]")
}

func TestPublicHTTPURLRejectsNonPublicAndCredentializedLocations(t *testing.T) {
	rejected := []string{
		"file:///tmp/private",
		"http://localhost:8080/private",
		"http://api.local/private",
		"http://printer/private",
		"http://127.0.0.1/private",
		"http://2130706433/private",
		"http://0x7f000001/private",
		"http://0177.0.0.1/private",
		"http://[::1]/private",
		"http://10.0.0.8/private",
		"http://169.254.1.8/private",
		"http://100.64.0.1/private",
		"https://user:secret@example.com/private",
		"https://example.com/private?token=secret",
		"https://example.com/private?X-Amz-Signature=secret",
		"https://example.com/private#access_token=secret",
		"https://example.com/redirect?next=http%3A%2F%2F127.0.0.1%2Fprivate",
		"https://example.com/redirect?next=file%3A%2F%2F%2Ftmp%2Fprivate",
	}
	for _, candidate := range rejected {
		t.Run(candidate, func(t *testing.T) {
			_, ok := publicHTTPURL(candidate)
			require.False(t, ok)
		})
	}

	normalized, ok := publicHTTPURL(
		"HTTPS://EXAMPLE.COM/reference?edition=1#private-fragment",
	)
	require.True(t, ok)
	require.Equal(t, "https://example.com/reference?edition=1", normalized)
}

func TestServiceFiltersModelControlledLinksOutsideTheValidatedSourceAppendix(
	t *testing.T,
) {
	loader := &fakeContextLoader{
		context: EpisodeContext{
			EpisodeID:    47,
			EpisodeTitle: "Output safety",
			PodcastTitle: "System Notes",
			ShowNotes:    "Only validated source links may be clickable.",
		},
	}
	runtime := newFakeRuntime(
		fakeExecution{
			result: json.RawMessage(`{
				"resources":[{
					"title":"Official [guide](http://127.0.0.1/title)",
					"url":"https://example.com/reference",
					"relevance":"See https://private.example/?token=secret"
				}],
				"conflicts":["Read [private](http://127.0.0.1/conflict)"],
				"limitations":["Try www.unverified.example instead"]
			}`),
		},
		fakeExecution{
			deltas: []string{
				"结论 [Show Notes L1-L1] 见 [伪资源](htt",
				"p://127.0.0.1/private)，也不要访问 www.",
				"unverified.example。",
			},
			result: json.RawMessage(
				`{"text":"结论只使用服务端验证来源。"}`,
			),
		},
	)
	service, err := NewService(
		loader,
		runtime,
		t.TempDir(),
		WithClock(func() time.Time {
			return time.Date(2026, 8, 24, 22, 0, 0, 0, time.UTC)
		}),
	)
	require.NoError(t, err)

	events, err := service.Ask(context.Background(), QuestionRequest{
		EpisodeID: 47,
		Question:  "有哪些来源？",
	})
	require.NoError(t, err)
	var answer strings.Builder
	for event := range events {
		if event.Type == EventTypeAnswerDelta {
			answer.WriteString(event.Message)
		}
	}

	rendered := answer.String()
	require.Contains(t, rendered, "https://example.com/reference")
	require.NotContains(t, rendered, "127.0.0.1")
	require.NotContains(t, rendered, "private.example")
	require.NotContains(t, rendered, "unverified.example")
	require.NotContains(t, rendered, "](http://127")
	require.Contains(t, rendered, "[链接由服务端来源区提供]")
}

func TestServiceFailsClosedWhenAnswerHasNoVerifiableCitation(t *testing.T) {
	loader := &fakeContextLoader{
		context: EpisodeContext{
			EpisodeID:    48,
			EpisodeTitle: "Citation gate",
			PodcastTitle: "System Notes",
			ShowNotes:    "A sourced statement.",
		},
	}
	runtime := newFakeRuntime(
		fakeExecution{
			result: json.RawMessage(
				`{"resources":[],"conflicts":[],"limitations":[]}`,
			),
		},
		fakeExecution{
			deltas: []string{"这是没有任何来源定位的事实性回答。"},
			result: json.RawMessage(
				`{"text":"这是没有任何来源定位的事实性回答。"}`,
			),
		},
	)
	service, err := NewService(loader, runtime, t.TempDir())
	require.NoError(t, err)

	events, err := service.Ask(context.Background(), QuestionRequest{
		EpisodeID: 48,
		Question:  "这句话可靠吗？",
	})
	require.NoError(t, err)
	var answer strings.Builder
	var failure *StreamEvent
	for event := range events {
		if event.Type == EventTypeAnswerDelta {
			answer.WriteString(event.Message)
		}
		if event.Type == EventTypeError {
			copied := event
			failure = &copied
		}
	}

	require.Empty(t, answer.String())
	require.NotNil(t, failure)
	require.Equal(t, "runtime_protocol_error", failure.Code)
	require.Contains(t, failure.Message, "可核验的来源定位")
}

func TestServiceRefusesFactsWhenTheEpisodeHasNoEvidence(t *testing.T) {
	loader := &fakeContextLoader{
		context: EpisodeContext{
			EpisodeID:    49,
			EpisodeTitle: "Metadata only",
			PodcastTitle: "System Notes",
		},
	}
	runtime := newFakeRuntime(
		fakeExecution{
			result: json.RawMessage(
				`{"resources":[],"conflicts":[],"limitations":[]}`,
			),
		},
		fakeExecution{
			deltas: []string{"模型声称了一个没有依据的事实。"},
			result: json.RawMessage(
				`{"text":"模型声称了一个没有依据的事实。"}`,
			),
		},
	)
	service, err := NewService(loader, runtime, t.TempDir())
	require.NoError(t, err)

	events, err := service.Ask(context.Background(), QuestionRequest{
		EpisodeID: 49,
		Question:  "给出事实结论",
	})
	require.NoError(t, err)
	var answer strings.Builder
	for event := range events {
		if event.Type == EventTypeAnswerDelta {
			answer.WriteString(event.Message)
		}
		require.NotEqual(t, EventTypeError, event.Type)
	}

	require.Contains(t, answer.String(), noEvidenceAnswerMessage)
	require.NotContains(t, answer.String(), "模型声称")
}

func TestServiceCancelsTheActiveRuntimeWhenTheStreamDisconnects(t *testing.T) {
	loader := &fakeContextLoader{
		context: EpisodeContext{
			EpisodeID:    44,
			EpisodeTitle: "Cancellation",
			PodcastTitle: "System Notes",
			ShowNotes:    "Cancellation must target only the active turn.",
		},
	}
	runtime := newFakeRuntime(fakeExecution{block: true})
	service, err := NewService(loader, runtime, t.TempDir())
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())

	events, err := service.Ask(ctx, QuestionRequest{
		EpisodeID: 44,
		Question:  "等待时如何取消？",
	})
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		return len(runtime.Requests()) == 1
	}, time.Second, 10*time.Millisecond)
	cancel()
	for range events {
	}

	require.Len(t, runtime.CancelledExecutions(), 1)
}

func TestServicePreservesPartialAnswerAndReportsTerminalFailure(t *testing.T) {
	loader := &fakeContextLoader{
		context: EpisodeContext{
			EpisodeID:    45,
			EpisodeTitle: "Partial failure",
			PodcastTitle: "System Notes",
			ShowNotes:    "The answer may fail after a readable delta.",
		},
	}
	runtime := newFakeRuntime(
		fakeExecution{
			result: json.RawMessage(
				`{"resources":[],"conflicts":[],"limitations":[]}`,
			),
		},
		fakeExecution{
			deltas: []string{"已经生成的部分答案 [Show Notes L1-L1]。"},
			status: codexruntime.StatusFailed,
		},
	)
	service, err := NewService(loader, runtime, t.TempDir())
	require.NoError(t, err)

	events, err := service.Ask(context.Background(), QuestionRequest{
		EpisodeID: 45,
		Question:  "失败时保留什么？",
	})
	require.NoError(t, err)

	var answer strings.Builder
	var errorEvent *StreamEvent
	completed := false
	for event := range events {
		if event.Type == EventTypeAnswerDelta {
			answer.WriteString(event.Message)
		}
		if event.Type == EventTypeError {
			copied := event
			errorEvent = &copied
		}
		completed = completed || event.Type == EventTypeComplete
	}
	require.Contains(t, answer.String(), "已经生成的部分答案")
	require.NotNil(t, errorEvent)
	require.True(t, errorEvent.Retryable)
	require.False(t, completed)
}

func TestServiceRejectsSelectionOutsideTheCurrentEpisode(t *testing.T) {
	loader := &fakeContextLoader{
		context: EpisodeContext{
			EpisodeID:    46,
			EpisodeTitle: "Selection scope",
			PodcastTitle: "System Notes",
			ShowNotes:    "Only this current episode text is authorized.",
		},
	}
	runtime := newFakeRuntime()
	service, err := NewService(loader, runtime, t.TempDir())
	require.NoError(t, err)

	_, err = service.Ask(context.Background(), QuestionRequest{
		EpisodeID:       46,
		Question:        "解释选区",
		Selection:       "Text copied from another episode.",
		SelectionSource: SelectionSourceShowNotes,
	})
	require.ErrorIs(t, err, ErrInvalidQuestion)
	require.Empty(t, runtime.Requests())
}

type fakeContextLoader struct {
	context EpisodeContext
	scope   ContextScope
	err     error
}

func (f *fakeContextLoader) Describe(
	context.Context,
	uint,
) (ContextScope, error) {
	if f.err != nil {
		return ContextScope{}, f.err
	}
	if f.scope.EpisodeID != 0 {
		return f.scope, nil
	}
	return ContextScope{
		EpisodeID:            f.context.EpisodeID,
		ShowNotesAvailable:   strings.TrimSpace(f.context.ShowNotes) != "",
		TranscriptAvailable:  strings.TrimSpace(f.context.Transcript) != "",
		PrivateNoteAvailable: strings.TrimSpace(f.context.PrivateNotes) != "",
	}, nil
}

func (f *fakeContextLoader) Load(
	context.Context,
	uint,
	bool,
) (EpisodeContext, error) {
	if f.err != nil {
		return EpisodeContext{}, f.err
	}
	return f.context, nil
}

type fakeExecution struct {
	deltas []string
	result json.RawMessage
	block  bool
	status codexruntime.ExecutionStatus
}

type fakeRuntime struct {
	mu           sync.Mutex
	queue        []fakeExecution
	requests     []codexruntime.ExecutionRequest
	snapshots    map[codexruntime.ExecutionID]codexruntime.ExecutionSnapshot
	events       map[codexruntime.ExecutionID]chan codexruntime.Event
	cancelled    []codexruntime.ExecutionID
	nextID       int
	createErr    error
	getErr       error
	subscribeErr error
}

func newFakeRuntime(queue ...fakeExecution) *fakeRuntime {
	return &fakeRuntime{
		queue:     append([]fakeExecution(nil), queue...),
		snapshots: make(map[codexruntime.ExecutionID]codexruntime.ExecutionSnapshot),
		events:    make(map[codexruntime.ExecutionID]chan codexruntime.Event),
	}
}

func (f *fakeRuntime) CreateExecution(
	_ context.Context,
	request codexruntime.ExecutionRequest,
) (codexruntime.ExecutionSnapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.createErr != nil {
		return codexruntime.ExecutionSnapshot{}, f.createErr
	}
	if len(f.queue) == 0 {
		return codexruntime.ExecutionSnapshot{}, errors.New("unexpected execution")
	}
	next := f.queue[0]
	f.queue = f.queue[1:]
	f.nextID++
	id := codexruntime.ExecutionID(
		"episode-copilot-" + time.Unix(int64(f.nextID), 0).UTC().Format("150405"),
	)
	f.requests = append(f.requests, request)
	now := time.Now().UTC()
	snapshot := codexruntime.ExecutionSnapshot{
		ID:             id,
		Kind:           request.Kind,
		Status:         codexruntime.StatusRunning,
		RuntimeVersion: "fake-runtime",
		CreatedAt:      now,
		StartedAt:      &now,
	}
	events := make(chan codexruntime.Event, len(next.deltas)+2)
	events <- codexruntime.Event{
		ExecutionID: id,
		Sequence:    1,
		Type:        codexruntime.EventStarted,
		ObservedAt:  now,
	}
	for index, delta := range next.deltas {
		events <- codexruntime.Event{
			ExecutionID: id,
			Sequence:    uint64(index + 2),
			Type:        codexruntime.EventOutputDelta,
			Text:        delta,
			ObservedAt:  now,
		}
	}
	if !next.block {
		completedAt := now
		snapshot.Status = next.status
		if snapshot.Status == "" {
			snapshot.Status = codexruntime.StatusCompleted
		}
		snapshot.Result = append(json.RawMessage(nil), next.result...)
		snapshot.CompletedAt = &completedAt
		events <- codexruntime.Event{
			ExecutionID: id,
			Sequence:    uint64(len(next.deltas) + 2),
			Type:        codexruntime.EventTerminal,
			ObservedAt:  now,
		}
		close(events)
	}
	f.snapshots[id] = snapshot
	f.events[id] = events
	return snapshot, nil
}

func (f *fakeRuntime) SubscribeExecution(
	ctx context.Context,
	id codexruntime.ExecutionID,
) (<-chan codexruntime.Event, error) {
	f.mu.Lock()
	if f.subscribeErr != nil {
		err := f.subscribeErr
		f.mu.Unlock()
		return nil, err
	}
	source := f.events[id]
	f.mu.Unlock()
	if source == nil {
		return nil, errors.New("missing execution")
	}
	output := make(chan codexruntime.Event)
	go func() {
		defer close(output)
		for {
			select {
			case event, open := <-source:
				if !open {
					return
				}
				select {
				case output <- event:
				case <-ctx.Done():
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()
	return output, nil
}

func (f *fakeRuntime) CancelExecution(
	_ context.Context,
	id codexruntime.ExecutionID,
) (codexruntime.CancellationResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cancelled = append(f.cancelled, id)
	snapshot := f.snapshots[id]
	snapshot.Status = codexruntime.StatusCancelled
	f.snapshots[id] = snapshot
	if events := f.events[id]; events != nil {
		close(events)
		f.events[id] = nil
	}
	return codexruntime.CancellationResult{
		ExecutionID: id,
		Status:      codexruntime.StatusCancelled,
		Method:      codexruntime.CancellationNativeInterrupt,
	}, nil
}

func (f *fakeRuntime) GetExecution(
	_ context.Context,
	id codexruntime.ExecutionID,
) (codexruntime.ExecutionSnapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.getErr != nil {
		return codexruntime.ExecutionSnapshot{}, f.getErr
	}
	snapshot, ok := f.snapshots[id]
	if !ok {
		return codexruntime.ExecutionSnapshot{}, errors.New("missing execution")
	}
	return snapshot, nil
}

func (f *fakeRuntime) Close(context.Context) error { return nil }

func (f *fakeRuntime) Requests() []codexruntime.ExecutionRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]codexruntime.ExecutionRequest(nil), f.requests...)
}

func (f *fakeRuntime) CancelledExecutions() []codexruntime.ExecutionID {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]codexruntime.ExecutionID(nil), f.cancelled...)
}
