import { describe, expect, it } from "vitest";
import {
  formatPodcastLatestEpisodeDurationLabel,
  formatPodcastNewestEpisodeDate,
  getPodcastDescriptionHtml,
  getPodcastDetailInfoCoverUrl,
  shouldShowPodcastLatestEpisodePlayButton,
  shouldShowPodcastPopularityBadge,
  shouldShowPodcastWebsiteLink,
} from "../podcastDetailDisplay";

describe("podcastDetailDisplay", () => {
  it("uses the custom cover before the source cover", () => {
    expect(
      getPodcastDetailInfoCoverUrl({
        custom_cover_url: "custom.jpg",
        cover_url: "source.jpg",
      }),
    ).toBe("custom.jpg");
    expect(
      getPodcastDetailInfoCoverUrl({
        custom_cover_url: "",
        cover_url: "source.jpg",
      }),
    ).toBe("source.jpg");
  });

  it("formats newest episode dates and falls back for invalid values", () => {
    expect(formatPodcastNewestEpisodeDate()).toBe("未知");
    expect(formatPodcastNewestEpisodeDate("not-a-date")).toBe("未知");
    expect(formatPodcastNewestEpisodeDate("2026-01-01T00:00:00Z")).toContain(
      "2026",
    );
  });

  it("formats latest episode duration labels", () => {
    expect(formatPodcastLatestEpisodeDurationLabel()).toBeNull();
    expect(formatPodcastLatestEpisodeDurationLabel(0)).toBeNull();
    expect(formatPodcastLatestEpisodeDurationLabel(125)).toBe("2分5秒");
    expect(formatPodcastLatestEpisodeDurationLabel(125.9)).toBe("2分5秒");
  });

  it("keeps description fallback explicit", () => {
    expect(getPodcastDescriptionHtml("简介")).toBe("简介");
    expect(getPodcastDescriptionHtml("")).toBe("暂无简介");
    expect(getPodcastDescriptionHtml(null)).toBe("暂无简介");
  });

  it("keeps optional detail sections explicit", () => {
    expect(shouldShowPodcastWebsiteLink("https://example.com")).toBe(true);
    expect(shouldShowPodcastWebsiteLink("")).toBe(false);
    expect(shouldShowPodcastPopularityBadge(7)).toBe(true);
    expect(shouldShowPodcastPopularityBadge(6.9)).toBe(false);
    expect(shouldShowPodcastPopularityBadge(0)).toBe(false);
    expect(shouldShowPodcastLatestEpisodePlayButton("audio.mp3")).toBe(true);
    expect(shouldShowPodcastLatestEpisodePlayButton("")).toBe(false);
  });
});
