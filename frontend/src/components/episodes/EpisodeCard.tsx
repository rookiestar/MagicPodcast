"use client";

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
      className="relative bg-white rounded-xl shadow-sm hover:shadow-xl transition-all duration-300 overflow-hidden border border-slate-200 flex flex-col h-full"
      onMouseEnter={() => setIsExpanded(true)}
      onMouseLeave={() => setIsExpanded(false)}
      onFocus={() => setIsExpanded(true)}
      onBlur={handleBlur}
    >
      {/* Content */}
      <div className="p-3 sm:p-4 flex-1 flex flex-col">
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
            <div className="flex items-start justify-between gap-2 mb-1.5">
              {showTitleLink ? (
                <a
                  href={episode.link}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="flex-1 rounded-sm font-semibold text-slate-900 text-xs sm:text-sm md:text-base line-clamp-2 leading-snug hover:text-blue-600 dark:hover:text-blue-400 transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500 focus-visible:ring-offset-2"
                >
                  {episode.title}
                </a>
              ) : (
                <span className="flex-1 font-semibold text-slate-900 text-xs sm:text-sm md:text-base line-clamp-2 leading-snug">
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
                  className="flex-shrink-0 w-10 h-10 sm:w-11 sm:h-11 flex items-center justify-center rounded-full bg-blue-600 hover:bg-blue-700 text-white transition-all duration-200 hover:scale-110 active:scale-95 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500 focus-visible:ring-offset-2"
                  aria-label="播放"
                >
                  <svg
                    className="w-5 h-5 ml-0.5"
                    fill="currentColor"
                    viewBox="0 0 24 24"
                  >
                    <path d="M8 5v14l11-7z" />
                  </svg>
                </button>
              )}
            </div>

            {/* Meta Info */}
            <div className="flex items-center gap-2 sm:gap-3 text-[10px] sm:text-xs text-slate-500 dark:text-slate-400">
              {episode.episode_no && (
                <span className="px-2 py-0.5 bg-blue-100 dark:bg-blue-900/30 text-blue-700 dark:text-blue-300 rounded-md font-medium">
                  {episode.episode_no}
                </span>
              )}
              <span>{formatDate(episode.published_date)}</span>
              {durationLabel && (
                <>
                  <span>•</span>
                  <span>{durationLabel}</span>
                </>
              )}
              {fileSizeLabel && (
                <>
                  <span>•</span>
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
