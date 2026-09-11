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
