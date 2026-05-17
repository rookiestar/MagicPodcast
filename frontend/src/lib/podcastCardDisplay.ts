import type { Podcast } from "@/types";
import { getEffectiveCoverUrl } from "@/lib/imageProxy";
import { stripHtml } from "@/lib/textUtils";
import { getRelativeTime } from "@/lib/timeUtils";

const RECENT_UPDATE_WINDOW_DAYS = 7;
const MOBILE_DESCRIPTION_LENGTH = 72;
const DESKTOP_DESCRIPTION_LENGTH = 96;

export function getPodcastCardDescription(
  description: string | undefined,
  isMobile: boolean,
) {
  if (!description) {
    return "";
  }

  return stripHtml(
    description,
    isMobile ? MOBILE_DESCRIPTION_LENGTH : DESKTOP_DESCRIPTION_LENGTH,
  );
}

export function getPodcastCardTagLimit(isMobile: boolean) {
  return isMobile ? 2 : 3;
}

export function getPodcastCardCoverUrl(podcast: Podcast) {
  return getEffectiveCoverUrl(podcast.custom_cover_url, podcast.cover_url);
}

export function getPodcastCardRelativeTime(podcast: Podcast) {
  return getRelativeTime(podcast.newest_episode_date);
}

export function getPodcastCardEpisodeCountText(podcast: Podcast) {
  return `${podcast.episode_count || 0} 集`;
}

export function isPodcastRecentlyUpdated(
  newestEpisodeDate: string | undefined,
  now = new Date(),
) {
  if (!newestEpisodeDate) {
    return false;
  }

  const newestDate = new Date(newestEpisodeDate);
  if (Number.isNaN(newestDate.getTime())) {
    return false;
  }

  const threshold = new Date(now);
  threshold.setDate(threshold.getDate() - RECENT_UPDATE_WINDOW_DAYS);
  return newestDate >= threshold;
}
