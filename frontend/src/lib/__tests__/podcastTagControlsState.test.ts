import { describe, expect, it } from "vitest";
import type { Tag } from "@/types";
import {
  getRemainingPodcastDetailTagCount,
  getVisiblePodcastDetailTags,
  removePodcastDetailTag,
  shouldShowPodcastTagSummary,
} from "../podcastTagControlsState";

const tags: Tag[] = [
  { id: 1, name: "AI", color: "#111111" },
  { id: 2, name: "Tech", color: "#222222" },
  { id: 3, name: "News", color: "#333333" },
  { id: 4, name: "Culture", color: "#444444" },
];

describe("podcastTagControlsState", () => {
  it("shows tag summary only when tags exist", () => {
    expect(shouldShowPodcastTagSummary(tags)).toBe(true);
    expect(shouldShowPodcastTagSummary([])).toBe(false);
  });

  it("limits visible mobile detail tags", () => {
    expect(getVisiblePodcastDetailTags(tags).map((tag) => tag.id)).toEqual([
      1,
      2,
      3,
    ]);
    expect(getVisiblePodcastDetailTags(tags, 2).map((tag) => tag.id)).toEqual([
      1,
      2,
    ]);
  });

  it("calculates remaining mobile tag count", () => {
    expect(getRemainingPodcastDetailTagCount(tags)).toBe(1);
    expect(getRemainingPodcastDetailTagCount(tags, 4)).toBe(0);
    expect(getRemainingPodcastDetailTagCount(tags, 10)).toBe(0);
  });

  it("removes one detail tag by id", () => {
    expect(removePodcastDetailTag(tags, 2).map((tag) => tag.id)).toEqual([
      1,
      3,
      4,
    ]);
    expect(removePodcastDetailTag(tags, 99)).toEqual(tags);
  });
});
