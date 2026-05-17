import { useEffect, useState } from "react";
import { podcastApi } from "@/lib/api";
import { getErrorMessage } from "@/lib/errorMessage";
import {
  getPodcastNotesAfterCancel,
  getPodcastNotesOptimisticPayload,
  getPodcastTagChanges,
  getPodcastTagsOptimisticPayload,
  shouldSyncPodcastNotes,
} from "@/lib/podcastMetadataEditingState";
import { toast } from "@/lib/toast";
import type { Tag } from "@/types";
import { useExclusiveAsyncAction } from "./useExclusiveAsyncAction";

type MutateFn = (
  data?: unknown,
  shouldRevalidate?: boolean,
) => Promise<unknown>;

interface UsePodcastMetadataEditingOptions {
  podcastId: number;
  tags: Tag[];
  swrNotes: string;
  mutateTags: MutateFn;
  mutateNotes: MutateFn;
}

export { getPodcastTagChanges as getTagChanges };

export function usePodcastMetadataEditing({
  podcastId,
  tags,
  swrNotes,
  mutateTags,
  mutateNotes,
}: UsePodcastMetadataEditingOptions) {
  const [notes, setNotes] = useState("");
  const [isEditingNotes, setIsEditingNotes] = useState(false);
  const [isSavingNotes, setIsSavingNotes] = useState(false);
  const [isUpdatingTags, setIsUpdatingTags] = useState(false);
  const runTagsUpdate = useExclusiveAsyncAction({ isBlocked: false });
  const runNotesSave = useExclusiveAsyncAction({ isBlocked: false });

  useEffect(() => {
    if (shouldSyncPodcastNotes(swrNotes)) {
      setNotes(swrNotes);
    }
  }, [swrNotes]);

  const handleTagsChange = async (newTags: Tag[]) => {
    await runTagsUpdate(async () => {
      setIsUpdatingTags(true);

      const { toAdd, toRemove } = getPodcastTagChanges(tags, newTags);

      mutateTags(getPodcastTagsOptimisticPayload(newTags), false);

      try {
        for (const tag of toAdd) {
          await podcastApi.addTag(podcastId, tag.id);
        }

        for (const tag of toRemove) {
          await podcastApi.removeTag(podcastId, tag.id);
        }

        mutateTags();
      } catch (err) {
        toast.error(`标签更新失败: ${getErrorMessage(err)}`);
        console.error("Failed to update tags:", err);
        mutateTags();
      } finally {
        setIsUpdatingTags(false);
      }
    });
  };

  const handleNotesSave = async () => {
    await runNotesSave(async () => {
      setIsSavingNotes(true);
      mutateNotes(getPodcastNotesOptimisticPayload(podcastId, notes), false);
      setIsEditingNotes(false);

      try {
        await podcastApi.updateNotes(podcastId, notes);
        mutateNotes();
      } catch (err) {
        toast.error(`保存失败: ${getErrorMessage(err)}`);
        console.error("Failed to save notes:", err);
        mutateNotes();
        setIsEditingNotes(true);
      } finally {
        setIsSavingNotes(false);
      }
    });
  };

  const cancelNotesEdit = () => {
    if (isSavingNotes) return;

    setIsEditingNotes(false);
    setNotes(getPodcastNotesAfterCancel(swrNotes));
  };

  return {
    notes,
    setNotes,
    isEditingNotes,
    setIsEditingNotes,
    isSavingNotes,
    isUpdatingTags,
    handleTagsChange,
    handleNotesSave,
    cancelNotesEdit,
  };
}
