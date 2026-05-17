"use client";

import { useState, useEffect, useRef, KeyboardEvent, memo } from "react";
import { Tag } from "@/types";
import { tagApi } from "@/lib/api";
import TagBadge from "./TagBadge";

interface TagInputProps {
  selectedTags: Tag[];
  onTagsChange: (tags: Tag[]) => void;
  placeholder?: string;
  showSelectedTags?: boolean;
  disabled?: boolean;
}

let cachedAvailableTags: Tag[] | null = null;
let pendingAvailableTagsRequest: Promise<Tag[]> | null = null;

async function loadAvailableTags() {
  if (cachedAvailableTags) {
    return cachedAvailableTags;
  }

  if (!pendingAvailableTagsRequest) {
    pendingAvailableTagsRequest = tagApi
      .list()
      .then((tags) => {
        cachedAvailableTags = tags;
        return tags;
      })
      .finally(() => {
        pendingAvailableTagsRequest = null;
      });
  }

  return pendingAvailableTagsRequest;
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
  const [availableTags, setAvailableTags] = useState<Tag[]>([]);
  const [showSuggestions, setShowSuggestions] = useState(false);
  const [filteredTags, setFilteredTags] = useState<Tag[]>([]);
  const [loading, setLoading] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);
  const [highlightedIndex, setHighlightedIndex] = useState(-1); // -1表示未高亮

  const ensureAvailableTags = async () => {
    try {
      setLoading(true);
      setAvailableTags(await loadAvailableTags());
    } catch (error) {
      console.error("Failed to fetch tags:", error);
    } finally {
      setLoading(false);
    }
  };

  // 过滤建议标签
  useEffect(() => {
    const selectedIds = selectedTags.map((t) => t.id);

    if (inputValue.trim()) {
      // 有输入内容时，过滤匹配的标签
      const filtered = availableTags.filter(
        (t) =>
          !selectedIds.includes(t.id) &&
          t.name.toLowerCase().includes(inputValue.toLowerCase()),
      );
      setFilteredTags(filtered);
    } else {
      // 没有输入内容时，显示所有未选择的标签
      const filtered = availableTags.filter((t) => !selectedIds.includes(t.id));
      setFilteredTags(filtered);
    }
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

    // 生成随机颜色
    const colors = [
      "#3B82F6",
      "#10B981",
      "#F59E0B",
      "#EF4444",
      "#8B5CF6",
      "#EC4899",
      "#6366F1",
    ];
    const randomColor = colors[Math.floor(Math.random() * colors.length)];

    try {
      const newTag = await tagApi.create({
        name: name.trim(),
        color: randomColor,
      });

      onTagsChange([...selectedTags, newTag]);
      setInputValue("");
      // 将新标签添加到availableTags中
      const nextAvailableTags = [...availableTags, newTag];
      cachedAvailableTags = nextAvailableTags;
      setAvailableTags(nextAvailableTags);
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
          const nextIndex = highlightedIndex + 1;
          setHighlightedIndex(nextIndex >= filteredTags.length ? 0 : nextIndex);
          return;

        case "ArrowUp":
          e.preventDefault();
          const prevIndex = highlightedIndex - 1;
          setHighlightedIndex(
            prevIndex < 0 ? filteredTags.length - 1 : prevIndex,
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
            const selectedIds = selectedTags.map((t) => t.id);
            const matchedTag = availableTags.find(
              (t) =>
                !selectedIds.includes(t.id) &&
                t.name.toLowerCase() === inputValue.toLowerCase().trim(),
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
      const selectedIds = selectedTags.map((t) => t.id);
      const matchedTag = availableTags.find(
        (t) =>
          !selectedIds.includes(t.id) &&
          t.name.toLowerCase() === inputValue.toLowerCase().trim(),
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
        {showSuggestions && (filteredTags.length > 0 || inputValue.trim()) && (
          <>
            {/* 点击外部关闭 */}
            <div
              className="fixed inset-0 z-10"
              onClick={() => {
                setShowSuggestions(false);
                setInputValue("");
              }}
            />

            {/* 下拉菜单 */}
            <div className="absolute z-20 w-full mt-1 max-h-60 overflow-auto rounded-lg shadow-lg bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700">
              {filteredTags.length > 0 ? (
                <>
                  {inputValue.trim() && (
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
                        onClick={() => addTag(tag)}
                        onMouseEnter={() => setHighlightedIndex(index)}
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
                inputValue.trim() && (
                  <div className="py-1">
                    <button
                      type="button"
                      disabled={disabled}
                      onClick={() => createTag(inputValue)}
                      className="w-full px-4 py-2 text-left hover:bg-slate-100 dark:hover:bg-slate-700 transition-colors focus:outline-none focus:bg-slate-100 dark:focus:bg-slate-700 disabled:cursor-not-allowed disabled:opacity-50"
                    >
                      <div className="flex items-center gap-3">
                        <span className="text-sm text-blue-600 dark:text-blue-400">
                          + 创建 “{inputValue.trim()}”
                        </span>
                      </div>
                    </button>
                  </div>
                )
              )}

              {/* 当没有匹配且没有输入时显示提示 */}
              {!inputValue.trim() &&
                filteredTags.length === 0 &&
                availableTags.length > 0 && (
                  <div className="px-4 py-3 text-sm text-slate-500 dark:text-slate-400 text-center">
                    所有标签都已选择
                  </div>
                )}

              {!inputValue.trim() && availableTags.length === 0 && !loading && (
                <div className="px-4 py-3 text-sm text-slate-500 dark:text-slate-400 text-center">
                  暂无可用标签，输入名称创建新标签
                </div>
              )}
            </div>
          </>
        )}
      </div>
    </div>
  );
}, arePropsEqual);

export default TagInput;
