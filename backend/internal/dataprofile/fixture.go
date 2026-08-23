package dataprofile

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"magicpodcast/internal/database"
)

const (
	FixtureSeries          = "complete-v2"
	DefaultFixtureScenario = "journey"

	FixtureScenarioEmpty          = "empty"
	FixtureScenarioFocusZero      = "focus-0"
	FixtureScenarioFocusSeven     = "focus-7"
	FixtureScenarioFocusOverLimit = "focus-over-limit"
	FixtureScenarioReportEmpty    = "report-empty"
	FixtureScenarioReportSingle   = "report-single"
)

var fixtureScenarioSet = map[string]struct{}{
	DefaultFixtureScenario:        {},
	FixtureScenarioEmpty:          {},
	FixtureScenarioFocusZero:      {},
	FixtureScenarioFocusSeven:     {},
	FixtureScenarioFocusOverLimit: {},
	FixtureScenarioReportEmpty:    {},
	FixtureScenarioReportSingle:   {},
}

type Fixture struct {
	Version      string
	Scenario     string
	AnchorAt     time.Time
	DatabasePath string
	ManifestPath string
	Manifest     Manifest
}

func CurrentFixtureVersion() string {
	version, _ := CurrentFixtureVersionForScenario(DefaultFixtureScenario)
	return version
}

func CurrentFixtureVersionForScenario(scenario string) (string, error) {
	return fixtureVersion(scenario, fixtureAnchor(time.Now()))
}

func SupportedFixtureScenarios() []string {
	return []string{
		DefaultFixtureScenario,
		FixtureScenarioEmpty,
		FixtureScenarioFocusZero,
		FixtureScenarioFocusSeven,
		FixtureScenarioFocusOverLimit,
		FixtureScenarioReportEmpty,
		FixtureScenarioReportSingle,
	}
}

func EnsureFixture(profileHome string) (Fixture, error) {
	return EnsureFixtureScenario(profileHome, DefaultFixtureScenario)
}

func EnsureFixtureScenario(profileHome, scenario string) (Fixture, error) {
	return ensureFixtureScenarioAt(profileHome, scenario, time.Now())
}

func ensureFixtureScenarioAt(profileHome, scenario string, now time.Time) (Fixture, error) {
	root, err := ensureRoot(profileHome)
	if err != nil {
		return Fixture{}, err
	}
	anchor := fixtureAnchor(now)
	version, err := fixtureVersion(scenario, anchor)
	if err != nil {
		return Fixture{}, err
	}
	fixtureDir := filepath.Join(root, "fixtures", version)
	databasePath := filepath.Join(fixtureDir, "magicpodcast.db")
	manifestPath := filepath.Join(fixtureDir, "manifest.json")

	if _, err := os.Stat(databasePath); err == nil {
		manifest, _, validationErr := ValidateManifest(databasePath, manifestPath, "fixture")
		if validationErr != nil {
			return Fixture{}, fmt.Errorf("existing fixture is invalid and was preserved: %w", validationErr)
		}
		if manifest.FixtureVersion != version ||
			manifest.FixtureScenario != scenario ||
			manifest.FixtureAnchorAt != anchor.Format(time.RFC3339) ||
			manifest.ID != version {
			return Fixture{}, fmt.Errorf("existing fixture metadata does not match %s", version)
		}
		return Fixture{
			Version:      version,
			Scenario:     scenario,
			AnchorAt:     anchor,
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
	if err := buildFixtureScenarioDatabase(tempDatabase, scenario, anchor); err != nil {
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
		FormatVersion:   ManifestFormatVersion,
		Kind:            "fixture",
		ID:              version,
		SchemaVersion:   status.SchemaVersion,
		FixtureVersion:  version,
		FixtureScenario: scenario,
		FixtureAnchorAt: anchor.Format(time.RFC3339),
		SHA256:          hash,
		Counts:          status.Counts,
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
		Scenario:     scenario,
		AnchorAt:     anchor,
		DatabasePath: databasePath,
		ManifestPath: manifestPath,
		Manifest:     manifest,
	}, nil
}

func fixtureVersion(scenario string, anchor time.Time) (string, error) {
	if _, ok := fixtureScenarioSet[scenario]; !ok {
		return "", fmt.Errorf(
			"unsupported fixture scenario %q (supported: %s)",
			scenario,
			strings.Join(SupportedFixtureScenarios(), ", "),
		)
	}
	return fmt.Sprintf(
		"%s-%s-%s-schema-%d",
		FixtureSeries,
		scenario,
		anchor.Format("20060102T15"),
		database.CurrentSchemaVersion,
	), nil
}

func fixtureAnchor(now time.Time) time.Time {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		location = time.FixedZone("Asia/Shanghai", 8*60*60)
	}
	local := now.In(location)
	return time.Date(
		local.Year(),
		local.Month(),
		local.Day(),
		local.Hour(),
		0,
		0,
		0,
		location,
	)
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
	return nil
}

func buildFixtureScenarioDatabase(path, scenario string, anchor time.Time) error {
	db, closeDB, err := openSQLite(path, false)
	if err != nil {
		return err
	}
	defer closeDB()
	if err := database.ApplyMigrations(db); err != nil {
		return fmt.Errorf("create fixture schema: %w", err)
	}

	if err := db.Exec("UPDATE schema_migrations SET applied_at = ?", anchor).Error; err != nil {
		return fmt.Errorf("stabilize fixture migration timestamps: %w", err)
	}
	if err := db.Exec("UPDATE consumption_queue_orders SET updated_at = ?", anchor).Error; err != nil {
		return fmt.Errorf("stabilize fixture queue order timestamps: %w", err)
	}
	if scenario == FixtureScenarioEmpty {
		return nil
	}
	return seedCompleteFixture(db, scenario, anchor)
}
