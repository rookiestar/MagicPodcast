package sync

import (
	"fmt"
	"time"

	"magicpodcast/internal/feed"
	"magicpodcast/internal/logger"
)

type metadataSyncStats struct {
	successCount         int
	failedCount          int
	skippedCount         int
	noUpdateCount        int
	totalEpisodesCreated int
	totalEpisodesUpdated int
	totalEpisodesSkipped int
	totalEpisodesErrors  int
}

func (stats *metadataSyncStats) record(result metadataSyncResult, current, total int, reporter ProgressReporter) {
	podcast := result.podcast

	if result.err != nil {
		if shouldSkip, reasonStr, _ := feed.GetSkipReasonFromError(result.err); shouldSkip {
			stats.skippedCount++
			reason := SkipReason(reasonStr)
			reporter.ReportSkip(reason, fmt.Sprintf("[%d/%d] %s - %s", current, total, podcast.Title, result.err.Error()))
			return
		}

		stats.failedCount++
		retryMsg := ""
		if result.retries > 0 {
			retryMsg = fmt.Sprintf(" (重试%d次后失败)", result.retries)
		}
		reporter.ReportError(fmt.Sprintf("[%d/%d] %s - %s%s", current, total, podcast.Title, result.err.Error(), retryMsg))
		return
	}

	if result.noUpdate {
		stats.noUpdateCount++
		msg := fmt.Sprintf("[%d/%d] %s - 无内容更新", current, total, podcast.Title)
		if result.episodeResult != nil && (result.episodeResult.Created > 0 || result.episodeResult.Updated > 0) {
			msg += fmt.Sprintf(" (单集: +%d, ~%d)", result.episodeResult.Created, result.episodeResult.Updated)
		}
		reporter.ReportSkip(SkipReasonNoUpdate, msg)
		stats.addEpisodeResult(result.episodeResult)
		return
	}

	stats.successCount++
	msg := fmt.Sprintf("[%d/%d] 成功同步: %s", current, total, podcast.Title)
	if result.episodeResult != nil && (result.episodeResult.Created > 0 || result.episodeResult.Updated > 0) {
		msg += fmt.Sprintf(" (单集: +%d, ~%d)", result.episodeResult.Created, result.episodeResult.Updated)
	}
	reporter.ReportSuccess(msg)
	stats.addEpisodeResult(result.episodeResult)
}

func (stats *metadataSyncStats) addEpisodeResult(result *EpisodeSyncResult) {
	if result == nil {
		return
	}

	stats.totalEpisodesCreated += result.Created
	stats.totalEpisodesUpdated += result.Updated
	stats.totalEpisodesSkipped += result.Skipped
	stats.totalEpisodesErrors += result.Errors
}

func (stats metadataSyncStats) reportProgress(current, total int, reporter ProgressReporter) {
	reporter.ReportProgress(
		current,
		total,
		fmt.Sprintf(
			"已处理 %d/%d (成功: %d, 跳过: %d, 无更新: %d)",
			current,
			total,
			stats.successCount,
			stats.skippedCount,
			stats.noUpdateCount,
		),
	)
}

func (stats metadataSyncStats) summary(total int, duration time.Duration) *SyncSummary {
	return &SyncSummary{
		Operation:        "sync",
		TotalPodcasts:    total,
		SuccessPodcasts:  stats.successCount,
		FailedPodcasts:   stats.failedCount,
		SkippedPodcasts:  stats.skippedCount,
		NoUpdatePodcasts: stats.noUpdateCount,
		TotalEpisodes: stats.totalEpisodesCreated +
			stats.totalEpisodesUpdated +
			stats.totalEpisodesSkipped +
			stats.totalEpisodesErrors,
		NewEpisodes:     stats.totalEpisodesCreated,
		UpdatedEpisodes: stats.totalEpisodesUpdated,
		Duration:        duration,
	}
}

func (stats metadataSyncStats) logSummary(duration time.Duration) {
	logger.Infof("✅ 元数据同步完成: 成功=%d, 失败=%d, 跳过=%d, 无更新=%d, 耗时=%s",
		stats.successCount, stats.failedCount, stats.skippedCount, stats.noUpdateCount, formatDuration(duration))
	logger.Infof("📝 单集统计: 新增=%d, 更新=%d, 跳过=%d, 错误=%d",
		stats.totalEpisodesCreated, stats.totalEpisodesUpdated, stats.totalEpisodesSkipped, stats.totalEpisodesErrors)
}
