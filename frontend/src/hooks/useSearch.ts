import { useState, useCallback, useEffect, useRef } from "react";
import { useRouter, useSearchParams } from "next/navigation";

interface SearchOptions {
  initialQuery?: string;
  debounceMs?: number;
  minQueryLength?: number;
  syncWithUrl?: boolean;
  maxHistory?: number;
}

interface SearchHistory {
  query: string;
  timestamp: number;
}

/**
 * useSearch Hook - 搜索状态管理
 *
 * @param options - 配置选项
 * @returns 搜索状态和控制函数
 *
 * @example
 * const { query, setQuery, results, searching, clearSearch } = useSearch({
 *   debounceMs: 300,
 *   minQueryLength: 2,
 *   syncWithUrl: true,
 * })
 */
export function useSearch(options: SearchOptions = {}) {
  const {
    initialQuery = "",
    debounceMs = 300,
    minQueryLength = 0,
    syncWithUrl = true,
    maxHistory = 10,
  } = options;

  const router = useRouter();
  const searchParams = useSearchParams();
  const urlQuery = syncWithUrl ? searchParams.get("q") || "" : initialQuery;

  const [query, setQueryState] = useState(urlQuery);
  const [debouncedQuery, setDebouncedQuery] = useState(urlQuery);
  const [history, setHistory] = useState<SearchHistory[]>([]);
  const debounceTimerRef = useRef<NodeJS.Timeout>();

  // 从localStorage加载搜索历史
  useEffect(() => {
    try {
      const saved = localStorage.getItem("search_history");
      if (saved) {
        const parsed = JSON.parse(saved);
        setHistory(parsed);
      }
    } catch (error) {
      console.error("Failed to load search history:", error);
    }
  }, []);

  // 保存搜索历史到localStorage
  const saveToHistory = useCallback(
    (searchQuery: string) => {
      if (!searchQuery.trim()) return;

      const newEntry: SearchHistory = {
        query: searchQuery,
        timestamp: Date.now(),
      };

      setHistory((prev) => {
        // 过滤掉相同的查询
        const filtered = prev.filter((h) => h.query !== searchQuery);
        // 添加新查询到开头
        const updated = [newEntry, ...filtered];
        // 限制历史记录数量
        const limited = updated.slice(0, maxHistory);

        // 保存到localStorage
        try {
          localStorage.setItem("search_history", JSON.stringify(limited));
        } catch (error) {
          console.error("Failed to save search history:", error);
        }

        return limited;
      });
    },
    [maxHistory],
  );

  // 防抖处理
  useEffect(() => {
    if (debounceTimerRef.current) {
      clearTimeout(debounceTimerRef.current);
    }

    debounceTimerRef.current = setTimeout(() => {
      setDebouncedQuery(query);

      // 同步到URL
      if (syncWithUrl) {
        const params = new URLSearchParams(searchParams.toString());
        if (query.trim()) {
          params.set("q", query);
        } else {
          params.delete("q");
        }

        const newUrl = `${window.location.pathname}?${params.toString()}`;
        router.replace(newUrl);
      }

      // 保存到历史记录
      if (query.trim() && query.length >= minQueryLength) {
        saveToHistory(query);
      }
    }, debounceMs);

    return () => {
      if (debounceTimerRef.current) {
        clearTimeout(debounceTimerRef.current);
      }
    };
  }, [
    query,
    debounceMs,
    syncWithUrl,
    searchParams,
    router,
    minQueryLength,
    saveToHistory,
  ]);

  // 设置搜索查询
  const setQuery = useCallback((newQuery: string) => {
    setQueryState(newQuery);
  }, []);

  // 清除搜索
  const clearSearch = useCallback(() => {
    setQueryState("");

    if (syncWithUrl) {
      const params = new URLSearchParams(searchParams.toString());
      params.delete("q");

      const newUrl = `${window.location.pathname}?${params.toString()}`;
      router.replace(newUrl);
    }
  }, [syncWithUrl, searchParams, router]);

  // 从历史记录中选择
  const selectFromHistory = useCallback((historyQuery: string) => {
    setQueryState(historyQuery);
  }, []);

  // 清除历史记录
  const clearHistory = useCallback(() => {
    setHistory([]);
    try {
      localStorage.removeItem("search_history");
    } catch (error) {
      console.error("Failed to clear search history:", error);
    }
  }, []);

  // 检查是否可以搜索
  const canSearch = query.length >= minQueryLength;

  return {
    // 状态
    query,
    debouncedQuery,
    canSearch,
    searching: query !== debouncedQuery,
    history,

    // 方法
    setQuery,
    clearSearch,
    selectFromHistory,
    clearHistory,
  };
}

/**
 * useSearchResults Hook - 带搜索结果的Hook
 *
 * @param searchFn - 搜索函数
 * @param options - 配置选项
 * @returns 搜索状态和结果
 *
 * @example
 * const { query, setQuery, results, loading, error } = useSearchResults(
 *   (q) => searchPodcasts(q),
 *   { debounceMs: 300 }
 * )
 */
export function useSearchResults<T>(
  searchFn: (query: string) => Promise<T[]>,
  options: SearchOptions = {},
) {
  const { debouncedQuery, canSearch, ...searchState } = useSearch(options);

  const [results, setResults] = useState<T[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<Error | null>(null);

  // 执行搜索
  useEffect(() => {
    const performSearch = async () => {
      if (!canSearch || !debouncedQuery.trim()) {
        setResults([]);
        setError(null);
        return;
      }

      setLoading(true);
      setError(null);

      try {
        const data = await searchFn(debouncedQuery);
        setResults(data);
      } catch (err) {
        const error = err instanceof Error ? err : new Error(String(err));
        setError(error);
        setResults([]);
      } finally {
        setLoading(false);
      }
    };

    performSearch();
  }, [debouncedQuery, canSearch, searchFn]);

  return {
    ...searchState,
    debouncedQuery,
    canSearch,
    results,
    loading,
    error,
  };
}

/**
 * useAdvancedSearch Hook - 高级搜索（带筛选条件）
 *
 * @param options - 配置选项
 * @returns 搜索状态和方法
 *
 * @example
 * const { filters, setFilter, clearFilters, hasActiveFilters } = useAdvancedSearch({
 *   initialFilters: { tag: '', date: '', status: '' }
 * })
 */
export function useAdvancedSearch<T extends Record<string, any>>({
  initialFilters = {} as T,
  syncWithUrl = true,
}: {
  initialFilters?: T;
  syncWithUrl?: boolean;
} = {}) {
  const router = useRouter();
  const searchParams = useSearchParams();

  // 从URL初始化筛选条件
  const getFiltersFromUrl = useCallback((): T => {
    const filters = { ...initialFilters };
    for (const [key, value] of searchParams.entries()) {
      if (key !== "q" && key !== "page" && key !== "page_size") {
        // 类型转换
        const numValue = Number(value);
        filters[key as keyof T] = (
          isNaN(numValue) ? value : numValue
        ) as T[keyof T];
      }
    }
    return filters;
  }, [initialFilters, searchParams]);

  const [filters, setFiltersState] = useState<T>(getFiltersFromUrl());

  // 同步到URL
  useEffect(() => {
    if (syncWithUrl) {
      const params = new URLSearchParams(searchParams.toString());

      // 更新筛选参数
      Object.entries(filters).forEach(([key, value]) => {
        if (value && value !== "") {
          params.set(key, String(value));
        } else {
          params.delete(key);
        }
      });

      const newUrl = `${window.location.pathname}?${params.toString()}`;
      router.replace(newUrl);
    }
  }, [filters, syncWithUrl, searchParams, router]);

  // 设置单个筛选条件
  const setFilter = useCallback(<K extends keyof T>(key: K, value: T[K]) => {
    setFiltersState((prev) => ({ ...prev, [key]: value }));
  }, []);

  // 批量设置筛选条件
  const setFilters = useCallback((newFilters: Partial<T>) => {
    setFiltersState((prev) => ({ ...prev, ...newFilters }));
  }, []);

  // 清除单个筛选条件
  const clearFilter = useCallback(
    <K extends keyof T>(key: K) => {
      setFiltersState((prev) => ({ ...prev, [key]: initialFilters[key] }));
    },
    [initialFilters],
  );

  // 清除所有筛选条件
  const clearFilters = useCallback(() => {
    setFiltersState(initialFilters);
  }, [initialFilters]);

  // 检查是否有激活的筛选条件
  const hasActiveFilters = Object.entries(filters).some(([key, value]) => {
    const initialValue = initialFilters[key as keyof T];
    return (
      value !== initialValue &&
      value !== "" &&
      value !== null &&
      value !== undefined
    );
  });

  // 获取激活的筛选条件
  const activeFiltersCount = Object.entries(filters).filter(([key, value]) => {
    const initialValue = initialFilters[key as keyof T];
    return (
      value !== initialValue &&
      value !== "" &&
      value !== null &&
      value !== undefined
    );
  }).length;

  return {
    filters,
    setFilter,
    setFilters,
    clearFilter,
    clearFilters,
    hasActiveFilters,
    activeFiltersCount,
  };
}
