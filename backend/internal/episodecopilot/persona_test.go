package episodecopilot

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"magicpodcast/internal/codexruntime"
	"magicpodcast/internal/contentsearch"
	"magicpodcast/internal/personidentity"

	"github.com/stretchr/testify/require"
)

func TestServicePersonaAskUsesLibraryFirstAndKeepsNoAtPath(t *testing.T) {
	people := &fakePeopleModule{
		listed: personidentity.EpisodePeople{
			EpisodeID:     81,
			SourceVersion: "v1",
			IndexReady:    true,
			People: []personidentity.PersonView{{
				ID: 9, DisplayName: "张三", Role: personidentity.RoleHost,
				Status: personidentity.StatusConfirmed, IdentityNote: "技术漫谈主播",
			}},
		},
		ids: []uint{81, 82},
	}
	search := &fakeSearchModule{
		result: contentsearch.Result{
			Hits: []contentsearch.Hit{{
				EpisodeID: 81, SourceKind: "transcript", SourceVersion: "v1",
				FragmentOrder: 3, Text: "我不赞成无限制加班，长期靠加班堆产出会把团队拖垮。",
				AttributionStatus: "confirmed", PersonID: uintPtr(9),
				PublishedAt: "2025-03-12",
			}},
			Coverage: contentsearch.Coverage{Complete: true, Reason: contentsearch.CoverageComplete},
		},
	}
	loader := &fakeContextLoader{
		context: EpisodeContext{
			EpisodeID:    81,
			EpisodeTitle: "加班",
			PodcastTitle: "技术漫谈",
			ShowNotes:    "主播张三",
			Transcript:   "我不赞成无限制加班，长期靠加班堆产出会把团队拖垮。",
			PrivateNotes: "不要发送到网页搜索",
		},
	}
	runtime := newFakeRuntime(
		fakeExecution{
			deltas: []string{"我不赞成无限制加班 [库内 S1]。"},
			result: json.RawMessage(`{"text":"我不赞成无限制加班"}`),
		},
	)
	service, err := NewService(
		loader,
		runtime,
		t.TempDir(),
		WithLibrary(people, search),
	)
	require.NoError(t, err)

	events, err := service.Ask(context.Background(), QuestionRequest{
		EpisodeID:          81,
		Question:           "他对加班怎么看？",
		TargetPersonID:     9,
		IncludePrivateNote: true,
	})
	require.NoError(t, err)
	var answer strings.Builder
	for event := range events {
		if event.Type == EventTypeAnswerDelta {
			answer.WriteString(event.Message)
		}
		require.NotEqual(t, EventTypeError, event.Type)
	}
	require.Len(t, runtime.Requests(), 2)
	require.Empty(t, runtime.Requests()[0].ToolRestriction.Allowed)
	require.NotContains(t, runtime.Requests()[0].Prompt, "不要发送到网页搜索")
	require.Contains(t, answer.String(), personaDisclaimer)
	require.Contains(t, answer.String(), "不赞成无限制加班")
	require.NotNil(t, search.last.Filter.PersonID)
	require.Equal(t, uint(9), *search.last.Filter.PersonID)
	require.Equal(t, []uint{81, 82}, search.last.Scope.EpisodeIDs, "inspect library history even when the current episode has a topic hit")
}

func TestServicePersonaOffTopicNameHitStillSearchesWeb(t *testing.T) {
	people := &fakePeopleModule{
		listed: personidentity.EpisodePeople{
			EpisodeID:     83,
			SourceVersion: "v1",
			IndexReady:    true,
			People: []personidentity.PersonView{{
				ID: 5, DisplayName: "赵六", Aliases: []string{"六哥"},
				Status: personidentity.StatusConfirmed, Role: personidentity.RoleHost,
			}},
		},
		ids: []uint{83},
	}
	search := &fakeSearchModule{
		result: contentsearch.Result{
			Hits: []contentsearch.Hit{{
				EpisodeID: 83, SourceKind: "transcript", SourceVersion: "v1",
				FragmentOrder: 1, Text: "我是赵六。报道伦理的底线是核对原话。",
				AttributionStatus: "confirmed", PersonID: uintPtr(5),
				PublishedAt: "2025-09-01",
			}},
			Coverage: contentsearch.Coverage{Complete: true, Reason: contentsearch.CoverageComplete},
		},
	}
	loader := &fakeContextLoader{
		context: EpisodeContext{
			EpisodeID: 83, EpisodeTitle: "伦理", PodcastTitle: "媒体观察",
			ShowNotes: "主播赵六只谈报道伦理", Transcript: "我是赵六。报道伦理的底线是核对原话。",
			PrivateNotes: "私有备注不得外发",
		},
	}
	runtime := newFakeRuntime(
		fakeExecution{result: json.RawMessage(`{"resources":[],"conflicts":[],"limitations":["没有本人原文"]}`)},
		fakeExecution{deltas: []string{"无法判断赵六对量子计算商业化的看法。[库内 S1]"}},
	)
	service, err := NewService(loader, runtime, t.TempDir(), WithLibrary(people, search))
	require.NoError(t, err)
	events, err := service.Ask(context.Background(), QuestionRequest{
		EpisodeID: 83, Question: "赵六对量子计算的商业化怎么看？", TargetPersonID: 5,
	})
	require.NoError(t, err)
	for range events {
	}
	require.Len(t, runtime.Requests(), 2)
	require.Equal(t, []codexruntime.ToolCapability{codexruntime.ToolWebSearch}, runtime.Requests()[0].ToolRestriction.Allowed)
	require.NotContains(t, runtime.Requests()[0].Prompt, "私有备注不得外发")
}

func TestServiceRejectsPendingAndForeignTargetsAndKeepsNoAtBehavior(t *testing.T) {
	people := &fakePeopleModule{
		listed: personidentity.EpisodePeople{
			EpisodeID:  82,
			IndexReady: true,
			People: []personidentity.PersonView{{
				ID: 4, DisplayName: "匿名工程师", Status: personidentity.StatusPending,
			}},
		},
	}
	loader := &fakeContextLoader{
		context: EpisodeContext{
			EpisodeID: 82, EpisodeTitle: "匿名", PodcastTitle: "技术漫谈",
			ShowNotes: "嘉宾匿名", Transcript: "Speaker 1 说话。",
		},
	}
	runtime := newFakeRuntime()
	service, err := NewService(loader, runtime, t.TempDir(), WithLibrary(people, &fakeSearchModule{}))
	require.NoError(t, err)
	_, err = service.Ask(context.Background(), QuestionRequest{
		EpisodeID: 82, Question: "降本方案？", TargetPersonID: 4,
	})
	require.ErrorIs(t, err, ErrPersonPending)

	_, err = service.Ask(context.Background(), QuestionRequest{
		EpisodeID: 82, Question: "降本方案？", TargetPersonID: 99,
	})
	require.ErrorIs(t, err, ErrTargetPersonInvalid)

	runtime = newFakeRuntime(
		fakeExecution{result: json.RawMessage(`{"resources":[],"conflicts":[],"limitations":[]}`)},
		fakeExecution{deltas: []string{"普通问答 [Show Notes L1-L1]。"}},
	)
	service, err = NewService(loader, runtime, t.TempDir(), WithLibrary(people, &fakeSearchModule{}))
	require.NoError(t, err)
	events, err := service.Ask(context.Background(), QuestionRequest{
		EpisodeID: 82, Question: "这期在讲什么？",
	})
	require.NoError(t, err)
	for range events {
	}
	require.Len(t, runtime.Requests(), 2)
}

func TestServicePersonaGapCallsBoundedWebSearchWithoutPrivateNotes(t *testing.T) {
	people := &fakePeopleModule{
		listed: personidentity.EpisodePeople{
			EpisodeID: 83, IndexReady: true, SourceVersion: "v1",
			People: []personidentity.PersonView{{
				ID: 5, DisplayName: "赵六", Status: personidentity.StatusConfirmed,
				Role: personidentity.RoleHost, IdentityNote: "媒体观察主播",
			}},
		},
		ids: []uint{83},
	}
	search := &fakeSearchModule{
		result: contentsearch.Result{
			Coverage: contentsearch.Coverage{Complete: true, Reason: contentsearch.CoverageMiss},
		},
	}
	loader := &fakeContextLoader{
		context: EpisodeContext{
			EpisodeID: 83, EpisodeTitle: "伦理", PodcastTitle: "媒体观察",
			ShowNotes: "主播赵六只谈报道伦理", Transcript: "报道伦理的底线是核对原话。",
			PrivateNotes: "私有备注不得外发",
		},
	}
	runtime := newFakeRuntime(
		fakeExecution{result: json.RawMessage(`{"resources":[],"conflicts":[],"limitations":["没有本人原文"]}`)},
		fakeExecution{deltas: []string{"无法判断 [逐字稿 L1-L1]。"}},
	)
	service, err := NewService(loader, runtime, t.TempDir(), WithClock(func() time.Time {
		return time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	}), WithLibrary(people, search))
	require.NoError(t, err)
	events, err := service.Ask(context.Background(), QuestionRequest{
		EpisodeID: 83, Question: "赵六对量子计算怎么看？", TargetPersonID: 5,
		IncludePrivateNote: true,
	})
	require.NoError(t, err)
	var answer strings.Builder
	for event := range events {
		if event.Type == EventTypeAnswerDelta {
			answer.WriteString(event.Message)
		}
	}
	require.Len(t, runtime.Requests(), 2)
	require.Equal(t, []codexruntime.ToolCapability{codexruntime.ToolWebSearch}, runtime.Requests()[0].ToolRestriction.Allowed)
	require.NotContains(t, runtime.Requests()[0].Prompt, "私有备注不得外发")
	require.Empty(t, runtime.Requests()[1].ToolRestriction.Allowed)
	require.Contains(t, answer.String(), personaDisclaimer)
}

func uintPtr(id uint) *uint { return &id }

type fakePeopleModule struct {
	listed personidentity.EpisodePeople
	ids    []uint
}

func (f *fakePeopleModule) Prepare(context.Context, personidentity.EpisodeSources) (personidentity.EpisodePeople, error) {
	return f.listed, nil
}
func (f *fakePeopleModule) ListEpisodePeople(context.Context, uint) (personidentity.EpisodePeople, error) {
	return f.listed, nil
}
func (f *fakePeopleModule) CorrectName(context.Context, uint, personidentity.NameCorrection) (personidentity.EpisodePeople, error) {
	return f.listed, nil
}
func (f *fakePeopleModule) CorrectAttribution(context.Context, uint, personidentity.AttributionCorrection) (personidentity.EpisodePeople, error) {
	return f.listed, nil
}
func (f *fakePeopleModule) ReliableSpeech(context.Context, uint, uint) ([]personidentity.AttributionFact, error) {
	return nil, nil
}
func (f *fakePeopleModule) CurrentFacts(context.Context, uint) ([]personidentity.AttributionFact, error) {
	return nil, nil
}
func (f *fakePeopleModule) AccessibleEpisodeIDs(context.Context) ([]uint, error) {
	if len(f.ids) == 0 {
		return []uint{f.listed.EpisodeID}, nil
	}
	return f.ids, nil
}

type fakeSearchModule struct {
	result contentsearch.Result
	err    error
	last   contentsearch.Request
}

func (f *fakeSearchModule) Search(_ context.Context, request contentsearch.Request) (contentsearch.Result, error) {
	f.last = request
	if f.err != nil {
		return contentsearch.Result{}, f.err
	}
	return f.result, nil
}
func (f *fakeSearchModule) ReplaceEpisode(context.Context, contentsearch.EpisodeDocument) error {
	return nil
}
func (f *fakeSearchModule) RemoveEpisode(context.Context, uint) error { return nil }

func TestPersonaTopicOverlapDoesNotOverrideCoverageAssessment(t *testing.T) {
	p := personidentity.PersonView{ID: 9, DisplayName: "张三", Status: "confirmed"}
	people := &fakePeopleModule{listed: personidentity.EpisodePeople{EpisodeID: 81, People: []personidentity.PersonView{p}}, ids: []uint{81}}
	search := &fakeSearchModule{result: contentsearch.Result{Hits: []contentsearch.Hit{{EpisodeID: 81, PersonID: uintPtr(9), Text: "反对无限制加班。", AttributionStatus: "confirmed", SourceKind: "transcript", SourceVersion: "v1", FragmentOrder: 1}}, Coverage: contentsearch.Coverage{Complete: true}}}
	rt := newFakeRuntime(fakeExecution{result: json.RawMessage(`{"resources":[],"conflicts":[],"limitations":[]}`)}, fakeExecution{deltas: []string{"我反对无限制加班 [库内 S1]；期权方面无法判断。"}})
	sufficient := false
	rt.coverageAnswer = &sufficient
	service, err := NewService(&fakeContextLoader{context: EpisodeContext{EpisodeID: 81, Transcript: "反对无限制加班。"}}, rt, t.TempDir(), WithLibrary(people, search))
	require.NoError(t, err)
	stream, err := service.Ask(context.Background(), QuestionRequest{EpisodeID: 81, TargetPersonID: 9, Question: "你对加班以及员工期权回购怎么看？"})
	require.NoError(t, err)
	for event := range stream {
		require.NotEqual(t, EventTypeError, event.Type, event.Message)
	}
	requests := rt.Requests()
	require.Len(t, requests, 3)
	require.Contains(t, string(requests[0].OutputSchema), "sufficient")
	require.Equal(t, []codexruntime.ToolCapability{codexruntime.ToolWebSearch}, requests[1].ToolRestriction.Allowed)
	require.Contains(t, string(requests[1].OutputSchema), "original_read")
}
