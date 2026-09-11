package personidentity

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"magicpodcast/internal/codexruntime"
	"magicpodcast/internal/contentsearch"
	"magicpodcast/internal/models"
)

type speechReviewRuntime struct {
	mu        sync.Mutex
	requests  []codexruntime.ExecutionRequest
	snapshots map[codexruntime.ExecutionID]codexruntime.ExecutionSnapshot
	review    func(speechReviewInput) json.RawMessage
}

func (r *speechReviewRuntime) CreateExecution(_ context.Context, request codexruntime.ExecutionRequest) (codexruntime.ExecutionSnapshot, error) {
	raw := json.RawMessage(`{"people":[{"name":"林言","kind":"participant","status":"confirmed","role":"host","presence_basis":"self_introduction","name_evidence":{"source":"show_notes","fragment":0,"quote":"主播林言"},"presence_evidence":{"source":"transcript","fragment":1,"quote":"我是林言。"},"role_evidence":{"source":"show_notes","fragment":0,"quote":"主播林言"},"speech_bindings":[{"speaker_label":"A","basis":"self_introduction","evidence":{"source":"transcript","fragment":1,"quote":"我是林言。"},"scope":"stable_speaker","orders":[],"excluded_orders":[]}]}]}`)
	if strings.Contains(string(request.OutputSchema), `"reviews"`) {
		start := strings.LastIndex(request.Prompt, "<source_data>")
		end := strings.LastIndex(request.Prompt, "</source_data>")
		if start < 0 || end < start {
			return codexruntime.ExecutionSnapshot{}, fmt.Errorf("missing review input")
		}
		var input speechReviewInput
		if err := json.Unmarshal([]byte(request.Prompt[start+len("<source_data>"):end]), &input); err != nil {
			return codexruntime.ExecutionSnapshot{}, err
		}
		raw = r.review(input)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.requests = append(r.requests, request)
	id := codexruntime.ExecutionID(fmt.Sprintf("review-%d", len(r.requests)))
	snapshot := codexruntime.ExecutionSnapshot{ID: id, Status: codexruntime.StatusCompleted, Result: raw}
	if r.snapshots == nil {
		r.snapshots = map[codexruntime.ExecutionID]codexruntime.ExecutionSnapshot{}
	}
	r.snapshots[id] = snapshot
	return snapshot, nil
}
func (r *speechReviewRuntime) SubscribeExecution(context.Context, codexruntime.ExecutionID) (<-chan codexruntime.Event, error) {
	ch := make(chan codexruntime.Event)
	close(ch)
	return ch, nil
}
func (r *speechReviewRuntime) GetExecution(_ context.Context, id codexruntime.ExecutionID) (codexruntime.ExecutionSnapshot, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.snapshots[id], nil
}
func (r *speechReviewRuntime) CancelExecution(_ context.Context, id codexruntime.ExecutionID) (codexruntime.CancellationResult, error) {
	return codexruntime.CancellationResult{ExecutionID: id, Status: codexruntime.StatusCancelled}, nil
}
func (r *speechReviewRuntime) Close(context.Context) error { return nil }

func TestRuntimeSpeechReviewPublishesOnlyReviewedSingleSpeakerFragments(t *testing.T) {
	for _, omit := range []bool{false, true} {
		t.Run(fmt.Sprintf("missing=%t", omit), func(t *testing.T) {
			db := openPersonIdentityDB(t)
			pod := models.Podcast{XYZID: "review", Title: "访谈", FeedURL: "https://example.test/review"}
			require.NoError(t, db.Create(&pod).Error)
			ep := models.Episode{PodcastID: pod.ID, GUID: "review", Title: "对话", ShowNotes: "本集主播林言", Notes: "private-never-in-review"}
			require.NoError(t, db.Create(&ep).Error)
			runtime := &speechReviewRuntime{review: func(input speechReviewInput) json.RawMessage {
				if omit {
					return json.RawMessage(`{"reviews":[{"order":1,"verdict":"single_speaker"}]}`)
				}
				return json.RawMessage(`{"reviews":[{"order":1,"verdict":"single_speaker"},{"order":2,"verdict":"mixed"},{"order":3,"verdict":"single_speaker"}]}`)
			}}
			search, err := contentsearch.NewService(db)
			require.NoError(t, err)
			service, err := NewService(db, NewRuntimeSuggester(runtime, t.TempDir()), search)
			require.NoError(t, err)
			got, err := service.Prepare(context.Background(), EpisodeSources{EpisodeID: ep.ID, SourceVersion: "review-fixture", Segments: []Segment{{Order: 1, SpeakerLabel: "A", Text: "我是林言。"}, {Order: 2, SpeakerLabel: "A", Text: "准备公开吗？同意，你接着说。我继续整理。"}, {Order: 3, SpeakerLabel: "A", Text: "我还会继续研究。"}}})
			if omit {
				require.Error(t, err)
				var count int64
				require.NoError(t, db.Model(&models.EpisodeAppearance{}).Count(&count).Error)
				require.Zero(t, count)
				require.NoError(t, db.Model(&models.ContentSearchFragment{}).Count(&count).Error)
				require.Zero(t, count)
			} else {
				require.NoError(t, err)
				require.Len(t, got.People, 1)
				require.Equal(t, 2, got.People[0].ConfirmedSpeech)
				require.Nil(t, got.Attributions[1].PersonID)
				require.Equal(t, StatusPending, got.Attributions[1].Status)
				result, err := search.Search(context.Background(), contentsearch.Request{Query: "公开", Scope: contentsearch.Scope{EpisodeIDs: []uint{ep.ID}}})
				require.NoError(t, err)
				require.NotEmpty(t, result.Hits)
				require.Nil(t, result.Hits[0].PersonID)
			}
			require.Len(t, runtime.requests, 2)
			for _, request := range runtime.requests {
				require.NotContains(t, request.Prompt, ep.Notes)
				require.NotNil(t, request.ToolRestriction)
				require.Empty(t, request.ToolRestriction.Allowed)
			}
		})
	}
}

func TestSpeechReviewRejectsPartialDuplicateOrForeignCoverage(t *testing.T) {
	for _, raw := range []string{`{"reviews":[]}`, `{"reviews":[{"order":1,"verdict":"single_speaker"},{"order":1,"verdict":"mixed"}]}`, `{"reviews":[{"order":2,"verdict":"single_speaker"}]}`, `{"reviews":[{"order":1,"verdict":"guessed"}]}`} {
		_, err := decodeSpeechReviews(json.RawMessage(raw), map[int]bool{1: true})
		require.Error(t, err)
	}
}

func TestLargeSpeechReviewUsesAtMostThreeGroupsWithoutDroppingAssignments(t *testing.T) {
	runtime := &speechReviewRuntime{review: func(input speechReviewInput) json.RawMessage {
		rows := []map[string]any{}
		for _, fragment := range input.Fragments {
			if fragment.Review {
				rows = append(rows, map[string]any{"order": fragment.Order, "verdict": "single_speaker"})
			}
		}
		raw, _ := json.Marshal(map[string]any{"reviews": rows})
		return raw
	}}
	segments := make([]Segment, 251)
	for i := range segments {
		segments[i] = Segment{Order: i + 1, SpeakerLabel: "A", Text: "我是林言。"}
	}
	got, err := NewRuntimeSuggester(runtime, t.TempDir()).Suggest(context.Background(), EpisodeSources{ShowNotes: "主播林言", Segments: segments})
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Len(t, got[0].SpeechOrders, 251)
	require.Len(t, runtime.requests, 4)
}

type blockingSpeechReviewRuntime struct {
	*speechReviewRuntime
	started   chan struct{}
	cancelled chan codexruntime.ExecutionID
}

func (r *blockingSpeechReviewRuntime) SubscribeExecution(ctx context.Context, id codexruntime.ExecutionID) (<-chan codexruntime.Event, error) {
	if id == "review-1" {
		return r.speechReviewRuntime.SubscribeExecution(ctx, id)
	}
	close(r.started)
	events := make(chan codexruntime.Event)
	go func() { <-ctx.Done(); close(events) }()
	return events, nil
}
func (r *blockingSpeechReviewRuntime) CancelExecution(_ context.Context, id codexruntime.ExecutionID) (codexruntime.CancellationResult, error) {
	r.cancelled <- id
	return codexruntime.CancellationResult{ExecutionID: id, Status: codexruntime.StatusCancelled}, nil
}
func TestCancellationDuringSpeechReviewCancelsRuntimeAndDoesNotPublish(t *testing.T) {
	db := openPersonIdentityDB(t)
	pod := models.Podcast{XYZID: "review-cancel", Title: "访谈", FeedURL: "https://example.test/review-cancel"}
	require.NoError(t, db.Create(&pod).Error)
	ep := models.Episode{PodcastID: pod.ID, GUID: "review-cancel", Title: "对话", ShowNotes: "主播林言"}
	require.NoError(t, db.Create(&ep).Error)
	runtime := &blockingSpeechReviewRuntime{speechReviewRuntime: &speechReviewRuntime{review: func(speechReviewInput) json.RawMessage { return json.RawMessage(`{"reviews":[]}`) }}, started: make(chan struct{}), cancelled: make(chan codexruntime.ExecutionID, 1)}
	service, err := NewService(db, NewRuntimeSuggester(runtime, t.TempDir()))
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	finished := make(chan error, 1)
	go func() {
		_, err := service.Prepare(ctx, EpisodeSources{EpisodeID: ep.ID, SourceVersion: "cancel-review", Segments: []Segment{{Order: 1, SpeakerLabel: "A", Text: "我是林言。"}}})
		finished <- err
	}()
	select {
	case <-runtime.started:
	case <-time.After(3 * time.Second):
		t.Fatal("review did not start")
	}
	cancel()
	select {
	case err := <-finished:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(3 * time.Second):
		t.Fatal("cancel did not return")
	}
	require.Equal(t, codexruntime.ExecutionID("review-2"), <-runtime.cancelled)
	var count int64
	require.NoError(t, db.Model(&models.EpisodeAppearance{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestReplyBoundaryOnlyDefersUnquotedEmbeddedResponses(t *testing.T) {
	for _, text := range []string{
		"我们讨论如何发布。不能改稿。对，改稿会影响理解。",
		"需要和社区保持联系。是的。但是我还会继续推进。",
		"这是一段完整的观点。对对对对。",
		"不能叫组织。好的，所以组织会发生变化。",
	} {
		require.True(t, hasUnresolvedReplyBoundary(text), text)
	}
	for _, text := range []string{
		"对。我们继续聊下一件事情。",
		"这是我引用的对话：“不能改稿。对，改稿会影响理解。”然后我继续叙述。",
		"朋友问我，我说。不能改稿。对，改稿会影响理解。",
		"我很认同，所以我们继续推进。",
	} {
		require.False(t, hasUnresolvedReplyBoundary(text), text)
	}
}

func TestReplyBoundaryDefersEvenAPermissiveRuntimeReview(t *testing.T) {
	runtime := &speechReviewRuntime{review: func(input speechReviewInput) json.RawMessage {
		rows := []map[string]any{}
		for _, fragment := range input.Fragments {
			if fragment.Review {
				rows = append(rows, map[string]any{"order": fragment.Order, "verdict": "single_speaker"})
			}
		}
		raw, _ := json.Marshal(map[string]any{"reviews": rows})
		return raw
	}}
	got, err := NewRuntimeSuggester(runtime, t.TempDir()).Suggest(context.Background(), EpisodeSources{ShowNotes: "主播林言", Segments: []Segment{
		{Order: 1, SpeakerLabel: "A", Text: "我是林言。"},
		{Order: 2, SpeakerLabel: "A", Text: "我们讨论怎样发布。不能改稿。对，改稿会影响理解。"},
		{Order: 3, SpeakerLabel: "A", Text: "我会继续研究这个问题。"},
	}})
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, []int{1, 3}, got[0].SpeechOrders)
}
