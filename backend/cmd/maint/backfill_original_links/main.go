// Command backfill_original_links audits and (only with --apply) repairs
// historical episodes whose original link (原节目链接) is empty but
// restorable through the shared strict originallink resolution entry.
//
// The tool never decides link rules itself: every planned value comes from
// originallink.Resolve (currently only the verified WavPub page GUID rule can
// restore a missing link). It never overwrites a non-empty stored link.
//
// Default mode is a read-only dry run that prints every planned write and
// every skip with an explicit reason. Only `--apply --confirm <确认串>` writes,
// after showing the target database and the planned write count. Use it
// against a Fixture/Snapshot or a temporary copy for local verification; a
// real production backfill is a separately authorized operation that requires
// a verified backup, a stopped or maintenance-mode writer, and an explicit
// operator decision.
package main

import (
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"magicpodcast/internal/originallink"

	_ "github.com/mattn/go-sqlite3"
)

const (
	defaultDBPath     = "data/magicpodcast.db"
	applyConfirmation = "I_UNDERSTAND_THIS_WRITES_ORIGINAL_LINKS"
)

type config struct {
	dbPath     string
	backupPath string
	apply      bool
	confirm    string
}

type auditSummary struct {
	EpisodesScanned       int
	PlannedWrites         int
	SkippedExisting       int
	SkippedMissingPodcast int
	SkippedUnresolvable   int
}

type plannedWrite struct {
	EpisodeID    int64
	EpisodeGUID  string
	EpisodeTitle string
	PodcastID    int64
	FeedURL      string
	PlannedLink  string
	Source       originallink.Source
	Reason       string
}

type skippedRecord struct {
	EpisodeID    int64
	EpisodeGUID  string
	EpisodeTitle string
	SkipReason   string
}

func main() {
	cfg, err := parseFlags(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "backfill_original_links: %v\n", err)
		os.Exit(2)
	}
	if err := run(cfg, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "backfill_original_links: %v\n", err)
		os.Exit(1)
	}
}

func parseFlags(args []string) (config, error) {
	cfg := config{}
	fs := flag.NewFlagSet("backfill_original_links", flag.ContinueOnError)
	fs.StringVar(&cfg.dbPath, "db", defaultDBPath, "目标 SQLite 数据库路径")
	fs.BoolVar(&cfg.apply, "apply", false, "写入模式（默认 dry-run，只预览零写入）")
	fs.StringVar(&cfg.confirm, "confirm", "", "--apply 必须同时提供 --confirm "+applyConfirmation)
	fs.StringVar(&cfg.backupPath, "backup", "", "--apply 使用的备份路径（默认 data/backups/）")
	if err := fs.Parse(args); err != nil {
		return cfg, err
	}
	if fs.NArg() != 0 {
		return cfg, fmt.Errorf("不接受位置参数: %s", strings.Join(fs.Args(), " "))
	}
	if cfg.apply && cfg.confirm != applyConfirmation {
		return cfg, fmt.Errorf("--apply 必须同时提供 --confirm %s", applyConfirmation)
	}
	if !cfg.apply && cfg.confirm != "" {
		return cfg, errors.New("--confirm 只能与 --apply 一起使用")
	}
	return cfg, nil
}

func run(cfg config, out io.Writer) error {
	db, err := openDB(cfg.dbPath, !cfg.apply)
	if err != nil {
		return err
	}
	defer db.Close()

	plan, skipped, audit, err := loadBackfillPlan(db)
	if err != nil {
		return err
	}

	fmt.Fprintf(out, "模式: %s\n目标数据库: %s\n扫描单集: %d，计划写入: %d，跳过: %d\n",
		modeName(cfg.apply), cfg.dbPath, audit.EpisodesScanned, audit.PlannedWrites,
		audit.SkippedExisting+audit.SkippedMissingPodcast+audit.SkippedUnresolvable)
	for _, entry := range plan {
		fmt.Fprintf(out, "  计划写入 episode_id=%d guid=%q 旧值=\"\" 新值=%q 来源=%s 原因=%s\n",
			entry.EpisodeID, entry.EpisodeGUID, entry.PlannedLink, entry.Source, entry.Reason)
	}
	for _, skip := range skipped {
		fmt.Fprintf(out, "  跳过 episode_id=%d guid=%q 原因=%s\n",
			skip.EpisodeID, skip.EpisodeGUID, skip.SkipReason)
	}

	if !cfg.apply {
		fmt.Fprintf(out, "dry-run 未修改任何数据。本地验证请对 Fixture/Snapshot 或临时库执行；生产库回填需单独授权，并先完成备份与停写。\n")
	} else {
		fmt.Fprintf(out, "apply 将向上述数据库写入 %d 条原节目链接。\n", audit.PlannedWrites)
		if audit.PlannedWrites == 0 {
			fmt.Fprintln(out, "没有可写入的记录，跳过备份与写入。")
		} else {
			backupPath := prepareBackup(cfg)
			if err := backupDatabase(db, backupPath); err != nil {
				return fmt.Errorf("创建并校验备份失败，已取消写入: %w", err)
			}
			fmt.Fprintf(out, "已备份到 %s\n", backupPath)

			applied, conflicts, err := applyBackfill(db, plan)
			if err != nil {
				return fmt.Errorf("写入失败（事务已回滚）: %w", err)
			}
			fmt.Fprintf(out, "已写入 %d 条原节目链接。\n", applied)
			for _, conflict := range conflicts {
				fmt.Fprintf(out, "  跳过冲突 episode_id=%d guid=%q 原因=%s\n",
					conflict.EpisodeID, conflict.EpisodeGUID, conflict.SkipReason)
			}
		}
		fmt.Fprintf(out, "完成。重复执行是幂等的：已有非空链接的记录会被跳过。\n")
	}

	return nil
}

func prepareBackup(cfg config) string {
	if cfg.backupPath != "" {
		return cfg.backupPath
	}
	stamp := time.Now().UTC().Format("20060102T150405Z")
	return filepath.Join("data", "backups", "backfill_original_links_"+stamp+".db")
}

func modeName(apply bool) string {
	if apply {
		return "apply"
	}
	return "dry-run"
}

func openDB(path string, queryOnly bool) (*sql.DB, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
	dsn := path
	if queryOnly {
		absolutePath, err := filepath.Abs(path)
		if err != nil {
			return nil, err
		}
		readOnlyURL := &url.URL{Scheme: "file", Path: absolutePath}
		query := readOnlyURL.Query()
		query.Set("mode", "ro")
		query.Set("_query_only", "1")
		query.Set("_busy_timeout", "10000")
		readOnlyURL.RawQuery = query.Encode()
		dsn = readOnlyURL.String()
	}
	db, err := sql.Open("sqlite3", dsn)
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

// loadBackfillPlan scans active episodes, routes each record through the
// shared originallink entry, and classifies the outcome. The write decision
// never leaves this entry: only a strict resolution hit (non-empty URL from
// the resolver while the stored link is empty) becomes a planned write.
func loadBackfillPlan(db *sql.DB) ([]plannedWrite, []skippedRecord, auditSummary, error) {
	rows, err := db.Query(`
		SELECT
			e.id,
			COALESCE(e.guid, ''),
			COALESCE(e.title, ''),
			e.podcast_id,
			COALESCE(e.link, ''),
			p.id,
			COALESCE(p.feed_url, '')
		FROM episodes e
			LEFT JOIN podcasts p ON p.id = e.podcast_id AND p.deleted_at IS NULL
		WHERE e.deleted_at IS NULL
		ORDER BY e.id`)
	if err != nil {
		return nil, nil, auditSummary{}, err
	}
	defer rows.Close()

	plan := make([]plannedWrite, 0)
	skipped := make([]skippedRecord, 0)
	audit := auditSummary{}
	for rows.Next() {
		var (
			id              int64
			guid            string
			title           string
			podcastID       int64
			link            string
			joinedPodcastID sql.NullInt64
			feedURL         sql.NullString
		)
		if err := rows.Scan(&id, &guid, &title, &podcastID, &link, &joinedPodcastID, &feedURL); err != nil {
			return nil, nil, auditSummary{}, err
		}
		audit.EpisodesScanned++

		if strings.TrimSpace(link) != "" {
			audit.SkippedExisting++
			skipped = append(skipped, skippedRecord{
				EpisodeID: id, EpisodeGUID: guid, EpisodeTitle: title,
				SkipReason: "已有非空原节目链接，不覆盖",
			})
			continue
		}
		if !joinedPodcastID.Valid {
			audit.SkippedMissingPodcast++
			skipped = append(skipped, skippedRecord{
				EpisodeID: id, EpisodeGUID: guid, EpisodeTitle: title,
				SkipReason: "找不到所属播客，无法确定 Feed 身份",
			})
			continue
		}

		decision := originallink.Resolve(originallink.Input{
			Feed:         originallink.FeedIdentity{FeedURL: feedURL.String},
			GUID:         guid,
			ExistingLink: link,
		})
		if decision.URL == "" {
			audit.SkippedUnresolvable++
			skipped = append(skipped, skippedRecord{
				EpisodeID: id, EpisodeGUID: guid, EpisodeTitle: title,
				SkipReason: "无法用严格规则解析出原节目链接（" + decision.Reason + "）",
			})
			continue
		}

		plan = append(plan, plannedWrite{
			EpisodeID:    id,
			EpisodeGUID:  guid,
			EpisodeTitle: title,
			PodcastID:    podcastID,
			FeedURL:      feedURL.String,
			PlannedLink:  decision.URL,
			Source:       decision.Source,
			Reason:       decision.Reason,
		})
		audit.PlannedWrites++
	}
	if err := rows.Err(); err != nil {
		return nil, nil, auditSummary{}, err
	}
	return plan, skipped, audit, nil
}

// applyBackfill writes planned links inside one transaction. The WHERE clause
// re-checks that the stored link is still empty at write time, so the run
// never overwrites a non-empty value even if data drifted after the scan.
func applyBackfill(db *sql.DB, plan []plannedWrite) (int, []skippedRecord, error) {
	if len(plan) == 0 {
		return 0, nil, nil
	}
	tx, err := db.Begin()
	if err != nil {
		return 0, nil, err
	}
	defer tx.Rollback()

	now := time.Now().UTC().Format(time.RFC3339Nano)
	applied := 0
	conflicts := make([]skippedRecord, 0)
	for _, entry := range plan {
		result, err := tx.Exec(`
			UPDATE episodes
			SET link = ?, updated_at = ?
			WHERE id = ?
			  AND podcast_id = ?
			  AND guid = ?
			  AND deleted_at IS NULL
			  AND TRIM(COALESCE(link, '')) = ''
			  AND EXISTS (
				SELECT 1
				FROM podcasts p
				WHERE p.id = episodes.podcast_id
				  AND p.deleted_at IS NULL
				  AND p.feed_url = ?
			  )`,
			entry.PlannedLink, now, entry.EpisodeID, entry.PodcastID, entry.EpisodeGUID, entry.FeedURL)
		if err != nil {
			return 0, nil, fmt.Errorf("单集 %d: %w", entry.EpisodeID, err)
		}
		rowsAffected, err := result.RowsAffected()
		if err != nil {
			return 0, nil, err
		}
		if rowsAffected == 0 {
			conflicts = append(conflicts, skippedRecord{
				EpisodeID:    entry.EpisodeID,
				EpisodeGUID:  entry.EpisodeGUID,
				EpisodeTitle: entry.EpisodeTitle,
				SkipReason:   "记录在计划后发生变化：链接、GUID、播客或 Feed 身份已漂移",
			})
			continue
		}
		if rowsAffected != 1 {
			return 0, nil, fmt.Errorf("单集 %d: 预期最多写入1行，实际写入%d行",
				entry.EpisodeID, rowsAffected)
		}
		applied++
	}
	if err := tx.Commit(); err != nil {
		return 0, nil, err
	}
	return applied, conflicts, nil
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
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
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
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	readOnlyURL := &url.URL{Scheme: "file", Path: absolutePath}
	query := readOnlyURL.Query()
	query.Set("mode", "ro")
	query.Set("_busy_timeout", "10000")
	readOnlyURL.RawQuery = query.Encode()

	db, err := sql.Open("sqlite3", readOnlyURL.String())
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
