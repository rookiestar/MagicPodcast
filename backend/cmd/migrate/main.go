package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"magicpodcast/internal/config"
	"magicpodcast/internal/database"
	"magicpodcast/internal/logger"
)

const migrationConfirmation = "I_UNDERSTAND_THIS_WRITES_DATA"

func main() {
	dryRun := flag.Bool("dry-run", false, "只读取并展示迁移计划")
	apply := flag.Bool("apply", false, "应用待执行的版本化迁移")
	flag.Parse()

	if *dryRun == *apply {
		logger.Fatalf("必须且只能指定 --dry-run 或 --apply")
	}

	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "./configs/config.yaml"
	}
	absPath, err := filepath.Abs(configPath)
	if err != nil {
		logger.Fatalf("Failed to resolve config path: %v", err)
	}
	if _, err := config.Load(absPath); err != nil {
		logger.Fatalf("Failed to load config: %v", err)
	}

	db := database.GetDB()
	defer database.Close()

	status, err := database.InspectSchema(db)
	if err != nil {
		logger.Fatalf("Failed to inspect schema: %v", err)
	}
	printStatus(status)

	if *dryRun {
		return
	}

	if os.Getenv("MAGICPODCAST_MIGRATION_CONFIRM") != migrationConfirmation {
		logger.Fatalf("拒绝执行真实迁移：请设置 MAGICPODCAST_MIGRATION_CONFIRM=%s", migrationConfirmation)
	}
	backup := os.Getenv("MAGICPODCAST_MIGRATION_BACKUP")
	if backup == "" {
		logger.Fatalf("拒绝执行真实迁移：MAGICPODCAST_MIGRATION_BACKUP 未设置")
	}
	if info, err := os.Stat(backup); err != nil || info.IsDir() {
		logger.Fatalf("拒绝执行真实迁移：备份文件不可用: %s", backup)
	}

	if err := database.ApplyMigrations(db); err != nil {
		logger.Fatalf("Versioned migration failed; transaction rolled back: %v", err)
	}
	if err := database.RequireSchemaReady(db); err != nil {
		logger.Fatalf("Schema verification failed after migration: %v", err)
	}
	fmt.Println("migration_result=ok")
}

func printStatus(status database.SchemaStatus) {
	fmt.Printf("migration_table_present=%t\n", status.MigrationTablePresent)
	fmt.Printf("current_version=%d\n", status.CurrentVersion)
	fmt.Printf("required_tables_missing=%v\n", status.RequiredTablesMissing)
	if len(status.Pending) == 0 {
		fmt.Println("pending_migrations=none")
		return
	}
	for _, migration := range status.Pending {
		fmt.Printf("pending_migration=%d:%s:%s\n", migration.Version, migration.Name, migration.Description)
	}
}
