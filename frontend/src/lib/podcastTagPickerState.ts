import type { Tag } from "@/types";
import { getNextHighlightedIndex } from "@/lib/tagInputState";

export type PodcastTagPanelItem =
  | { type: "tag"; tag: Tag; selected: boolean }
  | { type: "create"; name: string };

function mergePodcastTagSources(
  availableTags: Tag[],
  selectedTags: Tag[],
) {
  const byId = new Map<number, Tag>();

  for (const tag of availableTags) {
    byId.set(tag.id, tag);
  }

  for (const tag of selectedTags) {
    if (!byId.has(tag.id)) {
      byId.set(tag.id, tag);
    }
  }

  return [...byId.values()];
}

export function getPodcastTagPanelItems(
  availableTags: Tag[],
  selectedTags: Tag[],
  query: string,
): PodcastTagPanelItem[] {
  const selectedIds = new Set(selectedTags.map((tag) => tag.id));
  const trimmedQuery = query.trim();
  const normalizedQuery = trimmedQuery.toLowerCase();
  const matches = mergePodcastTagSources(availableTags, selectedTags)
    .filter((tag) =>
      normalizedQuery ? tag.name.toLowerCase().includes(normalizedQuery) : true,
    )
    .map((tag) => ({
      type: "tag" as const,
      tag,
      selected: selectedIds.has(tag.id),
    }));

  if (trimmedQuery && matches.length === 0) {
    return [{ type: "create", name: trimmedQuery }];
  }

  return matches;
}

export function getPodcastTagCreateLabel(name: string) {
  return `创建『${name}』`;
}

export type PodcastTagPanelKeyboardAction =
  | { type: "highlight"; index: number; preventDefault: true }
  | { type: "confirm"; item: PodcastTagPanelItem; preventDefault: true }
  | { type: "close"; preventDefault: true }
  | { type: "none"; preventDefault: false };

export function getPodcastTagPanelKeyboardAction({
  key,
  items,
  highlightedIndex,
}: {
  key: string;
  items: PodcastTagPanelItem[];
  highlightedIndex: number;
}): PodcastTagPanelKeyboardAction {
  if (key === "Escape") {
    return { type: "close", preventDefault: true };
  }

  if (items.length > 0 && (key === "ArrowDown" || key === "ArrowUp")) {
    return {
      type: "highlight",
      index: getNextHighlightedIndex(
        highlightedIndex,
        items.length,
        key === "ArrowDown" ? "next" : "previous",
      ),
      preventDefault: true,
    };
  }

  if (key === "Enter") {
    const item =
      highlightedIndex >= 0 ? items[highlightedIndex] : items[0];

    if (item) {
      return { type: "confirm", item, preventDefault: true };
    }
  }

  return { type: "none", preventDefault: false };
}

export function togglePodcastDetailTag(tags: Tag[], tag: Tag) {
  if (tags.some((current) => current.id === tag.id)) {
    return tags.filter((current) => current.id !== tag.id);
  }

  return [...tags, tag];
}
