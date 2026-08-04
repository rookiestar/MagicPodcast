"use client";

import { IconPlayerPlay } from "@tabler/icons-react";
import { memo, useState, type FocusEvent } from "react";
import type { Episode } from "@/types";
import { EpisodeShowNotes } from "@/components/episodes/EpisodeShowNotes";
import { EpisodeThumbnail } from "@/components/episodes/EpisodeThumbnail";
import {
  formatEpisodeDuration,
  formatEpisodeFileSize,
  shouldShowEpisodePlayButton,
  shouldShowEpisodeShowNotes,
  shouldShowEpisodeTitleLink,
  type EpisodeImagePriority,
} from "@/lib/episodeDisplay";
import { formatDate } from "@/lib/timeUtils";

interface EpisodeCardProps {
  episode: Episode;
  podcastCover?: string;
  index?: number;
  priority?: EpisodeImagePriority;
}

function EpisodeCard({
  episode,
  podcastCover,
  index = 0,
  priority = "medium",
}: EpisodeCardProps) {
  const [isExpanded, setIsExpanded] = useState(false);

  const durationLabel = formatEpisodeDuration(episode.duration);
  const fileSizeLabel = formatEpisodeFileSize(episode.enclosure_length);
  const showTitleLink = shouldShowEpisodeTitleLink(episode.link);
  const showPlayButton = shouldShowEpisodePlayButton(episode.medium_url);
  const showNotes = shouldShowEpisodeShowNotes(episode.show_notes);

  const handleBlur = (event: FocusEvent<HTMLDivElement>) => {
    const nextFocusedElement = event.relatedTarget as Node | null;
    if (
      !nextFocusedElement ||
      !event.currentTarget.contains(nextFocusedElement)
    ) {
      setIsExpanded(false);
    }
  };

  return (
    <div
      className="podcast-episode-card"
      onMouseEnter={() => setIsExpanded(true)}
      onMouseLeave={() => setIsExpanded(false)}
      onFocus={() => setIsExpanded(true)}
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
              {showTitleLink ? (
                <a
                  href={episode.link}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="podcast-episode-title line-clamp-2"
                >
                  {episode.title}
                </a>
              ) : (
                <span className="podcast-episode-title line-clamp-2">
                  {episode.title}
                </span>
              )}

              {/* Play Button Icon */}
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
            </div>

            {/* Meta Info */}
            <div className="podcast-episode-meta">
              {episode.episode_no && (
                <span className="podcast-episode-number">
                  {episode.episode_no}
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
            html={episode.show_notes}
            link={episode.link}
            isExpanded={isExpanded}
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
    prevProps.podcastCover === nextProps.podcastCover &&
    prevProps.index === nextProps.index &&
    prevProps.priority === nextProps.priority
  );
}

// 使用 React.memo 包装组件
export default memo(EpisodeCard, arePropsEqual);

// 添加 displayName 用于调试
EpisodeCard.displayName = "EpisodeCard";
