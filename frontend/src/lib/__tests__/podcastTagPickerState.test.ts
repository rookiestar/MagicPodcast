import { describe, expect, it } from "vitest";
import type { Tag } from "@/types";
import {
  getPodcastTagCreateLabel,
  getPodcastTagPanelItems,
  getPodcastTagPanelKeyboardAction,
  togglePodcastDetailTag,
} from "../podcastTagPickerState";

const tags: Tag[] = [
  { id: 1, name: "科技", color: "#111111" },
  { id: 2, name: "AI", color: "#222222" },
  { id: 3, name: "生活", color: "#333333" },
];

describe("podcastTagPickerState", () => {
  it("lists selected and available tags with checked state", () => {
    const items = getPodcastTagPanelItems(tags, [tags[0]], "");
    expect(items).toEqual([
      { type: "tag", tag: tags[0], selected: true },
      { type: "tag", tag: tags[1], selected: false },
      { type: "tag", tag: tags[2], selected: false },
    ]);
  });

  it("filters by search without creating automatically", () => {
    const items = getPodcastTagPanelItems(tags, [tags[0]], "科");
    expect(items).toEqual([{ type: "tag", tag: tags[0], selected: true }]);
  });

  it("offers an explicit create action only when nothing matches", () => {
    expect(getPodcastTagPanelItems(tags, [], "新品")).toEqual([
      { type: "create", name: "新品" },
    ]);
    expect(getPodcastTagCreateLabel("新品")).toBe("创建『新品』");
  });

  it("moves, confirms, and closes from the keyboard", () => {
    const items = getPodcastTagPanelItems(tags, [], "");
    expect(
      getPodcastTagPanelKeyboardAction({
        key: "ArrowDown",
        items,
        highlightedIndex: 0,
      }),
    ).toEqual({ type: "highlight", index: 1, preventDefault: true });
    expect(
      getPodcastTagPanelKeyboardAction({
        key: "Enter",
        items,
        highlightedIndex: 1,
      }),
    ).toEqual({
      type: "confirm",
      item: items[1],
      preventDefault: true,
    });
    expect(
      getPodcastTagPanelKeyboardAction({
        key: "Escape",
        items,
        highlightedIndex: 1,
      }),
    ).toEqual({ type: "close", preventDefault: true });
  });

  it("toggles selected tags without duplicating them", () => {
    expect(togglePodcastDetailTag([tags[0]], tags[1])).toEqual([
      tags[0],
      tags[1],
    ]);
    expect(togglePodcastDetailTag([tags[0], tags[1]], tags[0])).toEqual([
      tags[1],
    ]);
  });
});
