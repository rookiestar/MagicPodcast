"use client";

import type { CSSProperties } from "react";
import { useOriginalEpisodeRecovery } from "@/hooks/useOriginalEpisodeRecovery";
import {
  getEpisodeListDisplayTotal,
  getEpisodeListFinishedMessage,
  getEpisodeListStatus,
  shouldShowEpisodeListFinished,
  shouldShowEpisodeListFooter,
  shouldShowEpisodeListHeading,
} from "@/lib/episodeListState";
import { getEpisodeImagePriority } from "@/lib/episodeDisplay";
import type { Episode } from "@/types";
import EpisodeCard from "./EpisodeCard";

interface EpisodeListSectionProps {
  episodes: Episode[];
  episodesLoading: boolean;
  isLoadingMore: boolean;
  hasMoreEpisodes: boolean;
  totalEpisodes: number;
  episodesError?: string | null;
  podcastCover?: string;
  loadMoreRef: (element: HTMLDivElement | null) => void;
  onRetry?: () => void;
}

const EPISODE_CARD_VISIBILITY_STYLE: CSSProperties = {
  contentVisibility: "auto",
  containIntrinsicSize: "280px",
};

function EpisodeListSkeleton() {
  return (
    <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
      {[1, 2, 3, 4].map((item) => (
        <div
          key={item}
          className="bg-white rounded-lg shadow-sm p-6 animate-pulse"
        >
          <div className="flex items-start gap-3">
            <div className="w-16 h-16 bg-slate-200 rounded-lg"></div>
            <div className="flex-1 space-y-2">
              <div className="h-4 bg-slate-200 rounded w-3/4"></div>
              <div className="h-3 bg-slate-200 rounded w-1/2"></div>
            </div>
          </div>
        </div>
      ))}
    </div>
  );
}

function EmptyEpisodeList() {
  return (
    <div className="bg-white rounded-lg p-12 text-center shadow-sm">
      <p className="text-slate-600 text-lg">暂无单集</p>
      <p className="text-slate-500 text-sm mt-2">点击下方按钮同步单集数据</p>
    </div>
  );
}

function EpisodeErrorState({
  message,
  onRetry,
  compact = false,
}: {
  message: string;
  onRetry?: () => void;
  compact?: boolean;
}) {
  return (
    <div
      role="alert"
      className={`rounded-lg border border-red-200 bg-red-50 text-red-700 ${
        compact ? "mt-6 p-4" : "p-8 text-center shadow-sm"
      }`}
    >
      <p className="font-semibold">单集加载失败</p>
      <p className="mt-1 text-sm">{message}</p>
      {onRetry && (
        <button
          type="button"
          onClick={onRetry}
          className="mt-4 min-h-[44px] cursor-pointer rounded-lg bg-red-600 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-red-700 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-red-500"
        >
          重试
        </button>
      )}
    </div>
  );
}

function EpisodeListFooter({
  hasMoreEpisodes,
  isLoadingMore,
  loadMoreRef,
}: {
  hasMoreEpisodes: boolean;
  isLoadingMore: boolean;
  loadMoreRef: (element: HTMLDivElement | null) => void;
}) {
  if (!shouldShowEpisodeListFooter(hasMoreEpisodes, isLoadingMore)) {
    return null;
  }

  return (
    <div
      ref={hasMoreEpisodes ? loadMoreRef : undefined}
      className="mt-8 flex min-h-14 items-center justify-center text-center"
      aria-live="polite"
    >
      {isLoadingMore ? (
        <p className="text-sm text-slate-600 flex items-center justify-center gap-2">
          <span className="inline-block animate-spin rounded-full h-4 w-4 border-b-2 border-blue-600"></span>
          正在加载更多单集...
        </p>
      ) : (
        <span className="sr-only">继续加载更多单集</span>
      )}
    </div>
  );
}

export default function EpisodeListSection({
  episodes,
  episodesLoading,
  isLoadingMore,
  hasMoreEpisodes,
  totalEpisodes,
  episodesError,
  podcastCover,
  loadMoreRef,
  onRetry,
}: EpisodeListSectionProps) {
  const originalRecovery = useOriginalEpisodeRecovery();
  const episodeListStatus = getEpisodeListStatus({
    episodeCount: episodes.length,
    episodesLoading,
    episodesError,
  });
  const displayTotal = getEpisodeListDisplayTotal(
    totalEpisodes,
    episodes.length,
  );
  const showHeading = shouldShowEpisodeListHeading(
    totalEpisodes,
    episodesLoading,
  );
  const showFinishedMessage = shouldShowEpisodeListFinished({
    episodeCount: episodes.length,
    episodesError,
    hasMoreEpisodes,
  });

  return (
    <section className="podcast-episode-ledger">
      {showHeading ? (
        <h2>
          单集列表 ({displayTotal} 集)
        </h2>
      ) : (
        <div className="mb-6 h-8 w-40 bg-slate-200 rounded animate-pulse"></div>
      )}

      {episodeListStatus === "initial-loading" ? (
        <EpisodeListSkeleton />
      ) : episodeListStatus === "initial-error" ? (
        <EpisodeErrorState message={episodesError} onRetry={onRetry} />
      ) : episodeListStatus === "empty" ? (
        <EmptyEpisodeList />
      ) : (
        <>
          <div className="podcast-episode-list">
            {episodes.map((episode, index) => (
              <div
                key={episode.id}
                id={`episode-${episode.id}`}
                className="transition-all duration-200"
                style={EPISODE_CARD_VISIBILITY_STYLE}
              >
                <EpisodeCard
                  episode={episode}
                  podcastCover={podcastCover}
                  index={index}
                  priority={getEpisodeImagePriority(index)}
                  originalRecovery={originalRecovery}
                />
              </div>
            ))}
          </div>

          {episodesError && (
            <EpisodeErrorState
              message={episodesError}
              onRetry={onRetry}
              compact
            />
          )}

          {!episodesError && (
            <EpisodeListFooter
              hasMoreEpisodes={hasMoreEpisodes}
              isLoadingMore={isLoadingMore}
              loadMoreRef={loadMoreRef}
            />
          )}

          {showFinishedMessage && (
            <div className="text-center mt-8 text-sm text-slate-500">
              {getEpisodeListFinishedMessage(episodes.length)}
            </div>
          )}
        </>
      )}
    </section>
  );
}
