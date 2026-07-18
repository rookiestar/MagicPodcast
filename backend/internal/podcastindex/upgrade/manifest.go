package upgrade

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func NewManifest(scope string) Manifest {
	now := time.Now().UTC()
	return Manifest{
		SchemaVersion: ManifestSchemaVersion,
		RunID:         now.Format("20060102T150405Z"),
		Scope:         scope,
		CreatedAt:     now,
		UpdatedAt:     now,
		Decision: Decision{
			CheckedAt: now,
		},
		Source: SourceManifest{
			URL:                         DefaultDownloadURL,
			LicensingNote:               "保留 PodcastIndex 官方来源记录，并遵守适用的许可与服务条款。",
			ThirdPartyContentConstraint: "仅供 MagicPodcast 内部候选目录使用，不公开镜像或重新分发 Feed 内容。",
		},
	}
}

func ReadManifest(path string) (Manifest, error) {
	file, err := os.Open(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("open manifest: %w", err)
	}
	defer file.Close()
	var manifest Manifest
	if err := json.NewDecoder(file).Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode manifest: %w", err)
	}
	if manifest.SchemaVersion != ManifestSchemaVersion {
		return Manifest{}, fmt.Errorf("unsupported manifest schema version %d", manifest.SchemaVersion)
	}
	return manifest, nil
}

func WriteManifestAtomic(path string, manifest Manifest) error {
	manifest.UpdatedAt = time.Now().UTC()
	if manifest.CreatedAt.IsZero() {
		manifest.CreatedAt = manifest.UpdatedAt
	}
	if manifest.SchemaVersion == 0 {
		manifest.SchemaVersion = ManifestSchemaVersion
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create manifest directory: %w", err)
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".manifest-*.partial")
	if err != nil {
		return fmt.Errorf("create manifest temporary file: %w", err)
	}
	temporaryPath := file.Name()
	defer func() {
		file.Close()
		os.Remove(temporaryPath)
	}()
	if err := file.Chmod(0o600); err != nil {
		return fmt.Errorf("protect manifest: %w", err)
	}
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(manifest); err != nil {
		return fmt.Errorf("encode manifest: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync manifest: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close manifest: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("promote manifest: %w", err)
	}
	return nil
}

func WriteJSONAtomic(path string, value any, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create JSON directory: %w", err)
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".json-*.partial")
	if err != nil {
		return fmt.Errorf("create JSON temporary file: %w", err)
	}
	temporaryPath := file.Name()
	defer func() {
		file.Close()
		os.Remove(temporaryPath)
	}()
	if err := file.Chmod(mode); err != nil {
		return fmt.Errorf("protect JSON file: %w", err)
	}
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return fmt.Errorf("encode JSON: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync JSON: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close JSON: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("promote JSON: %w", err)
	}
	return nil
}

func AddBlocker(manifest *Manifest, reason string) {
	if manifest == nil || reason == "" {
		return
	}
	for _, existing := range manifest.Blockers {
		if existing == reason {
			return
		}
	}
	manifest.Blockers = append(manifest.Blockers, reason)
	manifest.Decision.Go = false
	manifest.Decision.Reasons = append(manifest.Decision.Reasons, reason)
}
