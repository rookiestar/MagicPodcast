package dataprofile

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"magicpodcast/internal/database"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const ManifestFormatVersion = 1

type Manifest struct {
	FormatVersion    int              `json:"format_version"`
	Kind             string           `json:"kind"`
	ID               string           `json:"id"`
	SchemaVersion    int              `json:"schema_version"`
	FixtureVersion   string           `json:"fixture_version,omitempty"`
	CapturedAt       string           `json:"captured_at,omitempty"`
	SanitizerVersion string           `json:"sanitizer_version,omitempty"`
	SHA256           string           `json:"sha256"`
	Counts           map[string]int64 `json:"counts,omitempty"`
}

type DatabaseStatus struct {
	SchemaVersion int
	Counts        map[string]int64
}

func ValidateDatabase(path string) (DatabaseStatus, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return DatabaseStatus{}, fmt.Errorf("database file unavailable: %w", err)
	}
	if !info.Mode().IsRegular() {
		return DatabaseStatus{}, fmt.Errorf("database path is not a regular file")
	}

	db, closeDB, err := openSQLite(path, true)
	if err != nil {
		return DatabaseStatus{}, err
	}
	defer closeDB()

	var integrity string
	if err := db.Raw("PRAGMA integrity_check").Scan(&integrity).Error; err != nil {
		return DatabaseStatus{}, fmt.Errorf("sqlite integrity check failed: %w", err)
	}
	if integrity != "ok" {
		return DatabaseStatus{}, fmt.Errorf("sqlite integrity check returned %q", integrity)
	}

	var foreignKeyProblems int64
	if err := db.Raw("SELECT COUNT(*) FROM pragma_foreign_key_check").Scan(&foreignKeyProblems).Error; err != nil {
		return DatabaseStatus{}, fmt.Errorf("sqlite foreign key check failed: %w", err)
	}
	if foreignKeyProblems != 0 {
		return DatabaseStatus{}, fmt.Errorf("sqlite foreign key check found %d problem(s)", foreignKeyProblems)
	}

	schema, err := database.InspectSchema(db)
	if err != nil {
		return DatabaseStatus{}, err
	}
	if !schema.MigrationTablePresent {
		return DatabaseStatus{}, fmt.Errorf("schema_migrations is missing")
	}
	if len(schema.RequiredTablesMissing) != 0 {
		return DatabaseStatus{}, fmt.Errorf("required tables missing: %v", schema.RequiredTablesMissing)
	}
	if schema.CurrentVersion != database.CurrentSchemaVersion || len(schema.Pending) != 0 {
		return DatabaseStatus{}, fmt.Errorf(
			"schema version %d is not current version %d",
			schema.CurrentVersion,
			database.CurrentSchemaVersion,
		)
	}

	counts := make(map[string]int64)
	for _, table := range []string{"podcasts", "episodes", "tags", "workflows", "reports", "episode_triage_decisions"} {
		var count int64
		if err := db.Table(table).Count(&count).Error; err != nil {
			return DatabaseStatus{}, fmt.Errorf("count %s: %w", table, err)
		}
		counts[table] = count
	}
	return DatabaseStatus{SchemaVersion: schema.CurrentVersion, Counts: counts}, nil
}

func ReadManifest(path string) (Manifest, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("inspect manifest: %w", err)
	}
	if !info.Mode().IsRegular() {
		return Manifest{}, fmt.Errorf("manifest is not a regular file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("read manifest: %w", err)
	}
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode manifest: %w", err)
	}
	if manifest.FormatVersion != ManifestFormatVersion {
		return Manifest{}, fmt.Errorf("unsupported manifest format %d", manifest.FormatVersion)
	}
	if manifest.SchemaVersion != database.CurrentSchemaVersion {
		return Manifest{}, fmt.Errorf(
			"manifest schema version %d is not current version %d",
			manifest.SchemaVersion,
			database.CurrentSchemaVersion,
		)
	}
	return manifest, nil
}

func ValidateManifest(databasePath, manifestPath, expectedKind string) (Manifest, DatabaseStatus, error) {
	manifest, err := ReadManifest(manifestPath)
	if err != nil {
		return Manifest{}, DatabaseStatus{}, err
	}
	if manifest.Kind != expectedKind {
		return Manifest{}, DatabaseStatus{}, fmt.Errorf("manifest kind %q, expected %q", manifest.Kind, expectedKind)
	}
	if !isSafeIdentifier(manifest.ID) {
		return Manifest{}, DatabaseStatus{}, fmt.Errorf("manifest ID is invalid")
	}
	actualSHA, err := SHA256File(databasePath)
	if err != nil {
		return Manifest{}, DatabaseStatus{}, err
	}
	if manifest.SHA256 == "" || manifest.SHA256 != actualSHA {
		return Manifest{}, DatabaseStatus{}, fmt.Errorf("database checksum does not match manifest")
	}
	status, err := ValidateDatabase(databasePath)
	if err != nil {
		return Manifest{}, DatabaseStatus{}, err
	}
	if status.SchemaVersion != manifest.SchemaVersion {
		return Manifest{}, DatabaseStatus{}, fmt.Errorf("database schema does not match manifest")
	}
	for table, actual := range status.Counts {
		expected, found := manifest.Counts[table]
		if !found {
			return Manifest{}, DatabaseStatus{}, fmt.Errorf(
				"manifest does not record required table count for %s",
				table,
			)
		}
		if actual != expected {
			return Manifest{}, DatabaseStatus{}, fmt.Errorf(
				"database count for %s is %d, manifest records %d",
				table,
				actual,
				expected,
			)
		}
	}
	return manifest, status, nil
}

func WriteManifest(path string, manifest Manifest) error {
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("encode manifest: %w", err)
	}
	data = append(data, '\n')
	return writeFileAtomic(path, data, 0o600)
}

func SHA256File(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", fmt.Errorf("inspect file for checksum: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("file for checksum is not a regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open file for checksum: %w", err)
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("hash file: %w", err)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func openSQLite(path string, readOnly bool) (*gorm.DB, func(), error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, nil, fmt.Errorf("resolve database path: %w", err)
	}
	databaseURL := (&url.URL{Scheme: "file", Path: absolute}).String()
	query := "?_foreign_keys=on&_busy_timeout=5000&_journal_mode=DELETE"
	if readOnly {
		query = "?mode=ro&_query_only=1&_foreign_keys=on&_busy_timeout=5000"
	}
	db, err := gorm.Open(sqlite.Open(databaseURL+query), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: false,
		SkipDefaultTransaction:                   true,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("open sqlite database: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, nil, fmt.Errorf("get sqlite connection: %w", err)
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	return db, func() { _ = sqlDB.Close() }, nil
}

func openSQLDatabase(path string, readOnly bool) (*sql.DB, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve database path: %w", err)
	}
	databaseURL := (&url.URL{Scheme: "file", Path: absolute}).String()
	query := "?_foreign_keys=on&_busy_timeout=5000"
	if readOnly {
		query = "?mode=ro&_query_only=1&_foreign_keys=on&_busy_timeout=5000"
	}
	db, err := sql.Open("sqlite3", databaseURL+query)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	return db, nil
}

func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create parent directory: %w", err)
	}
	temp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temporary file: %w", err)
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(mode); err != nil {
		_ = temp.Close()
		return fmt.Errorf("set temporary file mode: %w", err)
	}
	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return fmt.Errorf("write temporary file: %w", err)
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return fmt.Errorf("sync temporary file: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close temporary file: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("publish file: %w", err)
	}
	return nil
}

func parseRFC3339(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid RFC3339 time %q: %w", value, err)
	}
	return parsed, nil
}
