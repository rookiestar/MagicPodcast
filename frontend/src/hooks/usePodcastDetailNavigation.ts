import { useCallback, useEffect, useRef } from "react";
import type { Episode } from "@/types";

const HIGHLIGHT_CLASSES = ["ring-2", "ring-blue-500", "ring-offset-2"];
const TARGET_SCROLL_SETTLE_DELAY_MS = 700;
const TARGET_VISIBLE_TOP_PADDING_PX = 96;
const TARGET_VISIBLE_BOTTOM_PADDING_PX = 32;

interface PodcastListBackUrlOptions {
  sortBy?: string | null;
  tagIds?: string | null;
}

interface UseTargetEpisodeNavigationOptions {
  targetEpisodeId: string | null;
  episodes: Episode[];
  episodesLoading: boolean;
  totalEpisodes: number;
  hasMoreEpisodes: boolean;
  isLoadingMore: boolean;
  loadMoreEpisodes: () => Promise<void>;
  scrollDelayMs?: number;
  highlightDurationMs?: number;
}

export function buildPodcastListBackUrl({
  sortBy,
  tagIds,
}: PodcastListBackUrlOptions) {
  const params = new URLSearchParams();

  if (sortBy) {
    params.append("sort_by", sortBy);
  }

  if (tagIds) {
    tagIds
      .split(",")
      .map((id) => id.trim())
      .filter(Boolean)
      .forEach((id) => {
        params.append("tag_id", id);
      });
  }

  const queryString = params.toString();
  return `/podcasts${queryString ? `?${queryString}` : ""}`;
}

export function parseTargetEpisodeId(value: string | null) {
  if (!value) return null;

  const id = Number(value);
  return Number.isInteger(id) && id > 0 ? id : null;
}

export function scrollEpisodeIntoView(element: HTMLElement) {
  const rect = element.getBoundingClientRect();
  const targetTop = Math.max(
    0,
    rect.top + window.scrollY - window.innerHeight / 2 + rect.height / 2,
  );

  try {
    if (typeof window.scrollTo === "function") {
      window.scrollTo(0, targetTop);
      document.documentElement.scrollTop = targetTop;
      document.body.scrollTop = targetTop;
      return;
    }
  } catch {
    // Fall through to more primitive scrolling methods.
  }

  try {
    if (typeof element.scrollIntoView === "function") {
      element.scrollIntoView({ behavior: "smooth", block: "center" });
      return;
    }
  } catch {
    // Fall through to direct scroll offsets.
  }

  try {
    document.documentElement.scrollTop = targetTop;
    document.body.scrollTop = targetTop;
    return;
  } catch {
    // Fall through to hash navigation.
  }

  try {
    window.location.hash = element.id;
  } catch {
    // Some test browsers expose read-only scroll primitives; highlighting still helps.
  }
}

export function isEpisodeElementVisible(element: HTMLElement) {
  const rect = element.getBoundingClientRect();
  const viewportHeight =
    window.innerHeight || document.documentElement.clientHeight;

  return (
    rect.bottom > TARGET_VISIBLE_TOP_PADDING_PX &&
    rect.top < viewportHeight - TARGET_VISIBLE_BOTTOM_PADDING_PX
  );
}

export function useTargetEpisodeNavigation({
  targetEpisodeId,
  episodes,
  episodesLoading,
  totalEpisodes,
  hasMoreEpisodes,
  isLoadingMore,
  loadMoreEpisodes,
  scrollDelayMs = 300,
  highlightDurationMs = 2000,
}: UseTargetEpisodeNavigationOptions) {
  const highlightedEpisodeIdRef = useRef<number | null>(null);
  const timersRef = useRef<ReturnType<typeof setTimeout>[]>([]);

  const clearTimers = useCallback(() => {
    timersRef.current.forEach((timer) => clearTimeout(timer));
    timersRef.current = [];
  }, []);

  useEffect(() => clearTimers, [clearTimers]);

  useEffect(() => {
    const targetEpisodeIdNum = parseTargetEpisodeId(targetEpisodeId);
    if (!targetEpisodeIdNum || episodesLoading) {
      return;
    }

    const targetExists = episodes.some(
      (episode) => episode.id === targetEpisodeIdNum,
    );

    if (!targetExists) {
      if (episodes.length === 0 && totalEpisodes === 0) {
        return;
      }

      if (hasMoreEpisodes && !isLoadingMore) {
        void loadMoreEpisodes();
      }
      return;
    }

    if (highlightedEpisodeIdRef.current === targetEpisodeIdNum) {
      return;
    }

    highlightedEpisodeIdRef.current = targetEpisodeIdNum;
    clearTimers();

    const scrollTimer = setTimeout(() => {
      const element = document.getElementById(`episode-${targetEpisodeIdNum}`);
      if (!element) return;

      scrollEpisodeIntoView(element);
      element.classList.add(...HIGHLIGHT_CLASSES);

      const settleTimer = setTimeout(() => {
        const settledElement = document.getElementById(
          `episode-${targetEpisodeIdNum}`,
        );
        if (settledElement && !isEpisodeElementVisible(settledElement)) {
          scrollEpisodeIntoView(settledElement);
        }
      }, TARGET_SCROLL_SETTLE_DELAY_MS);

      const highlightTimer = setTimeout(() => {
        element.classList.remove(...HIGHLIGHT_CLASSES);
      }, highlightDurationMs);

      timersRef.current.push(settleTimer);
      timersRef.current.push(highlightTimer);
    }, scrollDelayMs);

    timersRef.current.push(scrollTimer);
  }, [
    clearTimers,
    episodes,
    episodesLoading,
    hasMoreEpisodes,
    highlightDurationMs,
    isLoadingMore,
    loadMoreEpisodes,
    scrollDelayMs,
    targetEpisodeId,
    totalEpisodes,
  ]);
}
