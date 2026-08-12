import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { SWRConfig } from "swr";
import { SearchProvider } from "@/contexts/SearchContext";
import type { Podcast } from "@/types";

vi.mock("@/components/layout/PageLayout", () => ({
  default: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));

vi.mock("@/hooks/useTagSWR", () => ({
  useTags: () => ({ tags: [] }),
}));

vi.mock("@/components/podcasts/ResponsivePodcastCard", () => ({
  default: ({ podcast }: { podcast: Podcast }) => (
    <div data-testid={`podcast-${podcast.id}`}>{podcast.title}</div>
  ),
}));

vi.mock("@tanstack/react-virtual", () => ({
  useWindowVirtualizer: ({ count }: { count: number }) => ({
    getVirtualItems: () =>
      Array.from({ length: count }, (_, index) => ({
        key: index,
        index,
        start: index * 100,
        end: (index + 1) * 100,
      })),
    getTotalSize: () => count * 100,
    measureElement: vi.fn(),
    options: { scrollMargin: 0 },
  }),
}));

import PodcastsContent from "../PodcastsContent";

function podcast(id: number): Podcast {
  return {
    id,
    xyz_id: `xyz-${id}`,
    title: `播客 ${id}`,
    description: "",
    author: "作者",
    cover_url: "",
    episode_count: 1,
    newest_episode_date: "2026-08-09T00:00:00Z",
    created_at: "2026-08-09T00:00:00Z",
    is_subscribed: true,
    is_dead: false,
  };
}

function initialPage(total: number) {
  return {
    podcasts: Array.from({ length: 10 }, (_, index) => podcast(index + 1)),
    pagination: {
      page: 1,
      page_size: 10,
      total,
      total_pages: Math.ceil(total / 10),
    },
  };
}

function pageResponse(
  page: number,
  pageSize: number,
  total: number,
  idOffset = 0,
): Response {
  const start = (page - 1) * pageSize;
  const podcasts = Array.from(
    { length: Math.min(pageSize, total - start) },
    (_, index) => podcast(idOffset + start + index + 1),
  );

  return new Response(
    JSON.stringify({
      success: true,
      data: podcasts,
      pagination: {
        page,
        page_size: pageSize,
        total,
        total_pages: Math.ceil(total / pageSize),
      },
    }),
    { status: 200, headers: { "Content-Type": "application/json" } },
  );
}

function requestParams(url: string) {
  const parsed = new URL(url, "http://localhost");
  return {
    page: Number(parsed.searchParams.get("page")),
    pageSize: Number(parsed.searchParams.get("page_size")),
  };
}

function callsFor(
  calls: Array<{ url: string; signal?: AbortSignal }>,
  pageSize: number,
  page: number,
) {
  return calls.filter(({ url }) => {
    const params = requestParams(url);
    return params.pageSize === pageSize && params.page === page;
  });
}

function setViewport(width: number) {
  Object.defineProperty(window, "innerWidth", {
    configurable: true,
    writable: true,
    value: width,
  });
}

function renderPodcastList(total: number) {
  return render(
    <SWRConfig
      value={{ provider: () => new Map(), revalidateOnFocus: false }}
    >
      <SearchProvider>
        <PodcastsContent initialPage={initialPage(total)} />
      </SearchProvider>
    </SWRConfig>,
  );
}

function renderedPodcastIds() {
  return screen
    .getAllByTestId(/^podcast-\d+$/)
    .map((element) => Number(element.dataset.testid?.replace("podcast-", "")));
}

describe("播客列表跨断点分页验收 (#110/#111)", () => {
  const calls: Array<{
    url: string;
    signal?: AbortSignal;
  }> = [];

  beforeEach(() => {
    calls.length = 0;
    window.history.replaceState({}, "", "/podcasts");
    setViewport(1280);
    Object.defineProperty(window, "innerHeight", {
      configurable: true,
      writable: true,
      value: 800,
    });
    Object.defineProperty(window, "scrollY", {
      configurable: true,
      writable: true,
      value: 0,
    });
    vi.stubGlobal(
      "requestAnimationFrame",
      (callback: FrameRequestCallback) => {
        callback(0);
        return 1;
      },
    );

    vi.stubGlobal(
      "fetch",
      vi.fn((input: string | URL, init?: RequestInit) => {
        const url = typeof input === "string" ? input : input.toString();
        const signal = init?.signal ?? undefined;
        calls.push({ url, signal });
        const { page, pageSize } = requestParams(url);

        if (pageSize === 15 && page === 2) {
          return new Promise<Response>((_resolve, reject) => {
            const rejectAbort = () =>
              reject(new DOMException("The user aborted a request", "AbortError"));
            if (signal?.aborted) {
              rejectAbort();
              return;
            }
            signal?.addEventListener("abort", rejectAbort, { once: true });
          });
        }

        return Promise.resolve(pageResponse(page, pageSize, 24));
      }),
    );
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("五列触底后缩至四列，会取消旧页并按新作用域继续到第二页", async () => {
    renderPodcastList(24);

    await waitFor(() => {
      expect(callsFor(calls, 15, 1)).toHaveLength(1);
      expect(screen.getByTestId("podcast-15")).toBeInTheDocument();
    });

    act(() => {
      window.scrollY = 1_000;
      fireEvent.scroll(window);
    });

    await waitFor(() => {
      expect(callsFor(calls, 15, 2)).toHaveLength(1);
    });

    act(() => {
      setViewport(1024);
      fireEvent.resize(window);
    });

    await waitFor(() => {
      expect(callsFor(calls, 12, 1)).toHaveLength(1);
    });

    await waitFor(() => {
      expect(callsFor(calls, 12, 2)).toHaveLength(1);
      expect(screen.getByTestId("podcast-24")).toBeInTheDocument();
    });

    const oldSecondPage = calls.find(({ url }) => {
      const params = requestParams(url);
      return params.pageSize === 15 && params.page === 2;
    });
    expect(oldSecondPage?.signal?.aborted).toBe(true);

    expect(renderedPodcastIds()).toEqual(
      Array.from({ length: 24 }, (_, index) => index + 1),
    );
  });
});

describe("播客列表响应式分页完整验收 (#112-#114)", () => {
  const calls: Array<{
    url: string;
    signal?: AbortSignal;
  }> = [];

  beforeEach(() => {
    calls.length = 0;
    window.history.replaceState({}, "", "/podcasts");
    Object.defineProperty(window, "innerHeight", {
      configurable: true,
      writable: true,
      value: 800,
    });
    Object.defineProperty(window, "scrollY", {
      configurable: true,
      writable: true,
      value: 0,
    });
    vi.stubGlobal(
      "requestAnimationFrame",
      (callback: FrameRequestCallback) => {
        callback(0);
        return 1;
      },
    );
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it.each([
    ["四列缩至三列", 1024, 900, 12, 9, 18],
    ["四列放大至五列", 1024, 1280, 12, 15, 30],
  ])(
    "%s后取消旧页，并按最终作用域继续分页",
    async (_name, startWidth, endWidth, startPageSize, endPageSize, total) => {
      setViewport(startWidth);
      vi.stubGlobal(
        "fetch",
        vi.fn((input: string | URL, init?: RequestInit) => {
          const url = typeof input === "string" ? input : input.toString();
          const signal = init?.signal ?? undefined;
          calls.push({ url, signal });
          const { page, pageSize } = requestParams(url);

          if (pageSize === startPageSize && page === 2) {
            return new Promise<Response>((_resolve, reject) => {
              const rejectAbort = () =>
                reject(
                  new DOMException("The user aborted a request", "AbortError"),
                );
              if (signal?.aborted) {
                rejectAbort();
                return;
              }
              signal?.addEventListener("abort", rejectAbort, { once: true });
            });
          }

          return Promise.resolve(pageResponse(page, pageSize, total));
        }),
      );

      renderPodcastList(total);
      await waitFor(() => {
        expect(callsFor(calls, startPageSize, 1)).toHaveLength(1);
        expect(
          screen.getByTestId(`podcast-${startPageSize}`),
        ).toBeInTheDocument();
      });

      act(() => {
        window.scrollY = 1_000;
        fireEvent.scroll(window);
      });
      await waitFor(() =>
        expect(callsFor(calls, startPageSize, 2)).toHaveLength(1),
      );

      act(() => {
        setViewport(endWidth);
        fireEvent.resize(window);
      });

      await waitFor(() => {
        expect(callsFor(calls, endPageSize, 1)).toHaveLength(1);
        expect(callsFor(calls, endPageSize, 2)).toHaveLength(1);
        expect(screen.getByTestId(`podcast-${total}`)).toBeInTheDocument();
      });

      expect(callsFor(calls, startPageSize, 2)[0]?.signal?.aborted).toBe(true);
      expect(renderedPodcastIds()).toEqual(
        Array.from({ length: total }, (_, index) => index + 1),
      );
    },
  );

  it("同一断点内 resize 不重建作用域或丢失已加载节目", async () => {
    const total = 24;
    setViewport(1024);
    vi.stubGlobal(
      "fetch",
      vi.fn((input: string | URL, init?: RequestInit) => {
        const url = typeof input === "string" ? input : input.toString();
        calls.push({ url, signal: init?.signal ?? undefined });
        const { page, pageSize } = requestParams(url);
        return Promise.resolve(pageResponse(page, pageSize, total));
      }),
    );

    renderPodcastList(total);
    await waitFor(() => expect(screen.getByTestId("podcast-12")).toBeInTheDocument());
    act(() => {
      window.scrollY = 1_000;
      fireEvent.scroll(window);
    });
    await waitFor(() => expect(screen.getByTestId("podcast-24")).toBeInTheDocument());

    const callsBeforeResize = calls.length;
    act(() => {
      setViewport(1190);
      fireEvent.resize(window);
    });

    await waitFor(() => expect(renderedPodcastIds()).toHaveLength(24));
    expect(calls).toHaveLength(callsBeforeResize);
    expect(callsFor(calls, 12, 1)).toHaveLength(1);
    expect(callsFor(calls, 12, 2)).toHaveLength(1);
  });

  it("跨断点 resize 保持当前排序和标签条件", async () => {
    const total = 18;
    window.history.replaceState(
      {},
      "",
      "/podcasts?sort_by=title&tag_id=7",
    );
    setViewport(1024);
    vi.stubGlobal(
      "fetch",
      vi.fn((input: string | URL, init?: RequestInit) => {
        const url = typeof input === "string" ? input : input.toString();
        calls.push({ url, signal: init?.signal ?? undefined });
        const { page, pageSize } = requestParams(url);
        return Promise.resolve(pageResponse(page, pageSize, total));
      }),
    );

    renderPodcastList(total);
    await waitFor(() => expect(callsFor(calls, 12, 1)).toHaveLength(1));
    act(() => {
      setViewport(900);
      fireEvent.resize(window);
    });
    await waitFor(() => expect(callsFor(calls, 9, 1)).toHaveLength(1));

    for (const { url } of calls) {
      const parsed = new URL(url, "http://localhost");
      expect(parsed.searchParams.get("sort_by")).toBe("title");
      expect(parsed.searchParams.getAll("tag_id")).toEqual(["7"]);
    }
    expect(window.location.search).toBe("?sort_by=title&tag_id=7");
  });

  it("快速连续跨断点时只允许最终作用域落地", async () => {
    const total = 18;
    let resolveIntermediate:
      | ((response: Response) => void)
      | undefined;
    setViewport(1280);
    vi.stubGlobal(
      "fetch",
      vi.fn((input: string | URL, init?: RequestInit) => {
        const url = typeof input === "string" ? input : input.toString();
        const signal = init?.signal ?? undefined;
        calls.push({ url, signal });
        const { page, pageSize } = requestParams(url);

        if (pageSize === 15 && page === 2) {
          return new Promise<Response>((_resolve, reject) => {
            signal?.addEventListener(
              "abort",
              () =>
                reject(
                  new DOMException("The user aborted a request", "AbortError"),
                ),
              { once: true },
            );
          });
        }
        if (pageSize === 12 && page === 1) {
          return new Promise<Response>((resolve) => {
            resolveIntermediate = resolve;
          });
        }
        return Promise.resolve(pageResponse(page, pageSize, total));
      }),
    );

    renderPodcastList(total);
    await waitFor(() => expect(screen.getByTestId("podcast-15")).toBeInTheDocument());
    act(() => {
      window.scrollY = 1_000;
      fireEvent.scroll(window);
    });
    await waitFor(() => expect(callsFor(calls, 15, 2)).toHaveLength(1));

    act(() => {
      setViewport(1024);
      fireEvent.resize(window);
    });
    await waitFor(() => expect(callsFor(calls, 12, 1)).toHaveLength(1));

    act(() => {
      setViewport(900);
      fireEvent.resize(window);
    });
    await waitFor(() => {
      expect(callsFor(calls, 9, 1)).toHaveLength(1);
      expect(callsFor(calls, 9, 2)).toHaveLength(1);
      expect(screen.getByTestId("podcast-18")).toBeInTheDocument();
    });

    expect(callsFor(calls, 12, 1)[0]?.signal?.aborted).toBe(true);
    act(() => resolveIntermediate?.(pageResponse(1, 12, total, 1_200)));

    await waitFor(() => {
      expect(screen.queryByTestId("podcast-1201")).toBeNull();
      expect(renderedPodcastIds()).toEqual(
        Array.from({ length: total }, (_, index) => index + 1),
      );
    });
  });

  it("慢首屏期间多次触底只合并一次，成功后兑现一次下一页", async () => {
    const total = 24;
    let resolveFirstPage: ((response: Response) => void) | undefined;
    setViewport(1024);
    vi.stubGlobal(
      "fetch",
      vi.fn((input: string | URL, init?: RequestInit) => {
        const url = typeof input === "string" ? input : input.toString();
        calls.push({ url, signal: init?.signal ?? undefined });
        const { page, pageSize } = requestParams(url);
        if (page === 1) {
          return new Promise<Response>((resolve) => {
            resolveFirstPage = resolve;
          });
        }
        return Promise.resolve(pageResponse(page, pageSize, total));
      }),
    );

    renderPodcastList(total);
    await waitFor(() => expect(callsFor(calls, 12, 1)).toHaveLength(1));
    act(() => {
      window.scrollY = 1_000;
      fireEvent.scroll(window);
      fireEvent.scroll(window);
      fireEvent.scroll(window);
    });
    expect(callsFor(calls, 12, 2)).toHaveLength(0);

    act(() => resolveFirstPage?.(pageResponse(1, 12, total)));
    await waitFor(() => {
      expect(callsFor(calls, 12, 2)).toHaveLength(1);
      expect(screen.getByTestId("podcast-24")).toBeInTheDocument();
    });
  });

  it("分页失败不循环请求，显式重试后恢复失败页", async () => {
    const total = 24;
    let failSecondPage = true;
    setViewport(1024);
    vi.stubGlobal(
      "fetch",
      vi.fn((input: string | URL, init?: RequestInit) => {
        const url = typeof input === "string" ? input : input.toString();
        calls.push({ url, signal: init?.signal ?? undefined });
        const { page, pageSize } = requestParams(url);
        if (page === 2 && failSecondPage) {
          return Promise.resolve(new Response("失败", { status: 500 }));
        }
        return Promise.resolve(pageResponse(page, pageSize, total));
      }),
    );

    renderPodcastList(total);
    await waitFor(() => expect(screen.getByTestId("podcast-12")).toBeInTheDocument());
    act(() => {
      window.scrollY = 1_000;
      fireEvent.scroll(window);
    });
    await waitFor(() => expect(screen.getByTestId("pagination-error")).toBeInTheDocument());
    expect(renderedPodcastIds()).toEqual(
      Array.from({ length: 12 }, (_, index) => index + 1),
    );

    act(() => {
      fireEvent.scroll(window);
      fireEvent.scroll(window);
      setViewport(1100);
      fireEvent.resize(window);
    });
    expect(callsFor(calls, 12, 2)).toHaveLength(1);

    failSecondPage = false;
    fireEvent.click(screen.getByRole("button", { name: "重试" }));
    await waitFor(() => expect(screen.getByTestId("podcast-24")).toBeInTheDocument());
    expect(callsFor(calls, 12, 2)).toHaveLength(2);
    expect(renderedPodcastIds()).toEqual(
      Array.from({ length: 24 }, (_, index) => index + 1),
    );
  });

  it("连续加载三页后在末页停止，节目无重复、缺口或错序", async () => {
    const total = 36;
    setViewport(1024);
    vi.stubGlobal(
      "fetch",
      vi.fn((input: string | URL, init?: RequestInit) => {
        const url = typeof input === "string" ? input : input.toString();
        calls.push({ url, signal: init?.signal ?? undefined });
        const { page, pageSize } = requestParams(url);
        return Promise.resolve(pageResponse(page, pageSize, total));
      }),
    );

    renderPodcastList(total);
    await waitFor(() => expect(screen.getByTestId("podcast-12")).toBeInTheDocument());
    act(() => {
      window.scrollY = 1_000;
      fireEvent.scroll(window);
    });
    await waitFor(() => expect(screen.getByTestId("podcast-36")).toBeInTheDocument());

    expect(callsFor(calls, 12, 1)).toHaveLength(1);
    expect(callsFor(calls, 12, 2)).toHaveLength(1);
    expect(callsFor(calls, 12, 3)).toHaveLength(1);
    expect(callsFor(calls, 12, 4)).toHaveLength(0);
    expect(renderedPodcastIds()).toEqual(
      Array.from({ length: total }, (_, index) => index + 1),
    );

    act(() => {
      fireEvent.scroll(window);
      fireEvent.scroll(window);
      fireEvent.resize(window);
    });
    expect(callsFor(calls, 12, 4)).toHaveLength(0);
  });
});
