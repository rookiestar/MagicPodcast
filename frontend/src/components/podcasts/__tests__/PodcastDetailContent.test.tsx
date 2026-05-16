import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { Episode, Podcast, Tag } from "@/types";
import PodcastDetailContent from "../PodcastDetailContent";

vi.mock("../PodcastDetailInfo", () => ({
  MobilePodcastDetailInfo: ({ podcast }: { podcast: Podcast }) => (
    <section data-testid="mobile-detail">{podcast.title}</section>
  ),
  DesktopPodcastDetailInfo: ({ podcast }: { podcast: Podcast }) => (
    <section data-testid="desktop-detail">{podcast.title}</section>
  ),
}));

vi.mock("@/components/episodes/EpisodeListSection", () => ({
  default: ({ episodes }: { episodes: Episode[] }) => (
    <section data-testid="episode-list">{episodes.length}</section>
  ),
}));

const podcast: Podcast = {
  id: 1,
  xyz_id: "podcast-1",
  title: "测试播客",
  description: "",
  author: "作者",
  cover_url: "",
  episode_count: 1,
  newest_episode_date: "2026-01-01T00:00:00Z",
  created_at: "2026-01-01T00:00:00Z",
  is_subscribed: true,
  is_dead: false,
};

const tag: Tag = {
  id: 1,
  name: "科技",
  color: "#2563eb",
};

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
  error: null,
  podcast,
  tags: [tag],
  notes: "",
  isEditingNotes: false,
  isSavingNotes: false,
  isUpdatingTags: false,
  episodes: [makeEpisode(1)],
  episodesLoading: false,
  isLoadingMore: false,
  hasMoreEpisodes: false,
  totalEpisodes: 1,
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

describe("PodcastDetailContent", () => {
  it("shows an error state instead of detail content", () => {
    render(<PodcastDetailContent {...baseProps} error="加载播客失败" />);

    expect(screen.getByRole("alert")).toHaveTextContent("加载播客失败");
    expect(screen.queryByTestId("mobile-detail")).not.toBeInTheDocument();
  });

  it("renders detail sections and episode list when podcast is available", () => {
    render(<PodcastDetailContent {...baseProps} />);

    expect(screen.getByTestId("mobile-detail")).toHaveTextContent("测试播客");
    expect(screen.getByTestId("desktop-detail")).toHaveTextContent("测试播客");
    expect(screen.getByTestId("episode-list")).toHaveTextContent("1");
  });
});
