import { describe, expect, it } from "vitest";
import {
  getEpisodeListDisplayTotal,
  getEpisodeListFinishedMessage,
  getEpisodeListStatus,
  shouldShowEpisodeListFinished,
  shouldShowEpisodeListFooter,
  shouldShowEpisodeListHeading,
} from "../episodeListState";

describe("episodeListState", () => {
  it("separates initial loading, first-page error, empty, and ready states", () => {
    expect(
      getEpisodeListStatus({
        episodeCount: 0,
        episodesLoading: true,
      }),
    ).toBe("initial-loading");

    expect(
      getEpisodeListStatus({
        episodeCount: 0,
        episodesLoading: false,
        episodesError: "network error",
      }),
    ).toBe("initial-error");

    expect(
      getEpisodeListStatus({
        episodeCount: 0,
        episodesLoading: false,
      }),
    ).toBe("empty");

    expect(
      getEpisodeListStatus({
        episodeCount: 2,
        episodesLoading: false,
        episodesError: "page 2 failed",
      }),
    ).toBe("ready");
  });

  it("prefers API total count but falls back to loaded count", () => {
    expect(getEpisodeListDisplayTotal(218, 20)).toBe(218);
    expect(getEpisodeListDisplayTotal(0, 3)).toBe(3);
  });

  it("keeps heading hidden only while the first page is loading without a total", () => {
    expect(shouldShowEpisodeListHeading(0, true)).toBe(false);
    expect(shouldShowEpisodeListHeading(10, true)).toBe(true);
    expect(shouldShowEpisodeListHeading(0, false)).toBe(true);
  });

  it("shows footer only while more episodes can load or are loading", () => {
    expect(shouldShowEpisodeListFooter(true, false)).toBe(true);
    expect(shouldShowEpisodeListFooter(false, true)).toBe(true);
    expect(shouldShowEpisodeListFooter(false, false)).toBe(false);
  });

  it("shows the finished message only for complete successful lists", () => {
    expect(
      shouldShowEpisodeListFinished({
        episodeCount: 1,
        hasMoreEpisodes: false,
      }),
    ).toBe(true);

    expect(
      shouldShowEpisodeListFinished({
        episodeCount: 1,
        episodesError: "page 2 failed",
        hasMoreEpisodes: false,
      }),
    ).toBe(false);

    expect(
      shouldShowEpisodeListFinished({
        episodeCount: 0,
        hasMoreEpisodes: false,
      }),
    ).toBe(false);
  });

  it("builds the finished message", () => {
    expect(getEpisodeListFinishedMessage(3)).toBe("已加载全部 3 集单集");
  });
});
