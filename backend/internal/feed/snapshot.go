package feed

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sort"
	"sync"
	"time"
)

const (
	defaultLastGoodMaxEntries       = 256
	defaultLastGoodMaxResponseBytes = 2 * 1024 * 1024
	defaultLastGoodMaxTotalBytes    = 32 * 1024 * 1024
	defaultLastGoodMaxStaleAge      = 7 * 24 * time.Hour
	maxCapturedFeedSnapshotBytes    = defaultLastGoodMaxResponseBytes
)

var ErrSnapshotResponseTooLarge = errors.New("feed snapshot response exceeds the configured limit")

// FeedSnapshot is the bounded, parsed-feed source retained for last-good
// fallback. RawContent is kept out of logs and API responses.
type FeedSnapshot struct {
	FeedURL     string
	RetrievedAt time.Time
	Fingerprint string
	RawContent  []byte
}

type SnapshotStore interface {
	Save(snapshot FeedSnapshot) error
	Load(feedURL string) (*FeedSnapshot, bool)
	Stats() SnapshotStoreStats
}

type LastGoodStoreConfig struct {
	MaxEntries       int
	MaxResponseBytes int
	MaxTotalBytes    int64
}

type SnapshotStoreStats struct {
	Entries    int
	TotalBytes int64
}

type memorySnapshot struct {
	FeedSnapshot
	storedAt time.Time
}

// MemorySnapshotStore keeps bounded snapshots in process memory. It is
// deliberately finite and independently injectable so a later durable store
// can be introduced without changing the Feed selection contract.
type MemorySnapshotStore struct {
	mu               sync.RWMutex
	maxEntries       int
	maxResponseBytes int
	maxTotalBytes    int64
	entries          map[string]memorySnapshot
	totalBytes       int64
}

func NewMemorySnapshotStore(config LastGoodStoreConfig) *MemorySnapshotStore {
	if config.MaxEntries <= 0 {
		config.MaxEntries = defaultLastGoodMaxEntries
	}
	if config.MaxResponseBytes <= 0 {
		config.MaxResponseBytes = defaultLastGoodMaxResponseBytes
	}
	if config.MaxTotalBytes <= 0 {
		config.MaxTotalBytes = defaultLastGoodMaxTotalBytes
	}
	return &MemorySnapshotStore{
		maxEntries:       config.MaxEntries,
		maxResponseBytes: config.MaxResponseBytes,
		maxTotalBytes:    config.MaxTotalBytes,
		entries:          make(map[string]memorySnapshot),
	}
}

func (s *MemorySnapshotStore) Save(snapshot FeedSnapshot) error {
	if s == nil {
		return errors.New("snapshot store is nil")
	}
	key := CanonicalizeURL(snapshot.FeedURL)
	if key == "" || len(snapshot.RawContent) == 0 {
		return errors.New("snapshot URL and content are required")
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
	if snapshot.Fingerprint == "" {
		snapshot.Fingerprint = fingerprint(snapshot.RawContent)
	}
	snapshot.FeedURL = key
	snapshot.RawContent = append([]byte(nil), snapshot.RawContent...)

	s.mu.Lock()
	defer s.mu.Unlock()

	if previous, ok := s.entries[key]; ok {
		s.totalBytes -= int64(len(previous.RawContent))
		delete(s.entries, key)
	}
	for len(s.entries) >= s.maxEntries || s.totalBytes+int64(len(snapshot.RawContent)) > s.maxTotalBytes {
		oldestKey, ok := s.oldestKey()
		if !ok {
			break
		}
		oldest := s.entries[oldestKey]
		delete(s.entries, oldestKey)
		s.totalBytes -= int64(len(oldest.RawContent))
	}
	s.entries[key] = memorySnapshot{FeedSnapshot: snapshot, storedAt: time.Now()}
	s.totalBytes += int64(len(snapshot.RawContent))
	return nil
}

func (s *MemorySnapshotStore) Load(feedURL string) (*FeedSnapshot, bool) {
	if s == nil {
		return nil, false
	}
	key := CanonicalizeURL(feedURL)
	s.mu.RLock()
	snapshot, ok := s.entries[key]
	s.mu.RUnlock()
	if !ok {
		return nil, false
	}
	copy := snapshot.FeedSnapshot
	copy.RawContent = append([]byte(nil), snapshot.RawContent...)
	return &copy, true
}

func (s *MemorySnapshotStore) Stats() SnapshotStoreStats {
	if s == nil {
		return SnapshotStoreStats{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return SnapshotStoreStats{Entries: len(s.entries), TotalBytes: s.totalBytes}
}

func (s *MemorySnapshotStore) oldestKey() (string, bool) {
	if len(s.entries) == 0 {
		return "", false
	}
	keys := make([]string, 0, len(s.entries))
	for key := range s.entries {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		left, right := s.entries[keys[i]], s.entries[keys[j]]
		if left.storedAt.Equal(right.storedAt) {
			return keys[i] < keys[j]
		}
		return left.storedAt.Before(right.storedAt)
	})
	return keys[0], true
}

func fingerprint(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

func validateSnapshot(snapshot *FeedSnapshot) error {
	if snapshot == nil || len(snapshot.RawContent) == 0 {
		return errors.New("last-good snapshot is empty")
	}
	if len(snapshot.RawContent) > maxCapturedFeedSnapshotBytes {
		return ErrSnapshotResponseTooLarge
	}
	if snapshot.Fingerprint == "" || snapshot.Fingerprint != fingerprint(snapshot.RawContent) {
		return errors.New("last-good snapshot fingerprint mismatch")
	}
	return nil
}

type boundedFeedCapture struct {
	maxBytes  int
	content   []byte
	oversized bool
}

func (c *boundedFeedCapture) Write(p []byte) (int, error) {
	if c.oversized {
		return len(p), nil
	}
	remaining := c.maxBytes - len(c.content)
	if len(p) > remaining {
		if remaining > 0 {
			c.content = append(c.content, p[:remaining]...)
		}
		c.oversized = true
		return len(p), nil
	}
	c.content = append(c.content, p...)
	return len(p), nil
}

func (c *boundedFeedCapture) Bytes() []byte {
	if c == nil || c.oversized || len(c.content) == 0 {
		return nil
	}
	return append([]byte(nil), c.content...)
}
