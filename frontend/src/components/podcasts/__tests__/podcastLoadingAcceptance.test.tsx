import { act, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import PodcastCover from "@/components/podcasts/PodcastCover";
import PodcastListResults from "@/components/podcasts/PodcastListResults";
import type { PodcastSortBy } from "@/lib/podcastListState";
import type { Podcast } from "@/types";

beforeEach(() => {
  vi.useFakeTimers({ shouldAdvanceTime: true });
});

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("封面加载收敛验收 (#13/#14)", () => {
  it("32px 代理封面交给响应式优化器生成尺寸候选", () => {
    render(
      <PodcastCover
        coverUrl="https://i.typlog.com/cover-32.png"
        title="工作流小图"
        index={0}
        priority="high"
        sizes="32px"
      />,
    );

    const image = screen.getByRole("img", { name: "工作流小图" });
    expect(image).toHaveAttribute("sizes", "32px");
    expect(image).toHaveAttribute("data-optimized", "true");
    expect(image.getAttribute("src")).toContain("/images/proxy?url=");
  });

  it("嵌套滚动容器内首次可见的小图无需交互即可挂载", () => {
    vi.stubGlobal(
      "IntersectionObserver",
      class {
        observe() {}
        disconnect() {}
      },
    );
    vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockImplementation(
      function getBoundingClientRect() {
        if (this.dataset.scrollRoot === "true") {
          return {
            top: 0,
            right: 320,
            bottom: 240,
            left: 0,
            width: 320,
            height: 240,
            x: 0,
            y: 0,
            toJSON: () => ({}),
          };
        }
        return {
          top: 16,
          right: 48,
          bottom: 48,
          left: 16,
          width: 32,
          height: 32,
          x: 16,
          y: 16,
          toJSON: () => ({}),
        };
      },
    );

    render(
      <div
        data-testid="scroll-root"
        data-scroll-root="true"
        style={{ height: 240, overflowY: "auto" }}
      >
        <PodcastCover
          coverUrl="https://i.typlog.com/visible.png"
          title="首次可见节目"
          index={20}
          priority="low"
          sizes="32px"
        />
      </div>,
    );

    expect(
      screen.getByRole("img", { name: "首次可见节目" }),
    ).toBeInTheDocument();
  });

  it("不可见小图延迟加载且观察实际滚动容器", () => {
    let observedRoot: Element | Document | null | undefined;
    let intersectionCallback: IntersectionObserverCallback | undefined;
    vi.stubGlobal(
      "IntersectionObserver",
      class {
        constructor(
          callback: IntersectionObserverCallback,
          options?: IntersectionObserverInit,
        ) {
          intersectionCallback = callback;
          observedRoot = options?.root;
        }
        observe() {}
        disconnect() {}
      },
    );
    vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockImplementation(
      function getBoundingClientRect() {
        if (this.dataset.scrollRoot === "true") {
          return {
            top: 0,
            right: 320,
            bottom: 240,
            left: 0,
            width: 320,
            height: 240,
            x: 0,
            y: 0,
            toJSON: () => ({}),
          };
        }
        return {
          top: 2_000,
          right: 48,
          bottom: 2_032,
          left: 16,
          width: 32,
          height: 32,
          x: 16,
          y: 2_000,
          toJSON: () => ({}),
        };
      },
    );

    render(
      <div
        data-testid="scroll-root"
        data-scroll-root="true"
        style={{ height: 240, overflowY: "auto" }}
      >
        <PodcastCover
          coverUrl="https://i.typlog.com/offscreen.png"
          title="不可见节目"
          index={20}
          priority="low"
          sizes="32px"
        />
      </div>,
    );

    const scrollRoot = screen.getByTestId("scroll-root");
    expect(observedRoot).toBe(scrollRoot);
    expect(screen.queryByRole("img", { name: "不可见节目" })).toBeNull();

    act(() => {
      intersectionCallback?.(
        [{ isIntersecting: true } as IntersectionObserverEntry],
        {} as IntersectionObserver,
      );
    });
    expect(
      screen.getByRole("img", { name: "不可见节目" }),
    ).toBeInTheDocument();
  });

  it("非首屏封面不会仅因索引靠前就立即请求", () => {
    vi.stubGlobal(
      "IntersectionObserver",
      class {
        observe() {}
        disconnect() {}
      },
    );
    vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockReturnValue({
      top: 2_000,
      right: 260,
      bottom: 2_228,
      left: 32,
      width: 228,
      height: 228,
      x: 32,
      y: 2_000,
      toJSON: () => ({}),
    });

    render(
      <PodcastCover
        coverUrl="https://i.typlog.com/not-first-screen.png"
        title="非首屏节目"
        index={13}
        priority="medium"
        sizes="228px"
      />,
    );

    expect(screen.queryByRole("img", { name: "非首屏节目" })).toBeNull();
  });

  it("布局中隐藏的零尺寸封面不会开始请求", () => {
    vi.stubGlobal(
      "IntersectionObserver",
      class {
        observe() {}
        disconnect() {}
      },
    );
    vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockReturnValue({
      top: 0,
      right: 0,
      bottom: 0,
      left: 0,
      width: 0,
      height: 0,
      x: 0,
      y: 0,
      toJSON: () => ({}),
    });

    render(
      <PodcastCover
        coverUrl="https://i.typlog.com/hidden.png"
        title="隐藏节目"
        index={0}
        priority="low"
        sizes="40px"
      />,
    );

    expect(screen.queryByRole("img", { name: "隐藏节目" })).toBeNull();
  });

  it("图片长时间未完成时，会收敛到稳定占位状态", async () => {
    render(
      <PodcastCover
        coverUrl="https://i.typlog.com/cover-1.png"
        title="播客 1"
        index={0}
        priority="high"
      />,
    );

    await act(async () => {
      await vi.advanceTimersByTimeAsync(15_000);
    });
    expect(
      screen.getByRole("img", { name: "播客 1封面暂不可用" }),
    ).toBeInTheDocument();
  });

  it("图片反复失败后会在有限重试后显示稳定占位", async () => {
    render(
      <PodcastCover
        coverUrl="https://i.typlog.com/cover-2.png"
        title="播客 2"
        index={1}
        priority="high"
      />,
    );

    const initialImage = screen.getByRole("img");
    const canonicalSrc = initialImage.getAttribute("src");
    fireEvent.error(initialImage);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(400);
    });
    const firstRetryImage = screen.getByRole("img");
    expect(firstRetryImage).not.toBe(initialImage);
    expect(firstRetryImage).toHaveAttribute("src", canonicalSrc);
    expect(firstRetryImage.getAttribute("src")).not.toContain("_retry");

    fireEvent.error(firstRetryImage);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(800);
    });
    fireEvent.error(screen.getByRole("img"));

    expect(
      screen.getByRole("img", { name: "播客 2封面暂不可用" }),
    ).toBeInTheDocument();
  });
});

describe("分页失败保留已加载节目验收 (#13/#12)", () => {
  vi.mock("@/components/common/VirtualPodcastGrid", () => ({
    default: ({ podcasts }: { podcasts: Podcast[] }) => (
      <div data-testid="virtual-grid">
        {podcasts.map((podcast) => (
          <div key={podcast.id} data-testid={`podcast-${podcast.id}`}>
            {podcast.title}
          </div>
        ))}
      </div>
    ),
  }));

  function makePodcasts(): Podcast[] {
    return [1, 2].map((id) => ({
      id,
      title: `播客 ${id}`,
      image_url: `https://i.typlog.com/cover-${id}.png`,
    })) as unknown as Podcast[];
  }

  const baseProps = {
    podcasts: makePodcasts(),
    columns: 5,
    isMobile: false,
    listStateKey: "key",
    sortBy: "recent_update" as PodcastSortBy,
    selectedTagIds: [],
    hasMore: true,
    isLoading: false,
    isLoadingMore: false,
    isError: false,
    errorMessage: "",
    onLoadMore: vi.fn(),
    onRetry: vi.fn(),
    onClearFilters: vi.fn(),
  };

  it("后续分页失败时，已加载节目仍可见并提供重试", () => {
    const onRetry = vi.fn();
    render(
      <PodcastListResults
        {...baseProps}
        isError
        errorMessage="分页加载失败，请重试"
        onRetry={onRetry}
      />,
    );

    expect(screen.getByTestId("podcast-1")).toBeInTheDocument();
    expect(screen.getByTestId("podcast-2")).toBeInTheDocument();

    const footer = screen.getByTestId("pagination-error");
    fireEvent.click(within(footer).getByRole("button", { name: /重试/ }));
    expect(onRetry).toHaveBeenCalled();
  });

  it("首屏失败时仍显示整页错误态与重试", () => {
    const onRetry = vi.fn();
    render(
      <PodcastListResults
        {...baseProps}
        podcasts={[]}
        isError
        errorMessage="首屏加载失败"
        onRetry={onRetry}
      />,
    );

    expect(screen.queryByTestId("virtual-grid")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: /重试/ }));
    expect(onRetry).toHaveBeenCalled();
  });
});
