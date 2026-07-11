import { describe, expect, it } from "vitest";
import type { Tag } from "@/types";
import {
  areTagBadgePropsEqual,
  getTagBadgeSizeClass,
  getTagRemoveButtonClass,
  getTagRemoveButtonStyle,
  getTagRemoveTitle,
  shouldStopTagRemovePropagation,
} from "../tagBadgeState";

const tag: Tag = {
  id: 1,
  name: "科技",
  color: "#2563eb",
};

describe("tagBadgeState", () => {
  it("returns the existing badge size classes", () => {
    expect(getTagBadgeSizeClass("sm")).toBe("text-xs px-2 py-0.5");
    expect(getTagBadgeSizeClass("md")).toBe("text-sm px-3 py-1");
    expect(getTagBadgeSizeClass("lg")).toBe("text-base px-4 py-1.5");
  });

  it("builds remove button titles and propagation rules", () => {
    expect(getTagRemoveTitle("科技")).toBe('移除 "科技" 标签');
    expect(shouldStopTagRemovePropagation("simple")).toBe(true);
    expect(shouldStopTagRemovePropagation("colorful")).toBe(false);
  });

  it("returns remove button classes and styles per variant", () => {
    expect(getTagRemoveButtonClass("simple")).toContain("hover:bg-slate-300");
    expect(getTagRemoveButtonClass("colorful")).toContain("hover:bg-white/50");

    expect(getTagRemoveButtonStyle(tag.color, "simple")).toEqual({
      minWidth: "44px",
      minHeight: "44px",
    });
    expect(getTagRemoveButtonStyle(tag.color, "colorful")).toEqual({
      color: tag.color,
      minWidth: "44px",
      minHeight: "44px",
    });
  });

  it("compares badge props by rendered fields", () => {
    expect(
      areTagBadgePropsEqual(
        {
          tag,
          size: "md",
          removable: true,
          variant: "simple",
        },
        {
          tag: { ...tag },
          size: "md",
          removable: true,
          variant: "simple",
        },
      ),
    ).toBe(true);

    expect(
      areTagBadgePropsEqual(
        {
          tag,
          size: "md",
          removable: true,
          variant: "simple",
        },
        {
          tag: { ...tag, color: "#000000" },
          size: "md",
          removable: true,
          variant: "simple",
        },
      ),
    ).toBe(false);
  });
});
