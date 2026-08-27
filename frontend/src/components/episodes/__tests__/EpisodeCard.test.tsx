import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ComponentProps } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useOriginalEpisodeRecovery } from "@/hooks/useOriginalEpisodeRecovery";
import type { Episode } from "@/types";
import { useQueuedEpisodeImage } from "@/hooks/useQueuedEpisodeImage";
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

type TestEpisodeCardProps = Omit<
  ComponentProps<typeof EpisodeCard>,
  "originalRecovery"
>;

function TestEpisodeCard(props: TestEpisodeCardProps) {
  const originalRecovery = useOriginalEpisodeRecovery();
  return <EpisodeCard {...props} originalRecovery={originalRecovery} />;
}

function EpisodeCardPairHarness() {
  const originalRecovery = useOriginalEpisodeRecovery();
  const episodeUrl = (id: number) =>
    `https://www.xiaoyuzhoufm.com/episode/${id}?utm_source=rss`;

  return (
    <>
      <EpisodeCard
        episode={makeEpisode({ id: 1, title: "单集 A", link: episodeUrl(1) })}
        originalRecovery={originalRecovery}
      />
      <EpisodeCard
        episode={makeEpisode({ id: 2, title: "单集 B", link: episodeUrl(2) })}
        originalRecovery={originalRecovery}
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
  });

  it("renders core episode information and opens the audio link", () => {
    const openSpy = vi.spyOn(window, "open").mockImplementation(() => null);

    render(<TestEpisodeCard episode={makeEpisode()} podcastCover="cover.jpg" />);

    expect(screen.getByRole("link", { name: "单集标题" })).toHaveAttribute(
      "href",
      "https://example.com/episode",
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

  it("expands show notes when keyboard focus enters the card", () => {
    const { container } = render(<TestEpisodeCard episode={makeEpisode()} />);
    const showNotes = screen.getByText("旧简介").parentElement;

    expect(showNotes).toHaveClass("md:max-h-24");
    expect(screen.queryByTestId("rich-text")).not.toBeInTheDocument();

    fireEvent.focus(screen.getByRole("link", { name: "单集标题" }));

    expect(screen.getByTestId("rich-text").parentElement).toHaveClass(
      "md:max-h-96",
    );
    expect(container.querySelectorAll('[data-testid="rich-text"]')).toHaveLength(
      1,
    );
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
