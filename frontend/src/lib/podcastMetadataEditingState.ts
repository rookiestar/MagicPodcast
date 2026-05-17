import type { Tag } from "@/types";

export function getPodcastTagChanges(currentTags: Tag[], newTags: Tag[]) {
  const currentIds = new Set(currentTags.map((tag) => tag.id));
  const newIds = new Set(newTags.map((tag) => tag.id));

  return {
    toAdd: newTags.filter((tag) => !currentIds.has(tag.id)),
    toRemove: currentTags.filter((tag) => !newIds.has(tag.id)),
  };
}

export function getPodcastTagsOptimisticPayload(newTags: Tag[]) {
  return { tags: newTags };
}

export function getPodcastNotesOptimisticPayload(
  podcastId: number,
  notes: string,
) {
  return { id: podcastId, notes };
}

export function shouldSyncPodcastNotes(swrNotes: string | undefined) {
  return swrNotes !== undefined;
}

export function getPodcastNotesAfterCancel(swrNotes: string | undefined) {
  return swrNotes || "";
}
