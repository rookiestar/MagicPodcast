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
		episodeHandler := handlers.NewEpisodeHandler()
		podcasts := v1.Group("/podcasts")
		{
			podcasts.GET("", podcastHandler.List)
			podcasts.GET("/:id", podcastHandler.Get)
			podcasts.GET("/:id/episodes", episodeHandler.ListByPodcast)
		}

		// Search 路由
		searchHandler := handlers.NewSearchHandler()
		v1.GET("/search", searchHandler.Search)

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

		// Sync 路由
		syncHandler, err := handlers.NewSyncHandler()
		if err != nil {
			panic(err) // 同步服务初始化失败，无法继续
		}
		sync := v1.Group("/sync")
		{
			sync.POST("/import", syncHandler.ImportOPML)                       // 导入OPML文件
			sync.POST("/import-sse", syncHandler.ImportOPMLSSE)                // 导入OPML文件（SSE流式）
			sync.POST("/subscriptions", syncHandler.SyncSubscriptions)         // 同步所有订阅
			sync.GET("/status", syncHandler.GetSyncStatus)                     // 获取同步状态
			sync.POST("/podcasts/metadata-sse", syncHandler.SyncPodcastsMetadataSSE) // 同步所有播客元数据（SSE流式）
			sync.POST("/podcasts/full", syncHandler.SyncPodcastsFull)           // 完整同步：元数据 + 单集（SSE流式）
			sync.POST("/episodes", syncHandler.SyncAllEpisodes)                // 同步所有podcast的episodes（SSE流式）
			sync.POST("/episodes/sync", syncHandler.SyncAllEpisodesNonStreaming) // 同步所有podcast的episodes（非流式，用于定时任务）
		}

		// Episode Sync 路由（单个podcast的episode同步）
		v1.POST("/podcasts/:id/episodes/sync", syncHandler.SyncPodcastEpisodes) // 同步指定podcast的episodes

		// Workflow 路由
		workflowHandler := handlers.NewWorkflowHandler()
		workflows := v1.Group("/workflows")
		{
			workflows.GET("", workflowHandler.List)           // 获取工作流列表
			workflows.POST("", workflowHandler.Create)         // 创建工作流
			workflows.GET("/:id", workflowHandler.Get)         // 获取工作流详情
			workflows.PUT("/:id", workflowHandler.Update)      // 更新工作流
			workflows.DELETE("/:id", workflowHandler.Delete)   // 删除工作流
			workflows.POST("/:id/toggle", workflowHandler.Toggle) // 启用/禁用工作流
			workflows.GET("/:id/jobs", workflowHandler.ListJobs) // 获取工作流执行历史
			workflows.POST("/:id/trigger", workflowHandler.Trigger) // 手动触发工作流
		}

		// Job 路由
		v1.GET("/jobs/:id", workflowHandler.GetJob) // 获取任务详情
	}

	return r
}
