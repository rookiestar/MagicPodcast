import { describe, expect, it } from "vitest";
import {
  canAutoLoadMorePodcastEpisodes,
  getPodcastDetailCoverUrl,
  getPodcastDetailDescription,
  getPodcastDetailErrorMessage,
  getPodcastDetailTitle,
  parsePodcastDetailId,
} from "../podcastDetailState";

describe("podcastDetailState", () => {
  it("parses only valid detail page podcast ids", () => {
    expect(parsePodcastDetailId("42")).toBe(42);
    expect(parsePodcastDetailId(["7", "8"])).toBe(7);
    expect(parsePodcastDetailId("0")).toBeNull();
    expect(parsePodcastDetailId("-1")).toBeNull();
    expect(parsePodcastDetailId("abc")).toBeNull();
    expect(parsePodcastDetailId(undefined)).toBeNull();
  });

  it("keeps the podcast detail error message stable", () => {
    expect(getPodcastDetailErrorMessage(true)).toBe("加载播客失败");
    expect(getPodcastDetailErrorMessage(false)).toBeNull();
  });

  it("builds the toolbar title and description", () => {
    expect(getPodcastDetailTitle({ title: "节目" })).toBe("节目");
    expect(getPodcastDetailTitle(null)).toBe("播客详情");
    expect(getPodcastDetailDescription({ episode_count: 12 }, 3)).toBe(
      "共 12 个单集",
    );
    expect(getPodcastDetailDescription({ episode_count: 0 }, 3)).toBe(
      "共 3 个单集",
    );
    expect(getPodcastDetailDescription({ episode_count: 0 }, 0)).toBeUndefined();
    expect(getPodcastDetailDescription(null, 3)).toBeUndefined();
  });

  it("uses the custom cover before the source cover", () => {
    expect(
      getPodcastDetailCoverUrl({
        custom_cover_url: "custom.jpg",
        cover_url: "source.jpg",
      }),
    ).toBe("custom.jpg");
    expect(
      getPodcastDetailCoverUrl({
        custom_cover_url: "",
        cover_url: "source.jpg",
      }),
    ).toBe("source.jpg");
    expect(getPodcastDetailCoverUrl(null)).toBeUndefined();
  });

  it("allows automatic episode loading only from an idle loaded list", () => {
    expect(
      canAutoLoadMorePodcastEpisodes({
        episodeCount: 1,
        episodesLoading: false,
        isLoadingMore: false,
        hasMoreEpisodes: true,
      }),
    ).toBe(true);

    expect(
      canAutoLoadMorePodcastEpisodes({
        episodeCount: 0,
        episodesLoading: false,
        isLoadingMore: false,
        hasMoreEpisodes: true,
      }),
    ).toBe(false);
    expect(
      canAutoLoadMorePodcastEpisodes({
        episodeCount: 1,
        episodesLoading: true,
        isLoadingMore: false,
        hasMoreEpisodes: true,
      }),
    ).toBe(false);
    expect(
      canAutoLoadMorePodcastEpisodes({
        episodeCount: 1,
        episodesLoading: false,
        isLoadingMore: true,
        hasMoreEpisodes: true,
      }),
    ).toBe(false);
    expect(
      canAutoLoadMorePodcastEpisodes({
        episodeCount: 1,
        episodesLoading: false,
        isLoadingMore: false,
        hasMoreEpisodes: false,
      }),
    ).toBe(false);
    expect(
      canAutoLoadMorePodcastEpisodes({
        episodeCount: 1,
        episodesLoading: false,
        isLoadingMore: false,
        hasMoreEpisodes: true,
        episodesError: "page failed",
      }),
    ).toBe(false);
  });
});
