import { act, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import PodcastCover from "@/components/podcasts/PodcastCover";
import PodcastListResults from "@/components/podcasts/PodcastListResults";
import type { PodcastSortBy } from "@/lib/podcastListState";
import type { Podcast } from "@/types";

/**
 * 加载性能回归验收（PRD #11 / 子任务 #13）—— 组件可见层。
 *
 * 覆盖截图症状：
 *   1. 封面长时间停留在灰色区域（图片请求慢/挂起时永不收敛）。
 *   2. 后续分页失败时整列表被错误态替换，已加载节目消失。
 *
 * 修复前失败、修复后通过。纯前端渲染，不访问真实后端或数据库。
 */

beforeEach(() => {
  vi.useFakeTimers({ shouldAdvanceTime: true });
});

afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
});

describe("封面加载收敛验收 (#13/#14)", () => {
  it("图片长时间未加载完成时，必须收敛到稳定占位状态而非永久灰块", async () => {
    render(
      <PodcastCover
        coverUrl="https://i.typlog.com/cover-1.png"
        title="播客 1"
        index={0}
        priority="high"
      />,
    );

    // happy-dom 不会自动触发 <img> onload，模拟图片始终未完成加载。
    // 等待超过封面加载的有界上限后，必须出现稳定占位（🎧）。
    await act(async () => {
      await vi.advanceTimersByTimeAsync(30_000);
    });

    expect(screen.getByText("🎧")).toBeInTheDocument();
  });

  it("图片加载失败时立即显示稳定占位", () => {
    render(
      <PodcastCover
        coverUrl="https://i.typlog.com/cover-2.png"
        title="播客 2"
        index={1}
        priority="high"
      />,
    );

    const img = document.querySelector("img");
    expect(img).not.toBeNull();
    // 模拟上游图片加载失败。
    fireEvent.error(img as Element);

    expect(screen.getByText("🎧")).toBeInTheDocument();
  });
});

describe("分页失败保留已加载节目验收 (#13/#12)", () => {
  // 虚拟网格依赖窗口滚动与测量，验收只关心错误态判定，故简化渲染。
  vi.mock("@/components/common/VirtualPodcastGrid", () => ({
    default: ({ podcasts }: { podcasts: Podcast[] }) => (
      <div data-testid="virtual-grid">
        {podcasts.map((p) => (
          <div key={p.id} data-testid={`podcast-${p.id}`}>
            {p.title}
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

  it("后续分页失败时，已加载节目仍可见并提供重试，而不是整页替换为错误态", () => {
    const onRetry = vi.fn();
    render(
      <PodcastListResults
        {...baseProps}
        isError
        errorMessage="分页加载失败，请重试"
        onRetry={onRetry}
      />,
    );

    // 已加载的两个节目必须仍然在页面上。
    expect(screen.getByTestId("podcast-1")).toBeInTheDocument();
    expect(screen.getByTestId("podcast-2")).toBeInTheDocument();

    // 必须提供可点击的重试入口。
    const footer = document.querySelector("[data-testid='pagination-error']");
    expect(footer).not.toBeNull();
    const retryButton = within(footer as HTMLElement).getByRole("button", {
      name: /重试/,
    });
    retryButton.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    expect(onRetry).toHaveBeenCalled();
  });

  it("首屏（无已加载节目）失败时仍显示整页错误态与重试", () => {
    const onRetry = vi.fn();
    render(
      <PodcastListResults
        {...baseProps}
        podcasts={[]}
        isError
        errorMessage="首屏加载失败"
        isLoading={false}
        onRetry={onRetry}
      />,
    );

    expect(screen.queryByTestId("virtual-grid")).toBeNull();
    const retryButton = screen.getByRole("button", { name: /重试/ });
    retryButton.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    expect(onRetry).toHaveBeenCalled();
  });
});
