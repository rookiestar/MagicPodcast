package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"

	"magicpodcast/internal/config"
	"magicpodcast/internal/database"
	"magicpodcast/internal/logger"
)

const migrationConfirmation = "I_UNDERSTAND_THIS_WRITES_DATA"

func main() {
	preflight := flag.Bool("preflight", false, "在已验证备份的隔离副本上执行影子迁移")
	dryRun := flag.Bool("dry-run", false, "--preflight 的兼容别名")
	apply := flag.Bool("apply", false, "应用待执行的版本化迁移")
	flag.Parse()

	modes := 0
	if *preflight || *dryRun {
		modes++
	}
	if *apply {
		modes++
	}
	if modes != 1 || (*preflight && *dryRun) {
		logger.Fatalf("必须且只能指定 --preflight（或 --dry-run 兼容别名）或 --apply")
	}

	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "./configs/config.yaml"
	}
	absPath, err := filepath.Abs(configPath)
	if err != nil {
		logger.Fatalf("Failed to resolve migration config path")
	}
	if _, err := config.Load(absPath); err != nil {
		logger.Fatalf("Failed to load migration config")
	}

	if *preflight || *dryRun {
		db, closeDB, err := database.OpenMigrationPreflightSource(config.Get().Database.Path)
		if err != nil {
			logger.Fatalf("Failed to open migration source read-only")
		}
		defer closeDB()
		status, err := database.InspectSchema(db)
		if err != nil {
			logger.Fatalf("Failed to inspect schema: %v", err)
		}
		printStatus(status)
		backup := os.Getenv("MAGICPODCAST_MIGRATION_BACKUP")
		if backup == "" {
			logger.Fatalf("迁移 preflight 需要 MAGICPODCAST_MIGRATION_BACKUP")
		}
		reportPath := os.Getenv("MAGICPODCAST_MIGRATION_REPORT")
		if reportPath == "" {
			logger.Fatalf("迁移 preflight 需要 MAGICPODCAST_MIGRATION_REPORT")
		}
		targetCommit, err := resolveTargetCommit()
		if err != nil {
			logger.Fatalf("Failed to resolve target commit: %v", err)
		}
		report, preflightErr := database.PreflightProductionMigrations(db, database.MigrationPreflightOptions{
			BackupPath: backup, TargetCommit: targetCommit,
		})
		if err := database.WriteMigrationReport(reportPath, report); err != nil {
			logger.Fatalf("Failed to write Migration Report")
		}
		printMigrationReportSummary(report)
		if preflightErr != nil {
			logger.Fatalf("Migration preflight failed: %s: %s", report.Result.FailureCode, report.Result.FailureDetail)
		}
		return
	}
	if err := requireMigrationMaintenanceWindow(); err != nil {
		logger.Fatalf("拒绝执行真实迁移：必须由共享 migration 维护窗口调用")
	}

	db := database.GetDB()
	defer database.Close()
	status, err := database.InspectSchema(db)
	if err != nil {
		logger.Fatalf("Failed to inspect schema: %v", err)
	}
	printStatus(status)

	if os.Getenv("MAGICPODCAST_MIGRATION_CONFIRM") != migrationConfirmation {
		logger.Fatalf("拒绝执行真实迁移：请设置 MAGICPODCAST_MIGRATION_CONFIRM=%s", migrationConfirmation)
	}
	backup := os.Getenv("MAGICPODCAST_MIGRATION_BACKUP")
	if backup == "" {
		logger.Fatalf("拒绝执行真实迁移：MAGICPODCAST_MIGRATION_BACKUP 未设置")
	}
	if info, err := os.Stat(backup); err != nil || info.IsDir() {
		logger.Fatalf("拒绝执行真实迁移：备份文件不可用")
	}
	reportPath := os.Getenv("MAGICPODCAST_MIGRATION_REPORT")
	if reportPath == "" {
		logger.Fatalf("拒绝执行真实迁移：MAGICPODCAST_MIGRATION_REPORT 未设置")
	}
	report, err := database.ReadMigrationReport(reportPath)
	if err != nil {
		logger.Fatalf("拒绝执行真实迁移：Migration Report 无效")
	}
	targetCommit, err := resolveTargetCommit()
	if err != nil {
		logger.Fatalf("Failed to resolve target commit: %v", err)
	}
	result, applyErr := database.ApplyProductionMigrationReport(db, report, database.MigrationApplyOptions{
		BackupPath: backup, TargetCommit: targetCommit,
	})
	report.Apply = &result
	printMigrationApplySummary(report, result)
	if err := database.WriteMigrationReport(reportPath, report); err != nil {
		logger.Fatalf("Failed to update Migration Report after apply")
	}
	if applyErr != nil {
		logger.Fatalf("Migration apply failed: %s: %s", result.FailureCode, result.FailureDetail)
	}
}

func requireMigrationMaintenanceWindow() error {
	lockDir := strings.TrimSpace(os.Getenv("MAGICPODCAST_DEPLOY_LOCK_DIR"))
	if lockDir == "" {
		lockDir = "/tmp/magicpodcast-production-deploy.lock"
	}
	if !filepath.IsAbs(lockDir) {
		return fmt.Errorf("maintenance lock must be absolute")
	}
	read := func(name string) (string, error) {
		content, err := os.ReadFile(filepath.Join(lockDir, name))
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(content)), nil
	}
	ownerPID, err := read("owner.pid")
	if err != nil || ownerPID == "" || ownerPID != strings.TrimSpace(os.Getenv("MAGICPODCAST_MAINTENANCE_OWNER_PID")) {
		return fmt.Errorf("maintenance owner mismatch")
	}
	pid, err := strconv.Atoi(ownerPID)
	if err != nil || pid < 1 || syscall.Kill(pid, 0) != nil {
		return fmt.Errorf("maintenance owner is not alive")
	}
	ownerStart, err := read("owner.started_at")
	if err != nil || ownerStart != strings.TrimSpace(os.Getenv("MAGICPODCAST_MAINTENANCE_OWNER_START")) {
		return fmt.Errorf("maintenance owner start mismatch")
	}
	currentStart, err := exec.Command("ps", "-p", ownerPID, "-o", "lstart=").Output()
	if err != nil || strings.TrimSpace(string(currentStart)) != ownerStart {
		return fmt.Errorf("maintenance owner process was replaced")
	}
	operation, err := read("operation")
	if err != nil || operation != "migration" {
		return fmt.Errorf("maintenance operation mismatch")
	}
	state, err := read("state")
	if err != nil || state != "critical" {
		return fmt.Errorf("maintenance state mismatch")
	}
	return nil
}

func resolveTargetCommit() (string, error) {
	output, err := exec.Command("git", "rev-parse", "--verify", "HEAD").Output()
	if err != nil {
		return "", errors.New("target checkout commit is unavailable")
	}
	head := strings.ToLower(strings.TrimSpace(string(output)))
	if len(head) != 40 {
		return "", errors.New("target checkout commit must be a full SHA")
	}
	status, err := exec.Command("git", "status", "--porcelain=v1", "--untracked-files=normal").Output()
	if err != nil {
		return "", errors.New("target checkout status is unavailable")
	}
	if strings.TrimSpace(string(status)) != "" {
		return "", errors.New("target checkout must be clean")
	}
	requested := strings.ToLower(strings.TrimSpace(os.Getenv("MAGICPODCAST_TARGET_COMMIT")))
	if requested != "" && requested != head {
		return "", errors.New("target commit does not match checkout HEAD")
	}
	return head, nil
}

func printMigrationReportSummary(report database.MigrationReport) {
	fmt.Printf("migration_report_version=%s\n", report.ReportVersion)
	fmt.Printf("migration_plan_id=%s\n", report.PlanID)
	fmt.Printf("migration_preflight_status=%s\n", report.Result.Status)
	fmt.Printf("migration_apply_eligible=%t\n", report.Result.ApplyEligible)
	fmt.Printf("source_schema_version=%d\n", report.SourceSchemaVersion)
	fmt.Printf("target_schema_version=%d\n", report.TargetSchemaVersion)
	fmt.Printf("pending_migration_count=%d\n", len(report.PendingMigrations))
	if report.Result.FailureCode != "" {
		fmt.Printf("migration_failure_code=%s\n", report.Result.FailureCode)
	}
}

func printMigrationApplySummary(report database.MigrationReport, result database.MigrationApplyResult) {
	fmt.Printf("migration_plan_id=%s\n", report.PlanID)
	fmt.Printf("migration_apply_status=%s\n", result.Status)
	fmt.Printf("migration_database_committed=%t\n", result.DatabaseCommitted)
	fmt.Printf("migration_schema_version=%d\n", result.SchemaVersion)
	fmt.Printf("migration_key_projection_readable=%t\n", result.KeyProjectionReadable)
	if result.KeyProjectionReadable {
		for _, state := range []string{"inbox", "focus", "someday", "done"} {
			fmt.Printf("migration_queue_%s_count=%d\n", state, result.QueueCounts[state])
		}
		statuses := make([]string, 0, len(result.ProcessingStatusCounts))
		for status := range result.ProcessingStatusCounts {
			statuses = append(statuses, status)
		}
		sort.Strings(statuses)
		for _, status := range statuses {
			fmt.Printf("migration_processing_%s_count=%d\n", status, result.ProcessingStatusCounts[status])
		}
	}
	if result.FailureCode != "" {
		fmt.Printf("migration_failure_code=%s\n", result.FailureCode)
	}
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
