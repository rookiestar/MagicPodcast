import { describe, expect, it, vi } from "vitest";
import {
  getEpisodeShowNotesToggleLabel,
  shouldAnimateEpisodeShowNotesHeight,
  shouldKeepEpisodeShowNotesPreview,
  syncEpisodeShowNotesBodyHeight,
} from "../episodeShowNotesUi";

describe("episodeShowNotesUi", () => {
  it("keeps the summary visible until a successful full document is shown", () => {
    expect(shouldKeepEpisodeShowNotesPreview(false, "idle")).toBe(true);
    expect(shouldKeepEpisodeShowNotesPreview(true, "loading")).toBe(true);
    expect(shouldKeepEpisodeShowNotesPreview(true, "error")).toBe(true);
    expect(shouldKeepEpisodeShowNotesPreview(true, "success")).toBe(false);
  });

  it("uses explicit expand and collapse labels", () => {
    expect(getEpisodeShowNotesToggleLabel(false)).toBe("展开简介");
    expect(getEpisodeShowNotesToggleLabel(true)).toBe("收起");
  });

  it("animates only when both heights are measurable and motion is allowed", () => {
    expect(shouldAnimateEpisodeShowNotesHeight(80, 240, false)).toBe(true);
    expect(shouldAnimateEpisodeShowNotesHeight(80, 240, true)).toBe(false);
    expect(shouldAnimateEpisodeShowNotesHeight(0, 240, false)).toBe(false);
    expect(shouldAnimateEpisodeShowNotesHeight(80, 80, false)).toBe(false);
  });

  it("animates from the stored previous height instead of re-reading the post-commit node", () => {
    const node = document.createElement("div");
    vi.spyOn(node, "getBoundingClientRect").mockReturnValue({
      height: 240,
      width: 320,
      top: 0,
      left: 0,
      bottom: 240,
      right: 320,
      x: 0,
      y: 0,
      toJSON() {
        return {};
      },
    } as DOMRect);

    const nextStored = syncEpisodeShowNotesBodyHeight(node, 80, false);

    expect(nextStored).toBe(240);
    expect(node.style.height).toBe("240px");
  });
});
