"use client";

import { useState, useEffect, useRef } from "react";
import { SearchSidebarContent } from "@/components/search/SearchSidebarContent";
import { SearchSidebarHeader } from "@/components/search/SearchSidebarHeader";
import { useSearchSidebar } from "@/hooks/useSearchSidebar";
import { getSearchSidebarPanelState } from "@/lib/searchSidebarState";

interface SearchSidebarProps {
  isOpen: boolean;
  onClose: () => void;
}

export default function SearchSidebar({ isOpen, onClose }: SearchSidebarProps) {
  const searchInputRef = useRef<HTMLInputElement>(null);
  const sidebarRef = useRef<HTMLDivElement>(null);
  const previousFocusRef = useRef<HTMLElement | null>(null);
  const [isFocused, setIsFocused] = useState(false);
  const [expandedPodcasts, setExpandedPodcasts] = useState(false);
  const [expandedEpisodes, setExpandedEpisodes] = useState(false);
  const {
    query,
    setQuery,
    searchType,
    setSearchType,
    allResults,
    results,
    loading,
    searchError,
    searchHistory,
    hasResults,
    isQueryTooShort,
    showHistory,
    selectHistory,
    clearHistory,
  } = useSearchSidebar({ isOpen });

  // 自动聚焦
  useEffect(() => {
    if (isOpen && searchInputRef.current) {
      previousFocusRef.current = document.activeElement as HTMLElement | null;
      searchInputRef.current.focus();
      setIsFocused(true); // 打开时设置焦点状态
      return;
    }

    previousFocusRef.current?.focus();
    previousFocusRef.current = null;
  }, [isOpen]);

  // 重置状态当关闭时
  useEffect(() => {
    if (!isOpen) {
      setExpandedPodcasts(false);
      setExpandedEpisodes(false);
      setIsFocused(false);
    }
  }, [isOpen]);

  useEffect(() => {
    setExpandedPodcasts(false);
    setExpandedEpisodes(false);
  }, [query, searchType]);

  // 焦点管理：当焦点移出侧边栏时自动关闭
  useEffect(() => {
    if (!isOpen || isFocused) return;

    // 延迟关闭，避免在点击侧边栏内部元素时误触发
    const timer = setTimeout(() => {
      // 检查当前焦点元素是否在侧边栏内
      if (
        sidebarRef.current &&
        !sidebarRef.current.contains(document.activeElement)
      ) {
        onClose();
      }
    }, 100);

    return () => clearTimeout(timer);
  }, [isFocused, isOpen, onClose]);

  useEffect(() => {
    if (!isOpen) return;

    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        onClose();
      }
    };

    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [isOpen, onClose]);

  const handleClose = () => {
    onClose();
  };

  const handleHistoryClick = (historyQuery: string) => {
    selectHistory(historyQuery);
  };

  const handleClearHistory = () => {
    clearHistory();
  };

  if (!isOpen) return null;

  const panelState = getSearchSidebarPanelState({
    loading,
    isQueryTooShort,
    showHistory,
    searchError,
    hasResults,
  });

  return (
    <>
      {/* 遮罩层 */}
      <div
        className={`search-workbench-backdrop fixed inset-0 z-40 ${
          isOpen ? "opacity-100" : "opacity-0 pointer-events-none"
        }`}
        onClick={handleClose}
        aria-hidden="true"
      />

      {/* 侧边栏 */}
      <div
        ref={sidebarRef}
        role="dialog"
        aria-modal="true"
        aria-label="全站搜索"
        tabIndex={-1}
        onFocus={() => setIsFocused(true)}
        onBlur={() => setIsFocused(false)}
        className={`search-workbench fixed right-0 top-0 z-50 flex h-full w-full flex-col ${
          isOpen
            ? "translate-x-0 opacity-100"
            : "translate-x-full opacity-0"
        }`}
      >
        <SearchSidebarHeader
          inputRef={searchInputRef}
          query={query}
          searchType={searchType}
          podcastCount={allResults.podcasts.length}
          episodeCount={allResults.episodes.length}
          onQueryChange={setQuery}
          onSearchTypeChange={setSearchType}
          onClose={handleClose}
        />

        <SearchSidebarContent
          panelState={panelState}
          query={query}
          searchType={searchType}
          results={results}
          searchError={searchError}
          searchHistory={searchHistory}
          expandedPodcasts={expandedPodcasts}
          expandedEpisodes={expandedEpisodes}
          onHistoryClick={handleHistoryClick}
          onClearHistory={handleClearHistory}
          onTogglePodcasts={() => setExpandedPodcasts(!expandedPodcasts)}
          onToggleEpisodes={() => setExpandedEpisodes(!expandedEpisodes)}
        />
      </div>
    </>
  );
}
