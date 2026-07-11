import { useCallback, useEffect, useMemo, useState } from "react";
import type { Tag } from "@/types";
import {
  filterTagSuggestions,
  shouldShowTagSuggestions,
} from "@/lib/tagInputState";

interface UseTagInputInteractionsOptions {
  availableTags: Tag[];
  selectedTags: Tag[];
  loading: boolean;
  disabled: boolean;
  ensureAvailableTags: () => Promise<void>;
}

export function useTagInputInteractions({
  availableTags,
  selectedTags,
  loading,
  disabled,
  ensureAvailableTags,
}: UseTagInputInteractionsOptions) {
  const [inputValue, setInputValue] = useState("");
  const [showSuggestions, setShowSuggestions] = useState(false);
  const [highlightedIndex, setHighlightedIndex] = useState(-1);

  const filteredTags = useMemo(
    () => filterTagSuggestions(availableTags, selectedTags, inputValue),
    [availableTags, inputValue, selectedTags],
  );

  useEffect(() => {
    setHighlightedIndex(-1);
  }, [filteredTags]);

  const updateInputValue = useCallback(
    (value: string) => {
      if (disabled) return;

      setInputValue(value);
    },
    [disabled],
  );

  const openSuggestions = useCallback(() => {
    if (disabled) return;

    if (!availableTags.length && !loading) {
      void ensureAvailableTags();
    }

    setShowSuggestions(true);
    setHighlightedIndex(-1);
  }, [availableTags.length, disabled, ensureAvailableTags, loading]);

  const closeSuggestions = useCallback(() => {
    setShowSuggestions(false);
    setHighlightedIndex(-1);
  }, []);

  const closeSuggestionsAndClearInput = useCallback(() => {
    closeSuggestions();
    setInputValue("");
  }, [closeSuggestions]);

  const closeSuggestionsAfterBlur = useCallback(() => {
    if (disabled) return;

    setTimeout(closeSuggestions, 200);
  }, [closeSuggestions, disabled]);

  const resetAfterTagChange = useCallback(() => {
    setInputValue("");
    setShowSuggestions(true);
  }, []);

  return {
    inputValue,
    setInputValue,
    showSuggestions,
    filteredTags,
    highlightedIndex,
    setHighlightedIndex,
    shouldRenderSuggestions: shouldShowTagSuggestions(
      showSuggestions,
      filteredTags,
      inputValue,
    ),
    updateInputValue,
    openSuggestions,
    closeSuggestions,
    closeSuggestionsAndClearInput,
    closeSuggestionsAfterBlur,
    resetAfterTagChange,
  };
}
