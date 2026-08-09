import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const {
  mockUseBreakpoint,
  mockUsePodcastListInfinite,
  mockUseUrlState,
} = vi.hoisted(() => ({
  mockUseBreakpoint: vi.fn(),
  mockUsePodcastListInfinite: vi.fn(),
  mockUseUrlState: vi.fn(),
}));

vi.mock("@/components/layout/PageLayout", () => ({
  default: ({ toolbar, children }: any) => (
    <div>
      <div data-testid="toolbar">
        {toolbar?.breadcrumbs?.map((item: any) => (
          <a key={item.label} href={item.href}>
            {item.label}
          </a>
        ))}
        {toolbar?.title && <h1>{toolbar.title}</h1>}
        {toolbar?.rightContent}
      </div>
      {children}
    </div>
  ),
}));

vi.mock("@/contexts/SearchContext", () => ({
  useSearch: () => ({ openSearch: vi.fn() }),
}));

vi.mock("@/hooks/useBreakpoint", () => ({
  getPageSize: (columns: number) => {
    if (columns === 1) return 10;
    if (columns === 2) return 8;
    if (columns === 3) return 9;
    if (columns === 4) return 12;
    return 15;
  },
  getPageSizeForViewportWidth: (width: number) => {
    if (width < 640) return 10;
    if (width < 768) return 8;
    if (width < 1024) return 9;
    if (width < 1280) return 12;
    return 15;
  },
  useBreakpoint: mockUseBreakpoint,
}));

vi.mock("@/hooks/useTagSWR", () => ({
  useTags: () => ({ tags: [] }),
}));

vi.mock("@/hooks/usePodcastSWR", () => ({
  usePodcastListInfinite: mockUsePodcastListInfinite,
}));

vi.mock("@/hooks/useUrlState", () => ({
  useUrlState: mockUseUrlState,
}));

vi.mock("@/components/podcasts/PodcastListResults", () => ({
  default: () => <div data-testid="podcast-list-results" />,
}));

vi.mock("@/components/podcasts/PodcastListSortControls", () => ({
  default: () => <div data-testid="podcast-sort-controls" />,
}));

vi.mock("@/components/podcasts/PodcastListStates", () => ({
  MobilePodcastListSummary: () => <div data-testid="podcast-list-summary" />,
}));

vi.mock("@/components/podcasts/PodcastTagFilter", () => ({
  default: () => <div data-testid="podcast-tag-filter" />,
}));

vi.mock("@/lib/podcastListState", () => ({
  PODCAST_SORT_OPTIONS: [],
  getDefaultPodcastTagCount: () => 5,
  getPodcastListDescription: () => "共 0 个节目",
  getPodcastListErrorMessage: () => "",
  getPodcastTagsWithPodcasts: (tags: unknown[]) => tags,
  getValidPodcastTagIds: () => [],
  getVisiblePodcastTags: (tags: unknown[]) => tags,
  hasMorePodcastTags: () => false,
  normalizePodcastTagIds: (values: unknown[]) => values,
}));

vi.mock("@/lib/podcastListScrollState", () => ({
  clearPodcastListScrollSnapshot: vi.fn(),
  getPodcastListScrollRestoreAction: () => "none",
  getPodcastListStateKey: () => "podcasts",
  readPodcastListScrollSnapshot: () => null,
  restorePodcastListScroll: vi.fn(),
}));

import PodcastsContent from "../PodcastsContent";

describe("podcast list page navigation", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUseBreakpoint.mockReturnValue({
      isMobile: false,
      columns: 3,
      isReady: true,
    });
    mockUseUrlState.mockImplementation(
      (_key: string, initialValue: unknown) => [initialValue, vi.fn()],
    );
    mockUsePodcastListInfinite.mockReturnValue({
      podcasts: [],
      totalCount: 0,
      hasMore: false,
      isLoading: false,
      isLoadingMore: false,
      isError: false,
      error: null,
      loadMore: vi.fn(),
      retryLastPage: vi.fn(),
    });
  });

  it("does not repeat the global home navigation in the page toolbar", () => {
    render(<PodcastsContent />);

    expect(screen.getByRole("heading", { name: "我的订阅" })).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "返回首页" })).not.toBeInTheDocument();
  });

  it("starts the first list request with the viewport page size before the breakpoint effect settles", () => {
    Object.defineProperty(window, "innerWidth", {
      configurable: true,
      value: 375,
    });
    mockUseBreakpoint.mockReturnValue({
      isMobile: false,
      columns: 5,
      isReady: false,
    });

    render(<PodcastsContent />);

    expect(mockUsePodcastListInfinite).toHaveBeenCalledWith(
      expect.objectContaining({
        enabled: true,
        page_size: 10,
        sort_by: "recent_update",
      }),
    );
  });

  it("forwards the server page only to the default unfiltered list", () => {
    const initialPage = {
      podcasts: [],
      pagination: {
        page: 1,
        page_size: 10,
        total: 0,
        total_pages: 0,
      },
    };

    render(<PodcastsContent initialPage={initialPage} />);

    expect(mockUsePodcastListInfinite).toHaveBeenCalledWith(
      expect.objectContaining({ initialPage }),
    );

    mockUseUrlState.mockImplementation(
      (key: string, initialValue: unknown) => [
        key === "sort_by" ? "title" : initialValue,
        vi.fn(),
      ],
    );
    mockUsePodcastListInfinite.mockClear();

    render(<PodcastsContent initialPage={initialPage} />);

    expect(mockUsePodcastListInfinite).toHaveBeenCalledWith(
      expect.objectContaining({ initialPage: undefined, sort_by: "title" }),
    );
  });
});
