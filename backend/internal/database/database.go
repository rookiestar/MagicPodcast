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

const defaultSQLiteBusyTimeoutMS = 5000

const minSQLiteBusyTimeoutMS = 100

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

	// 配置 GORM。生产启动只负责打开并校验连接，不在这里执行结构迁移。
	gormConfig := &gorm.Config{
		// 迁移只在显式迁移命令中运行；显式迁移也应保留模型外键约束。
		DisableForeignKeyConstraintWhenMigrating: false,
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

	// SQLite 的 PRAGMA 大多是“每个连接”生效，必须放入 DSN，避免连接池
	// 新建连接后退回默认值。单用户场景固定单连接，WAL 和 busy timeout
	// 负责降低后台任务之间的锁竞争。
	busyTimeoutMS := cfg.Database.BusyTimeoutMS
	if busyTimeoutMS < minSQLiteBusyTimeoutMS {
		busyTimeoutMS = defaultSQLiteBusyTimeoutMS
	}
	dsn := fmt.Sprintf("%s?_journal_mode=WAL&_foreign_keys=on&_busy_timeout=%d", dbPath, busyTimeoutMS)
	db, err := gorm.Open(sqlite.Open(dsn), gormConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// 获取通用数据库对象 sql.DB
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get database instance: %w", err)
	}

	// SQLite 单文件、单用户运行时固定一个连接，确保外键和 busy timeout
	// 的语义一致，也避免多个写连接互相放大锁竞争。
	maxOpenConns := cfg.Database.MaxOpenConns
	if maxOpenConns <= 0 || maxOpenConns > 1 {
		if maxOpenConns > 1 {
			logger.Warnf("SQLite max_open_conns=%d is unsafe for this single-file runtime; clamping to 1", maxOpenConns)
		}
		maxOpenConns = 1
	}
	maxIdleConns := cfg.Database.MaxIdleConns
	if maxIdleConns <= 0 || maxIdleConns > maxOpenConns {
		maxIdleConns = maxOpenConns
	}
	sqlDB.SetMaxIdleConns(maxIdleConns)
	sqlDB.SetMaxOpenConns(maxOpenConns)
	sqlDB.SetConnMaxLifetime(time.Duration(cfg.Database.ConnMaxLifetime) * time.Second)
	sqlDB.SetConnMaxIdleTime(5 * time.Minute) // 空闲连接超时：防止连接堆积

	if err := VerifySQLiteSettings(db); err != nil {
		return nil, err
	}
	logger.Infof("✅ Database connected (journal_mode=WAL, foreign_keys=ON, busy_timeout=%dms, max_open_conns=%d)", busyTimeoutMS, maxOpenConns)

	return db, nil
}

// VerifySQLiteSettings verifies the per-connection SQLite safety settings.
// The DSN applies these settings to every pooled connection; this query also
// proves that the first runtime connection received the expected values.
func VerifySQLiteSettings(database *gorm.DB) error {
	if database == nil {
		return fmt.Errorf("database is nil")
	}

	var foreignKeys int
	if err := database.Raw("PRAGMA foreign_keys").Row().Scan(&foreignKeys); err != nil {
		return fmt.Errorf("failed to read foreign_keys pragma: %w", err)
	}
	if foreignKeys != 1 {
		return fmt.Errorf("sqlite foreign_keys pragma is %d, want 1", foreignKeys)
	}

	var busyTimeout int
	if err := database.Raw("PRAGMA busy_timeout").Row().Scan(&busyTimeout); err != nil {
		return fmt.Errorf("failed to read busy_timeout pragma: %w", err)
	}
	if busyTimeout < minSQLiteBusyTimeoutMS {
		return fmt.Errorf("sqlite busy_timeout is %dms, want at least %dms", busyTimeout, minSQLiteBusyTimeoutMS)
	}

	if sqlDB, err := database.DB(); err == nil {
		if stats := sqlDB.Stats(); stats.MaxOpenConnections != 1 {
			return fmt.Errorf("sqlite max open connections is %d, want 1", stats.MaxOpenConnections)
		}
	}
	return nil
}

// EnableForeignKeys is retained for explicit maintenance callers. Runtime
// connections already enable the pragma through the DSN; this function now
// verifies the invariant instead of changing only one pooled connection.
func EnableForeignKeys() error {
	if db == nil {
		return fmt.Errorf("database not initialized")
	}
	return VerifySQLiteSettings(db)
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
