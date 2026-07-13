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
  page_size?: number;
  sort_by?: string;
  tag_id?: number[];
  search?: string;
  view?: "summary" | "full";
}

// 分页请求的有界超时：超过该时长仍未返回时主动中止，避免页尾永久停留在“加载更多…”。
// 选取保守上限，兼顾弱网下的正常分页与卡死保护；不通过放宽该值让用例通过。
const PODCAST_LIST_REQUEST_TIMEOUT_MS = 15_000;

// 自定义 fetcher，返回完整的页面数据（包含 podcasts 和 pagination）
// 使用 AbortController 施加有界超时；超时后中止请求并把错误交给 SWR，
// 从而让 isLoadingMore 收敛到 false，页尾不再永久转圈。
const podcastListFetcher = async (url: string): Promise<{ podcasts: Podcast[]; pagination: PodcastListApiResponse['pagination'] }> => {
  const controller = new AbortController();
  const timeoutId = setTimeout(() => controller.abort(), PODCAST_LIST_REQUEST_TIMEOUT_MS);
  try {
    const response = await fetch(url, { signal: controller.signal });
    if (!response.ok) {
      throw new Error('Network response was not ok');
    }
    const json: PodcastListApiResponse = await response.json();
    return parsePodcastListApiPayload(json);
  } finally {
    clearTimeout(timeoutId);
  }
};

/**
 * 播客无限滚动列表 Hook
 * 使用 SWR Infinite 实现分页缓存和自动去重
 */
export function usePodcastListInfinite(params: UsePodcastListParams = {}) {
  type PageData = { podcasts: Podcast[]; pagination: PodcastListApiResponse['pagination'] };

  const buildKey = (pageIndex: number, previousPageData: PageData | null) => {
    if (shouldStopPodcastListPagination(previousPageData)) {
      return null;
    }

    return buildPodcastListPath({
      view: "summary",
      ...params,
      page: pageIndex + 1,
    });
  };

  const { data, error, isLoading, isValidating, size, setSize, mutate } = useSWRInfinite(
    buildKey,
    podcastListFetcher,
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

  // 用 ref 做同步防抖：同一渲染周期内连续触发的 loadMore 只允许一次 size 递增，
  // 避免快速滚动把同一页重复请求多次。SWR 完成本轮校验后释放锁。
  const loadMoreLockRef = useRef(false);

  useEffect(() => {
    if (!isValidating) {
      loadMoreLockRef.current = false;
    }
  }, [isValidating]);

  const loadMore = useCallback(() => {
    if (loadMoreLockRef.current) {
      return;
    }
    if (!hasMore) {
      return;
    }
    if (size > 1 && isValidating) {
      return;
    }
    loadMoreLockRef.current = true;
    setSize((currentSize) => currentSize + 1);
  }, [hasMore, isValidating, size, setSize]);

  // 单页重试：仅对失败的那一页重新校验，不重置 size、不清空已加载节目。
  // SWR 按 key 去重，已成功页面在去重窗口内不会重复发请求，只有失败页会重取。
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
