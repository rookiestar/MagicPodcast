import { describe, expect, it } from "vitest";
import {
  arePodcastNotesControlsDisabled,
  getPodcastNotesReadOnlyText,
  getPodcastNotesSaveButtonLabel,
  hasPodcastNotes,
  shouldShowPodcastNotesAddButton,
  shouldShowPodcastNotesEditButton,
} from "../podcastNotesEditorState";

describe("podcastNotesEditorState", () => {
  it("shows the edit button only for existing notes outside editing mode", () => {
    expect(shouldShowPodcastNotesEditButton(false, true)).toBe(true);
    expect(shouldShowPodcastNotesEditButton(true, true)).toBe(false);
    expect(shouldShowPodcastNotesEditButton(false, false)).toBe(false);
  });

  it("shows the add entry only when notes are empty", () => {
    expect(shouldShowPodcastNotesAddButton(false, false)).toBe(true);
    expect(shouldShowPodcastNotesAddButton(true, false)).toBe(false);
    expect(shouldShowPodcastNotesAddButton(false, true)).toBe(false);
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
