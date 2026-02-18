package database

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"magicpodcast/internal/config"
	"magicpodcast/internal/logger"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

var (
	db   *gorm.DB
	once sync.Once
)

// GetDB 获取数据库实例（单例模式）
func GetDB() *gorm.DB {
	// 如果 db 已经设置（例如通过 SetTestDB），直接返回
	if db != nil {
		return db
	}
	once.Do(func() {
		// 再次检查，避免竞态条件
		if db != nil {
			return
		}
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
		// 禁用迁移时的外键约束检查（避免迁移时重建表导致数据丢失）
		// 外键约束在运行时通过 PRAGMA foreign_keys = ON 启用
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
	logLevel := gormlogger.Silent
	if cfg.Database.Debug {
		logLevel = gormlogger.Info
	}
	gormConfig.Logger = gormlogger.Default.LogMode(logLevel)

	// 打开数据库连接（不在 DSN 中启用外键，避免迁移时触发 CASCADE DELETE）
	// _journal_mode=WAL: 使用 WAL 模式提升并发性能
	// 注意：外键约束将在迁移完成后通过 PRAGMA 启用
	dsn := fmt.Sprintf("%s?_journal_mode=WAL", dbPath)
	db, err := gorm.Open(sqlite.Open(dsn), gormConfig)
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
	sqlDB.SetConnMaxIdleTime(5 * time.Minute) // 空闲连接超时：防止连接堆积

	// 注意：不在此处启用外键约束，避免迁移时触发 CASCADE DELETE
	// 外键约束将在 EnableForeignKeys() 中启用，该方法应在迁移完成后调用
	logger.Infof("✅ Database connected: %s (journal_mode=WAL, foreign_keys=OFF during migration)", dbPath)

	return db, nil
}

// EnableForeignKeys 启用外键约束（应在迁移完成后调用）
func EnableForeignKeys() error {
	if db == nil {
		return fmt.Errorf("database not initialized")
	}

	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("failed to get database instance: %w", err)
	}

	if _, err := sqlDB.Exec("PRAGMA foreign_keys = ON"); err != nil {
		return fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	logger.Info("✅ Foreign keys enabled (PRAGMA foreign_keys = ON)")
	return nil
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

// SetTestDB 设置测试数据库（仅用于测试）
// 使用此函数可以在测试中注入内存数据库
func SetTestDB(testDB *gorm.DB) {
	db = testDB
	// 注意：不重置 once，这样 GetDB 不会再调用 initDB
	// once 已经执行过，db 已设置
}

// ResetDB 重置数据库实例（仅用于测试后清理）
func ResetDB() {
	db = nil
	// 重置 once 以允许下次正常初始化
	once = sync.Once{}
}
