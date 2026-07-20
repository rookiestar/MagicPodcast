package sync

import (
	"context"
	"net/url"
	"strconv"
	"strings"
	"time"

	"magicpodcast/internal/feed"
	"magicpodcast/internal/models"
	"magicpodcast/internal/podcastindex"

	"github.com/mmcdole/gofeed"
)

type alternativeIdentity struct {
	itunesID    int
	podcastGUID string
	title       string
	author      string
}

func (s *Service) persistPodcastFeedIdentity(podcast *models.Podcast, parsed *gofeed.Feed) {
	if s == nil || s.db == nil || podcast == nil || parsed == nil {
		return
	}
	updates := make(map[string]interface{}, 2)
	if podcast.PodcastGUID == "" {
		if guid := extractPodcastGUID(parsed); guid != "" {
			podcast.PodcastGUID = guid
			updates["podcast_guid"] = guid
		}
	}
	if podcast.ITunesID == "" {
		if itunesID := extractITunesID(parsed); itunesID != "" {
			podcast.ITunesID = itunesID
			updates["i_tunes_id"] = itunesID
		}
	}
	if len(updates) > 0 {
		if err := s.db.Model(&models.Podcast{}).Where("id = ?", podcast.ID).Updates(updates).Error; err != nil {
			return
		}
	}
}

// fetchVerifiedAlternative is called only after the subscribed Feed has
// failed. It never searches by title alone: a stable PodcastIndex identity is
// required before a candidate can even be considered.
func (s *Service) fetchVerifiedAlternative(
	ctx context.Context,
	podcast *models.Podcast,
	lastFetchTime time.Time,
	incremental bool,
	failure *feed.FetchResult,
) (*feed.FetchResult, bool) {
	if s == nil || podcast == nil || s.podcastIndexQuery == nil {
		setAlternativeVerification(failure, feed.IdentityVerificationUnavailable)
		return nil, false
	}

	identity, err := s.resolveAlternativeIdentity(podcast)
	if err != nil {
		setAlternativeVerification(failure, feed.IdentityVerificationUnavailable)
		return nil, false
	}
	if identity.itunesID <= 0 && identity.podcastGUID == "" {
		setAlternativeVerification(failure, feed.IdentityVerificationRejectedNoStableID)
		return nil, false
	}

	candidates, err := s.podcastIndexQuery.FindCandidatesByIdentity(identity.itunesID, identity.podcastGUID)
	if err != nil {
		setAlternativeVerification(failure, feed.IdentityVerificationUnavailable)
		return nil, false
	}
	candidates = filterAlternativeCandidates(candidates, podcast.FeedURL)
	if len(candidates) == 0 {
		setAlternativeVerification(failure, feed.IdentityVerificationUnavailable)
		return nil, false
	}

	unique := make([]*podcastindex.PodcastInfo, 0, len(candidates))
	seenURLs := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		if !candidateIdentityCompatible(candidate, identity) {
			setAlternativeVerification(failure, feed.IdentityVerificationRejectedConflict)
			return nil, false
		}
		key := feed.CanonicalizeURL(candidate.FeedURL)
		if _, exists := seenURLs[key]; exists {
			continue
		}
		seenURLs[key] = struct{}{}
		unique = append(unique, candidate)
	}
	if len(unique) != 1 {
		setAlternativeVerification(failure, feed.IdentityVerificationRejectedAmbiguous)
		return nil, false
	}
	candidate := unique[0]

	verification := feed.IdentityVerificationRejectedInsufficientEvidence
	if candidateMetadataEvidence(identity, candidate) {
		verification = feed.IdentityVerificationVerifiedMetadata
	}

	var alternative *feed.FetchResult
	if incremental {
		alternative, err = s.feedFetcher.FetchIncrementalWithContext(ctx, candidate.FeedURL, lastFetchTime)
	} else {
		alternative, err = s.feedFetcher.FetchFeedWithContextDetailed(ctx, candidate.FeedURL)
	}
	if err != nil || alternative == nil || alternative.Feed == nil {
		setAlternativeVerification(failure, feed.IdentityVerificationUnavailable)
		return nil, false
	}

	if verification != feed.IdentityVerificationVerifiedMetadata {
		if candidateFeedMetadataEvidence(identity, alternative.Feed) {
			verification = feed.IdentityVerificationVerifiedMetadata
		} else if s.hasEpisodeEvidence(podcast.ID, alternative.Feed) {
			verification = feed.IdentityVerificationVerifiedEpisode
		} else {
			setAlternativeVerification(failure, feed.IdentityVerificationRejectedInsufficientEvidence)
			return nil, false
		}
	}

	// FeedFetcher records the network outcome; this final layer records which
	// verified source was actually selected and keeps the URL safe for history.
	alternative.Access.SourceType = feed.AccessSourceAlternative
	alternative.Access.SourceURL = feed.SanitizeFeedURL(candidate.FeedURL)
	alternative.Access.IdentityVerification = verification
	return alternative, true
}

func setAlternativeVerification(result *feed.FetchResult, verification string) {
	if result != nil {
		result.Access.IdentityVerification = verification
	}
}

func (s *Service) resolveAlternativeIdentity(podcast *models.Podcast) (alternativeIdentity, error) {
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
	primary, err := s.podcastIndexQuery.FindByFeedURL(podcast.FeedURL)
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
