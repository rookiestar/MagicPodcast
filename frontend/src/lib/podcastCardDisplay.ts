import type { Podcast } from "@/types";
import { getEffectiveCoverUrl } from "@/lib/imageProxy";
import { stripHtml } from "@/lib/textUtils";
import { getRelativeTime, isValidDisplayDate } from "@/lib/timeUtils";

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

// getPodcastSyncStateText 返回缺失有效日期时卡片/详情应展示的同步状态文案。
// 依据嵌入的 history_sync 状态区分待同步、排队中、同步中、暂无可获取单集、
// 部分同步与同步未完成；不从 0 集推断源站为空（#462/#463）。
export function getPodcastSyncStateText(
  podcast: Pick<Podcast, "history_sync" | "episode_count">,
): string {
  const sync = podcast.history_sync;
  if (!sync) {
    return "待同步";
  }
  switch (sync.status) {
    case "pending":
      return "待同步";
    case "queued":
      return "排队中";
    case "running":
      return "同步中";
    case "partial":
      return "部分同步";
    case "failed":
      return "同步未完成";
    case "completed":
      return (podcast.episode_count || 0) > 0 ? "日期未知" : "暂无可获取单集";
    default:
      return "日期未知";
  }
}

// getPodcastCardDateStatusText 卡片日期槽的统一展示：有有效日期展示相对
// 时间；缺失或无效日期展示同步状态文案。
export function getPodcastCardDateStatusText(podcast: Podcast): string {
  if (isValidDisplayDate(podcast.newest_episode_date)) {
    return getRelativeTime(podcast.newest_episode_date);
  }
  return getPodcastSyncStateText(podcast);
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
  newestEpisodeDate: string | null | undefined,
  now = new Date(),
) {
  if (!newestEpisodeDate) {
    return false;
  }

  const newestDate = new Date(newestEpisodeDate);
  if (Number.isNaN(newestDate.getTime()) || newestDate.getFullYear() < 1970) {
    return false;
  }

  const threshold = new Date(now);
  threshold.setDate(threshold.getDate() - RECENT_UPDATE_WINDOW_DAYS);
  return newestDate >= threshold;
}
