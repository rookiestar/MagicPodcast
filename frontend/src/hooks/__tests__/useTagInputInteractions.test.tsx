import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { Tag } from "@/types";
import { useTagInputInteractions } from "../useTagInputInteractions";

const tags: Tag[] = [
  { id: 1, name: "科技", color: "#2563eb" },
  { id: 2, name: "AI", color: "#16a34a" },
  { id: 3, name: "生活", color: "#f97316" },
];

function renderInteractions(overrides = {}) {
  return renderHook((props) => useTagInputInteractions(props), {
    initialProps: {
      availableTags: tags,
      selectedTags: [],
      loading: false,
      disabled: false,
      ensureAvailableTags: vi.fn().mockResolvedValue(undefined),
      ...overrides,
    },
  });
}

describe("useTagInputInteractions", () => {
  afterEach(() => {
    vi.useRealTimers();
  });

  it("filters suggestions from the current input value", () => {
    const { result } = renderInteractions();

    act(() => {
      result.current.updateInputValue("a");
    });

    expect(result.current.filteredTags).toEqual([tags[1]]);
  });

  it("opens suggestions and loads available tags when needed", () => {
    const ensureAvailableTags = vi.fn().mockResolvedValue(undefined);
    const { result } = renderInteractions({
      availableTags: [],
      ensureAvailableTags,
    });

    act(() => {
      result.current.openSuggestions();
    });

    expect(ensureAvailableTags).toHaveBeenCalledTimes(1);
    expect(result.current.showSuggestions).toBe(true);
  });

  it("closes suggestions after blur", () => {
    vi.useFakeTimers();
    const { result } = renderInteractions();

    act(() => {
      result.current.openSuggestions();
      result.current.closeSuggestionsAfterBlur();
    });

    expect(result.current.showSuggestions).toBe(true);

    act(() => {
      vi.advanceTimersByTime(200);
    });

    expect(result.current.showSuggestions).toBe(false);
  });

  it("ignores input updates when disabled", () => {
    const { result } = renderInteractions({ disabled: true });

    act(() => {
      result.current.updateInputValue("AI");
      result.current.openSuggestions();
    });

    expect(result.current.inputValue).toBe("");
    expect(result.current.showSuggestions).toBe(false);
  });
});
