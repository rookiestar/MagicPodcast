"use client";

import type { Tag } from "@/types";

interface PodcastTagFilterProps {
  displayTags: Tag[];
  selectedTagIds: number[];
  hasMoreTags: boolean;
  showAllTags: boolean;
  onTagToggle: (tagId: number | null) => void;
  onShowAllTagsChange: (showAllTags: boolean) => void;
}

export default function PodcastTagFilter({
  displayTags,
  selectedTagIds,
  hasMoreTags,
  showAllTags,
  onTagToggle,
  onShowAllTagsChange,
}: PodcastTagFilterProps) {
  if (displayTags.length === 0) {
    return null;
  }

  return (
    <div className="mt-4 sm:mt-6 md:mt-8 mb-3 sm:mb-4 md:mb-6">
      <div className="flex flex-wrap gap-2 sm:gap-3 items-center">
        <button
          onClick={() => onTagToggle(null)}
          className={`min-h-[44px] px-3 py-2 rounded-lg text-sm transition-all duration-200 active:scale-95 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 ${
            selectedTagIds.length === 0
              ? "bg-slate-800 text-white active:bg-slate-900"
              : "bg-slate-100 text-slate-600 hover:bg-slate-200 active:bg-slate-300"
          }`}
        >
          全部
        </button>

        {displayTags.map((tag) => {
          const isSelected = selectedTagIds.includes(tag.id);

          return (
            <button
              key={tag.id}
              onClick={() => onTagToggle(tag.id)}
              className={`min-h-[44px] px-3 py-2 rounded-lg text-sm transition-all duration-200 active:scale-95 flex items-center gap-1.5 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 ${
                isSelected
                  ? "bg-slate-800 text-white active:bg-slate-900"
                  : "bg-slate-100 text-slate-600 hover:bg-slate-200 active:bg-slate-300"
              }`}
              title={tag.name}
              aria-pressed={isSelected}
            >
              <span
                className="w-1.5 h-1.5 rounded-full flex-shrink-0"
                style={{
                  backgroundColor: isSelected ? "#ffffff" : tag.color,
                }}
              />
              <span className="max-w-[100px] truncate">{tag.name}</span>
            </button>
          );
        })}

        {hasMoreTags && (
          <button
            onClick={() => onShowAllTagsChange(!showAllTags)}
            className="w-11 h-11 min-w-[44px] min-h-[44px] rounded-lg flex items-center justify-center text-slate-500 hover:text-blue-600 hover:bg-blue-50 active:bg-blue-100 active:scale-95 transition-all duration-200 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2"
            title={showAllTags ? "收起" : "展开"}
            aria-label={showAllTags ? "收起标签" : "展开标签"}
            aria-expanded={showAllTags}
          >
            {showAllTags ? (
              <svg
                className="w-5 h-5"
                fill="none"
                stroke="currentColor"
                viewBox="0 0 24 24"
              >
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  strokeWidth={2}
                  d="M5 15l7-7 7 7"
                />
              </svg>
            ) : (
              <svg
                className="w-5 h-5"
                fill="none"
                stroke="currentColor"
                viewBox="0 0 24 24"
              >
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  strokeWidth={2}
                  d="M19 9l-7 7-7-7"
                />
              </svg>
            )}
          </button>
        )}

        {selectedTagIds.length > 0 && (
          <button
            onClick={() => onTagToggle(null)}
            className="min-h-[44px] px-3 py-2 rounded-lg text-sm text-slate-500 hover:text-slate-700 hover:bg-slate-100 active:bg-slate-200 active:scale-95 transition-all duration-200 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2"
          >
            清空
          </button>
        )}
      </div>
    </div>
  );
}
