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
	"magicpodcast/internal/config"
	"magicpodcast/internal/database"
	"magicpodcast/internal/feed"
	"magicpodcast/internal/logger"
	"magicpodcast/internal/router"
)

func main() {
	// 加载 .env 文件（如果存在）
	// 优先从当前目录查找，然后从上级目录查找
	envPaths := []string{".env", "../.env", "../../.env"}
	envLoaded := false
	for _, envPath := range envPaths {
		if err := godotenv.Load(envPath); err == nil {
			logger.Infof("✅ Loaded .env from: %s", envPath)
			envLoaded = true
			break
		}
	}
	if !envLoaded {
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

	// 打印配置信息
	logger.Info("✅ Config loaded successfully")
	logger.Infof("   Server Mode: %s", cfg.Server.Mode)
	logger.Infof("   Server Port: %d", cfg.Server.Port)
	logger.Infof("   Database: %s", cfg.Database.Path)
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
	r := router.SetupRouter()

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
	<-quit

	logger.Info("\n🛑 Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Fatalf("Server forced to shutdown: %v", err)
	}

	logger.Info("✅ Server exited gracefully")
}
