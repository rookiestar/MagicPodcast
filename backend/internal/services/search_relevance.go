package services

import (
	"strings"

	"magicpodcast/internal/config"
	"magicpodcast/internal/models"
)

// calculatePodcastRelevance 计算播客相关性得分
func calculatePodcastRelevance(title, author, description, keyword string, cfg config.SearchConfig) float64 {
	// 优化：缓存ToLower结果，避免重复调用
	keywordLower := strings.ToLower(keyword)
	titleLower := strings.ToLower(title)
	authorLower := strings.ToLower(author)
	descLower := strings.ToLower(description)

	var score float64

	// 检查关键词是否为纯数字且较短（如"42"）
	isShortNumber := isPureNumber(keyword) && len([]rune(keyword)) <= 3

	// 标题匹配（使用缓存的titleLower）
	if titleLower == keywordLower {
		score += cfg.Weights.PodcastTitle * cfg.MatchMultipliers.Exact
	} else if strings.HasPrefix(titleLower, keywordLower) {
		score += cfg.Weights.PodcastTitle * cfg.MatchMultipliers.Prefix
	} else if strings.Contains(titleLower, keywordLower) {
		// 如果是短数字关键词，只有在标题中完整包含时才给分
		if isShortNumber {
			// 检查是否是独立的数字（被空格、标点包围，或在开头/结尾）
			if isStandaloneNumber(titleLower, keywordLower) {
				score += cfg.Weights.PodcastTitle * cfg.MatchMultipliers.Contains * 0.5
			}
		} else {
			score += cfg.Weights.PodcastTitle * cfg.MatchMultipliers.Contains
		}
	}

	// 作者匹配（使用缓存的authorLower）
	if authorLower == keywordLower {
		score += cfg.Weights.Author * cfg.MatchMultipliers.Exact
	} else if strings.HasPrefix(authorLower, keywordLower) {
		score += cfg.Weights.Author * cfg.MatchMultipliers.Prefix
	} else if strings.Contains(authorLower, keywordLower) {
		if isShortNumber {
			if isStandaloneNumber(authorLower, keywordLower) {
				score += cfg.Weights.Author * cfg.MatchMultipliers.Contains * 0.5
			}
		} else {
			score += cfg.Weights.Author * cfg.MatchMultipliers.Contains
		}
	}

	// 简介匹配（使用缓存的descLower）
	if strings.Contains(descLower, keywordLower) {
		// 如果是短数字关键词，大幅降低描述匹配的权重
		if isShortNumber {
			if isStandaloneNumber(descLower, keywordLower) {
				occurrences := strings.Count(descLower, keywordLower)
				descScore := cfg.Weights.PodcastDesc * cfg.MatchMultipliers.Contains * 0.3
				if occurrences > 1 {
					descScore *= (1 + float64(occurrences-1) * cfg.MatchMultipliers.Occurrence)
				}
				score += descScore
			}
		} else {
			occurrences := strings.Count(descLower, keywordLower)
			descScore := cfg.Weights.PodcastDesc * cfg.MatchMultipliers.Contains
			if occurrences > 1 {
				descScore *= (1 + float64(occurrences-1) * cfg.MatchMultipliers.Occurrence)
			}
			score += descScore
		}
	}

	// 改进：对完整关键词匹配给予额外加分
	// 如果关键词是多字符且在内容中完整出现，给予额外权重
	if len([]rune(keyword)) >= 2 {
		// 检查是否在标题中完整出现
		if strings.Contains(titleLower, keywordLower) && titleLower != keywordLower {
			score += 0.3 // 标题包含完整关键词的额外加分
		}
		// 检查是否在作者中完整出现
		if strings.Contains(authorLower, keywordLower) && authorLower != keywordLower {
			score += 0.2 // 作者包含完整关键词的额外加分
		}
	}

	return score
}

// calculateEpisodeRelevance 计算单集相关性得分
func calculateEpisodeRelevance(title, showNotes, keyword string, cfg config.SearchConfig) float64 {
	// 优化：缓存ToLower结果
	keywordLower := strings.ToLower(keyword)
	titleLower := strings.ToLower(title)
	notesLower := strings.ToLower(showNotes)

	var score float64

	// 检查关键词是否为纯数字且较短（如"42"）
	isShortNumber := isPureNumber(keyword) && len([]rune(keyword)) <= 3

	// 标题匹配（使用缓存的titleLower）
	if titleLower == keywordLower {
		score += cfg.Weights.EpisodeTitle * cfg.MatchMultipliers.Exact
	} else if strings.HasPrefix(titleLower, keywordLower) {
		score += cfg.Weights.EpisodeTitle * cfg.MatchMultipliers.Prefix
	} else if strings.Contains(titleLower, keywordLower) {
		// 如果是短数字关键词，只有在标题中完整包含时才给分
		if isShortNumber {
			if isStandaloneNumber(titleLower, keywordLower) {
				score += cfg.Weights.EpisodeTitle * cfg.MatchMultipliers.Contains * 0.5
			}
		} else {
			score += cfg.Weights.EpisodeTitle * cfg.MatchMultipliers.Contains
		}
	}

	// 内容匹配（使用缓存的notesLower）
	if strings.Contains(notesLower, keywordLower) {
		// 如果是短数字关键词，大幅降低内容匹配的权重
		if isShortNumber {
			if isStandaloneNumber(notesLower, keywordLower) {
				occurrences := strings.Count(notesLower, keywordLower)
				notesScore := cfg.Weights.EpisodeContent * cfg.MatchMultipliers.Contains * 0.3
				if occurrences > 1 {
					notesScore *= (1 + float64(occurrences-1) * cfg.MatchMultipliers.Occurrence)
				}
				score += notesScore
			}
		} else {
			occurrences := strings.Count(notesLower, keywordLower)
			notesScore := cfg.Weights.EpisodeContent * cfg.MatchMultipliers.Contains
			if occurrences > 1 {
				notesScore *= (1 + float64(occurrences-1) * cfg.MatchMultipliers.Occurrence)
			}
			score += notesScore
		}
	}

	// 改进：对完整关键词匹配给予额外加分
	// 如果关键词是多字符且在内容中完整出现，给予额外权重
	if len([]rune(keyword)) >= 2 {
		// 检查是否在标题中完整出现
		if strings.Contains(titleLower, keywordLower) && titleLower != keywordLower {
			score += 0.3 // 标题包含完整关键词的额外加分
		}
	}

	return score
}

// extractMatchedFields 提取匹配字段（播客）
func extractMatchedFields(title, author, description, keyword string, cfg config.SearchConfig) []models.MatchedField {
	var fields []models.MatchedField
	keywordLower := strings.ToLower(keyword)

	if strings.Contains(strings.ToLower(title), keywordLower) {
		fields = append(fields, models.MatchedField{
			Field:   "title",
			Score:   cfg.Weights.PodcastTitle,
			Snippet: generateSnippet(title, keyword),
		})
	}

	if strings.Contains(strings.ToLower(author), keywordLower) {
		fields = append(fields, models.MatchedField{
			Field:   "author",
			Score:   cfg.Weights.Author,
			Snippet: generateSnippet(author, keyword),
		})
	}

	if strings.Contains(strings.ToLower(description), keywordLower) {
		fields = append(fields, models.MatchedField{
			Field:   "description",
			Score:   cfg.Weights.PodcastDesc,
			Snippet: generateSnippet(description, keyword),
		})
	}

	return fields
}

// extractMatchedFieldsFromEpisode 提取匹配字段（单集）
func extractMatchedFieldsFromEpisode(title, showNotes, keyword string, cfg config.SearchConfig) []models.MatchedField {
	var fields []models.MatchedField
	keywordLower := strings.ToLower(keyword)
	titleLower := strings.ToLower(title)
	showNotesLower := strings.ToLower(showNotes)

	if strings.Contains(titleLower, keywordLower) {
		fields = append(fields, models.MatchedField{
			Field:   "title",
			Score:   cfg.Weights.EpisodeTitle,
			Snippet: generateSnippet(title, keyword),
		})
	}

	if strings.Contains(showNotesLower, keywordLower) {
		fields = append(fields, models.MatchedField{
			Field:   "show_notes",
			Score:   cfg.Weights.EpisodeContent,
			Snippet: generateSnippet(showNotes, keyword),
		})
	}

	return fields
}
