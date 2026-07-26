package upgrade

import (
	"database/sql"
	"strings"
	"testing"

	"magicpodcast/internal/podcastindex"
)

func TestValidateCandidateCreatesViewAndChecksQueryContract(t *testing.T) {
	databasePath := createCandidateFixture(t, true)
	result, err := ValidateCandidate(databasePath, projectViewSQL(t), false)
	if err != nil {
		t.Fatalf("ValidateCandidate() error = %v; result=%+v", err, result)
	}
	if !result.Passed || !result.ViewCreated || !result.Query.Passed {
		t.Fatalf("result = %+v", result)
	}
	if result.Metrics.TotalRows != 5 || result.Query.ViewCount != 4 {
		t.Fatalf("metrics/query = %+v / %+v", result.Metrics, result.Query)
	}
	finalSHA256, finalSizeBytes, err := SHA256File(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	if result.SHA256 != finalSHA256 || result.SizeBytes != finalSizeBytes {
		t.Fatalf("final candidate identity = %s/%d, actual = %s/%d", result.SHA256, result.SizeBytes, finalSHA256, finalSizeBytes)
	}

	query, err := podcastindex.NewQuery(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer query.Close()
	info, err := query.FindByFeedURL("https://example.com/empty.xml")
	if err != nil {
		t.Fatalf("empty-string iTunes ID URL query error = %v", err)
	}
	if info == nil || info.ITunesID != 0 {
		t.Fatalf("empty-string iTunes ID result = %+v", info)
	}
	if _, err := query.FindByFeedURL("https://example.com/text.xml"); err != nil {
		t.Fatalf("text iTunes ID URL query error = %v", err)
	}
	info, err = query.FindByITunesID(123)
	if err != nil || info == nil || info.ITunesID != 123 {
		t.Fatalf("numeric iTunes ID query result=%+v err=%v", info, err)
	}
}

func TestValidateCandidateCreatesStableIdentityIndexes(t *testing.T) {
	databasePath := createCandidateFixture(t, true)
	result, err := ValidateCandidate(databasePath, projectViewSQL(t), false)
	if err != nil || !result.Passed {
		t.Fatalf("ValidateCandidate() result=%+v err=%v", result, err)
	}

	db, err := OpenSQLite(databasePath, true)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	rows, err := db.Query(`EXPLAIN QUERY PLAN SELECT p.id FROM podcasts AS p
		WHERE (p.itunesId = ? OR p.podcastGuid = ? COLLATE NOCASE)
		  AND p.url IS NOT NULL AND p.url <> ''
		ORDER BY p.dead ASC, p.lastHttpStatus DESC, p.newestItemPubdate DESC, p.episodeCount DESC, p.id ASC`, 123, "guid-1")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	var details []string
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		details = append(details, detail)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	plan := strings.Join(details, " | ")
	if !strings.Contains(plan, "idx_podcasts_itunes_id") || !strings.Contains(plan, "idx_podcasts_podcast_guid_nocase") {
		t.Fatalf("stable identity lookup is not index-backed: %s", plan)
	}
}

func TestValidateCandidateRejectsMissingRequiredColumn(t *testing.T) {
	schema := strings.Replace(fixtureSchema("INTEGER", "INTEGER", true), "  podcastGuid TEXT,\n", "", 1)
	databasePath := createFixtureWithSchema(t, schema, false)
	result, err := ValidateCandidate(databasePath, projectViewSQL(t), false)
	if err == nil || result.Passed {
		t.Fatalf("result=%+v err=%v; expected missing podcastGuid rejection", result, err)
	}
	if !strings.Contains(err.Error(), "podcastGuid") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateCandidateRejectsWrongTypeForRankingOrITunesID(t *testing.T) {
	tests := []struct {
		name       string
		itunesType string
		deadType   string
		want       string
	}{
		{name: "itunes id text", itunesType: "TEXT", deadType: "INTEGER", want: "itunesId"},
		{name: "dead text", itunesType: "INTEGER", deadType: "TEXT", want: "dead"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			databasePath := createFixtureWithSchema(t, fixtureSchema(test.itunesType, test.deadType, true), true)
			result, err := ValidateCandidate(databasePath, projectViewSQL(t), false)
			if err == nil || result.Passed {
				t.Fatalf("result=%+v err=%v; expected type rejection", result, err)
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want mention %q", err, test.want)
			}
		})
	}
}

func TestValidateCandidateRejectsEmptyTable(t *testing.T) {
	databasePath := createCandidateFixture(t, false)
	result, err := ValidateCandidate(databasePath, projectViewSQL(t), false)
	if err == nil || result.Passed {
		t.Fatalf("result=%+v err=%v; expected empty table rejection", result, err)
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Fatalf("error = %v", err)
	}
}

func TestReadDatabaseMetricsIsReadOnly(t *testing.T) {
	databasePath := createCandidateFixture(t, true)
	metrics, err := ReadDatabaseMetrics(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	if metrics.TotalRows != 5 || metrics.LiveRows != 4 || metrics.HTTP200Rows != 4 {
		t.Fatalf("metrics = %+v", metrics)
	}
	db, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var viewCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='view'").Scan(&viewCount); err != nil {
		t.Fatal(err)
	}
	if viewCount != 0 {
		t.Fatalf("ReadDatabaseMetrics created views: %d", viewCount)
	}
}
