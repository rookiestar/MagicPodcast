package services

import (
	"magicpodcast/internal/models"
)

// buildPaginationInfo 构建分页信息
func buildPaginationInfo(total int64, page, pageSize int) models.PaginationInfo {
	totalPages := int(total) / pageSize
	if int(total)%pageSize > 0 {
		totalPages++
	}

	return models.PaginationInfo{
		Page:       page,
		PageSize:   pageSize,
		Total:      int(total),
		TotalPages: totalPages,
	}
}

// paginatePodcastResults 对播客结果进行分页
func paginatePodcastResults(results []podcastWithScore, page, pageSize int) []models.PodcastSearchResult {
	// 过滤低相关性结果（只保留得分大于0的结果）
	filteredResults := make([]podcastWithScore, 0)
	for _, r := range results {
		if r.RelevanceScore > 0 {
			filteredResults = append(filteredResults, r)
		}
	}

	// 手动分页
	offset := (page - 1) * pageSize
	end := offset + pageSize
	if end > len(filteredResults) {
		end = len(filteredResults)
	}
	if offset > len(filteredResults) {
		return []models.PodcastSearchResult{}
	}

	// 转换为响应格式（在 search_core.go 中处理）
	return nil // 返回 nil 表示需要在外部处理
}

// paginateEpisodeResults 对单集结果进行分页
func paginateEpisodeResults(results []episodeWithScore, page, pageSize int) []models.EpisodeSearchResult {
	// 过滤低相关性结果（只保留得分大于0的结果）
	filteredResults := make([]episodeWithScore, 0)
	for _, r := range results {
		if r.RelevanceScore > 0 {
			filteredResults = append(filteredResults, r)
		}
	}

	// 手动分页
	offset := (page - 1) * pageSize
	end := offset + pageSize
	if end > len(filteredResults) {
		end = len(filteredResults)
	}
	if offset > len(filteredResults) {
		return []models.EpisodeSearchResult{}
	}

	// 转换为响应格式（在 search_core.go 中处理）
	return nil // 返回 nil 表示需要在外部处理
}
