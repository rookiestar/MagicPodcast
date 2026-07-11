"use client";

import { KeyboardEvent, memo } from "react";
import { Tag } from "@/types";
import { useAvailableTags } from "@/hooks/useAvailableTags";
import { useTagInputActions } from "@/hooks/useTagInputActions";
import { useTagInputInteractions } from "@/hooks/useTagInputInteractions";
import {
  areTagInputPropsEqual,
  getTagInputKeyboardAction,
  type TagInputKeyboardAction,
} from "@/lib/tagInputState";
import SelectedTagList from "./SelectedTagList";
import TagTextInput from "./TagTextInput";
import TagSuggestionsDropdown from "./TagSuggestionsDropdown";

interface TagInputProps {
  selectedTags: Tag[];
  onTagsChange: (tags: Tag[]) => void;
  placeholder?: string;
  showSelectedTags?: boolean;
  disabled?: boolean;
}

const TagInput = memo(function TagInput({
  selectedTags,
  onTagsChange,
  placeholder = "输入标签按回车添加",
  showSelectedTags = true,
  disabled = false,
}: TagInputProps) {
  const {
    availableTags,
    loading,
    ensureAvailableTags,
    appendAvailableTag,
  } = useAvailableTags();
  const {
    inputValue,
    showSuggestions,
    filteredTags,
    highlightedIndex,
    setHighlightedIndex,
    shouldRenderSuggestions,
    updateInputValue,
    openSuggestions,
    closeSuggestionsAndClearInput,
    closeSuggestionsAfterBlur,
    resetAfterTagChange,
  } = useTagInputInteractions({
    availableTags,
    selectedTags,
    loading,
    disabled,
    ensureAvailableTags,
  });
  const { addTag, createTag, removeTag, submitTagAction } = useTagInputActions({
    selectedTags,
    onTagsChange,
    disabled,
    appendAvailableTag,
    resetAfterTagChange,
  });

  const applyKeyboardAction = (action: TagInputKeyboardAction) => {
    if (action.type === "highlight") {
      setHighlightedIndex(action.index);
      return;
    }

    if (action.type === "submit") {
      submitTagAction(action.submitAction);
      setHighlightedIndex(-1);
      return;
    }

    if (action.type === "clear") {
      closeSuggestionsAndClearInput();
    }
  };

  // 处理键盘事件
  const handleKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    if (disabled) return;

    const action = getTagInputKeyboardAction({
      key: e.key,
      showSuggestions,
      availableTags,
      selectedTags,
      filteredTags,
      highlightedIndex,
      inputValue,
    });

    if (action.preventDefault) {
      e.preventDefault();
    }

    applyKeyboardAction(action);
  };

  // 处理失去焦点
  const handleBlur = () => {
    closeSuggestionsAfterBlur();
  };

  // 处理获得焦点
  const handleFocus = () => {
    openSuggestions();
  };

  return (
    <div className="w-full">
      <SelectedTagList
        selectedTags={selectedTags}
        showSelectedTags={showSelectedTags}
        onRemoveTag={removeTag}
      />

      {/* 输入框 - 始终显示 */}
      <div className="relative">
        <TagTextInput
          value={inputValue}
          onChangeValue={updateInputValue}
          onKeyDown={handleKeyDown}
          onBlur={handleBlur}
          onFocus={handleFocus}
          disabled={disabled}
          placeholder={placeholder}
        />

        {/* 建议下拉列表 */}
        {shouldRenderSuggestions && (
          <TagSuggestionsDropdown
            filteredTags={filteredTags}
            inputValue={inputValue}
            availableTags={availableTags}
            loading={loading}
            highlightedIndex={highlightedIndex}
            disabled={disabled}
            onClose={closeSuggestionsAndClearInput}
            onSelectTag={addTag}
            onCreateTag={createTag}
            onHighlightTag={setHighlightedIndex}
          />
        )}
      </div>
    </div>
  );
}, areTagInputPropsEqual);

export default TagInput;
