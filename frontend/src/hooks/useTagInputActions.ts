import { useCallback } from "react";
import type { Tag } from "@/types";
import { tagApi } from "@/lib/api";
import {
  pickTagCreationColor,
  type TagInputSubmitAction,
} from "@/lib/tagInputState";

interface UseTagInputActionsOptions {
  selectedTags: Tag[];
  onTagsChange: (tags: Tag[]) => void;
  disabled: boolean;
  appendAvailableTag: (tag: Tag) => void;
  resetAfterTagChange: () => void;
}

export function useTagInputActions({
  selectedTags,
  onTagsChange,
  disabled,
  appendAvailableTag,
  resetAfterTagChange,
}: UseTagInputActionsOptions) {
  const addTag = useCallback(
    (tag: Tag) => {
      if (disabled) return;

      onTagsChange([...selectedTags, tag]);
      resetAfterTagChange();
    },
    [disabled, onTagsChange, resetAfterTagChange, selectedTags],
  );

  const createTag = useCallback(
    async (name: string) => {
      if (disabled) return;

      try {
        const newTag = await tagApi.create({
          name: name.trim(),
          color: pickTagCreationColor(),
        });

        onTagsChange([...selectedTags, newTag]);
        appendAvailableTag(newTag);
        resetAfterTagChange();
      } catch (err) {
        console.error("创建标签失败:", err);
      }
    },
    [
      appendAvailableTag,
      disabled,
      onTagsChange,
      resetAfterTagChange,
      selectedTags,
    ],
  );

  const removeTag = useCallback(
    (tagId: number) => {
      if (disabled) return;

      onTagsChange(selectedTags.filter((tag) => tag.id !== tagId));
    },
    [disabled, onTagsChange, selectedTags],
  );

  const submitTagAction = useCallback(
    (action: TagInputSubmitAction) => {
      if (action.type === "select") {
        addTag(action.tag);
        return;
      }

      if (action.type === "create") {
        void createTag(action.name);
      }
    },
    [addTag, createTag],
  );

  return {
    addTag,
    createTag,
    removeTag,
    submitTagAction,
  };
}
