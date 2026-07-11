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

interface TagInputComparableProps {
  selectedTags: Pick<Tag, "id">[];
  placeholder?: string;
  showSelectedTags?: boolean;
  disabled?: boolean;
}

export function areTagInputPropsEqual(
  prevProps: TagInputComparableProps,
  nextProps: TagInputComparableProps,
) {
  if (
    prevProps.showSelectedTags !== nextProps.showSelectedTags ||
    prevProps.placeholder !== nextProps.placeholder ||
    prevProps.disabled !== nextProps.disabled
  ) {
    return false;
  }

  if (prevProps.selectedTags.length !== nextProps.selectedTags.length) {
    return false;
  }

  return prevProps.selectedTags.every(
    (tag, index) => tag.id === nextProps.selectedTags[index]?.id,
  );
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

export type TagInputSubmitAction =
  | { type: "select"; tag: Tag }
  | { type: "create"; name: string }
  | { type: "none" };

interface GetTagInputSubmitActionOptions {
  availableTags: Tag[];
  selectedTags: Pick<Tag, "id">[];
  filteredTags: Tag[];
  highlightedIndex: number;
  inputValue: string;
  preferHighlightedTag: boolean;
}

export function getTagInputSubmitAction({
  availableTags,
  selectedTags,
  filteredTags,
  highlightedIndex,
  inputValue,
  preferHighlightedTag,
}: GetTagInputSubmitActionOptions): TagInputSubmitAction {
  if (preferHighlightedTag && highlightedIndex >= 0) {
    const highlightedTag = filteredTags[highlightedIndex];

    if (highlightedTag) {
      return { type: "select", tag: highlightedTag };
    }
  }

  if (!inputValue.trim()) {
    return { type: "none" };
  }

  const matchedTag = findExactTagMatch(
    availableTags,
    selectedTags,
    inputValue,
  );

  if (matchedTag) {
    return { type: "select", tag: matchedTag };
  }

  return { type: "create", name: inputValue };
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

export type TagInputKeyboardAction =
  | { type: "highlight"; index: number; preventDefault: true }
  | {
      type: "submit";
      submitAction: TagInputSubmitAction;
      preventDefault: true;
    }
  | { type: "clear"; preventDefault: false }
  | { type: "none"; preventDefault: false };

interface GetTagInputKeyboardActionOptions {
  key: string;
  showSuggestions: boolean;
  availableTags: Tag[];
  selectedTags: Pick<Tag, "id">[];
  filteredTags: Tag[];
  highlightedIndex: number;
  inputValue: string;
}

export function getTagInputKeyboardAction({
  key,
  showSuggestions,
  availableTags,
  selectedTags,
  filteredTags,
  highlightedIndex,
  inputValue,
}: GetTagInputKeyboardActionOptions): TagInputKeyboardAction {
  if (showSuggestions && filteredTags.length > 0) {
    if (key === "ArrowDown") {
      return {
        type: "highlight",
        index: getNextHighlightedIndex(
          highlightedIndex,
          filteredTags.length,
          "next",
        ),
        preventDefault: true,
      };
    }

    if (key === "ArrowUp") {
      return {
        type: "highlight",
        index: getNextHighlightedIndex(
          highlightedIndex,
          filteredTags.length,
          "previous",
        ),
        preventDefault: true,
      };
    }

    if (key === "Enter") {
      return {
        type: "submit",
        submitAction: getTagInputSubmitAction({
          availableTags,
          selectedTags,
          filteredTags,
          highlightedIndex,
          inputValue,
          preferHighlightedTag: true,
        }),
        preventDefault: true,
      };
    }

    if (key === "Escape") {
      return { type: "clear", preventDefault: false };
    }

    return { type: "none", preventDefault: false };
  }

  if (key === "Enter" && inputValue.trim() && !showSuggestions) {
    return {
      type: "submit",
      submitAction: getTagInputSubmitAction({
        availableTags,
        selectedTags,
        filteredTags,
        highlightedIndex,
        inputValue,
        preferHighlightedTag: false,
      }),
      preventDefault: true,
    };
  }

  if (key === "Escape") {
    return { type: "clear", preventDefault: false };
  }

  return { type: "none", preventDefault: false };
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

export type TagSuggestionsDisplayState =
  | { view: "matches"; trimmedInput: string; showMatchHeader: boolean }
  | { view: "create"; trimmedInput: string }
  | { view: "allSelected"; trimmedInput: string }
  | { view: "empty"; trimmedInput: string }
  | { view: "none"; trimmedInput: string };

interface GetTagSuggestionsDisplayStateOptions {
  filteredTags: Tag[];
  inputValue: string;
  availableTags: Tag[];
  loading: boolean;
}

export function getTagSuggestionsDisplayState({
  filteredTags,
  inputValue,
  availableTags,
  loading,
}: GetTagSuggestionsDisplayStateOptions): TagSuggestionsDisplayState {
  const trimmedInput = inputValue.trim();

  if (filteredTags.length > 0) {
    return {
      view: "matches",
      trimmedInput,
      showMatchHeader: trimmedInput.length > 0,
    };
  }

  if (trimmedInput) {
    return { view: "create", trimmedInput };
  }

  if (availableTags.length > 0) {
    return { view: "allSelected", trimmedInput };
  }

  if (!loading) {
    return { view: "empty", trimmedInput };
  }

  return { view: "none", trimmedInput };
}

export function pickTagCreationColor(random = Math.random) {
  const index = Math.floor(random() * TAG_CREATION_COLORS.length);
  return TAG_CREATION_COLORS[index] ?? TAG_CREATION_COLORS[0];
}
