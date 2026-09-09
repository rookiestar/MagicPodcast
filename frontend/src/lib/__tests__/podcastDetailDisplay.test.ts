import { describe, expect, it } from "vitest";
import {
  formatPodcastDetailMetaLine,
  formatPodcastLatestEpisodeDurationLabel,
  formatPodcastNewestEpisodeDate,
  getPodcastDescriptionHtml,
  getPodcastDetailInfoCoverUrl,
  shouldOfferPodcastDescriptionToggle,
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

  it("formats newest episode dates to the minute and falls back for invalid values", () => {
    expect(formatPodcastNewestEpisodeDate()).toBe("未知");
    expect(formatPodcastNewestEpisodeDate("not-a-date")).toBe("未知");
    const local = new Date(2026, 7, 29, 13, 36, 45);
    expect(formatPodcastNewestEpisodeDate(local.toISOString())).toBe(
      "2026/08/29 13:36",
    );
    expect(formatPodcastNewestEpisodeDate(local.toISOString())).toMatch(
      /^\d{4}\/\d{2}\/\d{2} \d{2}:\d{2}$/,
    );
  });

  it("builds a compact metadata line", () => {
    expect(
      formatPodcastDetailMetaLine("是柚子呀23333", 64, "not-a-date"),
    ).toBe("是柚子呀23333 · 64 集 · 更新于 未知");
    expect(formatPodcastDetailMetaLine("", 0, null)).toBe(
      "0 集 · 更新于 未知",
    );
  });

  it("offers a description toggle only for long copy", () => {
    expect(shouldOfferPodcastDescriptionToggle("短简介")).toBe(false);
    expect(shouldOfferPodcastDescriptionToggle("")).toBe(false);
    expect(
      shouldOfferPodcastDescriptionToggle("很长的节目简介。".repeat(30)),
    ).toBe(true);
    expect(
      shouldOfferPodcastDescriptionToggle(
        "<p>一行</p><p>二行</p><p>三行</p><p>四行</p><p>五行</p><p>六行</p><p>七行</p>",
      ),
    ).toBe(true);
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
