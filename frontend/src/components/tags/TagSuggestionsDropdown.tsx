"use client";

import { memo } from "react";
import type { Tag } from "@/types";
import { getTagSuggestionsDisplayState } from "@/lib/tagInputState";

interface TagSuggestionsDropdownProps {
  filteredTags: Tag[];
  inputValue: string;
  availableTags: Tag[];
  loading: boolean;
  highlightedIndex: number;
  disabled: boolean;
  onClose: () => void;
  onSelectTag: (tag: Tag) => void;
  onCreateTag: (name: string) => void;
  onHighlightTag: (index: number) => void;
}

function TagSuggestionsDropdown({
  filteredTags,
  inputValue,
  availableTags,
  loading,
  highlightedIndex,
  disabled,
  onClose,
  onSelectTag,
  onCreateTag,
  onHighlightTag,
}: TagSuggestionsDropdownProps) {
  const displayState = getTagSuggestionsDisplayState({
    filteredTags,
    inputValue,
    availableTags,
    loading,
  });

  return (
    <>
      <div className="fixed inset-0 z-10" onClick={onClose} />

      <div className="absolute z-20 w-full mt-1 max-h-60 overflow-auto rounded-lg shadow-lg bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700">
        {displayState.view === "matches" ? (
          <>
            {displayState.showMatchHeader && (
              <div className="px-4 py-2 text-xs text-slate-500 dark:text-slate-400 border-b border-slate-200 dark:border-slate-700">
                匹配的标签
              </div>
            )}
            <div className="py-1">
              {filteredTags.map((tag, index) => (
                <TagSuggestionOption
                  key={tag.id}
                  tag={tag}
                  highlighted={highlightedIndex === index}
                  disabled={disabled}
                  onSelect={() => onSelectTag(tag)}
                  onHighlight={() => onHighlightTag(index)}
                />
              ))}
            </div>
          </>
        ) : displayState.view === "create" ? (
          <CreateTagOption
            disabled={disabled}
            label={displayState.trimmedInput}
            onCreate={() => onCreateTag(inputValue)}
          />
        ) : null}

        {displayState.view === "allSelected" && (
          <TagSuggestionsMessage message="所有标签都已选择" />
        )}

        {displayState.view === "empty" && (
          <TagSuggestionsMessage message="暂无可用标签，输入名称创建新标签" />
        )}
      </div>
    </>
  );
}

interface TagSuggestionOptionProps {
  tag: Tag;
  highlighted: boolean;
  disabled: boolean;
  onSelect: () => void;
  onHighlight: () => void;
}

function TagSuggestionOption({
  tag,
  highlighted,
  disabled,
  onSelect,
  onHighlight,
}: TagSuggestionOptionProps) {
  return (
    <button
      type="button"
      disabled={disabled}
      onClick={onSelect}
      onMouseEnter={onHighlight}
      className={`
        w-full px-4 py-2 text-left transition-colors focus:outline-none
        disabled:cursor-not-allowed disabled:opacity-50
        ${
          highlighted
            ? "bg-blue-50 dark:bg-blue-900/30 border-l-4 border-blue-500"
            : "hover:bg-slate-100 dark:hover:bg-slate-700"
        }
      `}
    >
      <div className="flex items-center gap-3">
        <span
          className="w-3 h-3 rounded-full flex-shrink-0"
          style={{ backgroundColor: tag.color }}
        />
        <span className="text-sm font-medium text-slate-900 dark:text-slate-100">
          {tag.name}
        </span>
      </div>
    </button>
  );
}

interface CreateTagOptionProps {
  label: string;
  disabled: boolean;
  onCreate: () => void;
}

function CreateTagOption({ label, disabled, onCreate }: CreateTagOptionProps) {
  return (
    <div className="py-1">
      <button
        type="button"
        disabled={disabled}
        onClick={onCreate}
        className="w-full px-4 py-2 text-left hover:bg-slate-100 dark:hover:bg-slate-700 transition-colors focus:outline-none focus:bg-slate-100 dark:focus:bg-slate-700 disabled:cursor-not-allowed disabled:opacity-50"
      >
        <div className="flex items-center gap-3">
          <span className="text-sm text-blue-600 dark:text-blue-400">
            + 创建 “{label}”
          </span>
        </div>
      </button>
    </div>
  );
}

function TagSuggestionsMessage({ message }: { message: string }) {
  return (
    <div className="px-4 py-3 text-sm text-slate-500 dark:text-slate-400 text-center">
      {message}
    </div>
  );
}

export default memo(TagSuggestionsDropdown);
