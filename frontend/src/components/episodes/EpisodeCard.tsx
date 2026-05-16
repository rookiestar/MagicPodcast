"use client";

import { memo, useMemo, useState, type FocusEvent } from "react";
import type { Episode } from "@/types";
import RichText from "@/components/RichText";
import { useQueuedEpisodeImage } from "@/hooks/useQueuedEpisodeImage";
import {
  formatEpisodeDuration,
  formatEpisodeFileSize,
  getEpisodeCoverDisplay,
  type EpisodeImagePriority,
} from "@/lib/episodeDisplay";
import { stripHtml } from "@/lib/textUtils";
import { formatDate } from "@/lib/timeUtils";

interface EpisodeCardProps {
  episode: Episode;
  podcastCover?: string;
  index?: number;
  priority?: EpisodeImagePriority;
}

function EpisodeShowNotes({
  html,
  link,
  isExpanded,
}: {
  html: string;
  link: string;
  isExpanded: boolean;
}) {
  const preview = useMemo(() => stripHtml(html, 220), [html]);

  return (
    <div className="relative">
      <div
        className={`relative max-h-20 overflow-hidden text-xs text-slate-600 transition-[max-height] duration-300 md:text-sm md:text-slate-600 md:dark:text-slate-400 ${
          isExpanded
            ? "md:max-h-96 md:overflow-y-auto"
            : "md:max-h-24 md:overflow-hidden"
        }`}
      >
        {isExpanded ? (
          <RichText
            html={html}
            className="prose prose-sm dark:prose-invert max-w-none prose-headings:text-base prose-h1:text-base prose-h2:text-base prose-h3:text-base line-clamp-3 md:line-clamp-none"
          />
        ) : (
          <p className="line-clamp-3 whitespace-pre-line">{preview}</p>
        )}
      </div>

      <div
        className={`absolute bottom-0 left-0 right-0 h-6 bg-gradient-to-t from-white dark:from-slate-800 to-transparent pointer-events-none md:h-8 ${
          isExpanded ? "md:hidden" : ""
        }`}
      />

      {link && (
        <a
          href={link}
          target="_blank"
          rel="noopener noreferrer"
          className="block rounded-sm text-center text-xs text-blue-600 dark:text-blue-400 hover:text-blue-700 dark:hover:text-blue-300 mt-2 py-2 border-t border-slate-200 dark:border-slate-700 transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500 focus-visible:ring-offset-2 md:hidden"
        >
          查看详情 →
        </a>
      )}
    </div>
  );
}

function EpisodeCard({
  episode,
  podcastCover,
  index = 0,
  priority = "medium",
}: EpisodeCardProps) {
  const [isExpanded, setIsExpanded] = useState(false);

  const coverDisplay = getEpisodeCoverDisplay(episode, podcastCover);
  const {
    imageLoaded: queuedImageLoaded,
    imageError: queuedImageError,
    imgRef,
  } = useQueuedEpisodeImage({
    episodeId: episode.id,
    src: coverDisplay.shouldQueue ? coverDisplay.src : "",
    priority,
    index,
  });
  const imageLoaded = coverDisplay.shouldQueue
    ? queuedImageLoaded
    : Boolean(coverDisplay.src);
  const imageError = coverDisplay.shouldQueue ? queuedImageError : false;
  const durationLabel = formatEpisodeDuration(episode.duration);
  const fileSizeLabel = formatEpisodeFileSize(episode.enclosure_length);

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
          {/* Thumbnail with LQIP */}
          <div className="flex-shrink-0 w-8 h-8 sm:w-10 sm:h-10 md:w-12 md:h-12 lg:w-14 lg:h-14 rounded-lg overflow-hidden bg-slate-200 relative">
            {/* 播客封面作为模糊占位图（LQIP） */}
            {coverDisplay.placeholderSrc && (
              <img
                src={coverDisplay.placeholderSrc}
                alt=""
                aria-hidden="true"
                loading={index < 3 ? "eager" : "lazy"}
                decoding="async"
                className="absolute inset-0 w-full h-full object-cover blur-md opacity-40"
              />
            )}

            {/* 加载指示器 */}
            {!imageLoaded && !imageError && coverDisplay.shouldQueue && (
              <div className="absolute inset-0 flex items-center justify-center">
                <div className="w-4 h-4 border-2 border-slate-400 border-t-transparent rounded-full animate-spin" />
              </div>
            )}

            {/* 真实单集封面 */}
            {coverDisplay.src ? (
              <img
                ref={coverDisplay.shouldQueue ? imgRef : undefined}
                src={coverDisplay.shouldQueue ? undefined : coverDisplay.src}
                alt={episode.title}
                loading={index < 3 ? "eager" : "lazy"}
                decoding="async"
                className={`w-full h-full object-cover transition-all duration-500 ${
                  imageLoaded ? "opacity-100" : "opacity-0"
                }`}
              />
            ) : null}

            {/* 占位符：当没有封面或图片加载失败时显示 */}
            {(!coverDisplay.src || imageError) && (
              <div
                className="w-full h-full flex items-center justify-center bg-slate-200"
                aria-hidden="true"
              >
                <svg
                  className="h-5 w-5 text-slate-400"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  strokeWidth="2"
                  strokeLinecap="round"
                  strokeLinejoin="round"
                >
                  <path d="M3 18v-6a9 9 0 0 1 18 0v6" />
                  <path d="M21 19a2 2 0 0 1-2 2h-1a2 2 0 0 1-2-2v-3a2 2 0 0 1 2-2h3z" />
                  <path d="M3 19a2 2 0 0 0 2 2h1a2 2 0 0 0 2-2v-3a2 2 0 0 0-2-2H3z" />
                </svg>
              </div>
            )}
          </div>

          {/* Title, Meta Info and Play Button */}
          <div className="flex-1 min-w-0">
            {/* Title with Play Button */}
            <div className="flex items-start justify-between gap-2 mb-1.5">
              {episode.link ? (
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
              {episode.medium_url && (
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
        {episode.show_notes && (
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
