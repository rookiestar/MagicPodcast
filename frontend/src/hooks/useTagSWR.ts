import useSWR from "swr";
import { fetcher } from "@/lib/fetcher";
import { swrConfig, cacheStrategies } from "@/lib/swrConfig";
import type { Tag } from "@/types";

// ============ 标签列表 Hook ============

export function useTags() {
  const { data, error, isLoading, mutate } = useSWR(
    "/api/v1/tags",
    () => fetcher<Tag[]>("/api/v1/tags"),
    { ...swrConfig, ...cacheStrategies.tags }
  );

  return {
    tags: data ?? [],
    isLoading,
    isError: !!error,
    error,
    mutate,
  };
}
