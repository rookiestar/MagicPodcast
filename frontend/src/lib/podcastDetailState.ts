import { positiveID } from "@/lib/navigationParams";
import { getEffectiveCoverUrl } from "@/lib/imageProxy";
import type { Podcast } from "@/types";

export function parsePodcastDetailId(value: string | string[] | undefined) {
  return Array.isArray(value) ? (value.length === 1 ? positiveID(value[0]) : null) : positiveID(value);
}

export function getPodcastDetailErrorMessage(podcastError: boolean) {
  return podcastError ? "加载播客失败" : null;
}

export function getPodcastDetailTitle(
  podcast?: Pick<Podcast, "title"> | null,
) {
  return podcast?.title || "播客详情";
}

export function getPodcastDetailDescription(
  podcast: Pick<Podcast, "episode_count"> | null | undefined,
  loadedEpisodeCount: number,
) {
  const episodeCount = podcast?.episode_count || loadedEpisodeCount;
  return podcast && episodeCount > 0 ? `共 ${episodeCount} 个单集` : undefined;
}

export function getPodcastDetailCoverUrl(
  podcast:
    | Pick<Podcast, "custom_cover_url" | "cover_url">
    | null
    | undefined,
) {
  if (!podcast) {
    return undefined;
  }

  return getEffectiveCoverUrl(podcast.custom_cover_url, podcast.cover_url);
}

interface CanAutoLoadMorePodcastEpisodesParams {
  episodeCount: number;
  episodesLoading: boolean;
  isLoadingMore: boolean;
  hasMoreEpisodes: boolean;
  episodesError?: string | null;
}

export function canAutoLoadMorePodcastEpisodes({
  episodeCount,
  episodesLoading,
  isLoadingMore,
  hasMoreEpisodes,
  episodesError,
}: CanAutoLoadMorePodcastEpisodesParams) {
  return Boolean(
    episodeCount > 0 &&
      !episodesLoading &&
      !isLoadingMore &&
      hasMoreEpisodes &&
      !episodesError,
  );
}
