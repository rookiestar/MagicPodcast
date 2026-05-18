"use client";

import Image from "next/image";
import PlainImage from "@/components/ui/PlainImage";
import type { Episode } from "@/types";
import { useQueuedEpisodeImage } from "@/hooks/useQueuedEpisodeImage";
import {
  getEpisodeCoverDisplay,
  getEpisodeImageLoading,
  shouldShowEpisodeImageLoader,
  shouldShowEpisodeImagePlaceholder,
  type EpisodeImagePriority,
} from "@/lib/episodeDisplay";
import { canUseNextImage, getOptimizedImageUrl } from "@/lib/imageOptimization";

interface EpisodeThumbnailProps {
  episode: Episode;
  podcastCover?: string;
  index: number;
  priority: EpisodeImagePriority;
}

export function EpisodeThumbnail({
  episode,
  podcastCover,
  index,
  priority,
}: EpisodeThumbnailProps) {
  const coverDisplay = getEpisodeCoverDisplay(episode, podcastCover);
  const optimizedCoverSrc = getOptimizedImageUrl(coverDisplay.src);
  const optimizedPlaceholderSrc = getOptimizedImageUrl(
    coverDisplay.placeholderSrc,
  );
  const {
    imageLoaded: queuedImageLoaded,
    imageError: queuedImageError,
  } = useQueuedEpisodeImage({
    episodeId: episode.id,
    src: coverDisplay.shouldQueue ? optimizedCoverSrc : "",
    priority,
    index,
  });
  const imageLoaded = coverDisplay.shouldQueue
    ? queuedImageLoaded
    : Boolean(optimizedCoverSrc);
  const imageError = coverDisplay.shouldQueue ? queuedImageError : false;
  const showImageLoader = shouldShowEpisodeImageLoader(
    imageLoaded,
    imageError,
    coverDisplay.shouldQueue,
  );
  const showImagePlaceholder = shouldShowEpisodeImagePlaceholder(
    coverDisplay.src,
    imageError,
  );
  const imageLoading = getEpisodeImageLoading(index);
  const visibleCoverSrc =
    coverDisplay.shouldQueue && !imageLoaded ? "" : optimizedCoverSrc;

  return (
    <div className="flex-shrink-0 w-8 h-8 sm:w-10 sm:h-10 md:w-12 md:h-12 lg:w-14 lg:h-14 rounded-lg overflow-hidden bg-slate-200 relative">
      {optimizedPlaceholderSrc && (
        <ThumbnailImage
          src={optimizedPlaceholderSrc}
          alt=""
          aria-hidden="true"
          loading={imageLoading}
          decoding="async"
          className="absolute inset-0 w-full h-full object-cover blur-md opacity-40"
        />
      )}

      {showImageLoader && (
        <div className="absolute inset-0 flex items-center justify-center">
          <div className="w-4 h-4 border-2 border-slate-400 border-t-transparent rounded-full animate-spin" />
        </div>
      )}

      {visibleCoverSrc ? (
        <ThumbnailImage
          src={visibleCoverSrc}
          alt={episode.title}
          loading={imageLoading}
          decoding="async"
          className={`w-full h-full object-cover transition-all duration-500 ${
            imageLoaded ? "opacity-100" : "opacity-0"
          }`}
        />
      ) : null}

      {showImagePlaceholder && (
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
  );
}

interface ThumbnailImageProps {
  src: string;
  alt: string;
  loading: "eager" | "lazy";
  decoding: "async";
  className: string;
  "aria-hidden"?: "true";
}

function ThumbnailImage({
  src,
  alt,
  loading,
  decoding,
  className,
  "aria-hidden": ariaHidden,
}: ThumbnailImageProps) {
  if (canUseNextImage(src)) {
    return (
      <Image
        src={src}
        alt={alt}
        fill
        sizes="56px"
        unoptimized
        loading={loading}
        decoding={decoding}
        aria-hidden={ariaHidden}
        className={className}
      />
    );
  }

  return (
    <PlainImage
      src={src}
      alt={alt}
      loading={loading}
      decoding={decoding}
      aria-hidden={ariaHidden}
      className={className}
    />
  );
}
