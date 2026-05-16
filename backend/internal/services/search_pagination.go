package services

import (
	"magicpodcast/internal/models"
)

const searchCandidateBuffer = 50

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

func buildSearchCandidateLimit(page, pageSize int) int {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 1
	}

	return page*pageSize + searchCandidateBuffer
}
