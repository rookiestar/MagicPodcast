package sync

import (
	"time"

	"magicpodcast/internal/feed"
	"magicpodcast/internal/logger"
	"magicpodcast/internal/models"
)

func (s *Service) syncPodcastMetadataWithRetry(podcast *models.Podcast, workerID int) metadataSyncResult {
	var lastErr error
	var noUpdate bool
	retries := 0

	for retries <= DefaultRetryConfig.MaxRetries {
		if retries > 0 {
			logger.Infof("[Worker %d] 重试 %d/%d: %s", workerID, retries, DefaultRetryConfig.MaxRetries, podcast.Title)
			delay := DefaultRetryConfig.InitialDelay * time.Duration(1<<uint(retries-1))
			time.Sleep(delay)
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

		if shouldSkip, _, _ := feed.GetSkipReasonFromError(err); shouldSkip {
			break
		}

		retries++
	}

	return metadataSyncResult{
		podcast:       podcast,
		err:           lastErr,
		retries:       retries - 1,
		noUpdate:      noUpdate,
		episodeResult: nil,
	}
}
