import { describe, expect, it } from "vitest";
import type { Tag } from "@/types";
import {
  getPodcastNotesAfterCancel,
  getPodcastNotesOptimisticPayload,
  getPodcastTagChanges,
  getPodcastTagsOptimisticPayload,
  shouldSyncPodcastNotes,
} from "../podcastMetadataEditingState";

const techTag: Tag = { id: 1, name: "科技", color: "#2563eb" };
const businessTag: Tag = { id: 2, name: "商业", color: "#16a34a" };
const aiTag: Tag = { id: 3, name: "AI", color: "#7c3aed" };

describe("podcastMetadataEditingState", () => {
  it("detects tags to add and remove", () => {
    expect(
      getPodcastTagChanges([techTag, businessTag], [businessTag, aiTag]),
    ).toEqual({
      toAdd: [aiTag],
      toRemove: [techTag],
    });
  });

  it("builds optimistic mutation payloads", () => {
    expect(getPodcastTagsOptimisticPayload([techTag])).toEqual({
      tags: [techTag],
    });
    expect(getPodcastNotesOptimisticPayload(12, "备注")).toEqual({
      id: 12,
      notes: "备注",
    });
  });

  it("keeps note sync and cancel rules explicit", () => {
    expect(shouldSyncPodcastNotes("")).toBe(true);
    expect(shouldSyncPodcastNotes("备注")).toBe(true);
    expect(shouldSyncPodcastNotes(undefined)).toBe(false);
    expect(getPodcastNotesAfterCancel("旧备注")).toBe("旧备注");
    expect(getPodcastNotesAfterCancel("")).toBe("");
    expect(getPodcastNotesAfterCancel(undefined)).toBe("");
  });
});
