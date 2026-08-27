import { getSafeOriginalUrl } from "@/lib/originalEpisodeOpen";
import type { Episode } from "@/types";

export type EpisodeImagePriority = "high" | "medium" | "low";

export interface EpisodeCoverDisplay {
  src: string;
  placeholderSrc: string;
  shouldQueue: boolean;
}

export function getEpisodeCoverDisplay(
  episode: Pick<Episode, "image_url">,
  podcastCover?: string,
): EpisodeCoverDisplay {
  if (episode.image_url) {
    return {
      src: episode.image_url,
      placeholderSrc: podcastCover || "",
      shouldQueue: true,
    };
  }

  return {
    src: podcastCover || "",
    placeholderSrc: "",
    shouldQueue: false,
  };
}

export function getEpisodeCoverImage(
  episode: Pick<Episode, "image_url">,
  podcastCover?: string,
) {
  return getEpisodeCoverDisplay(episode, podcastCover).src;
}

export function getEpisodeImageLoadDelay(
  priority: EpisodeImagePriority,
  index = 0,
) {
  if (priority === "high") {
    return 0;
  }

  if (priority === "medium") {
    return index >= 3 ? 200 : 0;
  }

  return index >= 10 ? 500 : 0;
}

export function getEpisodeImagePriority(index: number): EpisodeImagePriority {
  if (index < 3) return "high";
  if (index < 10) return "medium";
  return "low";
}

export function getEpisodeImageLoading(index: number): "eager" | "lazy" {
  return index < 3 ? "eager" : "lazy";
}

export function formatEpisodeDuration(seconds?: number | null) {
  if (!seconds || seconds <= 0) {
    return null;
  }

  const totalSeconds = Math.floor(seconds);
  const hours = Math.floor(totalSeconds / 3600);
  const minutes = Math.floor((totalSeconds % 3600) / 60);
  const remainingSeconds = totalSeconds % 60;

  const parts: string[] = [];

  if (hours > 0) {
    parts.push(`${hours}小时`);
  }

  if (minutes > 0) {
    parts.push(`${minutes}分`);
  }

  if (remainingSeconds > 0 || parts.length === 0) {
    parts.push(`${remainingSeconds}秒`);
  }

  return parts.join("");
}

export function formatEpisodeNumber(value?: string | null) {
  const normalized = value?.trim() ?? "";
  if (!normalized) return null;

  if (/^s\d{1,3}e\d{1,4}$/i.test(normalized)) {
    return normalized.toUpperCase();
  }

  const numeric = normalized.match(
    /^(?:#|e(?:pisode)?|ep|vol(?:ume)?)[ ._-]*(\d{1,4})$/i,
  );
  if (numeric) return `#${numeric[1]}`;
  if (/^\d{1,4}$/.test(normalized)) return `#${normalized}`;
  if (/^\d+$/.test(normalized)) return null;

  return normalized;
}

export function formatEpisodeFileSize(bytes?: number | null) {
  if (!bytes || bytes <= 0) {
    return null;
  }

  const mb = bytes / (1024 * 1024);
  return `${mb.toFixed(1)} MB`;
}

export function shouldShowEpisodeImageLoader(
  imageLoaded: boolean,
  imageError: boolean,
  shouldQueue: boolean,
) {
  return !imageLoaded && !imageError && shouldQueue;
}

export function shouldShowEpisodeImagePlaceholder(
  src: string,
  imageError: boolean,
) {
  return !src || imageError;
}

export function shouldShowEpisodeTitleLink(link?: string | null) {
  return Boolean(link);
}

export function shouldShowEpisodePlayButton(mediumUrl?: string | null) {
  return Boolean(mediumUrl);
}

export function shouldShowEpisodeShowNotes(showNotes?: string | null) {
  return Boolean(showNotes);
}

export type EpisodeVideoAction =
  | { show: false }
  | { show: true; href: string };

export function planEpisodeVideoAction(
  videoAvailability: string | undefined | null,
  openUrl: string | undefined | null,
): EpisodeVideoAction {
  if (videoAvailability !== "available") {
    return { show: false };
  }
  const href = getSafeOriginalUrl(openUrl);
  if (!href) {
    return { show: false };
  }
  return { show: true, href };
}
