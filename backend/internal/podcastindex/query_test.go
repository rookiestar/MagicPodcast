package podcastindex

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

type queryFixtureRow struct {
	id       int
	title    string
	author   string
	feedURL  string
	itunesID any
	guid     string
	dead     int
	status   int
}

func createQueryFixture(t *testing.T, rows []queryFixtureRow) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "podcastindex.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(`
CREATE TABLE podcasts (
  id INTEGER PRIMARY KEY,
  url TEXT NOT NULL,
  title TEXT NOT NULL,
  lastUpdate INTEGER,
  link TEXT,
  lastHttpStatus INTEGER,
  dead INTEGER,
  itunesAuthor TEXT,
  itunesId INTEGER,
  imageUrl TEXT,
  newestItemPubdate INTEGER,
  language TEXT,
  oldestItemPubdate INTEGER,
  episodeCount INTEGER,
  popularityScore INTEGER,
  priority INTEGER,
  updateFrequency INTEGER,
  newestEnclosureUrl TEXT,
  podcastGuid TEXT,
  description TEXT,
  newestEnclosureDuration INTEGER
)`)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		_, err = db.Exec(`INSERT INTO podcasts
 (id, url, title, lastUpdate, link, lastHttpStatus, dead, itunesAuthor, itunesId,
  imageUrl, newestItemPubdate, language, oldestItemPubdate, episodeCount,
  popularityScore, priority, updateFrequency, newestEnclosureUrl, podcastGuid,
  description, newestEnclosureDuration)
 VALUES (?, ?, ?, 1, 'https://example.com', ?, ?, ?, ?,
         'https://example.com/image.jpg', 1, 'en', 1, 1, 1, 1, 1,
         'https://example.com/episode.mp3', ?, 'description', 60)`,
			row.id, row.feedURL, row.title, row.status, row.dead, row.author, row.itunesID, row.guid)
		if err != nil {
			t.Fatal(err)
		}
	}
	return path
}

func TestQueryUsesRawTableWhenUniqueViewIsAbsent(t *testing.T) {
	path := createQueryFixture(t, []queryFixtureRow{{
		id: 1, title: "稳定节目", author: "作者", feedURL: "https://primary.example/feed.xml", itunesID: 123, guid: "guid-123", status: 200,
	}})
	query, err := NewQuery(path)
	if err != nil {
		t.Fatal(err)
	}
	defer query.Close()

	info, err := query.FindByFeedURLContext(context.Background(), "https://primary.example/feed.xml")
	if err != nil || info == nil {
		t.Fatalf("FindByFeedURL() info=%+v err=%v", info, err)
	}
	if info.ITunesID != 123 || info.PodcastGUID != "guid-123" {
		t.Fatalf("stable identity = %+v", info)
	}
}

func TestFindByFeedURLUsesRawURLLookupEvenWhenUniqueViewExists(t *testing.T) {
	path := createQueryFixture(t, []queryFixtureRow{{
		id: 1, title: "稳定节目", author: "作者", feedURL: "https://primary.example/feed.xml", itunesID: 123, guid: "guid-123", status: 200,
	}})
	indexDB, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = indexDB.Exec(`CREATE VIEW v_unique_podcasts AS SELECT * FROM podcasts WHERE 0`)
	if err != nil {
		indexDB.Close()
		t.Fatal(err)
	}
	indexDB.Close()

	query, err := NewQuery(path)
	if err != nil {
		t.Fatal(err)
	}
	defer query.Close()

	info, err := query.FindByFeedURLContext(context.Background(), "https://primary.example/feed.xml")
	if err != nil || info == nil {
		t.Fatalf("FindByFeedURL() info=%+v err=%v", info, err)
	}
	if info.ID != 1 {
		t.Fatalf("raw URL lookup returned %+v", info)
	}
}

func TestPodcastIndexContextQueriesHonorCancellation(t *testing.T) {
	path := createQueryFixture(t, []queryFixtureRow{{
		id: 1, title: "稳定节目", author: "作者", feedURL: "https://primary.example/feed.xml", itunesID: 123, guid: "guid-123", status: 200,
	}})
	query, err := NewQuery(path)
	if err != nil {
		t.Fatal(err)
	}
	defer query.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := query.FindByFeedURLContext(ctx, "https://primary.example/feed.xml"); err == nil {
		t.Fatal("FindByFeedURLContext should reject an already-cancelled context")
	}
	if _, err := query.FindCandidatesByIdentityContext(ctx, 123, "guid-123"); err == nil {
		t.Fatal("FindCandidatesByIdentityContext should reject an already-cancelled context")
	}
}

func TestFindCandidatesByIdentityPreservesConflicts(t *testing.T) {
	path := createQueryFixture(t, []queryFixtureRow{
		{id: 1, title: "主源", author: "作者", feedURL: "https://primary.example/feed.xml", itunesID: 123, guid: "guid-123", status: 403},
		{id: 2, title: "替代一", author: "作者", feedURL: "https://alt-one.example/feed.xml", itunesID: 123, guid: "guid-123", status: 200},
		{id: 3, title: "替代二", author: "作者", feedURL: "https://alt-two.example/feed.xml", itunesID: 123, guid: "guid-123", status: 200},
	})
	query, err := NewQuery(path)
	if err != nil {
		t.Fatal(err)
	}
	defer query.Close()

	candidates, err := query.FindCandidatesByIdentity(123, "guid-123")
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 3 {
		t.Fatalf("candidate count = %d, want 3: %+v", len(candidates), candidates)
	}
}
