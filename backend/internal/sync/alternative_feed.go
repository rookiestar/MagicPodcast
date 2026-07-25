package sync

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"time"

	"magicpodcast/internal/feed"
	"magicpodcast/internal/logger"
	"magicpodcast/internal/models"
	"magicpodcast/internal/podcastindex"

	"github.com/mmcdole/gofeed"
)

// AlternativeLiveQueryTimeout caps live PodcastIndex / candidate verification
// when no durable cache hit is available (#37). Pre-verified cache reads skip
// this path entirely.
const AlternativeLiveQueryTimeout = 8 * time.Second

// Transient lookup/probe failures must expire before the first scheduled 403
// retry so one short outage cannot disable alternatives for the whole batch.
const AlternativeTransientUnavailableTTL = time.Minute

type alternativeIdentity struct {
	itunesID    int
	podcastGUID string
	title       string
	author      string
}

func (identity alternativeIdentity) key() string {
	return strconv.Itoa(identity.itunesID) + "|" + identity.podcastGUID
}

func (s *Service) persistPodcastFeedIdentity(podcast *models.Podcast, parsed *gofeed.Feed) {
	if s == nil || s.db == nil || podcast == nil || parsed == nil {
		return
	}
	updates := make(map[string]interface{}, 2)
	identityChanged := false
	if podcast.PodcastGUID == "" {
		if guid := extractPodcastGUID(parsed); guid != "" {
			podcast.PodcastGUID = guid
			updates["podcast_guid"] = guid
			identityChanged = true
		}
	}
	if podcast.ITunesID == "" {
		if itunesID := extractITunesID(parsed); itunesID != "" {
			podcast.ITunesID = itunesID
			updates["i_tunes_id"] = itunesID
			identityChanged = true
		}
	}
	if len(updates) > 0 {
		if err := s.db.Model(&models.Podcast{}).Where("id = ?", podcast.ID).Updates(updates).Error; err != nil {
			return
		}
	}
	// Filling a previously empty identity does not invalidate an existing cache
	// keyed on the old empty identity; re-warm happens on the next verify pass.
	_ = identityChanged
}

// InvalidateAlternativeCache drops cached alternative verification for a
// podcast. Call when the main Feed URL or stable identity is replaced by
// import/update so stale alternatives cannot be reused (#37).
func (s *Service) InvalidateAlternativeCache(podcastID uint) {
	if s == nil || s.db == nil || podcastID == 0 {
		return
	}
	if err := s.db.Where("podcast_id = ?", podcastID).Delete(&models.PodcastAlternativeFeed{}).Error; err != nil {
		logger.Warnf("invalidate alternative cache for podcast %d: %v", podcastID, err)
	}
}

// UpdatePodcastMainFeed replaces the subscribed main Feed URL and immediately
// invalidates any alternative cache keyed on the previous main Feed / identity.
// It never writes an alternative URL into podcasts.feed_url.
func (s *Service) UpdatePodcastMainFeed(podcastID uint, newFeedURL string) error {
	if s == nil || s.db == nil {
		return nil
	}
	newFeedURL = strings.TrimSpace(newFeedURL)
	if podcastID == 0 || newFeedURL == "" {
		return nil
	}
	var podcast models.Podcast
	if err := s.db.First(&podcast, podcastID).Error; err != nil {
		return err
	}
	if feed.CanonicalizeURL(podcast.FeedURL) == feed.CanonicalizeURL(newFeedURL) {
		return nil
	}
	if err := s.db.Model(&models.Podcast{}).Where("id = ?", podcastID).Update("feed_url", newFeedURL).Error; err != nil {
		return err
	}
	s.InvalidateAlternativeCache(podcastID)
	return nil
}

// EnsureAlternativeVerified warms the alternative cache for a podcast that has
// a stable identity. Safe to call after a successful primary fetch; failures
// are recorded as unavailable reasons without touching the main Feed URL.
func (s *Service) EnsureAlternativeVerified(ctx context.Context, podcast *models.Podcast) {
	if s == nil || podcast == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	queryCtx, cancel := context.WithTimeout(ctx, AlternativeLiveQueryTimeout)
	defer cancel()
	_, _, _, _ = s.verifyAndCacheAlternative(queryCtx, podcast)
}

// fetchVerifiedAlternative is called only after the subscribed Feed has
// failed. It prefers a durable, identity-keyed cache hit so failure windows
// do not re-scan PodcastIndex. Live verification is strictly time-bounded.
// Successful alternatives never overwrite podcasts.feed_url.
func (s *Service) fetchVerifiedAlternative(
	ctx context.Context,
	podcast *models.Podcast,
	lastFetchTime time.Time,
	incremental bool,
	failure *feed.FetchResult,
) (*feed.FetchResult, bool) {
	if s == nil || podcast == nil {
		setAlternativeVerification(failure, feed.IdentityVerificationUnavailable)
		return nil, false
	}
	if ctx == nil {
		ctx = context.Background()
	}
	queryCtx, cancel := context.WithTimeout(ctx, AlternativeLiveQueryTimeout)
	defer cancel()

	identity, err := s.resolveAlternativeIdentityWithContext(queryCtx, podcast)
	if err != nil {
		reason := "identity_resolve_error"
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			reason = "live_query_timeout"
		}
		s.persistAlternativeUnavailable(podcast, identity, feed.IdentityVerificationUnavailable, reason)
		setAlternativeVerification(failure, feed.IdentityVerificationUnavailable)
		return nil, false
	}
	if identity.itunesID <= 0 && identity.podcastGUID == "" {
		setAlternativeVerification(failure, feed.IdentityVerificationRejectedNoStableID)
		s.persistAlternativeUnavailable(podcast, identity, feed.IdentityVerificationRejectedNoStableID, "no_stable_identity")
		return nil, false
	}

	mainKey := feed.CanonicalizeURL(podcast.FeedURL)
	identityKey := identity.key()

	// --- Cache hit path: use only when main feed + identity still match ---
	if cached, ok := s.loadAlternativeCache(podcast.ID, mainKey, identityKey); ok {
		if cached.Status == models.AlternativeCacheUnavailable {
			setAlternativeVerification(failure, cached.Verification)
			return nil, false
		}
		if cached.Status == models.AlternativeCacheVerified && cached.AlternativeFeedURL != "" {
			return s.fetchCachedAlternative(ctx, podcast, cached, lastFetchTime, incremental, failure)
		}
	}

	// --- Live verify with hard short timeout ---
	candidateURL, verification, reason, ok := s.verifyAndCacheAlternative(queryCtx, podcast)
	if !ok {
		if verification == "" {
			verification = feed.IdentityVerificationUnavailable
		}
		setAlternativeVerification(failure, verification)
		_ = reason
		return nil, false
	}

	return s.fetchAlternativeURL(ctx, podcast, candidateURL, verification, lastFetchTime, incremental, failure)
}

func (s *Service) fetchCachedAlternative(
	ctx context.Context,
	podcast *models.Podcast,
	cached *models.PodcastAlternativeFeed,
	lastFetchTime time.Time,
	incremental bool,
	failure *feed.FetchResult,
) (*feed.FetchResult, bool) {
	return s.fetchAlternativeURL(ctx, podcast, cached.AlternativeFeedURL, cached.Verification, lastFetchTime, incremental, failure)
}

func (s *Service) fetchAlternativeURL(
	ctx context.Context,
	podcast *models.Podcast,
	candidateURL string,
	verification string,
	lastFetchTime time.Time,
	incremental bool,
	failure *feed.FetchResult,
) (*feed.FetchResult, bool) {
	var (
		alternative *feed.FetchResult
		err         error
	)
	if incremental {
		alternative, err = s.feedFetcher.FetchIncrementalWithContextAsSource(ctx, candidateURL, lastFetchTime, feed.AccessSourceAlternative)
	} else {
		alternative, err = s.feedFetcher.FetchFeedWithContextDetailedAsSource(ctx, candidateURL, feed.AccessSourceAlternative)
	}
	if err != nil || alternative == nil || alternative.Feed == nil {
		setAlternativeVerification(failure, feed.IdentityVerificationUnavailable)
		return nil, false
	}

	// If cache said metadata-verified, keep it; otherwise re-check episode/metadata
	// evidence against the live body (defense in depth).
	if verification != feed.IdentityVerificationVerifiedMetadata &&
		verification != feed.IdentityVerificationVerifiedEpisode {
		verifyCtx, cancel := context.WithTimeout(ctx, AlternativeLiveQueryTimeout)
		identity, _ := s.resolveAlternativeIdentityWithContext(verifyCtx, podcast)
		cancel()
		if candidateFeedMetadataEvidence(identity, alternative.Feed) {
			verification = feed.IdentityVerificationVerifiedMetadata
		} else if s.hasEpisodeEvidence(podcast.ID, alternative.Feed) {
			verification = feed.IdentityVerificationVerifiedEpisode
		} else {
			setAlternativeVerification(failure, feed.IdentityVerificationRejectedInsufficientEvidence)
			return nil, false
		}
	}

	alternative.Access.SourceType = feed.AccessSourceAlternative
	alternative.Access.SourceURL = feed.SanitizeFeedURL(candidateURL)
	alternative.Access.IdentityVerification = verification
	// Hard invariant: main Feed URL is never rewritten by alternative success.
	return alternative, true
}

// verifyAndCacheAlternative resolves a single verified candidate and persists
// the outcome. Returns ok=false when no usable alternative exists.
func (s *Service) verifyAndCacheAlternative(ctx context.Context, podcast *models.Podcast) (candidateURL, verification, reason string, ok bool) {
	if s == nil || podcast == nil || s.podcastIndexQuery == nil {
		s.persistAlternativeUnavailable(podcast, alternativeIdentity{}, feed.IdentityVerificationUnavailable, "index_unavailable")
		return "", feed.IdentityVerificationUnavailable, "index_unavailable", false
	}
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		s.persistAlternativeUnavailable(podcast, alternativeIdentity{}, feed.IdentityVerificationUnavailable, "live_query_timeout")
		return "", feed.IdentityVerificationUnavailable, "live_query_timeout", false
	default:
	}

	identity, err := s.resolveAlternativeIdentityWithContext(ctx, podcast)
	if err != nil {
		reason := "identity_resolve_error"
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			reason = "live_query_timeout"
		}
		s.persistAlternativeUnavailable(podcast, identity, feed.IdentityVerificationUnavailable, reason)
		return "", feed.IdentityVerificationUnavailable, reason, false
	}
	if identity.itunesID <= 0 && identity.podcastGUID == "" {
		s.persistAlternativeUnavailable(podcast, identity, feed.IdentityVerificationRejectedNoStableID, "no_stable_identity")
		return "", feed.IdentityVerificationRejectedNoStableID, "no_stable_identity", false
	}

	candidates, err := s.podcastIndexQuery.FindCandidatesByIdentityContext(ctx, identity.itunesID, identity.podcastGUID)
	if err != nil {
		reason := "candidate_query_error"
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			reason = "live_query_timeout"
		}
		s.persistAlternativeUnavailable(podcast, identity, feed.IdentityVerificationUnavailable, reason)
		return "", feed.IdentityVerificationUnavailable, reason, false
	}
	candidates = filterAlternativeCandidates(candidates, podcast.FeedURL)
	if len(candidates) == 0 {
		s.persistAlternativeUnavailable(podcast, identity, feed.IdentityVerificationUnavailable, "no_candidates")
		return "", feed.IdentityVerificationUnavailable, "no_candidates", false
	}

	unique := make([]*podcastindex.PodcastInfo, 0, len(candidates))
	seenURLs := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		if !candidateIdentityCompatible(candidate, identity) {
			s.persistAlternativeUnavailable(podcast, identity, feed.IdentityVerificationRejectedConflict, "identity_conflict")
			return "", feed.IdentityVerificationRejectedConflict, "identity_conflict", false
		}
		key := feed.CanonicalizeURL(candidate.FeedURL)
		if _, exists := seenURLs[key]; exists {
			continue
		}
		seenURLs[key] = struct{}{}
		unique = append(unique, candidate)
	}
	if len(unique) != 1 {
		s.persistAlternativeUnavailable(podcast, identity, feed.IdentityVerificationRejectedAmbiguous, "ambiguous_candidates")
		return "", feed.IdentityVerificationRejectedAmbiguous, "ambiguous_candidates", false
	}
	candidate := unique[0]

	verification = feed.IdentityVerificationRejectedInsufficientEvidence
	if candidateMetadataEvidence(identity, candidate) {
		verification = feed.IdentityVerificationVerifiedMetadata
	} else {
		// Without metadata evidence, attempt a short live body check for
		// episode/title evidence. This still respects the caller's deadline.
		probe, err := s.feedFetcher.FetchFeedWithContextDetailedAsSource(ctx, candidate.FeedURL, feed.AccessSourceAlternative)
		if err != nil || probe == nil || probe.Feed == nil {
			s.persistAlternativeUnavailable(podcast, identity, feed.IdentityVerificationUnavailable, "probe_failed")
			return "", feed.IdentityVerificationUnavailable, "probe_failed", false
		}
		if candidateFeedMetadataEvidence(identity, probe.Feed) {
			verification = feed.IdentityVerificationVerifiedMetadata
		} else if s.hasEpisodeEvidence(podcast.ID, probe.Feed) {
			verification = feed.IdentityVerificationVerifiedEpisode
		} else {
			s.persistAlternativeUnavailable(podcast, identity, feed.IdentityVerificationRejectedInsufficientEvidence, "insufficient_evidence")
			return "", feed.IdentityVerificationRejectedInsufficientEvidence, "insufficient_evidence", false
		}
	}

	s.persistAlternativeVerified(podcast, identity, candidate.FeedURL, verification)
	return candidate.FeedURL, verification, "", true
}

func (s *Service) loadAlternativeCache(podcastID uint, mainFeedURL, identityKey string) (*models.PodcastAlternativeFeed, bool) {
	if s == nil || s.db == nil || podcastID == 0 {
		return nil, false
	}
	if !s.db.Migrator().HasTable(&models.PodcastAlternativeFeed{}) {
		return nil, false
	}
	var row models.PodcastAlternativeFeed
	err := s.db.Where("podcast_id = ?", podcastID).First(&row).Error
	if err != nil {
		return nil, false
	}
	if feed.CanonicalizeURL(row.MainFeedURL) != feed.CanonicalizeURL(mainFeedURL) {
		// Stale relative to current main Feed — drop and re-verify.
		s.InvalidateAlternativeCache(podcastID)
		return nil, false
	}
	if row.IdentityKey != identityKey {
		s.InvalidateAlternativeCache(podcastID)
		return nil, false
	}
	if row.Status == models.AlternativeCacheUnavailable &&
		isTransientAlternativeUnavailable(row.UnavailableReason) &&
		time.Since(row.VerifiedAt) >= AlternativeTransientUnavailableTTL {
		s.InvalidateAlternativeCache(podcastID)
		return nil, false
	}
	return &row, true
}

func isTransientAlternativeUnavailable(reason string) bool {
	switch reason {
	case "index_unavailable", "live_query_timeout", "identity_resolve_error",
		"candidate_query_error", "probe_failed":
		return true
	default:
		return false
	}
}

func (s *Service) persistAlternativeVerified(podcast *models.Podcast, identity alternativeIdentity, altURL, verification string) {
	if s == nil || s.db == nil || podcast == nil || podcast.ID == 0 {
		return
	}
	if !s.db.Migrator().HasTable(&models.PodcastAlternativeFeed{}) {
		// Tests using AutoMigrate on core models get the table; production uses
		// migration v8. Without the table, verification still works live-only.
		return
	}
	now := time.Now()
	row := models.PodcastAlternativeFeed{
		PodcastID:          podcast.ID,
		MainFeedURL:        feed.CanonicalizeURL(podcast.FeedURL),
		IdentityKey:        identity.key(),
		AlternativeFeedURL: altURL,
		Status:             models.AlternativeCacheVerified,
		Verification:       verification,
		UnavailableReason:  "",
		VerifiedAt:         now,
	}
	var existing models.PodcastAlternativeFeed
	if err := s.db.Where("podcast_id = ?", podcast.ID).First(&existing).Error; err == nil {
		row.ID = existing.ID
		row.CreatedAt = existing.CreatedAt
		_ = s.db.Save(&row).Error
		return
	}
	_ = s.db.Create(&row).Error
}

func (s *Service) persistAlternativeUnavailable(podcast *models.Podcast, identity alternativeIdentity, verification, reason string) {
	if s == nil || s.db == nil || podcast == nil || podcast.ID == 0 {
		return
	}
	if !s.db.Migrator().HasTable(&models.PodcastAlternativeFeed{}) {
		return
	}
	now := time.Now()
	row := models.PodcastAlternativeFeed{
		PodcastID:          podcast.ID,
		MainFeedURL:        feed.CanonicalizeURL(podcast.FeedURL),
		IdentityKey:        identity.key(),
		AlternativeFeedURL: "",
		Status:             models.AlternativeCacheUnavailable,
		Verification:       verification,
		UnavailableReason:  reason,
		VerifiedAt:         now,
	}
	var existing models.PodcastAlternativeFeed
	if err := s.db.Where("podcast_id = ?", podcast.ID).First(&existing).Error; err == nil {
		row.ID = existing.ID
		row.CreatedAt = existing.CreatedAt
		_ = s.db.Save(&row).Error
		return
	}
	_ = s.db.Create(&row).Error
}

func setAlternativeVerification(result *feed.FetchResult, verification string) {
	if result != nil {
		result.Access.IdentityVerification = verification
	}
}

func (s *Service) resolveAlternativeIdentity(podcast *models.Podcast) (alternativeIdentity, error) {
	return s.resolveAlternativeIdentityWithContext(context.Background(), podcast)
}

func (s *Service) resolveAlternativeIdentityWithContext(ctx context.Context, podcast *models.Podcast) (alternativeIdentity, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	identity := alternativeIdentity{
		itunesID:    parseITunesID(podcast.ITunesID),
		podcastGUID: normalizeIdentity(podcast.PodcastGUID),
		title:       strings.TrimSpace(podcast.Title),
		author:      strings.TrimSpace(podcast.Author),
	}
	if s.podcastIndexQuery == nil {
		return identity, nil
	}
	if identity.itunesID > 0 || identity.podcastGUID != "" {
		// A stable identity already persisted on the subscription is enough to
		// query candidates. Avoid scanning the large PodcastIndex URL view just
		// to rediscover the same identity during a failure window.
		return identity, nil
	}

	// Existing subscriptions may predate PodcastGUID/iTunesID persistence. A
	// URL lookup supplies those fields without weakening the stable-ID gate.
	primary, err := s.podcastIndexQuery.FindByFeedURLContext(ctx, podcast.FeedURL)
	if err != nil || primary == nil {
		return identity, err
	}
	if identity.itunesID <= 0 {
		identity.itunesID = primary.ITunesID
	}
	if identity.podcastGUID == "" {
		identity.podcastGUID = normalizeIdentity(primary.PodcastGUID)
	}
	if identity.title == "" {
		identity.title = strings.TrimSpace(primary.Title)
	}
	if identity.author == "" {
		identity.author = strings.TrimSpace(primary.Author)
	}
	return identity, nil
}

func parseITunesID(raw string) int {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value <= 0 {
		return 0
	}
	return value
}

func normalizeIdentity(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func filterAlternativeCandidates(candidates []*podcastindex.PodcastInfo, primaryURL string) []*podcastindex.PodcastInfo {
	filtered := make([]*podcastindex.PodcastInfo, 0, len(candidates))
	primary := feed.CanonicalizeURL(primaryURL)
	for _, candidate := range candidates {
		if candidate == nil || feed.CanonicalizeURL(candidate.FeedURL) == primary {
			continue
		}
		parsed, err := url.Parse(candidate.FeedURL)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" {
			continue
		}
		if strings.EqualFold(parsed.Hostname(), feed.XiaoyuzhouFeedDomain) {
			continue
		}
		filtered = append(filtered, candidate)
	}
	return filtered
}

func candidateIdentityCompatible(candidate *podcastindex.PodcastInfo, identity alternativeIdentity) bool {
	if candidate == nil {
		return false
	}
	itunesMatch := identity.itunesID > 0 && candidate.ITunesID == identity.itunesID
	guidMatch := identity.podcastGUID != "" && normalizeIdentity(candidate.PodcastGUID) == identity.podcastGUID
	if !itunesMatch && !guidMatch {
		return false
	}
	// When both stable identifiers are available, a candidate matching only
	// one while contradicting the other is a conflict, not a safe fallback.
	if identity.itunesID > 0 && candidate.ITunesID > 0 && candidate.ITunesID != identity.itunesID {
		return false
	}
	if identity.podcastGUID != "" && candidate.PodcastGUID != "" && normalizeIdentity(candidate.PodcastGUID) != identity.podcastGUID {
		return false
	}
	return true
}

func candidateMetadataEvidence(identity alternativeIdentity, candidate *podcastindex.PodcastInfo) bool {
	if candidate == nil {
		return false
	}
	return equalIdentityText(identity.title, candidate.Title) || equalIdentityText(identity.author, candidate.Author)
}

func candidateFeedMetadataEvidence(identity alternativeIdentity, candidate *gofeed.Feed) bool {
	if candidate == nil {
		return false
	}
	author := ""
	if candidate.Author != nil {
		author = candidate.Author.Name
	}
	if candidate.ITunesExt != nil && candidate.ITunesExt.Author != "" {
		author = candidate.ITunesExt.Author
	}
	return equalIdentityText(identity.title, candidate.Title) || equalIdentityText(identity.author, author)
}

func equalIdentityText(left, right string) bool {
	return normalizeIdentity(left) != "" && normalizeIdentity(left) == normalizeIdentity(right)
}

func (s *Service) hasEpisodeEvidence(podcastID uint, candidate *gofeed.Feed) bool {
	if s == nil || s.db == nil || candidate == nil || len(candidate.Items) == 0 {
		return false
	}
	var episodes []models.Episode
	if err := s.db.Select("guid, title").Where("podcast_id = ?", podcastID).Find(&episodes).Error; err != nil {
		return false
	}
	if len(episodes) == 0 {
		return false
	}
	knownGUIDs := make(map[string]struct{}, len(episodes))
	knownTitles := make(map[string]struct{}, len(episodes))
	for _, episode := range episodes {
		if episode.GUID != "" {
			knownGUIDs[normalizeIdentity(episode.GUID)] = struct{}{}
		}
		if episode.Title != "" {
			knownTitles[normalizeIdentity(episode.Title)] = struct{}{}
		}
	}
	for _, item := range candidate.Items {
		if item == nil {
			continue
		}
		if _, ok := knownGUIDs[normalizeIdentity(item.GUID)]; item.GUID != "" && ok {
			return true
		}
		if _, ok := knownTitles[normalizeIdentity(item.Title)]; item.Title != "" && ok {
			return true
		}
	}
	return false
}
