package processing

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type DiskArtifactStore struct {
	root string
}

type artifactManifest struct {
	SchemaVersion        string            `json:"schema_version"`
	RunID                uint              `json:"run_id"`
	EpisodeID            uint              `json:"episode_id"`
	AudioDigest          string            `json:"audio_digest"`
	PipelineVersion      string            `json:"pipeline_version"`
	TranscriptionAdapter string            `json:"transcription_adapter"`
	TranscriptionVersion string            `json:"transcription_version"`
	RuntimeAdapter       string            `json:"runtime_adapter"`
	RuntimeVersion       string            `json:"runtime_version"`
	PromptVersion        string            `json:"prompt_version"`
	SkillVersions        map[string]string `json:"skill_versions"`
	GeneratedAt          string            `json:"generated_at"`
	Sources              map[string]string `json:"sources"`
	Files                []artifactFile    `json:"files"`
}

type artifactFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

func NewDiskArtifactStore(root string) (*DiskArtifactStore, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, fmt.Errorf("%w: root is required", ErrInvalidArtifact)
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve artifact root: %w", err)
	}
	if err := os.MkdirAll(absolute, 0o700); err != nil {
		return nil, fmt.Errorf("create artifact root: %w", err)
	}
	if err := os.Chmod(absolute, 0o700); err != nil {
		return nil, fmt.Errorf("protect artifact root: %w", err)
	}
	canonical, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return nil, fmt.Errorf("resolve canonical artifact root: %w", err)
	}
	return &DiskArtifactStore{root: filepath.Clean(canonical)}, nil
}

func (s *DiskArtifactStore) Publish(
	ctx context.Context,
	request ArtifactPublishRequest,
) (ArtifactPublishResult, error) {
	if err := ctx.Err(); err != nil {
		return ArtifactPublishResult{}, err
	}
	if request.RunID == 0 || request.EpisodeID == 0 ||
		strings.TrimSpace(request.AudioDigest) == "" ||
		strings.TrimSpace(request.PipelineVersion) == "" ||
		strings.TrimSpace(request.Transcript) == "" ||
		strings.TrimSpace(request.EpisodeNotes) == "" ||
		strings.TrimSpace(request.TranscriptionAdapter) == "" ||
		strings.TrimSpace(request.TranscriptionVersion) == "" ||
		strings.TrimSpace(request.RuntimeAdapter) == "" ||
		strings.TrimSpace(request.RuntimeVersion) == "" ||
		strings.TrimSpace(request.PromptVersion) == "" ||
		request.GeneratedAt.IsZero() ||
		!validVersionMap(request.SkillVersions) {
		return ArtifactPublishResult{}, fmt.Errorf("%w: required content is missing", ErrInvalidArtifact)
	}

	setsRoot := filepath.Join(
		s.root,
		"episodes",
		fmt.Sprintf("%d", request.EpisodeID),
		"sets",
	)
	if err := os.MkdirAll(setsRoot, 0o700); err != nil {
		return ArtifactPublishResult{}, fmt.Errorf("create artifact set root: %w", err)
	}
	finalPath := filepath.Join(setsRoot, fmt.Sprintf("run-%d", request.RunID))
	if _, err := os.Stat(finalPath); err == nil {
		return ArtifactPublishResult{}, ErrArtifactExists
	} else if !os.IsNotExist(err) {
		return ArtifactPublishResult{}, fmt.Errorf("inspect artifact destination: %w", err)
	}

	stagingPath, err := os.MkdirTemp(setsRoot, fmt.Sprintf(".run-%d-", request.RunID))
	if err != nil {
		return ArtifactPublishResult{}, fmt.Errorf("create artifact staging directory: %w", err)
	}
	published := false
	defer func() {
		if !published {
			_ = os.RemoveAll(stagingPath)
		}
	}()
	if err := os.Chmod(stagingPath, 0o700); err != nil {
		return ArtifactPublishResult{}, fmt.Errorf("protect artifact staging directory: %w", err)
	}

	files := make([]artifactFile, 0, len(request.RawArtifacts)+2)
	transcriptHash, err := writeArtifactFile(stagingPath, "transcript.md", []byte(request.Transcript))
	if err != nil {
		return ArtifactPublishResult{}, err
	}
	files = append(files, artifactFile{Path: "transcript.md", SHA256: transcriptHash})
	notesHash, err := writeArtifactFile(stagingPath, "episode-notes.md", []byte(request.EpisodeNotes))
	if err != nil {
		return ArtifactPublishResult{}, err
	}
	files = append(files, artifactFile{Path: "episode-notes.md", SHA256: notesHash})

	rawRoot := filepath.Join(stagingPath, "raw")
	if err := os.Mkdir(rawRoot, 0o700); err != nil {
		return ArtifactPublishResult{}, fmt.Errorf("create raw artifact directory: %w", err)
	}
	rawNames := make([]string, 0, len(request.RawArtifacts))
	for name := range request.RawArtifacts {
		rawNames = append(rawNames, name)
	}
	sort.Strings(rawNames)
	for _, name := range rawNames {
		if name == "" || filepath.Base(name) != name || name == "." || name == ".." {
			return ArtifactPublishResult{}, fmt.Errorf("%w: invalid raw artifact name", ErrInvalidArtifact)
		}
		relativePath := filepath.Join("raw", name)
		hash, err := writeArtifactFile(stagingPath, relativePath, request.RawArtifacts[name])
		if err != nil {
			return ArtifactPublishResult{}, err
		}
		files = append(files, artifactFile{Path: filepath.ToSlash(relativePath), SHA256: hash})
	}

	manifest := artifactManifest{
		SchemaVersion:        "1.0.0",
		RunID:                request.RunID,
		EpisodeID:            request.EpisodeID,
		AudioDigest:          request.AudioDigest,
		PipelineVersion:      request.PipelineVersion,
		TranscriptionAdapter: request.TranscriptionAdapter,
		TranscriptionVersion: request.TranscriptionVersion,
		RuntimeAdapter:       request.RuntimeAdapter,
		RuntimeVersion:       request.RuntimeVersion,
		PromptVersion:        request.PromptVersion,
		SkillVersions:        cloneStringMap(request.SkillVersions),
		GeneratedAt:          request.GeneratedAt.UTC().Format("2006-01-02T15:04:05.000000000Z"),
		Sources:              cloneStringMap(request.Sources),
		Files:                files,
	}
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return ArtifactPublishResult{}, fmt.Errorf("encode artifact manifest: %w", err)
	}
	manifestBytes = append(manifestBytes, '\n')
	manifestHash, err := writeArtifactFile(stagingPath, "manifest.json", manifestBytes)
	if err != nil {
		return ArtifactPublishResult{}, err
	}

	if err := ctx.Err(); err != nil {
		return ArtifactPublishResult{}, err
	}
	if err := os.Rename(stagingPath, finalPath); err != nil {
		return ArtifactPublishResult{}, fmt.Errorf("publish artifact set: %w", err)
	}
	published = true
	return ArtifactPublishResult{
		RootPath:         finalPath,
		ManifestPath:     "manifest.json",
		ManifestSHA256:   manifestHash,
		TranscriptSHA256: transcriptHash,
		NotesSHA256:      notesHash,
	}, nil
}

// Discard removes only an unrecorded set previously returned by Publish.
// The manifest hash and run-owned path are verified before deletion so a
// failed database commit cannot turn cleanup into an arbitrary path delete.
func (s *DiskArtifactStore) Discard(
	ctx context.Context,
	published ArtifactPublishResult,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	root := filepath.Clean(published.RootPath)
	relative, err := filepath.Rel(s.root, root)
	if err != nil ||
		relative == "." ||
		relative == ".." ||
		strings.HasPrefix(relative, ".."+string(filepath.Separator)) ||
		published.ManifestPath != "manifest.json" ||
		len(published.ManifestSHA256) != sha256.Size*2 {
		return fmt.Errorf("%w: published artifact identity is invalid", ErrInvalidArtifact)
	}

	rootInfo, err := os.Lstat(root)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect published artifact set: %w", err)
	}
	if !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%w: published artifact root is not an owned directory", ErrInvalidArtifact)
	}
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("resolve published artifact set: %w", err)
	}
	if filepath.Clean(canonicalRoot) != root {
		return fmt.Errorf("%w: published artifact path contains a symbolic link", ErrInvalidArtifact)
	}

	manifestPath := filepath.Join(root, published.ManifestPath)
	manifestInfo, err := os.Lstat(manifestPath)
	if err != nil {
		return fmt.Errorf("inspect published artifact manifest: %w", err)
	}
	if !manifestInfo.Mode().IsRegular() || manifestInfo.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%w: published artifact manifest is not a regular file", ErrInvalidArtifact)
	}
	manifestBytes, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("read published artifact manifest: %w", err)
	}
	manifestHash := sha256.Sum256(manifestBytes)
	if hex.EncodeToString(manifestHash[:]) != published.ManifestSHA256 {
		return fmt.Errorf("%w: published artifact manifest hash does not match", ErrInvalidArtifact)
	}
	var manifest artifactManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return fmt.Errorf("%w: published artifact manifest is invalid", ErrInvalidArtifact)
	}
	expectedRelative := filepath.Join(
		"episodes",
		fmt.Sprintf("%d", manifest.EpisodeID),
		"sets",
		fmt.Sprintf("run-%d", manifest.RunID),
	)
	if relative != expectedRelative {
		return fmt.Errorf("%w: published artifact path does not match its manifest", ErrInvalidArtifact)
	}
	if err := os.RemoveAll(root); err != nil {
		return fmt.Errorf("discard unrecorded artifact set: %w", err)
	}
	return nil
}

func writeArtifactFile(root, relativePath string, content []byte) (string, error) {
	fullPath := filepath.Join(root, relativePath)
	relative, err := filepath.Rel(root, fullPath)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: artifact path escapes root", ErrInvalidArtifact)
	}
	if err := os.WriteFile(fullPath, content, 0o600); err != nil {
		return "", fmt.Errorf("write artifact %s: %w", relativePath, err)
	}
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:]), nil
}

func cloneStringMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return map[string]string{}
	}
	output := make(map[string]string, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

func validVersionMap(input map[string]string) bool {
	for name, version := range input {
		if strings.TrimSpace(name) == "" || strings.TrimSpace(version) == "" {
			return false
		}
	}
	return true
}
