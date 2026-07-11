import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { searchApi } from "@/lib/api";
import {
  addToSearchHistory,
  clearSearchHistory,
  getSearchHistory,
} from "@/lib/searchHistoryState";
import {
  createEmptySearchData,
  filterSearchResults,
  isCanceledSearchError,
  normalizeSearchData,
  type SearchResultsData,
  type SearchType,
} from "@/lib/searchSidebarState";

const DEFAULT_DEBOUNCE_MS = 200;
const DEFAULT_MIN_QUERY_LENGTH = 2;
const DEFAULT_SEARCH_PAGE_SIZE = 20;

export type { SearchResultsData, SearchType } from "@/lib/searchSidebarState";
interface UseSearchSidebarOptions {
  isOpen: boolean;
  debounceMs?: number;
  minQueryLength?: number;
  pageSize?: number;
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
    createEmptySearchData,
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
      setAllResults(createEmptySearchData());
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
          include_totals: false,
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
        setAllResults(createEmptySearchData());
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
        setAllResults(createEmptySearchData());
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
