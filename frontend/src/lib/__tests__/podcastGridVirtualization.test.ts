import { describe, expect, it } from "vitest";
import {
  getPodcastGridCoverPriority,
  getPodcastGridEstimateRowHeight,
  getLastVisiblePodcastRowIndex,
  getPodcastGridRowGap,
  shouldLoadMorePodcastRows,
} from "../podcastGridVirtualization";

describe("podcastGridVirtualization", () => {
  it("only marks the estimated first screen as eager", () => {
    expect(getPodcastGridCoverPriority(4, 5, false)).toBe("high");
    expect(getPodcastGridCoverPriority(5, 5, false)).toBe("low");
    expect(getPodcastGridCoverPriority(4, 1, true)).toBe("high");
    expect(getPodcastGridCoverPriority(5, 1, true)).toBe("low");
  });

  it("keeps row height estimates close to the rendered card shape", () => {
    expect(getPodcastGridEstimateRowHeight(true)).toBe(124);
    expect(getPodcastGridEstimateRowHeight(false)).toBe(482);
  });

  it("keeps row gaps aligned with the responsive grid spacing", () => {
    expect(getPodcastGridRowGap(true)).toBe(12);
    expect(getPodcastGridRowGap(false)).toBe(24);
  });

  it("ignores overscan rows below the visible viewport", () => {
    expect(
      getLastVisiblePodcastRowIndex(
        [
          { index: 0, start: 100, end: 360 },
          { index: 1, start: 400, end: 680 },
          { index: 2, start: 900, end: 1180 },
        ],
        720,
      ),
    ).toBe(1);
  });

  it("does not count a row as visible when only its top edge entered", () => {
    expect(
      getLastVisiblePodcastRowIndex(
        [
          { index: 0, start: 0, end: 480 },
          { index: 1, start: 506, end: 986 },
          { index: 2, start: 1012, end: 1492 },
        ],
        1100,
      ),
    ).toBe(1);
  });

  it("returns null when no virtual row is visible yet", () => {
    expect(
      getLastVisiblePodcastRowIndex([{ index: 0, start: 900 }], 720),
    ).toBeNull();
  });

  it("loads more rows when the visible window reaches the buffer", () => {
    expect(
      shouldLoadMorePodcastRows({
        lastVisibleRowIndex: 7,
        rowCount: 10,
        hasMore: true,
        isLoading: false,
      }),
    ).toBe(true);
  });

  it("does not load more while far from the end", () => {
    expect(
      shouldLoadMorePodcastRows({
        lastVisibleRowIndex: 5,
        rowCount: 10,
        hasMore: true,
        isLoading: false,
      }),
    ).toBe(false);
  });

  it("does not load more without a visible row, more data, or idle state", () => {
    expect(
      shouldLoadMorePodcastRows({
        lastVisibleRowIndex: null,
        rowCount: 10,
        hasMore: true,
        isLoading: false,
      }),
    ).toBe(false);
    expect(
      shouldLoadMorePodcastRows({
        lastVisibleRowIndex: 9,
        rowCount: 10,
        hasMore: false,
        isLoading: false,
      }),
    ).toBe(false);
    expect(
      shouldLoadMorePodcastRows({
        lastVisibleRowIndex: 9,
        rowCount: 10,
        hasMore: true,
        isLoading: true,
      }),
    ).toBe(false);
  });
});
