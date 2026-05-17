"use client";

import type { RefObject } from "react";
import {
  getSearchTypeOptionConfigs,
  getSearchTypeOptionLabel,
} from "@/lib/searchResultDisplay";
import type { SearchType } from "@/lib/searchSidebarState";

interface SearchSidebarHeaderProps {
  inputRef: RefObject<HTMLInputElement>;
  query: string;
  searchType: SearchType;
  podcastCount: number;
  episodeCount: number;
  onQueryChange: (query: string) => void;
  onSearchTypeChange: (searchType: SearchType) => void;
  onClose: () => void;
}

export function SearchSidebarHeader({
  inputRef,
  query,
  searchType,
  podcastCount,
  episodeCount,
  onQueryChange,
  onSearchTypeChange,
  onClose,
}: SearchSidebarHeaderProps) {
  const searchResultCounts = { podcastCount, episodeCount };

  return (
    <div className="border-b border-slate-200 dark:border-slate-700 p-4">
      <div className="flex items-center gap-3">
        <div className="flex-1 relative">
          <span className="absolute left-3 top-1/2 -translate-y-1/2 text-slate-400">
            🔍
          </span>
          <input
            ref={inputRef}
            type="text"
            value={query}
            onChange={(e) => onQueryChange(e.target.value)}
            placeholder="搜索节目、单集..."
            className="w-full pl-10 pr-4 py-2 bg-slate-100 dark:bg-slate-700 rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500 dark:text-slate-100"
          />
        </div>
        <button
          onClick={onClose}
          className="p-2 hover:bg-slate-100 dark:hover:bg-slate-700 rounded-lg text-2xl leading-none text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:hover:text-slate-200 transition-colors"
          aria-label="关闭搜索"
          title="关闭"
        >
          ×
        </button>
      </div>

      <div className="flex gap-2 mt-3">
        {getSearchTypeOptionConfigs().map((option) => (
          <button
            key={option.type}
            onClick={() => onSearchTypeChange(option.type)}
            className={`px-3 py-1 rounded-full text-sm transition-colors ${
              searchType === option.type
                ? "bg-blue-600 text-white"
                : "bg-slate-200 dark:bg-slate-700 text-slate-700 dark:text-slate-300"
            }`}
          >
            {getSearchTypeOptionLabel(option.type, searchResultCounts)}
          </button>
        ))}
      </div>
    </div>
  );
}
