import { renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Episode } from "@/types";
import {
  buildPodcastListBackUrl,
  getTargetEpisodeNavigationAction,
  parseTargetEpisodeId,
  useTargetEpisodeNavigation,
} from "../usePodcastDetailNavigation";

function makeEpisode(id: number): Episode {
  return {
    id,
    guid: `episode-${id}`,
    podcast_id: 1,
    episode_no: "",
    title: `Episode ${id}`,
    medium_url: "",
    show_notes: "",
    published_date: "2026-01-01T00:00:00Z",
    duration: 0,
    link: "",
    image_url: "",
    enclosure_type: "",
    enclosure_length: 0,
    my_rate: 0,
    notes: "",
  };
}

describe("podcast detail navigation", () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
    document.body.innerHTML = "";
    vi.restoreAllMocks();
  });

  it("builds the podcast list back URL from list filters", () => {
    expect(
      buildPodcastListBackUrl({ sortBy: "newest", tagIds: "1, 2,,3" }),
    ).toBe("/podcasts?sort_by=newest&tag_id=1&tag_id=2&tag_id=3");
    expect(buildPodcastListBackUrl({ sortBy: "", tagIds: null })).toBe(
      "/podcasts",
    );
  });

  it("parses only valid target episode ids", () => {
    expect(parseTargetEpisodeId("42")).toBe(42);
    expect(parseTargetEpisodeId("0")).toBeNull();
    expect(parseTargetEpisodeId("abc")).toBeNull();
    expect(parseTargetEpisodeId(null)).toBeNull();
  });

  it("decides target episode navigation actions", () => {
    expect(
      getTargetEpisodeNavigationAction({
        targetEpisodeId: "2",
        episodes: [makeEpisode(1)],
        episodesLoading: false,
        totalEpisodes: 3,
        hasMoreEpisodes: true,
        isLoadingMore: false,
      }),
    ).toBe("load-more");

    expect(
      getTargetEpisodeNavigationAction({
        targetEpisodeId: "2",
        episodes: [makeEpisode(2)],
        episodesLoading: false,
        totalEpisodes: 3,
        hasMoreEpisodes: true,
        isLoadingMore: false,
      }),
    ).toBe("focus");

    expect(
      getTargetEpisodeNavigationAction({
        targetEpisodeId: "2",
        episodes: [],
        episodesLoading: false,
        totalEpisodes: 0,
        hasMoreEpisodes: false,
        isLoadingMore: false,
      }),
    ).toBeNull();
  });

  it("loads another page when the target episode is not loaded yet", () => {
    const loadMoreEpisodes = vi.fn(() => Promise.resolve());

    renderHook(() =>
      useTargetEpisodeNavigation({
        targetEpisodeId: "2",
        episodes: [makeEpisode(1)],
        episodesLoading: false,
        totalEpisodes: 3,
        hasMoreEpisodes: true,
        isLoadingMore: false,
        loadMoreEpisodes,
      }),
    );

    expect(loadMoreEpisodes).toHaveBeenCalledTimes(1);
  });

  it("scrolls to and highlights a loaded target episode", () => {
    const loadMoreEpisodes = vi.fn(() => Promise.resolve());
    const scrollTo = vi.fn();
    const target = document.createElement("div");
    target.id = "episode-2";
    target.scrollIntoView = vi.fn();
    target.getBoundingClientRect = vi.fn(() => ({
      top: 1000,
      bottom: 1100,
      left: 0,
      right: 100,
      width: 100,
      height: 100,
      x: 0,
      y: 1000,
      toJSON: () => ({}),
    }));
    Object.defineProperty(window, "innerHeight", {
      configurable: true,
      value: 800,
    });
    Object.defineProperty(window, "scrollY", {
      configurable: true,
      value: 0,
    });
    Object.defineProperty(window, "scrollTo", {
      configurable: true,
      value: scrollTo,
    });
    document.body.appendChild(target);

    renderHook(() =>
      useTargetEpisodeNavigation({
        targetEpisodeId: "2",
        episodes: [makeEpisode(2)],
        episodesLoading: false,
        totalEpisodes: 1,
        hasMoreEpisodes: false,
        isLoadingMore: false,
        loadMoreEpisodes,
        scrollDelayMs: 10,
        highlightDurationMs: 20,
      }),
    );

    expect(target.classList.contains("ring-2")).toBe(false);

    vi.advanceTimersByTime(10);

    expect(scrollTo).toHaveBeenCalledWith(0, 650);
    expect(target.classList.contains("ring-2")).toBe(true);

    vi.advanceTimersByTime(20);

    expect(target.classList.contains("ring-2")).toBe(false);
  });
});
