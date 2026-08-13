package runtimeprofile

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	ProfileUnmanaged  = "unmanaged"
	ProfileFixture    = "fixture"
	ProfileSnapshot   = "snapshot"
	ProfileProduction = "production"

	ProductionConfirmation = "I_UNDERSTAND_THIS_USES_PRODUCTION_DATA"
)

var safeIdentifier = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,127}$`)

type Metadata struct {
	Profile            string
	InstanceID         string
	FixtureVersion     string
	FixtureScenario    string
	FixtureAnchorAt    string
	SnapshotID         string
	SnapshotCapturedAt string
}

func Load(databasePath, serverMode string) (Metadata, error) {
	profile := strings.TrimSpace(os.Getenv("MAGICPODCAST_DATA_PROFILE"))
	if profile == "" {
		if serverMode == "release" {
			return Metadata{}, fmt.Errorf("release server mode requires an explicit production data profile")
		}
		return Metadata{Profile: ProfileUnmanaged}, nil
	}

	metadata := Metadata{
		Profile:            profile,
		InstanceID:         strings.TrimSpace(os.Getenv("MAGICPODCAST_DATA_PROFILE_INSTANCE_ID")),
		FixtureVersion:     strings.TrimSpace(os.Getenv("MAGICPODCAST_FIXTURE_VERSION")),
		FixtureScenario:    strings.TrimSpace(os.Getenv("MAGICPODCAST_FIXTURE_SCENARIO")),
		FixtureAnchorAt:    strings.TrimSpace(os.Getenv("MAGICPODCAST_FIXTURE_ANCHOR_AT")),
		SnapshotID:         strings.TrimSpace(os.Getenv("MAGICPODCAST_SNAPSHOT_ID")),
		SnapshotCapturedAt: strings.TrimSpace(os.Getenv("MAGICPODCAST_SNAPSHOT_CAPTURED_AT")),
	}

	switch profile {
	case ProfileFixture, ProfileSnapshot:
		if !safeIdentifier.MatchString(metadata.InstanceID) {
			return Metadata{}, fmt.Errorf("managed data profile requires a safe instance ID")
		}
		root := strings.TrimSpace(os.Getenv("MAGICPODCAST_DATA_PROFILE_HOME"))
		if root == "" {
			return Metadata{}, fmt.Errorf("managed data profile requires MAGICPODCAST_DATA_PROFILE_HOME")
		}
		if err := requirePathWithin(databasePath, filepath.Join(root, "work")); err != nil {
			return Metadata{}, fmt.Errorf("%s database path rejected: %w", profile, err)
		}
	case ProfileProduction:
		if strings.TrimSpace(databasePath) == "" {
			return Metadata{}, fmt.Errorf("production profile requires a database path")
		}
		if os.Getenv("MAGICPODCAST_PRODUCTION_PROFILE_CONFIRM") != ProductionConfirmation {
			return Metadata{}, fmt.Errorf("production profile requires the canonical production startup gate")
		}
		if serverMode != "release" {
			return Metadata{}, fmt.Errorf("production profile requires release server mode")
		}
		info, err := os.Lstat(databasePath)
		if err != nil {
			return Metadata{}, fmt.Errorf("production profile database unavailable: %w", err)
		}
		if !info.Mode().IsRegular() {
			return Metadata{}, fmt.Errorf("production profile database must be a regular file")
		}
	default:
		return Metadata{}, fmt.Errorf("unknown data profile %q", profile)
	}

	switch profile {
	case ProfileFixture:
		if !safeIdentifier.MatchString(metadata.FixtureVersion) {
			return Metadata{}, fmt.Errorf("fixture profile requires a safe fixture version")
		}
		legacy := strings.HasPrefix(metadata.FixtureVersion, "basic-v1")
		if legacy {
			if metadata.FixtureScenario != "" || metadata.FixtureAnchorAt != "" {
				return Metadata{}, fmt.Errorf("legacy fixture profile metadata is invalid")
			}
		} else {
			if !safeIdentifier.MatchString(metadata.FixtureScenario) {
				return Metadata{}, fmt.Errorf("fixture profile requires a safe fixture scenario")
			}
			if _, err := time.Parse(time.RFC3339, metadata.FixtureAnchorAt); err != nil {
				return Metadata{}, fmt.Errorf("fixture profile requires an RFC3339 anchor time: %w", err)
			}
		}
	case ProfileSnapshot:
		if !safeIdentifier.MatchString(metadata.SnapshotID) {
			return Metadata{}, fmt.Errorf("snapshot profile requires a safe snapshot ID")
		}
		if _, err := time.Parse(time.RFC3339, metadata.SnapshotCapturedAt); err != nil {
			return Metadata{}, fmt.Errorf("snapshot profile requires an RFC3339 capture time: %w", err)
		}
	}

	return metadata, nil
}

func (m Metadata) PublicFields() map[string]any {
	fields := map[string]any{
		"data_profile": m.Profile,
	}
	if m.InstanceID != "" {
		fields["data_profile_instance_id"] = m.InstanceID
	}
	if m.FixtureVersion != "" {
		fields["fixture_version"] = m.FixtureVersion
	}
	if m.FixtureScenario != "" {
		fields["fixture_scenario"] = m.FixtureScenario
	}
	if m.FixtureAnchorAt != "" {
		fields["fixture_anchor_at"] = m.FixtureAnchorAt
	}
	if m.SnapshotID != "" {
		fields["snapshot_id"] = m.SnapshotID
	}
	if m.SnapshotCapturedAt != "" {
		fields["snapshot_captured_at"] = m.SnapshotCapturedAt
	}
	return fields
}

func requirePathWithin(path, root string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect database path: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("database path must be a regular file")
	}
	pathAbs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve database path: %w", err)
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("resolve profile root: %w", err)
	}

	pathResolved, err := filepath.EvalSymlinks(pathAbs)
	if err != nil {
		return fmt.Errorf("resolve database symlinks: %w", err)
	}
	rootResolved, err := filepath.EvalSymlinks(rootAbs)
	if err != nil {
		return fmt.Errorf("resolve profile symlinks: %w", err)
	}
	relative, err := filepath.Rel(rootResolved, pathResolved)
	if err != nil {
		return fmt.Errorf("compare paths: %w", err)
	}
	if relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return fmt.Errorf("database must be a file below the managed %s directory", filepath.Base(rootResolved))
	}
	return nil
}
