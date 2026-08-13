package dataprofile

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var safeIdentifierPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,127}$`)

func isSafeIdentifier(value string) bool {
	return safeIdentifierPattern.MatchString(value)
}

func ensureRoot(root string) (string, error) {
	if strings.TrimSpace(root) == "" {
		return "", fmt.Errorf("profile home is empty")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve profile home: %w", err)
	}
	if err := os.MkdirAll(absolute, 0o700); err != nil {
		return "", fmt.Errorf("create profile home: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", fmt.Errorf("resolve profile home symlinks: %w", err)
	}
	return resolved, nil
}

func requireExistingPathWithin(path, root string) (string, error) {
	pathAbs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve path: %w", err)
	}
	pathResolved, err := filepath.EvalSymlinks(pathAbs)
	if err != nil {
		return "", fmt.Errorf("resolve path symlinks: %w", err)
	}
	rootResolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("resolve root symlinks: %w", err)
	}
	relative, err := filepath.Rel(rootResolved, pathResolved)
	if err != nil {
		return "", fmt.Errorf("compare managed path: %w", err)
	}
	if relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path is outside the managed directory")
	}
	return pathResolved, nil
}

func requireRegularFileWithin(path, root string) (string, error) {
	resolved, err := requireExistingPathWithin(path, root)
	if err != nil {
		return "", err
	}
	info, err := os.Lstat(resolved)
	if err != nil {
		return "", fmt.Errorf("inspect managed file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("managed path is not a regular file")
	}
	return resolved, nil
}

func requireRealDirectory(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("directory path is empty")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve directory path: %w", err)
	}
	info, err := os.Lstat(absolute)
	if err != nil {
		return "", fmt.Errorf("inspect directory: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("path must be a real directory, not a symlink or file")
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", fmt.Errorf("resolve directory symlinks: %w", err)
	}
	resolvedInfo, err := os.Lstat(resolved)
	if err != nil {
		return "", fmt.Errorf("inspect resolved directory: %w", err)
	}
	if !resolvedInfo.IsDir() {
		return "", fmt.Errorf("resolved path is not a real directory")
	}
	return resolved, nil
}

func requireAbsoluteExecutable(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("executable path must be absolute")
	}
	absolute := filepath.Clean(path)
	info, err := os.Lstat(absolute)
	if err != nil {
		return "", fmt.Errorf("inspect executable: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("executable must be a regular file, not a symlink")
	}
	if info.Mode().Perm()&0o111 == 0 {
		return "", fmt.Errorf("executable file is not executable")
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", fmt.Errorf("resolve executable path: %w", err)
	}
	if resolved != absolute {
		return "", fmt.Errorf("executable path must be canonical and contain no symlink components")
	}
	return resolved, nil
}
