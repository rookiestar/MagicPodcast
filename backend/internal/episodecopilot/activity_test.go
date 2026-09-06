package episodecopilot

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"magicpodcast/internal/codexruntime"

	"github.com/stretchr/testify/require"
)

type observedEvent struct {
	typeName string
	stage    string
	state    string
	id       string
	category string
	text     string
	ordinal  uint64
}

func observe(stream <-chan StreamEvent) (
	[]observedEvent,
	*StreamEvent,
	*StreamEvent,
	strings.Builder,
) {
	var history []observedEvent
	var completed *StreamEvent
	var failure *StreamEvent
	var answer strings.Builder
	for event := range stream {
		switch event.Type {
		case EventTypeContext:
			history = append(history, observedEvent{
				typeName: string(event.Type),
				stage:    event.Stage,
				text:     event.Message,
			})
		case EventTypeStatus:
			entry := observedEvent{
				typeName: string(event.Type),
				stage:    event.Stage,
				text:     event.Message,
			}
			if event.Activity != nil {
				entry.state = event.Activity.State
				entry.id = event.Activity.ID
				entry.category = event.Activity.Category
				entry.text = event.Activity.Text
				entry.ordinal = event.Activity.Ordinal
			}
			history = append(history, entry)
		case EventTypeAnswerDelta:
			answer.WriteString(event.Message)
		case EventTypeError:
			copied := event
			failure = &copied
		case EventTypeComplete:
			copied := event
			completed = &copied
		}
	}
	return history, completed, failure, answer
}

func TestServiceStreamsDeterministicStagesAndForwardedActivities(
	t *testing.T,
) {
	loader := &fakeContextLoader{
		context: EpisodeContext{
			EpisodeID:    51,
			EpisodeTitle: "Activity flow",
			PodcastTitle: "System Notes",
			ShowNotes:    "The runtime explains its work while answering.",
		},
	}
	runtime := newFakeRuntime(
		fakeExecution{
			progress: []codexruntime.Progress{
				{
					ActivityID:  "a1",
					Ordinal:     1,
					Category:    codexruntime.CategoryWebSearch,
					State:       codexruntime.ProgressStarted,
					DisplayText: "runtime contract 主题",
				},
				{
					ActivityID:  "a1",
					Ordinal:     1,
					Category:    codexruntime.CategoryWebSearch,
					State:       codexruntime.ProgressCompleted,
					DisplayText: "runtime contract 主题",
					Metadata: map[string]string{
						"candidate_domains": "authority.example.com",
						"candidate_count":   "2",
					},
				},
			},
			result: json.RawMessage(
				`{"resources":[{"title":"Guide","url":"https://developers.openai.com/codex/sdk/","relevance":"official"}],"conflicts":[],"limitations":[]}`,
			),
		},
		fakeExecution{
			progress: []codexruntime.Progress{
				{
					ActivityID:  "a2",
					Ordinal:     2,
					Category:    codexruntime.CategoryReasoning,
					State:       codexruntime.ProgressStarted,
					DisplayText: "先核对来源，再回答。",
				},
			},
			deltas: []string{"依据现有内容 [Show Notes L1-L1]。"},
			result: json.RawMessage(`{"text":"依据现有内容。"}`),
		},
	)
	service, err := NewService(loader, runtime, t.TempDir())
	require.NoError(t, err)

	events, err := service.Ask(context.Background(), QuestionRequest{
		EpisodeID: 51,
		Question:  "这次执行发生了什么？",
	})
	require.NoError(t, err)

	history, completed, failure, answer := observe(events)
	require.Nil(t, failure)
	require.NotNil(t, completed)
	require.Contains(t, answer.String(), "依据现有内容")

	require.NotEmpty(t, history)
	require.Equal(t, EventTypeContext, EventType(history[0].typeName))
	require.Equal(t, StageReadContext, history[0].stage)

	type expectation struct {
		stage    string
		id       string
		state    string
		category string
	}
	expected := []expectation{
		{StageResearchRuntime, "stage:" + StageResearchRuntime, "started", "stage"},
		{StageResearchRuntime, "stage:" + StageResearchRuntime, "completed", "stage"},
		{StagePublicResearch, "research:a1", "started", "web_search"},
		{StagePublicResearch, "research:a1", "completed", "web_search"},
		{StageSourceValidation, "stage:" + StageSourceValidation, "started", "stage"},
		{StageSourceValidation, "stage:" + StageSourceValidation, "completed", "stage"},
		{StageAnswerRuntime, "stage:" + StageAnswerRuntime, "started", "stage"},
		{StageAnswerRuntime, "stage:" + StageAnswerRuntime, "completed", "stage"},
		{StageComposeAnswer, "answer:a2", "started", "reasoning"},
		{StageCitationValidation, "stage:" + StageCitationValidation, "started", "stage"},
		{StageCitationValidation, "stage:" + StageCitationValidation, "completed", "stage"},
	}
	statusEntries := make([]observedEvent, 0, len(history))
	for _, entry := range history {
		if entry.typeName == string(EventTypeStatus) {
			statusEntries = append(statusEntries, entry)
		}
	}
	require.Len(t, statusEntries, len(expected))
	for index, want := range expected {
		got := statusEntries[index]
		require.Equal(t, want.stage, got.stage, "entry %d", index)
		require.Equal(t, want.id, got.id, "entry %d", index)
		require.Equal(t, want.state, got.state, "entry %d", index)
		require.Equal(t, want.category, got.category, "entry %d", index)
	}

	// Activities share stable IDs for in-place updates and carry the
	// question-scoped continuous ordinal.
	require.Equal(t, "runtime contract 主题", statusEntries[2].text)
	require.Equal(t, "runtime contract 主题", statusEntries[3].text)
	for index, entry := range statusEntries {
		require.Equal(t, uint64(index+1), entry.ordinal, "entry %d", index)
	}

	// The verified-source count comes from server validation only.
	require.Contains(t, statusEntries[5].text, "已验证 1 个公开来源")

	timings := completed.StageTimings
	require.NotNil(t, timings)
	require.GreaterOrEqual(t, timings.ResearchRuntimeReadyMS, int64(0))
	require.GreaterOrEqual(t, timings.PublicResearchMS, int64(0))
	require.GreaterOrEqual(t, timings.SourceValidationMS, int64(0))
	require.GreaterOrEqual(t, timings.AnswerRuntimeReadyMS, int64(0))
	require.GreaterOrEqual(t, timings.CitationValidationMS, int64(0))
	require.GreaterOrEqual(t, completed.TotalMS, completed.FirstContentMS)
}

func TestServiceKeepsCandidatesDistinctFromVerifiedSources(t *testing.T) {
	loader := &fakeContextLoader{
		context: EpisodeContext{
			EpisodeID:    52,
			EpisodeTitle: "Candidate sources",
			PodcastTitle: "System Notes",
			ShowNotes:    "Search hits stay candidates until validated.",
		},
	}
	runtime := newFakeRuntime(
		fakeExecution{
			progress: []codexruntime.Progress{
				{
					ActivityID: "a1",
					Ordinal:    1,
					Category:   codexruntime.CategoryWebSearch,
					State:      codexruntime.ProgressCompleted,
					Metadata: map[string]string{
						"candidate_domains": "candidate.example.com",
						"candidate_count":   "3",
					},
				},
			},
			result: json.RawMessage(
				`{"resources":[
					{"title":"Valid","url":"https://developers.openai.com/codex/sdk/","relevance":"official"},
					{"title":"Loopback","url":"http://127.0.0.1:8080/private","relevance":"rejected"}
				],"conflicts":[],"limitations":[]}`,
			),
		},
		fakeExecution{
			deltas: []string{"结论 [Show Notes L1-L1]。"},
			result: json.RawMessage(`{"text":"结论。"}`),
		},
	)
	service, err := NewService(loader, runtime, t.TempDir())
	require.NoError(t, err)

	events, err := service.Ask(context.Background(), QuestionRequest{
		EpisodeID: 52,
		Question:  "候选来源算数吗？",
	})
	require.NoError(t, err)

	history, completed, failure, _ := observe(events)
	require.Nil(t, failure)
	require.NotNil(t, completed)

	var validationTexts []string
	for _, entry := range history {
		if entry.stage == StageSourceValidation && entry.state != "" {
			validationTexts = append(validationTexts, entry.text)
		}
	}
	require.Len(t, validationTexts, 2)
	require.Equal(t, "正在校验公开来源…", validationTexts[0])
	require.Contains(t, validationTexts[1], "已验证 1 个公开来源")
	require.NotContains(t, validationTexts[1], "3")
	require.NotContains(t, validationTexts[1], "2 个")
}

func TestServiceAnnouncesDegradationBeforeAnswerRuntime(t *testing.T) {
	loader := &fakeContextLoader{
		context: EpisodeContext{
			EpisodeID:    53,
			EpisodeTitle: "Degraded research",
			PodcastTitle: "System Notes",
			ShowNotes:    "Research may fail without blocking the answer.",
		},
	}
	runtime := newFakeRuntime(
		fakeExecution{
			createErr: &codexruntime.RuntimeError{
				Code:        codexruntime.ErrorRuntimeUnavailable,
				SafeMessage: "runtime unavailable",
				Retryable:   true,
			},
		},
		fakeExecution{
			deltas: []string{"仅依据单集内容回答 [Show Notes L1-L1]。"},
			result: json.RawMessage(`{"text":"仅依据单集内容回答。"}`),
		},
	)
	service, err := NewService(loader, runtime, t.TempDir())
	require.NoError(t, err)

	events, err := service.Ask(context.Background(), QuestionRequest{
		EpisodeID: 53,
		Question:  "检索失败会怎样？",
	})
	require.NoError(t, err)

	history, completed, failure, answer := observe(events)
	require.Nil(t, failure)
	require.NotNil(t, completed)
	require.Contains(t, answer.String(), "仅依据单集内容回答")

	var degradationIndex, answerStartIndex int = -1, -1
	for index, entry := range history {
		if entry.id == "stage:"+StageSourceValidation &&
			entry.state == "failed" {
			degradationIndex = index
		}
		if entry.id == "stage:"+StageAnswerRuntime &&
			entry.state == "started" && answerStartIndex < 0 {
			answerStartIndex = index
		}
	}
	require.GreaterOrEqual(t, degradationIndex, 0)
	require.Greater(t, answerStartIndex, degradationIndex)
	require.Contains(
		t,
		history[degradationIndex].text,
		"公开资料检索失败，将仅依据单集内部内容回答",
	)
	require.NotNil(t, completed.StageTimings)
}

func TestServiceAbortsOnProfileUnavailableWithoutAnswerStage(t *testing.T) {
	loader := &fakeContextLoader{
		context: EpisodeContext{
			EpisodeID:    54,
			EpisodeTitle: "Profile unavailable",
			PodcastTitle: "System Notes",
			ShowNotes:    "An unsupported tier must terminate the question.",
		},
	}
	runtime := newFakeRuntime(
		fakeExecution{
			result:    json.RawMessage(`{"resources":[],"conflicts":[],"limitations":[]}`),
			status:    codexruntime.StatusFailed,
			errorCode: codexruntime.ErrorProfileUnavailable,
		},
	)
	service, err := NewService(loader, runtime, t.TempDir())
	require.NoError(t, err)

	events, err := service.Ask(context.Background(), QuestionRequest{
		EpisodeID: 54,
		Question:  "档位不可用怎么办？",
	})
	require.NoError(t, err)

	history, completed, failure, _ := observe(events)
	require.NotNil(t, failure)
	require.Equal(t, "profile_unavailable", failure.Code)
	require.Nil(t, completed)
	for _, entry := range history {
		require.NotEqual(t, StageAnswerRuntime, entry.stage)
		require.NotEqual(t, StageComposeAnswer, entry.stage)
		require.NotEqual(t, StageCitationValidation, entry.stage)
	}
}

func TestServiceCancellationKeepsActivitiesAndTargetsOneExecution(
	t *testing.T,
) {
	loader := &fakeContextLoader{
		context: EpisodeContext{
			EpisodeID:    55,
			EpisodeTitle: "Cancellation",
			PodcastTitle: "System Notes",
			ShowNotes:    "Cancellation targets only the active execution.",
		},
	}
	runtime := newFakeRuntime(
		fakeExecution{
			block: true,
			progress: []codexruntime.Progress{
				{
					ActivityID:  "a1",
					Ordinal:     1,
					Category:    codexruntime.CategoryWebSearch,
					State:       codexruntime.ProgressStarted,
					DisplayText: "取消前的活动",
				},
			},
		},
	)
	service, err := NewService(loader, runtime, t.TempDir())
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())

	events, err := service.Ask(ctx, QuestionRequest{
		EpisodeID: 55,
		Question:  "取消时保留什么？",
	})
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		return len(runtime.Requests()) == 1
	}, time.Second, 10*time.Millisecond)
	cancel()

	var sawForwardedActivity bool
	for event := range events {
		if event.Type == EventTypeStatus && event.Activity != nil &&
			event.Activity.ID == "research:a1" {
			sawForwardedActivity = true
		}
	}
	require.True(t, sawForwardedActivity)
	require.Len(t, runtime.CancelledExecutions(), 1)
}

func TestServiceForwardTruncatesAndBoundsRuntimeActivities(t *testing.T) {
	loader := &fakeContextLoader{
		context: EpisodeContext{
			EpisodeID:    56,
			EpisodeTitle: "Bounds",
			PodcastTitle: "System Notes",
			ShowNotes:    "Forwarded activities stay bounded.",
		},
	}
	oversized := strings.Repeat("长", maxActivityTextRunes+50)
	manyDomains := strings.Repeat("d.example.com ", maxActivityMetadata+50)
	runtime := newFakeRuntime(
		fakeExecution{
			progress: []codexruntime.Progress{
				{
					ActivityID:  "a1",
					Ordinal:     1,
					Category:    codexruntime.CategoryWebSearch,
					State:       codexruntime.ProgressCompleted,
					DisplayText: oversized,
					Metadata: map[string]string{
						"candidate_domains": manyDomains,
						"extra":             "second entry still forwarded",
					},
				},
			},
			result: json.RawMessage(`{"resources":[],"conflicts":[],"limitations":[]}`),
		},
		fakeExecution{
			deltas: []string{"完成 [Show Notes L1-L1]。"},
			result: json.RawMessage(`{"text":"完成。"}`),
		},
	)
	service, err := NewService(loader, runtime, t.TempDir())
	require.NoError(t, err)

	events, err := service.Ask(context.Background(), QuestionRequest{
		EpisodeID: 56,
		Question:  "活动有边界吗？",
	})
	require.NoError(t, err)

	var forwarded *Activity
	for event := range events {
		if event.Type == EventTypeStatus && event.Activity != nil &&
			event.Activity.ID == "research:a1" {
			forwarded = event.Activity
		}
	}
	require.NotNil(t, forwarded)
	require.LessOrEqual(t, len([]rune(forwarded.Text)), maxActivityTextRunes)
	require.LessOrEqual(t, len(forwarded.Metadata), maxActivityMetadata)
	for key, value := range forwarded.Metadata {
		require.LessOrEqual(t, len([]rune(key)), 64)
		require.LessOrEqual(
			t,
			len([]rune(value)),
			maxActivityMetadataRunes,
		)
	}
}

func TestServiceStageTimingReflectsDegradedResearch(t *testing.T) {
	loader := &fakeContextLoader{
		context: EpisodeContext{
			EpisodeID:    57,
			EpisodeTitle: "Timing",
			PodcastTitle: "System Notes",
			ShowNotes:    "Stage timing reflects what actually ran.",
		},
	}
	runtime := newFakeRuntime(
		fakeExecution{
			createErr: errors.New("runtime unavailable"),
		},
		fakeExecution{
			deltas: []string{"答案 [Show Notes L1-L1]。"},
			result: json.RawMessage(`{"text":"答案。"}`),
		},
	)
	service, err := NewService(loader, runtime, t.TempDir())
	require.NoError(t, err)

	events, err := service.Ask(context.Background(), QuestionRequest{
		EpisodeID: 57,
		Question:  "降级时计时如何？",
	})
	require.NoError(t, err)

	_, completed, failure, _ := observe(events)
	require.Nil(t, failure)
	require.NotNil(t, completed)
	timings := completed.StageTimings
	require.NotNil(t, timings)
	// The research runtime never became ready, so its stages report no
	// fabricated durations.
	require.Equal(t, int64(0), timings.ResearchRuntimeReadyMS)
	require.Equal(t, int64(0), timings.PublicResearchMS)
}
