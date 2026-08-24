package main

import (
	"context"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"magicpodcast/internal/codexruntime"
	"magicpodcast/internal/config"
	"magicpodcast/internal/database"
	"magicpodcast/internal/episodecopilot"
	"magicpodcast/internal/feed"
	"magicpodcast/internal/logger"
	"magicpodcast/internal/processing"
	"magicpodcast/internal/router"
	"magicpodcast/internal/runtimeprofile"
)

func main() {
	// 加载 .env 文件（如果存在）
	// 优先从当前目录查找，然后从上级目录查找
	envLoaded := false
	if os.Getenv("MAGICPODCAST_SKIP_DOTENV") == "1" {
		logger.Info("ℹ️  .env loading explicitly disabled for managed data profile")
	} else {
		envPaths := []string{".env", "../.env", "../../.env"}
		for _, envPath := range envPaths {
			if err := godotenv.Load(envPath); err == nil {
				logger.Infof("✅ Loaded .env from: %s", envPath)
				envLoaded = true
				break
			}
		}
	}
	if !envLoaded && os.Getenv("MAGICPODCAST_SKIP_DOTENV") != "1" {
		logger.Info("ℹ️  No .env file found, using config file values only")
	}

	// 获取配置文件路径
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "./configs/config.yaml"
	}

	// 转换为绝对路径
	absPath, err := filepath.Abs(configPath)
	if err != nil {
		logger.Fatalf("Failed to get absolute path for config: %v", err)
	}

	logger.Info("🚀 MagicPodcast Backend starting...")
	logger.Infof("📝 Loading config from: %s", absPath)

	// 加载配置
	cfg, err := config.Load(absPath)
	if err != nil {
		logger.Fatalf("Failed to load config: %v", err)
	}
	if err := cfg.AssertManagedProfileSafe(); err != nil {
		logger.Fatalf("Managed data profile configuration failed closed: %v", err)
	}
	profileMetadata, err := runtimeprofile.Load(cfg.Database.Path, cfg.Server.Mode)
	if err != nil {
		logger.Fatalf("Data profile validation failed: %v", err)
	}

	// 打印配置信息
	logger.Info("✅ Config loaded successfully")
	logger.Infof("   Server Mode: %s", cfg.Server.Mode)
	logger.Infof("   Server Port: %d", cfg.Server.Port)
	logger.Infof("   Data Profile: %s", profileMetadata.Profile)
	logger.Infof("   XYZ API: %s", cfg.XYZAPI.URL)

	// Apply the startup-loaded Feed fetcher / coordinator configuration to the
	// process-wide coordinator and the shared Fetcher HTTP behavior. This runs
	// exactly once, before any Feed fetch and before router setup, so the
	// configured User-Agent, layered timeouts, honest headers, circuit tuning,
	// snapshot bounds, and configured-egress tag are in effect when the
	// workflow first builds its Fetcher. There is no hot reload: changes
	// require a restart.
	if err := feed.ConfigureSharedRuntime(cfg.Feed); err != nil {
		logger.Fatalf("Failed to apply feed config: %v", err)
	}

	// 初始化日志系统
	logger.Info("\n📝 Initializing logger...")
	logger.Init(cfg.Logging.Level, cfg.Logging.Output, cfg.Server.Mode)

	// 初始化数据库
	logger.Info("\n📊 Initializing database...")
	db := database.GetDB() // 初始化数据库连接
	defer database.Close()

	// 普通服务启动只读检查迁移状态，不静默改写数据库结构。结构变化
	// 必须先通过 scripts/migrate-db.sh 显式执行并记录版本。
	if err := database.RequireSchemaReady(db); err != nil {
		logger.Fatalf("Database schema is not ready: %v", err)
	}

	// 设置路由
	logger.Info("🔧 Setting up routes...")
	var (
		routerOptions       []router.Option
		processingWorker    *processing.Worker
		processingRuntime   codexruntime.Runtime
		processingCancel    context.CancelFunc
		processingWorkerErr chan error
		processingStopped   bool
	)
	if cfg.Processing.Enabled {
		audioStore, err := processing.NewDiskAudioStore(db, cfg.Processing.AudioRoot)
		if err != nil {
			logger.Fatalf("Failed to initialize managed audio: %v", err)
		}
		inputResolver, err := processing.NewManagedAudioInputResolver(
			audioStore,
			cfg.Processing.PipelineVersion,
		)
		if err != nil {
			logger.Fatalf("Failed to initialize processing input resolver: %v", err)
		}
		artifactStore, err := processing.NewDiskArtifactStore(
			cfg.Processing.ArtifactRoot,
		)
		if err != nil {
			logger.Fatalf("Failed to initialize processing artifacts: %v", err)
		}
		if err := os.MkdirAll(cfg.Processing.Runtime.WorkRoot, 0o700); err != nil {
			logger.Fatalf("Failed to initialize Codex Runtime work root: %v", err)
		}
		if err := os.Chmod(cfg.Processing.Runtime.WorkRoot, 0o700); err != nil {
			logger.Fatalf("Failed to protect Codex Runtime work root: %v", err)
		}
		runtimeHost, err := codexruntime.NewProcessHost(
			codexruntime.ProcessHostConfig{
				Command: []string{
					cfg.Processing.Runtime.Python,
					cfg.Processing.Runtime.HostScript,
				},
				WorkRoot: cfg.Processing.Runtime.WorkRoot,
				Profiles: codexruntime.DefaultProfiles(),
			},
		)
		if err != nil {
			logger.Fatalf("Failed to initialize Codex Runtime Host: %v", err)
		}
		processingRuntime = runtimeHost
		runtimeAdapter, err := processing.NewLocalRuntimeAdapter(
			runtimeHost,
			cfg.Processing.Runtime.WorkRoot,
		)
		if err != nil {
			logger.Fatalf("Failed to initialize processing Runtime adapter: %v", err)
		}
		minutesAdapter, err := processing.NewFeishuMinutesAdapter(
			cfg.Processing.LarkCLI,
			cfg.Processing.LarkWorkRoot,
			func(
				ctx context.Context,
				episodeID uint,
			) (string, string, error) {
				audio, resolveErr := audioStore.ResolveReadyAudio(ctx, episodeID)
				return audio.Path, audio.SHA256, resolveErr
			},
		)
		if err != nil {
			logger.Fatalf("Failed to initialize Feishu Minutes adapter: %v", err)
		}
		processingService := processing.NewService(
			db,
			processing.WithProcessingInputResolver(inputResolver),
			processing.WithAudioPreparer(audioStore),
			processing.WithArtifactReader(artifactStore),
		)
		engine, err := processing.NewEngine(
			processingService,
			minutesAdapter,
			runtimeAdapter,
			artifactStore,
			nil,
		)
		if err != nil {
			logger.Fatalf("Failed to initialize processing engine: %v", err)
		}
		processingWorker, err = processing.NewWorker(
			processingService,
			engine,
			audioStore,
			processing.WorkerConfig{
				ScanInterval:         cfg.Processing.WorkerScanInterval,
				ExternalPollInterval: cfg.Processing.ExternalPollInterval,
				BatchSize:            cfg.Processing.WorkerBatchSize,
			},
		)
		if err != nil {
			logger.Fatalf("Failed to initialize processing worker: %v", err)
		}
		copilotContextLoader, err := episodecopilot.NewGORMContextLoader(
			db,
			artifactStore,
		)
		if err != nil {
			logger.Fatalf("Failed to initialize episode Copilot context: %v", err)
		}
		episodeCopilot, err := episodecopilot.NewService(
			copilotContextLoader,
			runtimeHost,
			cfg.Processing.Runtime.WorkRoot,
		)
		if err != nil {
			logger.Fatalf("Failed to initialize episode Copilot: %v", err)
		}
		routerOptions = append(
			routerOptions,
			router.WithProcessingModule(
				processingService,
				processingWorker.Canceler(),
			),
			router.WithEpisodeCopilotModule(episodeCopilot),
		)
	}
	r := router.SetupRouter(routerOptions...)

	if processingWorker != nil {
		var workerContext context.Context
		workerContext, processingCancel = context.WithCancel(context.Background())
		processingWorkerErr = make(chan error, 1)
		go func() {
			processingWorkerErr <- processingWorker.Run(workerContext)
		}()
	}

	listenAddress := net.JoinHostPort(cfg.Server.Host, strconv.Itoa(cfg.Server.Port))

	// 创建 HTTP 服务器
	srv := &http.Server{
		Addr:         listenAddress,
		Handler:      r,
		ReadTimeout:  time.Duration(cfg.Server.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(cfg.Server.WriteTimeout) * time.Second,
	}

	// 在 goroutine 中启动服务器
	go func() {
		logger.Info("\n🎉 Server started successfully!")
		logger.Infof("   Listening on: http://%s", listenAddress)
		logger.Infof("   Health check: http://%s/health", listenAddress)
		logger.Infof("   API endpoint: http://%s/api/v1/podcasts\n", listenAddress)

		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatalf("Failed to start server: %v", err)
		}
	}()

	// 优雅关闭
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	if processingWorkerErr == nil {
		<-quit
	} else {
		select {
		case <-quit:
		case workerErr := <-processingWorkerErr:
			processingStopped = true
			if workerErr != nil {
				logger.Errorf("Processing worker stopped: %v", workerErr)
			}
		}
	}

	logger.Info("\n🛑 Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Fatalf("Server forced to shutdown: %v", err)
	}
	if processingCancel != nil {
		processingCancel()
		if !processingStopped {
			select {
			case workerErr := <-processingWorkerErr:
				if workerErr != nil {
					logger.Errorf("Processing worker shutdown failed: %v", workerErr)
				}
			case <-time.After(10 * time.Second):
				logger.Error("Processing worker shutdown timed out")
			}
		}
	}
	if processingRuntime != nil {
		runtimeCtx, runtimeCancel := context.WithTimeout(
			context.Background(),
			10*time.Second,
		)
		if err := processingRuntime.Close(runtimeCtx); err != nil {
			logger.Errorf("Codex Runtime shutdown failed: %v", err)
		}
		runtimeCancel()
	}

	logger.Info("✅ Server exited gracefully")
}
