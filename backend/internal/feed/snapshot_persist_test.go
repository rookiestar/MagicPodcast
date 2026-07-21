package feed

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// openSnapshotTestDB opens a SQLite database (in a temp dir unless path is
// given) and creates the feed_snapshots table so the durable store can be
// exercised without the full migration suite. The returned *sql.DB is the same
// handle the store uses, so tests can also run raw statements; the caller owns
// closing it.
func openSnapshotTestDB(t *testing.T, path string) (*SQLiteSnapshotStore, *sql.DB) {
	t.Helper()
	if path == "" {
		path = filepath.Join(t.TempDir(), "snapshots.db")
	}
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, db.Exec(FeedSnapshotsCreateTableSQL).Error)
	require.NoError(t, db.Exec(FeedSnapshotsCreateIndexSQL).Error)
	store, err := NewSQLiteSnapshotStore(sqlDB, LastGoodStoreConfig{})
	require.NoError(t, err)
	return store, sqlDB
}

func TestSQLiteSnapshotStoreRoundtripMissAndStats(t *testing.T) {
	store, sqlDB := openSnapshotTestDB(t, "")
	defer sqlDB.Close()

	retrievedAt := time.Date(2026, 7, 21, 9, 0, 0, 0, time.UTC)
	snapshot := FeedSnapshot{
		FeedURL:      "https://example.test/feed.xml",
		RetrievedAt:  retrievedAt,
		RawContent:   []byte(testSnapshotFeed("Persisted")),
		ETag:         `"abc123"`,
		LastModified: "Mon, 21 Jul 2026 09:00:00 GMT",
	}
	require.NoError(t, store.Save(snapshot))

	loaded, ok, err := store.Load(snapshot.FeedURL)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, snapshot.FeedURL, loaded.FeedURL)
	require.Equal(t, fingerprint(snapshot.RawContent), loaded.Fingerprint)
	require.Equal(t, snapshot.RawContent, loaded.RawContent)
	require.Equal(t, snapshot.ETag, loaded.ETag)
	require.Equal(t, snapshot.LastModified, loaded.LastModified)
	require.Equal(t, retrievedAt.UTC(), loaded.RetrievedAt)

	// A clean miss returns ok=false with a nil error, never a fake hit.
	missing, ok, err := store.Load("https://example.test/missing.xml")
	require.NoError(t, err)
	require.False(t, ok)
	require.Nil(t, missing)

	stats, err := store.Stats()
	require.NoError(t, err)
	require.Equal(t, 1, stats.Entries)
	require.Equal(t, int64(len(snapshot.RawContent)), stats.TotalBytes)
	require.Zero(t, stats.WriteFailures)
}

// TestSQLiteSnapshotStoreDistinguishesMissFromCorruption is the #26 acceptance
// guarantee: a real miss (ok=false, err=nil) must be distinguishable from a row
// whose content no longer matches its stored fingerprint (err wraps
// ErrSnapshotCorrupted), so corruption is never silently disguised as a miss.
func TestSQLiteSnapshotStoreDistinguishesMissFromCorruption(t *testing.T) {
	store, sqlDB := openSnapshotTestDB(t, "")
	defer sqlDB.Close()

	feedURL := CanonicalizeURL("https://example.test/corrupt.xml")
	_, err := sqlDB.Exec(
		`INSERT INTO feed_snapshots (feed_url, retrieved_at, fingerprint, raw_content, content_length, etag, last_modified, validated_at, source_at_capture)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		feedURL,
		time.Now().UnixMilli(),
		"deadbeef", // deliberately wrong fingerprint
		[]byte("real content that should hash differently"),
		42,
		"", "", time.Now().UnixMilli(), "",
	)
	require.NoError(t, err)

	_, corrupted, corruptErr := store.Load(feedURL)
	require.False(t, corrupted)
	require.ErrorIs(t, corruptErr, ErrSnapshotCorrupted)

	_, missing, missErr := store.Load("https://example.test/truly-missing.xml")
	require.False(t, missing)
	require.NoError(t, missErr)
}

// TestSQLiteSnapshotStoreSurfacesDatabaseErrorsDistinctFromMiss closes the DB
// handle and confirms a real persistence error (lock contention, closed
// connection, etc.) is surfaced as err != nil rather than silently degraded to
// a miss, and is not mislabeled as corruption. Combined with the miss and
// corruption cases this proves all three outcomes are independently observable.
func TestSQLiteSnapshotStoreSurfacesDatabaseErrorsDistinctFromMiss(t *testing.T) {
	store, sqlDB := openSnapshotTestDB(t, "")
	require.NoError(t, sqlDB.Close())

	_, ok, err := store.Load("https://example.test/feed.xml")
	require.False(t, ok)
	require.Error(t, err)
	require.False(t, errors.Is(err, ErrSnapshotCorrupted), "a generic DB error must not be mislabeled as corruption")

	_, statsErr := store.Stats()
	require.Error(t, statsErr)
}

// TestSQLiteSnapshotStoreSurvivesRestart proves a snapshot persisted by one
// store is loadable by a fresh store opened on the same database file.
func TestSQLiteSnapshotStoreSurvivesRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "restart.db")

	first, sqlDBFirst := openSnapshotTestDB(t, path)
	snapshot := FeedSnapshot{
		FeedURL:     "https://example.test/feed.xml",
		RetrievedAt: time.Date(2026, 7, 21, 9, 0, 0, 0, time.UTC),
		RawContent:  []byte(testSnapshotFeed("Restart survivor")),
	}
	require.NoError(t, first.Save(snapshot))
	require.NoError(t, sqlDBFirst.Close())

	second, sqlDBSecond := openSnapshotTestDB(t, path)
	defer sqlDBSecond.Close()
	loaded, ok, err := second.Load(snapshot.FeedURL)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, snapshot.RawContent, loaded.RawContent)
	require.Equal(t, fingerprint(snapshot.RawContent), loaded.Fingerprint)
}

func TestSQLiteSnapshotStoreEvictsToEntryCap(t *testing.T) {
	store, sqlDB := openSnapshotTestDB(t, "")
	defer sqlDB.Close()
	// Rebuild the store with a tiny entry cap so eviction is observable.
	small, err := NewSQLiteSnapshotStore(store.db, LastGoodStoreConfig{MaxEntries: 2, MaxResponseBytes: 1024, MaxTotalBytes: 4096})
	require.NoError(t, err)

	base := time.Date(2026, 7, 21, 9, 0, 0, 0, time.UTC)
	first := FeedSnapshot{FeedURL: "https://example.test/1.xml", RetrievedAt: base, ValidatedAt: base, RawContent: []byte("one")}
	second := FeedSnapshot{FeedURL: "https://example.test/2.xml", RetrievedAt: base.Add(time.Second), ValidatedAt: base.Add(time.Second), RawContent: []byte("two")}
	third := FeedSnapshot{FeedURL: "https://example.test/3.xml", RetrievedAt: base.Add(2 * time.Second), ValidatedAt: base.Add(2 * time.Second), RawContent: []byte("three")}

	require.NoError(t, small.Save(first))
	require.NoError(t, small.Save(second))
	require.NoError(t, small.Save(third)) // evicts the oldest (first)

	_, firstOk, err := small.Load(first.FeedURL)
	require.NoError(t, err)
	require.False(t, firstOk)
	_, thirdOk, err := small.Load(third.FeedURL)
	require.NoError(t, err)
	require.True(t, thirdOk)

	stats, err := small.Stats()
	require.NoError(t, err)
	require.Equal(t, 2, stats.Entries)
	require.Equal(t, int64(1), stats.EvictedCount)
}

func TestSQLiteSnapshotStoreUpsertReplacesWithoutDoubleCount(t *testing.T) {
	store, sqlDB := openSnapshotTestDB(t, "")
	defer sqlDB.Close()
	feedURL := "https://example.test/feed.xml"
	require.NoError(t, store.Save(FeedSnapshot{FeedURL: feedURL, RetrievedAt: time.Now(), RawContent: []byte(testSnapshotFeed("V1"))}))
	require.NoError(t, store.Save(FeedSnapshot{FeedURL: feedURL, RetrievedAt: time.Now(), RawContent: []byte(testSnapshotFeed("V2"))}))

	stats, err := store.Stats()
	require.NoError(t, err)
	require.Equal(t, 1, stats.Entries)

	loaded, ok, err := store.Load(feedURL)
	require.NoError(t, err)
	require.True(t, ok)
	parsed, err := parseSnapshot(loaded)
	require.NoError(t, err)
	require.Equal(t, "V2", parsed.Title)
}

func TestSQLiteSnapshotStoreDeleteRemovesEntry(t *testing.T) {
	store, sqlDB := openSnapshotTestDB(t, "")
	defer sqlDB.Close()
	feedURL := "https://example.test/feed.xml"
	require.NoError(t, store.Save(FeedSnapshot{FeedURL: feedURL, RetrievedAt: time.Now(), RawContent: []byte(testSnapshotFeed("Gone"))}))
	require.NoError(t, store.Delete(feedURL))
	_, ok, err := store.Load(feedURL)
	require.NoError(t, err)
	require.False(t, ok)
}

// TestSQLiteSnapshotStoreTouchValidatedAtAdvancesOnlyValidatedAt is the #27
// durability guarantee: a 304 bumps validated_at (driving oldest-first
// eviction) without rewriting the body, fingerprint, or retrieved_at. A touch
// on a missing row is a no-op rather than an error.
func TestSQLiteSnapshotStoreTouchValidatedAtAdvancesOnlyValidatedAt(t *testing.T) {
	store, sqlDB := openSnapshotTestDB(t, "")
	defer sqlDB.Close()
	feedURL := "https://example.test/feed.xml"
	// A clearly-past base so a real wall-clock validated_at always advances it.
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, store.Save(FeedSnapshot{
		FeedURL:     feedURL,
		RetrievedAt: base,
		RawContent:  []byte(testSnapshotFeed("Touch")),
		ETag:        `"t1"`,
	}))

	before, ok, err := store.Load(feedURL)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, base, before.ValidatedAt)
	require.Equal(t, base, before.RetrievedAt)

	// A missing row is a no-op.
	require.NoError(t, store.TouchValidatedAt("https://example.test/absent.xml"))

	require.NoError(t, store.TouchValidatedAt(feedURL))

	after, ok, err := store.Load(feedURL)
	require.NoError(t, err)
	require.True(t, ok)
	require.True(t, after.ValidatedAt.After(base), "validated_at must advance")
	require.Equal(t, base, after.RetrievedAt, "retrieved_at must be untouched")
	require.Equal(t, before.Fingerprint, after.Fingerprint, "fingerprint must be untouched")
	require.Equal(t, before.RawContent, after.RawContent, "body must be untouched")
	require.Equal(t, `"t1"`, after.ETag, "etag must be untouched")
}

// TestTieredSnapshotStoreReadsL2OnMissAndBackfillsL1 verifies the L1+L2
// layering: a snapshot saved through one tiered store is visible to a second
// tiered store sharing the same durable L2, and a read warms the L1.
func TestTieredSnapshotStoreReadsL2OnMissAndBackfillsL1(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tiered.db")

	l2a, sqlDBFirst := openSnapshotTestDB(t, path)
	tieredA := NewTieredSnapshotStore(l2a, LastGoodStoreConfig{})

	feedURL := "https://example.test/feed.xml"
	require.NoError(t, tieredA.Save(FeedSnapshot{FeedURL: feedURL, RetrievedAt: time.Now(), RawContent: []byte(testSnapshotFeed("Tiered"))}))
	require.NoError(t, sqlDBFirst.Close())

	l2b, sqlDBSecond := openSnapshotTestDB(t, path)
	defer sqlDBSecond.Close()
	tieredB := NewTieredSnapshotStore(l2b, LastGoodStoreConfig{})

	// L1 of tieredB is empty; the read must be served from L2 and backfill L1.
	require.NoError(t, tieredB.l1.Delete(feedURL))
	loaded, ok, err := tieredB.Load(feedURL)
	require.NoError(t, err)
	require.True(t, ok)
	parsed, err := parseSnapshot(loaded)
	require.NoError(t, err)
	require.Equal(t, "Tiered", parsed.Title)

	cached, cachedOk, err := tieredB.l1.Load(feedURL)
	require.NoError(t, err)
	require.True(t, cachedOk)
	require.Equal(t, loaded.Fingerprint, cached.Fingerprint)
}

// TestTieredSnapshotStoreSurfacesDurabilityFailure keeps the #26 contract that
// an L2 failure is reported (ErrSnapshotNotPersisted) while the in-process L1
// copy still serves the snapshot — never silently reporting durable success.
func TestTieredSnapshotStoreSurfacesDurabilityFailure(t *testing.T) {
	tiered := NewTieredSnapshotStore(failingSnapshotStore{}, LastGoodStoreConfig{})
	feedURL := "https://example.test/feed.xml"
	err := tiered.Save(FeedSnapshot{FeedURL: feedURL, RetrievedAt: time.Now(), RawContent: []byte(testSnapshotFeed("Best effort"))})
	require.ErrorIs(t, err, ErrSnapshotNotPersisted)

	// L1 retained the snapshot despite the L2 failure.
	loaded, ok, loadErr := tiered.Load(feedURL)
	require.NoError(t, loadErr)
	require.True(t, ok)
	parsed, err := parseSnapshot(loaded)
	require.NoError(t, err)
	require.Equal(t, "Best effort", parsed.Title)
}

type failingSnapshotStore struct{}

func (failingSnapshotStore) Save(FeedSnapshot) error                        { return errors.New("disk full") }
func (failingSnapshotStore) Load(string) (*FeedSnapshot, bool, error)       { return nil, false, nil }
func (failingSnapshotStore) Delete(string) error                            { return nil }
func (failingSnapshotStore) TouchValidatedAt(string) error                  { return nil }
func (failingSnapshotStore) Stats() (SnapshotStoreStats, error)             { return SnapshotStoreStats{}, nil }
