import { describe, expect, it } from "vitest";
import type { Tag } from "@/types";
import {
  getDefaultPodcastTagCount,
  getPodcastListDescription,
  getPodcastListErrorMessage,
  getPodcastListPageTotals,
  getPodcastTagsWithPodcasts,
  getUniquePodcastsFromPages,
  getValidPodcastTagIds,
  getVisiblePodcastTags,
  hasMorePodcastTags,
  normalizePodcastTagIds,
  parsePodcastListApiPayload,
  shouldStopPodcastListPagination,
} from "../podcastListState";

const tags: Tag[] = [
  { id: 1, name: "AI", color: "#111111", podcast_count: 2 },
  { id: 2, name: "News", color: "#222222", podcast_count: 0 },
  { id: 3, name: "Tech", color: "#333333", podcast_count: 4 },
];

describe("podcastListState", () => {
  it("keeps only tags that are attached to podcasts", () => {
    expect(getPodcastTagsWithPodcasts(tags).map((tag) => tag.id)).toEqual([
      1,
      3,
    ]);
  });

  it("uses different default tag counts for mobile and desktop", () => {
    expect(getDefaultPodcastTagCount(true)).toBe(5);
    expect(getDefaultPodcastTagCount(false)).toBe(8);
  });

  it("limits visible tags until the user expands them", () => {
    expect(getVisiblePodcastTags(tags, false, 2).map((tag) => tag.id)).toEqual([
      1,
      2,
    ]);
    expect(getVisiblePodcastTags(tags, true, 2).map((tag) => tag.id)).toEqual([
      1,
      2,
      3,
    ]);
    expect(hasMorePodcastTags(tags, 2)).toBe(true);
  });

  it("drops selected tag ids that are no longer valid", () => {
    expect(
      getValidPodcastTagIds([1, 2, 99], getPodcastTagsWithPodcasts(tags)),
    ).toEqual([1]);
  });

  it("normalizes tag ids read from the URL", () => {
    expect(normalizePodcastTagIds(["52", 52, "bad", "0", -1, 7])).toEqual([
      52,
      7,
    ]);
  });

  it("builds the list description from total and selected counts", () => {
    expect(getPodcastListDescription(20, 0)).toBe("共 20 个节目");
    expect(getPodcastListDescription(20, 2)).toBe(
      "共 20 个节目（已选 2 个标签）",
    );
    expect(getPodcastListDescription(0, 2)).toBeUndefined();
  });

  it("keeps readable error messages", () => {
    expect(getPodcastListErrorMessage(new Error("离线"))).toBe("离线");
    expect(getPodcastListErrorMessage("bad")).toBe("加载失败");
  });

  it("flattens podcast pages without duplicate or invalid entries", () => {
    expect(
      getUniquePodcastsFromPages([
        { podcasts: [{ id: 1, title: "A" }, { id: 2, title: "B" }] },
        { podcasts: [null, { id: 2, title: "B again" }, { id: 3, title: "C" }] },
        { podcasts: [{ id: 0, title: "Invalid" }, { id: undefined }] },
      ]),
    ).toEqual([
      { id: 1, title: "A" },
      { id: 2, title: "B" },
      { id: 3, title: "C" },
    ]);
  });

  it("decides when infinite list pagination should stop", () => {
    expect(shouldStopPodcastListPagination(null)).toBe(false);
    expect(shouldStopPodcastListPagination({ podcasts: [] })).toBe(true);
    expect(
      shouldStopPodcastListPagination({
        podcasts: [{ id: 1 }],
        pagination: { page: 2, total_pages: 3 },
      }),
    ).toBe(false);
    expect(
      shouldStopPodcastListPagination({
        podcasts: [{ id: 1 }],
        pagination: { page: 3, total_pages: 3 },
      }),
    ).toBe(true);
  });

  it("summarizes podcast list totals from the first page", () => {
    expect(
      getPodcastListPageTotals(
        [
          {
            podcasts: [{ id: 1 }],
            pagination: { page: 1, total: 25, total_pages: 3 },
          },
        ],
        1,
      ),
    ).toEqual({
      totalCount: 25,
      totalPages: 3,
      hasMore: true,
    });

    expect(getPodcastListPageTotals(undefined, 1)).toEqual({
      totalCount: 0,
      totalPages: 0,
      hasMore: false,
    });
  });

  it("parses successful podcast list payloads", () => {
    expect(
      parsePodcastListApiPayload({
        success: true,
        data: [{ id: 1, title: "A" }],
        pagination: {
          page: 1,
          page_size: 20,
          total: 1,
          total_pages: 1,
        },
      }),
    ).toEqual({
      podcasts: [{ id: 1, title: "A" }],
      pagination: {
        page: 1,
        page_size: 20,
        total: 1,
        total_pages: 1,
      },
    });
  });

  it("turns failed podcast list payloads into errors", () => {
    expect(() =>
      parsePodcastListApiPayload({
        success: false,
        error: { message: "数据库忙" },
      }),
    ).toThrow("数据库忙");
  });

  it("rejects podcast list payloads without pagination", () => {
    expect(() =>
      parsePodcastListApiPayload({
        success: true,
        data: [],
      }),
    ).toThrow("播客列表响应缺少分页信息");
  });
});
