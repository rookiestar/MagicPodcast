import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { SWRConfig } from "swr";
import { createElement } from "react";
import { usePodcastListInfinite } from "@/hooks/usePodcastSWR";
import type { Podcast } from "@/types";

/**
 * 播客列表请求与失败恢复验收（#65）。
 *
 * 只驱动前端分页契约，不请求真实后端，不写数据库，也不触发同步、工作流或付费能力。
 * 这些断言覆盖首屏、多页、超时、失败保留、重试、重复触发和分页收敛。
 */

const TOTAL = 20;
const PAGE_SIZE = 5;

function pageFromUrl(url: string): { page: number; pageSize: number } {
  const parsed = new URL(url, "http://localhost");
  return {
    page: Number(parsed.searchParams.get("page") ?? "1"),
    pageSize: Number(parsed.searchParams.get("page_size") ?? PAGE_SIZE),
  };
}

function pageResponse(
  page: number,
  pageSize: number,
  total = TOTAL,
): Response {
  const totalPages = Math.ceil(total / pageSize);
  const start = (page - 1) * pageSize;
  const data: Record<string, unknown>[] = [];
  for (let i = 0; i < pageSize && start + i < total; i += 1) {
    const id = start + i + 1;
    data.push({
      id,
      title: `播客 ${id}`,
      image_url: `https://i.typlog.com/cover-${id}.png`,
    });
  }
  return new Response(
    JSON.stringify({
      success: true,
      data,
      pagination: { page, page_size: pageSize, total, total_pages: totalPages },
    }),
    { status: 200, headers: { "Content-Type": "application/json" } },
  );
}

function initialPage(pageSize = 10, total = TOTAL) {
  return {
    podcasts: Array.from<unknown, Podcast>(
      { length: pageSize },
      (_, index) => ({
        id: index + 1,
        xyz_id: `xyz-${index + 1}`,
        title: `播客 ${index + 1}`,
        description: "",
        author: "作者",
        cover_url: "",
        episode_count: 1,
        newest_episode_date: "2026-08-09T00:00:00Z",
        created_at: "2026-08-09T00:00:00Z",
        is_subscribed: true,
        is_dead: false,
      }),
    ),
    pagination: {
      page: 1,
      page_size: pageSize,
      total,
      total_pages: Math.ceil(total / pageSize),
    },
  };
}

function errorResponse(status: number, message: string): Response {
  return new Response(
    JSON.stringify({ success: false, error: { message } }),
    { status, headers: { "Content-Type": "application/json" } },
  );
}

interface FetchController {
  calls: string[];
  hangPages: Set<number>;
  errorPages: Map<number, { status: number; message: string }>;
}

function installFetch(controller: FetchController) {
  const fetchMock = vi.fn((url: string | URL, init?: { signal?: AbortSignal }) => {
    const urlStr = typeof url === "string" ? url : url.toString();
    controller.calls.push(urlStr);
    const { page, pageSize } = pageFromUrl(urlStr);

    if (controller.hangPages.has(page)) {
      return new Promise<Response>((_resolve, reject) => {
        const signal = init?.signal;
        const onAbort = () => {
          reject(new DOMException("The user aborted a request", "AbortError"));
        };
        if (!signal) return;
        if (signal.aborted) {
          onAbort();
          return;
        }
        signal.addEventListener("abort", onAbort, { once: true });
      });
    }

    const err = controller.errorPages.get(page);
    if (err) return Promise.resolve(errorResponse(err.status, err.message));
    return Promise.resolve(pageResponse(page, pageSize));
  });
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

function renderInfiniteHook(props: Parameters<typeof usePodcastListInfinite>[0]) {
  return renderHook(() => usePodcastListInfinite(props), {
    wrapper: ({ children }) =>
      createElement(
        SWRConfig,
        { value: { provider: () => new Map(), revalidateOnFocus: false } },
        children,
      ),
  });
}

beforeEach(() => {
  vi.useFakeTimers({ shouldAdvanceTime: true });
});

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("播客无限滚动加载性能验收 (#65)", () => {
  it("移动端直接复用服务端 10 条，且不重复请求第一页", async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);

    const { result } = renderInfiniteHook({
      page_size: 10,
      initialPage: initialPage(10),
    });

    expect(result.current.podcasts).toHaveLength(10);
    expect(result.current.isLoading).toBe(false);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(100);
    });
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("桌面端先显示服务端 10 条，再用一次第一页请求补齐到 15 条", async () => {
    let resolveFirstPage: ((response: Response) => void) | undefined;
    const calls: string[] = [];
    const fetchMock = vi.fn((url: string | URL) => {
      calls.push(typeof url === "string" ? url : url.toString());
      return new Promise<Response>((resolve) => {
        resolveFirstPage = resolve;
      });
    });
    vi.stubGlobal("fetch", fetchMock);

    const { result } = renderInfiniteHook({
      page_size: 15,
      initialPage: initialPage(10),
    });

    expect(result.current.podcasts).toHaveLength(10);
    await waitFor(() => expect(calls).toHaveLength(1));
    expect(calls[0]).toContain("page=1");
    expect(calls[0]).toContain("page_size=15");

    act(() => resolveFirstPage?.(pageResponse(1, 15)));
    await waitFor(() => expect(result.current.podcasts).toHaveLength(15));

    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("四列首屏补齐期间触底，补齐成功后自动加载一次第二页", async () => {
    const total = 24;
    let resolveFirstPage: ((response: Response) => void) | undefined;
    const calls: string[] = [];
    const fetchMock = vi.fn((url: string | URL) => {
      const urlStr = typeof url === "string" ? url : url.toString();
      calls.push(urlStr);
      const { page, pageSize } = pageFromUrl(urlStr);
      if (page === 1) {
        return new Promise<Response>((resolve) => {
          resolveFirstPage = resolve;
        });
      }
      return Promise.resolve(pageResponse(page, pageSize, total));
    });
    vi.stubGlobal("fetch", fetchMock);

    const { result } = renderInfiniteHook({
      page_size: 12,
      initialPage: initialPage(10, total),
    });

    expect(result.current.podcasts).toHaveLength(10);
    await waitFor(() => expect(calls).toHaveLength(1));

    act(() => {
      result.current.loadMore();
      result.current.loadMore();
      result.current.loadMore();
    });
    expect(calls.filter((url) => url.includes("page=2"))).toHaveLength(0);

    act(() => resolveFirstPage?.(pageResponse(1, 12, total)));
    await waitFor(() => expect(result.current.podcasts).toHaveLength(24));

    expect(calls.filter((url) => url.includes("page=2"))).toHaveLength(1);
    const ids = result.current.podcasts.map((podcast) => podcast.id);
    expect(new Set(ids).size).toBe(24);
  });

  it("四列首屏补齐失败后，重试成功再兑现触底意图", async () => {
    const total = 24;
    let resolveFirstPage: ((response: Response) => void) | undefined;
    let shouldFailFirstPage = true;
    const calls: string[] = [];
    const fetchMock = vi.fn((url: string | URL) => {
      const urlStr = typeof url === "string" ? url : url.toString();
      calls.push(urlStr);
      const { page, pageSize } = pageFromUrl(urlStr);
      if (page === 1 && shouldFailFirstPage) {
        return new Promise<Response>((resolve) => {
          resolveFirstPage = resolve;
        });
      }
      return Promise.resolve(pageResponse(page, pageSize, total));
    });
    vi.stubGlobal("fetch", fetchMock);

    const { result } = renderInfiniteHook({
      page_size: 12,
      initialPage: initialPage(10, total),
    });

    await waitFor(() => expect(calls).toHaveLength(1));
    act(() => result.current.loadMore());
    act(() => resolveFirstPage?.(errorResponse(500, "服务器错误")));
    await waitFor(() => expect(result.current.isError).toBe(true));

    expect(result.current.podcasts).toHaveLength(10);
    expect(calls.filter((url) => url.includes("page=2"))).toHaveLength(0);

    shouldFailFirstPage = false;
    act(() => result.current.retryLastPage());
    await waitFor(() => expect(result.current.podcasts).toHaveLength(24));

    expect(calls.filter((url) => url.includes("page=2"))).toHaveLength(1);
    const ids = result.current.podcasts.map((podcast) => podcast.id);
    expect(new Set(ids).size).toBe(24);
  });

  it("响应式页大小变化时丢弃旧首屏的触底意图", async () => {
    const total = 24;
    const calls: string[] = [];
    const signals: AbortSignal[] = [];
    const fetchMock = vi.fn(
      (url: string | URL, init?: { signal?: AbortSignal }) => {
        const urlStr = typeof url === "string" ? url : url.toString();
        calls.push(urlStr);
        if (init?.signal) {
          signals.push(init.signal);
        }
        if (calls.length === 1) {
          return new Promise<Response>((_resolve, reject) => {
            init?.signal?.addEventListener(
              "abort",
              () =>
                reject(
                  new DOMException("The user aborted a request", "AbortError"),
                ),
              { once: true },
            );
          });
        }
        const { page, pageSize } = pageFromUrl(urlStr);
        return Promise.resolve(pageResponse(page, pageSize, total));
      },
    );
    vi.stubGlobal("fetch", fetchMock);

    const { result, rerender } = renderHook(
      (props: Parameters<typeof usePodcastListInfinite>[0]) =>
        usePodcastListInfinite(props),
      {
        initialProps: {
          page_size: 12,
          initialPage: initialPage(10, total),
        },
        wrapper: ({ children }) =>
          createElement(
            SWRConfig,
            { value: { provider: () => new Map(), revalidateOnFocus: false } },
            children,
          ),
      },
    );

    await waitFor(() => expect(calls).toHaveLength(1));
    act(() => result.current.loadMore());

    rerender({
      page_size: 9,
      initialPage: initialPage(10, total),
    });
    await waitFor(() => expect(result.current.podcasts).toHaveLength(9));

    expect(signals[0]?.aborted).toBe(true);
    expect(calls.filter((url) => url.includes("page=2"))).toHaveLength(0);
  });

  it("桌面补齐失败时保留服务端 10 条并允许重试", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue(errorResponse(500, "服务器错误"));
    vi.stubGlobal("fetch", fetchMock);

    const { result } = renderInfiniteHook({
      page_size: 15,
      initialPage: initialPage(10),
    });

    expect(result.current.podcasts).toHaveLength(10);
    await waitFor(() => expect(result.current.isError).toBe(true));

    expect(result.current.podcasts).toHaveLength(10);
    expect(result.current.error).toMatchObject({
      message: "播客列表请求失败（HTTP 500）",
    });
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("响应式页大小确定前不请求，确定后仅请求一次首屏", async () => {
    const controller: FetchController = {
      calls: [],
      hangPages: new Set(),
      errorPages: new Map(),
    };
    installFetch(controller);

    const { result, rerender } = renderHook(
      ({ ready }: { ready: boolean }) =>
        usePodcastListInfinite({
          enabled: ready,
          page_size: ready ? 10 : undefined,
        }),
      {
        initialProps: { ready: false },
        wrapper: ({ children }) =>
          createElement(
            SWRConfig,
            { value: { provider: () => new Map(), revalidateOnFocus: false } },
            children,
          ),
      },
    );

    await act(async () => {
      await vi.advanceTimersByTimeAsync(100);
    });
    expect(controller.calls).toEqual([]);

    rerender({ ready: true });
    await waitFor(() => expect(result.current.podcasts.length).toBe(10));

    expect(controller.calls).toHaveLength(1);
    expect(controller.calls[0]).toContain("page=1");
    expect(controller.calls[0]).toContain("page_size=10");
  });

  it("响应式页大小确定后的首请求不会被旧作用域清理误取消", async () => {
    const signals: AbortSignal[] = [];
    const fetchMock = vi.fn(
      (url: string | URL, init?: { signal?: AbortSignal }) =>
        new Promise<Response>((resolve, reject) => {
          const urlStr = typeof url === "string" ? url : url.toString();
          const { page, pageSize } = pageFromUrl(urlStr);
          const timer = setTimeout(
            () => resolve(pageResponse(page, pageSize)),
            50,
          );
          if (init?.signal) {
            signals.push(init.signal);
            init.signal.addEventListener(
              "abort",
              () => {
                clearTimeout(timer);
                reject(
                  new DOMException("The user aborted a request", "AbortError"),
                );
              },
              { once: true },
            );
          }
        }),
    );
    vi.stubGlobal("fetch", fetchMock);

    const { result, rerender } = renderHook(
      ({ ready }: { ready: boolean }) =>
        usePodcastListInfinite({
          enabled: ready,
          page_size: ready ? 10 : undefined,
        }),
      {
        initialProps: { ready: false },
        wrapper: ({ children }) =>
          createElement(
            SWRConfig,
            { value: { provider: () => new Map(), revalidateOnFocus: false } },
            children,
          ),
      },
    );

    rerender({ ready: true });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(50);
    });

    await waitFor(() => expect(result.current.podcasts.length).toBe(10));
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(signals[0]?.aborted).toBe(false);
  });

  it.each([
    [
      "排序",
      { page_size: PAGE_SIZE, sort_by: "recent_update" },
      { page_size: PAGE_SIZE, sort_by: "title" },
    ],
    [
      "标签",
      { page_size: PAGE_SIZE, tag_id: [1] },
      { page_size: PAGE_SIZE, tag_id: [2] },
    ],
    [
      "页大小",
      { page_size: PAGE_SIZE },
      { page_size: 10 },
    ],
    [
      "搜索",
      { page_size: PAGE_SIZE, search: "技术" },
      { page_size: PAGE_SIZE, search: "商业" },
    ],
    [
      "视图",
      { page_size: PAGE_SIZE, view: "summary" as const },
      { page_size: PAGE_SIZE, view: "full" as const },
    ],
  ])("%s变化时取消旧请求", async (_name, initialParams, nextParams) => {
    const calls: string[] = [];
    const signals: AbortSignal[] = [];
    const fetchMock = vi.fn(
      (url: string | URL, init?: { signal?: AbortSignal }) => {
        const urlStr = typeof url === "string" ? url : url.toString();
        calls.push(urlStr);
        if (init?.signal) {
          signals.push(init.signal);
        }

        if (calls.length === 1) {
          return new Promise<Response>((_resolve, reject) => {
            init?.signal?.addEventListener(
              "abort",
              () =>
                reject(
                  new DOMException("The user aborted a request", "AbortError"),
                ),
              { once: true },
            );
          });
        }

        const { page, pageSize } = pageFromUrl(urlStr);
        return Promise.resolve(pageResponse(page, pageSize));
      },
    );
    vi.stubGlobal("fetch", fetchMock);

    const { result, rerender } = renderHook(
      (params: Parameters<typeof usePodcastListInfinite>[0]) =>
        usePodcastListInfinite(params),
      {
        initialProps: initialParams,
        wrapper: ({ children }) =>
          createElement(
            SWRConfig,
            { value: { provider: () => new Map(), revalidateOnFocus: false } },
            children,
          ),
      },
    );

    await waitFor(() => expect(calls).toHaveLength(1));
    rerender(nextParams);
    await waitFor(() => expect(result.current.podcasts.length).toBeGreaterThan(0));

    expect(calls).toHaveLength(2);
    expect(signals[0]?.aborted).toBe(true);
  });

  it.each([502, 503, 504])(
    "HTTP %s 时自动重试一次并恢复首屏",
    async (status) => {
      const fetchMock = vi
        .fn()
        .mockResolvedValueOnce(errorResponse(status, "网关暂时异常"))
        .mockResolvedValueOnce(pageResponse(1, PAGE_SIZE));
      vi.stubGlobal("fetch", fetchMock);

      const { result } = renderInfiniteHook({ page_size: PAGE_SIZE });

      await waitFor(() =>
        expect(result.current.podcasts.length).toBe(PAGE_SIZE),
      );
      expect(fetchMock).toHaveBeenCalledTimes(2);
    },
  );

  it("网络错误时自动重试一次并恢复首屏", async () => {
    const fetchMock = vi
      .fn()
      .mockRejectedValueOnce(new TypeError("Failed to fetch"))
      .mockResolvedValueOnce(pageResponse(1, PAGE_SIZE));
    vi.stubGlobal("fetch", fetchMock);

    const { result } = renderInfiniteHook({ page_size: PAGE_SIZE });

    await waitFor(() => expect(result.current.podcasts.length).toBe(PAGE_SIZE));
    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(fetchMock).toHaveBeenNthCalledWith(
      1,
      expect.any(String),
      expect.objectContaining({ method: "GET" }),
    );
  });

  it("4xx 不自动重试", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue(errorResponse(400, "请求参数错误"));
    vi.stubGlobal("fetch", fetchMock);

    const { result } = renderInfiniteHook({ page_size: PAGE_SIZE });

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("可重试错误连续发生时也只自动重试一次", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue(errorResponse(503, "服务暂时不可用"));
    vi.stubGlobal("fetch", fetchMock);

    const { result } = renderInfiniteHook({ page_size: PAGE_SIZE });

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it("自动重试与首请求共用 15 秒总等待上限", async () => {
    const fetchMock = vi.fn(
      (_url: string | URL, init?: { signal?: AbortSignal }) => {
        if (fetchMock.mock.calls.length === 1) {
          return new Promise<Response>((_resolve, reject) => {
            setTimeout(
              () => reject(new TypeError("Failed to fetch")),
              14_000,
            );
          });
        }
        return new Promise<Response>((_resolve, reject) => {
          init?.signal?.addEventListener(
            "abort",
            () =>
              reject(
                new DOMException("The user aborted a request", "AbortError"),
              ),
            { once: true },
          );
        });
      },
    );
    vi.stubGlobal("fetch", fetchMock);

    const { result } = renderInfiniteHook({ page_size: PAGE_SIZE });

    await act(async () => {
      await vi.advanceTimersByTimeAsync(14_999);
    });
    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(result.current.isError).toBe(false);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1);
    });
    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(result.current.error).toMatchObject({
      message: "播客列表请求超时，请重试",
    });
  });

  it("首屏后可连续成功滚动至少三页且不重复节目", async () => {
    const controller: FetchController = {
      calls: [],
      hangPages: new Set(),
      errorPages: new Map(),
    };
    installFetch(controller);

    const { result } = renderInfiniteHook({ page_size: PAGE_SIZE });
    await waitFor(() => expect(result.current.podcasts.length).toBe(PAGE_SIZE));

    act(() => result.current.loadMore());
    await waitFor(() => expect(result.current.podcasts.length).toBe(PAGE_SIZE * 2));
    act(() => result.current.loadMore());
    await waitFor(() => expect(result.current.podcasts.length).toBe(PAGE_SIZE * 3));

    const ids = result.current.podcasts.map((podcast) => podcast.id);
    expect(new Set(ids).size).toBe(ids.length);
    expect(ids).toEqual([1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15]);
  });

  it("分页请求长时间不返回时，会在有界时间内离开加载状态", async () => {
    const controller: FetchController = {
      calls: [],
      hangPages: new Set([2]),
      errorPages: new Map(),
    };
    installFetch(controller);

    const { result } = renderInfiniteHook({ page_size: PAGE_SIZE });
    await waitFor(() => expect(result.current.podcasts.length).toBe(PAGE_SIZE));
    act(() => result.current.loadMore());

    await act(async () => {
      await vi.advanceTimersByTimeAsync(15_000);
    });

    expect(result.current.isLoadingMore).toBe(false);
    expect(result.current.isError).toBe(true);
  });

  it("分页失败时保留已加载节目，并可单独重试失败页", async () => {
    const controller: FetchController = {
      calls: [],
      hangPages: new Set(),
      errorPages: new Map([[2, { status: 500, message: "服务器错误" }]]),
    };
    installFetch(controller);

    const { result } = renderInfiniteHook({ page_size: PAGE_SIZE });
    await waitFor(() => expect(result.current.podcasts.length).toBe(PAGE_SIZE));
    act(() => result.current.loadMore());
    await waitFor(() => expect(result.current.isError).toBe(true));

    expect(result.current.podcasts.map((podcast) => podcast.id)).toEqual([1, 2, 3, 4, 5]);
    expect(typeof result.current.retryLastPage).toBe("function");

    controller.errorPages.delete(2);
    act(() => result.current.retryLastPage());
    await waitFor(() => expect(result.current.podcasts.length).toBe(PAGE_SIZE * 2));

    const ids = result.current.podcasts.map((podcast) => podcast.id);
    expect(new Set(ids).size).toBe(ids.length);
    expect(ids).toEqual([1, 2, 3, 4, 5, 6, 7, 8, 9, 10]);
  });

  it("连续快速触发不会重复请求同一页", async () => {
    const controller: FetchController = {
      calls: [],
      hangPages: new Set(),
      errorPages: new Map(),
    };
    installFetch(controller);

    const { result } = renderInfiniteHook({ page_size: PAGE_SIZE });
    await waitFor(() => expect(result.current.podcasts.length).toBe(PAGE_SIZE));

    act(() => {
      result.current.loadMore();
      result.current.loadMore();
      result.current.loadMore();
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1_000);
    });

    expect(controller.calls.filter((url) => url.includes("page=2")).length).toBe(1);
  });

  it("后台重新验证期间的触底不会消耗下一页意图", async () => {
    const calls: string[] = [];
    let holdRequests = false;
    const heldRequests: Array<{
      resolve: (response: Response) => void;
      page: number;
      pageSize: number;
    }> = [];
    vi.stubGlobal(
      "fetch",
      vi.fn((url: string | URL) => {
        const urlStr = typeof url === "string" ? url : url.toString();
        calls.push(urlStr);
        const { page, pageSize } = pageFromUrl(urlStr);
        if (holdRequests) {
          return new Promise<Response>((resolve) => {
            heldRequests.push({ resolve, page, pageSize });
          });
        }
        return Promise.resolve(pageResponse(page, pageSize));
      }),
    );

    const { result } = renderInfiniteHook({ page_size: PAGE_SIZE });
    await waitFor(() => expect(result.current.podcasts.length).toBe(PAGE_SIZE));
    act(() => result.current.loadMore());
    await waitFor(() =>
      expect(result.current.podcasts.length).toBe(PAGE_SIZE * 2),
    );

    holdRequests = true;
    act(() => {
      void result.current.mutate();
    });
    await waitFor(() => expect(result.current.isLoadingMore).toBe(true));
    act(() => result.current.loadMore());

    holdRequests = false;
    act(() => {
      heldRequests.forEach(({ resolve, page, pageSize }) =>
        resolve(pageResponse(page, pageSize)),
      );
    });
    await waitFor(() => expect(result.current.isLoadingMore).toBe(false));

    act(() => result.current.loadMore());
    await waitFor(() =>
      expect(result.current.podcasts.length).toBe(PAGE_SIZE * 3),
    );
    expect(calls.filter((url) => url.includes("page=3"))).toHaveLength(1);
  });

  it("成功加载全部节目后不再触发额外分页请求", async () => {
    const controller: FetchController = {
      calls: [],
      hangPages: new Set(),
      errorPages: new Map(),
    };
    installFetch(controller);

    const { result } = renderInfiniteHook({ page_size: 10 });
    await waitFor(() => expect(result.current.podcasts.length).toBe(10));
    act(() => result.current.loadMore());
    await waitFor(() => expect(result.current.podcasts.length).toBe(20));
    expect(result.current.hasMore).toBe(false);

    const callsBeforeEnd = controller.calls.length;
    act(() => result.current.loadMore());
    await act(async () => {
      await vi.advanceTimersByTimeAsync(2_000);
    });
    expect(controller.calls.length).toBe(callsBeforeEnd);
  });
});
