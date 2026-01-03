package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"magicpodcast/internal/config"
	"magicpodcast/internal/database"
	"magicpodcast/internal/router"
)

func main() {
	// 获取配置文件路径
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "./configs/config.yaml"
	}

	// 转换为绝对路径
	absPath, err := filepath.Abs(configPath)
	if err != nil {
		log.Fatalf("Failed to get absolute path for config: %v", err)
	}

	fmt.Println("🚀 MagicPodcast Backend starting...")
	fmt.Printf("📝 Loading config from: %s\n", absPath)

	// 加载配置
	cfg, err := config.Load(absPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// 打印配置信息
	fmt.Println("✅ Config loaded successfully")
	fmt.Printf("   Server Mode: %s\n", cfg.Server.Mode)
	fmt.Printf("   Server Port: %d\n", cfg.Server.Port)
	fmt.Printf("   Database: %s\n", cfg.Database.Path)
	fmt.Printf("   XYZ API: %s\n", cfg.XYZAPI.URL)

	// 初始化数据库
	fmt.Println("\n📊 Initializing database...")
	db := database.GetDB() // 初始化数据库连接
	defer database.Close()

	// 运行数据库迁移
	if err := database.AutoMigrate(db); err != nil {
		log.Fatalf("Failed to run database migrations: %v", err)
	}

	// 创建自定义索引
	if err := database.CreateIndexes(db); err != nil {
		log.Fatalf("Failed to create custom indexes: %v", err)
	}

	// 设置路由
	fmt.Println("🔧 Setting up routes...")
	r := router.SetupRouter()

	// 创建 HTTP 服务器
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:      r,
		ReadTimeout:  time.Duration(cfg.Server.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(cfg.Server.WriteTimeout) * time.Second,
	}

	// 在 goroutine 中启动服务器
	go func() {
		fmt.Printf("\n🎉 Server started successfully!\n")
		fmt.Printf("   Listening on: http://localhost:%d\n", cfg.Server.Port)
		fmt.Printf("   Health check: http://localhost:%d/health\n", cfg.Server.Port)
		fmt.Printf("   API endpoint: http://localhost:%d/api/v1/podcasts\n\n", cfg.Server.Port)

		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// 优雅关闭
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	fmt.Println("\n🛑 Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	fmt.Println("✅ Server exited gracefully")
}
