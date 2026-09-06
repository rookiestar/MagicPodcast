package episodecopilot

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"magicpodcast/internal/codexruntime"

	"github.com/stretchr/testify/require"
)

func balancedScopeLoader() *fakeContextLoader {
	return &fakeContextLoader{
		context: EpisodeContext{
			EpisodeID:    91,
			EpisodeTitle: "Balanced tier",
			PodcastTitle: "System Notes",
			ShowNotes:    "The host picks one fixed profile per question.",
		},
	}
}

func TestServiceDefaultsLegacyRequestsToBalancedOnBothExecutions(
	t *testing.T,
) {
	loader := balancedScopeLoader()
	runtime := newFakeRuntime(
		fakeExecution{result: json.RawMessage(`{
			"resources":[],
			"conflicts":[],
			"limitations":[]
		}`)},
		fakeExecution{
			deltas: []string{"均衡档回答。"},
			result: json.RawMessage(`{"text":"均衡档回答。"}`),
		},
	)
	service, err := NewService(loader, runtime, t.TempDir())
	require.NoError(t, err)

	events, err := service.Ask(context.Background(), QuestionRequest{
		EpisodeID: 91,
		Question:  "这次用的是哪个档位？",
	})
	require.NoError(t, err)

	var eventProfiles []string
	for event := range events {
		eventProfiles = append(eventProfiles, event.ProfileID)
	}

	requests := runtime.Requests()
	require.Len(t, requests, 2)
	require.Equal(
		t,
		codexruntime.ModelProfileID("balanced"),
		requests[0].ModelProfile,
	)
	require.Equal(
		t,
		codexruntime.ModelProfileID("balanced"),
		requests[1].ModelProfile,
	)
	require.NotEmpty(t, eventProfiles)
	for _, profile := range eventProfiles {
		require.Equal(t, "balanced", profile)
	}
}

func TestServiceAcceptsExplicitBalancedProfile(t *testing.T) {
	loader := balancedScopeLoader()
	runtime := newFakeRuntime(
		fakeExecution{result: json.RawMessage(`{
			"resources":[],
			"conflicts":[],
			"limitations":[]
		}`)},
		fakeExecution{
			deltas: []string{"按单集内容 [Show Notes L1-L1] 回答。"},
			result: json.RawMessage(`{"text":"按单集内容 [Show Notes L1-L1] 回答。"}`),
		},
	)
	service, err := NewService(loader, runtime, t.TempDir())
	require.NoError(t, err)

	events, err := service.Ask(context.Background(), QuestionRequest{
		EpisodeID: 91,
		Question:  "显式携带均衡档。",
		ProfileID: "balanced",
	})
	require.NoError(t, err)
	for range events {
	}

	requests := runtime.Requests()
	require.Len(t, requests, 2)
	for _, request := range requests {
		require.Equal(
			t,
			codexruntime.ModelProfileID("balanced"),
			request.ModelProfile,
		)
	}
}

func TestServiceRejectsUnsupportedProfileBeforeRuntime(t *testing.T) {
	loader := balancedScopeLoader()
	runtime := newFakeRuntime()
	service, err := NewService(loader, runtime, t.TempDir())
	require.NoError(t, err)

	events, err := service.Ask(context.Background(), QuestionRequest{
		EpisodeID: 91,
		Question:  "不要替我换成别的模型。",
		ProfileID: "turbo",
	})
	require.ErrorIs(t, err, ErrUnsupportedProfile)
	require.Nil(t, events)
	require.Empty(t, runtime.Requests())
}

func TestServiceScopeExposesAllThreeTiersAndDefault(t *testing.T) {
	loader := balancedScopeLoader()
	runtime := newFakeRuntime()
	service, err := NewService(loader, runtime, t.TempDir())
	require.NoError(t, err)

	scope, err := service.ContextScope(context.Background(), 91)
	require.NoError(t, err)
	require.Equal(t, "balanced", scope.DefaultProfileID)
	require.Len(t, scope.Profiles, 3)

	byID := make(map[string]ProfileDescriptor, len(scope.Profiles))
	for _, profile := range scope.Profiles {
		byID[profile.ID] = profile
		require.Equal(
			t,
			profile.ID == "balanced",
			profile.Default,
			"only balanced is the default",
		)
	}

	require.Equal(t, "gpt-5.6-sol", byID["quick"].Model)
	require.Equal(t, "medium", byID["quick"].Effort)
	require.Equal(t, "fast", byID["quick"].ServiceTier)
	require.Equal(t, "gpt-5.6-luna", byID["balanced"].Model)
	require.Equal(t, "max", byID["balanced"].Effort)
	require.Equal(t, "fast", byID["balanced"].ServiceTier)
	require.Equal(t, "gpt-5.6-sol", byID["deep"].Model)
	require.Equal(t, "xhigh", byID["deep"].Effort)
	require.Empty(t, byID["deep"].ServiceTier)
}

func TestServiceAppliesQuickAndDeepProfilesToBothExecutions(
	t *testing.T,
) {
	for _, profileID := range []string{"quick", "deep"} {
		loader := balancedScopeLoader()
		runtime := newFakeRuntime(
			fakeExecution{result: json.RawMessage(`{
				"resources":[],
				"conflicts":[],
				"limitations":[]
			}`)},
			fakeExecution{
				deltas: []string{"按单集内容 [Show Notes L1-L1] 回答。"},
				result: json.RawMessage(`{"text":"按单集内容 [Show Notes L1-L1] 回答。"}`),
			},
		)
		service, err := NewService(loader, runtime, t.TempDir())
		require.NoError(t, err)

		events, err := service.Ask(context.Background(), QuestionRequest{
			EpisodeID: 91,
			Question:  "三档映射验证。",
			ProfileID: profileID,
		})
		require.NoError(t, err)
		for range events {
		}

		requests := runtime.Requests()
		require.Len(t, requests, 2, "profile %s", profileID)
		for _, request := range requests {
			require.Equal(
				t,
				codexruntime.ModelProfileID(profileID),
				request.ModelProfile,
			)
		}
	}
}

func TestServiceAbortsWhenProfileUnavailableOnResearchPhase(
	t *testing.T,
) {
	loader := balancedScopeLoader()
	runtime := newFakeRuntime(
		fakeExecution{
			status:    codexruntime.StatusFailed,
			errorCode: codexruntime.ErrorProfileUnavailable,
		},
		fakeExecution{
			deltas: []string{"不应发生的回答。"},
		},
	)
	service, err := NewService(loader, runtime, t.TempDir())
	require.NoError(t, err)

	events, err := service.Ask(context.Background(), QuestionRequest{
		EpisodeID: 91,
		Question:  "不支持时绝不能换模型继续。",
		ProfileID: "deep",
	})
	require.NoError(t, err)

	var sawFailure bool
	for event := range events {
		if event.Type == EventTypeError {
			sawFailure = true
			require.Equal(t, "profile_unavailable", event.Code)
			require.False(t, event.Retryable)
			require.Equal(t, "deep", event.ProfileID)
			require.Contains(t, event.Message, "不支持所选档位")
		}
		if event.Type == EventTypeComplete {
			t.Fatal("question must not complete on an unsupported profile")
		}
	}
	require.True(t, sawFailure)
	require.Len(t, runtime.Requests(), 1, "answer phase must not run")
}

func TestServiceKeepsProfileWhenPublicResearchFails(t *testing.T) {
	loader := balancedScopeLoader()
	runtime := newFakeRuntime(
		fakeExecution{status: codexruntime.StatusFailed},
		fakeExecution{
			deltas: []string{"按单集内容 [Show Notes L1-L1] 回答。"},
			result: json.RawMessage(`{"text":"按单集内容 [Show Notes L1-L1] 回答。"}`),
		},
	)
	service, err := NewService(loader, runtime, t.TempDir())
	require.NoError(t, err)

	events, err := service.Ask(context.Background(), QuestionRequest{
		EpisodeID: 91,
		Question:  "检索失败时档位保持一致。",
		ProfileID: "balanced",
	})
	require.NoError(t, err)

	var sawComplete bool
	for event := range events {
		if event.Type == EventTypeComplete {
			sawComplete = true
			require.Equal(t, "balanced", event.ProfileID)
		}
		if event.Type == EventTypeError {
			require.Equal(t, "balanced", event.ProfileID)
		}
	}
	require.True(t, sawComplete)
}

func TestNormalizeQuestionRequestTrimsProfileWhitespace(t *testing.T) {
	request, err := normalizeQuestionRequest(QuestionRequest{
		EpisodeID: 91,
		Question:  " 问题 ",
		ProfileID: " balanced ",
	})
	require.NoError(t, err)
	require.Equal(t, "balanced", request.ProfileID)

	_, err = normalizeQuestionRequest(QuestionRequest{
		EpisodeID: 91,
		Question:  "问题",
		ProfileID: "not-a-profile",
	})
	require.True(t, errors.Is(err, ErrUnsupportedProfile))
}
