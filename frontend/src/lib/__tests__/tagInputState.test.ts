import { describe, expect, it } from "vitest";
import type { Tag } from "@/types";
import {
  areTagInputPropsEqual,
  filterTagSuggestions,
  findExactTagMatch,
  getTagSuggestionsDisplayState,
  getTagInputKeyboardAction,
  getNextHighlightedIndex,
  getTagInputSubmitAction,
  pickTagCreationColor,
  shouldShowTagSuggestions,
} from "../tagInputState";

const tags: Tag[] = [
  { id: 1, name: "科技", color: "#2563eb" },
  { id: 2, name: "AI", color: "#16a34a" },
  { id: 3, name: "生活", color: "#f97316" },
];

describe("tagInputState", () => {
  it("compares tag input props by selected tag ids and visible options", () => {
    const sameIdsWithDifferentDisplay = [
      { ...tags[0], name: "另一个名称" },
      { ...tags[1], color: "#000000" },
    ];

    expect(
      areTagInputPropsEqual(
        {
          selectedTags: [tags[0], tags[1]],
          placeholder: "输入标签",
          showSelectedTags: true,
          disabled: false,
        },
        {
          selectedTags: sameIdsWithDifferentDisplay,
          placeholder: "输入标签",
          showSelectedTags: true,
          disabled: false,
        },
      ),
    ).toBe(true);

    expect(
      areTagInputPropsEqual(
        {
          selectedTags: [tags[0], tags[1]],
          placeholder: "输入标签",
          showSelectedTags: true,
          disabled: false,
        },
        {
          selectedTags: [tags[1], tags[0]],
          placeholder: "输入标签",
          showSelectedTags: true,
          disabled: false,
        },
      ),
    ).toBe(false);
  });

  it("rerenders tag input when visible options change", () => {
    expect(
      areTagInputPropsEqual(
        {
          selectedTags: [tags[0]],
          placeholder: "输入标签",
          showSelectedTags: true,
          disabled: false,
        },
        {
          selectedTags: [tags[0]],
          placeholder: "输入标签",
          showSelectedTags: false,
          disabled: false,
        },
      ),
    ).toBe(false);

    expect(
      areTagInputPropsEqual(
        {
          selectedTags: [tags[0]],
          placeholder: "输入标签",
          showSelectedTags: true,
          disabled: false,
        },
        {
          selectedTags: [tags[0]],
          placeholder: "新占位",
          showSelectedTags: true,
          disabled: true,
        },
      ),
    ).toBe(false);
  });

  it("filters out selected tags and matches typed text", () => {
    expect(filterTagSuggestions(tags, [tags[0]], "a")).toEqual([tags[1]]);
  });

  it("shows all unselected tags when input is empty", () => {
    expect(filterTagSuggestions(tags, [tags[1]], "")).toEqual([
      tags[0],
      tags[2],
    ]);
  });

  it("finds exact matches without returning already selected tags", () => {
    expect(findExactTagMatch(tags, [], " ai ")).toEqual(tags[1]);
    expect(findExactTagMatch(tags, [tags[1]], " ai ")).toBeUndefined();
  });

  it("chooses the highlighted tag before matching typed text", () => {
    expect(
      getTagInputSubmitAction({
        availableTags: tags,
        selectedTags: [],
        filteredTags: [tags[1], tags[2]],
        highlightedIndex: 1,
        inputValue: "ai",
        preferHighlightedTag: true,
      }),
    ).toEqual({ type: "select", tag: tags[2] });
  });

  it("matches existing tags before creating a new tag", () => {
    expect(
      getTagInputSubmitAction({
        availableTags: tags,
        selectedTags: [],
        filteredTags: [],
        highlightedIndex: -1,
        inputValue: " ai ",
        preferHighlightedTag: false,
      }),
    ).toEqual({ type: "select", tag: tags[1] });

    expect(
      getTagInputSubmitAction({
        availableTags: tags,
        selectedTags: [],
        filteredTags: [],
        highlightedIndex: -1,
        inputValue: "新标签",
        preferHighlightedTag: false,
      }),
    ).toEqual({ type: "create", name: "新标签" });
  });

  it("does nothing when submitting blank input", () => {
    expect(
      getTagInputSubmitAction({
        availableTags: tags,
        selectedTags: [],
        filteredTags: [],
        highlightedIndex: -1,
        inputValue: " ",
        preferHighlightedTag: true,
      }),
    ).toEqual({ type: "none" });
  });

  it("wraps highlighted index in both directions", () => {
    expect(getNextHighlightedIndex(-1, 3, "next")).toBe(0);
    expect(getNextHighlightedIndex(2, 3, "next")).toBe(0);
    expect(getNextHighlightedIndex(0, 3, "previous")).toBe(2);
    expect(getNextHighlightedIndex(0, 0, "next")).toBe(-1);
  });

  it("maps arrow keys to highlighted indexes while suggestions are open", () => {
    expect(
      getTagInputKeyboardAction({
        key: "ArrowDown",
        showSuggestions: true,
        availableTags: tags,
        selectedTags: [],
        filteredTags: tags,
        highlightedIndex: 2,
        inputValue: "",
      }),
    ).toEqual({ type: "highlight", index: 0, preventDefault: true });

    expect(
      getTagInputKeyboardAction({
        key: "ArrowUp",
        showSuggestions: true,
        availableTags: tags,
        selectedTags: [],
        filteredTags: tags,
        highlightedIndex: 0,
        inputValue: "",
      }),
    ).toEqual({ type: "highlight", index: 2, preventDefault: true });
  });

  it("maps Enter to submit actions using the current suggestion state", () => {
    expect(
      getTagInputKeyboardAction({
        key: "Enter",
        showSuggestions: true,
        availableTags: tags,
        selectedTags: [],
        filteredTags: [tags[1], tags[2]],
        highlightedIndex: 0,
        inputValue: "生活",
      }),
    ).toEqual({
      type: "submit",
      submitAction: { type: "select", tag: tags[1] },
      preventDefault: true,
    });

    expect(
      getTagInputKeyboardAction({
        key: "Enter",
        showSuggestions: false,
        availableTags: tags,
        selectedTags: [],
        filteredTags: [],
        highlightedIndex: -1,
        inputValue: "新标签",
      }),
    ).toEqual({
      type: "submit",
      submitAction: { type: "create", name: "新标签" },
      preventDefault: true,
    });
  });

  it("keeps the existing Enter behavior when no matches are visible", () => {
    expect(
      getTagInputKeyboardAction({
        key: "Enter",
        showSuggestions: true,
        availableTags: tags,
        selectedTags: [],
        filteredTags: [],
        highlightedIndex: -1,
        inputValue: "新标签",
      }),
    ).toEqual({ type: "none", preventDefault: false });
  });

  it("maps Escape to clearing the input state", () => {
    expect(
      getTagInputKeyboardAction({
        key: "Escape",
        showSuggestions: true,
        availableTags: tags,
        selectedTags: [],
        filteredTags: tags,
        highlightedIndex: 0,
        inputValue: "AI",
      }),
    ).toEqual({ type: "clear", preventDefault: false });
  });

  it("keeps the existing suggestion visibility rule", () => {
    expect(shouldShowTagSuggestions(true, [], "")).toBeFalsy();
    expect(shouldShowTagSuggestions(true, [], "新标签")).toBeTruthy();
    expect(shouldShowTagSuggestions(true, [tags[0]], "")).toBeTruthy();
    expect(shouldShowTagSuggestions(false, [tags[0]], "新标签")).toBe(false);
  });

  it("chooses the dropdown display state", () => {
    expect(
      getTagSuggestionsDisplayState({
        filteredTags: [tags[0]],
        inputValue: "科",
        availableTags: tags,
        loading: false,
      }),
    ).toEqual({
      view: "matches",
      trimmedInput: "科",
      showMatchHeader: true,
    });

    expect(
      getTagSuggestionsDisplayState({
        filteredTags: [],
        inputValue: "新标签",
        availableTags: tags,
        loading: false,
      }),
    ).toEqual({ view: "create", trimmedInput: "新标签" });

    expect(
      getTagSuggestionsDisplayState({
        filteredTags: [],
        inputValue: "",
        availableTags: tags,
        loading: false,
      }),
    ).toEqual({ view: "allSelected", trimmedInput: "" });

    expect(
      getTagSuggestionsDisplayState({
        filteredTags: [],
        inputValue: "",
        availableTags: [],
        loading: false,
      }),
    ).toEqual({ view: "empty", trimmedInput: "" });
  });

  it("picks colors from the creation palette", () => {
    expect(pickTagCreationColor(() => 0)).toBe("#3B82F6");
    expect(pickTagCreationColor(() => 0.99)).toBe("#6366F1");
  });
});
