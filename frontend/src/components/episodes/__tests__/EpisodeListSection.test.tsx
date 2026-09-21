import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { OriginalEpisodeRecoveryController } from "@/hooks/useOriginalEpisodeRecovery";
import type { Episode } from "@/types";
import EpisodeListSection from "../EpisodeListSection";

vi.mock("../EpisodeCard", () => ({
  default: ({
    episode,
    originalRecovery,
  }: {
    episode: Episode;
    originalRecovery?: OriginalEpisodeRecoveryController;
  }) => {
    const active = originalRecovery?.activeKey === episode.id;
    const retryUrl = `https://www.xiaoyuzhoufm.com/episode/${episode.id}`;
    return (
      <article data-testid="episode-card">
        {episode.title}
        {originalRecovery ? (
          <button
            type="button"
            onClick={() =>
              originalRecovery.activate(episode.id, {
                recovery: true,
                openUrl: `${retryUrl}?utm_source=rss`,
                retryUrl,
                appUrl: `cosmos://page.cos/episode/${episode.id}`,
                copyText: retryUrl,
              })
            }
          >
            打开 {episode.title}
          </button>
        ) : null}
        {active ? <div role="region">恢复 {episode.title}</div> : null}
      </article>
    );
  },
}));

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

const baseProps = {
  episodes: [],
  episodesLoading: false,
  isLoadingMore: false,
  hasMoreEpisodes: false,
  totalEpisodes: 0,
  podcastCover: "",
  loadMoreRef: vi.fn(),
};

describe("EpisodeListSection", () => {
  it("shows the loading skeleton before the first page is ready", () => {
    const { container } = render(
      <EpisodeListSection {...baseProps} episodesLoading />,
    );

    expect(container.querySelectorAll(".animate-pulse").length).toBeGreaterThan(
      0,
    );
  });

  it("shows the empty state with a real sync button when the podcast has no episodes", () => {
    const onStart = vi.fn();
    const syncControl = {
      status: "idle" as const,
      label: "同步历史单集",
      disabled: false,
      inFlight: false,
      progressText: null,
      errorMessage: null,
      sourceNote: null,
      onStart,
    };
    render(<EpisodeListSection {...baseProps} syncControl={syncControl} />);

    expect(screen.getByText("暂无单集")).toBeInTheDocument();
    // 空态提供直接可见的同步按钮，替换原先指向不存在按钮的提示文案（#465）。
    expect(
      screen.queryByText("点击下方按钮同步单集数据"),
    ).not.toBeInTheDocument();
    // 标题行常驻入口与空态按钮共享同一动作，两处同时可见。
    const syncButtons = screen.getAllByRole("button", {
      name: "同步历史单集",
    });
    expect(syncButtons).toHaveLength(2);
    syncButtons.forEach((button) => expect(button).toBeEnabled());
    fireEvent.click(syncButtons[syncButtons.length - 1]);
    expect(onStart).toHaveBeenCalledTimes(1);
  });

  it("renders episode cards and the total count", () => {
    const { container } = render(
      <EpisodeListSection
        {...baseProps}
        episodes={[makeEpisode(1), makeEpisode(2)]}
        totalEpisodes={218}
        hasMoreEpisodes
      />,
    );

    expect(screen.getByText("单集列表 (218 集)")).toBeInTheDocument();
    expect(screen.getAllByTestId("episode-card")).toHaveLength(2);
    expect(screen.getByText("Episode 1")).toBeInTheDocument();
    expect(container.querySelector("#episode-1")).toHaveStyle({
      contentVisibility: "auto",
    });
  });

  it("replaces the previous episode recovery when another episode opens", () => {
    render(
      <EpisodeListSection
        {...baseProps}
        episodes={[makeEpisode(1), makeEpisode(2)]}
        totalEpisodes={2}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "打开 Episode 1" }));
    expect(screen.getByRole("region")).toHaveTextContent("恢复 Episode 1");

    fireEvent.click(screen.getByRole("button", { name: "打开 Episode 2" }));
    expect(screen.getAllByRole("region")).toHaveLength(1);
    expect(screen.getByRole("region")).toHaveTextContent("恢复 Episode 2");
  });

  it("shows loading-more and finished messages", () => {
    const { rerender } = render(
      <EpisodeListSection
        {...baseProps}
        episodes={[makeEpisode(1)]}
        isLoadingMore
        hasMoreEpisodes
      />,
    );

    expect(screen.getByText("正在加载更多单集...")).toBeInTheDocument();

    rerender(
      <EpisodeListSection
        {...baseProps}
        episodes={[makeEpisode(1)]}
        hasMoreEpisodes={false}
      />,
    );

    expect(screen.getByText("已加载全部 1 集单集")).toBeInTheDocument();
  });

  it("shows a retryable error when the first page fails", () => {
    const onRetry = vi.fn();

    render(
      <EpisodeListSection
        {...baseProps}
        episodesError="network error"
        onRetry={onRetry}
      />,
    );

    expect(screen.getByRole("alert")).toHaveTextContent("单集加载失败");
    expect(screen.getByText("network error")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "重试" }));
    expect(onRetry).toHaveBeenCalledTimes(1);
  });

  it("shows an inline error without replacing already loaded episodes", () => {
    render(
      <EpisodeListSection
        {...baseProps}
        episodes={[makeEpisode(1)]}
        episodesError="page 2 failed"
        hasMoreEpisodes={false}
      />,
    );

    expect(screen.getByText("Episode 1")).toBeInTheDocument();
    expect(screen.getByRole("alert")).toHaveTextContent("page 2 failed");
    expect(screen.queryByText("已加载全部 1 集单集")).not.toBeInTheDocument();
  });

  // #465：标题行常驻同步入口——运行中禁用并展示真实进度。
  it("keeps the persistent sync entry disabled with progress while running", () => {
    const onStart = vi.fn();
    render(
      <EpisodeListSection
        {...baseProps}
        episodes={[makeEpisode(1)]}
        syncControl={{
          status: "running",
          label: "同步中…",
          disabled: true,
          inFlight: true,
          progressText: "已处理 320 / 1200",
          errorMessage: null,
          sourceNote: null,
          onStart,
        }}
      />,
    );

    const button = screen.getByRole("button", { name: "同步中…" });
    expect(button).toBeDisabled();
    expect(screen.getByText("已处理 320 / 1200")).toBeInTheDocument();
    expect(onStart).not.toHaveBeenCalled();
  });

  // #465：失败后展示原因并提供「重试同步」；列表读取失败的「重试」与
  // 同步操作是两个互不替代的入口。
  it("shows the failure reason with a retry sync action when the task failed", () => {
    const onStart = vi.fn();
    const onRetry = vi.fn();
    render(
      <EpisodeListSection
        {...baseProps}
        episodes={[makeEpisode(1)]}
        syncControl={{
          status: "failed",
          label: "重试同步",
          disabled: false,
          inFlight: false,
          progressText: null,
          errorMessage: "upstream timeout",
          sourceNote: null,
          onStart,
        }}
      />,
    );

    expect(screen.getByRole("alert")).toHaveTextContent("upstream timeout");
    const retryButton = screen.getByRole("button", { name: "重试同步" });
    fireEvent.click(retryButton);
    expect(onStart).toHaveBeenCalledTimes(1);
    expect(onRetry).not.toHaveBeenCalled();
  });
});
