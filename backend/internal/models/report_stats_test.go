package models

import "testing"

func TestBuildReportStatsLine(t *testing.T) {
	tokens := 7245
	tests := []struct {
		name string
		in   Report
		want ReportStats
	}{
		{
			name: "generated with usage",
			in: Report{
				PodcastsCount: 3,
				MatchedCount:  3,
				LLMSummary:    "可读摘要",
				LLMModelUsed:  "deepseek-v4-flash",
				LLMTokensUsed: 7245,
			},
			want: ReportStats{
				PodcastsCount: 3,
				EpisodesCount: 3,
				AIStatus:      AIStatusGenerated,
				AIStatusLabel: AIStatusLabelGenerated,
				Model:         "deepseek-v4-flash",
				ModelStatus:   FieldStatusKnown,
				Tokens:        &tokens,
				TokensStatus:  FieldStatusKnown,
				Line:          "3 个节目 · 3 集 · AI 已生成 · deepseek-v4-flash · 7.2K Token",
			},
		},
		{
			name: "regen incomplete error keeps generated status",
			in: Report{
				PodcastsCount: 1,
				MatchedCount:  1,
				LLMSummary:    "原有效摘要",
				LLMModelUsed:  "deepseek-v4-flash",
				LLMTokensUsed: 12,
				LLMError:      LLMErrorRegeneratePrefix + ": LLM摘要不完整: finish_reason=length",
			},
			want: ReportStats{
				PodcastsCount: 1,
				EpisodesCount: 1,
				AIStatus:      AIStatusGenerated,
				AIStatusLabel: AIStatusLabelGenerated,
				Model:         "deepseek-v4-flash",
				ModelStatus:   FieldStatusKnown,
				TokensStatus:  FieldStatusKnown,
				Line:          "1 个节目 · 1 集 · AI 已生成 · deepseek-v4-flash · 12 Token",
			},
		},
		{
			name: "empty body with tokens is not generated",
			in: Report{
				PodcastsCount: 3,
				MatchedCount:  3,
				LLMSummary:    "  \n",
				LLMModelUsed:  "deepseek-v4-flash",
				LLMTokensUsed: 7245,
				LLMError:      LLMErrorEmptyCompletion,
			},
			want: ReportStats{
				PodcastsCount: 3,
				EpisodesCount: 3,
				AIStatus:      AIStatusNotGenerated,
				AIStatusLabel: AIStatusLabelNotGenerated,
				Model:         "deepseek-v4-flash",
				ModelStatus:   FieldStatusKnown,
				Tokens:        &tokens,
				TokensStatus:  FieldStatusKnown,
				Line:          "3 个节目 · 3 集 · AI 未生成 · deepseek-v4-flash · 7.2K Token",
			},
		},
		{
			name: "truncated",
			in: Report{
				PodcastsCount: 1,
				MatchedCount:  2,
				LLMSummary:    "半份结论",
				LLMModelUsed:  "deepseek-v4-flash",
				LLMTokensUsed: 120,
				LLMError:      LLMErrorIncompletePrefix + ": length",
			},
			want: ReportStats{
				PodcastsCount: 1,
				EpisodesCount: 2,
				AIStatus:      AIStatusIncomplete,
				AIStatusLabel: AIStatusLabelIncomplete,
				Model:         "deepseek-v4-flash",
				ModelStatus:   FieldStatusKnown,
				TokensStatus:  FieldStatusKnown,
				Line:          "1 个节目 · 2 集 · AI 不完整 · deepseek-v4-flash · 120 Token",
			},
		},
		{
			name: "disabled",
			in: Report{
				PodcastsCount: 2,
				MatchedCount:  4,
				LLMError:      LLMErrorDisabled,
			},
			want: ReportStats{
				PodcastsCount: 2,
				EpisodesCount: 4,
				AIStatus:      AIStatusDisabled,
				AIStatusLabel: AIStatusLabelDisabled,
				ModelStatus:   FieldStatusUnused,
				TokensStatus:  FieldStatusUnused,
				Line:          "2 个节目 · 4 集 · AI 未启用 · 未调用 · —",
			},
		},
		{
			name: "no episodes",
			in:   Report{},
			want: ReportStats{
				AIStatus:      AIStatusNotNeeded,
				AIStatusLabel: AIStatusLabelNotNeeded,
				ModelStatus:   FieldStatusUnused,
				TokensStatus:  FieldStatusUnused,
				Line:          "0 个节目 · 0 集 · AI 无需生成 · 未调用 · —",
			},
		},
		{
			name: "historical unknown does not forge zero tokens",
			in: Report{
				PodcastsCount: 3,
				MatchedCount:  3,
			},
			want: ReportStats{
				PodcastsCount: 3,
				EpisodesCount: 3,
				AIStatus:      AIStatusUnknown,
				AIStatusLabel: AIStatusLabelUnknown,
				ModelStatus:   FieldStatusUnknown,
				TokensStatus:  FieldStatusUnknown,
				Line:          "3 个节目 · 3 集 · AI 状态未知 · 模型未知 · Token 未知",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.in.BuildReportStats()
			if got.Line != tt.want.Line {
				t.Fatalf("line: got %q want %q", got.Line, tt.want.Line)
			}
			if got.AIStatus != tt.want.AIStatus {
				t.Fatalf("status: got %q want %q", got.AIStatus, tt.want.AIStatus)
			}
			if got.ModelStatus != tt.want.ModelStatus || got.TokensStatus != tt.want.TokensStatus {
				t.Fatalf("field status: got model=%s tokens=%s", got.ModelStatus, got.TokensStatus)
			}
			if tt.want.TokensStatus == FieldStatusKnown {
				if got.Tokens == nil || *got.Tokens != tt.in.LLMTokensUsed {
					t.Fatalf("tokens: got %#v", got.Tokens)
				}
			} else if got.Tokens != nil {
				t.Fatalf("tokens should be omitted, got %d", *got.Tokens)
			}
		})
	}
}
