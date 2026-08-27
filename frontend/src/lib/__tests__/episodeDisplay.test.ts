import { describe, expect, it } from "vitest";
import {
	formatEpisodeNumber,
	formatEpisodeDuration,
  formatEpisodeFileSize,
  getEpisodeCoverDisplay,
  getEpisodeCoverImage,
  getEpisodeImageLoadDelay,
  getEpisodeImageLoading,
  getEpisodeImagePriority,
  shouldShowEpisodeImageLoader,
  shouldShowEpisodeImagePlaceholder,
  planEpisodeVideoAction,
  shouldShowEpisodePlayButton,
  shouldShowEpisodeShowNotes,
  shouldShowEpisodeTitleLink,
} from "../episodeDisplay";

describe("episodeDisplay", () => {
	it("formats reliable episode numbers with a hash prefix", () => {
		expect(formatEpisodeNumber("246")).toBe("#246");
		expect(formatEpisodeNumber("E246")).toBe("#246");
		expect(formatEpisodeNumber("S10E24")).toBe("S10E24");
		expect(formatEpisodeNumber("20240438")).toBeNull();
		expect(formatEpisodeNumber("")).toBeNull();
	});

  it("chooses the episode cover before the podcast cover", () => {
    expect(
      getEpisodeCoverImage(
        { image_url: "https://example.com/episode.jpg" },
        "https://example.com/podcast.jpg",
      ),
    ).toBe("https://example.com/episode.jpg");
    expect(
      getEpisodeCoverDisplay(
        { image_url: "https://example.com/episode.jpg" },
        "https://example.com/podcast.jpg",
      ),
    ).toEqual({
      src: "https://example.com/episode.jpg",
      placeholderSrc: "https://example.com/podcast.jpg",
      shouldQueue: true,
    });
  });

  it("falls back to the podcast cover and then an empty string", () => {
    expect(
      getEpisodeCoverImage(
        { image_url: "" },
        "https://example.com/podcast.jpg",
      ),
    ).toBe("https://example.com/podcast.jpg");
    expect(getEpisodeCoverImage({ image_url: "" })).toBe("");
    expect(
      getEpisodeCoverDisplay(
        { image_url: "" },
        "https://example.com/podcast.jpg",
      ),
    ).toEqual({
      src: "https://example.com/podcast.jpg",
      placeholderSrc: "",
      shouldQueue: false,
    });
  });

  it("keeps first visible images fast and delays lower priority images", () => {
    expect(getEpisodeImageLoadDelay("high", 20)).toBe(0);
    expect(getEpisodeImageLoadDelay("medium", 2)).toBe(0);
    expect(getEpisodeImageLoadDelay("medium", 3)).toBe(200);
    expect(getEpisodeImageLoadDelay("low", 9)).toBe(0);
    expect(getEpisodeImageLoadDelay("low", 10)).toBe(500);
  });

  it("gets episode image priority and loading mode by position", () => {
    expect(getEpisodeImagePriority(0)).toBe("high");
    expect(getEpisodeImagePriority(2)).toBe("high");
    expect(getEpisodeImagePriority(3)).toBe("medium");
    expect(getEpisodeImagePriority(9)).toBe("medium");
    expect(getEpisodeImagePriority(10)).toBe("low");
    expect(getEpisodeImageLoading(0)).toBe("eager");
    expect(getEpisodeImageLoading(2)).toBe("eager");
    expect(getEpisodeImageLoading(3)).toBe("lazy");
  });

  it("formats episode duration consistently", () => {
    expect(formatEpisodeDuration(0)).toBeNull();
    expect(formatEpisodeDuration(5)).toBe("5秒");
    expect(formatEpisodeDuration(60)).toBe("1分");
    expect(formatEpisodeDuration(3661)).toBe("1小时1分1秒");
  });

  it("formats positive file sizes as megabytes", () => {
    expect(formatEpisodeFileSize(0)).toBeNull();
    expect(formatEpisodeFileSize(1024 * 1024)).toBe("1.0 MB");
    expect(formatEpisodeFileSize(2.25 * 1024 * 1024)).toBe("2.3 MB");
  });

  it("shows image loader only while a queued image is pending", () => {
    expect(shouldShowEpisodeImageLoader(false, false, true)).toBe(true);
    expect(shouldShowEpisodeImageLoader(true, false, true)).toBe(false);
    expect(shouldShowEpisodeImageLoader(false, true, true)).toBe(false);
    expect(shouldShowEpisodeImageLoader(false, false, false)).toBe(false);
  });

  it("shows the image placeholder when there is no source or loading failed", () => {
    expect(shouldShowEpisodeImagePlaceholder("", false)).toBe(true);
    expect(shouldShowEpisodeImagePlaceholder("cover.jpg", true)).toBe(true);
    expect(shouldShowEpisodeImagePlaceholder("cover.jpg", false)).toBe(false);
  });

  it("keeps optional episode actions explicit", () => {
    expect(shouldShowEpisodeTitleLink("https://example.com")).toBe(true);
    expect(shouldShowEpisodeTitleLink("")).toBe(false);
    expect(shouldShowEpisodePlayButton("https://example.com/audio.mp3")).toBe(
      true,
    );
    expect(shouldShowEpisodePlayButton("")).toBe(false);
    expect(shouldShowEpisodeShowNotes("<p>notes</p>")).toBe(true);
    expect(shouldShowEpisodeShowNotes("")).toBe(false);
  });

  it("shows the video action only for available episodes with a safe page URL", () => {
    const xyz =
      "https://www.xiaoyuzhoufm.com/episode/6a734c29ab3a91c24a1067fa?utm_source=rss";
    expect(planEpisodeVideoAction("available", xyz)).toEqual({
      show: true,
      href: xyz,
    });
    expect(planEpisodeVideoAction("unknown", xyz)).toEqual({ show: false });
    expect(planEpisodeVideoAction("unavailable", xyz)).toEqual({ show: false });
    expect(planEpisodeVideoAction("available", "javascript:alert(1)")).toEqual({
      show: false,
    });
    expect(planEpisodeVideoAction("available", "")).toEqual({ show: false });
  });
});
