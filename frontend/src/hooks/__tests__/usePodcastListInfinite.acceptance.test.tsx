import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { SWRConfig } from "swr";
import { createElement } from "react";
import { usePodcastListInfinite } from "@/hooks/usePodcastSWR";

/**
 * 加载性能回归验收（PRD #11 / 子任务 #13）。
 *
 * 直接复现线上截图症状：封面灰块、页尾永久“加载更多…”、分页错误后整列表被替换。
 * 它们是 #12（无限滚动超时与恢复）和 #14（封面链路）修复的判定入口：
 *   - 修复前：失败，对应可见症状。
 *   - 修复后：通过。
 *
 * 本验收只驱动前端分页契约，不发起到真实后端的请求，也不写入数据库或触发同步/工作流。
 * 每个用例通过 SWRConfig provider 使用独立缓存，避免跨用例污染。
 */

const TOTAL = 20;

function pageFromUrl(url: string): { page: number; pageSize: number } {
  const parsed = new URL(url, "http://localhost");
  return {
    page: Number(parsed.searchParams.get("page") ?? "1"),
    pageSize: Number(parsed.searchParams.get("page_size") ?? "5"),
  };
}

function pageResponse(page: number, pageSize: number): Response {
  const totalPages = Math.ceil(TOTAL / pageSize);
  const start = (page - 1) * pageSize;
  // mock API 载荷，字段宽松；仅 id/title 参与断言。
  const data: Record<string, unknown>[] = [];
  for (let i = 0; i < pageSize && start + i < TOTAL; i += 1) {
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
      pagination: { page, page_size: pageSize, total: TOTAL, total_pages: totalPages },
    }),
    { status: 200, headers: { "Content-Type": "application/json" } },
  );
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
      // 模拟上游永不返回；必须尊重 AbortSignal，使有界超时能够中止请求。
      return new Promise<Response>((_resolve, reject) => {
        const signal = init?.signal;
        const onAbort = () => {
          reject(new DOMException("The user aborted a request", "AbortError"));
        };
        if (!signal) {
          return;
        }
        if (signal.aborted) {
          onAbort();
          return;
        }
        signal.addEventListener("abort", onAbort, { once: true });
      });
    }

    const err = controller.errorPages.get(page);
    if (err) {
      return Promise.resolve(errorResponse(err.status, err.message));
    }

    return Promise.resolve(pageResponse(page, pageSize));
  });
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

// 每个用例独立的 SWR 缓存，避免上一个用例的页面残留影响下一个。
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

const PAGE_SIZE = 5;

beforeEach(() => {
  vi.useFakeTimers({ shouldAdvanceTime: true });
});

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("播客无限滚动加载性能验收 (#13)", () => {
  it("首屏后可连续成功滚动至少三页", async () => {
    const controller: FetchController = {
      calls: [],
      hangPages: new Set(),
      errorPages: new Map(),
    };
    installFetch(controller);

    const { result } = renderInfiniteHook({ page_size: PAGE_SIZE });

    await waitFor(() => expect(result.current.podcasts.length).toBe(PAGE_SIZE));
    act(() => result.current.loadMore());
    await waitFor(() =>
      expect(result.current.podcasts.length).toBe(PAGE_SIZE * 2),
    );
    act(() => result.current.loadMore());
    await waitFor(() =>
      expect(result.current.podcasts.length).toBe(PAGE_SIZE * 3),
    );

    const ids = result.current.podcasts.map((p) => p.id);
    expect(new Set(ids).size).toBe(ids.length);
    expect(ids).toEqual([1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15]);
  });

  it("分页请求长时间不返回时，必须离开永久“加载更多…”状态", async () => {
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
      await vi.advanceTimersByTimeAsync(60_000);
    });

    expect(result.current.isLoadingMore).toBe(false);
    expect(result.current.isError).toBe(true);
  });

  it("后续分页失败时保留已加载节目，且可单独重试该页（不重复/不跳页）", async () => {
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
    expect(result.current.podcasts.map((p) => p.id)).toEqual([1, 2, 3, 4, 5]);

    expect(typeof result.current.retryLastPage).toBe("function");
    controller.errorPages.delete(2);

    await act(async () => {
      result.current.retryLastPage?.();
    });
    await waitFor(() =>
      expect(result.current.podcasts.length).toBe(PAGE_SIZE * 2),
    );

    const ids = result.current.podcasts.map((p) => p.id);
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

    const page2Calls = controller.calls.filter((u) => u.includes("page=2")).length;
    expect(page2Calls).toBe(1);
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
