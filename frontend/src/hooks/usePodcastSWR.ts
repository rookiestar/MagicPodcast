import useSWR from "swr";
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
