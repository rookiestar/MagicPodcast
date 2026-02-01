package handlers

import (
	"fmt"
	"strconv"

	"github.com/gin-gonic/gin"
)

// parseUintParam 解析uint路径参数
func parseUintParam(c *gin.Context, key string) (uint, error) {
	value := c.Param(key)
	id, err := strconv.ParseUint(value, 10, 32)
	if err != nil {
		return 0, err
	}
	return uint(id), nil
}

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
