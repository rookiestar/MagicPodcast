import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { Episode, Podcast } from "@/types";
import type { EpisodeShowNotesPayload } from "@/types/showNotes";
import PodcastDetailContent from "../PodcastDetailContent";

const apiMocks = vi.hoisted(() => ({
  getShowNotes: vi.fn(),
}));

vi.mock("@/lib/api/episode", () => ({
  episodeApi: {
    getShowNotes: apiMocks.getShowNotes,
  },
}));

vi.mock("@/hooks/useQueuedEpisodeImage", () => ({
  useQueuedEpisodeImage: vi.fn(() => ({
    imageLoaded: true,
    imageError: false,
    imgRef: { current: null },
  })),
}));

vi.mock("../PodcastDetailInfo", () => ({
  MobilePodcastDetailInfo: () => <section data-testid="mobile-detail" />,
  DesktopPodcastDetailInfo: () => <section data-testid="desktop-detail" />,
}));

const podcast: Podcast = {
  id: 10,
  xyz_id: "podcast-10",
  title: "Show Notes 测试播客",
  description: "",
  author: "作者",
  cover_url: "",
  episode_count: 2,
  newest_episode_date: "2026-08-31T00:00:00Z",
  created_at: "2026-01-01T00:00:00Z",
  is_subscribed: true,
  is_dead: false,
};

function episode(id: number, title: string): Episode {
  return {
    id,
    guid: `episode-${id}`,
    podcast_id: podcast.id,
    episode_no: "",
    title,
    medium_url: "",
    show_notes: `${title} 的三行轻量预览`,
    published_date: "2026-08-31T00:00:00Z",
    duration: 600,
    link: `https://example.com/episodes/${id}`,
    image_url: "",
    enclosure_type: "",
    enclosure_length: 0,
    my_rate: 0,
    notes: "",
  };
}

const episodes = [episode(1, "单集 A"), episode(2, "单集 B")];

const baseProps = {
  error: null,
  podcast,
  tags: [],
  notes: "",
  isEditingNotes: false,
  episodes,
  episodesLoading: false,
  isLoadingMore: false,
  hasMoreEpisodes: false,
  totalEpisodes: 2,
  episodesError: null,
  podcastCover: "",
  loadMoreRef: vi.fn(),
  onNotesChange: vi.fn(),
  onEditNotes: vi.fn(),
  onSaveNotes: vi.fn(),
  onCancelNotesEdit: vi.fn(),
  onTagsChange: vi.fn(),
  onRetryEpisodes: vi.fn(),
};

function setDesktopViewport(desktop: boolean) {
  Object.defineProperty(window, "matchMedia", {
    configurable: true,
    value: vi.fn((query: string) => ({
      matches: desktop && query === "(min-width: 768px)",
      media: query,
      onchange: null,
      addListener: vi.fn(),
      removeListener: vi.fn(),
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      dispatchEvent: vi.fn(),
    })),
  });
}

function cardFor(title: string) {
  return screen
    .getByRole("link", { name: title })
    .closest(".podcast-episode-card") as HTMLElement;
}

describe("podcast detail Show Notes user flow", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    setDesktopViewport(true);
    apiMocks.getShowNotes.mockImplementation((episodeId: number) =>
      Promise.resolve({
        episode_id: episodeId,
        show_notes_document: {
          content: `## ${episodeId === 1 ? "完整 A" : "完整 B"}\n\n**正文**`,
          format: "markdown" as const,
        },
      }),
    );
  });

  it("starts from summaries, loads only the active episode, and reuses success", async () => {
    render(<PodcastDetailContent {...baseProps} />);

    expect(screen.getByText("单集 A 的三行轻量预览")).toBeVisible();
    expect(screen.getByText("单集 B 的三行轻量预览")).toBeVisible();
    expect(apiMocks.getShowNotes).not.toHaveBeenCalled();

    fireEvent.mouseEnter(cardFor("单集 A"));
    expect(await screen.findByRole("heading", { name: "完整 A" })).toBeVisible();
    expect(apiMocks.getShowNotes).toHaveBeenCalledTimes(1);
    expect(apiMocks.getShowNotes).toHaveBeenCalledWith(1);
    expect(
      screen.getByRole("region", { name: "完整 Show Notes" }),
    ).toHaveClass("podcast-episode-show-notes-reader");

    fireEvent.mouseLeave(cardFor("单集 A"));
    expect(screen.queryByRole("heading", { name: "完整 A" })).not.toBeInTheDocument();
    fireEvent.focus(screen.getByRole("link", { name: "单集 A" }));
    expect(await screen.findByRole("heading", { name: "完整 A" })).toBeVisible();
    expect(apiMocks.getShowNotes).toHaveBeenCalledTimes(1);
  });

  it("keeps summary content through slow and failed reads, then retries", async () => {
    let rejectFirst!: (reason?: unknown) => void;
    apiMocks.getShowNotes
      .mockImplementationOnce(
        () => new Promise((_resolve, reject) => (rejectFirst = reject)),
      )
      .mockResolvedValueOnce({
        episode_id: 1,
        show_notes_document: {
          content: "## 重试后的完整 A",
          format: "markdown",
        },
      });
    render(<PodcastDetailContent {...baseProps} />);

    fireEvent.mouseEnter(cardFor("单集 A"));
    expect(screen.getByText("单集 A 的三行轻量预览")).toBeVisible();
    expect(await screen.findByRole("status")).toHaveTextContent("正在读取完整");

    await act(async () => rejectFirst(new Error("offline")));
    expect(await screen.findByRole("alert")).toHaveTextContent("预览仍可查看");
    expect(screen.getByRole("link", { name: "单集 A" })).toHaveAttribute(
      "href",
      "https://example.com/episodes/1",
    );

    fireEvent.click(screen.getByRole("button", { name: "重试全文" }));
    expect(
      await screen.findByRole("heading", { name: "重试后的完整 A" }),
    ).toBeVisible();
    expect(apiMocks.getShowNotes).toHaveBeenCalledTimes(2);
  });

  it("keeps a late A response from replacing the active B card", async () => {
    const resolvers = new Map<
      number,
      (value: EpisodeShowNotesPayload) => void
    >();
    apiMocks.getShowNotes.mockImplementation(
      (episodeId: number) =>
        new Promise<EpisodeShowNotesPayload>((resolve) => {
          resolvers.set(episodeId, resolve);
        }),
    );
    render(<PodcastDetailContent {...baseProps} />);

    fireEvent.mouseEnter(cardFor("单集 A"));
    await waitFor(() => expect(resolvers.has(1)).toBe(true));
    fireEvent.mouseLeave(cardFor("单集 A"));
    fireEvent.mouseEnter(cardFor("单集 B"));
    await waitFor(() => expect(resolvers.has(2)).toBe(true));

    await act(async () => {
      resolvers.get(2)?.({
        episode_id: 2,
        show_notes_document: {
          content: "## 当前单集 B",
          format: "markdown",
        },
      });
    });
    expect(
      await screen.findByRole("heading", { name: "当前单集 B" }),
    ).toBeVisible();

    await act(async () => {
      resolvers.get(1)?.({
        episode_id: 1,
        show_notes_document: {
          content: "## 迟到单集 A",
          format: "markdown",
        },
      });
    });
    expect(screen.queryByRole("heading", { name: "迟到单集 A" })).not.toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "当前单集 B" })).toBeVisible();
  });

  it("keeps the 390px path summary-only without a full-text request", () => {
    setDesktopViewport(false);
    render(<PodcastDetailContent {...baseProps} />);

    fireEvent.mouseEnter(cardFor("单集 A"));
    fireEvent.focus(screen.getByRole("link", { name: "单集 A" }));

    expect(screen.getByText("单集 A 的三行轻量预览")).toBeVisible();
    expect(screen.getAllByRole("link", { name: /查看详情/ })).toHaveLength(2);
    expect(apiMocks.getShowNotes).not.toHaveBeenCalled();
  });
});
