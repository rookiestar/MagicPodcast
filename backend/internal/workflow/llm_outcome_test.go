package workflow

import (
	"fmt"
	"strings"
	"testing"

	"magicpodcast/internal/llm"
	"magicpodcast/internal/models"

	"github.com/stretchr/testify/require"
)

func TestReplaceOrInsertLLMSummaryDoesNotDuplicate(t *testing.T) {
	original := "# 标题\n\n## 🤖 AI智能摘要\n\n旧摘要\n\n---\n\n> **🕐 执行**: 2026-09-10\n"
	once := ReplaceOrInsertLLMSummary(original, "新摘要")
	twice := ReplaceOrInsertLLMSummary(once, "新摘要")
	require.Equal(t, 1, strings.Count(twice, "## 🤖 AI智能摘要"))
	require.Contains(t, twice, "新摘要")
	require.NotContains(t, twice, "旧摘要")
}

func TestApplyLLMOutcomeKeepsValidSummaryOnFailure(t *testing.T) {
	report := &models.Report{
		LLMSummary:    "原有效摘要",
		LLMModelUsed:  "deepseek-v4-flash",
		LLMTokensUsed: 12,
		Content:       ReplaceOrInsertLLMSummary("# 标题\n\n正文\n", "原有效摘要"),
	}
	ApplyLLMOutcome(report, nil, fmt.Errorf("本次调用失败"), true)
	require.Equal(t, "原有效摘要", report.LLMSummary)
	require.Contains(t, report.LLMError, "本次调用失败")
	require.Contains(t, report.Content, "原有效摘要")
	require.Equal(t, 1, strings.Count(report.Content, "## 🤖 AI智能摘要"))
}

func TestApplyLLMOutcomeKeepsValidSummaryAcrossIncompleteRegenerates(t *testing.T) {
	original := "原有效摘要"
	report := &models.Report{
		LLMSummary:    original,
		LLMModelUsed:  "deepseek-v4-flash",
		LLMTokensUsed: 12,
		Content:       ReplaceOrInsertLLMSummary("# 标题\n\n正文\n", original),
	}
	truncated := &llm.SummaryResult{
		Summary:      "半份结论",
		ModelUsed:    "deepseek-v4-flash",
		TokensUsed:   80,
		FinishReason: "length",
		Incomplete:   true,
	}

	ApplyLLMOutcome(report, truncated, llm.ErrIncompleteCompletion, true)
	require.Equal(t, original, report.LLMSummary)
	require.Equal(t, models.AIStatusGenerated, report.DeriveAIStatus())
	require.Contains(t, report.LLMError, models.LLMErrorRegeneratePrefix)
	require.Contains(t, report.Content, original)
	require.NotContains(t, report.Content, "半份结论")
	require.Equal(t, 1, strings.Count(report.Content, "## 🤖 AI智能摘要"))

	ApplyLLMOutcome(report, truncated, llm.ErrIncompleteCompletion, true)
	require.Equal(t, original, report.LLMSummary)
	require.Equal(t, models.AIStatusGenerated, report.DeriveAIStatus())
	require.Contains(t, report.Content, original)
	require.NotContains(t, report.Content, "半份结论")
	require.Equal(t, 1, strings.Count(report.Content, "## 🤖 AI智能摘要"))
}

func TestApplyLLMOutcomeRecordsIncomplete(t *testing.T) {
	report := &models.Report{Content: "# 标题\n\n正文\n"}
	ApplyLLMOutcome(report, &llm.SummaryResult{
		Summary: "半份", ModelUsed: "deepseek-v4-flash", TokensUsed: 9,
		FinishReason: "length", Incomplete: true,
	}, llm.ErrIncompleteCompletion, true)
	require.Equal(t, models.AIStatusIncomplete, report.DeriveAIStatus())
	require.Equal(t, "半份", report.LLMSummary)
	require.Equal(t, 9, report.LLMTokensUsed)
}

func TestIncompleteSummaryRemainsIncompleteAfterNetworkFailure(t *testing.T) {
	r := &models.Report{LLMSummary: "半份", LLMError: models.LLMErrorIncompletePrefix + ": length", Content: "# 报告\n"}
	ApplyLLMOutcome(r, nil, fmt.Errorf("network failed"), true)
	require.Equal(t, models.AIStatusIncomplete, r.DeriveAIStatus())
	require.Contains(t, r.LLMError, "network failed")
}

func TestReplaceSummaryWithInternalHeadingsAndSeparators(t *testing.T) {
	old := "## 总体概览\n旧概览\n\n---\n\n## 核心内容\n旧观点"
	body := "# 报告\n\n## 🤖 AI智能摘要\n\n" + old + "\n\n---\n\n> **🕐 执行**: today\n\n## 📝 单集详情\n保留单集"
	got := ReplaceOrInsertLLMSummary(body, "新摘要")
	require.NotContains(t, got, "旧概览")
	require.NotContains(t, got, "旧观点")
	require.Contains(t, got, "保留单集")
	require.Contains(t, got, "> **🕐 执行**: today")
}
