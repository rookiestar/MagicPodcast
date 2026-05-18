import type { Tag } from "@/types";
import { tagApi } from "@/lib/api";

type FetchAvailableTags = () => Promise<Tag[]>;

export function createAvailableTagCache(fetchAvailableTags: FetchAvailableTags) {
  let cachedAvailableTags: Tag[] | null = null;
  let pendingAvailableTagsRequest: Promise<Tag[]> | null = null;

  return {
    load() {
      if (cachedAvailableTags) {
        return Promise.resolve(cachedAvailableTags);
      }

      if (!pendingAvailableTagsRequest) {
        pendingAvailableTagsRequest = fetchAvailableTags()
          .then((tags) => {
            cachedAvailableTags = tags;
            return tags;
          })
          .finally(() => {
            pendingAvailableTagsRequest = null;
          });
      }

      return pendingAvailableTagsRequest;
    },

    replace(tags: Tag[]) {
      cachedAvailableTags = tags;
      return cachedAvailableTags;
    },

    clear() {
      cachedAvailableTags = null;
      pendingAvailableTagsRequest = null;
    },
  };
}

export const availableTagCache = createAvailableTagCache(() => tagApi.list());
