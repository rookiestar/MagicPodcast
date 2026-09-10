package models

import (
	"fmt"
	"strings"
)

const (
	AIStatusGenerated    = "generated"
	AIStatusNotGenerated = "not_generated"
	AIStatusIncomplete   = "incomplete"
	AIStatusDisabled     = "disabled"
	AIStatusNotNeeded    = "not_needed"
	AIStatusUnknown      = "unknown"

	AIStatusLabelGenerated    = "AI 已生成"
	AIStatusLabelNotGenerated = "AI 未生成"
	AIStatusLabelIncomplete   = "AI 不完整"
	AIStatusLabelDisabled     = "AI 未启用"
	AIStatusLabelNotNeeded    = "AI 无需生成"
	AIStatusLabelUnknown      = "AI 状态未知"

	LLMErrorDisabled            = "AI未启用"
	LLMErrorIncompletePrefix    = "摘要不完整"
	LLMErrorRegeneratePrefix    = "重新生成失败"
	LLMErrorEmptyCompletion     = "LLM返回空摘要"
	LLMErrorInvalidFinishReason = "LLM结束原因异常"

	FieldStatusKnown   = "known"
	FieldStatusUnknown = "unknown"
	FieldStatusUnused  = "unused"
)

// ReportStats is the compact reading-line payload shared by homepage and report APIs.
type ReportStats struct {
	PodcastsCount int    `json:"podcasts_count"`
	EpisodesCount int    `json:"episodes_count"`
	AIStatus      string `json:"ai_status"`
	AIStatusLabel string `json:"ai_status_label"`
	Model         string `json:"model,omitempty"`
	ModelStatus   string `json:"model_status"`
	Tokens        *int   `json:"tokens"`
	TokensStatus  string `json:"tokens_status"`
	Line          string `json:"line"`
}

func AIStatusLabel(status string) string {
	switch status {
	case AIStatusGenerated:
		return AIStatusLabelGenerated
	case AIStatusNotGenerated:
		return AIStatusLabelNotGenerated
	case AIStatusIncomplete:
		return AIStatusLabelIncomplete
	case AIStatusDisabled:
		return AIStatusLabelDisabled
	case AIStatusNotNeeded:
		return AIStatusLabelNotNeeded
	default:
		return AIStatusLabelUnknown
	}
}

func (r *Report) generationEpisodeCount() int {
	if r == nil {
		return 0
	}
	if r.MatchedCount > 0 {
		return r.MatchedCount
	}
	return r.EpisodesCount
}

func (r *Report) DeriveAIStatus() string {
	if r == nil {
		return AIStatusUnknown
	}
	errorText := strings.TrimSpace(r.LLMError)
	summary := strings.TrimSpace(r.LLMSummary)
	episodes := r.generationEpisodeCount()

	if strings.HasPrefix(errorText, LLMErrorIncompletePrefix) {
		return AIStatusIncomplete
	}
	if errorText == LLMErrorDisabled {
		return AIStatusDisabled
	}
	if summary != "" {
		return AIStatusGenerated
	}
	if episodes == 0 && r.PodcastsCount == 0 && errorText == "" && strings.TrimSpace(r.LLMModelUsed) == "" {
		return AIStatusNotNeeded
	}
	if errorText != "" {
		return AIStatusNotGenerated
	}
	if episodes == 0 && r.PodcastsCount == 0 {
		return AIStatusNotNeeded
	}
	return AIStatusUnknown
}

func (r *Report) BuildReportStats() ReportStats {
	if r == nil {
		return ReportStats{
			AIStatus:      AIStatusUnknown,
			AIStatusLabel: AIStatusLabelUnknown,
			ModelStatus:   FieldStatusUnknown,
			TokensStatus:  FieldStatusUnknown,
			Line:          formatReportStatsLine(0, 0, AIStatusUnknown, FieldStatusUnknown, "", FieldStatusUnknown, nil),
		}
	}
	status := r.DeriveAIStatus()
	modelStatus, model := r.modelField(status)
	tokensStatus, tokens := r.tokenField(status)
	episodes := r.generationEpisodeCount()
	return ReportStats{
		PodcastsCount: r.PodcastsCount,
		EpisodesCount: episodes,
		AIStatus:      status,
		AIStatusLabel: AIStatusLabel(status),
		Model:         model,
		ModelStatus:   modelStatus,
		Tokens:        tokens,
		TokensStatus:  tokensStatus,
		Line:          formatReportStatsLine(r.PodcastsCount, episodes, status, modelStatus, model, tokensStatus, tokens),
	}
}

func (r *Report) modelField(status string) (string, string) {
	if status == AIStatusDisabled || status == AIStatusNotNeeded {
		return FieldStatusUnused, ""
	}
	model := strings.TrimSpace(r.LLMModelUsed)
	if model == "" {
		return FieldStatusUnknown, ""
	}
	return FieldStatusKnown, model
}

func (r *Report) tokenField(status string) (string, *int) {
	if status == AIStatusDisabled || status == AIStatusNotNeeded {
		return FieldStatusUnused, nil
	}
	if r.LLMTokensUsed > 0 {
		value := r.LLMTokensUsed
		return FieldStatusKnown, &value
	}
	return FieldStatusUnknown, nil
}

func formatReportStatsLine(podcasts, episodes int, status, modelStatus, model, tokensStatus string, tokens *int) string {
	modelPart := "模型未知"
	switch modelStatus {
	case FieldStatusUnused:
		modelPart = "未调用"
	case FieldStatusKnown:
		if model != "" {
			modelPart = model
		}
	}

	tokenPart := "Token 未知"
	switch tokensStatus {
	case FieldStatusUnused:
		tokenPart = "—"
	case FieldStatusKnown:
		if tokens != nil {
			tokenPart = formatTokenCount(*tokens) + " Token"
		}
	}

	return fmt.Sprintf("%d 个节目 · %d 集 · %s · %s · %s",
		podcasts, episodes, AIStatusLabel(status), modelPart, tokenPart)
}

func formatTokenCount(tokens int) string {
	if tokens < 1000 {
		return fmt.Sprintf("%d", tokens)
	}
	if tokens < 1000000 {
		return fmt.Sprintf("%.1fK", float64(tokens)/1000)
	}
	return fmt.Sprintf("%.1fM", float64(tokens)/1000000)
}
