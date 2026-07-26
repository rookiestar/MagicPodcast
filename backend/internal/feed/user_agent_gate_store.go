package feed

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// FeedUserAgentGatesTableName is the durable cross-job User-Agent policy
// table. It stores only a target domain and a one-way User-Agent fingerprint.
const FeedUserAgentGatesTableName = "feed_user_agent_gates"

// FeedUserAgentGateAuditsTableName stores the bounded operator audit trail for
// dry-run and apply approval actions. It contains no raw User-Agent or HTTP
// material.
const FeedUserAgentGateAuditsTableName = "feed_user_agent_gate_audits"

// FeedUserAgentGateRecoveryFeedsTableName stores one-way fingerprints of
// distinct Feeds that have succeeded during gradual recovery.
const FeedUserAgentGateRecoveryFeedsTableName = "feed_user_agent_gate_recovery_feeds"

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
    approved_by              TEXT NOT NULL DEFAULT '',
    approved_at              INTEGER,
    last_probe_at            INTEGER,
    updated_at               INTEGER NOT NULL,
    PRIMARY KEY (domain, user_agent_fingerprint)
)`

// FeedUserAgentGatesCreateTableSQLV13 is kept for the version-13 migration.
// Fresh databases may use the current DDL above, while databases that already
// reached #48 receive these recovery columns in schema 14.
const FeedUserAgentGatesCreateTableSQLV13 = `CREATE TABLE IF NOT EXISTS ` + FeedUserAgentGatesTableName + ` (
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

const FeedUserAgentGateAuditsCreateTableSQL = `CREATE TABLE IF NOT EXISTS ` + FeedUserAgentGateAuditsTableName + ` (
    id                       INTEGER PRIMARY KEY AUTOINCREMENT,
    domain                   TEXT NOT NULL,
    user_agent_fingerprint   TEXT NOT NULL,
    action                   TEXT NOT NULL,
    mode                     TEXT NOT NULL,
    actor                    TEXT NOT NULL DEFAULT '',
    result                   TEXT NOT NULL DEFAULT '',
    created_at               INTEGER NOT NULL
)`

const FeedUserAgentGateAuditsCreateIndexSQL = `CREATE INDEX IF NOT EXISTS idx_feed_user_agent_gate_audits_key
ON ` + FeedUserAgentGateAuditsTableName + ` (domain, user_agent_fingerprint, id)`

const FeedUserAgentGateRecoveryFeedsCreateTableSQL = `CREATE TABLE IF NOT EXISTS ` + FeedUserAgentGateRecoveryFeedsTableName + ` (
    domain                   TEXT NOT NULL,
    user_agent_fingerprint   TEXT NOT NULL,
    feed_fingerprint         TEXT NOT NULL,
    status                   TEXT NOT NULL DEFAULT 'in_flight',
    first_observed_at        INTEGER NOT NULL,
    last_success_at          INTEGER,
    PRIMARY KEY (domain, user_agent_fingerprint, feed_fingerprint)
)`

const FeedUserAgentGateRecoveryFeedsCreateIndexSQL = `CREATE INDEX IF NOT EXISTS idx_feed_user_agent_gate_recovery_status
ON ` + FeedUserAgentGateRecoveryFeedsTableName + ` (domain, user_agent_fingerprint, status)`

const (
	UserAgentGateStateActive        = "active"
	UserAgentGateStateBlocked       = "blocked"
	UserAgentGateStateProbePending  = "probe_pending"
	UserAgentGateStateProbeInFlight = "probe_in_flight"
	UserAgentGateStateRecovering    = "recovering"

	// DefaultUserAgentBlockCooldown is an eligibility time, not an automatic
	// unblock. A later human-approved recovery action is #49.
	DefaultUserAgentBlockCooldown = 24 * time.Hour
)

const (
	UserAgentGateFetchModeActive   UserAgentGateFetchMode = "active"
	UserAgentGateFetchModeBlocked  UserAgentGateFetchMode = "blocked"
	UserAgentGateFetchModeProbe    UserAgentGateFetchMode = "probe"
	UserAgentGateFetchModeRecovery UserAgentGateFetchMode = "recovery"
)

const (
	UserAgentGateAuditActionApproveProbe = "approve_probe"
	UserAgentGateAuditModeDryRun         = "dry_run"
	UserAgentGateAuditModeApply          = "apply"
)

const recoveryFeedStatusSuccess = "success"

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
// primary request. #48 admits ordinary active traffic; #49 adds one probe and
// one new Feed at a time during gradual recovery.
type UserAgentGateFetchMode string

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
	ApprovedBy           string
	ApprovedAt           *time.Time
	LastProbeAt          *time.Time
	UpdatedAt            time.Time
}

type UserAgentGateFetchDecision struct {
	Mode            UserAgentGateFetchMode
	Record          UserAgentGateRecord
	FeedFingerprint string
}

// UserAgentGateApproval is the result of a dry-run or apply maintenance action.
// Dry-run still writes an audit row but never mutates the gate state.
type UserAgentGateApproval struct {
	Applied  bool
	Eligible bool
	Record   UserAgentGateRecord
}

// UserAgentGateAudit is the bounded operator audit projection.
type UserAgentGateAudit struct {
	ID                   int64
	Domain               string
	UserAgentFingerprint string
	Action               string
	Mode                 string
	Actor                string
	Result               string
	CreatedAt            time.Time
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

// UserAgentGateRecoveryStore extends the #48 store with the #49 recovery
// state machine. Keeping it as an optional extension preserves test and
// maintenance seams that only need ordinary active/blocked admission.
type UserAgentGateRecoveryStore interface {
	UserAgentGateStore
	PreparePrimaryFetchForFeed(ctx context.Context, domain, userAgentFingerprint, feedFingerprint string, now time.Time) (UserAgentGateFetchDecision, error)
	RecordPrimaryFetchResult(ctx context.Context, domain, userAgentFingerprint, feedFingerprint string, outcome AccessOutcome, now time.Time) (UserAgentGateRecord, error)
}

// UserAgentGateMaintenanceStore is the protected operator-action seam. The
// maintenance operation changes only durable policy state and never performs a
// Feed request.
type UserAgentGateMaintenanceStore interface {
	UserAgentGateStore
	ApproveProbe(ctx context.Context, domain, userAgentFingerprint, actor string, now time.Time, apply bool) (UserAgentGateApproval, error)
	ListAudits(ctx context.Context, domain, userAgentFingerprint string) ([]UserAgentGateAudit, error)
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

type gateExecer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func (s *SQLiteUserAgentGateStore) ensure() error {
	if s == nil || s.db == nil {
		return errors.New("sqlite User-Agent gate store requires a database")
	}
	return nil
}

func (s *SQLiteUserAgentGateStore) get(ctx context.Context, q gateQueryer, domain, userAgentFingerprint string) (UserAgentGateRecord, bool, error) {
	var (
		record                      UserAgentGateRecord
		detectedAtMs, eligibleAtMs  int64
		updatedAtMs                 int64
		approvedAtMs, lastProbeAtMs sql.NullInt64
	)
	err := q.QueryRowContext(ctx, `SELECT domain, user_agent_fingerprint, state,
		detected_at, probe_eligible_at, last_probe_result, recovery_success_count,
		approved_by, approved_at, last_probe_at, updated_at
		FROM `+FeedUserAgentGatesTableName+`
		WHERE domain = ? AND user_agent_fingerprint = ?`, domain, userAgentFingerprint).
		Scan(&record.Domain, &record.UserAgentFingerprint, &record.State,
			&detectedAtMs, &eligibleAtMs, &record.LastProbeResult,
			&record.RecoverySuccessCount, &record.ApprovedBy, &approvedAtMs,
			&lastProbeAtMs, &updatedAtMs)
	if errors.Is(err, sql.ErrNoRows) {
		return UserAgentGateRecord{}, false, nil
	}
	if err != nil {
		return UserAgentGateRecord{}, false, fmt.Errorf("read User-Agent gate: %w", err)
	}
	record.DetectedAt = timeFromMillis(detectedAtMs)
	record.ProbeEligibleAt = timeFromMillis(eligibleAtMs)
	record.ApprovedAt = nullableTimeFromMillis(approvedAtMs)
	record.LastProbeAt = nullableTimeFromMillis(lastProbeAtMs)
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
		return exists && record.State != UserAgentGateStateActive, record, err
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
	now = normalizeGateTime(now)
	when := now.UnixMilli()
	if _, err := s.db.ExecContext(ctx, `INSERT INTO `+FeedUserAgentGatesTableName+` (
		domain, user_agent_fingerprint, state, detected_at, probe_eligible_at,
		last_probe_result, recovery_success_count, approved_by, approved_at, last_probe_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, 0, '', NULL, NULL, ?)
	ON CONFLICT(domain, user_agent_fingerprint) DO UPDATE SET
		state = excluded.state,
		detected_at = excluded.detected_at,
		probe_eligible_at = excluded.probe_eligible_at,
		last_probe_result = excluded.last_probe_result,
		recovery_success_count = 0,
		approved_by = '',
		approved_at = NULL,
		last_probe_at = NULL,
		updated_at = excluded.updated_at`,
		domain, userAgentFingerprint, UserAgentGateStateBlocked, when,
		now.Add(DefaultUserAgentBlockCooldown).UnixMilli(),
		string(ErrorCategoryUserAgentDenied), when); err != nil {
		return fmt.Errorf("persist User-Agent gate block: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM `+FeedUserAgentGateRecoveryFeedsTableName+`
		WHERE domain = ? AND user_agent_fingerprint = ?`, domain, userAgentFingerprint); err != nil {
		return fmt.Errorf("clear User-Agent recovery progress: %w", err)
	}
	return nil
}

// PreparePrimaryFetch is the #48 admission seam. Recovery-aware callers use
// PreparePrimaryFetchForFeed so a pending gate can claim exactly one probe.
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
		detected_at, probe_eligible_at, last_probe_result, recovery_success_count,
		approved_by, approved_at, last_probe_at, updated_at
		FROM `+FeedUserAgentGatesTableName+` ORDER BY domain ASC, user_agent_fingerprint ASC`)
	if err != nil {
		return nil, fmt.Errorf("list User-Agent gates: %w", err)
	}
	defer rows.Close()
	var records []UserAgentGateRecord
	for rows.Next() {
		var (
			record                      UserAgentGateRecord
			detectedAtMs, eligibleAtMs  int64
			updatedAtMs                 int64
			approvedAtMs, lastProbeAtMs sql.NullInt64
		)
		if err := rows.Scan(&record.Domain, &record.UserAgentFingerprint, &record.State,
			&detectedAtMs, &eligibleAtMs, &record.LastProbeResult,
			&record.RecoverySuccessCount, &record.ApprovedBy, &approvedAtMs,
			&lastProbeAtMs, &updatedAtMs); err != nil {
			return nil, fmt.Errorf("scan User-Agent gate: %w", err)
		}
		record.DetectedAt = timeFromMillis(detectedAtMs)
		record.ProbeEligibleAt = timeFromMillis(eligibleAtMs)
		record.ApprovedAt = nullableTimeFromMillis(approvedAtMs)
		record.LastProbeAt = nullableTimeFromMillis(lastProbeAtMs)
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
		record                      UserAgentGateRecord
		detectedAtMs, eligibleAtMs  int64
		updatedAtMs                 int64
		approvedAtMs, lastProbeAtMs sql.NullInt64
	)
	err := q.QueryRowContext(ctx, `SELECT domain, user_agent_fingerprint, state,
		detected_at, probe_eligible_at, last_probe_result, recovery_success_count,
		approved_by, approved_at, last_probe_at, updated_at
		FROM `+FeedUserAgentGatesTableName+`
		WHERE domain = ? AND state <> ? ORDER BY updated_at DESC LIMIT 1`, domain, UserAgentGateStateActive).
		Scan(&record.Domain, &record.UserAgentFingerprint, &record.State,
			&detectedAtMs, &eligibleAtMs, &record.LastProbeResult,
			&record.RecoverySuccessCount, &record.ApprovedBy, &approvedAtMs,
			&lastProbeAtMs, &updatedAtMs)
	if errors.Is(err, sql.ErrNoRows) {
		return UserAgentGateRecord{}, false, nil
	}
	if err != nil {
		return UserAgentGateRecord{}, false, fmt.Errorf("read domain User-Agent gate: %w", err)
	}
	record.DetectedAt = timeFromMillis(detectedAtMs)
	record.ProbeEligibleAt = timeFromMillis(eligibleAtMs)
	record.ApprovedAt = nullableTimeFromMillis(approvedAtMs)
	record.LastProbeAt = nullableTimeFromMillis(lastProbeAtMs)
	record.UpdatedAt = timeFromMillis(updatedAtMs)
	return record, true, nil
}

// ApproveProbe records an auditable human decision. Dry-run evaluates the
// cooldown and writes only the audit row; apply atomically changes an eligible
// blocked gate to probe_pending. It never performs network I/O.
func (s *SQLiteUserAgentGateStore) ApproveProbe(ctx context.Context, domain, userAgentFingerprint, actor string, now time.Time, apply bool) (UserAgentGateApproval, error) {
	if err := s.ensure(); err != nil {
		return UserAgentGateApproval{}, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	domain, userAgentFingerprint, err := normalizeUserAgentGateKey(domain, userAgentFingerprint)
	if err != nil {
		return UserAgentGateApproval{}, err
	}
	actor, err = normalizeGateActor(actor)
	if err != nil {
		return UserAgentGateApproval{}, err
	}
	now = normalizeGateTime(now)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return UserAgentGateApproval{}, fmt.Errorf("begin User-Agent probe approval: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	record, exists, err := s.get(ctx, tx, domain, userAgentFingerprint)
	if err != nil {
		return UserAgentGateApproval{}, err
	}
	if !exists {
		return UserAgentGateApproval{}, fmt.Errorf("User-Agent gate not found for domain %q", domain)
	}
	eligible := record.State == UserAgentGateStateBlocked && !now.Before(record.ProbeEligibleAt)
	approval := UserAgentGateApproval{Eligible: eligible, Record: record}
	mode := UserAgentGateAuditModeDryRun
	if apply {
		mode = UserAgentGateAuditModeApply
	}
	if apply && eligible {
		result, updateErr := tx.ExecContext(ctx, `UPDATE `+FeedUserAgentGatesTableName+`
			SET state = ?, approved_by = ?, approved_at = ?, last_probe_result = ?, updated_at = ?
			WHERE domain = ? AND user_agent_fingerprint = ? AND state = ? AND probe_eligible_at <= ?`,
			UserAgentGateStateProbePending, actor, now.UnixMilli(), "approved", now.UnixMilli(),
			domain, userAgentFingerprint, UserAgentGateStateBlocked, now.UnixMilli())
		if updateErr != nil {
			return UserAgentGateApproval{}, fmt.Errorf("apply User-Agent probe approval: %w", updateErr)
		}
		if affected, affectedErr := result.RowsAffected(); affectedErr == nil && affected == 1 {
			approval.Applied = true
			approval.Record, _, err = s.get(ctx, tx, domain, userAgentFingerprint)
			if err != nil {
				return UserAgentGateApproval{}, err
			}
		}
	}
	resultLabel := approval.Record.State
	if approval.Applied {
		resultLabel = UserAgentGateStateProbePending
	}
	if err := appendUserAgentGateAudit(ctx, tx, domain, userAgentFingerprint,
		UserAgentGateAuditActionApproveProbe, mode, actor, resultLabel, now); err != nil {
		return UserAgentGateApproval{}, err
	}
	if err := tx.Commit(); err != nil {
		return UserAgentGateApproval{}, fmt.Errorf("commit User-Agent probe approval: %w", err)
	}
	return approval, nil
}

// ListAudits lists the bounded approval history for one gate in insertion
// order. It is intentionally scoped to a gate so an operator cannot retrieve
// unrelated audit data through this seam.
func (s *SQLiteUserAgentGateStore) ListAudits(ctx context.Context, domain, userAgentFingerprint string) ([]UserAgentGateAudit, error) {
	if err := s.ensure(); err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	domain, userAgentFingerprint, err := normalizeUserAgentGateKey(domain, userAgentFingerprint)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, domain, user_agent_fingerprint,
		action, mode, actor, result, created_at
		FROM `+FeedUserAgentGateAuditsTableName+`
		WHERE domain = ? AND user_agent_fingerprint = ? ORDER BY id ASC`, domain, userAgentFingerprint)
	if err != nil {
		return nil, fmt.Errorf("list User-Agent gate audits: %w", err)
	}
	defer rows.Close()
	var audits []UserAgentGateAudit
	for rows.Next() {
		var audit UserAgentGateAudit
		var createdAtMs int64
		if err := rows.Scan(&audit.ID, &audit.Domain, &audit.UserAgentFingerprint,
			&audit.Action, &audit.Mode, &audit.Actor, &audit.Result, &createdAtMs); err != nil {
			return nil, fmt.Errorf("scan User-Agent gate audit: %w", err)
		}
		audit.CreatedAt = timeFromMillis(createdAtMs)
		audits = append(audits, audit)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read User-Agent gate audits: %w", err)
	}
	return audits, nil
}

// UserAgentGateFeedFingerprint returns a one-way identity for a Feed URL. The
// raw URL is never persisted in recovery progress.
func UserAgentGateFeedFingerprint(feedURL string) string {
	sum := sha256.Sum256([]byte(CanonicalizeURL(feedURL)))
	return hex.EncodeToString(sum[:])
}

// NormalizeUserAgentFingerprint validates the public maintenance input. The
// endpoint accepts only a SHA-256 digest, never a raw User-Agent value.
func NormalizeUserAgentFingerprint(value string) (string, error) {
	value, err := normalizeGateFingerprint(value, "User-Agent")
	if err != nil {
		return "", err
	}
	if len(value) != sha256.Size*2 {
		return "", errors.New("User-Agent fingerprint must be a SHA-256 hex digest")
	}
	if _, err := hex.DecodeString(value); err != nil {
		return "", errors.New("User-Agent fingerprint must be a SHA-256 hex digest")
	}
	return value, nil
}

// PreparePrimaryFetchForFeed admits one approved probe or one new Feed during
// gradual recovery. The recovery table reserves a Feed before network I/O, so
// concurrent processes cannot probe the same Feed twice.
func (s *SQLiteUserAgentGateStore) PreparePrimaryFetchForFeed(ctx context.Context, domain, userAgentFingerprint, feedFingerprint string, now time.Time) (UserAgentGateFetchDecision, error) {
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
	feedFingerprint, err = normalizeGateFingerprint(feedFingerprint, "Feed")
	if err != nil {
		return UserAgentGateFetchDecision{}, err
	}
	now = normalizeGateTime(now)

	record, exists, err := s.get(ctx, s.db, domain, userAgentFingerprint)
	if err != nil {
		return UserAgentGateFetchDecision{}, err
	}
	if !exists {
		if blocked, found, findErr := s.getDomainNonActive(ctx, s.db, domain); findErr != nil {
			return UserAgentGateFetchDecision{}, findErr
		} else if found {
			return UserAgentGateFetchDecision{Mode: UserAgentGateFetchModeBlocked, Record: blocked, FeedFingerprint: feedFingerprint}, nil
		}
		return UserAgentGateFetchDecision{Mode: UserAgentGateFetchModeActive, FeedFingerprint: feedFingerprint}, nil
	}

	switch record.State {
	case UserAgentGateStateActive:
		return UserAgentGateFetchDecision{Mode: UserAgentGateFetchModeActive, Record: record, FeedFingerprint: feedFingerprint}, nil
	case UserAgentGateStateBlocked, UserAgentGateStateProbeInFlight:
		return UserAgentGateFetchDecision{Mode: UserAgentGateFetchModeBlocked, Record: record, FeedFingerprint: feedFingerprint}, nil
	case UserAgentGateStateProbePending:
		result, updateErr := s.db.ExecContext(ctx, `UPDATE `+FeedUserAgentGatesTableName+`
			SET state = ?, last_probe_at = ?, last_probe_result = ?, updated_at = ?
			WHERE domain = ? AND user_agent_fingerprint = ? AND state = ?`,
			UserAgentGateStateProbeInFlight, now.UnixMilli(), "probe_in_flight", now.UnixMilli(),
			domain, userAgentFingerprint, UserAgentGateStateProbePending)
		if updateErr != nil {
			return UserAgentGateFetchDecision{}, fmt.Errorf("claim User-Agent probe: %w", updateErr)
		}
		if affected, affectedErr := result.RowsAffected(); affectedErr == nil && affected == 1 {
			record, _, err = s.get(ctx, s.db, domain, userAgentFingerprint)
			if err != nil {
				return UserAgentGateFetchDecision{}, err
			}
			return UserAgentGateFetchDecision{Mode: UserAgentGateFetchModeProbe, Record: record, FeedFingerprint: feedFingerprint}, nil
		}
		return UserAgentGateFetchDecision{Mode: UserAgentGateFetchModeBlocked, Record: record, FeedFingerprint: feedFingerprint}, nil
	case UserAgentGateStateRecovering:
		result, insertErr := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO `+FeedUserAgentGateRecoveryFeedsTableName+` (
			domain, user_agent_fingerprint, feed_fingerprint, status, first_observed_at, last_success_at
		) VALUES (?, ?, ?, 'in_flight', ?, NULL)`, domain, userAgentFingerprint, feedFingerprint, now.UnixMilli())
		if insertErr != nil {
			return UserAgentGateFetchDecision{}, fmt.Errorf("reserve User-Agent recovery Feed: %w", insertErr)
		}
		if affected, affectedErr := result.RowsAffected(); affectedErr == nil && affected == 1 {
			return UserAgentGateFetchDecision{Mode: UserAgentGateFetchModeRecovery, Record: record, FeedFingerprint: feedFingerprint}, nil
		}
		return UserAgentGateFetchDecision{Mode: UserAgentGateFetchModeBlocked, Record: record, FeedFingerprint: feedFingerprint}, nil
	default:
		return UserAgentGateFetchDecision{Mode: UserAgentGateFetchModeBlocked, Record: record, FeedFingerprint: feedFingerprint}, nil
	}
}

// RecordPrimaryFetchResult completes a claimed probe/recovery admission. Only
// HTTP 200/304 with a successful Feed outcome counts toward recovery. Every
// other result remains blocked; a fresh explicit UA ACL refusal also clears
// prior progress so it cannot be used to bypass the policy.
func (s *SQLiteUserAgentGateStore) RecordPrimaryFetchResult(ctx context.Context, domain, userAgentFingerprint, feedFingerprint string, outcome AccessOutcome, now time.Time) (UserAgentGateRecord, error) {
	if err := s.ensure(); err != nil {
		return UserAgentGateRecord{}, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	domain, userAgentFingerprint, err := normalizeUserAgentGateKey(domain, userAgentFingerprint)
	if err != nil {
		return UserAgentGateRecord{}, err
	}
	feedFingerprint, err = normalizeGateFingerprint(feedFingerprint, "Feed")
	if err != nil {
		return UserAgentGateRecord{}, err
	}
	now = normalizeGateTime(now)
	record, exists, err := s.get(ctx, s.db, domain, userAgentFingerprint)
	if err != nil {
		return UserAgentGateRecord{}, err
	}
	if !exists {
		return UserAgentGateRecord{}, fmt.Errorf("User-Agent gate not found for domain %q", domain)
	}
	if record.State != UserAgentGateStateProbeInFlight && record.State != UserAgentGateStateRecovering {
		return record, nil
	}

	if outcome.ErrorCategory == ErrorCategoryUserAgentDenied {
		if err := s.Block(ctx, domain, userAgentFingerprint, now); err != nil {
			return UserAgentGateRecord{}, err
		}
		return s.getExisting(ctx, domain, userAgentFingerprint)
	}

	if probeRecoverySuccess(outcome) {
		if _, err := s.db.ExecContext(ctx, `UPDATE `+FeedUserAgentGateRecoveryFeedsTableName+`
			SET status = ?, last_success_at = ?
			WHERE domain = ? AND user_agent_fingerprint = ? AND feed_fingerprint = ?`,
			recoveryFeedStatusSuccess, now.UnixMilli(), domain, userAgentFingerprint, feedFingerprint); err != nil {
			return UserAgentGateRecord{}, fmt.Errorf("record User-Agent recovery success: %w", err)
		}
		// A direct probe has no reservation row yet; create its successful row.
		if _, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO `+FeedUserAgentGateRecoveryFeedsTableName+` (
			domain, user_agent_fingerprint, feed_fingerprint, status, first_observed_at, last_success_at
		) VALUES (?, ?, ?, ?, ?, ?)`, domain, userAgentFingerprint, feedFingerprint, recoveryFeedStatusSuccess, now.UnixMilli(), now.UnixMilli()); err != nil {
			return UserAgentGateRecord{}, fmt.Errorf("persist User-Agent recovery Feed: %w", err)
		}
		count, err := s.recoverySuccessCount(ctx, domain, userAgentFingerprint)
		if err != nil {
			return UserAgentGateRecord{}, err
		}
		state := UserAgentGateStateRecovering
		if count >= 3 {
			state = UserAgentGateStateActive
		}
		if _, err := s.db.ExecContext(ctx, `UPDATE `+FeedUserAgentGatesTableName+`
			SET state = ?, last_probe_result = ?, recovery_success_count = ?, last_probe_at = ?, updated_at = ?
			WHERE domain = ? AND user_agent_fingerprint = ?`, state, recoveryResultLabel(outcome), count,
			now.UnixMilli(), now.UnixMilli(), domain, userAgentFingerprint); err != nil {
			return UserAgentGateRecord{}, fmt.Errorf("advance User-Agent recovery: %w", err)
		}
		return s.getExisting(ctx, domain, userAgentFingerprint)
	}

	// A failed recovery attempt releases its in-flight Feed reservation. Keep
	// previously proven distinct Feeds, but require a new human approval before
	// another network probe.
	if _, err := s.db.ExecContext(ctx, `DELETE FROM `+FeedUserAgentGateRecoveryFeedsTableName+`
		WHERE domain = ? AND user_agent_fingerprint = ? AND feed_fingerprint = ? AND status = 'in_flight'`,
		domain, userAgentFingerprint, feedFingerprint); err != nil {
		return UserAgentGateRecord{}, fmt.Errorf("release User-Agent recovery Feed: %w", err)
	}
	count, err := s.recoverySuccessCount(ctx, domain, userAgentFingerprint)
	if err != nil {
		return UserAgentGateRecord{}, err
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE `+FeedUserAgentGatesTableName+`
		SET state = ?, last_probe_result = ?, recovery_success_count = ?, last_probe_at = ?,
		probe_eligible_at = ?, updated_at = ?
		WHERE domain = ? AND user_agent_fingerprint = ?`, UserAgentGateStateBlocked,
		recoveryResultLabel(outcome), count, now.UnixMilli(), now.Add(DefaultUserAgentBlockCooldown).UnixMilli(), now.UnixMilli(),
		domain, userAgentFingerprint); err != nil {
		return UserAgentGateRecord{}, fmt.Errorf("preserve User-Agent recovery block: %w", err)
	}
	return s.getExisting(ctx, domain, userAgentFingerprint)
}

func (s *SQLiteUserAgentGateStore) getExisting(ctx context.Context, domain, userAgentFingerprint string) (UserAgentGateRecord, error) {
	record, exists, err := s.get(ctx, s.db, domain, userAgentFingerprint)
	if err != nil {
		return UserAgentGateRecord{}, err
	}
	if !exists {
		return UserAgentGateRecord{}, fmt.Errorf("User-Agent gate disappeared for domain %q", domain)
	}
	return record, nil
}

func (s *SQLiteUserAgentGateStore) recoverySuccessCount(ctx context.Context, domain, userAgentFingerprint string) (int, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+FeedUserAgentGateRecoveryFeedsTableName+`
		WHERE domain = ? AND user_agent_fingerprint = ? AND status = ?`, domain, userAgentFingerprint, recoveryFeedStatusSuccess).Scan(&count); err != nil {
		return 0, fmt.Errorf("count User-Agent recovery Feeds: %w", err)
	}
	return count, nil
}

func appendUserAgentGateAudit(ctx context.Context, q gateExecer, domain, userAgentFingerprint, action, mode, actor, result string, now time.Time) error {
	if _, err := q.ExecContext(ctx, `INSERT INTO `+FeedUserAgentGateAuditsTableName+` (
		domain, user_agent_fingerprint, action, mode, actor, result, created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?)`, domain, userAgentFingerprint, action, mode, actor, result, now.UnixMilli()); err != nil {
		return fmt.Errorf("record User-Agent gate audit: %w", err)
	}
	return nil
}

func probeRecoverySuccess(outcome AccessOutcome) bool {
	if outcome.ErrorCategory != ErrorCategoryNone || outcome.HTTPStatus == nil {
		return false
	}
	return *outcome.HTTPStatus == 200 || *outcome.HTTPStatus == 304
}

func recoveryResultLabel(outcome AccessOutcome) string {
	if outcome.HTTPStatus != nil && probeRecoverySuccess(outcome) {
		return strconv.Itoa(*outcome.HTTPStatus)
	}
	if outcome.ErrorCategory != "" && outcome.ErrorCategory != ErrorCategoryNotObserved {
		return string(outcome.ErrorCategory)
	}
	if outcome.HTTPStatus != nil {
		return fmt.Sprintf("http_%d", *outcome.HTTPStatus)
	}
	return "probe_failed"
}

func normalizeGateActor(actor string) (string, error) {
	actor = strings.TrimSpace(actor)
	if actor == "" {
		return "", errors.New("User-Agent probe actor is required")
	}
	if len(actor) > 128 {
		return "", errors.New("User-Agent probe actor is too long")
	}
	return actor, nil
}

func normalizeGateFingerprint(value, label string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "", fmt.Errorf("%s fingerprint is required", label)
	}
	return value, nil
}

func normalizeGateTime(now time.Time) time.Time {
	if now.IsZero() {
		now = time.Now()
	}
	return now.UTC()
}

func timeFromMillis(value int64) time.Time {
	if value == 0 {
		return time.Time{}
	}
	return time.UnixMilli(value).UTC()
}

func nullableTimeFromMillis(value sql.NullInt64) *time.Time {
	if !value.Valid || value.Int64 == 0 {
		return nil
	}
	when := timeFromMillis(value.Int64)
	return &when
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
