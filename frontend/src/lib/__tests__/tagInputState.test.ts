import { describe, expect, it } from "vitest";
import type { Tag } from "@/types";
import {
  filterTagSuggestions,
  findExactTagMatch,
  getNextHighlightedIndex,
  pickTagCreationColor,
  shouldShowTagSuggestions,
} from "../tagInputState";

const tags: Tag[] = [
  { id: 1, name: "科技", color: "#2563eb" },
  { id: 2, name: "AI", color: "#16a34a" },
  { id: 3, name: "生活", color: "#f97316" },
];

describe("tagInputState", () => {
  it("filters out selected tags and matches typed text", () => {
    expect(filterTagSuggestions(tags, [tags[0]], "a")).toEqual([tags[1]]);
  });

  it("shows all unselected tags when input is empty", () => {
    expect(filterTagSuggestions(tags, [tags[1]], "")).toEqual([
      tags[0],
      tags[2],
    ]);
  });

  it("finds exact matches without returning already selected tags", () => {
    expect(findExactTagMatch(tags, [], " ai ")).toEqual(tags[1]);
    expect(findExactTagMatch(tags, [tags[1]], " ai ")).toBeUndefined();
  });

  it("wraps highlighted index in both directions", () => {
    expect(getNextHighlightedIndex(-1, 3, "next")).toBe(0);
    expect(getNextHighlightedIndex(2, 3, "next")).toBe(0);
    expect(getNextHighlightedIndex(0, 3, "previous")).toBe(2);
    expect(getNextHighlightedIndex(0, 0, "next")).toBe(-1);
  });

  it("keeps the existing suggestion visibility rule", () => {
    expect(shouldShowTagSuggestions(true, [], "")).toBeFalsy();
    expect(shouldShowTagSuggestions(true, [], "新标签")).toBeTruthy();
    expect(shouldShowTagSuggestions(true, [tags[0]], "")).toBeTruthy();
    expect(shouldShowTagSuggestions(false, [tags[0]], "新标签")).toBe(false);
  });

  it("picks colors from the creation palette", () => {
    expect(pickTagCreationColor(() => 0)).toBe("#3B82F6");
    expect(pickTagCreationColor(() => 0.99)).toBe("#6366F1");
  });
});
