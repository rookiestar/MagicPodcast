import type {
  EpisodeSearchResult,
  PodcastSearchResult,
  SearchData,
} from "@/types";

export type SearchType = "all" | "podcasts" | "episodes";

export interface SearchResultsData {
  podcasts: PodcastSearchResult[];
  episodes: EpisodeSearchResult[];
  pagination: SearchData["pagination"] | null;
}

export function isCanceledSearchError(error: unknown) {
  return (
    error instanceof Error &&
    (error.name === "CanceledError" || error.message === "canceled")
  );
}

export function createEmptySearchData(): SearchResultsData {
  return { podcasts: [], episodes: [], pagination: null };
}

export function normalizeSearchData(
  data: Partial<SearchData> | null | undefined,
): SearchResultsData {
  const podcasts = Array.isArray(data?.podcasts) ? data.podcasts : [];
  const episodes = Array.isArray(data?.episodes) ? data.episodes : [];

  return {
    podcasts: podcasts.map((podcast) => ({
      ...podcast,
      matched_fields: podcast.matched_fields ?? [],
    })),
    episodes: episodes.map((episode) => ({
      ...episode,
      matched_fields: episode.matched_fields ?? [],
    })),
    pagination: data?.pagination ?? null,
  };
}

export function filterSearchResults(
  data: SearchResultsData,
  searchType: SearchType,
) {
  if (searchType === "podcasts") {
    return { podcasts: data.podcasts, episodes: [] };
  }

  if (searchType === "episodes") {
    return { podcasts: [], episodes: data.episodes };
  }

  return { podcasts: data.podcasts, episodes: data.episodes };
}

export type SearchSidebarPanelState =
  | "history"
  | "prompt"
  | "error"
  | "empty"
  | "loading"
  | "results";

export function getSearchSidebarPanelState({
  loading,
  isQueryTooShort,
  showHistory,
  searchError,
  hasResults,
}: {
  loading: boolean;
  isQueryTooShort: boolean;
  showHistory: boolean;
  searchError: string | null;
  hasResults: boolean;
}): SearchSidebarPanelState {
  if (loading) {
    return "loading";
  }

  if (showHistory) {
    return "history";
  }

  if (isQueryTooShort) {
    return "prompt";
  }

  if (searchError) {
    return "error";
  }

  if (!hasResults) {
    return "empty";
  }

  return "results";
}
