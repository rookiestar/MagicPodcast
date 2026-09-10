package workflow

import (
	"errors"
	"strings"

	"magicpodcast/internal/llm"
	"magicpodcast/internal/models"
)

const aiSummaryHeading = "## 🤖 AI智能摘要"

func existingSummaryIsReusable(report *models.Report) bool {
	if report == nil {
		return false
	}
	if strings.TrimSpace(report.LLMSummary) == "" {
		return false
	}
	status := report.DeriveAIStatus()
	return status == models.AIStatusGenerated
}

func incompleteErrorText(result *llm.SummaryResult, callErr error) string {
	reason := ""
	if result != nil {
		reason = strings.TrimSpace(result.FinishReason)
	}
	if reason == "" && callErr != nil {
		reason = callErr.Error()
	}
	if reason == "" {
		return models.LLMErrorIncompletePrefix
	}
	return models.LLMErrorIncompletePrefix + ": " + reason
}

// ApplyLLMOutcome writes summary validity, model, and usage onto a report.
// A failed or incomplete regenerate does not overwrite an existing valid summary.
func ApplyLLMOutcome(report *models.Report, result *llm.SummaryResult, callErr error, updateMarkdown bool) {
	if report == nil {
		return
	}

	keepValid := existingSummaryIsReusable(report) && callErr != nil

	if keepValid {
		report.LLMError = models.LLMErrorRegeneratePrefix + ": " + callErr.Error()
		return
	}

	if result != nil {
		if result.ModelUsed != "" {
			report.LLMModelUsed = result.ModelUsed
		}
		if result.TokensUsed > 0 {
			report.LLMTokensUsed = result.TokensUsed
		}
	}

	incomplete := callErr != nil && (errors.Is(callErr, llm.ErrIncompleteCompletion) || (result != nil && result.Incomplete))
	if incomplete {
		summary := ""
		if result != nil {
			summary = result.Summary
		}
		report.LLMSummary = summary
		report.LLMError = incompleteErrorText(result, callErr)
		if updateMarkdown {
			report.Content = ReplaceOrInsertLLMSummary(report.Content, summary)
		}
		return
	}

	if callErr != nil {
		wasIncomplete := report.DeriveAIStatus() == models.AIStatusIncomplete
		report.LLMError = callErr.Error()
		if wasIncomplete {
			report.LLMError = models.LLMErrorIncompletePrefix + ": " + callErr.Error()
		}
		if strings.TrimSpace(report.LLMSummary) == "" && updateMarkdown {
			report.Content = ReplaceOrInsertLLMSummary(report.Content, "")
		}
		return
	}

	if result == nil || strings.TrimSpace(result.Summary) == "" {
		report.LLMSummary = ""
		report.LLMError = models.LLMErrorEmptyCompletion
		if updateMarkdown {
			report.Content = ReplaceOrInsertLLMSummary(report.Content, "")
		}
		return
	}

	report.LLMSummary = result.Summary
	report.LLMError = ""
	if result.ModelUsed != "" {
		report.LLMModelUsed = result.ModelUsed
	}
	if result.TokensUsed > 0 {
		report.LLMTokensUsed = result.TokensUsed
	}
	if updateMarkdown {
		report.Content = ReplaceOrInsertLLMSummary(report.Content, result.Summary)
	}
}

// ReplaceOrInsertLLMSummary replaces any existing AI summary chapter, or inserts one after the title.
func ReplaceOrInsertLLMSummary(markdown, llmSummary string) string {
	stripped := stripAISummarySections(markdown)
	if strings.TrimSpace(llmSummary) == "" {
		return stripped
	}
	return insertLLMSummaryAfterTitle(stripped, llmSummary)
}

func stripAISummarySections(markdown string) string {
	lines := strings.Split(markdown, "\n")
	out := make([]string, 0, len(lines))
	skipping := false
	// Generated reports delimit the summary with system metadata. Model headings
	// and horizontal rules inside the summary are not chapter boundaries.
	hasMetadata := strings.Contains(markdown, "> **🕐 执行**")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, aiSummaryHeading) {
			skipping = true
			continue
		}
		if skipping {
			if !hasMetadata && trimmed == "---" {
				skipping = false
				continue
			}
			if strings.HasPrefix(trimmed, "> **🕐 执行**") || trimmed == "## 📝 单集详情" {
				skipping = false
			} else {
				continue
			}
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}
