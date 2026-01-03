package router

import (
	"magicpodcast/internal/config"
	"magicpodcast/internal/handlers"
	"magicpodcast/internal/middleware"

	"github.com/gin-gonic/gin"
)

// SetupRouter 配置并返回路由器
func SetupRouter() *gin.Engine {
	cfg := config.Get()

	// 设置 Gin 模式
	if cfg.IsProduction() {
		gin.SetMode(gin.ReleaseMode)
	} else {
		gin.SetMode(gin.DebugMode)
	}

	r := gin.New()

	// 中间件
	r.Use(gin.Recovery())               // 恢复 panic
	r.Use(gin.Logger())                  // 请求日志
	r.Use(middleware.CORS())            // CORS 跨域支持

	// 健康检查
	healthHandler := handlers.NewHealthHandler()
	r.GET("/health", healthHandler.Health)
	r.GET("/ping", healthHandler.Ping)

	// API 路由组
	v1 := r.Group("/api/v1")
	{
		// Podcast 路由
		podcastHandler := handlers.NewPodcastHandler()
		podcasts := v1.Group("/podcasts")
		{
			podcasts.GET("", podcastHandler.List)
			podcasts.GET("/:id", podcastHandler.Get)
		}

		// 其他路由（后续阶段实现）
		// v1.GET("/episodes", episodeHandler.List)
		// v1.GET("/tags", tagHandler.List)
		// v1.GET("/workflows", workflowHandler.List)
	}

	return r
}
