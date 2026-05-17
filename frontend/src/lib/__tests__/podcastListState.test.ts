import { describe, expect, it } from "vitest";
import type { Tag } from "@/types";
import {
  getDefaultPodcastTagCount,
  getPodcastListDescription,
  getPodcastListErrorMessage,
  getPodcastTagsWithPodcasts,
  getValidPodcastTagIds,
  getVisiblePodcastTags,
  hasMorePodcastTags,
  normalizePodcastTagIds,
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
});
