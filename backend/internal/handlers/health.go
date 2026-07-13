package handlers

import (
	"magicpodcast/internal/database"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

// HealthHandler 健康检查处理器
type HealthHandler struct{}

func runtimeMetadata() gin.H {
	releaseID := strings.TrimSpace(os.Getenv("MAGICPODCAST_RELEASE_ID"))
	if releaseID == "" {
		releaseID = "unknown"
	}
	frontendBuildID := strings.TrimSpace(os.Getenv("MAGICPODCAST_FRONTEND_BUILD_ID"))
	if frontendBuildID == "" {
		frontendBuildID = "unknown"
	}
	buildMode := strings.TrimSpace(os.Getenv("MAGICPODCAST_SERVER_MODE"))
	if buildMode == "" {
		buildMode = "unknown"
	}

	return gin.H{
		"release_id":        releaseID,
		"frontend_build_id": frontendBuildID,
		"build_mode":        buildMode,
	}
}

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

	statusCode := http.StatusOK
	status := "ok"
	if dbStatus != "ok" {
		statusCode = http.StatusServiceUnavailable
		status = "error"
	}

	response := gin.H{
		"status":   status,
		"service":  "magicpodcast-backend",
		"database": dbStatus,
	}
	for key, value := range runtimeMetadata() {
		response[key] = value
	}
	c.JSON(statusCode, response)
}

// Live reports only that the process and HTTP server are alive. It deliberately
// does not touch the database, so a dependency outage can be distinguished from
// a dead process.
func (h *HealthHandler) Live(c *gin.Context) {
	response := gin.H{
		"status":  "ok",
		"service": "magicpodcast-backend",
	}
	for key, value := range runtimeMetadata() {
		response[key] = value
	}
	c.JSON(http.StatusOK, response)
}

// Ready is the explicit dependency-aware readiness endpoint.
func (h *HealthHandler) Ready(c *gin.Context) {
	h.Health(c)
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
