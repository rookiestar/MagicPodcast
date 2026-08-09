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
  isScrolling?: boolean;
  onNavigate?: () => void;
}

export default function ResponsivePodcastCard({
  podcast,
  index = 0,
  priority = "medium",
  detailUrl,
  isMobile,
  isScrolling = false,
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
        isScrolling={isScrolling}
        onClick={onNavigate}
      >
        <article className="podcast-library-card is-mobile">
          <div className="podcast-library-card-cover">
            <PodcastCover
              coverUrl={effectiveCoverUrl}
              title={podcast.title}
              index={index}
              priority={priority}
              sizes="82px"
            />
            {isNew && (
              <div className="podcast-library-card-new">
                New
              </div>
            )}
          </div>

          <div className="podcast-library-card-copy">
            <h3 className="line-clamp-1">
              {podcast.title}
            </h3>

            <p className="podcast-library-card-author">{podcast.author}</p>

            {displayedDescription && (
              <p className="podcast-library-card-description line-clamp-2">
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
        </article>
      </PrefetchLink>
    );
  }

  return (
    <PrefetchLink
      href={detailUrl}
      prefetchId={podcast.id}
      prefetchType="podcast"
      isScrolling={isScrolling}
      onClick={onNavigate}
    >
      <article className="podcast-library-card">
        <div className="podcast-library-card-cover">
          <PodcastCover
            coverUrl={effectiveCoverUrl}
            title={podcast.title}
            index={index}
            priority={priority}
            sizes="(max-width: 767px) calc(50vw - 24px), (max-width: 1023px) calc(33.333vw - 24px), (max-width: 1279px) calc(25vw - 24px), 228px"
            className="!absolute !inset-0 !aspect-none"
          />
          {isNew && (
            <div className="podcast-library-card-new">
              New
            </div>
          )}
        </div>

        <div className="podcast-library-card-copy">
          <h3 className="line-clamp-1">
            {podcast.title}
          </h3>

          <p className="podcast-library-card-author">
            {podcast.author}
          </p>

          {displayedDescription && (
            <p className="podcast-library-card-description line-clamp-2 md:line-clamp-3">
              {displayedDescription}
            </p>
          )}

          <TagList
            tags={podcast.tags || []}
            maxDisplay={displayTagCount}
            maxNameWidth="60px"
            className="mt-auto"
          />

          <div className="podcast-library-card-footer">
            <span>{episodeCountText}</span>
            <span>{relativeTime}</span>
          </div>
        </div>
      </article>
    </PrefetchLink>
  );
}
