// Command repair_podcast_summaries audits and repairs the podcast summary
// fields that are derived from active episodes.
//
// The command is intentionally narrow: it only changes episode_count and
// newest_episode_date. It never uses fetch time or updated_date to determine
// the most recent episode.
package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

const (
	defaultDBPath     = "data/magicpodcast.db"
	applyConfirmation = "I_UNDERSTAND_THIS_WRITES_PODCAST_SUMMARIES"
)

type config struct {
	dbPath     string
	reportPath string
	backupPath string
	apply      bool
	confirm    string
}

type auditSummary struct {
	PodcastsScanned             int `json:"podcasts_scanned"`
	MismatchedPodcasts          int `json:"mismatched_podcasts"`
	EpisodeCountMismatches      int `json:"episode_count_mismatches"`
	NewestEpisodeDateMismatches int `json:"newest_episode_date_mismatches"`
}

type podcastRepair struct {
	PodcastID                 int64   `json:"podcast_id"`
	PodcastTitle              string  `json:"podcast_title"`
	CurrentEpisodeCount       int64   `json:"current_episode_count"`
	ExpectedEpisodeCount      int64   `json:"expected_episode_count"`
	CurrentNewestEpisodeDate  *string `json:"current_newest_episode_date"`
	ExpectedNewestEpisodeDate *string `json:"expected_newest_episode_date"`
}

type runReport struct {
	GeneratedAt     string          `json:"generated_at"`
	Mode            string          `json:"mode"`
	DatabasePath    string          `json:"database_path"`
	BackupPath      string          `json:"backup_path,omitempty"`
	BackupSizeBytes int64           `json:"backup_size_bytes,omitempty"`
	BackupSHA256    string          `json:"backup_sha256,omitempty"`
	Before          auditSummary    `json:"before"`
	PlannedChanges  []podcastRepair `json:"planned_changes,omitempty"`
	AppliedPodcasts int             `json:"applied_podcasts"`
	After           auditSummary    `json:"after"`
	Verified        bool            `json:"verified"`
	Error           string          `json:"error,omitempty"`
}

func main() {
	cfg, err := parseConfig(os.Args[1:])
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		exitf("参数错误: %v", err)
	}

	report := runReport{
		GeneratedAt:  time.Now().UTC().Format(time.RFC3339Nano),
		Mode:         modeName(cfg.apply),
		DatabasePath: cfg.dbPath,
	}
	report, runErr := run(cfg, report)
	if runErr != nil {
		report.Error = runErr.Error()
	}

	if err := writeReport(cfg.reportPath, report); err != nil {
		if runErr == nil {
			runErr = fmt.Errorf("写入审计报告失败: %w", err)
		} else {
			fmt.Fprintf(os.Stderr, "写入审计报告失败: %v\n", err)
		}
	}

	printReport(cfg.reportPath, report)
	if runErr != nil {
		exitf("播客汇总修复失败: %v", runErr)
	}
}

func parseConfig(args []string) (config, error) {
	cfg := config{dbPath: defaultDBPath}
	fs := flag.NewFlagSet("repair_podcast_summaries", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.StringVar(&cfg.dbPath, "db", cfg.dbPath, "SQLite database path")
	fs.StringVar(&cfg.reportPath, "report", "", "JSON audit report path")
	fs.StringVar(&cfg.backupPath, "backup", "", "backup path used by --apply; defaults to data/backups/")
	fs.BoolVar(&cfg.apply, "apply", false, "apply the planned summary repairs")
	fs.StringVar(&cfg.confirm, "confirm", "", "required confirmation for --apply")
	if err := fs.Parse(args); err != nil {
		return cfg, err
	}
	if fs.NArg() != 0 {
		return cfg, fmt.Errorf("不支持的位置参数: %s", strings.Join(fs.Args(), " "))
	}
	if err := validateApply(cfg.apply, cfg.confirm); err != nil {
		return cfg, err
	}

	stamp := time.Now().UTC().Format("20060102T150405Z")
	if cfg.reportPath == "" {
		cfg.reportPath = filepath.Join("data", "audit", "podcast_summary_repair_"+stamp+".json")
	}
	if cfg.apply && cfg.backupPath == "" {
		cfg.backupPath = filepath.Join("data", "backups", "magicpodcast_summary_repair_"+stamp+".db")
	}
	if !cfg.apply && cfg.backupPath != "" {
		return cfg, fmt.Errorf("--backup 只能与 --apply 一起使用")
	}
	return cfg, nil
}

func validateApply(apply bool, confirmation string) error {
	if !apply {
		return nil
	}
	if confirmation != applyConfirmation {
		return fmt.Errorf("--apply 必须同时提供 --confirm %s", applyConfirmation)
	}
	return nil
}

func modeName(apply bool) string {
	if apply {
		return "apply"
	}
	return "dry-run"
}

func run(cfg config, report runReport) (runReport, error) {
	dbPath, err := filepath.Abs(cfg.dbPath)
	if err != nil {
		return report, fmt.Errorf("解析数据库路径失败: %w", err)
	}
	report.DatabasePath = dbPath

	db, err := openDB(dbPath, !cfg.apply)
	if err != nil {
		return report, fmt.Errorf("打开数据库失败: %w", err)
	}
	defer db.Close()

	plan, before, err := loadRepairPlan(db)
	if err != nil {
		return report, fmt.Errorf("体检数据库失败: %w", err)
	}
	report.Before = before
	report.PlannedChanges = plan

	if !cfg.apply {
		report.After = before
		report.Verified = before.MismatchedPodcasts == 0
		return report, nil
	}

	backupPath, err := filepath.Abs(cfg.backupPath)
	if err != nil {
		return report, fmt.Errorf("解析备份路径失败: %w", err)
	}
	if filepath.Clean(backupPath) == filepath.Clean(dbPath) {
		return report, errors.New("备份路径不能与数据库路径相同")
	}
	report.BackupPath = backupPath
	if err := backupDatabase(db, backupPath); err != nil {
		return report, fmt.Errorf("创建并校验备份失败: %w", err)
	}
	if report.BackupSizeBytes, report.BackupSHA256, err = describeFile(backupPath); err != nil {
		return report, fmt.Errorf("读取备份校验信息失败: %w", err)
	}

	applied, err := applyRepairs(db, plan)
	if err != nil {
		return report, fmt.Errorf("应用汇总修复失败: %w", err)
	}
	report.AppliedPodcasts = applied

	_, after, err := loadRepairPlan(db)
	if err != nil {
		return report, fmt.Errorf("修复后复核失败: %w", err)
	}
	report.After = after
	report.Verified = after.MismatchedPodcasts == 0
	if !report.Verified {
		return report, fmt.Errorf("修复后仍有 %d 个播客汇总不一致", after.MismatchedPodcasts)
	}
	return report, nil
}

func openDB(path string, queryOnly bool) (*sql.DB, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if _, err := db.Exec("PRAGMA busy_timeout=10000"); err != nil {
		db.Close()
		return nil, err
	}
	if queryOnly {
		if _, err := db.Exec("PRAGMA query_only=ON"); err != nil {
			db.Close()
			return nil, err
		}
	}
	return db, nil
}

func loadRepairPlan(db *sql.DB) ([]podcastRepair, auditSummary, error) {
	rows, err := db.Query(`
		SELECT
			p.id,
			p.title,
			COALESCE(p.episode_count, 0),
			p.newest_episode_date,
			COALESCE(actual.episode_count, 0),
			actual.newest_episode_date
		FROM podcasts p
		LEFT JOIN (
			SELECT
				e.podcast_id,
				COUNT(*) AS episode_count,
				MAX(CASE
					WHEN e.published_date IS NOT NULL
					 AND trim(e.published_date) <> ''
					 AND substr(e.published_date, 1, 10) <> '0001-01-01'
					THEN e.published_date
				END) AS newest_episode_date
			FROM episodes e
			WHERE e.deleted_at IS NULL
			GROUP BY e.podcast_id
		) actual ON actual.podcast_id = p.id
		WHERE p.deleted_at IS NULL
		ORDER BY p.id`)
	if err != nil {
		return nil, auditSummary{}, err
	}
	defer rows.Close()

	plan := make([]podcastRepair, 0)
	audit := auditSummary{}
	for rows.Next() {
		var repair podcastRepair
		var currentCount, expectedCount int64
		var currentDate, expectedDate sql.NullString
		if err := rows.Scan(
			&repair.PodcastID,
			&repair.PodcastTitle,
			&currentCount,
			&currentDate,
			&expectedCount,
			&expectedDate,
		); err != nil {
			return nil, auditSummary{}, err
		}

		repair.CurrentEpisodeCount = currentCount
		repair.ExpectedEpisodeCount = expectedCount
		repair.CurrentNewestEpisodeDate = nullableString(currentDate)
		repair.ExpectedNewestEpisodeDate = normalizedExpectedDate(expectedDate)
		audit.PodcastsScanned++

		countMismatch := currentCount != expectedCount
		dateMismatch := !sameStoredDate(currentDate, repair.ExpectedNewestEpisodeDate)
		if countMismatch {
			audit.EpisodeCountMismatches++
		}
		if dateMismatch {
			audit.NewestEpisodeDateMismatches++
		}
		if countMismatch || dateMismatch {
			audit.MismatchedPodcasts++
			plan = append(plan, repair)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, auditSummary{}, err
	}
	return plan, audit, nil
}

func nullableString(value sql.NullString) *string {
	if !value.Valid || strings.TrimSpace(value.String) == "" {
		return nil
	}
	copy := strings.TrimSpace(value.String)
	return &copy
}

func normalizedExpectedDate(value sql.NullString) *string {
	valueString := nullableString(value)
	if valueString == nil || strings.HasPrefix(*valueString, "0001-01-01") {
		return nil
	}
	canonical := canonicalDate(*valueString)
	return &canonical
}

func sameStoredDate(current sql.NullString, expected *string) bool {
	if expected == nil {
		return !current.Valid || strings.TrimSpace(current.String) == ""
	}
	return current.Valid && canonicalDate(current.String) == canonicalDate(*expected)
}

func canonicalDate(value string) string {
	value = strings.TrimSpace(value)
	for _, layout := range []string{
		time.RFC3339Nano,
		"2006-01-02 15:04:05.999999999Z07:00",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05",
	} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.UTC().Format(time.RFC3339Nano)
		}
	}
	return value
}

func applyRepairs(db *sql.DB, repairs []podcastRepair) (int, error) {
	if len(repairs) == 0 {
		return 0, nil
	}
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	now := time.Now().UTC().Format(time.RFC3339Nano)
	applied := 0
	for _, repair := range repairs {
		var newest any
		if repair.ExpectedNewestEpisodeDate != nil {
			newest = *repair.ExpectedNewestEpisodeDate
		}
		result, err := tx.Exec(`
			UPDATE podcasts
			SET episode_count = ?, newest_episode_date = ?, updated_at = ?
			WHERE id = ? AND deleted_at IS NULL`,
			repair.ExpectedEpisodeCount, newest, now, repair.PodcastID)
		if err != nil {
			return 0, fmt.Errorf("播客 %d: %w", repair.PodcastID, err)
		}
		rowsAffected, err := result.RowsAffected()
		if err != nil {
			return 0, err
		}
		if rowsAffected != 1 {
			return 0, fmt.Errorf("播客 %d: 预期更新1行，实际更新%d行", repair.PodcastID, rowsAffected)
		}
		applied++
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return applied, nil
}

func backupDatabase(db *sql.DB, targetPath string) error {
	targetPath, err := filepath.Abs(targetPath)
	if err != nil {
		return err
	}
	if _, err := os.Stat(targetPath); err == nil {
		return fmt.Errorf("备份文件已存在，拒绝覆盖: %s", targetPath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		return err
	}
	tmpPath := fmt.Sprintf("%s.tmp-%d", targetPath, os.Getpid())
	_ = os.Remove(tmpPath)
	defer os.Remove(tmpPath)

	quotedPath := strings.ReplaceAll(tmpPath, "'", "''")
	if _, err := db.Exec("VACUUM INTO '" + quotedPath + "'"); err != nil {
		return err
	}
	if err := verifySQLiteFile(tmpPath); err != nil {
		return fmt.Errorf("临时备份校验失败: %w", err)
	}
	if err := os.Rename(tmpPath, targetPath); err != nil {
		return err
	}
	return verifySQLiteFile(targetPath)
}

func verifySQLiteFile(path string) error {
	db, err := sql.Open("sqlite3", "file:"+path+"?mode=ro&_busy_timeout=10000")
	if err != nil {
		return err
	}
	defer db.Close()
	var result string
	if err := db.QueryRow("PRAGMA integrity_check").Scan(&result); err != nil {
		return err
	}
	if result != "ok" {
		return fmt.Errorf("integrity_check=%s", result)
	}
	return nil
}

func describeFile(path string) (int64, string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, "", err
	}
	file, err := os.Open(path)
	if err != nil {
		return 0, "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return 0, "", err
	}
	return info.Size(), hex.EncodeToString(hash.Sum(nil)), nil
}

func writeReport(path string, report runReport) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, append(data, '\n'), 0644); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return nil
}

func printReport(reportPath string, report runReport) {
	fmt.Printf("模式: %s\n", report.Mode)
	fmt.Printf("数据库: %s\n", report.DatabasePath)
	fmt.Printf("扫描播客: %d\n", report.Before.PodcastsScanned)
	fmt.Printf("不一致播客: %d（数量 %d，最新发布时间 %d）\n",
		report.Before.MismatchedPodcasts,
		report.Before.EpisodeCountMismatches,
		report.Before.NewestEpisodeDateMismatches)
	fmt.Printf("计划修复: %d\n", len(report.PlannedChanges))
	if report.Mode == "apply" {
		fmt.Printf("备份: %s\n", report.BackupPath)
		fmt.Printf("已修复: %d\n", report.AppliedPodcasts)
		fmt.Printf("复核后不一致: %d\n", report.After.MismatchedPodcasts)
	}
	fmt.Printf("审计报告: %s\n", reportPath)
}

func exitf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
