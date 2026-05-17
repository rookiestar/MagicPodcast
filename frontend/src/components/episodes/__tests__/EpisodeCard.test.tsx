import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
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
    image_url: "https://example.com/episode.jpg",
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

    render(<EpisodeCard episode={makeEpisode()} podcastCover="cover.jpg" />);

    expect(screen.getByRole("link", { name: "单集标题" })).toHaveAttribute(
      "href",
      "https://example.com/episode",
    );
    expect(screen.getByText("E12")).toBeInTheDocument();
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

  it("queues only episode-specific cover images", () => {
    render(<EpisodeCard episode={makeEpisode()} podcastCover="cover.jpg" />);

    expect(useQueuedEpisodeImageMock).toHaveBeenCalledWith(
      expect.objectContaining({
        src: "/_next/image?url=https%3A%2F%2Fexample.com%2Fepisode.jpg&w=128&q=75",
      }),
    );
  });

  it("renders the podcast fallback cover directly without queueing it", () => {
    render(
      <EpisodeCard
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
      <EpisodeCard
        episode={makeEpisode({ image_url: "" })}
        podcastCover="cover.jpg"
        index={10}
      />,
    );

    expect(screen.getByAltText("单集标题")).toHaveAttribute("loading", "lazy");
  });

  it("expands show notes when keyboard focus enters the card", () => {
    const { container } = render(<EpisodeCard episode={makeEpisode()} />);
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
    const { rerender } = render(<EpisodeCard episode={episode} />);

    expect(screen.getAllByText("旧简介")).toHaveLength(1);

    rerender(
      <EpisodeCard
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
