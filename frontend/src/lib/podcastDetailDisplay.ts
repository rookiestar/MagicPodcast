import { getEffectiveCoverUrl } from "@/lib/imageProxy";
import type { Podcast } from "@/types";

export function getPodcastDetailInfoCoverUrl(
  podcast: Pick<Podcast, "custom_cover_url" | "cover_url">,
) {
  return getEffectiveCoverUrl(podcast.custom_cover_url, podcast.cover_url);
}

export function formatPodcastNewestEpisodeDate(value?: string | null) {
  try {
    const date = value ? new Date(value) : null;
    return date && !isNaN(date.getTime())
      ? date.toLocaleString("zh-CN", {
          year: "numeric",
          month: "2-digit",
          day: "2-digit",
          hour: "2-digit",
          minute: "2-digit",
          second: "2-digit",
          hour12: false,
        })
      : "未知";
  } catch {
    return "未知";
  }
}

export function formatPodcastLatestEpisodeDurationLabel(
  duration?: number | null,
) {
  if (!duration || duration <= 0) {
    return null;
  }

  const totalSeconds = Math.floor(duration);
  return `${Math.floor(totalSeconds / 60)}分${totalSeconds % 60}秒`;
}

export function getPodcastDescriptionHtml(description?: string | null) {
  return description || "暂无简介";
}

export function shouldShowPodcastWebsiteLink(link?: string | null) {
  return Boolean(link);
}

export function shouldShowPodcastPopularityBadge(score?: number | null) {
  return Boolean(score && score >= 7);
}

export function shouldShowPodcastLatestEpisodePlayButton(
  newestEnclosureUrl?: string | null,
) {
  return Boolean(newestEnclosureUrl);
}
