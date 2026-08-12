"use client";

import { IconSearch, IconX } from "@tabler/icons-react";
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
    <header className="search-workbench-header">
      <div className="search-workbench-kicker">
        <div>
          <span id="search-workbench-title">全站搜索</span>
          <small>节目与单集</small>
        </div>
        <button
          onClick={onClose}
          className="search-workbench-close"
          aria-label="关闭搜索"
          title="关闭"
        >
          <IconX aria-hidden="true" stroke={1.8} />
        </button>
      </div>

      <div className="search-workbench-query">
        <div className="search-workbench-input">
          <IconSearch aria-hidden="true" stroke={1.8} />
          <input
            ref={inputRef}
            type="text"
            value={query}
            onChange={(e) => onQueryChange(e.target.value)}
            placeholder="搜索节目、单集..."
            aria-label="搜索节目和单集"
          />
        </div>
      </div>

      <div className="search-workbench-tabs" aria-label="搜索范围">
        {getSearchTypeOptionConfigs().map((option) => (
          <button
            key={option.type}
            onClick={() => onSearchTypeChange(option.type)}
            aria-pressed={searchType === option.type}
          >
            {getSearchTypeOptionLabel(option.type, searchResultCounts)}
          </button>
        ))}
      </div>
    </header>
  );
}
