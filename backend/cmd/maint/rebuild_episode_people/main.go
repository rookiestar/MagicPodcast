// Command rebuild_episode_people previews identity repair by default. Production
// execution requires separate authorization, a verified backup and stopped writers.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"magicpodcast/internal/codexruntime"
	"magicpodcast/internal/contentsearch"
	"magicpodcast/internal/database"
	"magicpodcast/internal/personidentity"
	"magicpodcast/internal/processing"
)

const confirmation = "REBUILD_SELECTED_EPISODE_PEOPLE"

type options struct {
	db, episodes, backup, artifacts, workRoot, python, host, confirm string
	apply                                                            bool
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := run(ctx, os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func openDB(path string, writable bool) (*gorm.DB, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("database must be an existing regular file")
	}
	mode := "ro"
	if writable {
		mode = "rw"
	}
	u := url.URL{Scheme: "file", Path: absolute}
	q := url.Values{"mode": {mode}, "_foreign_keys": {"on"}, "_busy_timeout": {"5000"}}
	u.RawQuery = q.Encode()
	db, err := gorm.Open(sqlite.Open(u.String()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return nil, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(1)
	return db, nil
}

func parseIDs(value string) ([]uint, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	var ids []uint
	seen := map[uint]bool{}
	for _, part := range strings.Split(value, ",") {
		n, err := strconv.ParseUint(strings.TrimSpace(part), 10, 32)
		if err != nil || n == 0 || seen[uint(n)] {
			return nil, fmt.Errorf("episodes must contain distinct positive IDs")
		}
		seen[uint(n)] = true
		ids = append(ids, uint(n))
	}
	return ids, nil
}

func run(ctx context.Context, args []string, output io.Writer) error {
	var o options
	fs := flag.NewFlagSet("rebuild_episode_people", flag.ContinueOnError)
	fs.StringVar(&o.db, "db", "", "explicit SQLite path (required)")
	fs.StringVar(&o.episodes, "episodes", "", "comma-separated IDs; required for apply")
	fs.BoolVar(&o.apply, "apply", false, "rebuild only selected episodes; default read-only preview")
	fs.StringVar(&o.confirm, "confirm", "", "apply requires "+confirmation)
	fs.StringVar(&o.backup, "backup", "", "verified pre-operation SQLite backup path")
	fs.StringVar(&o.artifacts, "artifacts", "", "existing processing artifact root")
	fs.StringVar(&o.workRoot, "runtime-work-root", "", "isolated Runtime work root")
	fs.StringVar(&o.python, "python", "", "existing Codex SDK Python executable")
	fs.StringVar(&o.host, "runtime-host", "", "existing runtime_host.py path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 || o.db == "" {
		return fmt.Errorf("--db is required; positional arguments are not accepted")
	}
	ids, err := parseIDs(o.episodes)
	if err != nil {
		return err
	}
	if o.apply && (len(ids) == 0 || o.confirm != confirmation || o.backup == "" || o.artifacts == "" || o.workRoot == "" || o.python == "" || o.host == "") {
		return fmt.Errorf("apply requires explicit episodes, confirmation, backup, artifact root and Runtime paths")
	}
	if !o.apply && o.confirm != "" {
		return fmt.Errorf("confirmation requires --apply")
	}
	db, err := openDB(o.db, false)
	if err != nil {
		return err
	}
	sqlDB, _ := db.DB()
	preview, err := personidentity.PreviewRebuild(ctx, db, ids)
	closeErr := sqlDB.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	encoder := json.NewEncoder(output)
	if err := encoder.Encode(struct {
		Database string                          `json:"database"`
		Preview  []personidentity.RebuildPreview `json:"preview"`
	}{o.db, preview}); err != nil {
		return err
	}
	if !o.apply {
		return nil
	}
	targetInfo, err := os.Stat(o.db)
	if err != nil {
		return err
	}
	backupInfo, err := os.Stat(o.backup)
	if err != nil {
		return err
	}
	if os.SameFile(targetInfo, backupInfo) {
		return fmt.Errorf("backup must not be the target database")
	}
	backup, err := openDB(o.backup, false)
	if err != nil {
		return err
	}
	backupSQL, _ := backup.DB()
	var integrity string
	err = backup.Raw("PRAGMA quick_check").Scan(&integrity).Error
	_ = backupSQL.Close()
	if err != nil {
		return err
	}
	if integrity != "ok" {
		return fmt.Errorf("backup integrity check failed")
	}
	db, err = openDB(o.db, true)
	if err != nil {
		return err
	}
	sqlDB, _ = db.DB()
	defer sqlDB.Close()
	if err := database.RequireSchemaReady(db); err != nil {
		return err
	}
	store, err := processing.NewDiskArtifactStore(o.artifacts)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(o.workRoot, 0700); err != nil {
		return err
	}
	host, err := codexruntime.NewProcessHost(codexruntime.ProcessHostConfig{Command: []string{o.python, o.host}, WorkRoot: o.workRoot, Profiles: codexruntime.DefaultProfiles()})
	if err != nil {
		return err
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = host.Close(closeCtx)
	}()
	search, err := contentsearch.NewService(db)
	if err != nil {
		return err
	}
	service, err := personidentity.NewService(db, personidentity.NewRuntimeSuggester(host, o.workRoot), search)
	if err != nil {
		return err
	}
	return service.WithArtifactReader(store).RebuildSelected(ctx, ids, func(result personidentity.RebuildResult) error { return encoder.Encode(result) })
}
