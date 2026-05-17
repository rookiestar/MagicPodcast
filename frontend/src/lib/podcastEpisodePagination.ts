import type { Episode } from "@/types";

export interface EpisodeLoadMoreState {
  episodeCount: number;
  episodesLoading: boolean;
  isLoadingMore: boolean;
  hasMoreEpisodes: boolean;
}

export function mergeUniqueEpisodes(
  existingEpisodes: Episode[],
  incomingEpisodes: Episode[],
) {
  const loadedEpisodeIds = new Set(
    existingEpisodes.map((episode) => episode.id),
  );
  const uniqueIncomingEpisodes = incomingEpisodes.filter(
    (episode) => !loadedEpisodeIds.has(episode.id),
  );

  return [...existingEpisodes, ...uniqueIncomingEpisodes];
}

export function applyEpisodePage({
  existingEpisodes,
  incomingEpisodes,
  append,
}: {
  existingEpisodes: Episode[];
  incomingEpisodes: Episode[];
  append: boolean;
}) {
  if (!append) {
    return incomingEpisodes;
  }

  return mergeUniqueEpisodes(existingEpisodes, incomingEpisodes);
}

export function canLoadMoreEpisodes({
  episodeCount,
  episodesLoading,
  isLoadingMore,
  hasMoreEpisodes,
}: EpisodeLoadMoreState) {
  return (
    episodeCount > 0 &&
    !episodesLoading &&
    !isLoadingMore &&
    hasMoreEpisodes
  );
}
