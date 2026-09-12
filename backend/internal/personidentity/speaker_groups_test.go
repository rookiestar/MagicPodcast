package personidentity

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"magicpodcast/internal/codexruntime"
	"magicpodcast/internal/models"
)

type speakerGroupRuntime struct {
	mu        sync.Mutex
	requests  []codexruntime.ExecutionRequest
	snapshots map[codexruntime.ExecutionID]codexruntime.ExecutionSnapshot
}

func (r *speakerGroupRuntime) CreateExecution(_ context.Context, request codexruntime.ExecutionRequest) (codexruntime.ExecutionSnapshot, error) {
	raw := json.RawMessage(`{"people": [{"name": "林言", "kind": "participant", "status": "confirmed", "role": "host", "presence_basis": "self_introduction", "name_evidence": {"source": "show_notes", "fragment": 0, "quote": "主播林言"}, "presence_evidence": {"source": "transcript", "fragment": 1, "quote": "我是林言。"}, "role_evidence": {"source": "show_notes", "fragment": 0, "quote": "主播林言"}, "speech_bindings": [{"speaker_label": "A", "basis": "self_introduction", "evidence": {"source": "transcript", "fragment": 1, "quote": "我是林言。"}, "scope": "stable_speaker", "orders": [], "excluded_orders": []}]}], "speakers": [{"speaker_label": "A", "reason": "明确自我介绍", "candidates": [{"person_index": 0, "level": "direct", "basis": "self_introduction", "reason": "明确自我介绍", "evidence": [{"source": "transcript", "fragment": 1, "quote": "我是林言。"}], "counter_evidence": []}]}, {"speaker_label": "B", "reason": "没有姓名依据", "candidates": []}]}`)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.requests = append(r.requests, request)
	id := codexruntime.ExecutionID(fmt.Sprintf("identity-%d", len(r.requests)))
	snapshot := codexruntime.ExecutionSnapshot{ID: id, Status: codexruntime.StatusCompleted, Result: raw}
	if r.snapshots == nil {
		r.snapshots = map[codexruntime.ExecutionID]codexruntime.ExecutionSnapshot{}
	}
	r.snapshots[id] = snapshot
	return snapshot, nil
}
func (r *speakerGroupRuntime) SubscribeExecution(context.Context, codexruntime.ExecutionID) (<-chan codexruntime.Event, error) {
	ch := make(chan codexruntime.Event)
	close(ch)
	return ch, nil
}
func (r *speakerGroupRuntime) GetExecution(_ context.Context, id codexruntime.ExecutionID) (codexruntime.ExecutionSnapshot, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.snapshots[id], nil
}
func (r *speakerGroupRuntime) CancelExecution(_ context.Context, id codexruntime.ExecutionID) (codexruntime.CancellationResult, error) {
	return codexruntime.CancellationResult{ExecutionID: id, Status: codexruntime.StatusCancelled}, nil
}
func (r *speakerGroupRuntime) Close(context.Context) error { return nil }

func TestSpeakerGroupsUseOneIdentityCallAndPublishOnlyAfterConfirmation(t *testing.T) {
	db := openPersonIdentityDB(t)
	pod := models.Podcast{XYZID: "speaker-groups", Title: "访谈", FeedURL: "https://example.test/groups"}
	require.NoError(t, db.Create(&pod).Error)
	ep := models.Episode{PodcastID: pod.ID, GUID: "groups", Title: "对话", ShowNotes: "本集主播林言", Notes: "private-never-in-identity"}
	require.NoError(t, db.Create(&ep).Error)
	runtime := &speakerGroupRuntime{}
	service, err := NewService(db, NewRuntimeSuggester(runtime, t.TempDir()))
	require.NoError(t, err)
	segments := []Segment{{Order: 1, SpeakerLabel: "A", Text: "我是林言。"}, {Order: 2, SpeakerLabel: "A", Text: "我们讨论怎样发布。不能改稿。对，改稿会影响理解。"}, {Order: 3, SpeakerLabel: "B", Text: "这是未确认人物的声音。"}}
	for i := 4; i <= 251; i++ {
		segments = append(segments, Segment{Order: i, SpeakerLabel: "A", Text: "以后再聊。拜拜。拜拜。"})
	}
	source := publishReviewFixture(t, service, EpisodeSources{EpisodeID: ep.ID, SourceVersion: "groups-v1", Segments: segments})
	draft, err := service.Prepare(context.Background(), source)
	require.NoError(t, err)
	require.Empty(t, draft.People)
	require.Empty(t, draft.Attributions)
	require.Len(t, draft.Draft.Matches, 2)
	require.Len(t, draft.Draft.Matches[0].Orders, 250)
	require.Contains(t, draft.Draft.Matches[0].Orders, 2)
	require.NotContains(t, draft.Draft.Matches[0].Orders, 3)
	require.Len(t, runtime.requests, 1)
	require.NotContains(t, runtime.requests[0].Prompt, ep.Notes)
	require.Empty(t, runtime.requests[0].ToolRestriction.Allowed)
	require.NotContains(t, string(runtime.requests[0].OutputSchema), "excluded_orders")
	applied, err := service.Review(context.Background(), ep.ID, draftRequest(draft), true)
	require.NoError(t, err)
	require.Len(t, applied.People, 1)
	require.Equal(t, 250, applied.People[0].ConfirmedSpeech)
	require.Nil(t, applied.Attributions[2].PersonID)

	corrected, err := service.ApplyManual(context.Background(), ep.ID, ManualMatch{Revision: applied.Revision, SourceVersion: source.SourceVersion, FragmentOrder: 2, Scope: "fragment", Clear: true})
	require.NoError(t, err)
	require.Equal(t, 249, corrected.People[0].ConfirmedSpeech)
	fresh, err := service.Prepare(context.Background(), source)
	require.NoError(t, err)
	require.Len(t, fresh.Draft.Matches[0].Orders, 250)
	require.Equal(t, 249, fresh.People[0].ConfirmedSpeech, "new full-group proposal must not overwrite a manual exception before confirmation")
}

type blockingSpeakerGroupRuntime struct {
	*speakerGroupRuntime
	started   chan struct{}
	cancelled chan codexruntime.ExecutionID
}

func (r *blockingSpeakerGroupRuntime) SubscribeExecution(ctx context.Context, id codexruntime.ExecutionID) (<-chan codexruntime.Event, error) {
	close(r.started)
	events := make(chan codexruntime.Event)
	go func() { <-ctx.Done(); close(events) }()
	return events, nil
}
func (r *blockingSpeakerGroupRuntime) CancelExecution(_ context.Context, id codexruntime.ExecutionID) (codexruntime.CancellationResult, error) {
	r.cancelled <- id
	return codexruntime.CancellationResult{ExecutionID: id, Status: codexruntime.StatusCancelled}, nil
}
func TestCancellationDuringIdentityRecognitionCancelsRuntimeAndDoesNotPublish(t *testing.T) {
	db := openPersonIdentityDB(t)
	pod := models.Podcast{XYZID: "review-cancel", Title: "访谈", FeedURL: "https://example.test/review-cancel"}
	require.NoError(t, db.Create(&pod).Error)
	ep := models.Episode{PodcastID: pod.ID, GUID: "review-cancel", Title: "对话", ShowNotes: "主播林言"}
	require.NoError(t, db.Create(&ep).Error)
	runtime := &blockingSpeakerGroupRuntime{speakerGroupRuntime: &speakerGroupRuntime{}, started: make(chan struct{}), cancelled: make(chan codexruntime.ExecutionID, 1)}
	service, err := NewService(db, NewRuntimeSuggester(runtime, t.TempDir()))
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	finished := make(chan error, 1)
	go func() {
		_, err := prepareReviewed(service, ctx, EpisodeSources{EpisodeID: ep.ID, SourceVersion: "cancel-review", Segments: []Segment{{Order: 1, SpeakerLabel: "A", Text: "我是林言。"}}})
		finished <- err
	}()
	select {
	case <-runtime.started:
	case <-time.After(3 * time.Second):
		t.Fatal("identity did not start")
	}
	cancel()
	select {
	case err := <-finished:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(3 * time.Second):
		t.Fatal("cancel did not return")
	}
	require.Equal(t, codexruntime.ExecutionID("identity-1"), <-runtime.cancelled)
	var count int64
	require.NoError(t, db.Model(&models.EpisodeAppearance{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestConflictingSpeakerGroupsRemainUnassigned(t *testing.T) {
	s, src := reviewFixture(t)
	s.suggester = stubSuggester{candidates: []SuggestedCandidate{
		{DisplayName: "林言", Role: RoleHost, Status: StatusConfirmed, EvidenceKind: "verified_runtime", SpeechOrders: []int{1, 3, 4}},
		{DisplayName: "周宁", Role: RoleGuest, Status: StatusConfirmed, EvidenceKind: "verified_runtime", SpeechOrders: []int{1, 3, 4}},
	}}
	draft, err := s.Prepare(context.Background(), src)
	require.NoError(t, err)
	require.Len(t, draft.Draft.Matches, 2)
	for _, match := range draft.Draft.Matches {
		require.Empty(t, match.Orders)
		require.False(t, match.Selected)
		require.True(t, match.Uncertain)
	}
	require.Empty(t, draft.People)
}

func TestSpeakerIdentityCanCoverSeveralLabelsWithoutModelFragmentLists(t *testing.T) {
	sources := EpisodeSources{ShowNotes: "本集主播林言", Segments: []Segment{
		{Order: 3, SpeakerLabel: "B", Text: "我是林言。"},
		{Order: 1, SpeakerLabel: "A", Text: "我是林言。"},
		{Order: 2, SpeakerLabel: "A", Text: "观点仍然成立。是的。"},
		{Order: 4, SpeakerLabel: "B", Text: "后面继续。拜拜。"},
		{Order: 5, SpeakerLabel: "C", Text: "尚未确认人物。"},
	}}
	raw := json.RawMessage(`{"people":[{"name":"林言","kind":"participant","status":"confirmed","role":"host","presence_basis":"self_introduction","name_evidence":{"source":"show_notes","fragment":0,"quote":"主播林言"},"presence_evidence":{"source":"transcript","fragment":1,"quote":"我是林言。"},"role_evidence":{"source":"show_notes","fragment":0,"quote":"主播林言"},"speech_bindings":[{"speaker_label":"A","basis":"self_introduction","evidence":{"source":"transcript","fragment":1,"quote":"我是林言。"}},{"speaker_label":"B","basis":"self_introduction","evidence":{"source":"transcript","fragment":3,"quote":"我是林言。"}}]}]}`)
	candidates, err := decodeIdentitySuggestions(raw, sources)
	require.NoError(t, err)
	require.Len(t, candidates, 1)
	require.Equal(t, []int{1, 2, 3, 4}, candidates[0].SpeechOrders)
}
