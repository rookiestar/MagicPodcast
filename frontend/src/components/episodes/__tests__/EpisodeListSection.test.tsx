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

  it("shows the empty state when the podcast has no episodes", () => {
    render(<EpisodeListSection {...baseProps} />);

    expect(screen.getByText("暂无单集")).toBeInTheDocument();
    expect(screen.getByText("点击下方按钮同步单集数据")).toBeInTheDocument();
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
});
