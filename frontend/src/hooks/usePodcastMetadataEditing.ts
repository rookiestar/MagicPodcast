import { useEffect, useState } from "react";
import { podcastApi } from "@/lib/api";
import { getErrorMessage } from "@/lib/errorMessage";
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

export function getTagChanges(currentTags: Tag[], newTags: Tag[]) {
  const currentIds = new Set(currentTags.map((tag) => tag.id));
  const newIds = new Set(newTags.map((tag) => tag.id));

  return {
    toAdd: newTags.filter((tag) => !currentIds.has(tag.id)),
    toRemove: currentTags.filter((tag) => !newIds.has(tag.id)),
  };
}

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
    if (swrNotes !== undefined) {
      setNotes(swrNotes);
    }
  }, [swrNotes]);

  const handleTagsChange = async (newTags: Tag[]) => {
    await runTagsUpdate(async () => {
      setIsUpdatingTags(true);

      const { toAdd, toRemove } = getTagChanges(tags, newTags);

      mutateTags({ tags: newTags }, false);

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
      mutateNotes({ id: podcastId, notes }, false);
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
    setNotes(swrNotes || "");
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
