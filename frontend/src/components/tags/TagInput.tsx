"use client";

import { useState, useEffect, useRef, KeyboardEvent, memo } from "react";
import { Tag } from "@/types";
import { tagApi } from "@/lib/api";
import { useAvailableTags } from "@/hooks/useAvailableTags";
import {
  filterTagSuggestions,
  findExactTagMatch,
  getNextHighlightedIndex,
  pickTagCreationColor,
  shouldShowTagSuggestions,
} from "@/lib/tagInputState";
import TagBadge from "./TagBadge";
import TagSuggestionsDropdown from "./TagSuggestionsDropdown";

interface TagInputProps {
  selectedTags: Tag[];
  onTagsChange: (tags: Tag[]) => void;
  placeholder?: string;
  showSelectedTags?: boolean;
  disabled?: boolean;
}

// 自定义比较函数：比较 selectedTags 数组
const arePropsEqual = (prevProps: TagInputProps, nextProps: TagInputProps) => {
  // 比较 showSelectedTags 和 placeholder
  if (
    prevProps.showSelectedTags !== nextProps.showSelectedTags ||
    prevProps.placeholder !== nextProps.placeholder ||
    prevProps.disabled !== nextProps.disabled
  ) {
    return false;
  }

  // 比较 selectedTags 数组长度
  if (prevProps.selectedTags.length !== nextProps.selectedTags.length) {
    return false;
  }

  // 比较每个 tag 的 id
  return prevProps.selectedTags.every(
    (tag, index) => tag.id === nextProps.selectedTags[index]?.id,
  );
};

const TagInput = memo(function TagInput({
  selectedTags,
  onTagsChange,
  placeholder = "输入标签按回车添加",
  showSelectedTags = true,
  disabled = false,
}: TagInputProps) {
  const [inputValue, setInputValue] = useState("");
  const {
    availableTags,
    loading,
    ensureAvailableTags,
    appendAvailableTag,
  } = useAvailableTags();
  const [showSuggestions, setShowSuggestions] = useState(false);
  const [filteredTags, setFilteredTags] = useState<Tag[]>([]);
  const inputRef = useRef<HTMLInputElement>(null);
  const [highlightedIndex, setHighlightedIndex] = useState(-1); // -1表示未高亮

  // 过滤建议标签
  useEffect(() => {
    setFilteredTags(
      filterTagSuggestions(availableTags, selectedTags, inputValue),
    );
    // 注意：不在这里控制 showSuggestions，而是在 onFocus/onBlur 中控制

    // 当过滤结果变化时，重置高亮索引
    setHighlightedIndex(-1);
  }, [inputValue, availableTags, selectedTags]);

  // 添加标签
  const addTag = async (tag: Tag) => {
    if (disabled) return;

    onTagsChange([...selectedTags, tag]);
    setInputValue("");
    // 保持建议列表打开，方便连续添加标签
    setShowSuggestions(true);
  };

  // 创建新标签
  const createTag = async (name: string) => {
    if (disabled) return;

    try {
      const newTag = await tagApi.create({
        name: name.trim(),
        color: pickTagCreationColor(),
      });

      onTagsChange([...selectedTags, newTag]);
      setInputValue("");
      appendAvailableTag(newTag);
      // 保持建议列表打开，方便连续添加标签
      setShowSuggestions(true);
    } catch (err) {
      // 错误已通过axios拦截器自动处理，这里只需要记录日志
      console.error("创建标签失败:", err);
    }
  };

  // 移除标签
  const removeTag = (tagId: number) => {
    if (disabled) return;

    onTagsChange(selectedTags.filter((t) => t.id !== tagId));
  };

  // 处理键盘事件
  const handleKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    if (disabled) return;

    // 建议列表显示时的键盘导航
    if (showSuggestions && filteredTags.length > 0) {
      switch (e.key) {
        case "ArrowDown":
          e.preventDefault();
          setHighlightedIndex(
            getNextHighlightedIndex(
              highlightedIndex,
              filteredTags.length,
              "next",
            ),
          );
          return;

        case "ArrowUp":
          e.preventDefault();
          setHighlightedIndex(
            getNextHighlightedIndex(
              highlightedIndex,
              filteredTags.length,
              "previous",
            ),
          );
          return;

        case "Enter":
          e.preventDefault();
          if (highlightedIndex >= 0 && filteredTags[highlightedIndex]) {
            // 选择高亮的标签
            addTag(filteredTags[highlightedIndex]);
            setHighlightedIndex(-1);
          } else if (inputValue.trim()) {
            // 原有的Enter逻辑：精确匹配或创建新标签
            const matchedTag = findExactTagMatch(
              availableTags,
              selectedTags,
              inputValue,
            );
            if (matchedTag) {
              addTag(matchedTag);
            } else {
              createTag(inputValue);
            }
            setHighlightedIndex(-1);
          }
          return;

        case "Escape":
          setShowSuggestions(false);
          setInputValue("");
          setHighlightedIndex(-1);
          return;
      }
    }

    // 建议列表未显示时的原有Enter逻辑
    if (e.key === "Enter" && inputValue.trim() && !showSuggestions) {
      e.preventDefault();
      const matchedTag = findExactTagMatch(
        availableTags,
        selectedTags,
        inputValue,
      );
      if (matchedTag) {
        addTag(matchedTag);
      } else {
        createTag(inputValue);
      }
    } else if (e.key === "Escape") {
      setShowSuggestions(false);
      setInputValue("");
      setHighlightedIndex(-1);
    }
  };

  // 处理输入变化
  const handleInputChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    if (disabled) return;

    setInputValue(e.target.value);
  };

  // 处理失去焦点
  const handleBlur = () => {
    if (disabled) return;

    // 延迟关闭，以便点击建议项
    setTimeout(() => {
      setShowSuggestions(false);
      setHighlightedIndex(-1);
    }, 200);
  };

  // 处理获得焦点
  const handleFocus = () => {
    if (disabled) return;

    if (!availableTags.length && !loading) {
      void ensureAvailableTags();
    }

    // 显示所有可用标签
    setShowSuggestions(true);
    // 重置高亮索引
    setHighlightedIndex(-1);
  };

  return (
    <div className="w-full">
      {/* 已选择的标签 - 根据 showSelectedTags 控制显示 */}
      {showSelectedTags && selectedTags.length > 0 && (
        <div className="inline-flex flex-wrap items-center gap-1.5 mb-3 align-middle">
          {selectedTags.map((tag) => (
            <TagBadge
              key={tag.id}
              tag={tag}
              size="md"
              variant="simple"
              removable
              onRemove={() => removeTag(tag.id)}
            />
          ))}
        </div>
      )}

      {/* 输入框 - 始终显示 */}
      <div className="relative">
        <input
          ref={inputRef}
          type="text"
          value={inputValue}
          onChange={handleInputChange}
          onKeyDown={handleKeyDown}
          onBlur={handleBlur}
          onFocus={handleFocus}
          disabled={disabled}
          className={`
            w-full px-4 py-2
            border border-slate-300 dark:border-slate-600
            rounded-lg
            bg-white dark:bg-slate-800
            text-sm text-slate-900 dark:text-slate-100
            placeholder:text-slate-400 dark:placeholder:text-slate-500
            focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent
            disabled:cursor-not-allowed disabled:bg-slate-100 disabled:text-slate-400
            dark:disabled:bg-slate-900 dark:disabled:text-slate-500
            transition-colors
          `}
          placeholder={placeholder}
        />

        {/* 建议下拉列表 */}
        {shouldShowTagSuggestions(
          showSuggestions,
          filteredTags,
          inputValue,
        ) && (
          <TagSuggestionsDropdown
            filteredTags={filteredTags}
            inputValue={inputValue}
            availableTags={availableTags}
            loading={loading}
            highlightedIndex={highlightedIndex}
            disabled={disabled}
            onClose={() => {
                setShowSuggestions(false);
                setInputValue("");
              }}
            onSelectTag={addTag}
            onCreateTag={createTag}
            onHighlightTag={setHighlightedIndex}
          />
        )}
      </div>
    </div>
  );
}, arePropsEqual);

export default TagInput;
