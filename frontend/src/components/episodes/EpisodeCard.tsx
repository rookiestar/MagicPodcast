"use client";

import { IconPlayerPlay } from "@tabler/icons-react";
import { memo, type FocusEvent } from "react";
import { OriginalEpisodeRecovery } from "@/components/common/OriginalEpisodeRecovery";
import { EpisodeShowNotes } from "@/components/episodes/EpisodeShowNotes";
import { EpisodeThumbnail } from "@/components/episodes/EpisodeThumbnail";
import type { OriginalEpisodeRecoveryController } from "@/hooks/useOriginalEpisodeRecovery";
import { useEpisodeShowNotes } from "@/hooks/useEpisodeShowNotes";
import {
  formatEpisodeDuration,
  formatEpisodeFileSize,
  formatEpisodeNumber,
  planEpisodeVideoAction,
  shouldShowEpisodePlayButton,
  shouldShowEpisodeShowNotes,
  shouldShowEpisodeTitleLink,
  type EpisodeImagePriority,
} from "@/lib/episodeDisplay";
import { planSafeOriginalEpisodeOpen } from "@/lib/originalEpisodeOpen";
import type { EpisodeShowNotesStore } from "@/lib/episodeShowNotesStore";
import { formatDate } from "@/lib/timeUtils";
import type { Episode } from "@/types";

interface EpisodeCardProps {
  episode: Episode;
  podcastCover?: string;
  index?: number;
  priority?: EpisodeImagePriority;
  originalRecovery: OriginalEpisodeRecoveryController;
  showNotesStore: EpisodeShowNotesStore;
}

function EpisodeCard({
  episode,
  podcastCover,
  index = 0,
  priority = "medium",
  originalRecovery,
  showNotesStore,
}: EpisodeCardProps) {
  const durationLabel = formatEpisodeDuration(episode.duration);
  const fileSizeLabel = formatEpisodeFileSize(episode.enclosure_length);
  const episodeNumberLabel = formatEpisodeNumber(episode.episode_no);
  const originalPlan = planSafeOriginalEpisodeOpen(episode.link);
  const showTitleLink = shouldShowEpisodeTitleLink(originalPlan?.openUrl);
  const showPlayButton = shouldShowEpisodePlayButton(episode.medium_url);
  const videoAction = planEpisodeVideoAction(
    episode.video_availability,
    originalPlan?.openUrl,
  );
  const showNotes = shouldShowEpisodeShowNotes(
    episode.show_notes,
    episode.has_show_notes,
  );
  const showNotesState = useEpisodeShowNotes(
    episode.id,
    showNotes,
    showNotesStore,
  );
  const handleOriginalOpen = () => {
    if (originalPlan) {
      originalRecovery.activate(episode.id, originalPlan);
    }
  };

  const handleBlur = (event: FocusEvent<HTMLDivElement>) => {
    const nextFocusedElement = event.relatedTarget as Node | null;
    if (
      !nextFocusedElement ||
      !event.currentTarget.contains(nextFocusedElement)
    ) {
      showNotesState.leaveFocus();
    }
  };

  return (
    <div
      className="podcast-episode-card"
      onMouseEnter={showNotesState.enterHover}
      onMouseLeave={showNotesState.leaveHover}
      onFocus={showNotesState.enterFocus}
      onBlur={handleBlur}
    >
      {/* Content */}
      <div className="podcast-episode-card-inner">
        {/* Title with Thumbnail */}
        <div className="flex items-start gap-2 sm:gap-3 mb-3">
          <EpisodeThumbnail
            episode={episode}
            podcastCover={podcastCover}
            index={index}
            priority={priority}
          />

          {/* Title, Meta Info and Play Button */}
          <div className="flex-1 min-w-0">
            {/* Title with Play Button */}
            <div className="podcast-episode-card-heading">
              {showTitleLink && originalPlan ? (
                <a
                  href={originalPlan.openUrl}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="podcast-episode-title line-clamp-2"
                  onClick={handleOriginalOpen}
                >
                  {episode.title}
                </a>
              ) : (
                <span className="podcast-episode-title line-clamp-2">
                  {episode.title}
                </span>
              )}

              {(showPlayButton || videoAction.show) && (
                <div className="podcast-episode-actions">
                  {showPlayButton && (
                    <button
                      type="button"
                      onClick={(event) => {
                        event.stopPropagation();
                        window.open(episode.medium_url, "_blank");
                      }}
                      className="podcast-episode-play"
                      aria-label="播放"
                    >
                      <IconPlayerPlay aria-hidden="true" stroke={1.8} />
                    </button>
                  )}
                  {videoAction.show && (
                    <a
                      href={videoAction.href}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="podcast-episode-video"
                      aria-label="看视频"
                      onClick={handleOriginalOpen}
                    >
                      看视频
                    </a>
                  )}
                </div>
              )}
            </div>

            {/* Meta Info */}
            <div className="podcast-episode-meta">
              {episodeNumberLabel && (
                <span className="podcast-episode-number">
                  {episodeNumberLabel}
                </span>
              )}
              <span>{formatDate(episode.published_date)}</span>
              {durationLabel && (
                <>
                  <span aria-hidden="true">/</span>
                  <span>{durationLabel}</span>
                </>
              )}
              {fileSizeLabel && (
                <>
                  <span aria-hidden="true">/</span>
                  <span>{fileSizeLabel}</span>
                </>
              )}
            </div>
          </div>
        </div>

        {/* Show Notes */}
        {showNotes && (
          <EpisodeShowNotes
            summary={episode.show_notes}
            link={originalPlan?.openUrl ?? ""}
            isExpanded={showNotesState.isExpanded}
            status={showNotesState.status}
            document={showNotesState.document}
            onRetry={() => void showNotesState.retry()}
            onOriginalOpen={handleOriginalOpen}
          />
        )}
        {originalRecovery.plan &&
          originalRecovery.activeKey === episode.id && (
            <OriginalEpisodeRecovery
              copyError={originalRecovery.copyError}
              onRetry={originalRecovery.retry}
              onOpenApp={originalRecovery.openApp}
              onCopy={() => void originalRecovery.copy()}
              onDismiss={originalRecovery.dismiss}
            />
          )}
      </div>
    </div>
  );
}

// 自定义比较函数：只在关键 props 变化时才重新渲染
function arePropsEqual(
  prevProps: Readonly<EpisodeCardProps>,
  nextProps: Readonly<EpisodeCardProps>,
) {
  const wasRecoveryActive =
    prevProps.originalRecovery.activeKey === prevProps.episode.id;
  const isRecoveryActive =
    nextProps.originalRecovery.activeKey === nextProps.episode.id;

  return (
    prevProps.episode.id === nextProps.episode.id &&
    prevProps.episode.title === nextProps.episode.title &&
    prevProps.episode.image_url === nextProps.episode.image_url &&
    prevProps.episode.medium_url === nextProps.episode.medium_url &&
    prevProps.episode.link === nextProps.episode.link &&
    prevProps.episode.episode_no === nextProps.episode.episode_no &&
    prevProps.episode.published_date === nextProps.episode.published_date &&
    prevProps.episode.duration === nextProps.episode.duration &&
    prevProps.episode.enclosure_length === nextProps.episode.enclosure_length &&
    prevProps.episode.show_notes === nextProps.episode.show_notes &&
    prevProps.episode.has_show_notes === nextProps.episode.has_show_notes &&
    prevProps.episode.video_availability ===
      nextProps.episode.video_availability &&
    prevProps.podcastCover === nextProps.podcastCover &&
    prevProps.index === nextProps.index &&
    prevProps.priority === nextProps.priority &&
    prevProps.showNotesStore === nextProps.showNotesStore &&
    wasRecoveryActive === isRecoveryActive &&
    (!isRecoveryActive ||
      (prevProps.originalRecovery.plan ===
        nextProps.originalRecovery.plan &&
        prevProps.originalRecovery.copyError ===
          nextProps.originalRecovery.copyError))
  );
}

// 使用 React.memo 包装组件
export default memo(EpisodeCard, arePropsEqual);

// 添加 displayName 用于调试
EpisodeCard.displayName = "EpisodeCard";
