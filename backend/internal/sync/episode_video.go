package sync

import (
	"context"
	"time"

	"magicpodcast/internal/logger"
	"magicpodcast/internal/models"
	"magicpodcast/internal/xyzvideo"
)

const maxVideoProbesPerPodcast = 5

var maxVideoProbeBatchDuration = 15 * time.Second

type videoProbeCandidate struct {
	rowID     uint
	episodeID string
}

func enqueueVideoProbe(candidates []videoProbeCandidate, episode models.Episode, isNew, identityChanged bool) []videoProbeCandidate {
	if !xyzvideo.ShouldProbe(episode.Link, episode.VideoAvailability, identityChanged, isNew) {
		return candidates
	}
	episodeID, ok := xyzvideo.ParseEpisodeID(episode.Link)
	if !ok || episode.ID == 0 {
		return candidates
	}
	return append(candidates, videoProbeCandidate{rowID: episode.ID, episodeID: episodeID})
}

func (s *Service) probeEpisodeVideoAvailability(ctx context.Context, candidates []videoProbeCandidate) {
	if s == nil || s.videoProber == nil || len(candidates) == 0 {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	probeCtx, cancel := context.WithTimeout(ctx, maxVideoProbeBatchDuration)
	defer cancel()

	probed := 0
	for _, candidate := range candidates {
		if probed >= maxVideoProbesPerPodcast {
			break
		}
		if probeCtx.Err() != nil {
			return
		}
		if candidate.rowID == 0 || candidate.episodeID == "" {
			continue
		}
		probed++
		outcome := s.videoProber.Probe(probeCtx, candidate.episodeID)
		if outcome.Availability == models.VideoAvailabilityAvailable || outcome.Availability == models.VideoAvailabilityUnavailable {
			if err := s.db.Model(&models.Episode{}).
				Where("id = ?", candidate.rowID).
				Update("video_availability", outcome.Availability).Error; err != nil {
				logger.Infof("   ⚠️ 写入单集视频三态失败: %v", err)
			}
		}
		if outcome.HaltBatch {
			return
		}
	}
}
