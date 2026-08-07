import { useCallback, useEffect, useRef } from "react";
import useSWR from "swr";
import useSWRInfinite from "swr/infinite";
import { fetcher } from "@/lib/fetcher";
import {
  buildPodcastDetailPath,
  buildPodcastListPath,
  buildPodcastNotesPath,
  buildPodcastTagsPath,
} from "@/lib/podcastApiPaths";
import {
  getPodcastListPageTotals,
  getUniquePodcastsFromPages,
  parsePodcastListApiPayload,
  shouldStopPodcastListPagination,
} from "@/lib/podcastListState";
import { swrConfig, cacheStrategies } from "@/lib/swrConfig";
import type { Podcast, Tag } from "@/types";

// ============ 播客无限滚动列表 Hook ============

// API 原始响应格式
interface PodcastListApiResponse {
  data: Podcast[];
  pagination: {
    page: number;
    page_size: number;
    total: number;
    total_pages: number;
  };
  success: boolean;
  error?: {
    message?: string;
  };
}

// fetcher 返回的格式（已解包 data 字段）
// 由于 API 响应是 {success, data: Podcast[], pagination}
// fetcher 提取 response.data.data，返回的是 Podcast[]
// 但我们需要 pagination 信息，所以需要自定义 fetcher

interface UsePodcastListParams {
  enabled?: boolean;
  page_size?: number;
  sort_by?: string;
  tag_id?: number[];
  search?: string;
  view?: "summary" | "full";
}

// 分页请求必须有明确终点，避免网络挂起时页尾永久停留在“加载更多…”。
const PODCAST_LIST_REQUEST_TIMEOUT_MS = 15_000;
const PODCAST_LIST_RETRYABLE_STATUSES = new Set([502, 503, 504]);

function isSameOriginUrl(url: string): boolean {
  if (url.startsWith("/") && !url.startsWith("//")) {
    return true;
  }
  if (typeof window === "undefined") {
    return false;
  }
  try {
    return new URL(url).origin === window.location.origin;
  } catch {
    return false;
  }
}

// 自定义 fetcher，返回完整的页面数据（包含 podcasts 和 pagination）
const podcastListFetcher = async (
  url: string,
  activeControllers?: Set<AbortController>,
): Promise<{ podcasts: Podcast[]; pagination: PodcastListApiResponse['pagination'] }> => {
  const controller = new AbortController();
  activeControllers?.add(controller);
  let didTimeOut = false;
  const timeoutId = setTimeout(() => {
    didTimeOut = true;
    controller.abort();
  }, PODCAST_LIST_REQUEST_TIMEOUT_MS);

  try {
    const canRetry = isSameOriginUrl(url);
    let didRetry = false;
    let response: Response;
    try {
      response = await fetch(url, {
        method: "GET",
        signal: controller.signal,
      });
    } catch (error) {
      if (
        canRetry &&
        !controller.signal.aborted &&
        error instanceof TypeError
      ) {
        didRetry = true;
        response = await fetch(url, {
          method: "GET",
          signal: controller.signal,
        });
      } else {
        throw error;
      }
    }
    if (
      canRetry &&
      !didRetry &&
      PODCAST_LIST_RETRYABLE_STATUSES.has(response.status)
    ) {
      response = await fetch(url, {
        method: "GET",
        signal: controller.signal,
      });
    }
    if (!response.ok) {
      throw new Error(`播客列表请求失败（HTTP ${response.status}）`);
    }
    const json: PodcastListApiResponse = await response.json();
    return parsePodcastListApiPayload(json);
  } catch (error) {
    if (didTimeOut) {
      throw new Error("播客列表请求超时，请重试");
    }
    throw error;
  } finally {
    clearTimeout(timeoutId);
    activeControllers?.delete(controller);
  }
};

/**
 * 播客无限滚动列表 Hook
 * 使用 SWR Infinite 实现分页缓存和自动去重
 */
export function usePodcastListInfinite(params: UsePodcastListParams = {}) {
  type PageData = { podcasts: Podcast[]; pagination: PodcastListApiResponse['pagination'] };
  const activeControllersRef = useRef(new Set<AbortController>());
  const { enabled = true, ...requestParams } = params;
  const requestScopeKey = `${enabled ? "enabled" : "disabled"}:${buildPodcastListPath({
    view: "summary",
    ...requestParams,
    page: 1,
  })}`;
  const fetchPodcastListPage = useCallback(
    (url: string) =>
      podcastListFetcher(url, activeControllersRef.current),
    [],
  );

  useEffect(() => {
    const activeControllers = activeControllersRef.current;
    return () => {
      activeControllers.forEach((controller) => controller.abort());
      activeControllers.clear();
    };
  }, [requestScopeKey]);

  const buildKey = (pageIndex: number, previousPageData: PageData | null) => {
    if (!enabled) {
      return null;
    }
    if (shouldStopPodcastListPagination(previousPageData)) {
      return null;
    }

    return buildPodcastListPath({
      view: "summary",
      ...requestParams,
      page: pageIndex + 1,
    });
  };

  const { data, error, isLoading, isValidating, size, setSize, mutate } = useSWRInfinite(
    buildKey,
    fetchPodcastListPage,
    {
      ...swrConfig,
      ...cacheStrategies.podcasts,
      revalidateFirstPage: false, // 不重新验证第一页
      revalidateAll: false, // 不重新验证所有页
      persistSize: false, // 不持久化 size
    }
  );

  // 扁平化所有页面的数据
  const podcasts = getUniquePodcastsFromPages<Podcast>(data);
  const { totalCount, totalPages, hasMore } = getPodcastListPageTotals(
    data,
    size,
  );
  const isLoadingMore = isValidating && size > 1;

  // 同一渲染周期内连续触发 loadMore 时，只允许一个请求把 size 加一。
  // SWR 自带的 key 去重无法阻止多个不同 size 更新在同一批次排队，因此这里需要同步锁。
  const loadMoreLockRef = useRef(false);

  useEffect(() => {
    if (!isValidating) {
      loadMoreLockRef.current = false;
    }
  }, [isValidating]);

  const loadMore = useCallback(() => {
    if (loadMoreLockRef.current || !hasMore) {
      return;
    }
    if (size > 1 && isValidating) {
      return;
    }
    loadMoreLockRef.current = true;
    setSize((currentSize) => currentSize + 1);
  }, [hasMore, isValidating, setSize, size]);

  // 失败页重试不清空已有页面，也不把分页回退到第一页。
  const retryLastPage = useCallback(() => {
    loadMoreLockRef.current = false;
    void mutate();
  }, [mutate]);

  const reset = useCallback(() => {
    loadMoreLockRef.current = false;
    setSize(1);
  }, [setSize]);

  return {
    podcasts,
    totalCount,
    totalPages,
    currentPage: size,
    hasMore,
    isLoading: isLoading && size === 1,
    isLoadingMore,
    isError: !!error,
    error,
    loadMore,
    retryLastPage,
    mutate,
    // 用于重置列表
    reset,
  };
}

// ============ 播客详情 Hook ============

export function usePodcast(id: number | null) {
  const key = id ? buildPodcastDetailPath(id) : null;

  const { data, error, isLoading, mutate } = useSWR(
    key,
    () => fetcher<Podcast>(buildPodcastDetailPath(id as number)),
    { ...swrConfig, ...cacheStrategies.podcastDetail }
  );

  return {
    podcast: data,
    isLoading,
    isError: !!error,
    error,
    mutate,
  };
}

// ============ 播客标签 Hook ============

export function usePodcastTags(podcastId: number | null) {
  const key = podcastId ? buildPodcastTagsPath(podcastId) : null;

  const { data, error, isLoading, mutate } = useSWR(
    key,
    () => fetcher<{ tags: Tag[] }>(buildPodcastTagsPath(podcastId as number)),
    { ...swrConfig, ...cacheStrategies.podcastDetail }
  );

  return {
    tags: data?.tags ?? [],
    isLoading,
    isError: !!error,
    error,
    mutate,
  };
}

// ============ 播客备注 Hook ============

export function usePodcastNotes(podcastId: number | null) {
  const key = podcastId ? buildPodcastNotesPath(podcastId) : null;

  const { data, error, isLoading, mutate } = useSWR(
    key,
    () =>
      fetcher<{ id: number; notes: string }>(
        buildPodcastNotesPath(podcastId as number),
      ),
    { ...swrConfig, ...cacheStrategies.podcastDetail }
  );

  return {
    notes: data?.notes ?? "",
    isLoading,
    isError: !!error,
    error,
    mutate,
  };
}
