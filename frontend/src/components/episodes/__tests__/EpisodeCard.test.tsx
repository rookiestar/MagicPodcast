import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { useMemo, type ComponentProps } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useOriginalEpisodeRecovery } from "@/hooks/useOriginalEpisodeRecovery";
import type { Episode } from "@/types";
import { useQueuedEpisodeImage } from "@/hooks/useQueuedEpisodeImage";
import {
  createEpisodeShowNotesStore,
  type EpisodeShowNotesStore,
} from "@/lib/episodeShowNotesStore";
import EpisodeCard from "../EpisodeCard";

vi.mock("@/components/RichText", () => ({
  default: ({ html, className }: { html: string; className?: string }) => (
    <div className={className} data-testid="rich-text">
      {html}
    </div>
  ),
}));

vi.mock("@/hooks/useQueuedEpisodeImage", () => ({
  useQueuedEpisodeImage: vi.fn(() => ({
    imageLoaded: true,
    imageError: false,
    imgRef: { current: null },
  })),
}));

const useQueuedEpisodeImageMock = vi.mocked(useQueuedEpisodeImage);
const getShowNotesMock = vi.fn((episodeId: number) =>
  Promise.resolve({
    episode_id: episodeId,
    show_notes_document: {
      content: `<h2>完整正文 ${episodeId}</h2><p>可滚动内容</p>`,
      format: "html" as const,
    },
  }),
);

type TestEpisodeCardProps = Omit<
  ComponentProps<typeof EpisodeCard>,
  "originalRecovery" | "showNotesStore"
> & { showNotesStore?: EpisodeShowNotesStore };

function TestEpisodeCard({
  showNotesStore,
  ...props
}: TestEpisodeCardProps) {
  const originalRecovery = useOriginalEpisodeRecovery();
  const fallbackStore = useMemo(
    () => createEpisodeShowNotesStore(getShowNotesMock),
    [],
  );
  return (
    <EpisodeCard
      {...props}
      originalRecovery={originalRecovery}
      showNotesStore={showNotesStore ?? fallbackStore}
    />
  );
}

function EpisodeCardPairHarness() {
  const originalRecovery = useOriginalEpisodeRecovery();
  const showNotesStore = useMemo(
    () => createEpisodeShowNotesStore(getShowNotesMock),
    [],
  );
  const episodeUrl = (id: number) =>
    `https://www.xiaoyuzhoufm.com/episode/${id}?utm_source=rss`;

  return (
    <>
      <EpisodeCard
        episode={makeEpisode({ id: 1, title: "单集 A", link: episodeUrl(1) })}
        originalRecovery={originalRecovery}
        showNotesStore={showNotesStore}
      />
      <EpisodeCard
        episode={makeEpisode({ id: 2, title: "单集 B", link: episodeUrl(2) })}
        originalRecovery={originalRecovery}
        showNotesStore={showNotesStore}
      />
    </>
  );
}

function makeEpisode(overrides: Partial<Episode> = {}): Episode {
  return {
    id: 1,
    guid: "episode-1",
    podcast_id: 10,
    episode_no: "E12",
    title: "单集标题",
    medium_url: "https://example.com/audio.mp3",
    show_notes: "旧简介",
    published_date: "2026-01-01T00:00:00Z",
    duration: 3661,
    link: "https://example.com/episode",
    image_url: "https://i.typlog.com/episode.jpg",
    enclosure_type: "audio/mpeg",
    enclosure_length: 2.25 * 1024 * 1024,
    my_rate: 0,
    notes: "",
    ...overrides,
  };
}

describe("EpisodeCard", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    getShowNotesMock.mockImplementation((episodeId: number) =>
      Promise.resolve({
        episode_id: episodeId,
        show_notes_document: {
          content: `<h2>完整正文 ${episodeId}</h2><p>可滚动内容</p>`,
          format: "html" as const,
        },
      }),
    );
    Object.defineProperty(window, "matchMedia", {
      configurable: true,
      value: vi.fn((query: string) => ({
        matches: query === "(min-width: 768px)",
        media: query,
        onchange: null,
        addListener: vi.fn(),
        removeListener: vi.fn(),
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
        dispatchEvent: vi.fn(),
      })),
    });
  });

  it("renders core episode information and opens the audio link", () => {
    const openSpy = vi.spyOn(window, "open").mockImplementation(() => null);

    render(<TestEpisodeCard episode={makeEpisode()} podcastCover="cover.jpg" />);

    expect(screen.getByRole("link", { name: "单集标题" })).toHaveAttribute(
      "href",
      "https://example.com/episode",
    );
    expect(screen.getByRole("link", { name: "单集标题" })).toHaveAttribute(
      "data-editorial-display-text",
      "true",
    );
    expect(screen.getByText("#12")).toBeInTheDocument();
    expect(screen.getByText("1小时1分1秒")).toBeInTheDocument();
    expect(screen.getByText("2.3 MB")).toBeInTheDocument();
    expect(screen.getByAltText("单集标题")).toHaveAttribute(
      "decoding",
      "async",
    );

    fireEvent.click(screen.getByRole("button", { name: "播放" }));

    expect(openSpy).toHaveBeenCalledWith(
      "https://example.com/audio.mp3",
      "_blank",
    );
    openSpy.mockRestore();
  });

  it("shows 看视频 only for available episodes and opens the page instead of HLS", () => {
    const openSpy = vi.spyOn(window, "open").mockImplementation(() => null);
    const xyz =
      "https://www.xiaoyuzhoufm.com/episode/6a734c29ab3a91c24a1067fa?utm_source=rss";

    const { rerender } = render(
      <TestEpisodeCard
        episode={makeEpisode({
          link: xyz,
          video_availability: "available",
        })}
      />,
    );

    const videoLink = screen.getByRole("link", { name: "看视频" });
    expect(videoLink).toHaveAttribute("href", xyz);
    expect(videoLink.getAttribute("href")).not.toContain("m3u8");

    fireEvent.click(screen.getByRole("button", { name: "播放" }));
    expect(openSpy).toHaveBeenCalledWith(
      "https://example.com/audio.mp3",
      "_blank",
    );

    fireEvent.click(videoLink);
    expect(
      screen.getByRole("region", { name: "原节目页恢复" }),
    ).toBeInTheDocument();

    rerender(
      <TestEpisodeCard
        episode={makeEpisode({
          link: xyz,
          video_availability: "unknown",
        })}
      />,
    );
    expect(screen.queryByRole("link", { name: "看视频" })).not.toBeInTheDocument();

    rerender(
      <TestEpisodeCard
        episode={makeEpisode({
          link: xyz,
          video_availability: "unavailable",
        })}
      />,
    );
    expect(screen.queryByRole("link", { name: "看视频" })).not.toBeInTheDocument();

    rerender(
      <TestEpisodeCard
        episode={makeEpisode({
          link: "javascript:alert(1)",
          video_availability: "available",
        })}
      />,
    );
    expect(screen.queryByRole("link", { name: "看视频" })).not.toBeInTheDocument();
    openSpy.mockRestore();
  });

  it("offers Xiaoyuzhou recovery after opening the original episode page", async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: { writeText },
    });
    const openSpy = vi.spyOn(window, "open").mockImplementation(() => null);

    render(
      <TestEpisodeCard
        episode={makeEpisode({
          link: "https://www.xiaoyuzhoufm.com/episode/6a8cf80a1352af56ff3b7e2d?utm_source=rss",
        })}
      />,
    );

    fireEvent.click(screen.getByRole("link", { name: "单集标题" }));
    expect(
      screen.getByRole("region", { name: "原节目页恢复" }),
    ).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "重试打开" }));
    expect(openSpy).toHaveBeenCalledWith(
      "https://www.xiaoyuzhoufm.com/episode/6a8cf80a1352af56ff3b7e2d",
      "_blank",
      "noopener,noreferrer",
    );
    fireEvent.click(screen.getByRole("button", { name: "复制页面链接" }));
    await waitFor(() =>
      expect(writeText).toHaveBeenCalledWith(
        "https://www.xiaoyuzhoufm.com/episode/6a8cf80a1352af56ff3b7e2d",
      ),
    );
    openSpy.mockRestore();
  });

  it("keeps only the latest episode recovery in a shared list", () => {
    render(<EpisodeCardPairHarness />);

    fireEvent.click(screen.getByRole("link", { name: "单集 A" }));
    expect(screen.getAllByRole("region", { name: "原节目页恢复" })).toHaveLength(
      1,
    );

    fireEvent.click(screen.getByRole("link", { name: "单集 B" }));
    expect(screen.getAllByRole("region", { name: "原节目页恢复" })).toHaveLength(
      1,
    );
    expect(
      screen
        .getByRole("link", { name: "单集 B" })
        .closest(".podcast-episode-card"),
    ).toContainElement(
      screen.getByRole("region", { name: "原节目页恢复" }),
    );
  });

  it("ignores a stale copy failure after switching episodes", async () => {
    let rejectFirstCopy: (reason?: unknown) => void;
    const firstCopy = new Promise<void>((_resolve, reject) => {
      rejectFirstCopy = reject;
    });
    const writeText = vi.fn().mockImplementationOnce(() => firstCopy);
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: { writeText },
    });
    render(<EpisodeCardPairHarness />);

    fireEvent.click(screen.getByRole("link", { name: "单集 A" }));
    fireEvent.click(screen.getByRole("button", { name: "复制页面链接" }));
    fireEvent.click(screen.getByRole("link", { name: "单集 B" }));

    await act(async () => {
      rejectFirstCopy(new Error("denied"));
      await Promise.resolve();
    });

    expect(
      screen
        .getByRole("link", { name: "单集 B" })
        .closest(".podcast-episode-card"),
    ).toContainElement(
      screen.getByRole("region", { name: "原节目页恢复" }),
    );
    expect(
      screen.queryByText("复制失败，请改用重试或用小宇宙打开。"),
    ).not.toBeInTheDocument();
  });

  it("offers recovery from the mobile Show Notes detail link", () => {
    render(
      <TestEpisodeCard
        episode={makeEpisode({
          link: "https://www.xiaoyuzhoufm.com/episode/6a8cf80a1352af56ff3b7e2d?utm_source=rss",
        })}
      />,
    );

    fireEvent.click(screen.getByRole("link", { name: /查看详情/ }));
    expect(
      screen.getByRole("region", { name: "原节目页恢复" }),
    ).toBeInTheDocument();
  });

  it("queues only episode-specific cover images", () => {
    render(<TestEpisodeCard episode={makeEpisode()} podcastCover="cover.jpg" />);

    const queuedSource =
      useQueuedEpisodeImageMock.mock.calls[0]?.[0].src ?? "";
    const optimizerUrl = new URL(queuedSource, "http://localhost");
    expect(optimizerUrl.pathname).toBe("/_next/image.webp");
    expect(optimizerUrl.searchParams.get("url")).toBe(
      "/images/proxy?url=https%3A%2F%2Fi.typlog.com%2Fepisode.jpg",
    );
  });

  it("renders the podcast fallback cover directly without queueing it", () => {
    render(
      <TestEpisodeCard
        episode={makeEpisode({ image_url: "" })}
        podcastCover="cover.jpg"
      />,
    );

    expect(useQueuedEpisodeImageMock).toHaveBeenCalledWith(
      expect.objectContaining({
        src: "",
      }),
    );
    expect(screen.getByAltText("单集标题")).toHaveAttribute("src", "cover.jpg");
    expect(screen.getByAltText("单集标题")).toHaveAttribute("loading", "eager");
  });

  it("lazy-loads lower priority fallback cover images", () => {
    render(
      <TestEpisodeCard
        episode={makeEpisode({ image_url: "" })}
        podcastCover="cover.jpg"
        index={10}
      />,
    );

    expect(screen.getByAltText("单集标题")).toHaveAttribute("loading", "lazy");
  });

  it("loads full show notes when keyboard focus enters the card", async () => {
    render(<TestEpisodeCard episode={makeEpisode()} />);

    expect(screen.getByText("旧简介")).toBeVisible();
    expect(getShowNotesMock).not.toHaveBeenCalled();
    expect(screen.queryByTestId("rich-text")).not.toBeInTheDocument();

    fireEvent.focus(screen.getByRole("link", { name: "单集标题" }));

    expect(await screen.findByRole("region", { name: "完整 Show Notes" })).toBeVisible();
    expect(screen.getByTestId("rich-text")).toHaveTextContent("完整正文 1");
    expect(getShowNotesMock).toHaveBeenCalledTimes(1);
  });

  it("loads full show notes when the explicit availability flag has no text preview", async () => {
    render(
      <TestEpisodeCard
        episode={makeEpisode({ show_notes: "", has_show_notes: true })}
      />,
    );

    fireEvent.focus(screen.getByRole("link", { name: "单集标题" }));

    expect(
      await screen.findByRole("region", { name: "完整 Show Notes" }),
    ).toBeVisible();
    expect(getShowNotesMock).toHaveBeenCalledWith(1);
  });

  it("keeps the preview during a slow request and reuses the successful result", async () => {
    let resolveRequest!: (value: Awaited<ReturnType<typeof getShowNotesMock>>) => void;
    getShowNotesMock.mockImplementationOnce(
      () => new Promise((resolve) => (resolveRequest = resolve)),
    );
    render(<TestEpisodeCard episode={makeEpisode()} />);
    const card = screen.getByRole("link", { name: "单集标题" }).closest(
      ".podcast-episode-card",
    )!;

    fireEvent.mouseEnter(card);
    expect(screen.getByText("旧简介")).toBeVisible();
    expect(screen.getByRole("status")).toHaveTextContent(
      "正在读取完整 Show Notes",
    );

    resolveRequest({
      episode_id: 1,
      show_notes_document: {
        content: "<h2>慢请求完成</h2>",
        format: "html",
      },
    });
    expect(await screen.findByText("<h2>慢请求完成</h2>")).toBeVisible();

    fireEvent.mouseLeave(card);
    expect(screen.getByText("旧简介")).toBeVisible();
    fireEvent.mouseEnter(card);
    expect(await screen.findByText("<h2>慢请求完成</h2>")).toBeVisible();
    expect(getShowNotesMock).toHaveBeenCalledTimes(1);
  });

  it("keeps the preview and original entry on failure, then retries", async () => {
    getShowNotesMock
      .mockRejectedValueOnce(new Error("offline"))
      .mockResolvedValueOnce({
        episode_id: 1,
        show_notes_document: {
          content: "<h2>重试成功</h2>",
          format: "html",
        },
      });
    render(<TestEpisodeCard episode={makeEpisode()} />);

    fireEvent.focus(screen.getByRole("link", { name: "单集标题" }));
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "预览仍可查看",
    );
    expect(screen.getByText("旧简介")).toBeVisible();
    expect(screen.getByRole("link", { name: "单集标题" })).toHaveAttribute(
      "href",
      "https://example.com/episode",
    );

    fireEvent.click(screen.getByRole("button", { name: "重试全文" }));
    expect(await screen.findByText("<h2>重试成功</h2>")).toBeVisible();
    expect(getShowNotesMock).toHaveBeenCalledTimes(2);
  });

  it("does not collapse a focused card when the pointer leaves", async () => {
    render(<TestEpisodeCard episode={makeEpisode()} />);
    const title = screen.getByRole("link", { name: "单集标题" });
    const card = title.closest(".podcast-episode-card")!;

    fireEvent.mouseEnter(card);
    fireEvent.focus(title);
    expect(await screen.findByRole("region", { name: "完整 Show Notes" })).toBeVisible();
    fireEvent.mouseLeave(card);

    expect(screen.getByRole("region", { name: "完整 Show Notes" })).toBeVisible();
  });

  it("ignores a late response after the card identity changes", async () => {
    let resolveFirst!: (value: Awaited<ReturnType<typeof getShowNotesMock>>) => void;
    getShowNotesMock
      .mockImplementationOnce(
        () => new Promise((resolve) => (resolveFirst = resolve)),
      )
      .mockResolvedValueOnce({
        episode_id: 2,
        show_notes_document: {
          content: "<h2>单集 B 全文</h2>",
          format: "html",
        },
      });
    const { rerender } = render(
      <TestEpisodeCard episode={makeEpisode({ id: 1, title: "单集 A" })} />,
    );
    fireEvent.focus(screen.getByRole("link", { name: "单集 A" }));

    rerender(
      <TestEpisodeCard episode={makeEpisode({ id: 2, title: "单集 B" })} />,
    );
    fireEvent.focus(screen.getByRole("link", { name: "单集 B" }));
    expect(await screen.findByText("<h2>单集 B 全文</h2>")).toBeVisible();

    resolveFirst({
      episode_id: 1,
      show_notes_document: {
        content: "<h2>迟到的单集 A</h2>",
        format: "html",
      },
    });
    await act(async () => Promise.resolve());
    expect(screen.queryByText("<h2>迟到的单集 A</h2>")).not.toBeInTheDocument();
    expect(screen.getByText("<h2>单集 B 全文</h2>")).toBeVisible();
  });

  it("does not request full show notes in a mobile viewport", () => {
    Object.defineProperty(window, "matchMedia", {
      configurable: true,
      value: vi.fn((query: string) => ({
        matches: false,
        media: query,
        onchange: null,
        addListener: vi.fn(),
        removeListener: vi.fn(),
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
        dispatchEvent: vi.fn(),
      })),
    });
    render(<TestEpisodeCard episode={makeEpisode()} />);
    const title = screen.getByRole("link", { name: "单集标题" });
    const card = title.closest(".podcast-episode-card")!;

    fireEvent.mouseEnter(card);
    fireEvent.focus(title);

    expect(screen.getByText("旧简介")).toBeVisible();
    expect(screen.getByRole("link", { name: /查看详情/ })).toBeVisible();
    expect(getShowNotesMock).not.toHaveBeenCalled();
  });

  it("rerenders when memoized episode display fields change", () => {
    const episode = makeEpisode();
    const { rerender } = render(<TestEpisodeCard episode={episode} />);

    expect(screen.getAllByText("旧简介")).toHaveLength(1);

    rerender(
      <TestEpisodeCard
        episode={{
          ...episode,
          show_notes: "新简介",
          duration: 60,
          enclosure_length: 1024 * 1024,
        }}
      />,
    );

    expect(screen.queryByText("旧简介")).not.toBeInTheDocument();
    expect(screen.getAllByText("新简介")).toHaveLength(1);
    expect(screen.getByText("1分")).toBeInTheDocument();
    expect(screen.getByText("1.0 MB")).toBeInTheDocument();
  });
});
