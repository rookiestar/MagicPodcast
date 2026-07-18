package upgrade

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestExportFailedSamplesUsesDistinctFailuresSinceIssueBaseline(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "primary.db")
	db, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
		CREATE TABLE podcasts (
		  id INTEGER PRIMARY KEY, title TEXT, author TEXT, feed_url TEXT,
		  i_tunes_id TEXT, podcast_guid TEXT, deleted_at TEXT
		);
		CREATE TABLE job_executions (
		  id INTEGER PRIMARY KEY, created_at TEXT, deleted_at TEXT,
		  podcast_id INTEGER, podcast_title TEXT, podcast_feed_url TEXT, status TEXT
		);
		INSERT INTO podcasts VALUES (1, '节目一', '作者一', 'https://feed.xyzfm.space/old', '', '', NULL);
		INSERT INTO podcasts VALUES (2, '节目二', '作者二', 'https://feed.xyzfm.space/two', '123', 'guid-two', NULL);
		INSERT INTO job_executions VALUES (1, '2026-06-01T00:00:00Z', NULL, 1, '节目一', 'https://feed.xyzfm.space/old', 'failed');
		INSERT INTO job_executions VALUES (2, '2026-06-02T00:00:00Z', NULL, 1, '节目一', 'https://feed.xyzfm.space/new', 'failed');
		INSERT INTO job_executions VALUES (3, '2026-06-03T00:00:00Z', NULL, 1, '节目一', 'https://feed.xyzfm.space/new', 'success');
		INSERT INTO job_executions VALUES (4, '2026-06-03T00:00:00Z', NULL, 2, '节目二', 'https://feed.xyzfm.space/two', 'failed');
		INSERT INTO job_executions VALUES (5, '2026-06-03T00:00:00Z', NULL, 3, '节目三', 'https://example.com/other', 'failed');
	`)
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	samples, err := ExportFailedSamplesSince(databasePath, "https://feed.xyzfm.space/", "2026-06-02")
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != 2 {
		t.Fatalf("samples = %+v, want two distinct failed podcasts", samples)
	}
	if samples[0].FeedURL != "https://feed.xyzfm.space/new" || samples[1].ITunesID != "123" {
		t.Fatalf("samples = %+v", samples)
	}
}

func TestCompareFailedSamplesPrioritizesStableIdentityAndChecksAccessibility(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("<rss></rss>"))
	}))
	defer server.Close()
	databasePath := createCandidateFixture(t, true)
	db, err := OpenSQLite(databasePath, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []int{1, 3, 5} {
		if _, err := db.Exec("UPDATE podcasts SET url = ? WHERE id = ?", server.URL+"/"+fmt.Sprint(id), id); err != nil {
			db.Close()
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	samples := []FailedSample{
		{ID: 1, Title: "不同标题", FeedURL: "https://feed.xyzfm.space/1", ITunesID: "123"},
		{ID: 2, Title: "不同标题", FeedURL: "https://feed.xyzfm.space/2", PodcastGUID: "guid-3"},
		{ID: 3, Title: "仅标题节目", FeedURL: "https://feed.xyzfm.space/3"},
		{ID: 4, Title: "不存在", FeedURL: "https://feed.xyzfm.space/4"},
	}
	comparison, err := CompareFailedSamples(context.Background(), databasePath, samples, CompareOptions{
		CheckAccessibility:   true,
		AccessibilityClient:  NewDirectHTTPClient(5 * time.Second),
		AccessibilityTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("CompareFailedSamples() error = %v", err)
	}
	if comparison.Matched != 3 || comparison.NoMatch != 1 || comparison.IdentityConfirmed != 2 || comparison.TitleOnly != 1 {
		t.Fatalf("comparison = %+v", comparison)
	}
	if comparison.AccessibleIdentityConfirmed != 2 || comparison.AccessibleAny != 3 {
		t.Fatalf("accessibility counts = %+v", comparison)
	}
	if comparison.Matches[0].IdentityMethod != "itunes_id" || comparison.Matches[0].Confidence != "high" {
		t.Fatalf("iTunes identity match = %+v", comparison.Matches[0])
	}
	if !comparison.Matches[2].TitleOnly || comparison.Matches[2].IdentityConfirmed {
		t.Fatalf("title-only match = %+v", comparison.Matches[2])
	}
}

func TestCompareFailedSamplesDoesNotTreatSameTitleAsConfirmedIdentity(t *testing.T) {
	databasePath := createCandidateFixture(t, true)
	comparison, err := CompareFailedSamples(context.Background(), databasePath, []FailedSample{
		{ID: 10, Title: "测试节目", FeedURL: "https://feed.xyzfm.space/10"},
	}, CompareOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if comparison.Matched != 1 || comparison.TitleOnly != 1 || comparison.IdentityConfirmed != 0 {
		t.Fatalf("comparison = %+v", comparison)
	}
}
