"use client";

import { memo } from "react";
import type { Tag } from "@/types";

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
  const trimmedInput = inputValue.trim();

  return (
    <>
      <div className="fixed inset-0 z-10" onClick={onClose} />

      <div className="absolute z-20 w-full mt-1 max-h-60 overflow-auto rounded-lg shadow-lg bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700">
        {filteredTags.length > 0 ? (
          <>
            {trimmedInput && (
              <div className="px-4 py-2 text-xs text-slate-500 dark:text-slate-400 border-b border-slate-200 dark:border-slate-700">
                匹配的标签
              </div>
            )}
            <div className="py-1">
              {filteredTags.map((tag, index) => (
                <button
                  key={tag.id}
                  type="button"
                  disabled={disabled}
                  onClick={() => onSelectTag(tag)}
                  onMouseEnter={() => onHighlightTag(index)}
                  className={`
                    w-full px-4 py-2 text-left transition-colors focus:outline-none
                    disabled:cursor-not-allowed disabled:opacity-50
                    ${
                      highlightedIndex === index
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
              ))}
            </div>
          </>
        ) : (
          trimmedInput && (
            <div className="py-1">
              <button
                type="button"
                disabled={disabled}
                onClick={() => onCreateTag(inputValue)}
                className="w-full px-4 py-2 text-left hover:bg-slate-100 dark:hover:bg-slate-700 transition-colors focus:outline-none focus:bg-slate-100 dark:focus:bg-slate-700 disabled:cursor-not-allowed disabled:opacity-50"
              >
                <div className="flex items-center gap-3">
                  <span className="text-sm text-blue-600 dark:text-blue-400">
                    + 创建 “{trimmedInput}”
                  </span>
                </div>
              </button>
            </div>
          )
        )}

        {!trimmedInput && filteredTags.length === 0 && availableTags.length > 0 && (
          <div className="px-4 py-3 text-sm text-slate-500 dark:text-slate-400 text-center">
            所有标签都已选择
          </div>
        )}

        {!trimmedInput && availableTags.length === 0 && !loading && (
          <div className="px-4 py-3 text-sm text-slate-500 dark:text-slate-400 text-center">
            暂无可用标签，输入名称创建新标签
          </div>
        )}
      </div>
    </>
  );
}

export default memo(TagSuggestionsDropdown);
