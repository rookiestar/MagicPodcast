package handlers

import (
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"magicpodcast/internal/config"
	"magicpodcast/internal/database"
	"magicpodcast/internal/runtimeprofile"
)

var loadRuntimeProfile = runtimeprofile.Load

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
		if cfg := config.Get(); cfg != nil && strings.TrimSpace(cfg.Server.Mode) != "" {
			buildMode = cfg.Server.Mode
		} else {
			buildMode = "unknown"
		}
	}

	response := gin.H{
		"release_id":        releaseID,
		"frontend_build_id": frontendBuildID,
		"build_mode":        buildMode,
	}
	databasePath := ""
	if cfg := config.Get(); cfg != nil {
		databasePath = cfg.Database.Path
	}
	profile, err := loadRuntimeProfile(databasePath, buildMode)
	if err != nil {
		response["data_profile"] = "invalid"
		response["data_profile_error"] = "profile metadata validation failed"
		return response
	}
	for key, value := range profile.PublicFields() {
		response[key] = value
	}
	return response
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
	if response["data_profile"] == "invalid" {
		statusCode = http.StatusServiceUnavailable
		response["status"] = "error"
	}
	c.JSON(statusCode, response)
}

// Live reports only that the process and HTTP server are alive. It deliberately
// does not touch the database, so a dependency outage can be distinguished from
// a dead process.
func (h *HealthHandler) Live(c *gin.Context) {
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
		if cfg := config.Get(); cfg != nil && strings.TrimSpace(cfg.Server.Mode) != "" {
			buildMode = cfg.Server.Mode
		} else {
			buildMode = "unknown"
		}
	}
	response := gin.H{
		"status":            "ok",
		"service":           "magicpodcast-backend",
		"release_id":        releaseID,
		"frontend_build_id": frontendBuildID,
		"build_mode":        buildMode,
	}
	c.JSON(http.StatusOK, response)
}

// Ready is the explicit dependency-aware readiness endpoint.
func (h *HealthHandler) Ready(c *gin.Context) {
	db := database.GetDB()
	sqlDB, err := db.DB()
	if err != nil || sqlDB.Ping() != nil {
		response := gin.H{
			"status":   "error",
			"service":  "magicpodcast-backend",
			"database": "error",
		}
		for key, value := range runtimeMetadata() {
			response[key] = value
		}
		c.JSON(http.StatusServiceUnavailable, response)
		return
	}
	schema, err := database.InspectSchema(db)
	if err != nil || schema.CurrentVersion != database.CurrentSchemaVersion || len(schema.RequiredTablesMissing) != 0 || len(schema.Pending) != 0 {
		response := gin.H{
			"status":         "error",
			"service":        "magicpodcast-backend",
			"database":       "error",
			"schema_version": schema.CurrentVersion,
		}
		for key, value := range runtimeMetadata() {
			response[key] = value
		}
		c.JSON(http.StatusServiceUnavailable, response)
		return
	}
	response := gin.H{
		"status":         "ok",
		"service":        "magicpodcast-backend",
		"database":       "ok",
		"schema_version": schema.CurrentVersion,
	}
	for key, value := range runtimeMetadata() {
		response[key] = value
	}
	if response["data_profile"] == "invalid" {
		response["status"] = "error"
		c.JSON(http.StatusServiceUnavailable, response)
		return
	}
	c.JSON(http.StatusOK, response)
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
