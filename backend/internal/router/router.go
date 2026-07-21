package router

import (
	"magicpodcast/internal/logger"
	"time"

	"magicpodcast/internal/config"
	"magicpodcast/internal/database"
	"magicpodcast/internal/handlers"
	"magicpodcast/internal/llm"
	"magicpodcast/internal/middleware"
	"magicpodcast/internal/notifier"
	"magicpodcast/internal/repository"
	"magicpodcast/internal/scheduler"
	"magicpodcast/internal/services"
	syncsvc "magicpodcast/internal/sync"
	"magicpodcast/internal/workflow"

	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
)

// 全局scheduler实例
var globalScheduler *scheduler.Scheduler

// 全局PromptManager实例（用于Prompt模板API）
var globalPromptManager *llm.PromptManager

func workflowPodcastIndexPath(cfg *config.Config) string {
	if cfg == nil {
		return ""
	}
	return cfg.PodcastIndex.Path
}

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
	r.Use(gin.Recovery())                     // 恢复 panic
	r.Use(gin.Logger())                       // 请求日志
	r.Use(gzip.Gzip(gzip.DefaultCompression)) // Gzip 压缩
	r.Use(middleware.CORS())                  // CORS 跨域支持

	// 单人服务的高成本操作采用进程内准入控制：不信任客户端身份头，
	// 通过稳定的 409/413/429 错误让前端能明确区分冲突、超限和过快请求。
	resourceLimiter := middleware.NewOperationLimiter()
	syncOperation := resourceLimiter.Middleware("sync", middleware.OperationPolicy{
		MaxConcurrent: 1,
		MaxRequests:   2,
		Window:        time.Minute,
	})
	workflowOperation := resourceLimiter.Middleware("workflow", middleware.OperationPolicy{
		MaxConcurrent: 1,
		MaxRequests:   3,
		Window:        time.Minute,
	})
	llmOperation := resourceLimiter.Middleware("llm", middleware.OperationPolicy{
		MaxConcurrent: 1,
		MaxRequests:   5,
		Window:        time.Minute,
	})
	// 图片代理是无状态只读字节转发，且已有 reviewed-hosts 白名单、私网/CGNAT
	// 阻断、超时和大小/类型校验兜底；首屏会并发拉取多张封面，这里按真实首屏
	// 规模放宽准入，避免瞬时并发被中间件以 409 拒绝而进入 handler 之前。属调度
	// 行为变更，上限取值见 docs/HUMAN_REVIEW_QUEUE.md（issue #14）。
	imageOperation := resourceLimiter.Middleware("image", middleware.OperationPolicy{
		MaxConcurrent: 12,
		MaxRequests:   300,
		Window:        time.Minute,
	})
	cacheOperation := resourceLimiter.Middleware("cache", middleware.OperationPolicy{
		MaxConcurrent: 1,
		MaxRequests:   30,
		Window:        time.Minute,
	})

	// 健康检查
	healthHandler := handlers.NewHealthHandler()
	r.GET("/health", healthHandler.Health)
	r.GET("/live", healthHandler.Live)
	r.GET("/ready", healthHandler.Ready)
	r.GET("/ping", healthHandler.Ping)

	// 图片代理服务
	imageHandler := handlers.NewImageHandler()
	image := r.Group("/images")
	{
		image.GET("/proxy", imageOperation, imageHandler.ProxyImage)
		image.GET("/health", imageHandler.Health)
	}

	// API 路由组
	v1 := r.Group("/api/v1")
	{
		// Podcast 路由
		podcastHandler := handlers.NewPodcastHandler()
		episodeHandler := handlers.NewEpisodeHandler()
		podcasts := v1.Group("/podcasts")
		{
			podcasts.GET("", podcastHandler.List)
			podcasts.POST("/batch", podcastHandler.BatchGet)
			podcasts.GET("/:id", podcastHandler.Get)
			podcasts.GET("/:id/episodes", episodeHandler.ListByPodcast)
			podcasts.PUT("/:id/custom-cover", podcastHandler.UpdateCustomCover) // 更新自定义封面
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

		// 初始化 Repository 容器
		repos, err := repository.NewRepositories()
		if err != nil {
			panic(err) // Repository 初始化失败，无法继续
		}

		// Tag Relation 路由
		tagRelationService := services.NewTagRelationService(repos)
		tagRelationHandler := handlers.NewTagRelationHandler(tagRelationService)
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
			sync.POST("/import", middleware.RequestBodyLimit(middleware.DefaultUploadRequestLimitBytes), syncOperation, syncHandler.ImportOPML)        // 导入OPML文件
			sync.POST("/import-sse", middleware.RequestBodyLimit(middleware.DefaultUploadRequestLimitBytes), syncOperation, syncHandler.ImportOPMLSSE) // 导入OPML文件（SSE流式）
			sync.POST("/subscriptions", syncOperation, syncHandler.SyncSubscriptions)                                                                  // 同步所有订阅
			sync.GET("/status", syncHandler.GetSyncStatus)                                                                                             // 获取同步状态
			sync.POST("/podcasts/metadata-sse", syncOperation, syncHandler.SyncPodcastsMetadataSSE)                                                    // 同步所有播客元数据（SSE流式，已包含单集同步）
			sync.POST("/episodes", syncOperation, syncHandler.SyncAllEpisodes)                                                                         // 同步所有podcast的episodes（SSE流式）
			sync.POST("/episodes/sync", syncOperation, syncHandler.SyncAllEpisodesNonStreaming)                                                        // 同步所有podcast的episodes（非流式，用于定时任务）
		}

		// Episode Sync 路由（单个podcast的episode同步）
		v1.POST("/podcasts/:id/episodes/sync", syncOperation, syncHandler.SyncPodcastEpisodes) // 同步指定podcast的episodes

		// Workflow 路由
		// 创建workflow执行器
		db := database.GetDB()
		syncService, err := syncsvc.NewService(db, workflowPodcastIndexPath(cfg))
		if err != nil {
			// sync service初始化失败不影响workflow handler创建
			// workflow trigger会返回错误
			syncService = nil
		}

		// 创建邮件通知器
		var emailNotifier *notifier.EmailNotifier
		if cfg.Email.Enabled {
			emailNotifier = notifier.NewEmailNotifier(&cfg.Email)
			logger.Info("📧 邮件通知器已初始化")
		} else {
			logger.Info("📧 邮件通知未启用")
		}

		// 初始化LLM客户端和摘要生成器
		var summarizer workflow.SummarizerInterface
		if cfg.LLM.Enabled {
			llmClient := llm.NewClient(&cfg.LLM)
			globalPromptManager = llm.NewPromptManager(cfg.LLM.PromptsDir)
			summarizer = llm.NewSummarizer(llmClient, globalPromptManager)
			logger.Infof("✅ LLM客户端初始化成功 (Model: %s)", cfg.LLM.DefaultModel)
		} else {
			logger.Info("ℹ️  LLM功能未启用")
		}

		workflowExecutor := workflow.NewExecutor(db, syncService, emailNotifier, summarizer)

		// 创建全局scheduler实例
		globalScheduler = scheduler.NewScheduler(db, workflowExecutor)

		workflowHandler := handlers.NewWorkflowHandler(workflowExecutor, globalScheduler, summarizer)
		workflows := v1.Group("/workflows")
		{
			workflows.GET("", workflowHandler.List)                                    // 获取工作流列表
			workflows.POST("", workflowHandler.Create)                                 // 创建工作流
			workflows.GET("/:id", workflowHandler.Get)                                 // 获取工作流详情
			workflows.PUT("/:id", workflowHandler.Update)                              // 更新工作流
			workflows.DELETE("/:id", workflowHandler.Delete)                           // 删除工作流
			workflows.POST("/:id/toggle", workflowHandler.Toggle)                      // 启用/禁用工作流
			workflows.GET("/:id/jobs", workflowHandler.ListJobs)                       // 获取工作流执行历史
			workflows.POST("/:id/trigger", workflowOperation, workflowHandler.Trigger) // 手动触发工作流
		}

		// Job 路由
		v1.GET("/jobs/:id", workflowHandler.GetJob)                                             // 获取任务详情
		v1.GET("/jobs/:id/report", workflowHandler.GetJobReport)                                // 获取任务报告
		v1.POST("/jobs/:id/regenerate-llm", llmOperation, workflowHandler.RegenerateLLMSummary) // 重新生成AI摘要

		// Scheduler 路由
		schedulerHandler := handlers.NewSchedulerHandler(globalScheduler)
		schedulers := v1.Group("/scheduler")
		{
			schedulers.POST("/reload", schedulerHandler.Reload)
			schedulers.GET("/status", schedulerHandler.GetStatus)
			schedulers.POST("/workflows/:id/pause", schedulerHandler.PauseWorkflow)
			schedulers.POST("/workflows/:id/resume", schedulerHandler.ResumeWorkflow)
		}

		// Cache 路由
		cacheHandler := handlers.NewCacheHandler()
		cacheGroup := v1.Group("/cache")
		{
			cacheGroup.GET("/stats", cacheOperation, cacheHandler.GetStats)
			cacheGroup.POST("/clear", cacheOperation, cacheHandler.ClearCache)
		}

		// Admin 诊断路由：Feed 抓取可靠性诊断视图。仅暴露有界白名单聚合
		// （计数、延迟分桶、断路状态/转换、conditional-GET、last-good 命中、
		// last-good 容量），不含完整 Feed URL、正文、Cookie、凭据或任意响应头。
		// 访问控制依赖 loopback 绑定（config.Validate 拒绝非 loopback 绑定）+
		// Cloudflare Access；不新增 /metrics，不改变 /health 与 /ready 语义。
		adminFeedDiagnosticsHandler := handlers.NewAdminFeedDiagnosticsHandler(nil)
		v1.GET("/admin/feed-diagnostics", adminFeedDiagnosticsHandler.GetFeedDiagnostics)

		// LLM路由
		if globalPromptManager != nil {
			llmClient := llm.NewClient(&cfg.LLM)

			// LLM统计、健康检查和配置验证
			llmStatsHandler := handlers.NewLLMStatsHandler()
			llmHealthHandler := handlers.NewLLMHealthHandler(llmClient)
			llmConfigHandler := handlers.NewLLMConfigHandler(llmClient)

			llm := v1.Group("/llm")
			{
				llm.GET("/stats", llmStatsHandler.GetGlobalLLMStats)
				llm.GET("/health", llmOperation, llmHealthHandler.GetHealth)
				llm.POST("/validate-key", llmOperation, llmConfigHandler.ValidateKey)
				llm.GET("/models", llmConfigHandler.GetModels)
			}

			// Prompt模板路由
			promptTemplateHandler := handlers.NewPromptTemplateHandler(globalPromptManager)
			promptTemplates := v1.Group("/prompt-templates")
			{
				promptTemplates.GET("", promptTemplateHandler.ListTemplates)
				promptTemplates.GET("/:name", promptTemplateHandler.GetTemplate)
				promptTemplates.POST("", promptTemplateHandler.CreateTemplate)
				promptTemplates.PUT("/:name", promptTemplateHandler.UpdateTemplate)
				promptTemplates.DELETE("/:name", promptTemplateHandler.DeleteTemplate)
				promptTemplates.POST("/:name/reset", promptTemplateHandler.ResetTemplate)
			}
		}

		// 启动调度器（在独立goroutine中）
		go func() {
			if err := globalScheduler.Start(); err != nil {
				logger.Infof("❌ 启动调度器失败: %v", err)
			}
		}()

		return r
	}
}
