"use client";

import { memo } from "react";
import type { Tag } from "@/types";
import TagBadge from "./TagBadge";

interface SelectedTagListProps {
  selectedTags: Tag[];
  showSelectedTags: boolean;
  onRemoveTag: (tagId: number) => void;
}

function SelectedTagList({
  selectedTags,
  showSelectedTags,
  onRemoveTag,
}: SelectedTagListProps) {
  if (!showSelectedTags || selectedTags.length === 0) {
    return null;
  }

  return (
    <div className="inline-flex flex-wrap items-center gap-1.5 mb-3 align-middle">
      {selectedTags.map((tag) => (
        <TagBadge
          key={tag.id}
          tag={tag}
          size="md"
          variant="simple"
          removable
          onRemove={() => onRemoveTag(tag.id)}
        />
      ))}
    </div>
  );
}

export default memo(SelectedTagList);
