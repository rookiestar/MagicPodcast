import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import SearchSidebar from "../SearchSidebar";
import { useSearchSidebar } from "@/hooks/useSearchSidebar";

vi.mock("@/components/podcasts/PodcastCover", () => ({
  default: ({ title }: { title: string }) => (
    <div data-testid="podcast-cover">{title}</div>
  ),
}));

vi.mock("@/hooks/useSearchSidebar", () => ({
  useSearchSidebar: vi.fn(),
}));

const useSearchSidebarMock = vi.mocked(useSearchSidebar);

function makePodcast(id: number) {
  return {
    id,
    title: `Podcast ${id}`,
    author: "Author",
    description: "Description",
    cover_url: "",
    episode_count: 10,
    newest_episode_date: "2026-01-01T00:00:00Z",
    relevance_score: 10,
    matched_fields: [],
  };
}

function makeEpisode(id: number) {
  return {
    id,
    podcast_id: 1,
    podcast_title: "Podcast 1",
    podcast_cover_url: "",
    title: `Episode ${id}`,
    show_notes: "Notes",
    published_date: null,
    duration: 0,
    relevance_score: 8,
    matched_fields: [],
  };
}

function mockSearchSidebarState(overrides = {}) {
  const podcasts = Array.from({ length: 11 }, (_, index) =>
    makePodcast(index + 1),
  );
  const episodes = Array.from({ length: 11 }, (_, index) =>
    makeEpisode(index + 1),
  );

  useSearchSidebarMock.mockReturnValue({
    query: "cast",
    setQuery: vi.fn(),
    searchType: "all",
    setSearchType: vi.fn(),
    allResults: { podcasts, episodes, pagination: null },
    results: { podcasts, episodes },
    loading: false,
    searchError: null,
    searchHistory: [],
    hasResults: true,
    isQueryTooShort: false,
    showHistory: false,
    selectHistory: vi.fn(),
    clearHistory: vi.fn(),
    ...overrides,
  });
}

describe("SearchSidebar", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("keeps podcast and episode expansion independent", () => {
    mockSearchSidebarState();

    render(<SearchSidebar isOpen onClose={vi.fn()} />);

    expect(screen.getByText("Podcast 10")).toBeInTheDocument();
    expect(screen.queryByText("Podcast 11")).not.toBeInTheDocument();
    expect(screen.queryByText("Episode 11")).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "展开全部 11 个节目" }));

    expect(screen.getByText("Podcast 11")).toBeInTheDocument();
    expect(screen.queryByText("Episode 11")).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "展开全部 11 个单集" }));

    expect(screen.getByText("Episode 11")).toBeInTheDocument();
  });

  it("shows a search error instead of an empty result state", () => {
    mockSearchSidebarState({
      allResults: { podcasts: [], episodes: [], pagination: null },
      results: { podcasts: [], episodes: [] },
      searchError: "搜索失败，请稍后重试",
      hasResults: false,
    });

    render(<SearchSidebar isOpen onClose={vi.fn()} />);

    expect(screen.getByText("搜索失败")).toBeInTheDocument();
    expect(screen.getByText("搜索失败，请稍后重试")).toBeInTheDocument();
    expect(screen.queryByText("未找到相关结果")).not.toBeInTheDocument();
  });

  it("switches search result type from the header filters", () => {
    const setSearchType = vi.fn();
    mockSearchSidebarState({ setSearchType });

    render(<SearchSidebar isOpen onClose={vi.fn()} />);

    fireEvent.click(screen.getByRole("button", { name: "节目 (11)" }));

    expect(setSearchType).toHaveBeenCalledWith("podcasts");
  });

  it("closes when pressing Escape", () => {
    mockSearchSidebarState();
    const onClose = vi.fn();

    render(<SearchSidebar isOpen onClose={onClose} />);

    fireEvent.keyDown(window, { key: "Escape" });

    expect(onClose).toHaveBeenCalledTimes(1);
  });
});
