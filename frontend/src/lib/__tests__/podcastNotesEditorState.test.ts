import { describe, expect, it } from "vitest";
import {
  arePodcastNotesControlsDisabled,
  getPodcastNotesReadOnlyText,
  getPodcastNotesSaveButtonLabel,
  hasPodcastNotes,
  shouldShowPodcastNotesEditButton,
} from "../podcastNotesEditorState";

describe("podcastNotesEditorState", () => {
  it("shows the edit button only outside editing mode", () => {
    expect(shouldShowPodcastNotesEditButton(false)).toBe(true);
    expect(shouldShowPodcastNotesEditButton(true)).toBe(false);
  });

  it("disables notes controls while saving", () => {
    expect(arePodcastNotesControlsDisabled(true)).toBe(true);
    expect(arePodcastNotesControlsDisabled(false)).toBe(false);
  });

  it("keeps save labels explicit", () => {
    expect(getPodcastNotesSaveButtonLabel(true)).toBe("保存中...");
    expect(getPodcastNotesSaveButtonLabel(false)).toBe("保存");
  });

  it("builds readonly notes text and empty state", () => {
    expect(getPodcastNotesReadOnlyText("记录")).toBe("记录");
    expect(getPodcastNotesReadOnlyText("")).toBe("暂无备注");
    expect(hasPodcastNotes("记录")).toBe(true);
    expect(hasPodcastNotes("")).toBe(false);
  });
});
