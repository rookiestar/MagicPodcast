import type { Tag } from "@/types";

export type PodcastSortBy =
  | "recent_update"
  | "newest_added"
  | "episode_count"
  | "title";

export interface PodcastSortOption {
  label: string;
  value: PodcastSortBy;
}

export const PODCAST_SORT_OPTIONS: PodcastSortOption[] = [
  { label: "最近更新", value: "recent_update" },
  { label: "最新添加", value: "newest_added" },
  { label: "单集数量", value: "episode_count" },
  { label: "名称", value: "title" },
];

export function getPodcastTagsWithPodcasts(tags?: Tag[] | null) {
  return (tags || []).filter((tag) => (tag.podcast_count || 0) > 0);
}

export function getDefaultPodcastTagCount(isMobile: boolean) {
  return isMobile ? 5 : 8;
}

export function getVisiblePodcastTags(
  tags: Tag[],
  showAllTags: boolean,
  defaultTagCount: number,
) {
  return showAllTags ? tags : tags.slice(0, defaultTagCount);
}

export function hasMorePodcastTags(tags: Tag[], defaultTagCount: number) {
  return tags.length > defaultTagCount;
}

export function normalizePodcastTagIds(values: Array<number | string>) {
  const seen = new Set<number>();

  return values.reduce<number[]>((result, value) => {
    const id = typeof value === "number" ? value : Number(value);
    if (!Number.isInteger(id) || id <= 0 || seen.has(id)) {
      return result;
    }

    seen.add(id);
    result.push(id);
    return result;
  }, []);
}

export function getValidPodcastTagIds(selectedTagIds: number[], tags: Tag[]) {
  const validTagIds = new Set(tags.map((tag) => tag.id));
  return selectedTagIds.filter((id) => validTagIds.has(id));
}

export function getPodcastListDescription(
  totalCount: number,
  selectedTagCount: number,
) {
  if (totalCount <= 0) {
    return undefined;
  }

  return `共 ${totalCount} 个节目${
    selectedTagCount > 0 ? `（已选 ${selectedTagCount} 个标签）` : ""
  }`;
}

export function getPodcastListErrorMessage(error: unknown) {
  return error instanceof Error ? error.message : "加载失败";
}
