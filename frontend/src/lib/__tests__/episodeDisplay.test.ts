import { describe, expect, it } from "vitest";
import {
  formatEpisodeDuration,
  formatEpisodeFileSize,
  getEpisodeCoverDisplay,
  getEpisodeCoverImage,
  getEpisodeImageLoadDelay,
  shouldShowEpisodeImageLoader,
  shouldShowEpisodeImagePlaceholder,
  shouldShowEpisodePlayButton,
  shouldShowEpisodeShowNotes,
  shouldShowEpisodeTitleLink,
} from "../episodeDisplay";

describe("episodeDisplay", () => {
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
});
