import type { Tag } from "@/types";

const TAG_CREATION_COLORS = [
  "#3B82F6",
  "#10B981",
  "#F59E0B",
  "#EF4444",
  "#8B5CF6",
  "#EC4899",
  "#6366F1",
];

function getSelectedTagIds(selectedTags: Pick<Tag, "id">[]) {
  return new Set(selectedTags.map((tag) => tag.id));
}

export function filterTagSuggestions(
  availableTags: Tag[],
  selectedTags: Pick<Tag, "id">[],
  inputValue: string,
) {
  const selectedIds = getSelectedTagIds(selectedTags);
  const hasQuery = inputValue.trim().length > 0;
  const query = inputValue.toLowerCase();

  return availableTags.filter((tag) => {
    if (selectedIds.has(tag.id)) {
      return false;
    }

    return hasQuery ? tag.name.toLowerCase().includes(query) : true;
  });
}

export function findExactTagMatch(
  availableTags: Tag[],
  selectedTags: Pick<Tag, "id">[],
  inputValue: string,
) {
  const selectedIds = getSelectedTagIds(selectedTags);
  const query = inputValue.toLowerCase().trim();

  if (!query) {
    return undefined;
  }

  return availableTags.find(
    (tag) => !selectedIds.has(tag.id) && tag.name.toLowerCase() === query,
  );
}

export function getNextHighlightedIndex(
  currentIndex: number,
  itemCount: number,
  direction: "next" | "previous",
) {
  if (itemCount <= 0) {
    return -1;
  }

  if (direction === "next") {
    const nextIndex = currentIndex + 1;
    return nextIndex >= itemCount ? 0 : nextIndex;
  }

  const previousIndex = currentIndex - 1;
  return previousIndex < 0 ? itemCount - 1 : previousIndex;
}

export function shouldShowTagSuggestions(
  showSuggestions: boolean,
  filteredTags: Tag[],
  inputValue: string,
) {
  return (
    showSuggestions &&
    (filteredTags.length > 0 || inputValue.trim().length > 0)
  );
}

export function pickTagCreationColor(random = Math.random) {
  const index = Math.floor(random() * TAG_CREATION_COLORS.length);
  return TAG_CREATION_COLORS[index] ?? TAG_CREATION_COLORS[0];
}
