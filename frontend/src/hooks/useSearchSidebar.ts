import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { searchApi } from "@/lib/api";
import type {
  EpisodeSearchResult,
  PodcastSearchResult,
  SearchData,
} from "@/types";

export type SearchType = "all" | "podcasts" | "episodes";

const MAX_SEARCH_HISTORY = 6;
const STORAGE_KEY = "podcast_search_history";
const DEFAULT_DEBOUNCE_MS = 200;
const DEFAULT_MIN_QUERY_LENGTH = 2;
const DEFAULT_SEARCH_PAGE_SIZE = 50;

function isCanceledSearchError(error: unknown) {
  return (
    error instanceof Error &&
    (error.name === "CanceledError" || error.message === "canceled")
  );
}

export interface SearchResultsData {
  podcasts: PodcastSearchResult[];
  episodes: EpisodeSearchResult[];
  pagination: SearchData["pagination"] | null;
}

interface UseSearchSidebarOptions {
  isOpen: boolean;
  debounceMs?: number;
  minQueryLength?: number;
  pageSize?: number;
}

function emptySearchData(): SearchResultsData {
  return { podcasts: [], episodes: [], pagination: null };
}

export function getSearchHistory(): string[] {
  if (typeof window === "undefined") return [];

  try {
    const stored = localStorage.getItem(STORAGE_KEY);
    if (!stored) return [];

    const parsed = JSON.parse(stored);
    if (!Array.isArray(parsed)) return [];

    return parsed.filter((item): item is string => typeof item === "string");
  } catch {
    return [];
  }
}

export function saveSearchHistory(history: string[]) {
  if (typeof window === "undefined") return;

  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(history));
  } catch (error) {
    console.error("Failed to save search history:", error);
  }
}

export function addToSearchHistory(query: string): string[] {
  const normalizedQuery = query.trim();
  if (!normalizedQuery) return getSearchHistory();

  const history = getSearchHistory();
  const filtered = history.filter((item) => item !== normalizedQuery);
  const newHistory = [normalizedQuery, ...filtered].slice(
    0,
    MAX_SEARCH_HISTORY,
  );

  saveSearchHistory(newHistory);
  return newHistory;
}

export function clearSearchHistory() {
  saveSearchHistory([]);
}

export function normalizeSearchData(data: SearchData): SearchResultsData {
  return {
    podcasts: data.podcasts.map((podcast) => ({
      ...podcast,
      matched_fields: podcast.matched_fields ?? [],
    })),
    episodes: data.episodes.map((episode) => ({
      ...episode,
      matched_fields: episode.matched_fields ?? [],
    })),
    pagination: data.pagination,
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

export function useSearchSidebar({
  isOpen,
  debounceMs = DEFAULT_DEBOUNCE_MS,
  minQueryLength = DEFAULT_MIN_QUERY_LENGTH,
  pageSize = DEFAULT_SEARCH_PAGE_SIZE,
}: UseSearchSidebarOptions) {
  const [query, setQuery] = useState("");
  const [searchType, setSearchType] = useState<SearchType>("all");
  const [allResults, setAllResults] = useState<SearchResultsData>(
    emptySearchData,
  );
  const [loading, setLoading] = useState(false);
  const [searchError, setSearchError] = useState<string | null>(null);
  const [searchHistory, setSearchHistory] = useState<string[]>([]);
  const activeRequestIdRef = useRef(0);
  const activeAbortControllerRef = useRef<AbortController | null>(null);

  const results = useMemo(
    () => filterSearchResults(allResults, searchType),
    [allResults, searchType],
  );

  const hasResults =
    results.podcasts.length > 0 || results.episodes.length > 0;
  const isQueryTooShort = query.trim().length < minQueryLength;
  const showHistory =
    isQueryTooShort &&
    searchHistory.length > 0 &&
    !loading &&
    !hasResults &&
    !searchError;

  useEffect(() => {
    if (!isOpen) {
      activeRequestIdRef.current += 1;
      activeAbortControllerRef.current?.abort();
      activeAbortControllerRef.current = null;
      setQuery("");
      setSearchType("all");
      setAllResults(emptySearchData());
      setSearchError(null);
      setLoading(false);
      return;
    }

    setSearchHistory(getSearchHistory());
  }, [isOpen]);

  const performSearch = useCallback(
    async (
      searchQuery: string,
      requestId: number,
      signal: AbortSignal,
    ) => {
      setLoading(true);
      setSearchError(null);

      try {
        const response = await searchApi.search({
          q: searchQuery,
          type: "all",
          page: 1,
          page_size: pageSize,
          episode_page: 1,
          episode_page_size: pageSize,
        }, { signal });

        if (requestId !== activeRequestIdRef.current) return;

        setAllResults(normalizeSearchData(response.data));
        setSearchHistory(addToSearchHistory(searchQuery));
      } catch (error) {
        if (isCanceledSearchError(error)) {
          return;
        }

        if (requestId !== activeRequestIdRef.current) return;

        console.error("Search failed:", error);
        setAllResults(emptySearchData());
        setSearchError("搜索失败，请稍后重试");
      } finally {
        if (requestId === activeRequestIdRef.current) {
          setLoading(false);
        }
      }
    },
    [pageSize],
  );

  useEffect(() => {
    if (!isOpen) return;

    const requestId = activeRequestIdRef.current + 1;
    activeRequestIdRef.current = requestId;
    activeAbortControllerRef.current?.abort();
    activeAbortControllerRef.current = null;

    const timer = setTimeout(() => {
      const searchQuery = query.trim();

      if (searchQuery.length >= minQueryLength) {
        const controller = new AbortController();
        activeAbortControllerRef.current = controller;
        void performSearch(searchQuery, requestId, controller.signal);
      } else {
        setAllResults(emptySearchData());
        setSearchError(null);
        setLoading(false);
      }
    }, debounceMs);

    return () => clearTimeout(timer);
  }, [debounceMs, isOpen, minQueryLength, performSearch, query]);

  useEffect(() => {
    return () => {
      activeAbortControllerRef.current?.abort();
      activeAbortControllerRef.current = null;
    };
  }, []);

  const selectHistory = useCallback((historyQuery: string) => {
    setQuery(historyQuery);
  }, []);

  const clearHistory = useCallback(() => {
    clearSearchHistory();
    setSearchHistory([]);
  }, []);

  return {
    query,
    setQuery,
    searchType,
    setSearchType,
    allResults,
    results,
    loading,
    searchError,
    searchHistory,
    hasResults,
    isQueryTooShort,
    showHistory,
    selectHistory,
    clearHistory,
  };
}
