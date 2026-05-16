import { renderHook, waitFor, act } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { episodeApi } from "@/lib/api";
import type { Episode } from "@/types";
import { usePodcastEpisodes } from "../usePodcastEpisodes";

vi.mock("@/lib/api", () => ({
  episodeApi: {
    listByPodcast: vi.fn(),
  },
}));

const listByPodcast = vi.mocked(episodeApi.listByPodcast);

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

function makePage(episodes: Episode[], page: number, hasMore: boolean) {
  return {
    episodes,
    pagination: {
      page,
      page_size: 2,
      total: hasMore ? page * 2 + 1 : episodes.length,
      total_pages: hasMore ? page + 1 : page,
      has_more: hasMore,
    },
  };
}

function deferredPage() {
  let resolve!: (value: ReturnType<typeof makePage>) => void;
  const promise = new Promise<ReturnType<typeof makePage>>((innerResolve) => {
    resolve = innerResolve;
  });

  return { promise, resolve };
}

describe("usePodcastEpisodes", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("loads the first episode page when enabled", async () => {
    listByPodcast.mockResolvedValueOnce(makePage([makeEpisode(1)], 1, true));

    const { result } = renderHook(() =>
      usePodcastEpisodes({ podcastId: 1, enabled: true, pageSize: 2 }),
    );

    await waitFor(() => expect(result.current.episodesLoading).toBe(false));

    expect(listByPodcast).toHaveBeenCalledWith(1, 1, 2);
    expect(result.current.episodes.map((episode) => episode.id)).toEqual([1]);
    expect(result.current.totalEpisodes).toBe(3);
    expect(result.current.hasMoreEpisodes).toBe(true);
  });

  it("appends the next episode page", async () => {
    listByPodcast
      .mockResolvedValueOnce(makePage([makeEpisode(1)], 1, true))
      .mockResolvedValueOnce(makePage([makeEpisode(2)], 2, false));

    const { result } = renderHook(() =>
      usePodcastEpisodes({ podcastId: 1, enabled: true, pageSize: 2 }),
    );

    await waitFor(() => expect(result.current.episodesLoading).toBe(false));

    await act(async () => {
      await result.current.loadMoreEpisodes();
    });

    expect(listByPodcast).toHaveBeenNthCalledWith(2, 1, 2, 2);
    expect(result.current.episodes.map((episode) => episode.id)).toEqual([
      1, 2,
    ]);
    expect(result.current.hasMoreEpisodes).toBe(false);
  });

  it("ignores duplicate load-more requests while one page is in flight", async () => {
    const second = deferredPage();
    listByPodcast
      .mockResolvedValueOnce(makePage([makeEpisode(1)], 1, true))
      .mockReturnValueOnce(second.promise);

    const { result } = renderHook(() =>
      usePodcastEpisodes({ podcastId: 1, enabled: true, pageSize: 2 }),
    );

    await waitFor(() => expect(result.current.episodesLoading).toBe(false));

    await act(async () => {
      void result.current.loadMoreEpisodes();
      void result.current.loadMoreEpisodes();
    });

    expect(listByPodcast).toHaveBeenCalledTimes(2);
    expect(listByPodcast).toHaveBeenNthCalledWith(2, 1, 2, 2);

    await act(async () => {
      second.resolve(makePage([makeEpisode(2)], 2, false));
      await second.promise;
    });

    expect(result.current.episodes.map((episode) => episode.id)).toEqual([
      1, 2,
    ]);
  });

  it("deduplicates overlapping episodes when appending pages", async () => {
    listByPodcast
      .mockResolvedValueOnce(makePage([makeEpisode(1), makeEpisode(2)], 1, true))
      .mockResolvedValueOnce(makePage([makeEpisode(2), makeEpisode(3)], 2, false));

    const { result } = renderHook(() =>
      usePodcastEpisodes({ podcastId: 1, enabled: true, pageSize: 2 }),
    );

    await waitFor(() => expect(result.current.episodesLoading).toBe(false));

    await act(async () => {
      await result.current.loadMoreEpisodes();
    });

    expect(result.current.episodes.map((episode) => episode.id)).toEqual([
      1, 2, 3,
    ]);
  });

  it("does not request the next page before the first page settles", async () => {
    const first = deferredPage();
    listByPodcast.mockReturnValueOnce(first.promise);

    const { result } = renderHook(() =>
      usePodcastEpisodes({ podcastId: 1, enabled: true, pageSize: 2 }),
    );

    await waitFor(() => expect(listByPodcast).toHaveBeenCalledTimes(1));

    await act(async () => {
      await result.current.loadMoreEpisodes();
    });

    expect(listByPodcast).toHaveBeenCalledTimes(1);
    expect(listByPodcast).toHaveBeenCalledWith(1, 1, 2);

    await act(async () => {
      first.resolve(makePage([makeEpisode(1)], 1, true));
      await first.promise;
    });

    expect(result.current.episodes.map((episode) => episode.id)).toEqual([1]);
  });

  it("does not load when disabled", () => {
    renderHook(() =>
      usePodcastEpisodes({ podcastId: 1, enabled: false, pageSize: 2 }),
    );

    expect(listByPodcast).not.toHaveBeenCalled();
  });

  it("resets loaded episodes when disabled", async () => {
    listByPodcast.mockResolvedValueOnce(makePage([makeEpisode(1)], 1, false));

    const { result, rerender } = renderHook(
      ({ enabled }) =>
        usePodcastEpisodes({ podcastId: 1, enabled, pageSize: 2 }),
      { initialProps: { enabled: true } },
    );

    await waitFor(() => expect(result.current.episodesLoading).toBe(false));
    expect(result.current.episodes).toHaveLength(1);

    rerender({ enabled: false });

    expect(result.current.episodes).toEqual([]);
    expect(result.current.totalEpisodes).toBe(0);
    expect(result.current.episodesLoading).toBe(false);
  });

  it("ignores an outdated request after the podcast changes", async () => {
    const first = deferredPage();
    const second = deferredPage();
    listByPodcast
      .mockReturnValueOnce(first.promise)
      .mockReturnValueOnce(second.promise);

    const { result, rerender } = renderHook(
      ({ podcastId }) =>
        usePodcastEpisodes({ podcastId, enabled: true, pageSize: 2 }),
      { initialProps: { podcastId: 1 } },
    );

    rerender({ podcastId: 2 });

    await act(async () => {
      second.resolve(makePage([makeEpisode(2)], 1, false));
      await second.promise;
    });

    await waitFor(() =>
      expect(result.current.episodes.map((episode) => episode.id)).toEqual([2]),
    );

    await act(async () => {
      first.resolve(makePage([makeEpisode(1)], 1, false));
      await first.promise;
    });

    expect(result.current.episodes.map((episode) => episode.id)).toEqual([2]);
  });

  it("clears the first page when loading fails", async () => {
    const consoleSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    listByPodcast.mockRejectedValueOnce(new Error("network error"));

    const { result } = renderHook(() =>
      usePodcastEpisodes({ podcastId: 1, enabled: true, pageSize: 2 }),
    );

    await waitFor(() => expect(result.current.episodesLoading).toBe(false));

    expect(result.current.episodes).toEqual([]);
    expect(result.current.episodesError).toBe("network error");
    expect(result.current.hasMoreEpisodes).toBe(false);
    consoleSpy.mockRestore();
  });

  it("retries a failed first page request", async () => {
    const consoleSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    listByPodcast
      .mockRejectedValueOnce(new Error("network error"))
      .mockResolvedValueOnce(makePage([makeEpisode(1)], 1, false));

    const { result } = renderHook(() =>
      usePodcastEpisodes({ podcastId: 1, enabled: true, pageSize: 2 }),
    );

    await waitFor(() =>
      expect(result.current.episodesError).toBe("network error"),
    );

    await act(async () => {
      await result.current.retryEpisodes();
    });

    expect(listByPodcast).toHaveBeenCalledTimes(2);
    expect(result.current.episodes.map((episode) => episode.id)).toEqual([1]);
    expect(result.current.episodesError).toBeNull();
    consoleSpy.mockRestore();
  });

  it("keeps loaded episodes and stops auto-loading after loading more fails", async () => {
    const consoleSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    listByPodcast
      .mockResolvedValueOnce(makePage([makeEpisode(1)], 1, true))
      .mockRejectedValueOnce(new Error("page 2 failed"))
      .mockResolvedValueOnce(makePage([makeEpisode(2)], 2, false));

    const { result } = renderHook(() =>
      usePodcastEpisodes({ podcastId: 1, enabled: true, pageSize: 2 }),
    );

    await waitFor(() => expect(result.current.episodesLoading).toBe(false));

    await act(async () => {
      await result.current.loadMoreEpisodes();
    });

    expect(result.current.episodes.map((episode) => episode.id)).toEqual([1]);
    expect(result.current.episodesError).toBe("page 2 failed");
    expect(result.current.hasMoreEpisodes).toBe(false);

    await act(async () => {
      await result.current.retryEpisodes();
    });

    expect(listByPodcast).toHaveBeenNthCalledWith(3, 1, 2, 2);
    expect(result.current.episodes.map((episode) => episode.id)).toEqual([
      1, 2,
    ]);
    expect(result.current.episodesError).toBeNull();
    consoleSpy.mockRestore();
  });
});
