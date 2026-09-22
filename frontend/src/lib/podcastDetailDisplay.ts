import { getEffectiveCoverUrl } from "@/lib/imageProxy";
import { getPodcastSyncStateText } from "@/lib/podcastCardDisplay";
import { isValidDisplayDate } from "@/lib/timeUtils";
import type { Podcast } from "@/types";

export function getPodcastDetailInfoCoverUrl(
  podcast: Pick<Podcast, "custom_cover_url" | "cover_url">,
) {
  return getEffectiveCoverUrl(podcast.custom_cover_url, podcast.cover_url);
}

function padDatePart(value: number) {
  return String(value).padStart(2, "0");
}

export function formatPodcastNewestEpisodeDate(value?: string | null) {
  try {
    const date = value ? new Date(value) : null;
    if (!date || isNaN(date.getTime()) || date.getFullYear() < 1970) {
      return "未知";
    }

    return `${date.getFullYear()}/${padDatePart(date.getMonth() + 1)}/${padDatePart(date.getDate())} ${padDatePart(date.getHours())}:${padDatePart(date.getMinutes())}`;
  } catch {
    return "未知";
  }
}

// getPodcastDetailUpdateText 详情页「更新于」槽位的统一展示：有有效日期
// 展示格式化时间；缺失或无效日期展示同步状态，不再显示 0001/01/01（#463）。
export function getPodcastDetailUpdateText(
  podcast: Pick<Podcast, "newest_episode_date" | "history_sync" | "episode_count">,
) {
  if (isValidDisplayDate(podcast.newest_episode_date)) {
    return `更新于 ${formatPodcastNewestEpisodeDate(podcast.newest_episode_date)}`;
  }
  return getPodcastSyncStateText(podcast);
}

export function formatPodcastDetailMetaLine(
  author?: string | null,
  episodeCount?: number | null,
  newestEpisodeDate?: string | null,
  episodeCountLabel?: string,
  updateText?: string,
) {
  const parts: string[] = [];
  const host = author?.trim();

  if (host) {
    parts.push(host);
  }

  parts.push(episodeCountLabel ?? `${episodeCount || 0} 集`);
  parts.push(updateText ?? `更新于 ${formatPodcastNewestEpisodeDate(newestEpisodeDate)}`);
  return parts.join(" · ");
}

export function getPodcastDescriptionPlainText(html?: string | null) {
  return (html || "")
    .replace(/<br\s*\/?>/gi, "\n")
    .replace(/<\/(?:p|div|h[1-6]|li|tr)>/gi, "\n")
    .replace(/<[^>]+>/g, "")
    .replace(/&nbsp;/gi, " ")
    .replace(/&amp;/gi, "&")
    .replace(/&lt;/gi, "<")
    .replace(/&gt;/gi, ">")
    .replace(/[ \t]+\n/g, "\n")
    .replace(/\n{3,}/g, "\n\n")
    .trim();
}

export function shouldOfferPodcastDescriptionToggle(html?: string | null) {
  const text = getPodcastDescriptionPlainText(html);
  if (!text) {
    return false;
  }

  const lines = text.split(/\n+/).filter(Boolean);
  return text.length > 160 || lines.length > 6;
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
