package feed

import (
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"
)

// FeedSnapshotsTableName is the durable last-good store table created by
// migration v7.
const FeedSnapshotsTableName = "feed_snapshots"

// FeedSnapshotsCreateTableSQL is the authoritative CREATE TABLE statement for
// the durable last-good store. It is shared by migration v7 and the store so
// the two cannot drift. Times are stored as unix milliseconds; validated_at
// drives LRU eviction.
const FeedSnapshotsCreateTableSQL = `CREATE TABLE IF NOT EXISTS ` + FeedSnapshotsTableName + ` (
    feed_url          TEXT PRIMARY KEY,
    retrieved_at      INTEGER NOT NULL,
    fingerprint       TEXT NOT NULL,
    raw_content       BLOB NOT NULL,
    content_length    INTEGER NOT NULL,
    etag              TEXT NOT NULL DEFAULT '',
    last_modified     TEXT NOT NULL DEFAULT '',
    validated_at      INTEGER NOT NULL,
    source_at_capture TEXT NOT NULL DEFAULT ''
)`

// FeedSnapshotsCreateIndexSQL backs the oldest-first eviction scan.
const FeedSnapshotsCreateIndexSQL = `CREATE INDEX IF NOT EXISTS idx_feed_snapshots_evict ON ` + FeedSnapshotsTableName + ` (validated_at, retrieved_at)`

// SQLiteSnapshotStore is the durable L2 last-good store. It assumes the
// feed_snapshots table exists (created by migration v7); reads and writes are
// bounded by the same caps as the in-memory store, and a Load that finds a row
// whose fingerprint no longer matches reports ErrSnapshotCorrupted instead of a
// silent miss.
type SQLiteSnapshotStore struct {
	db               *sql.DB
	maxEntries       int
	maxResponseBytes int
	maxTotalBytes    int64

	mu            sync.Mutex
	evictedCount  int64
	writeFailures int64
}

// NewSQLiteSnapshotStore wraps an existing database handle. The handle must be
// backed by a database that already has the feed_snapshots table; the store
// never creates or migrates the table at runtime.
func NewSQLiteSnapshotStore(db *sql.DB, config LastGoodStoreConfig) (*SQLiteSnapshotStore, error) {
	if db == nil {
		return nil, errors.New("sqlite snapshot store requires a non-nil database handle")
	}
	if config.MaxEntries <= 0 {
		config.MaxEntries = defaultLastGoodMaxEntries
	}
	if config.MaxResponseBytes <= 0 {
		config.MaxResponseBytes = defaultLastGoodMaxResponseBytes
	}
	if config.MaxTotalBytes <= 0 {
		config.MaxTotalBytes = defaultLastGoodMaxTotalBytes
	}
	return &SQLiteSnapshotStore{
		db:               db,
		maxEntries:       config.MaxEntries,
		maxResponseBytes: config.MaxResponseBytes,
		maxTotalBytes:    config.MaxTotalBytes,
	}, nil
}

func (s *SQLiteSnapshotStore) Save(snapshot FeedSnapshot) error {
	if s == nil || s.db == nil {
		return errors.New("sqlite snapshot store requires a database")
	}
	key := CanonicalizeURL(snapshot.FeedURL)
	if key == "" {
		return errors.New("snapshot feed URL is required")
	}
	if len(snapshot.RawContent) == 0 {
		return errors.New("snapshot content is required")
	}
	if len(snapshot.RawContent) > s.maxResponseBytes {
		return ErrSnapshotResponseTooLarge
	}
	if int64(len(snapshot.RawContent)) > s.maxTotalBytes {
		return ErrSnapshotResponseTooLarge
	}
	if snapshot.RetrievedAt.IsZero() {
		snapshot.RetrievedAt = time.Now()
	}
	if snapshot.ValidatedAt.IsZero() {
		snapshot.ValidatedAt = snapshot.RetrievedAt
	}
	if snapshot.Fingerprint == "" {
		snapshot.Fingerprint = fingerprint(snapshot.RawContent)
	}
	snapshot.FeedURL = key
	contentLength := int64(len(snapshot.RawContent))

	tx, err := s.db.Begin()
	if err != nil {
		s.recordWriteFailure()
		return fmt.Errorf("begin snapshot transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	// If we are overwriting an existing row, its old bytes do not count toward
	// the cap during eviction.
	var existingLength int64
	_ = tx.QueryRow("SELECT content_length FROM "+FeedSnapshotsTableName+" WHERE feed_url = ?", key).Scan(&existingLength)

	evicted, err := s.enforceCaps(tx, key, contentLength, existingLength)
	if err != nil {
		s.recordWriteFailure()
		return err
	}

	_, err = tx.Exec(`INSERT INTO `+FeedSnapshotsTableName+` (
		feed_url, retrieved_at, fingerprint, raw_content, content_length, etag, last_modified, validated_at, source_at_capture
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(feed_url) DO UPDATE SET
		retrieved_at = excluded.retrieved_at,
		fingerprint = excluded.fingerprint,
		raw_content = excluded.raw_content,
		content_length = excluded.content_length,
		etag = excluded.etag,
		last_modified = excluded.last_modified,
		validated_at = excluded.validated_at,
		source_at_capture = excluded.source_at_capture`,
		key,
		snapshot.RetrievedAt.UTC().UnixMilli(),
		snapshot.Fingerprint,
		snapshot.RawContent,
		contentLength,
		snapshot.ETag,
		snapshot.LastModified,
		snapshot.ValidatedAt.UTC().UnixMilli(),
		snapshot.SourceAtCapture,
	)
	if err != nil {
		s.recordWriteFailure()
		return fmt.Errorf("upsert snapshot: %w", err)
	}

	if err := tx.Commit(); err != nil {
		s.recordWriteFailure()
		return fmt.Errorf("commit snapshot transaction: %w", err)
	}
	committed = true
	if evicted > 0 {
		s.recordEvictions(evicted)
	}
	return nil
}

// enforceCaps evicts the oldest snapshots (never the row about to be replaced)
// until the incoming write fits under both the entry count and total byte cap.
// The eviction count is returned and recorded only after the caller commits;
// otherwise a rolled-back write would leave diagnostics claiming that rows were
// evicted when the database still contains them.
func (s *SQLiteSnapshotStore) enforceCaps(tx *sql.Tx, key string, incoming, existingLength int64) (int64, error) {
	var evicted int64
	for {
		var count int
		var total sql.NullInt64
		if err := tx.QueryRow("SELECT COUNT(*), COALESCE(SUM(content_length), 0) FROM "+FeedSnapshotsTableName).Scan(&count, &total); err != nil {
			return evicted, fmt.Errorf("read snapshot stats: %w", err)
		}
		effectiveTotal := total.Int64 - existingLength
		effectiveCount := count
		if existingLength > 0 {
			effectiveCount = count - 1
		}
		if effectiveCount < s.maxEntries && effectiveTotal+incoming <= s.maxTotalBytes {
			return evicted, nil
		}
		var victim string
		err := tx.QueryRow(
			"SELECT feed_url FROM "+FeedSnapshotsTableName+" WHERE feed_url != ? ORDER BY validated_at ASC, retrieved_at ASC, feed_url ASC LIMIT 1",
			key,
		).Scan(&victim)
		if errors.Is(err, sql.ErrNoRows) {
			// Nothing left to evict (the only row is the one being replaced).
			return evicted, nil
		}
		if err != nil {
			return evicted, fmt.Errorf("select eviction victim: %w", err)
		}
		if _, err := tx.Exec("DELETE FROM "+FeedSnapshotsTableName+" WHERE feed_url = ?", victim); err != nil {
			return evicted, fmt.Errorf("evict snapshot: %w", err)
		}
		evicted++
	}
}

func (s *SQLiteSnapshotStore) Load(feedURL string) (*FeedSnapshot, bool, error) {
	if s == nil || s.db == nil {
		return nil, false, nil
	}
	key := CanonicalizeURL(feedURL)
	var (
		snap          FeedSnapshot
		retrievedAtMs int64
		validatedAtMs int64
		contentLength int64
		rawContent    []byte
	)
	err := s.db.QueryRow(
		`SELECT feed_url, retrieved_at, fingerprint, raw_content, content_length, etag, last_modified, validated_at, source_at_capture
		FROM `+FeedSnapshotsTableName+` WHERE feed_url = ?`,
		key,
	).Scan(&snap.FeedURL, &retrievedAtMs, &snap.Fingerprint, &rawContent, &contentLength, &snap.ETag, &snap.LastModified, &validatedAtMs, &snap.SourceAtCapture)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("load snapshot: %w", err)
	}
	snap.RawContent = rawContent
	snap.RetrievedAt = time.UnixMilli(retrievedAtMs).UTC()
	snap.ValidatedAt = time.UnixMilli(validatedAtMs).UTC()
	if err := validateSnapshot(&snap); err != nil {
		return nil, false, fmt.Errorf("%w: %v", ErrSnapshotCorrupted, err)
	}
	clone := snap
	clone.RawContent = append([]byte(nil), snap.RawContent...)
	return &clone, true, nil
}

func (s *SQLiteSnapshotStore) Delete(feedURL string) error {
	if s == nil || s.db == nil {
		return nil
	}
	key := CanonicalizeURL(feedURL)
	if _, err := s.db.Exec("DELETE FROM "+FeedSnapshotsTableName+" WHERE feed_url = ?", key); err != nil {
		return fmt.Errorf("delete snapshot: %w", err)
	}
	return nil
}

// TouchValidatedAt advances validated_at for an existing row in its own
// transaction, leaving the body, fingerprint, and retrieved_at untouched. A 304
// confirms the persisted content is still current, so only the validation
// timestamp (which drives oldest-first eviction) moves forward. A missing row is
// a no-op rather than an error.
func (s *SQLiteSnapshotStore) TouchValidatedAt(feedURL string) error {
	if s == nil || s.db == nil {
		return nil
	}
	key := CanonicalizeURL(feedURL)
	tx, err := s.db.Begin()
	if err != nil {
		s.recordWriteFailure()
		return fmt.Errorf("begin snapshot touch transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	res, err := tx.Exec("UPDATE "+FeedSnapshotsTableName+" SET validated_at = ? WHERE feed_url = ?", time.Now().UTC().UnixMilli(), key)
	if err != nil {
		s.recordWriteFailure()
		return fmt.Errorf("touch snapshot validated_at: %w", err)
	}
	if err := tx.Commit(); err != nil {
		s.recordWriteFailure()
		return fmt.Errorf("commit snapshot touch transaction: %w", err)
	}
	committed = true
	if rows, err := res.RowsAffected(); err == nil && rows == 0 {
		// No row matched: nothing to touch. Not an error.
		return nil
	}
	return nil
}

func (s *SQLiteSnapshotStore) Stats() (SnapshotStoreStats, error) {
	if s == nil || s.db == nil {
		return SnapshotStoreStats{}, nil
	}
	var (
		entries int
		total   sql.NullInt64
	)
	if err := s.db.QueryRow("SELECT COUNT(*), COALESCE(SUM(content_length), 0) FROM "+FeedSnapshotsTableName).Scan(&entries, &total); err != nil {
		return SnapshotStoreStats{}, fmt.Errorf("read snapshot stats: %w", err)
	}
	s.mu.Lock()
	evicted, failures := s.evictedCount, s.writeFailures
	s.mu.Unlock()
	return SnapshotStoreStats{
		Entries:       entries,
		TotalBytes:    total.Int64,
		EvictedCount:  evicted,
		WriteFailures: failures,
	}, nil
}

func (s *SQLiteSnapshotStore) recordEvictions(count int64) {
	if count <= 0 {
		return
	}
	s.mu.Lock()
	s.evictedCount += count
	s.mu.Unlock()
}

func (s *SQLiteSnapshotStore) recordWriteFailure() {
	s.mu.Lock()
	s.writeFailures++
	s.mu.Unlock()
}

// TieredSnapshotStore layers an in-process L1 cache over a durable L2. A Load
// reads L1 first and backfills it from L2 on a miss; a Save always writes L1
// and, if L2 is present, writes L2 as well. An L2 durability failure is
// surfaced as ErrSnapshotNotPersisted so the caller can distinguish "fresh this
// process only" from "durable", and never report a broken durable write as
// success.
type TieredSnapshotStore struct {
	l1 *MemorySnapshotStore
	l2 FeedStateStore
}

func NewTieredSnapshotStore(l2 FeedStateStore, config LastGoodStoreConfig) *TieredSnapshotStore {
	return &TieredSnapshotStore{
		l1: NewMemorySnapshotStore(config),
		l2: l2,
	}
}

func (t *TieredSnapshotStore) Save(snapshot FeedSnapshot) error {
	if t == nil {
		return errors.New("tiered snapshot store is nil")
	}
	if err := t.l1.Save(snapshot); err != nil {
		return err
	}
	if t.l2 == nil {
		return nil
	}
	if err := t.l2.Save(snapshot); err != nil {
		return fmt.Errorf("%w: in-process copy kept, durable store failed: %v", ErrSnapshotNotPersisted, err)
	}
	return nil
}

func (t *TieredSnapshotStore) Load(feedURL string) (*FeedSnapshot, bool, error) {
	if t == nil {
		return nil, false, nil
	}
	snap, ok, err := t.l1.Load(feedURL)
	if err != nil {
		return nil, false, err
	}
	if ok {
		return snap, true, nil
	}
	if t.l2 == nil {
		return nil, false, nil
	}
	snap, ok, err = t.l2.Load(feedURL)
	if err != nil {
		return nil, false, err
	}
	if !ok || snap == nil {
		return nil, false, nil
	}
	// Backfill L1 so subsequent reads are served from memory.
	_ = t.l1.Save(*snap)
	return snap, true, nil
}

func (t *TieredSnapshotStore) Delete(feedURL string) error {
	if t == nil {
		return nil
	}
	var first error
	if err := t.l1.Delete(feedURL); err != nil && first == nil {
		first = err
	}
	if t.l2 != nil {
		if err := t.l2.Delete(feedURL); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// TouchValidatedAt advances validated_at in both layers so a 304 keeps the
// durable row's eviction priority current. L1 is touched first so an in-process
// reader sees the new timestamp even if L2 is unavailable.
func (t *TieredSnapshotStore) TouchValidatedAt(feedURL string) error {
	if t == nil {
		return nil
	}
	if err := t.l1.TouchValidatedAt(feedURL); err != nil {
		return err
	}
	if t.l2 == nil {
		return nil
	}
	return t.l2.TouchValidatedAt(feedURL)
}

func (t *TieredSnapshotStore) Stats() (SnapshotStoreStats, error) {
	if t == nil {
		return SnapshotStoreStats{}, nil
	}
	l1Stats, err := t.l1.Stats()
	if err != nil {
		return SnapshotStoreStats{}, err
	}
	if t.l2 == nil {
		return l1Stats, nil
	}
	l2Stats, err := t.l2.Stats()
	if err != nil {
		return SnapshotStoreStats{}, err
	}
	return SnapshotStoreStats{
		Entries:       l2Stats.Entries,
		TotalBytes:    l2Stats.TotalBytes,
		EvictedCount:  l1Stats.EvictedCount + l2Stats.EvictedCount,
		WriteFailures: l2Stats.WriteFailures,
	}, nil
}
