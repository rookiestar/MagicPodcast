package feed

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestUserAgentGateIdentityMigrationDryRunApply covers the auditable dry-run ->
// apply lifecycle. Dry-run must evaluate eligibility and audit both fingerprints
// without mutating state; apply must atomically retire the old identity and
// create the new identity as probe_pending with the actor recorded.
func TestUserAgentGateIdentityMigrationDryRunApply(t *testing.T) {
	store := openUserAgentGateRecoveryStore(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 26, 3, 0, 0, 0, time.UTC)
	domain := "feeds.example"
	oldFingerprint := UserAgentFingerprint("MagicPodcast/1.0 (+https://github.com/rookiestar/MagicPodcast)")
	newFingerprint := UserAgentFingerprint("MagicPodcastFeed/2.0 (+https://github.com/rookiestar/MagicPodcast)")
	require.NoError(t, store.Block(ctx, domain, oldFingerprint, now))

	dryRun, err := store.MigrateIdentity(ctx, domain, oldFingerprint, newFingerprint, "owner", now.Add(time.Hour), false)
	require.NoError(t, err)
	require.True(t, dryRun.Eligible)
	require.False(t, dryRun.Applied)
	require.Equal(t, UserAgentGateStateBlocked, dryRun.Old.State, "dry-run must not retire the old identity")

	blocked, oldRecord, err := store.IsBlocked(ctx, domain, oldFingerprint, now.Add(time.Hour))
	require.NoError(t, err)
	require.True(t, blocked)
	require.Equal(t, UserAgentGateStateBlocked, oldRecord.State, "dry-run must not mutate policy state")
	_, newExists, err := store.get(ctx, store.db, domain, newFingerprint)
	require.NoError(t, err)
	require.False(t, newExists, "dry-run must not create the new identity")

	oldAudits, err := store.ListAudits(ctx, domain, oldFingerprint)
	require.NoError(t, err)
	require.Len(t, oldAudits, 1)
	require.Equal(t, UserAgentGateAuditActionMigrateIdentity, oldAudits[0].Action)
	require.Equal(t, UserAgentGateAuditModeDryRun, oldAudits[0].Mode)
	require.Equal(t, "owner", oldAudits[0].Actor)
	newAudits, err := store.ListAudits(ctx, domain, newFingerprint)
	require.NoError(t, err)
	require.Len(t, newAudits, 1)
	require.Equal(t, UserAgentGateAuditActionMigrateIdentity, newAudits[0].Action)
	require.Equal(t, UserAgentGateStateProbePending, newAudits[0].Result)

	applied, err := store.MigrateIdentity(ctx, domain, oldFingerprint, newFingerprint, "owner", now.Add(2*time.Hour), true)
	require.NoError(t, err)
	require.True(t, applied.Eligible)
	require.True(t, applied.Applied)
	require.Equal(t, UserAgentGateStateRetired, applied.Old.State)
	require.Equal(t, UserAgentGateStateProbePending, applied.New.State)
	require.Equal(t, "owner", applied.New.ApprovedBy)
	require.NotNil(t, applied.New.ApprovedAt)

	oldStillBlocked, oldRecord, err := store.IsBlocked(ctx, domain, oldFingerprint, now.Add(2*time.Hour))
	require.NoError(t, err)
	require.True(t, oldStillBlocked, "a retired identity is still blocked")
	require.Equal(t, UserAgentGateStateRetired, oldRecord.State)

	newBlocked, newRecord, err := store.IsBlocked(ctx, domain, newFingerprint, now.Add(2*time.Hour))
	require.NoError(t, err)
	require.True(t, newBlocked, "the new identity starts as probe_pending (non-active)")
	require.Equal(t, UserAgentGateStateProbePending, newRecord.State)

	oldAudits, err = store.ListAudits(ctx, domain, oldFingerprint)
	require.NoError(t, err)
	require.Len(t, oldAudits, 2)
	require.Equal(t, UserAgentGateAuditModeApply, oldAudits[1].Mode)
	require.Equal(t, UserAgentGateStateRetired, oldAudits[1].Result)
	newAudits, err = store.ListAudits(ctx, domain, newFingerprint)
	require.NoError(t, err)
	require.Len(t, newAudits, 2)
	require.Equal(t, UserAgentGateAuditModeApply, newAudits[1].Mode)
}

// TestUserAgentGateRetiredIdentityNeverReEligible proves the old identity is
// permanently retired: a fresh denial cannot re-arm it, and a human approval
// cannot restore it after migration.
func TestUserAgentGateRetiredIdentityNeverReEligible(t *testing.T) {
	store := openUserAgentGateRecoveryStore(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 26, 3, 0, 0, 0, time.UTC)
	domain := "feeds.example"
	oldFingerprint := UserAgentFingerprint("MagicPodcast/1.0 (+https://github.com/rookiestar/MagicPodcast)")
	newFingerprint := UserAgentFingerprint("MagicPodcastFeed/2.0 (+https://github.com/rookiestar/MagicPodcast)")
	require.NoError(t, store.Block(ctx, domain, oldFingerprint, now))

	migrateAt := now.Add(time.Hour)
	applied, err := store.MigrateIdentity(ctx, domain, oldFingerprint, newFingerprint, "owner", migrateAt, true)
	require.NoError(t, err)
	require.True(t, applied.Applied)
	retiredEligibleAt := applied.Old.ProbeEligibleAt

	// A renewed explicit ACL refusal must NOT re-arm the retired identity with a
	// fresh cooldown (which would make it eligible again).
	require.NoError(t, store.Block(ctx, domain, oldFingerprint, now.Add(2*time.Hour)))
	_, record, err := store.IsBlocked(ctx, domain, oldFingerprint, now.Add(2*time.Hour))
	require.NoError(t, err)
	require.Equal(t, UserAgentGateStateRetired, record.State, "Block must preserve the retired state")
	require.Equal(t, retiredEligibleAt, record.ProbeEligibleAt, "Block must not reset the retired cooldown")

	// A human approval must not restore a retired identity either.
	approval, err := store.ApproveProbe(ctx, domain, oldFingerprint, "owner", now.Add(72*time.Hour), true)
	require.NoError(t, err)
	require.False(t, approval.Eligible)
	require.False(t, approval.Applied)
	require.Equal(t, UserAgentGateStateRetired, approval.Record.State)

	// The retired identity admits no probe and no recovery Feed.
	decision, err := store.PreparePrimaryFetchForFeed(ctx, domain, oldFingerprint, UserAgentGateFeedFingerprint("https://feeds.example/one.xml"), now.Add(72*time.Hour))
	require.NoError(t, err)
	require.Equal(t, UserAgentGateFetchModeBlocked, decision.Mode)
}

// TestUserAgentGateUnapprovedNewIdentityCannotBypassGate proves the durable
// domain-level gate still blocks any identity the operator did not explicitly
// migrate to, while the single migrated identity is admitted for one probe.
func TestUserAgentGateUnapprovedNewIdentityCannotBypassGate(t *testing.T) {
	store := openUserAgentGateRecoveryStore(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 26, 3, 0, 0, 0, time.UTC)
	domain := "feeds.example"
	oldFingerprint := UserAgentFingerprint("MagicPodcast/1.0 (+https://github.com/rookiestar/MagicPodcast)")
	unapprovedFingerprint := UserAgentFingerprint("Unapproved/9.9 (+https://example.org/unapproved)")
	approvedFingerprint := UserAgentFingerprint("MagicPodcastFeed/2.0 (+https://github.com/rookiestar/MagicPodcast)")
	require.NoError(t, store.Block(ctx, domain, oldFingerprint, now))

	feedFingerprint := UserAgentGateFeedFingerprint("https://feeds.example/one.xml")

	// An un-migrated, unknown identity cannot bypass the blocked domain.
	decision, err := store.PreparePrimaryFetchForFeed(ctx, domain, unapprovedFingerprint, feedFingerprint, now.Add(time.Hour))
	require.NoError(t, err)
	require.Equal(t, UserAgentGateFetchModeBlocked, decision.Mode)

	applied, err := store.MigrateIdentity(ctx, domain, oldFingerprint, approvedFingerprint, "owner", now.Add(time.Hour), true)
	require.NoError(t, err)
	require.True(t, applied.Applied)

	// The single migrated identity is admitted for exactly one probe.
	approvedDecision, err := store.PreparePrimaryFetchForFeed(ctx, domain, approvedFingerprint, feedFingerprint, now.Add(time.Hour))
	require.NoError(t, err)
	require.Equal(t, UserAgentGateFetchModeProbe, approvedDecision.Mode)

	// The unapproved identity is STILL blocked after the migration.
	stillBlocked, err := store.PreparePrimaryFetchForFeed(ctx, domain, unapprovedFingerprint, UserAgentGateFeedFingerprint("https://feeds.example/two.xml"), now.Add(time.Hour))
	require.NoError(t, err)
	require.Equal(t, UserAgentGateFetchModeBlocked, stillBlocked.Mode)
}

// TestUserAgentGateMigrationRejectsBadInputs covers the precondition guards.
func TestUserAgentGateMigrationRejectsBadInputs(t *testing.T) {
	store := openUserAgentGateRecoveryStore(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 26, 3, 0, 0, 0, time.UTC)
	domain := "feeds.example"
	oldFingerprint := UserAgentFingerprint("MagicPodcast/1.0 (+https://github.com/rookiestar/MagicPodcast)")
	newFingerprint := UserAgentFingerprint("MagicPodcastFeed/2.0 (+https://github.com/rookiestar/MagicPodcast)")
	require.NoError(t, store.Block(ctx, domain, oldFingerprint, now))

	// Same fingerprint for old and new is rejected.
	_, err := store.MigrateIdentity(ctx, domain, oldFingerprint, oldFingerprint, "owner", now.Add(time.Hour), true)
	require.Error(t, err)

	// A missing old identity is rejected.
	missingFingerprint := UserAgentFingerprint("DoesNotExist/0.0 (+https://example.org/missing)")
	_, err = store.MigrateIdentity(ctx, domain, missingFingerprint, newFingerprint, "owner", now.Add(time.Hour), true)
	require.Error(t, err)

	// A new identity that already exists on the domain is not eligible: apply
	// must be a no-op that leaves both identities in their prior states.
	require.NoError(t, store.Block(ctx, domain, newFingerprint, now))
	applied, err := store.MigrateIdentity(ctx, domain, oldFingerprint, newFingerprint, "owner", now.Add(time.Hour), true)
	require.NoError(t, err)
	require.False(t, applied.Eligible)
	require.False(t, applied.Applied)
	require.Equal(t, UserAgentGateStateBlocked, applied.Old.State)
	require.Equal(t, UserAgentGateStateBlocked, applied.New.State)
	newAudits, err := store.ListAudits(ctx, domain, newFingerprint)
	require.NoError(t, err)
	require.Len(t, newAudits, 1)
	require.Equal(t, UserAgentGateStateBlocked, newAudits[0].Result,
		"a no-op migration must audit the existing new state, not probe_pending")

	// A non-blocked old identity (e.g. mid-recovery) is not eligible either.
	recoveringFingerprint := UserAgentFingerprint("RecoveringIdentity/3.0 (+https://example.org/recovering)")
	require.NoError(t, store.Block(ctx, domain, recoveringFingerprint, now))
	approved, err := store.ApproveProbe(ctx, domain, recoveringFingerprint, "owner", now.Add(25*time.Hour), true)
	require.NoError(t, err)
	require.True(t, approved.Applied)
	otherNew := UserAgentFingerprint("BrandNew/4.0 (+https://example.org/brandnew)")
	midRecovery, err := store.MigrateIdentity(ctx, domain, recoveringFingerprint, otherNew, "owner", now.Add(26*time.Hour), true)
	require.NoError(t, err)
	require.False(t, midRecovery.Eligible)
	require.False(t, midRecovery.Applied)
}

// TestUserAgentGateIdentityMigrationRequiresExplicitUserAgentDenial prevents a
// normal 403 from being treated as proof of a User-Agent ACL.
func TestUserAgentGateIdentityMigrationRequiresExplicitUserAgentDenial(t *testing.T) {
	store := openUserAgentGateRecoveryStore(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 26, 3, 0, 0, 0, time.UTC)
	domain := "feeds.example"
	oldFingerprint := UserAgentFingerprint("MagicPodcast/1.0 (+https://github.com/rookiestar/MagicPodcast)")
	newFingerprint := UserAgentFingerprint("MagicPodcastFeed/2.0 (+https://github.com/rookiestar/MagicPodcast)")
	require.NoError(t, store.Block(ctx, domain, oldFingerprint, now))

	approvedAt := now.Add(25 * time.Hour)
	approval, err := store.ApproveProbe(ctx, domain, oldFingerprint, "owner", approvedAt, true)
	require.NoError(t, err)
	require.True(t, approval.Applied)
	feedFingerprint := UserAgentGateFeedFingerprint("https://feeds.example/one.xml")
	decision, err := store.PreparePrimaryFetchForFeed(ctx, domain, oldFingerprint, feedFingerprint, approvedAt)
	require.NoError(t, err)
	require.Equal(t, UserAgentGateFetchModeProbe, decision.Mode)

	failedAt := approvedAt.Add(time.Minute)
	failed, err := store.RecordPrimaryFetchResult(ctx, domain, oldFingerprint, feedFingerprint, AccessOutcome{
		HTTPStatus:    intPointer(http.StatusForbidden),
		ErrorCategory: ErrorCategoryAccessDenied,
	}, failedAt)
	require.NoError(t, err)
	require.Equal(t, UserAgentGateStateBlocked, failed.State)
	require.Equal(t, string(ErrorCategoryAccessDenied), failed.LastProbeResult)

	migrationAt := failedAt.Add(DefaultUserAgentProbeFailureCooldown)
	migration, err := store.MigrateIdentity(ctx, domain, oldFingerprint, newFingerprint, "owner", migrationAt, true)
	require.NoError(t, err)
	require.False(t, migration.Eligible, "only an explicit UA ACL denial may authorize identity migration")
	require.False(t, migration.Applied)
	require.Equal(t, UserAgentGateStateBlocked, migration.Old.State)
	require.Equal(t, string(ErrorCategoryAccessDenied), migration.Old.LastProbeResult)
	require.Empty(t, migration.New.State, "an ineligible candidate must not be projected as admitted")

	_, oldExists, err := store.get(ctx, store.db, domain, oldFingerprint)
	require.NoError(t, err)
	require.True(t, oldExists)
	_, newExists, err := store.get(ctx, store.db, domain, newFingerprint)
	require.NoError(t, err)
	require.False(t, newExists, "an ineligible migration must not create the new identity")

	newAudits, err := store.ListAudits(ctx, domain, newFingerprint)
	require.NoError(t, err)
	require.Len(t, newAudits, 1)
	require.Equal(t, UserAgentGateAuditResultNotEligible, newAudits[0].Result)
}

// TestUserAgentGateMigrationThenRecovery1To2To3 proves that after a migration,
// the new identity recovers through the normal three-distinct-Feeds state
// machine while the old identity stays permanently retired.
func TestUserAgentGateMigrationThenRecovery1To2To3(t *testing.T) {
	store := openUserAgentGateRecoveryStore(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 26, 3, 0, 0, 0, time.UTC)
	domain := "feeds.example"
	oldFingerprint := UserAgentFingerprint("MagicPodcast/1.0 (+https://github.com/rookiestar/MagicPodcast)")
	newFingerprint := UserAgentFingerprint("MagicPodcastFeed/2.0 (+https://github.com/rookiestar/MagicPodcast)")
	require.NoError(t, store.Block(ctx, domain, oldFingerprint, now))

	migrateAt := now.Add(time.Hour)
	applied, err := store.MigrateIdentity(ctx, domain, oldFingerprint, newFingerprint, "owner", migrateAt, true)
	require.NoError(t, err)
	require.True(t, applied.Applied)

	firstFeed := UserAgentGateFeedFingerprint("https://feeds.example/one.xml")
	decision, err := store.PreparePrimaryFetchForFeed(ctx, domain, newFingerprint, firstFeed, migrateAt)
	require.NoError(t, err)
	require.Equal(t, UserAgentGateFetchModeProbe, decision.Mode)
	completed, err := store.RecordPrimaryFetchResult(ctx, domain, newFingerprint, firstFeed, AccessOutcome{
		HTTPStatus:    intPointer(http.StatusOK),
		ErrorCategory: ErrorCategoryNone,
	}, migrateAt.Add(time.Minute))
	require.NoError(t, err)
	require.Equal(t, UserAgentGateStateRecovering, completed.State)
	require.Equal(t, 1, completed.RecoverySuccessCount)

	secondFeed := UserAgentGateFeedFingerprint("https://feeds.example/two.xml")
	decision, err = store.PreparePrimaryFetchForFeed(ctx, domain, newFingerprint, secondFeed, migrateAt.Add(2*time.Minute))
	require.NoError(t, err)
	require.Equal(t, UserAgentGateFetchModeRecovery, decision.Mode)
	completed, err = store.RecordPrimaryFetchResult(ctx, domain, newFingerprint, secondFeed, AccessOutcome{
		HTTPStatus:    intPointer(http.StatusNotModified),
		ErrorCategory: ErrorCategoryNone,
	}, migrateAt.Add(3*time.Minute))
	require.NoError(t, err)
	require.Equal(t, UserAgentGateStateRecovering, completed.State)
	require.Equal(t, 2, completed.RecoverySuccessCount)

	thirdFeed := UserAgentGateFeedFingerprint("https://feeds.example/three.xml")
	decision, err = store.PreparePrimaryFetchForFeed(ctx, domain, newFingerprint, thirdFeed, migrateAt.Add(4*time.Minute))
	require.NoError(t, err)
	require.Equal(t, UserAgentGateFetchModeRecovery, decision.Mode)
	completed, err = store.RecordPrimaryFetchResult(ctx, domain, newFingerprint, thirdFeed, AccessOutcome{
		HTTPStatus:    intPointer(http.StatusOK),
		ErrorCategory: ErrorCategoryNone,
	}, migrateAt.Add(5*time.Minute))
	require.NoError(t, err)
	require.Equal(t, UserAgentGateStateActive, completed.State)
	require.Equal(t, 3, completed.RecoverySuccessCount)

	active, err := store.PreparePrimaryFetchForFeed(ctx, domain, newFingerprint, UserAgentGateFeedFingerprint("https://feeds.example/four.xml"), migrateAt.Add(6*time.Minute))
	require.NoError(t, err)
	require.Equal(t, UserAgentGateFetchModeActive, active.Mode)

	// The old identity remains permanently retired throughout the recovery.
	_, oldRecord, err := store.IsBlocked(ctx, domain, oldFingerprint, migrateAt.Add(6*time.Minute))
	require.NoError(t, err)
	require.Equal(t, UserAgentGateStateRetired, oldRecord.State)
}
