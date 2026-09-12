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
  const localCount = podcast.episode_count || 0;
  // 未关注节目：明确区分本地已收录数量与源站总数，不暗示已同步整档。
  if (!podcast.is_subscribed) {
    const localText = `未关注 · 已收录 ${localCount} 集`;
    const externalCount = podcast.external_episode_count || 0;
    if (externalCount > 0 && externalCount !== localCount) {
      return `${localText} · 源站 ${externalCount} 集`;
    }
    return localText;
  }
  const externalCount = podcast.external_episode_count || 0;
  if (externalCount > 0 && externalCount !== localCount) {
    return `${localCount} 集 · 源站 ${externalCount} 集`;
  }
  return `${localCount} 集`;
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
