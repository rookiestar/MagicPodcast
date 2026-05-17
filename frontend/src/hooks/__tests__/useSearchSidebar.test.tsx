import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { searchApi } from "@/lib/api";
import type { SearchData } from "@/types";
import { filterSearchResults, normalizeSearchData } from "@/lib/searchSidebarState";
import {
  addToSearchHistory,
  getSearchHistory,
  useSearchSidebar,
} from "../useSearchSidebar";

vi.mock("@/lib/api", () => ({
  searchApi: {
    search: vi.fn(),
  },
}));

const search = vi.mocked(searchApi.search);
const memoryStorage = new Map<string, string>();

function installLocalStorageMock() {
  memoryStorage.clear();

  const localStorageMock = {
    getItem: vi.fn((key: string) => memoryStorage.get(key) ?? null),
    setItem: vi.fn((key: string, value: string) => {
      memoryStorage.set(key, value);
    }),
    removeItem: vi.fn((key: string) => {
      memoryStorage.delete(key);
    }),
    clear: vi.fn(() => {
      memoryStorage.clear();
    }),
  };

  Object.defineProperty(globalThis, "localStorage", {
    value: localStorageMock,
    configurable: true,
  });
  Object.defineProperty(window, "localStorage", {
    value: localStorageMock,
    configurable: true,
  });
}

const emptyPagination = {
  page: 1,
  page_size: 50,
  total: 0,
  total_pages: 0,
};

function makeSearchData({
  podcastTitle = "科技播客",
  episodeTitle = "科技单集",
}: {
  podcastTitle?: string;
  episodeTitle?: string;
} = {}): SearchData {
  return {
    podcasts: [
      {
        id: 1,
        title: podcastTitle,
        author: "作者",
        description: "简介",
        cover_url: "",
        episode_count: 2,
        newest_episode_date: "2026-01-01T00:00:00Z",
        relevance_score: 10,
      },
    ],
    episodes: [
      {
        id: 10,
        podcast_id: 1,
        podcast_title: podcastTitle,
        podcast_cover_url: "",
        title: episodeTitle,
        show_notes: "内容",
        published_date: null,
        duration: 0,
        relevance_score: 8,
      },
    ],
    pagination: {
      podcasts: emptyPagination,
      episodes: emptyPagination,
    },
  };
}

function deferred<T>() {
  let resolve: (value: T) => void = () => {};
  let reject: (error: unknown) => void = () => {};
  const promise = new Promise<T>((promiseResolve, promiseReject) => {
    resolve = promiseResolve;
    reject = promiseReject;
  });

  return { promise, resolve, reject };
}

describe("search history helpers", () => {
  beforeEach(() => {
    installLocalStorageMock();
  });

  it("deduplicates and caps search history", () => {
    ["a", "b", "c", "d", "e", "f"].forEach(addToSearchHistory);

    expect(addToSearchHistory("c")).toEqual(["c", "f", "e", "d", "b", "a"]);
    expect(addToSearchHistory("g")).toEqual(["g", "c", "f", "e", "d", "b"]);
    expect(getSearchHistory()).toEqual(["g", "c", "f", "e", "d", "b"]);
  });

  it("ignores invalid local storage content", () => {
    localStorage.setItem("podcast_search_history", '{"bad":true}');

    expect(getSearchHistory()).toEqual([]);
  });
});

describe("filterSearchResults", () => {
  it("filters cached results by selected type", () => {
    const data = {
      ...makeSearchData(),
      pagination: null,
    };

    expect(filterSearchResults(data, "podcasts")).toEqual({
      podcasts: data.podcasts,
      episodes: [],
    });
    expect(filterSearchResults(data, "episodes")).toEqual({
      podcasts: [],
      episodes: data.episodes,
    });
  });
});

describe("normalizeSearchData", () => {
  it("normalizes missing result arrays without crashing", () => {
    expect(
      normalizeSearchData({
        pagination: null,
      } as unknown as SearchData),
    ).toEqual({
      podcasts: [],
      episodes: [],
      pagination: null,
    });
  });
});

describe("useSearchSidebar", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.clearAllMocks();
    installLocalStorageMock();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("waits for the minimum query length before searching", async () => {
    const { result } = renderHook(() =>
      useSearchSidebar({ isOpen: true, debounceMs: 10 }),
    );

    act(() => {
      result.current.setQuery("a");
    });

    await act(async () => {
      await vi.advanceTimersByTimeAsync(10);
    });

    expect(search).not.toHaveBeenCalled();
    expect(result.current.hasResults).toBe(false);
  });

  it("uses the trimmed query when checking the minimum length", async () => {
    const { result } = renderHook(() =>
      useSearchSidebar({ isOpen: true, debounceMs: 10 }),
    );

    act(() => {
      result.current.setQuery("  a  ");
    });

    await act(async () => {
      await vi.advanceTimersByTimeAsync(10);
    });

    expect(search).not.toHaveBeenCalled();
    expect(result.current.isQueryTooShort).toBe(true);
  });

  it("searches all result types and normalizes matched fields", async () => {
    search.mockResolvedValueOnce({ data: makeSearchData() });

    const { result } = renderHook(() =>
      useSearchSidebar({ isOpen: true, debounceMs: 10 }),
    );

    act(() => {
      result.current.setQuery(" 科技 ");
    });

    await act(async () => {
      await vi.advanceTimersByTimeAsync(10);
    });

    expect(search).toHaveBeenCalledWith(
      {
        q: "科技",
        type: "all",
        page: 1,
        page_size: 50,
        episode_page: 1,
        episode_page_size: 50,
      },
      { signal: expect.any(AbortSignal) },
    );
    expect(result.current.results.podcasts[0].matched_fields).toEqual([]);
    expect(result.current.results.episodes[0].matched_fields).toEqual([]);
    expect(result.current.searchHistory).toEqual(["科技"]);
  });

  it("switches visible result type without searching again", async () => {
    search.mockResolvedValueOnce({ data: makeSearchData() });

    const { result } = renderHook(() =>
      useSearchSidebar({ isOpen: true, debounceMs: 10 }),
    );

    act(() => {
      result.current.setQuery("科技");
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(10);
    });

    act(() => {
      result.current.setSearchType("episodes");
    });

    expect(search).toHaveBeenCalledTimes(1);
    expect(result.current.results.podcasts).toEqual([]);
    expect(result.current.results.episodes).toHaveLength(1);
  });

  it("does not let stale responses overwrite newer results", async () => {
    const first = deferred<{ data: SearchData }>();
    const second = deferred<{ data: SearchData }>();
    search.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);

    const { result } = renderHook(() =>
      useSearchSidebar({ isOpen: true, debounceMs: 10 }),
    );

    act(() => {
      result.current.setQuery("first");
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(10);
    });

    act(() => {
      result.current.setQuery("second");
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(10);
    });

    await act(async () => {
      second.resolve({ data: makeSearchData({ podcastTitle: "second" }) });
      await Promise.resolve();
    });

    expect(result.current.results.podcasts[0].title).toBe("second");

    await act(async () => {
      first.resolve({ data: makeSearchData({ podcastTitle: "first" }) });
      await Promise.resolve();
    });

    expect(result.current.results.podcasts[0].title).toBe("second");
  });

  it("aborts the in-flight request when the query changes", async () => {
    const first = deferred<{ data: SearchData }>();
    const second = deferred<{ data: SearchData }>();
    search.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);

    const { result } = renderHook(() =>
      useSearchSidebar({ isOpen: true, debounceMs: 10 }),
    );

    act(() => {
      result.current.setQuery("first");
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(10);
    });

    const firstSignal = search.mock.calls[0][1]?.signal;
    expect(firstSignal?.aborted).toBe(false);

    act(() => {
      result.current.setQuery("second");
    });

    expect(firstSignal?.aborted).toBe(true);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(10);
      second.resolve({ data: makeSearchData({ podcastTitle: "second" }) });
      await Promise.resolve();
    });

    expect(result.current.results.podcasts[0].title).toBe("second");
  });

  it("ignores canceled searches without showing an error", async () => {
    const canceledError = new Error("canceled");
    canceledError.name = "CanceledError";
    search.mockRejectedValueOnce(canceledError);

    const { result } = renderHook(() =>
      useSearchSidebar({ isOpen: true, debounceMs: 10 }),
    );

    act(() => {
      result.current.setQuery("科技");
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(10);
    });

    expect(result.current.loading).toBe(false);
    expect(result.current.searchError).toBeNull();
    expect(result.current.results).toEqual({ podcasts: [], episodes: [] });
  });

  it("clears results and loading state when search fails", async () => {
    const consoleSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    search.mockRejectedValueOnce(new Error("network error"));

    const { result } = renderHook(() =>
      useSearchSidebar({ isOpen: true, debounceMs: 10 }),
    );

    act(() => {
      result.current.setQuery("科技");
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(10);
    });

    expect(result.current.loading).toBe(false);
    expect(result.current.results).toEqual({ podcasts: [], episodes: [] });
    expect(result.current.searchError).toBe("搜索失败，请稍后重试");

    consoleSpy.mockRestore();
  });
});
