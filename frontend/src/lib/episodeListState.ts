export type EpisodeListStatus =
  | "initial-loading"
  | "initial-error"
  | "empty"
  | "ready";

interface EpisodeListStatusParams {
  episodeCount: number;
  episodesLoading: boolean;
  episodesError?: string | null;
}

export function getEpisodeListStatus({
  episodeCount,
  episodesLoading,
  episodesError,
}: EpisodeListStatusParams): EpisodeListStatus {
  if (episodesLoading && episodeCount === 0) {
    return "initial-loading";
  }

  if (episodesError && episodeCount === 0) {
    return "initial-error";
  }

  if (episodeCount === 0) {
    return "empty";
  }

  return "ready";
}

export function getEpisodeListDisplayTotal(
  totalEpisodes: number,
  episodeCount: number,
) {
  return totalEpisodes > 0 ? totalEpisodes : episodeCount;
}

export function shouldShowEpisodeListHeading(
  totalEpisodes: number,
  episodesLoading: boolean,
) {
  return totalEpisodes > 0 || !episodesLoading;
}

export function shouldShowEpisodeListFooter(
  hasMoreEpisodes: boolean,
  isLoadingMore: boolean,
) {
  return hasMoreEpisodes || isLoadingMore;
}

interface ShouldShowEpisodeListFinishedParams {
  episodeCount: number;
  episodesError?: string | null;
  hasMoreEpisodes: boolean;
}

export function shouldShowEpisodeListFinished({
  episodeCount,
  episodesError,
  hasMoreEpisodes,
}: ShouldShowEpisodeListFinishedParams) {
  return !episodesError && !hasMoreEpisodes && episodeCount > 0;
}

export function getEpisodeListFinishedMessage(episodeCount: number) {
  return `已加载全部 ${episodeCount} 集单集`;
}
