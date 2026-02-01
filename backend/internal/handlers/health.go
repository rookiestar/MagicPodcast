package handlers

import (
	"magicpodcast/internal/database"
	"net/http"

	"github.com/gin-gonic/gin"
)

// HealthHandler 健康检查处理器
type HealthHandler struct{}

// NewHealthHandler 创建健康检查处理器
func NewHealthHandler() *HealthHandler {
	return &HealthHandler{}
}

// Health 健康检查接口
// @Summary 健康检查
// @Description 检查服务是否正常运行
// @Tags Health
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /health [get]
func (h *HealthHandler) Health(c *gin.Context) {
	// 检查数据库连接
	db := database.GetDB()
	sqlDB, err := db.DB()
	dbStatus := "ok"
	if err != nil {
		dbStatus = "error"
	} else if err := sqlDB.Ping(); err != nil {
		dbStatus = "error"
	}

	c.JSON(http.StatusOK, gin.H{
		"status":   "ok",
		"service":  "magicpodcast-backend",
		"database": dbStatus,
	})
}

// Ping 简单的 ping 接口
// @Summary Ping
// @Description 简单的 ping 接口，用于测试连通性
// @Tags Health
// @Accept json
// @Produce json
// @Success 200 {object} map[string]string
// @Router /ping [get]
func (h *HealthHandler) Ping(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"message": "pong",
	})
}
