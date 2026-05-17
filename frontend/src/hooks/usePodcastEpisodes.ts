import { useCallback, useEffect, useRef, useState } from "react";
import { episodeApi } from "@/lib/api";
import { getErrorMessage } from "@/lib/errorMessage";
import {
  applyEpisodePage,
  canLoadMoreEpisodes,
} from "@/lib/podcastEpisodePagination";
import type { Episode } from "@/types";

interface UsePodcastEpisodesOptions {
  podcastId: number;
  enabled: boolean;
  pageSize?: number;
}

export function usePodcastEpisodes({
  podcastId,
  enabled,
  pageSize = 20,
}: UsePodcastEpisodesOptions) {
  const [episodes, setEpisodes] = useState<Episode[]>([]);
  const [episodesLoading, setEpisodesLoading] = useState(false);
  const [currentPage, setCurrentPage] = useState(1);
  const [hasMoreEpisodes, setHasMoreEpisodes] = useState(true);
  const [totalEpisodes, setTotalEpisodes] = useState(0);
  const [isLoadingMore, setIsLoadingMore] = useState(false);
  const [episodesError, setEpisodesError] = useState<string | null>(null);
  const requestIdRef = useRef(0);
  const inFlightRequestIdRef = useRef<number | null>(null);
  const failedRequestRef = useRef<{
    page: number;
    append: boolean;
  } | null>(null);

  const resetEpisodes = useCallback(() => {
    setEpisodes([]);
    setEpisodesLoading(false);
    setCurrentPage(1);
    setHasMoreEpisodes(true);
    setTotalEpisodes(0);
    setIsLoadingMore(false);
    setEpisodesError(null);
    inFlightRequestIdRef.current = null;
    failedRequestRef.current = null;
  }, []);

  const fetchEpisodes = useCallback(
    async (page: number = 1, append: boolean = false) => {
      if (!enabled || !podcastId) return;
      if (inFlightRequestIdRef.current !== null) return;

      const requestId = requestIdRef.current + 1;
      requestIdRef.current = requestId;
      inFlightRequestIdRef.current = requestId;

      try {
        setEpisodesError(null);
        failedRequestRef.current = null;

        if (page === 1) {
          setEpisodesLoading(true);
        } else {
          setIsLoadingMore(true);
        }

        const { episodes: newEpisodes, pagination } =
          await episodeApi.listByPodcast(podcastId, page, pageSize);

        if (requestId !== requestIdRef.current) {
          return;
        }

        setEpisodes((prev) =>
          applyEpisodePage({
            existingEpisodes: prev,
            incomingEpisodes: newEpisodes,
            append,
          }),
        );

        setCurrentPage(pagination.page);
        setTotalEpisodes(pagination.total);
        setHasMoreEpisodes(pagination.has_more);
      } catch (err) {
        if (requestId !== requestIdRef.current) {
          return;
        }

        console.error("Failed to fetch episodes:", err);
        setEpisodesError(getErrorMessage(err));
        failedRequestRef.current = { page, append };
        if (!append) {
          setEpisodes([]);
          setCurrentPage(1);
          setTotalEpisodes(0);
          setHasMoreEpisodes(false);
        } else {
          setHasMoreEpisodes(false);
        }
      } finally {
        if (requestId === requestIdRef.current) {
          setEpisodesLoading(false);
          setIsLoadingMore(false);
        }
        if (inFlightRequestIdRef.current === requestId) {
          inFlightRequestIdRef.current = null;
        }
      }
    },
    [enabled, podcastId, pageSize],
  );

  const retryEpisodes = useCallback(async () => {
    if (episodesLoading || isLoadingMore) {
      return;
    }

    const failedRequest = failedRequestRef.current;
    if (failedRequest) {
      await fetchEpisodes(failedRequest.page, failedRequest.append);
      return;
    }

    await fetchEpisodes(1, false);
  }, [episodesLoading, fetchEpisodes, isLoadingMore]);

  const loadMoreEpisodes = useCallback(async () => {
    if (
      !canLoadMoreEpisodes({
        episodeCount: episodes.length,
        episodesLoading,
        isLoadingMore,
        hasMoreEpisodes,
      })
    ) {
      return;
    }

    await fetchEpisodes(currentPage + 1, true);
  }, [
    currentPage,
    episodes.length,
    episodesLoading,
    fetchEpisodes,
    hasMoreEpisodes,
    isLoadingMore,
  ]);

  useEffect(() => {
    requestIdRef.current += 1;

    if (!enabled || !podcastId) {
      resetEpisodes();
      return;
    }

    setEpisodes([]);
    setCurrentPage(1);
    setTotalEpisodes(0);
    setHasMoreEpisodes(true);
    setEpisodesError(null);
    inFlightRequestIdRef.current = null;
    failedRequestRef.current = null;
    fetchEpisodes(1, false);
  }, [enabled, fetchEpisodes, podcastId, pageSize, resetEpisodes]);

  return {
    episodes,
    episodesLoading,
    isLoadingMore,
    hasMoreEpisodes,
    totalEpisodes,
    episodesError,
    loadMoreEpisodes,
    retryEpisodes,
  };
}
