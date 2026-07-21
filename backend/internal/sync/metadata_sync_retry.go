package sync

import (
	"magicpodcast/internal/feed"
	"magicpodcast/internal/logger"
	"magicpodcast/internal/models"
)

// syncPodcastMetadataWithRetry runs the metadata sync for one podcast under the
// unified outer-retry policy. It is the SINGLE retry behavior for this path:
// classification (retryable vs not), Retry-After, and bounded full-jitter
// backoff all come from feed.RetryPolicy, and every retry re-enters through the
// Fetcher/Coordinator so circuit, per-domain concurrency, dedup, and fallback
// semantics are never bypassed.
//
// Non-retryable errors (403/401/404/402/parse/redirect-policy) stop at once;
// only network/timeout/429/5xx retry, and only within the finite Budget. There
// is no infinite-retry path and no path that re-hits a circuit-open upstream
// for an access-denied source.
func (s *Service) syncPodcastMetadataWithRetry(podcast *models.Podcast, workerID int) metadataSyncResult {
	policy := s.retryPolicy
	var lastErr error
	var noUpdate bool
	retries := 0

	for attempt := 0; attempt <= policy.Budget; attempt++ {
		if attempt > 0 {
			delay, _ := policy.NextDelay(lastErr, attempt-1)
			category := feed.CategoryOf(lastErr)
			retryAfter := feed.RetryAfterOf(lastErr)
			logger.Infof("[Worker %d] feed 重试 %d/%d: title=%q category=%s retry_after=%q delay_ms=%d",
				workerID, attempt, policy.Budget, podcast.Title, category, retryAfter, delay.Milliseconds())
			policy.Sleep(delay)
			retries++
		}

		err, noUpdateResult, episodeResult := s.syncPodcastMetadataWithUpdateCheck(podcast)
		if err == nil {
			return metadataSyncResult{
				podcast:       podcast,
				err:           nil,
				retries:       retries,
				noUpdate:      noUpdateResult,
				episodeResult: episodeResult,
			}
		}

		lastErr = err
		noUpdate = noUpdateResult

		// Single classification: stop immediately on non-retryable errors so a
		// 403/404/parse failure never burns the retry budget or re-hits an
		// upstream that the Coordinator may have just opened a circuit for.
		if !policy.ShouldRetry(err) {
			break
		}
	}

	return metadataSyncResult{
		podcast:       podcast,
		err:           lastErr,
		retries:       retries,
		noUpdate:      noUpdate,
		episodeResult: nil,
	}
}
