import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
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

vi.mock("@/hooks/useAvailableTags", () => ({
  useAvailableTags: () => ({
    availableTags: [],
    loading: false,
    ensureAvailableTags: vi.fn(),
    appendAvailableTag: vi.fn(),
  }),
}));

vi.mock("../PodcastCover", () => ({
  default: ({ title }: { title: string }) => (
    <div data-testid="podcast-cover">{title}</div>
  ),
}));

const podcast: Podcast = {
  id: 10,
  xyz_id: "podcast-10",
  title: "Show Notes 测试播客",
  description: "短简介",
  author: "作者",
  cover_url: "",
  episode_count: 2,
  newest_episode_date: new Date(2026, 7, 31, 9, 5, 0).toISOString(),
  created_at: "2026-01-01T00:00:00Z",
  is_subscribed: true,
  is_dead: false,
  newest_enclosure_url: "https://example.com/latest.mp3",
  newest_enclosure_duration: 3600,
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

function expandButton(title: string) {
  return within(cardFor(title)).getByRole("button", { name: "阅读简介" });
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

  it("starts from the real title region and summaries without prefetching", () => {
    render(<PodcastDetailContent {...baseProps} />);

    expect(
      screen.getAllByRole("heading", { name: "Show Notes 测试播客" }).length,
    ).toBeGreaterThan(0);
    expect(
      screen.getAllByText("作者 · 2 集 · 更新于 2026/08/31 09:05").length,
    ).toBeGreaterThan(0);
    expect(
      screen.queryByRole("button", { name: "播放最新一集" }),
    ).not.toBeInTheDocument();
    expect(screen.queryByText("60分0秒")).not.toBeInTheDocument();
    expect(screen.getAllByRole("button", { name: "＋ 添加标签" }).length).toBeGreaterThan(
      0,
    );
    expect(screen.getAllByRole("button", { name: "＋ 添加备注" }).length).toBeGreaterThan(
      0,
    );
    expect(screen.getAllByText("单集 A 的三行轻量预览")[0]).toBeVisible();
    expect(screen.getByText("单集 B 的三行轻量预览")).toBeVisible();
    expect(apiMocks.getShowNotes).not.toHaveBeenCalled();
  });

  it("does not expand on hover or focus, then loads only the selected episode on click", async () => {
    render(<PodcastDetailContent {...baseProps} />);

    fireEvent.mouseEnter(cardFor("单集 A"));
    fireEvent.focus(screen.getByRole("link", { name: "单集 A" }));
    expect(screen.queryByRole("heading", { name: "完整 A" })).not.toBeInTheDocument();
    expect(apiMocks.getShowNotes).not.toHaveBeenCalled();

    fireEvent.click(expandButton("单集 A"));
    expect(await screen.findByRole("heading", { name: "完整 A" })).toBeVisible();
    expect(apiMocks.getShowNotes).toHaveBeenCalledTimes(1);
    expect(apiMocks.getShowNotes).toHaveBeenCalledWith(1);
    expect(
      screen.getByRole("region", { name: "完整 Show Notes" }),
    ).toBeInTheDocument();

    fireEvent.mouseLeave(cardFor("单集 A"));
    expect(screen.getByRole("heading", { name: "完整 A" })).toBeVisible();
    expect(within(cardFor("单集 A")).getByRole("button", { name: "关闭简介" })).toBeVisible();

    fireEvent.click(within(cardFor("单集 A")).getByRole("button", { name: "关闭简介" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(screen.queryByRole("heading", { name: "完整 A" })).not.toBeInTheDocument();
    fireEvent.click(expandButton("单集 A"));
    expect(await screen.findByRole("heading", { name: "完整 A" })).toBeVisible();
    expect(apiMocks.getShowNotes).toHaveBeenCalledTimes(1);
  });

  it("lets keyboard activation expand and collapse the same card", async () => {
    render(<PodcastDetailContent {...baseProps} />);
    const toggle = expandButton("单集 A");
    toggle.focus();
    fireEvent.keyDown(toggle, { key: "Enter" });
    fireEvent.click(toggle);
    expect(await screen.findByRole("heading", { name: "完整 A" })).toBeVisible();
    const collapse = within(cardFor("单集 A")).getByRole("button", {
      name: "关闭简介",
    });
    fireEvent.keyDown(collapse, { key: " " });
    fireEvent.click(collapse);
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(screen.queryByRole("heading", { name: "完整 A" })).not.toBeInTheDocument();
  });

  it("keeps long full Show Notes in page flow instead of a card scroll window", async () => {
    const longDocument = Array.from(
      { length: 80 },
      (_, index) => `长文段落 ${index + 1}：用于验证完整内容没有被截断。`,
    ).join("\n\n");
    apiMocks.getShowNotes.mockResolvedValueOnce({
      episode_id: 1,
      show_notes_document: {
        content: longDocument,
        format: "markdown",
      },
    });
    render(<PodcastDetailContent {...baseProps} />);

    fireEvent.click(expandButton("单集 A"));
    const reader = await screen.findByRole("region", {
      name: "完整 Show Notes",
    });
    expect(within(reader).getByText(/长文段落 80/)).toBeVisible();
    expect(reader).toHaveClass("podcast-notes-dialog-content");
    expect(cardFor("单集 A")).toContainElement(reader);
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

    fireEvent.click(expandButton("单集 A"));
    expect(screen.getAllByText("单集 A 的三行轻量预览")[0]).toBeVisible();
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

  it("keeps a late A response from replacing the active B card or reopening a collapsed card", async () => {
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

    fireEvent.click(expandButton("单集 A"));
    await waitFor(() => expect(resolvers.has(1)).toBe(true));
    fireEvent.click(within(cardFor("单集 A")).getByRole("button", { name: "关闭简介" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    fireEvent.click(expandButton("单集 B"));
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
    expect(within(cardFor("单集 A")).getByRole("button", { name: "阅读简介" })).toBeVisible();
  });

  it("lets the 390px path expand the selected episode without prefetching others", async () => {
    setDesktopViewport(false);
    render(<PodcastDetailContent {...baseProps} />);

    fireEvent.mouseEnter(cardFor("单集 A"));
    fireEvent.focus(screen.getByRole("link", { name: "单集 A" }));
    expect(apiMocks.getShowNotes).not.toHaveBeenCalled();

    fireEvent.click(expandButton("单集 A"));
    expect(await screen.findByRole("heading", { name: "完整 A" })).toBeVisible();
    expect(apiMocks.getShowNotes).toHaveBeenCalledTimes(1);
    expect(apiMocks.getShowNotes).toHaveBeenCalledWith(1);
    expect(screen.getAllByRole("link", { name: /查看详情/ })).toHaveLength(2);
  });
});
