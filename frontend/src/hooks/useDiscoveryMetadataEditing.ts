import { useEffect, useState } from "react";
import useSWR from "swr";
import { episodeApi, podcastApi } from "@/lib/api";
import { getErrorMessage } from "@/lib/errorMessage";
import { getPodcastTagChanges } from "@/lib/podcastMetadataEditingState";
import { cacheStrategies, swrConfig } from "@/lib/swrConfig";
import { toast } from "@/lib/toast";
import type { Tag } from "@/types";

export type DiscoveryMetadataTarget = "episode" | "podcast";

interface DiscoveryMetadata {
  tags: Tag[];
  notes: string;
}

interface UseDiscoveryMetadataEditingOptions {
  target: DiscoveryMetadataTarget;
  episodeId: number;
  podcastId: number;
}

function getTargetApi(target: DiscoveryMetadataTarget) {
  return target === "episode" ? episodeApi : podcastApi;
}

export function useDiscoveryMetadataEditing({
  target,
  episodeId,
  podcastId,
}: UseDiscoveryMetadataEditingOptions) {
  const targetId = target === "episode" ? episodeId : podcastId;
  const cacheKey = `discovery-metadata:${target}:${targetId}`;
  const targetApi = getTargetApi(target);
  const { data, error, isLoading, mutate } = useSWR<DiscoveryMetadata>(
    cacheKey,
    async () => {
      const [tags, notes] = await Promise.all([
        targetApi.getTags(targetId),
        targetApi.getNotes(targetId),
      ]);
      return { tags, notes };
    },
    { ...swrConfig, ...cacheStrategies.podcastDetail },
  );
  const [notes, setNotes] = useState("");
  const [isEditingNotes, setIsEditingNotes] = useState(false);
  const [isSavingNotes, setIsSavingNotes] = useState(false);
  const [isUpdatingTags, setIsUpdatingTags] = useState(false);

  useEffect(() => {
    setIsEditingNotes(false);
    setNotes("");
  }, [cacheKey]);

  useEffect(() => {
    if (data && !isEditingNotes) {
      setNotes(data.notes);
    }
  }, [data, isEditingNotes]);

  const handleTagsChange = async (newTags: Tag[]) => {
    if (!data || isUpdatingTags || isSavingNotes) return;

    const previousData = data;
    const { toAdd, toRemove } = getPodcastTagChanges(data.tags, newTags);
    setIsUpdatingTags(true);
    await mutate({ ...data, tags: newTags }, false);

    try {
      await Promise.all([
        ...toAdd.map((tag) => targetApi.addTag(targetId, tag.id)),
        ...toRemove.map((tag) => targetApi.removeTag(targetId, tag.id)),
      ]);
      await mutate();
    } catch (err) {
      await mutate(previousData, false);
      toast.error(`标签更新失败: ${getErrorMessage(err)}`);
    } finally {
      setIsUpdatingTags(false);
    }
  };

  const handleNotesSave = async () => {
    if (!data || isSavingNotes || isUpdatingTags) return;

    const previousData = data;
    setIsSavingNotes(true);
    await mutate({ ...data, notes }, false);
    setIsEditingNotes(false);

    try {
      await targetApi.updateNotes(targetId, notes);
      await mutate();
    } catch (err) {
      await mutate(previousData, false);
      setNotes(previousData.notes);
      setIsEditingNotes(true);
      toast.error(`保存失败: ${getErrorMessage(err)}`);
    } finally {
      setIsSavingNotes(false);
    }
  };

  const cancelNotesEdit = () => {
    if (isSavingNotes) return;
    setIsEditingNotes(false);
    setNotes(data?.notes ?? "");
  };

  return {
    tags: data?.tags ?? [],
    notes,
    setNotes,
    isLoading,
    isError: Boolean(error),
    isLoaded: Boolean(data),
    isEditingNotes,
    setIsEditingNotes,
    isSavingNotes,
    isUpdatingTags,
    handleTagsChange,
    handleNotesSave,
    cancelNotesEdit,
    reload: () => void mutate(),
  };
}
