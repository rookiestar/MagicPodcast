package feed

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// FeedUserAgentGatesTableName is the durable cross-job User-Agent policy
// table. It stores only a target domain and a one-way User-Agent fingerprint.
const FeedUserAgentGatesTableName = "feed_user_agent_gates"

// FeedUserAgentGatesCreateTableSQL is shared by the migration and SQLite store
// contract. Times are stored as Unix milliseconds, matching other durable Feed
// state tables.
const FeedUserAgentGatesCreateTableSQL = `CREATE TABLE IF NOT EXISTS ` + FeedUserAgentGatesTableName + ` (
    domain                   TEXT NOT NULL,
    user_agent_fingerprint   TEXT NOT NULL,
    state                    TEXT NOT NULL DEFAULT 'blocked',
    detected_at              INTEGER NOT NULL,
    probe_eligible_at        INTEGER NOT NULL,
    last_probe_result        TEXT NOT NULL DEFAULT '',
    recovery_success_count   INTEGER NOT NULL DEFAULT 0,
    updated_at               INTEGER NOT NULL,
    PRIMARY KEY (domain, user_agent_fingerprint)
)`

const FeedUserAgentGatesCreateIndexSQL = `CREATE INDEX IF NOT EXISTS idx_feed_user_agent_gates_state
ON ` + FeedUserAgentGatesTableName + ` (domain, state, probe_eligible_at)`

const (
	UserAgentGateStateActive       = "active"
	UserAgentGateStateBlocked      = "blocked"
	UserAgentGateStateProbePending = "probe_pending"
	UserAgentGateStateRecovering   = "recovering"

	// DefaultUserAgentBlockCooldown is an eligibility time, not an automatic
	// unblock. A later human-approved recovery action is #49.
	DefaultUserAgentBlockCooldown = 24 * time.Hour
)

// persistentUserAgentGateRuntime closes the check-then-fetch race between
// concurrent Job workers and separate Fetcher instances in one process. The
// durable row remains the source of truth across restarts; this narrow lock
// only serializes the same domain/fingerprint while a primary request is in
// flight.
var persistentUserAgentGateRuntime = struct {
	sync.Mutex
	locks map[string]chan struct{}
}{locks: make(map[string]chan struct{})}

func acquirePersistentUserAgentGate(ctx context.Context, domain, userAgentFingerprint string) (func(), bool) {
	if ctx == nil {
		ctx = context.Background()
	}
	domain, userAgentFingerprint, err := normalizeUserAgentGateKey(domain, userAgentFingerprint)
	if err != nil {
		return func() {}, true
	}
	key := domain + "|" + userAgentFingerprint
	persistentUserAgentGateRuntime.Lock()
	lock := persistentUserAgentGateRuntime.locks[key]
	if lock == nil {
		lock = make(chan struct{}, 1)
		lock <- struct{}{}
		persistentUserAgentGateRuntime.locks[key] = lock
	}
	persistentUserAgentGateRuntime.Unlock()

	select {
	case <-ctx.Done():
		return nil, false
	case <-lock:
		return func() { lock <- struct{}{} }, true
	}
}

// UserAgentGateFetchMode identifies the persistent admission used for one
// primary request. #48 only admits ordinary active traffic; probe/recovery
// admission is added by #49.
type UserAgentGateFetchMode string

const (
	UserAgentGateFetchModeActive  UserAgentGateFetchMode = "active"
	UserAgentGateFetchModeBlocked UserAgentGateFetchMode = "blocked"
)

// UserAgentGateRecord is the bounded durable policy state for one domain and
// one configured User-Agent fingerprint. The fingerprint is internal and must
// be reduced to a prefix at external diagnostics seams.
type UserAgentGateRecord struct {
	Domain               string
	UserAgentFingerprint string
	State                string
	DetectedAt           time.Time
	ProbeEligibleAt      time.Time
	LastProbeResult      string
	RecoverySuccessCount int
	UpdatedAt            time.Time
}

type UserAgentGateFetchDecision struct {
	Mode   UserAgentGateFetchMode
	Record UserAgentGateRecord
}

// UserAgentGateStore is the storage seam used by Fetcher. Implementations must
// never persist raw User-Agent strings, response headers, cookies, credentials,
// or Feed bodies.
type UserAgentGateStore interface {
	IsBlocked(ctx context.Context, domain, userAgentFingerprint string, now time.Time) (bool, UserAgentGateRecord, error)
	Block(ctx context.Context, domain, userAgentFingerprint string, now time.Time) error
	PreparePrimaryFetch(ctx context.Context, domain, userAgentFingerprint string, now time.Time) (UserAgentGateFetchDecision, error)
	List(ctx context.Context) ([]UserAgentGateRecord, error)
}

// SQLiteUserAgentGateStore is a durable store over an already-migrated SQLite
// database. It never creates or alters tables at runtime.
type SQLiteUserAgentGateStore struct {
	db *sql.DB
}

func NewSQLiteUserAgentGateStore(db *sql.DB) (*SQLiteUserAgentGateStore, error) {
	if db == nil {
		return nil, errors.New("sqlite User-Agent gate store requires a non-nil database handle")
	}
	return &SQLiteUserAgentGateStore{db: db}, nil
}

type gateQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (s *SQLiteUserAgentGateStore) ensure() error {
	if s == nil || s.db == nil {
		return errors.New("sqlite User-Agent gate store requires a database")
	}
	return nil
}

func (s *SQLiteUserAgentGateStore) get(ctx context.Context, q gateQueryer, domain, userAgentFingerprint string) (UserAgentGateRecord, bool, error) {
	var (
		record                     UserAgentGateRecord
		detectedAtMs, eligibleAtMs int64
		updatedAtMs                int64
	)
	err := q.QueryRowContext(ctx, `SELECT domain, user_agent_fingerprint, state,
		detected_at, probe_eligible_at, last_probe_result, recovery_success_count, updated_at
		FROM `+FeedUserAgentGatesTableName+`
		WHERE domain = ? AND user_agent_fingerprint = ?`, domain, userAgentFingerprint).
		Scan(&record.Domain, &record.UserAgentFingerprint, &record.State,
			&detectedAtMs, &eligibleAtMs, &record.LastProbeResult,
			&record.RecoverySuccessCount, &updatedAtMs)
	if errors.Is(err, sql.ErrNoRows) {
		return UserAgentGateRecord{}, false, nil
	}
	if err != nil {
		return UserAgentGateRecord{}, false, fmt.Errorf("read User-Agent gate: %w", err)
	}
	record.DetectedAt = timeFromMillis(detectedAtMs)
	record.ProbeEligibleAt = timeFromMillis(eligibleAtMs)
	record.UpdatedAt = timeFromMillis(updatedAtMs)
	return record, true, nil
}

func (s *SQLiteUserAgentGateStore) IsBlocked(ctx context.Context, domain, userAgentFingerprint string, now time.Time) (bool, UserAgentGateRecord, error) {
	if err := s.ensure(); err != nil {
		return false, UserAgentGateRecord{}, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	domain, userAgentFingerprint, err := normalizeUserAgentGateKey(domain, userAgentFingerprint)
	if err != nil {
		return false, UserAgentGateRecord{}, err
	}
	record, exists, err := s.get(ctx, s.db, domain, userAgentFingerprint)
	if err != nil || exists {
		return exists && record.State == UserAgentGateStateBlocked, record, err
	}
	blocked, found, err := s.getDomainNonActive(ctx, s.db, domain)
	return found, blocked, err
}

func (s *SQLiteUserAgentGateStore) Block(ctx context.Context, domain, userAgentFingerprint string, now time.Time) error {
	if err := s.ensure(); err != nil {
		return err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	domain, userAgentFingerprint, err := normalizeUserAgentGateKey(domain, userAgentFingerprint)
	if err != nil {
		return err
	}
	if now.IsZero() {
		now = time.Now()
	}
	now = now.UTC()
	when := now.UnixMilli()
	if _, err := s.db.ExecContext(ctx, `INSERT INTO `+FeedUserAgentGatesTableName+` (
		domain, user_agent_fingerprint, state, detected_at, probe_eligible_at,
		last_probe_result, recovery_success_count, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, 0, ?)
	ON CONFLICT(domain, user_agent_fingerprint) DO UPDATE SET
		state = excluded.state,
		detected_at = excluded.detected_at,
		probe_eligible_at = excluded.probe_eligible_at,
		last_probe_result = excluded.last_probe_result,
		recovery_success_count = 0,
		updated_at = excluded.updated_at`,
		domain, userAgentFingerprint, UserAgentGateStateBlocked, when,
		now.Add(DefaultUserAgentBlockCooldown).UnixMilli(),
		string(ErrorCategoryUserAgentDenied), when); err != nil {
		return fmt.Errorf("persist User-Agent gate block: %w", err)
	}
	return nil
}

func (s *SQLiteUserAgentGateStore) PreparePrimaryFetch(ctx context.Context, domain, userAgentFingerprint string, now time.Time) (UserAgentGateFetchDecision, error) {
	if err := s.ensure(); err != nil {
		return UserAgentGateFetchDecision{}, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	domain, userAgentFingerprint, err := normalizeUserAgentGateKey(domain, userAgentFingerprint)
	if err != nil {
		return UserAgentGateFetchDecision{}, err
	}
	record, exists, err := s.get(ctx, s.db, domain, userAgentFingerprint)
	if err != nil {
		return UserAgentGateFetchDecision{}, err
	}
	if exists {
		if record.State == UserAgentGateStateActive {
			return UserAgentGateFetchDecision{Mode: UserAgentGateFetchModeActive, Record: record}, nil
		}
		return UserAgentGateFetchDecision{Mode: UserAgentGateFetchModeBlocked, Record: record}, nil
	}
	// A changed or otherwise unknown UA cannot silently bypass a blocked domain.
	// It becomes eligible only after #49 explicitly approves a pending row.
	if blocked, found, err := s.getDomainNonActive(ctx, s.db, domain); err != nil {
		return UserAgentGateFetchDecision{}, err
	} else if found {
		return UserAgentGateFetchDecision{Mode: UserAgentGateFetchModeBlocked, Record: blocked}, nil
	}
	return UserAgentGateFetchDecision{Mode: UserAgentGateFetchModeActive}, nil
}

func (s *SQLiteUserAgentGateStore) List(ctx context.Context) ([]UserAgentGateRecord, error) {
	if err := s.ensure(); err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	rows, err := s.db.QueryContext(ctx, `SELECT domain, user_agent_fingerprint, state,
		detected_at, probe_eligible_at, last_probe_result, recovery_success_count, updated_at
		FROM `+FeedUserAgentGatesTableName+` ORDER BY domain ASC, user_agent_fingerprint ASC`)
	if err != nil {
		return nil, fmt.Errorf("list User-Agent gates: %w", err)
	}
	defer rows.Close()
	var records []UserAgentGateRecord
	for rows.Next() {
		var (
			record                     UserAgentGateRecord
			detectedAtMs, eligibleAtMs int64
			updatedAtMs                int64
		)
		if err := rows.Scan(&record.Domain, &record.UserAgentFingerprint, &record.State,
			&detectedAtMs, &eligibleAtMs, &record.LastProbeResult,
			&record.RecoverySuccessCount, &updatedAtMs); err != nil {
			return nil, fmt.Errorf("scan User-Agent gate: %w", err)
		}
		record.DetectedAt = timeFromMillis(detectedAtMs)
		record.ProbeEligibleAt = timeFromMillis(eligibleAtMs)
		record.UpdatedAt = timeFromMillis(updatedAtMs)
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read User-Agent gates: %w", err)
	}
	sort.Slice(records, func(i, j int) bool {
		if records[i].Domain != records[j].Domain {
			return records[i].Domain < records[j].Domain
		}
		return records[i].UserAgentFingerprint < records[j].UserAgentFingerprint
	})
	return records, nil
}

func (s *SQLiteUserAgentGateStore) getDomainNonActive(ctx context.Context, q gateQueryer, domain string) (UserAgentGateRecord, bool, error) {
	var (
		record                     UserAgentGateRecord
		detectedAtMs, eligibleAtMs int64
		updatedAtMs                int64
	)
	err := q.QueryRowContext(ctx, `SELECT domain, user_agent_fingerprint, state,
		detected_at, probe_eligible_at, last_probe_result, recovery_success_count, updated_at
		FROM `+FeedUserAgentGatesTableName+`
		WHERE domain = ? AND state <> ? ORDER BY updated_at DESC LIMIT 1`, domain, UserAgentGateStateActive).
		Scan(&record.Domain, &record.UserAgentFingerprint, &record.State,
			&detectedAtMs, &eligibleAtMs, &record.LastProbeResult,
			&record.RecoverySuccessCount, &updatedAtMs)
	if errors.Is(err, sql.ErrNoRows) {
		return UserAgentGateRecord{}, false, nil
	}
	if err != nil {
		return UserAgentGateRecord{}, false, fmt.Errorf("read domain User-Agent gate: %w", err)
	}
	record.DetectedAt = timeFromMillis(detectedAtMs)
	record.ProbeEligibleAt = timeFromMillis(eligibleAtMs)
	record.UpdatedAt = timeFromMillis(updatedAtMs)
	return record, true, nil
}

func timeFromMillis(value int64) time.Time {
	if value == 0 {
		return time.Time{}
	}
	return time.UnixMilli(value).UTC()
}

func normalizeUserAgentGateKey(domain, userAgentFingerprint string) (string, string, error) {
	domain = strings.ToLower(strings.TrimSpace(domain))
	userAgentFingerprint = strings.ToLower(strings.TrimSpace(userAgentFingerprint))
	if domain == "" {
		return "", "", errors.New("User-Agent gate domain is required")
	}
	if userAgentFingerprint == "" {
		return "", "", errors.New("User-Agent gate fingerprint is required")
	}
	return domain, userAgentFingerprint, nil
}
