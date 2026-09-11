package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"magicpodcast/internal/database"
	"magicpodcast/internal/models"
	"os"
	"path/filepath"
	"testing"
)

func TestPreviewDoesNotWriteAndApplyRequiresExplicitScope(t *testing.T) {
	path := filepath.Join(t.TempDir(), "preview.db")
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, database.ApplyMigrations(db))
	pod := models.Podcast{XYZID: "preview", Title: "访谈", FeedURL: "https://example.test/preview"}
	require.NoError(t, db.Create(&pod).Error)
	ep := models.Episode{PodcastID: pod.ID, Title: "访谈", GUID: "preview"}
	require.NoError(t, db.Create(&ep).Error)
	person := models.Person{StableKey: "preview", DisplayName: "旧候选"}
	require.NoError(t, db.Create(&person).Error)
	require.NoError(t, db.Create(&models.EpisodeAppearance{EpisodeID: ep.ID, PersonID: person.ID, Role: "unknown", Status: "confirmed", SourceVersion: "legacy"}).Error)
	sqlDB, _ := db.DB()
	require.NoError(t, sqlDB.Close())
	before, err := os.ReadFile(path)
	require.NoError(t, err)
	var output bytes.Buffer
	require.NoError(t, run(context.Background(), []string{"--db", path}, &output))
	require.Contains(t, output.String(), "旧候选")
	require.Contains(t, output.String(), `"needs_preparation":true`)
	for _, args := range [][]string{
		{"--db", path, "--apply"},
		{"--db", path, "--episodes", "1,1"},
		{"--db", path, "--episodes", "0"},
		{"--db", path, "--episodes", "999"},
		{"--db", path, "--confirm", confirmation},
	} {
		require.Error(t, run(context.Background(), args, &output))
	}
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, sha256.Sum256(before), sha256.Sum256(after))
}

func TestApplyRejectsHealthyBackupFromAnotherDatabase(t *testing.T) {
	dir := t.TempDir()
	targetPath := filepath.Join(dir, "target.db")
	backupPath := filepath.Join(dir, "unrelated.db")
	for _, path := range []string{targetPath, backupPath} {
		db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
		require.NoError(t, err)
		require.NoError(t, database.ApplyMigrations(db))
		pod := models.Podcast{XYZID: path, Title: filepath.Base(path), FeedURL: "https://example.test/" + filepath.Base(path)}
		require.NoError(t, db.Create(&pod).Error)
		ep := models.Episode{PodcastID: pod.ID, GUID: path, Title: filepath.Base(path)}
		require.NoError(t, db.Create(&ep).Error)
		sqlDB, err := db.DB()
		require.NoError(t, err)
		require.NoError(t, sqlDB.Close())
	}
	var output bytes.Buffer
	err := run(context.Background(), []string{
		"--db", targetPath, "--episodes", "1", "--apply", "--confirm", confirmation,
		"--backup", backupPath, "--artifacts", dir, "--runtime-work-root", dir,
		"--python", filepath.Join(dir, "python"), "--runtime-host", filepath.Join(dir, "runtime_host.py"),
	}, &output)
	require.ErrorContains(t, err, "backup does not match target database state")
}

func TestLogicalFingerprintMatchesAnExactDatabaseCopy(t *testing.T) {
	dir := t.TempDir()
	targetPath := filepath.Join(dir, "target.db")
	copyPath := filepath.Join(dir, "copy.db")
	db, err := gorm.Open(sqlite.Open(targetPath), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, database.ApplyMigrations(db))
	require.NoError(t, db.Create(&models.Podcast{XYZID: "fingerprint", Title: "访谈", FeedURL: "https://example.test/fingerprint"}).Error)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
	contents, err := os.ReadFile(targetPath)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(copyPath, contents, 0600))
	target, err := openDB(targetPath, false)
	require.NoError(t, err)
	copyDB, err := openDB(copyPath, false)
	require.NoError(t, err)
	targetFingerprint, err := logicalDatabaseFingerprint(context.Background(), target)
	require.NoError(t, err)
	copyFingerprint, err := logicalDatabaseFingerprint(context.Background(), copyDB)
	require.NoError(t, err)
	require.Equal(t, targetFingerprint, copyFingerprint)
	targetSQL, _ := target.DB()
	copySQL, _ := copyDB.DB()
	require.NoError(t, targetSQL.Close())
	require.NoError(t, copySQL.Close())
}
