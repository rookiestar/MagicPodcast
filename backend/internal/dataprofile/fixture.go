package dataprofile

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"magicpodcast/internal/database"
	"magicpodcast/internal/models"

	"gorm.io/gorm"
)

const FixtureSeries = "basic-v1"

type Fixture struct {
	Version      string
	DatabasePath string
	ManifestPath string
	Manifest     Manifest
}

func CurrentFixtureVersion() string {
	return fmt.Sprintf("%s-schema-%d", FixtureSeries, database.CurrentSchemaVersion)
}

func EnsureFixture(profileHome string) (Fixture, error) {
	root, err := ensureRoot(profileHome)
	if err != nil {
		return Fixture{}, err
	}
	version := CurrentFixtureVersion()
	fixtureDir := filepath.Join(root, "fixtures", version)
	databasePath := filepath.Join(fixtureDir, "magicpodcast.db")
	manifestPath := filepath.Join(fixtureDir, "manifest.json")

	if _, err := os.Stat(databasePath); err == nil {
		manifest, _, validationErr := ValidateManifest(databasePath, manifestPath, "fixture")
		if validationErr != nil {
			return Fixture{}, fmt.Errorf("existing fixture is invalid and was preserved: %w", validationErr)
		}
		if manifest.FixtureVersion != version || manifest.ID != version {
			return Fixture{}, fmt.Errorf("existing fixture metadata does not match %s", version)
		}
		return Fixture{
			Version:      version,
			DatabasePath: databasePath,
			ManifestPath: manifestPath,
			Manifest:     manifest,
		}, nil
	} else if !os.IsNotExist(err) {
		return Fixture{}, fmt.Errorf("inspect fixture: %w", err)
	}

	fixturesRoot := filepath.Join(root, "fixtures")
	if err := os.MkdirAll(fixturesRoot, 0o700); err != nil {
		return Fixture{}, fmt.Errorf("create fixtures directory: %w", err)
	}
	tempDir, err := os.MkdirTemp(fixturesRoot, "."+version+".tmp-*")
	if err != nil {
		return Fixture{}, fmt.Errorf("create fixture staging directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	tempDatabase := filepath.Join(tempDir, "magicpodcast.db")
	if err := buildFixtureDatabase(tempDatabase); err != nil {
		return Fixture{}, err
	}
	status, err := ValidateDatabase(tempDatabase)
	if err != nil {
		return Fixture{}, fmt.Errorf("validate generated fixture: %w", err)
	}
	hash, err := SHA256File(tempDatabase)
	if err != nil {
		return Fixture{}, err
	}
	manifest := Manifest{
		FormatVersion:  ManifestFormatVersion,
		Kind:           "fixture",
		ID:             version,
		SchemaVersion:  status.SchemaVersion,
		FixtureVersion: version,
		SHA256:         hash,
		Counts:         status.Counts,
	}
	if err := WriteManifest(filepath.Join(tempDir, "manifest.json"), manifest); err != nil {
		return Fixture{}, err
	}
	if err := os.Chmod(tempDatabase, 0o400); err != nil {
		return Fixture{}, fmt.Errorf("set fixture database mode: %w", err)
	}
	if err := os.Chmod(filepath.Join(tempDir, "manifest.json"), 0o400); err != nil {
		return Fixture{}, fmt.Errorf("set fixture manifest mode: %w", err)
	}
	if err := os.Rename(tempDir, fixtureDir); err != nil {
		return Fixture{}, fmt.Errorf("publish fixture: %w", err)
	}

	return Fixture{
		Version:      version,
		DatabasePath: databasePath,
		ManifestPath: manifestPath,
		Manifest:     manifest,
	}, nil
}

func buildFixtureDatabase(path string) error {
	db, closeDB, err := openSQLite(path, false)
	if err != nil {
		return err
	}
	defer closeDB()
	if err := database.ApplyMigrations(db); err != nil {
		return fmt.Errorf("create fixture schema: %w", err)
	}

	fixed := time.Date(2026, time.August, 1, 8, 0, 0, 0, time.UTC)
	if err := db.Exec("UPDATE schema_migrations SET applied_at = ?", fixed).Error; err != nil {
		return fmt.Errorf("stabilize fixture migration timestamps: %w", err)
	}
	podcasts := []models.Podcast{
		{
			BaseModel: models.BaseModel{ID: 1001, CreatedAt: fixed, UpdatedAt: fixed},
			XYZID:     "fixture-podcast-1001", Title: "Fixture：深度科技", FeedURL: "https://fixture.invalid/feeds/1001.xml",
			Description: "用于离线开发的确定性科技播客。", Author: "MagicPodcast Fixture",
			AddedDate: fixed, EpisodeCount: 2, NewestEpisodeDate: fixed.Add(-2 * time.Hour),
			IsSubscribed: true, FeedURLValid: true, DataSource: "fixture",
		},
		{
			BaseModel: models.BaseModel{ID: 1002, CreatedAt: fixed, UpdatedAt: fixed},
			XYZID:     "fixture-podcast-1002", Title: "Fixture：产品笔记", FeedURL: "https://fixture.invalid/feeds/1002.xml",
			Description: "用于验证节目列表和单集详情的确定性内容。", Author: "MagicPodcast Fixture",
			AddedDate: fixed, EpisodeCount: 1, NewestEpisodeDate: fixed.Add(-4 * time.Hour),
			IsSubscribed: true, FeedURLValid: true, DataSource: "fixture",
		},
	}
	if err := db.Session(&gorm.Session{SkipHooks: true}).Create(&podcasts).Error; err != nil {
		return fmt.Errorf("create fixture podcasts: %w", err)
	}

	fetched := fixed
	episodes := []models.Episode{
		{
			BaseModel: models.BaseModel{ID: 2001, CreatedAt: fixed, UpdatedAt: fixed},
			PodcastID: 1001, EpisodeNo: "01", Title: "Fixture 单集：离线开发",
			ShowNotes:     "<p>这是一条确定性 Show Notes，包含 <a href=\"https://example.invalid/report\">安全示例链接</a>。</p>",
			PublishedDate: fixed.Add(-2 * time.Hour), Duration: 2400,
			GUID: "fixture-episode-2001", FetchedAt: &fetched,
		},
		{
			BaseModel: models.BaseModel{ID: 2002, CreatedAt: fixed, UpdatedAt: fixed},
			PodcastID: 1001, EpisodeNo: "02", Title: "Fixture 单集：真实后端契约",
			ShowNotes:     "通过真实 Go API 提供，不使用前端 Mock。",
			PublishedDate: fixed.Add(-3 * time.Hour), Duration: 1800,
			GUID: "fixture-episode-2002", FetchedAt: &fetched,
		},
		{
			BaseModel: models.BaseModel{ID: 2003, CreatedAt: fixed, UpdatedAt: fixed},
			PodcastID: 1002, EpisodeNo: "A", Title: "Fixture 单集：稳定身份",
			ShowNotes:     "重复生成时保持相同节目、单集 ID 与 GUID。",
			PublishedDate: fixed.Add(-4 * time.Hour), Duration: 1500,
			GUID: "fixture-episode-2003", FetchedAt: &fetched,
		},
	}
	if err := db.Session(&gorm.Session{SkipHooks: true}).Create(&episodes).Error; err != nil {
		return fmt.Errorf("create fixture episodes: %w", err)
	}

	tags := []models.Tag{
		{ID: 3001, Name: "Fixture 科技", Color: "#5B6B8C", CreatedAt: fixed, UpdatedAt: fixed},
		{ID: 3002, Name: "Fixture 产品", Color: "#8A6F55", CreatedAt: fixed, UpdatedAt: fixed},
	}
	if err := db.Session(&gorm.Session{SkipHooks: true}).Create(&tags).Error; err != nil {
		return fmt.Errorf("create fixture tags: %w", err)
	}
	if err := db.Exec(
		"INSERT INTO podcasts_tags (podcast_id, tag_id) VALUES (?, ?), (?, ?)",
		1001, 3001, 1002, 3002,
	).Error; err != nil {
		return fmt.Errorf("create fixture tag relations: %w", err)
	}
	return nil
}
