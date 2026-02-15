import { useState, useCallback, useEffect, useRef } from "react";
import { useSearchParams } from "next/navigation";

interface PaginationOptions {
  initialPage?: number;
  initialPageSize?: number;
  pageSizeOptions?: number[];
  totalItems?: number;
  syncWithUrl?: boolean;
}

interface PaginationState {
  page: number;
  pageSize: number;
  totalPages: number;
  hasNextPage: boolean;
  hasPrevPage: boolean;
}

/**
 * usePagination Hook - 分页状态管理
 *
 * @param options - 配置选项
 * @returns 分页状态和控制函数
 *
 * @example
 * const { page, pageSize, totalPages, nextPage, prevPage, setPageSize } = usePagination({
 *   totalItems: 100,
 *   syncWithUrl: true,
 * })
 */
export function usePagination(options: PaginationOptions = {}) {
  const {
    initialPage = 1,
    initialPageSize = 20,
    pageSizeOptions = [10, 20, 50, 100],
    totalItems: initialTotalItems = 0,
    syncWithUrl = true,
  } = options;

  const searchParams = useSearchParams();
  const urlPage = syncWithUrl
    ? parseInt(searchParams.get("page") || "1", 10)
    : initialPage;
  const urlPageSize = syncWithUrl
    ? parseInt(searchParams.get("page_size") || String(initialPageSize), 10)
    : initialPageSize;

  const [page, setPage] = useState(urlPage || initialPage);
  const [pageSize, setPageSizeState] = useState(urlPageSize || initialPageSize);
  const [totalItems, setTotalItems] = useState(initialTotalItems);

  // 计算总页数
  const totalPages = Math.ceil(totalItems / pageSize) || 1;

  // 计算是否有下一页/上一页
  const hasNextPage = page < totalPages;
  const hasPrevPage = page > 1;

  // 同步到URL
  useEffect(() => {
    if (syncWithUrl) {
      const params = new URLSearchParams(searchParams.toString());
      params.set("page", String(page));
      params.set("page_size", String(pageSize));

      // 使用replaceState避免在历史记录中创建太多条目
      const newUrl = `${window.location.pathname}?${params.toString()}`;
      window.history.replaceState({}, "", newUrl);
    }
  }, [page, pageSize, syncWithUrl, searchParams]);

  // 从URL同步状态
  useEffect(() => {
    if (syncWithUrl) {
      const urlPage = parseInt(searchParams.get("page") || "1", 10);
      const urlPageSize = parseInt(
        searchParams.get("page_size") || String(initialPageSize),
        10,
      );

      if (urlPage && urlPage !== page) {
        setPage(urlPage);
      }
      if (urlPageSize && urlPageSize !== pageSize) {
        setPageSizeState(urlPageSize);
      }
    }
  }, [searchParams, syncWithUrl, initialPageSize]);

  // 更新总项目数
  const updateTotalItems = useCallback((total: number) => {
    setTotalItems(total);
  }, []);

  // 下一页
  const nextPage = useCallback(() => {
    setPage((prev) => Math.min(prev + 1, totalPages));
  }, [totalPages]);

  // 上一页
  const prevPage = useCallback(() => {
    setPage((prev) => Math.max(prev - 1, 1));
  }, []);

  // 跳转到指定页
  const goToPage = useCallback(
    (targetPage: number) => {
      const validPage = Math.max(1, Math.min(targetPage, totalPages));
      setPage(validPage);
    },
    [totalPages],
  );

  // 设置每页大小
  const setPageSize = useCallback((newPageSize: number) => {
    setPageSizeState(newPageSize);
    // 重置到第一页
    setPage(1);
  }, []);

  // 重置分页
  const reset = useCallback(() => {
    setPage(initialPage);
    setPageSizeState(initialPageSize);
  }, [initialPage, initialPageSize]);

  return {
    // 状态
    page,
    pageSize,
    totalItems,
    totalPages,
    hasNextPage,
    hasPrevPage,
    pageSizeOptions,

    // 计算属性
    offset: (page - 1) * pageSize,
    startIndex: (page - 1) * pageSize + 1,
    endIndex: Math.min(page * pageSize, totalItems),

    // 方法
    nextPage,
    prevPage,
    goToPage,
    setPageSize,
    updateTotalItems,
    reset,

    // 状态对象（便于传递给组件）
    state: {
      page,
      pageSize,
      totalItems,
      totalPages,
      hasNextPage,
      hasPrevPage,
    } as PaginationState,
  };
}

/**
 * useInfiniteScroll Hook - 无限滚动分页
 *
 * @param options - 配置选项
 * @returns 分页状态和控制函数
 *
 * @example
 * const { data, loading, hasMore, loadMore } = useInfiniteScroll({
 *   fetchFn: (page) => fetchPodcasts(page),
 * })
 */
export function useInfiniteScroll<T>({
  fetchFn,
  initialPageSize = 20,
  threshold = 200,
}: {
  fetchFn: (
    page: number,
    pageSize: number,
  ) => Promise<{ data: T[]; total: number }>;
  initialPageSize?: number;
  threshold?: number;
}) {
  const [data, setData] = useState<T[]>([]);
  const [loading, setLoading] = useState(false);
  const [page, setPage] = useState(1);
  const [totalItems, setTotalItems] = useState(0);
  const [hasMore, setHasMore] = useState(true);

  // 加载数据
  const loadMore = useCallback(async () => {
    if (loading || !hasMore) return;

    setLoading(true);
    try {
      const result = await fetchFn(page, initialPageSize);

      setData((prev) => [...prev, ...result.data]);
      setTotalItems(result.total);
      setPage((prev) => prev + 1);

      // 检查是否还有更多数据
      const loadedCount = data.length + result.data.length;
      setHasMore(loadedCount < result.total);
    } catch (error) {
      console.error("Failed to load more data:", error);
    } finally {
      setLoading(false);
    }
  }, [fetchFn, page, initialPageSize, loading, hasMore, data.length]);

  // 重置
  const reset = useCallback(() => {
    setData([]);
    setPage(1);
    setTotalItems(0);
    setHasMore(true);
  }, []);

  // 刷新（重新加载第一页）
  const refresh = useCallback(async () => {
    reset();
    await loadMore();
  }, [reset, loadMore]);

  return {
    data,
    loading,
    hasMore,
    loadMore,
    refresh,
    reset,
    totalItems,
  };
}

/**
 * 使用Intersection Observer实现无限滚动
 *
 * @param callback - 当触发滚动时调用的回调
 * @param options - Intersection Observer选项
 * @returns { ref, observer }
 *
 * @example
 * const { ref } = useInfiniteScrollTrigger(() => {
 *   loadMore()
 * })
 *
 * return <div ref={ref}>Loading more...</div>
 */
export function useInfiniteScrollTrigger(
  callback: () => void,
  options: IntersectionObserverInit = {},
) {
  const [element, setElement] = useState<HTMLElement | null>(null);
  const observer = useRef<IntersectionObserver | null>(null);

  useEffect(() => {
    if (!element) return;

    // 创建Intersection Observer
    observer.current = new IntersectionObserver(
      (entries) => {
        const [entry] = entries;
        if (entry.isIntersecting) {
          callback();
        }
      },
      {
        rootMargin: "200px",
        threshold: 0.1,
        ...options,
      },
    );

    // 开始观察
    observer.current.observe(element);

    // 清理
    return () => {
      if (observer.current) {
        observer.current.disconnect();
      }
    };
  }, [element, callback, options]);

  return {
    ref: setElement,
  };
}
