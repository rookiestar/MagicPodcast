package upgrade

import (
	"archive/tar"
	"os"
	"path/filepath"
	"testing"
)

func TestValidateArchiveAcceptsOneOrdinarySQLiteFile(t *testing.T) {
	archivePath := makeArchive(t, []archiveTestEntry{validArchiveEntry(sqliteHeaderBytes())})
	inspection, err := ValidateArchive(archivePath)
	if err != nil {
		t.Fatalf("ValidateArchive() error = %v", err)
	}
	if !inspection.GzipValid || !inspection.TarValid {
		t.Fatalf("inspection = %+v, expected valid gzip/tar", inspection)
	}
	if inspection.DatabaseEntry.Name != "podcastindex_feeds.db" {
		t.Fatalf("database entry = %+v", inspection.DatabaseEntry)
	}

	destination := filepath.Join(t.TempDir(), "candidate")
	databasePath, err := ExtractArchive(archivePath, destination)
	if err != nil {
		t.Fatalf("ExtractArchive() error = %v", err)
	}
	if _, err := os.Stat(databasePath); err != nil {
		t.Fatalf("extracted database missing: %v", err)
	}
	if err := verifySQLiteHeader(databasePath); err != nil {
		t.Fatalf("extracted header validation error = %v", err)
	}
}

func TestValidateArchiveAcceptsSingleDotPrefix(t *testing.T) {
	archivePath := makeArchive(t, []archiveTestEntry{{
		name:     "./podcastindex_feeds.db",
		contents: sqliteHeaderBytes(),
		mode:     0o600,
		typeflag: tar.TypeReg,
	}})
	if _, err := ValidateArchive(archivePath); err != nil {
		t.Fatalf("ValidateArchive() rejected a safe ./ prefix: %v", err)
	}
}

func TestValidateArchiveRejectsCorruptGzip(t *testing.T) {
	archivePath := makeArchive(t, []archiveTestEntry{validArchiveEntry(sqliteHeaderBytes())})
	contents, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(archivePath, contents[:len(contents)/2], 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateArchive(archivePath); err == nil {
		t.Fatal("ValidateArchive() succeeded for truncated gzip")
	}
}

func TestValidateArchiveRejectsUnsafeEntries(t *testing.T) {
	tests := []struct {
		name  string
		entry archiveTestEntry
	}{
		{
			name:  "path traversal",
			entry: archiveTestEntry{name: "../outside.db", contents: sqliteHeaderBytes(), mode: 0o600, typeflag: tar.TypeReg},
		},
		{
			name:  "absolute path",
			entry: archiveTestEntry{name: "/tmp/outside.db", contents: sqliteHeaderBytes(), mode: 0o600, typeflag: tar.TypeReg},
		},
		{
			name:  "symlink",
			entry: archiveTestEntry{name: "podcastindex_feeds.db", mode: 0o600, typeflag: tar.TypeSymlink, linkname: "/tmp/outside.db"},
		},
		{
			name:  "executable",
			entry: archiveTestEntry{name: "podcastindex_feeds.db", contents: sqliteHeaderBytes(), mode: 0o755, typeflag: tar.TypeReg},
		},
		{
			name:  "unexpected extension",
			entry: archiveTestEntry{name: "payload.bin", contents: sqliteHeaderBytes(), mode: 0o600, typeflag: tar.TypeReg},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			archivePath := makeArchive(t, []archiveTestEntry{test.entry})
			if _, err := ValidateArchive(archivePath); err == nil {
				t.Fatalf("ValidateArchive() accepted %s", test.name)
			}
		})
	}
}

func TestValidateArchiveRejectsUnexpectedExtraFile(t *testing.T) {
	archivePath := makeArchive(t, []archiveTestEntry{
		validArchiveEntry(sqliteHeaderBytes()),
		{name: "README.txt", contents: []byte("unexpected"), mode: 0o600, typeflag: tar.TypeReg},
	})
	if _, err := ValidateArchive(archivePath); err == nil {
		t.Fatal("ValidateArchive() accepted archive with an extra file")
	}
}

func TestExtractArchiveRejectsNonSQLitePayload(t *testing.T) {
	archivePath := makeArchive(t, []archiveTestEntry{validArchiveEntry([]byte("not sqlite"))})
	if _, err := ExtractArchive(archivePath, filepath.Join(t.TempDir(), "candidate")); err == nil {
		t.Fatal("ExtractArchive() accepted non-SQLite payload")
	}
}
