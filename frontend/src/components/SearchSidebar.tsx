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
      searchInputRef.current.focus();
      setIsFocused(true); // 打开时设置焦点状态
    }
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
        className={`fixed inset-0 bg-black/50 z-40 transition-all duration-700 ease-out ${
          isOpen ? "opacity-100" : "opacity-0 pointer-events-none"
        }`}
        onClick={handleClose}
      />

      {/* 侧边栏 */}
      <div
        ref={sidebarRef}
        tabIndex={-1}
        onFocus={() => setIsFocused(true)}
        onBlur={() => setIsFocused(false)}
        className={`fixed right-0 top-0 h-full w-full max-w-3xl bg-white dark:bg-slate-800 z-50 flex flex-col transition-all duration-700 ${
          isOpen
            ? "translate-x-0 scale-100 opacity-100 shadow-2xl"
            : "translate-x-full scale-90 opacity-0"
        }`}
        style={{
          transitionTimingFunction: "cubic-bezier(0.34, 1.56, 0.64, 1)",
        }}
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
