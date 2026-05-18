import { useCallback, useState } from "react";
import type { Tag } from "@/types";
import { availableTagCache } from "@/lib/tagAvailabilityCache";

export function useAvailableTags() {
  const [availableTags, setAvailableTags] = useState<Tag[]>([]);
  const [loading, setLoading] = useState(false);

  const ensureAvailableTags = useCallback(async () => {
    try {
      setLoading(true);
      setAvailableTags(await availableTagCache.load());
    } catch (error) {
      console.error("Failed to fetch tags:", error);
    } finally {
      setLoading(false);
    }
  }, []);

  const appendAvailableTag = useCallback((tag: Tag) => {
    setAvailableTags((currentTags) => {
      const nextAvailableTags = [...currentTags, tag];
      availableTagCache.replace(nextAvailableTags);
      return nextAvailableTags;
    });
  }, []);

  return {
    availableTags,
    loading,
    ensureAvailableTags,
    appendAvailableTag,
  };
}
