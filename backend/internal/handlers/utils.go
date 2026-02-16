package handlers

import (
	"fmt"
	"strconv"

	"github.com/gin-gonic/gin"
	"magicpodcast/internal/middleware"
)

// parseInt 解析int参数，带默认值
func parseInt(s string, defaultValue int) int {
	if s == "" {
		return defaultValue
	}
	var result int
	if _, err := fmt.Sscanf(s, "%d", &result); err != nil {
		return defaultValue
	}
	return result
}

// ============ 通用参数解析辅助函数 ============

// PaginationParams 分页参数结构
type PaginationParams struct {
	Page     int
	PageSize int
}

// ParseUintParam 解析路径中的 uint 参数，失败时自动返回错误响应
// 返回值: (id, ok) - ok 为 false 表示已返回错误响应，调用方应直接 return
func ParseUintParam(c *gin.Context, key string) (uint, bool) {
	value := c.Param(key)
	id, err := strconv.ParseUint(value, 10, 32)
	if err != nil {
		middleware.ValidationErrorResponse(c, key, "必须是有效的数字")
		return 0, false
	}
	return uint(id), true
}

// ParsePaginationParams 解析分页参数，带默认值和边界验证
// 默认: page=1, pageSize=15
// 限制: page >= 1, 1 <= pageSize <= 100
func ParsePaginationParams(c *gin.Context, defaultPageSize int) PaginationParams {
	if defaultPageSize <= 0 {
		defaultPageSize = 15
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", strconv.Itoa(defaultPageSize)))

	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = defaultPageSize
	}
	if pageSize > 100 {
		pageSize = 100
	}

	return PaginationParams{
		Page:     page,
		PageSize: pageSize,
	}
}

// ParseUintQueryParam 解析 query 中的 uint 参数
// 返回值: (value, ok) - ok 为 false 表示参数不存在或无效
func ParseUintQueryParam(c *gin.Context, key string) (uint, bool) {
	valueStr := c.Query(key)
	if valueStr == "" {
		return 0, false
	}
	value, err := strconv.ParseUint(valueStr, 10, 32)
	if err != nil {
		return 0, false
	}
	return uint(value), true
}

// ParseUintSliceQueryParam 解析 query 中的 uint 数组参数 (如 ?tag_id=1&tag_id=2)
func ParseUintSliceQueryParam(c *gin.Context, key string) []uint {
	values := c.QueryArray(key)
	result := make([]uint, 0, len(values))
	for _, v := range values {
		if id, err := strconv.ParseUint(v, 10, 32); err == nil {
			result = append(result, uint(id))
		}
	}
	return result
}
