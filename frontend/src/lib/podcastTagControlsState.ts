import type { Tag } from "@/types";

export const MOBILE_PODCAST_DETAIL_TAG_LIMIT = 3;

export function shouldShowPodcastTagSummary(tags: Tag[]) {
  return tags.length > 0;
}

export function getVisiblePodcastDetailTags(
  tags: Tag[],
  limit = MOBILE_PODCAST_DETAIL_TAG_LIMIT,
) {
  return tags.slice(0, limit);
}

export function getRemainingPodcastDetailTagCount(
  tags: Tag[],
  limit = MOBILE_PODCAST_DETAIL_TAG_LIMIT,
) {
  return Math.max(0, tags.length - limit);
}

export function removePodcastDetailTag(tags: Tag[], tagId: number) {
  return tags.filter((tag) => tag.id !== tagId);
}
