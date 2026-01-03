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

		// Tag 路由
		tagHandler := handlers.NewTagHandler()
		tags := v1.Group("/tags")
		{
			tags.GET("", tagHandler.List)
			tags.POST("", tagHandler.Create)
			tags.GET("/:id", tagHandler.Get)
			tags.PUT("/:id", tagHandler.Update)
			tags.DELETE("/:id", tagHandler.Delete)
		}

		// Note 路由
		noteHandler := handlers.NewNoteHandler()
		v1.PUT("/podcasts/:id/notes", noteHandler.UpdatePodcastNotes)
		v1.GET("/podcasts/:id/notes", noteHandler.GetPodcastNotes)
		v1.PUT("/episodes/:id/notes", noteHandler.UpdateEpisodeNotes)
		v1.GET("/episodes/:id/notes", noteHandler.GetEpisodeNotes)

		// Tag Relation 路由
		tagRelationHandler := handlers.NewTagRelationHandler()
		podcastTags := v1.Group("/podcasts/:id/tags")
		{
			podcastTags.GET("", tagRelationHandler.GetPodcastTags)
			podcastTags.POST("", tagRelationHandler.AddTagToPodcast)
			podcastTags.DELETE("/:tagId", tagRelationHandler.RemoveTagFromPodcast)
		}

		episodeTags := v1.Group("/episodes/:id/tags")
		{
			episodeTags.GET("", tagRelationHandler.GetEpisodeTags)
			episodeTags.POST("", tagRelationHandler.AddTagToEpisode)
			episodeTags.DELETE("/:tagId", tagRelationHandler.RemoveTagFromEpisode)
		}
	}

	return r
}
