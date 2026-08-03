import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

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
  getPageSize: () => 20,
  useBreakpoint: () => ({ isMobile: false, columns: 3 }),
}));

vi.mock("@/hooks/useTagSWR", () => ({
  useTags: () => ({ tags: [] }),
}));

vi.mock("@/hooks/usePodcastSWR", () => ({
  usePodcastListInfinite: () => ({
    podcasts: [],
    totalCount: 0,
    hasMore: false,
    isLoading: false,
    isLoadingMore: false,
    isError: false,
    error: null,
    loadMore: vi.fn(),
    retryLastPage: vi.fn(),
  }),
}));

vi.mock("@/hooks/useUrlState", () => ({
  useUrlState: (_key: string, initialValue: unknown) => [initialValue, vi.fn()],
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
  it("does not repeat the global home navigation in the page toolbar", () => {
    render(<PodcastsContent />);

    expect(screen.getByRole("heading", { name: "我的订阅" })).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "返回首页" })).not.toBeInTheDocument();
  });
});
