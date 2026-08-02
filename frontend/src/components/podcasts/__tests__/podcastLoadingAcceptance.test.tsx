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
  vi.restoreAllMocks();
});

describe("封面加载收敛验收 (#13/#14)", () => {
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

    fireEvent.error(screen.getByRole("img"));
    await act(async () => {
      await vi.advanceTimersByTimeAsync(400);
    });
    fireEvent.error(screen.getByRole("img"));
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
