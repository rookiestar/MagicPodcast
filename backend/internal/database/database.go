package database

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"magicpodcast/internal/config"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var (
	db   *gorm.DB
	once sync.Once
)

// GetDB 获取数据库实例（单例模式）
func GetDB() *gorm.DB {
	once.Do(func() {
		var err error
		db, err = initDB()
		if err != nil {
			panic(fmt.Sprintf("Failed to initialize database: %v", err))
		}
	})
	return db
}

// initDB 初始化数据库连接
func initDB() (*gorm.DB, error) {
	cfg := config.Get()

	// 确保数据库目录存在
	dbPath := cfg.Database.Path
	dbDir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	// 配置 GORM
	gormConfig := &gorm.Config{
		// 禁用外键约束（SQLite 的外键支持有限）
		DisableForeignKeyConstraintWhenMigrating: true,
		// 跳过默认事务（提升性能）
		SkipDefaultTransaction: true,
		// 禁用 RETURNING 子句（SQLite 驱动兼容性问题）
		DisableAutomaticPing: false,
	}

	// 禁用 RETURNING 子句，避免 SQLite 扫描错误
	// 参考: https://github.com/mattn/go-sqlite3/issues/804
	gormConfig.SkipDefaultTransaction = true

	// 配置日志级别
	logLevel := logger.Silent
	if cfg.Database.Debug {
		logLevel = logger.Info
	}
	gormConfig.Logger = logger.Default.LogMode(logLevel)

	// 打开数据库连接
	db, err := gorm.Open(sqlite.Open(dbPath), gormConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// 获取通用数据库对象 sql.DB
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get database instance: %w", err)
	}

	// 设置连接池参数
	sqlDB.SetMaxIdleConns(cfg.Database.MaxIdleConns)
	sqlDB.SetMaxOpenConns(cfg.Database.MaxOpenConns)
	sqlDB.SetConnMaxLifetime(time.Duration(cfg.Database.ConnMaxLifetime) * time.Second)

	fmt.Printf("✅ Database connected: %s\n", dbPath)

	return db, nil
}

// Close 关闭数据库连接
func Close() error {
	if db == nil {
		return nil
	}

	sqlDB, err := db.DB()
	if err != nil {
		return err
	}

	return sqlDB.Close()
}
