import { useCallback } from "react";
import useSWR from "swr";
import useSWRInfinite from "swr/infinite";
import { fetcher } from "@/lib/fetcher";
import { swrConfig, cacheStrategies } from "@/lib/swrConfig";
import type { Podcast, Tag, Episode } from "@/types";

// ============ 播客列表 Hook ============

interface UsePodcastsParams {
  page?: number;
  page_size?: number;
  sort_by?: string;
  tag_id?: number[];
  search?: string;
}

export function usePodcasts(params: UsePodcastsParams = {}) {
  const queryParams = new URLSearchParams();

  if (params.page) queryParams.set("page", params.page.toString());
  if (params.page_size) queryParams.set("page_size", params.page_size.toString());
  if (params.sort_by) queryParams.set("sort_by", params.sort_by);
  if (params.search) queryParams.set("search", params.search);
  if (params.tag_id && params.tag_id.length > 0) {
    params.tag_id.forEach((id) => queryParams.append("tag_id", id.toString()));
  }

  const key = queryParams.toString()
    ? `/api/v1/podcasts?${queryParams.toString()}`
    : "/api/v1/podcasts";

  const { data, error, isLoading, mutate } = useSWR(
    key,
    () =>
      fetcher<{
        data: Podcast[];
        pagination: {
          page: number;
          page_size: number;
          total: number;
          total_pages: number;
        };
      }>(key),
    { ...swrConfig, ...cacheStrategies.podcasts }
  );

  return {
    podcasts: data?.data ?? [],
    pagination: data?.pagination,
    isLoading,
    isError: !!error,
    error,
    mutate,
  };
}

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
}

// 自定义 fetcher，返回完整的页面数据（包含 podcasts 和 pagination）
const podcastListFetcher = async (url: string): Promise<{ podcasts: Podcast[]; pagination: PodcastListApiResponse['pagination'] }> => {
  const response = await fetch(url);
  if (!response.ok) {
    throw new Error('Network response was not ok');
  }
  const json: PodcastListApiResponse = await response.json();
  return {
    podcasts: json.data ?? [],
    pagination: json.pagination,
  };
};

/**
 * 播客无限滚动列表 Hook
 * 使用 SWR Infinite 实现分页缓存和自动去重
 */
export function usePodcastListInfinite(params: UsePodcastListParams = {}) {
  type PageData = { podcasts: Podcast[]; pagination: PodcastListApiResponse['pagination'] };

  const buildKey = (pageIndex: number, previousPageData: PageData | null) => {
    // 如果已到达末尾，返回 null 停止加载
    if (previousPageData && !previousPageData.podcasts.length) return null;
    if (previousPageData && previousPageData.pagination.page >= previousPageData.pagination.total_pages) {
      return null;
    }

    const queryParams = new URLSearchParams();
    queryParams.set("page", String(pageIndex + 1));
    if (params.page_size) queryParams.set("page_size", params.page_size.toString());
    if (params.sort_by) queryParams.set("sort_by", params.sort_by);
    if (params.search) queryParams.set("search", params.search);
    if (params.tag_id && params.tag_id.length > 0) {
      params.tag_id.forEach((id) => queryParams.append("tag_id", id.toString()));
    }

    return `/api/v1/podcasts?${queryParams.toString()}`;
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
  const podcasts = data?.flatMap((page) => page.podcasts ?? []).filter((p): p is Podcast => p != null && p.id != null) ?? [];
  const totalCount = data?.[0]?.pagination?.total ?? 0;
  const totalPages = data?.[0]?.pagination?.total_pages ?? 0;
  const hasMore = size < totalPages;
  const isLoadingMore = isValidating && size > 1;

  const loadMore = useCallback(() => {
    if (!isLoadingMore && hasMore) {
      setSize((currentSize) => currentSize + 1);
    }
  }, [hasMore, isLoadingMore, setSize]);

  const reset = useCallback(() => {
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
    mutate,
    // 用于重置列表
    reset,
  };
}

// ============ 播客详情 Hook ============

export function usePodcast(id: number | null) {
  const { data, error, isLoading, mutate } = useSWR(
    id ? `/api/v1/podcasts/${id}` : null,
    () => fetcher<Podcast>(`/api/v1/podcasts/${id}`),
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
  const { data, error, isLoading, mutate } = useSWR(
    podcastId ? `/api/v1/podcasts/${podcastId}/tags` : null,
    () => fetcher<{ tags: Tag[] }>(`/api/v1/podcasts/${podcastId}/tags`),
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
  const { data, error, isLoading, mutate } = useSWR(
    podcastId ? `/api/v1/podcasts/${podcastId}/notes` : null,
    () => fetcher<{ id: number; notes: string }>(`/api/v1/podcasts/${podcastId}/notes`),
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

// ============ 单集列表 Hook ============

export function useEpisodes(
  podcastId: number | null,
  page: number = 1,
  pageSize: number = 20
) {
  const key = podcastId
    ? `/api/v1/podcasts/${podcastId}/episodes?page=${page}&page_size=${pageSize}`
    : null;

  const { data, error, isLoading, mutate } = useSWR(
    key,
    () =>
      fetcher<{
        episodes: Episode[];
        pagination: {
          page: number;
          page_size: number;
          total: number;
          total_pages: number;
          has_more: boolean;
        };
      }>(
        `/api/v1/podcasts/${podcastId}/episodes?page=${page}&page_size=${pageSize}`
      ),
    { ...swrConfig, ...cacheStrategies.episodes }
  );

  return {
    episodes: data?.episodes ?? [],
    pagination: data?.pagination,
    isLoading,
    isError: !!error,
    error,
    mutate,
  };
}
