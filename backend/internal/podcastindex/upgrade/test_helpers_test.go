package upgrade

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	_ "modernc.org/sqlite"
)

func fixtureSchema(itunesType, deadType string, includePodcastGUID bool) string {
	podcastGUID := "podcastGuid TEXT,"
	if !includePodcastGUID {
		podcastGUID = ""
	}
	return fmt.Sprintf(`
CREATE TABLE podcasts (
  id INTEGER PRIMARY KEY,
  url TEXT NOT NULL,
  title TEXT NOT NULL,
  lastUpdate INTEGER,
  link TEXT,
  lastHttpStatus INTEGER,
  dead %s,
  contentType TEXT,
  itunesId %s,
  originalUrl TEXT,
  itunesAuthor TEXT,
  itunesOwnerName TEXT,
  explicit INTEGER,
  imageUrl TEXT,
  itunesType TEXT,
  generator TEXT,
  newestItemPubdate INTEGER,
  language TEXT,
  oldestItemPubdate INTEGER,
  episodeCount INTEGER,
  popularityScore INTEGER,
  priority INTEGER,
  createdOn INTEGER,
  updateFrequency INTEGER,
  chash TEXT,
  host TEXT,
  newestEnclosureUrl TEXT,
  %s
  description TEXT,
  category1 TEXT,
  category2 TEXT,
  category3 TEXT,
  category4 TEXT,
  category5 TEXT,
  category6 TEXT,
  category7 TEXT,
  category8 TEXT,
  category9 TEXT,
  category10 TEXT,
  newestEnclosureDuration INTEGER
);`, deadType, itunesType, podcastGUID)
}

func createCandidateFixture(t *testing.T, rows bool) string {
	t.Helper()
	return createFixtureWithSchema(t, fixtureSchema("INTEGER", "INTEGER", true), rows)
}

func createFixtureWithSchema(t *testing.T, schema string, rows bool) string {
	t.Helper()
	databasePath := filepath.Join(t.TempDir(), "podcastindex.db")
	db, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		t.Fatalf("create fixture schema: %v", err)
	}
	if rows {
		insertFixtureRows(t, db)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close fixture: %v", err)
	}
	return databasePath
}

func insertFixtureRows(t *testing.T, db *sql.DB) {
	t.Helper()
	rows := []struct {
		id                                                             int
		title, url, author, itunesID, guid                             string
		dead, status, explicit, newest, episodes, popularity, priority int
	}{
		{1, "测试节目", "https://example.com/test.xml", "作者", "123", "guid-1", 0, 200, 1, 200, 8, 5, 0},
		{2, "测试节目", "https://example.com/dead.xml", "作者", "456", "guid-2", 1, 500, 1, 300, 20, 9, 0},
		{3, "空 ID", "https://example.com/empty.xml", "另一作者", "", "guid-3", 0, 200, 0, 100, 2, 1, 1},
		{4, "文本 ID", "https://example.com/text.xml", "文本作者", "not-a-number", "guid-4", 0, 200, 0, 90, 1, 1, 1},
		{5, "仅标题节目", "https://example.com/title-only.xml", "其他作者", "789", "guid-5", 0, 200, 1, 80, 3, 1, 1},
	}
	const insertSQL = `INSERT INTO podcasts
  (id,url,title,lastUpdate,link,lastHttpStatus,dead,contentType,itunesId,originalUrl,itunesAuthor,itunesOwnerName,explicit,imageUrl,itunesType,generator,newestItemPubdate,language,oldestItemPubdate,episodeCount,popularityScore,priority,createdOn,updateFrequency,chash,host,newestEnclosureUrl,podcastGuid,description,newestEnclosureDuration)
  VALUES (
    ?, ?, ?, ?, ?, ?, ?, ?, ?, ?,
    ?, ?, ?, ?, ?, ?, ?, ?, ?, ?,
    ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
  )`
	for _, row := range rows {
		if _, err := db.Exec(insertSQL,
			row.id, row.url, row.title, row.newest, "https://example.com", row.status, row.dead, "application/rss+xml", row.itunesID, row.url,
			row.author, row.author, row.explicit, "https://example.com/image.jpg", "episodic", "fixture", row.newest, "en", 1, row.episodes,
			row.popularity, row.priority, 1, 1, "hash", "example.com", "https://example.com/episode.mp3", row.guid, "fixture description", 60,
		); err != nil {
			t.Fatalf("insert fixture row %d: %v", row.id, err)
		}
	}
}

func projectViewSQL(t *testing.T) string {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test file path")
	}
	root := filepath.Join(filepath.Dir(currentFile), "../../../../")
	contents, err := os.ReadFile(filepath.Join(root, "scripts/create_unique_podcasts_view.sql"))
	if err != nil {
		t.Fatalf("read project view SQL: %v", err)
	}
	return string(contents)
}

func makeArchive(t *testing.T, entries []archiveTestEntry) string {
	t.Helper()
	archivePath := filepath.Join(t.TempDir(), "dataset.tgz")
	file, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("create archive: %v", err)
	}
	gzipWriter := gzip.NewWriter(file)
	tarWriter := tar.NewWriter(gzipWriter)
	for _, entry := range entries {
		header := &tar.Header{
			Name:     entry.name,
			Mode:     entry.mode,
			Size:     int64(len(entry.contents)),
			Typeflag: entry.typeflag,
			Linkname: entry.linkname,
		}
		if entry.typeflag == tar.TypeSymlink {
			header.Size = 0
		}
		if err := tarWriter.WriteHeader(header); err != nil {
			file.Close()
			t.Fatalf("write archive header: %v", err)
		}
		if entry.typeflag == tar.TypeReg || entry.typeflag == tar.TypeRegA {
			if _, err := tarWriter.Write(entry.contents); err != nil {
				file.Close()
				t.Fatalf("write archive contents: %v", err)
			}
		}
	}
	if err := tarWriter.Close(); err != nil {
		file.Close()
		t.Fatalf("close tar: %v", err)
	}
	if err := gzipWriter.Close(); err != nil {
		file.Close()
		t.Fatalf("close gzip: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close archive: %v", err)
	}
	return archivePath
}

type archiveTestEntry struct {
	name     string
	contents []byte
	mode     int64
	typeflag byte
	linkname string
}

func validArchiveEntry(contents []byte) archiveTestEntry {
	return archiveTestEntry{name: "podcastindex_feeds.db", contents: contents, mode: 0o600, typeflag: tar.TypeReg}
}

func sqliteHeaderBytes() []byte {
	return append([]byte("SQLite format 3\x00"), bytes.Repeat([]byte{0}, 32)...)
}
