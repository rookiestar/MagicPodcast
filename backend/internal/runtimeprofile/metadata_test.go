package runtimeprofile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadFixtureRequiresManagedDatabase(t *testing.T) {
	root := t.TempDir()
	fixtureDir := filepath.Join(root, "work", "fixture-instance")
	require.NoError(t, os.MkdirAll(fixtureDir, 0o700))
	fixturePath := filepath.Join(fixtureDir, "basic-v1.db")
	require.NoError(t, os.WriteFile(fixturePath, []byte("fixture"), 0o600))

	t.Setenv("MAGICPODCAST_DATA_PROFILE", ProfileFixture)
	t.Setenv("MAGICPODCAST_DATA_PROFILE_HOME", root)
	t.Setenv("MAGICPODCAST_DATA_PROFILE_INSTANCE_ID", "fixture-instance")
	t.Setenv("MAGICPODCAST_FIXTURE_VERSION", "basic-v1")

	metadata, err := Load(fixturePath, "debug")
	require.NoError(t, err)
	require.Equal(t, ProfileFixture, metadata.Profile)
	require.Equal(t, "basic-v1", metadata.FixtureVersion)

	outside := filepath.Join(t.TempDir(), "production.db")
	require.NoError(t, os.WriteFile(outside, []byte("production"), 0o600))
	_, err = Load(outside, "debug")
	require.ErrorContains(t, err, "database path rejected")
}

func TestLoadCompleteFixtureExposesScenarioAndAnchor(t *testing.T) {
	root := t.TempDir()
	fixtureDir := filepath.Join(root, "work", "fixture-instance")
	require.NoError(t, os.MkdirAll(fixtureDir, 0o700))
	fixturePath := filepath.Join(fixtureDir, "complete-v1.db")
	require.NoError(t, os.WriteFile(fixturePath, []byte("fixture"), 0o600))

	t.Setenv("MAGICPODCAST_DATA_PROFILE", ProfileFixture)
	t.Setenv("MAGICPODCAST_DATA_PROFILE_HOME", root)
	t.Setenv("MAGICPODCAST_DATA_PROFILE_INSTANCE_ID", "fixture-instance")
	t.Setenv("MAGICPODCAST_FIXTURE_VERSION", "complete-v1-journey-schema-17")
	t.Setenv("MAGICPODCAST_FIXTURE_SCENARIO", "journey")
	t.Setenv("MAGICPODCAST_FIXTURE_ANCHOR_AT", "2026-08-14T10:00:00+08:00")

	metadata, err := Load(fixturePath, "debug")
	require.NoError(t, err)
	require.Equal(t, "journey", metadata.FixtureScenario)
	require.Equal(t, "2026-08-14T10:00:00+08:00", metadata.FixtureAnchorAt)
	require.Equal(t, map[string]any{
		"data_profile":             ProfileFixture,
		"data_profile_instance_id": "fixture-instance",
		"fixture_version":          "complete-v1-journey-schema-17",
		"fixture_scenario":         "journey",
		"fixture_anchor_at":        "2026-08-14T10:00:00+08:00",
	}, metadata.PublicFields())
}

func TestLoadCompleteFixtureRequiresScenarioAndAnchor(t *testing.T) {
	root := t.TempDir()
	fixtureDir := filepath.Join(root, "work", "fixture-instance")
	require.NoError(t, os.MkdirAll(fixtureDir, 0o700))
	fixturePath := filepath.Join(fixtureDir, "complete-v1.db")
	require.NoError(t, os.WriteFile(fixturePath, []byte("fixture"), 0o600))

	t.Setenv("MAGICPODCAST_DATA_PROFILE", ProfileFixture)
	t.Setenv("MAGICPODCAST_DATA_PROFILE_HOME", root)
	t.Setenv("MAGICPODCAST_DATA_PROFILE_INSTANCE_ID", "fixture-instance")
	t.Setenv("MAGICPODCAST_FIXTURE_VERSION", "complete-v1-journey-schema-17")

	_, err := Load(fixturePath, "debug")
	require.ErrorContains(t, err, "safe fixture scenario")

	t.Setenv("MAGICPODCAST_FIXTURE_SCENARIO", "journey")
	t.Setenv("MAGICPODCAST_FIXTURE_ANCHOR_AT", "not-a-time")
	_, err = Load(fixturePath, "debug")
	require.ErrorContains(t, err, "RFC3339 anchor time")
}

func TestLoadFixtureRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	fixtureDir := filepath.Join(root, "work")
	require.NoError(t, os.MkdirAll(fixtureDir, 0o700))
	outside := filepath.Join(t.TempDir(), "production.db")
	require.NoError(t, os.WriteFile(outside, []byte("production"), 0o600))
	link := filepath.Join(fixtureDir, "fixture.db")
	require.NoError(t, os.Symlink(outside, link))

	t.Setenv("MAGICPODCAST_DATA_PROFILE", ProfileFixture)
	t.Setenv("MAGICPODCAST_DATA_PROFILE_HOME", root)
	t.Setenv("MAGICPODCAST_DATA_PROFILE_INSTANCE_ID", "fixture-instance")
	t.Setenv("MAGICPODCAST_FIXTURE_VERSION", "basic-v1")

	_, err := Load(link, "debug")
	require.ErrorContains(t, err, "database path rejected")
}

func TestLoadManagedProfileRequiresRegularDatabaseFile(t *testing.T) {
	root := t.TempDir()
	databaseDirectory := filepath.Join(root, "work", "fixture-instance", "database.db")
	require.NoError(t, os.MkdirAll(databaseDirectory, 0o700))
	t.Setenv("MAGICPODCAST_DATA_PROFILE", ProfileFixture)
	t.Setenv("MAGICPODCAST_DATA_PROFILE_HOME", root)
	t.Setenv("MAGICPODCAST_DATA_PROFILE_INSTANCE_ID", "fixture-instance")
	t.Setenv("MAGICPODCAST_FIXTURE_VERSION", "basic-v1")

	_, err := Load(databaseDirectory, "debug")
	require.ErrorContains(t, err, "regular file")
}

func TestLoadProductionRequiresCanonicalReleaseGate(t *testing.T) {
	productionPath := filepath.Join(t.TempDir(), "magicpodcast.db")
	require.NoError(t, os.WriteFile(productionPath, []byte("production"), 0o600))
	t.Setenv("MAGICPODCAST_DATA_PROFILE", ProfileProduction)

	_, err := Load(productionPath, "release")
	require.ErrorContains(t, err, "canonical production startup gate")

	t.Setenv("MAGICPODCAST_PRODUCTION_PROFILE_CONFIRM", ProductionConfirmation)
	_, err = Load(productionPath, "debug")
	require.ErrorContains(t, err, "release server mode")

	metadata, err := Load(productionPath, "release")
	require.NoError(t, err)
	require.Equal(t, ProfileProduction, metadata.Profile)

	_, err = Load(filepath.Join(t.TempDir(), "missing.db"), "release")
	require.ErrorContains(t, err, "database unavailable")

	directory := t.TempDir()
	_, err = Load(directory, "release")
	require.ErrorContains(t, err, "regular file")

	symlink := filepath.Join(t.TempDir(), "database-link")
	require.NoError(t, os.Symlink(productionPath, symlink))
	_, err = Load(symlink, "release")
	require.ErrorContains(t, err, "regular file")
}

func TestLoadUnmanagedRemainsBackwardCompatible(t *testing.T) {
	t.Setenv("MAGICPODCAST_DATA_PROFILE", "")

	metadata, err := Load("./data/magicpodcast.db", "debug")
	require.NoError(t, err)
	require.Equal(t, ProfileUnmanaged, metadata.Profile)
	require.Equal(t, map[string]any{"data_profile": ProfileUnmanaged}, metadata.PublicFields())
}

func TestLoadReleaseRequiresExplicitProductionProfile(t *testing.T) {
	t.Setenv("MAGICPODCAST_DATA_PROFILE", "")

	_, err := Load("./data/magicpodcast.db", "release")
	require.ErrorContains(t, err, "requires an explicit production data profile")
}
