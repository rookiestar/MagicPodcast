package main

import (
	"bytes"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"magicpodcast/internal/originallink"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
)

func openBackfillTestDB(t *testing.T) (*sql.DB, string) {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "magicpodcast.db")
	db, err := sql.Open("sqlite3", dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	_, err = db.Exec(`
		CREATE TABLE podcasts (
			id INTEGER PRIMARY KEY,
			title TEXT NOT NULL,
			feed_url TEXT NOT NULL DEFAULT '',
			deleted_at DATETIME
		);
		CREATE TABLE episodes (
			id INTEGER PRIMARY KEY,
			podcast_id INTEGER NOT NULL,
			title TEXT NOT NULL DEFAULT '',
			guid TEXT NOT NULL DEFAULT '',
			link TEXT NOT NULL DEFAULT '',
			updated_at DATETIME,
			deleted_at DATETIME
		);`)
	require.NoError(t, err)
	return db, dbPath
}

func seedBackfillFixtures(t *testing.T, db *sql.DB) {
	t.Helper()

	_, err := db.Exec(`
		INSERT INTO podcasts(id, title, feed_url) VALUES
			(1, '后互联网时代的乱弹', 'https://hosting.wavpub.cn/pie/feed/'),
			(2, 'Unverified Show', 'https://rss.art19.com/unverified-show');
		INSERT INTO episodes(id, podcast_id, title, guid, link) VALUES
			(11, 1, '第229期 永远的钟鼓楼', 'https://hosting.wavpub.cn/pie/ep229/', ''),
			(12, 1, '第228期 机器人运动会', 'ep-plain-guid', ''),
			(13, 1, '已有链接单集', 'https://hosting.wavpub.cn/pie/ep227/', 'https://hosting.wavpub.cn/pie/ep227/'),
			(14, 2, 'URL GUID 但来源未验证', 'https://hosting.wavpub.cn/pie/ep229/', ''),
			(15, 1, '危险 GUID', 'javascript:alert(1)', ''),
			(16, 99, '找不到所属播客', 'https://hosting.wavpub.cn/pie/ep226/', '');`)
	require.NoError(t, err)
}

func TestLoadBackfillPlanOnlyPlansStrictWavPubHits(t *testing.T) {
	db, _ := openBackfillTestDB(t)
	seedBackfillFixtures(t, db)

	plan, skipped, audit, err := loadBackfillPlan(db)
	require.NoError(t, err)
	require.Len(t, plan, 1)
	require.Equal(t, 6, audit.EpisodesScanned)
	require.Equal(t, 1, audit.PlannedWrites)
	require.Equal(t, 1, audit.SkippedExisting)
	require.Equal(t, 1, audit.SkippedMissingPodcast)
	require.Equal(t, 3, audit.SkippedUnresolvable)

	require.Equal(t, int64(11), plan[0].EpisodeID)
	require.Equal(t, "https://hosting.wavpub.cn/pie/ep229/", plan[0].PlannedLink)
	require.Equal(t, originallink.SourceWavPubGUID, plan[0].Source)

	skipReasons := map[int64]string{}
	for _, skip := range skipped {
		skipReasons[skip.EpisodeID] = skip.SkipReason
	}
	require.Contains(t, skipReasons[13], "已有非空原节目链接")
	require.Contains(t, skipReasons[16], "找不到所属播客")
	require.Contains(t, skipReasons[12], "无法用严格规则解析")
	require.Contains(t, skipReasons[14], "无法用严格规则解析")
	require.Contains(t, skipReasons[15], "无法用严格规则解析")
}

func TestDryRunOpensReadOnlyAndWritesNothing(t *testing.T) {
	db, dbPath := openBackfillTestDB(t)
	seedBackfillFixtures(t, db)
	require.NoError(t, db.Close())
	before, err := os.ReadFile(dbPath)
	require.NoError(t, err)

	cfg := config{dbPath: dbPath}
	var out bytes.Buffer
	require.NoError(t, run(cfg, &out))
	require.Contains(t, out.String(), "dry-run 未修改任何数据")
	after, err := os.ReadFile(dbPath)
	require.NoError(t, err)
	require.Equal(t, before, after, "dry-run must leave the SQLite file byte-for-byte unchanged")

	ro, err := openDB(dbPath, true)
	require.NoError(t, err)
	defer ro.Close()

	var count int
	require.NoError(t, ro.QueryRow(
		`SELECT COUNT(*) FROM episodes WHERE link <> '' AND id = 11`).Scan(&count))
	require.Equal(t, 0, count, "dry-run must not fill any link")

	_, err = ro.Exec(`UPDATE episodes SET link = 'x' WHERE id = 11`)
	require.Error(t, err, "query_only mode must refuse writes")
}

func TestApplyWritesOnlyEmptyLinksAndIsIdempotent(t *testing.T) {
	db, _ := openBackfillTestDB(t)
	seedBackfillFixtures(t, db)

	plan, _, audit, err := loadBackfillPlan(db)
	require.NoError(t, err)
	require.Equal(t, 1, audit.PlannedWrites)

	applied, conflicts, err := applyBackfill(db, plan)
	require.NoError(t, err)
	require.Equal(t, 1, applied)
	require.Empty(t, conflicts)

	var link string
	require.NoError(t, db.QueryRow(`SELECT link FROM episodes WHERE id = 11`).Scan(&link))
	require.Equal(t, "https://hosting.wavpub.cn/pie/ep229/", link)

	// The non-empty record is never touched.
	require.NoError(t, db.QueryRow(`SELECT link FROM episodes WHERE id = 13`).Scan(&link))
	require.Equal(t, "https://hosting.wavpub.cn/pie/ep227/", link)

	// A second run plans nothing and applies nothing.
	plan2, _, audit2, err := loadBackfillPlan(db)
	require.NoError(t, err)
	require.Equal(t, 0, audit2.PlannedWrites)
	require.Equal(t, 2, audit2.SkippedExisting,
		"the just-written record and the original one are both non-empty now")
	applied2, conflicts2, err := applyBackfill(db, plan2)
	require.NoError(t, err)
	require.Equal(t, 0, applied2)
	require.Empty(t, conflicts2)
}

func TestApplySkipsALinkThatAppearedAfterScan(t *testing.T) {
	db, _ := openBackfillTestDB(t)
	seedBackfillFixtures(t, db)

	plan, _, _, err := loadBackfillPlan(db)
	require.NoError(t, err)
	require.Len(t, plan, 1)

	// Simulate drift between scan and write.
	_, err = db.Exec(`UPDATE episodes SET link = 'https://example.com/appeared' WHERE id = 11`)
	require.NoError(t, err)

	applied, conflicts, err := applyBackfill(db, plan)
	require.NoError(t, err)
	require.Zero(t, applied)
	require.Len(t, conflicts, 1)
	require.Equal(t, int64(11), conflicts[0].EpisodeID)
	require.Contains(t, conflicts[0].SkipReason, "发生变化")

	var link string
	require.NoError(t, db.QueryRow(`SELECT link FROM episodes WHERE id = 11`).Scan(&link))
	require.Equal(t, "https://example.com/appeared", link)
}

func TestApplySkipsIdentityDriftAfterScan(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, *sql.DB)
	}{
		{
			name: "episode GUID changed",
			mutate: func(t *testing.T, db *sql.DB) {
				_, err := db.Exec(`UPDATE episodes SET guid = 'changed-guid' WHERE id = 11`)
				require.NoError(t, err)
			},
		},
		{
			name: "podcast Feed identity changed",
			mutate: func(t *testing.T, db *sql.DB) {
				_, err := db.Exec(`UPDATE podcasts SET feed_url = 'https://example.com/feed.xml' WHERE id = 1`)
				require.NoError(t, err)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, _ := openBackfillTestDB(t)
			seedBackfillFixtures(t, db)
			plan, _, _, err := loadBackfillPlan(db)
			require.NoError(t, err)
			require.Len(t, plan, 1)

			tt.mutate(t, db)
			applied, conflicts, err := applyBackfill(db, plan)
			require.NoError(t, err)
			require.Zero(t, applied)
			require.Len(t, conflicts, 1)

			var link string
			require.NoError(t, db.QueryRow(`SELECT link FROM episodes WHERE id = 11`).Scan(&link))
			require.Empty(t, link)
		})
	}
}

func TestLoadBackfillPlanSupportsWavPubProxyFeed(t *testing.T) {
	db, _ := openBackfillTestDB(t)
	_, err := db.Exec(`
		INSERT INTO podcasts(id, title, feed_url) VALUES
			(1, '后互联网时代的乱弹', 'https://proxy.wavpub.com/pie.xml');
		INSERT INTO episodes(id, podcast_id, title, guid, link) VALUES
			(11, 1, '第229期 永远的钟鼓楼', 'https://hosting.wavpub.cn/pie/?p=822', '');`)
	require.NoError(t, err)

	plan, skipped, audit, err := loadBackfillPlan(db)
	require.NoError(t, err)
	require.Len(t, plan, 1)
	require.Empty(t, skipped)
	require.Equal(t, 1, audit.EpisodesScanned)
	require.Equal(t, 1, audit.PlannedWrites)
	require.Equal(t, int64(11), plan[0].EpisodeID)
	require.Equal(t, "https://hosting.wavpub.cn/pie/?p=822", plan[0].PlannedLink)
	require.Equal(t, originallink.SourceWavPubGUID, plan[0].Source)
}

func TestParseFlagsRequiresConfirmationForApply(t *testing.T) {
	_, err := parseFlags([]string{"--apply"})
	require.Error(t, err)
	require.Contains(t, err.Error(), applyConfirmation)

	cfg, err := parseFlags([]string{"--apply", "--confirm", applyConfirmation})
	require.NoError(t, err)
	require.True(t, cfg.apply)

	_, err = parseFlags([]string{"--confirm", applyConfirmation})
	require.Error(t, err, "--confirm without --apply must be rejected")
}

func TestParseFlagsRejectsUnscopedReportFileWrites(t *testing.T) {
	_, err := parseFlags([]string{"--report", "audit.json"})
	require.Error(t, err, "the dry-run command must not expose an extra file-writing flag")
}

func TestParseFlagsRejectsPositionalArguments(t *testing.T) {
	_, err := parseFlags([]string{"unexpected.db"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "不接受位置参数")
}

func TestRunApplyPrintsTargetDatabaseAndPlannedCount(t *testing.T) {
	db, dbPath := openBackfillTestDB(t)
	seedBackfillFixtures(t, db)
	require.NoError(t, db.Close())

	cfg := config{
		dbPath:     dbPath,
		backupPath: filepath.Join(t.TempDir(), "backup.db"),
		apply:      true,
		confirm:    applyConfirmation,
	}
	var out bytes.Buffer
	require.NoError(t, run(cfg, &out))

	output := out.String()
	require.Contains(t, output, "模式: apply")
	require.Contains(t, output, dbPath)
	require.Contains(t, output, "写入 1 条")
	require.Contains(t, output, "已写入 1 条原节目链接")
	require.NoError(t, verifySQLiteFile(cfg.backupPath))
	require.True(t,
		strings.Index(output, "apply 将向上述数据库写入 1 条") < strings.Index(output, "已写入 1 条"),
		"the planned write count must be shown before writing")
}
