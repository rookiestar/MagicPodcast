"use client";

import PrefetchLink from "@/components/common/PrefetchLink";
import PodcastCover from "@/components/podcasts/PodcastCover";
import TagList from "@/components/ui/TagList";
import {
  getPodcastCardCoverUrl,
  getPodcastCardDescription,
  getPodcastCardEpisodeCountText,
  getPodcastCardRelativeTime,
  getPodcastCardTagLimit,
  isPodcastRecentlyUpdated,
} from "@/lib/podcastCardDisplay";
import type { Podcast } from "@/types";

interface ResponsivePodcastCardProps {
  podcast: Podcast;
  index: number;
  priority?: "high" | "medium" | "low";
  detailUrl: string;
  isMobile: boolean;
  onNavigate?: () => void;
}

export default function ResponsivePodcastCard({
  podcast,
  index = 0,
  priority = "medium",
  detailUrl,
  isMobile,
  onNavigate,
}: ResponsivePodcastCardProps) {
  const displayedDescription = getPodcastCardDescription(
    podcast.description,
    isMobile,
  );
  const displayTagCount = getPodcastCardTagLimit(isMobile);
  const relativeTime = getPodcastCardRelativeTime(podcast);
  const episodeCountText = getPodcastCardEpisodeCountText(podcast);
  const effectiveCoverUrl = getPodcastCardCoverUrl(podcast);
  const isNew = isPodcastRecentlyUpdated(podcast.newest_episode_date);

  if (isMobile) {
    return (
      <PrefetchLink
        href={detailUrl}
        prefetchId={podcast.id}
        prefetchType="podcast"
        onClick={onNavigate}
      >
        <div className="flex flex-row gap-3 p-3 bg-white rounded-xl shadow-md hover:shadow-lg active:scale-[0.97] active:shadow-sm transition-all duration-200 ease-out overflow-hidden cursor-pointer">
          <div className="w-16 h-16 flex-shrink-0 relative rounded-lg overflow-hidden">
            <PodcastCover
              coverUrl={effectiveCoverUrl}
              title={podcast.title}
              index={index}
              priority={priority}
              sizes="64px"
            />
            {isNew && (
              <div className="absolute top-0.5 right-0.5 px-0.5 py-0.5 bg-emerald-500/65 text-white text-[6px] font-bold uppercase tracking-wider rounded shadow-sm">
                NEW
              </div>
            )}
          </div>

          <div className="flex-1 min-w-0 flex flex-col gap-2">
            <h3 className="text-base font-semibold text-slate-900 line-clamp-1 leading-tight">
              {podcast.title}
            </h3>

            <p className="text-xs text-slate-600 mb-0.5">{podcast.author}</p>

            {displayedDescription && (
              <p className="text-xs text-slate-500 leading-snug line-clamp-2">
                {displayedDescription}
              </p>
            )}

            <TagList
              tags={podcast.tags || []}
              maxDisplay={displayTagCount}
              maxNameWidth="60px"
              className="mt-auto"
            />
          </div>
        </div>
      </PrefetchLink>
    );
  }

  return (
    <PrefetchLink
      href={detailUrl}
      prefetchId={podcast.id}
      prefetchType="podcast"
      onClick={onNavigate}
    >
      <div className="flex flex-col h-full bg-white rounded-xl shadow-md hover:shadow-lg active:scale-[0.97] active:shadow-sm transition-all duration-200 ease-out overflow-hidden cursor-pointer">
        <div className="relative w-full pt-[100%] rounded-lg overflow-hidden bg-slate-200">
          <PodcastCover
            coverUrl={effectiveCoverUrl}
            title={podcast.title}
            index={index}
            priority={priority}
            sizes="(max-width: 640px) 64px, (max-width: 768px) 50vw, (max-width: 1024px) 33vw, (max-width: 1280px) 25vw, 240px"
            className="!absolute !inset-0 !aspect-none"
          />
          {isNew && (
            <div className="absolute top-2 right-2 px-1.5 py-0.5 bg-emerald-500/65 text-white text-[10px] font-bold uppercase tracking-wider rounded-md shadow-sm">
              NEW
            </div>
          )}
        </div>

        <div className="flex-1 gap-2 md:gap-3 p-4 flex flex-col">
          <h3 className="text-lg font-semibold text-slate-900 line-clamp-1 leading-tight">
            {podcast.title}
          </h3>

          <p className="text-xs sm:text-sm text-slate-600 mb-0.5">
            {podcast.author}
          </p>

          {displayedDescription && (
            <p className="text-sm text-slate-500 leading-snug line-clamp-2 md:line-clamp-3">
              {displayedDescription}
            </p>
          )}

          <TagList
            tags={podcast.tags || []}
            maxDisplay={displayTagCount}
            maxNameWidth="60px"
            className="mt-auto"
          />

          <div className="flex items-center justify-between text-xs text-slate-500 mt-auto pt-1 md:pt-3">
            <span className="font-medium">{episodeCountText}</span>
            <span className="text-slate-400">{relativeTime}</span>
          </div>
        </div>
      </div>
    </PrefetchLink>
  );
}
