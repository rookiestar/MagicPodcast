package upgrade

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

func ValidateArchive(archivePath string) (ArchiveInspection, error) {
	inspection := ArchiveInspection{ArchivePath: archivePath}
	file, err := os.Open(archivePath)
	if err != nil {
		return inspection, fmt.Errorf("open archive: %w", err)
	}
	defer file.Close()

	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return inspection, fmt.Errorf("invalid gzip archive: %w", err)
	}
	inspection.GzipValid = true
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return inspection, fmt.Errorf("invalid tar archive: %w", err)
		}
		entry, err := validateTarHeader(header)
		if err != nil {
			return inspection, err
		}
		inspection.Entries = append(inspection.Entries, entry)
		if len(inspection.Entries) > 1 {
			return inspection, fmt.Errorf("archive contains unexpected extra file %q", header.Name)
		}
		if _, err := io.Copy(io.Discard, tarReader); err != nil {
			return inspection, fmt.Errorf("read archive entry %q: %w", header.Name, err)
		}
		inspection.ExtractedBytes += header.Size
	}
	if err := gzipReader.Close(); err != nil {
		return inspection, fmt.Errorf("gzip checksum validation failed: %w", err)
	}
	if len(inspection.Entries) != 1 {
		return inspection, fmt.Errorf("archive must contain exactly one SQLite file, found %d entries", len(inspection.Entries))
	}
	inspection.TarValid = true
	inspection.DatabaseEntry = inspection.Entries[0]
	inspection.ExpectedDatabase = inspection.DatabaseEntry.Name
	return inspection, nil
}

func validateTarHeader(header *tar.Header) (ArchiveEntry, error) {
	if header == nil {
		return ArchiveEntry{}, fmt.Errorf("archive contains an empty header")
	}
	if err := validateArchiveName(header.Name); err != nil {
		return ArchiveEntry{}, err
	}
	if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA {
		return ArchiveEntry{}, fmt.Errorf("archive entry %q has unsafe type %q; symlinks, hard links and directories are rejected", header.Name, header.Typeflag)
	}
	if header.Size <= 0 {
		return ArchiveEntry{}, fmt.Errorf("archive entry %q is empty", header.Name)
	}
	if header.Mode&0o111 != 0 {
		return ArchiveEntry{}, fmt.Errorf("archive entry %q is executable", header.Name)
	}
	name := strings.ToLower(header.Name)
	if !strings.HasSuffix(name, ".db") && !strings.HasSuffix(name, ".sqlite") && !strings.HasSuffix(name, ".sqlite3") {
		return ArchiveEntry{}, fmt.Errorf("archive entry %q is not an expected SQLite file", header.Name)
	}
	return ArchiveEntry{
		Name:      header.Name,
		SizeBytes: header.Size,
		Mode:      header.Mode,
		Type:      "regular",
	}, nil
}

func validateArchiveName(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("archive contains an empty path")
	}
	if strings.ContainsRune(name, '\x00') || strings.HasPrefix(name, "/") || filepath.IsAbs(name) {
		return fmt.Errorf("archive path %q is absolute or malformed", name)
	}
	normalized := strings.ReplaceAll(name, "\\", "/")
	if strings.HasPrefix(normalized, "./") {
		normalized = strings.TrimPrefix(normalized, "./")
	}
	clean := path.Clean(normalized)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return fmt.Errorf("archive path %q traverses outside staging", name)
	}
	if clean != normalized || strings.Contains(normalized, "/") || strings.Contains(normalized, "\\") {
		return fmt.Errorf("archive path %q is not a plain relative file name", name)
	}
	if strings.Contains(name, ":") {
		return fmt.Errorf("archive path %q contains a drive-style prefix", name)
	}
	return nil
}

func ExtractArchive(archivePath, destinationDir string) (string, error) {
	inspection, err := ValidateArchive(archivePath)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(destinationDir, 0o700); err != nil {
		return "", fmt.Errorf("create extraction directory: %w", err)
	}
	entries, err := os.ReadDir(destinationDir)
	if err != nil {
		return "", fmt.Errorf("read extraction directory: %w", err)
	}
	if len(entries) != 0 {
		return "", fmt.Errorf("extraction directory %s is not empty", destinationDir)
	}

	file, err := os.Open(archivePath)
	if err != nil {
		return "", fmt.Errorf("open archive for extraction: %w", err)
	}
	defer file.Close()
	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return "", fmt.Errorf("open gzip for extraction: %w", err)
	}
	defer gzipReader.Close()
	tarReader := tar.NewReader(gzipReader)
	header, err := tarReader.Next()
	if err != nil {
		return "", fmt.Errorf("read SQLite entry: %w", err)
	}
	if header.Name != inspection.DatabaseEntry.Name {
		return "", fmt.Errorf("archive entry changed between validation and extraction")
	}

	databasePath := filepath.Join(destinationDir, filepath.Base(header.Name))
	out, err := os.OpenFile(databasePath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", fmt.Errorf("create extracted database: %w", err)
	}
	if _, err := io.Copy(out, tarReader); err != nil {
		out.Close()
		return "", fmt.Errorf("extract database: %w", err)
	}
	if err := out.Sync(); err != nil {
		out.Close()
		return "", fmt.Errorf("sync extracted database: %w", err)
	}
	if err := out.Close(); err != nil {
		return "", fmt.Errorf("close extracted database: %w", err)
	}

	if err := verifySQLiteHeader(databasePath); err != nil {
		return "", err
	}
	return databasePath, nil
}

func verifySQLiteHeader(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open extracted SQLite file: %w", err)
	}
	defer file.Close()
	header := make([]byte, 16)
	if _, err := io.ReadFull(file, header); err != nil {
		return fmt.Errorf("read SQLite header: %w", err)
	}
	if string(header) != "SQLite format 3\x00" {
		return fmt.Errorf("extracted file is not a SQLite database")
	}
	return nil
}
