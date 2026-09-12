"use client";

import { useState, useEffect, useRef, useMemo } from "react";
import { singleParam, updateQuery, useLocationHref, useEpisodeReturnRestoration } from "@/lib/navigation";
import type { SearchType } from "@/lib/searchSidebarState";
import { SearchSidebarContent } from "@/components/search/SearchSidebarContent";
import { SearchSidebarHeader } from "@/components/search/SearchSidebarHeader";
import { useSearchSidebar } from "@/hooks/useSearchSidebar";
import { getSearchSidebarPanelState } from "@/lib/searchSidebarState";

interface SearchSidebarProps {
  isOpen: boolean;
  onClose: () => void;
  standalone?: boolean;
}

export default function SearchSidebar({ isOpen, onClose, standalone = false }: SearchSidebarProps) {
  useEpisodeReturnRestoration();
  const href = useLocationHref();
  const params = useMemo(() => new URL(href || "/search", "http://navigation.local").searchParams, [href]);
  const committedQuery = (singleParam(params, "q") ?? "").trim();
  const rawType = singleParam(params, "type");
  const searchType: SearchType = rawType === "podcasts" || rawType === "episodes" ? rawType : "all";
  // Search opened as an overlay should keep one history entry while the user
  // refines the query/type, so closing it returns directly to its source page.
  // The standalone search page keeps normal back-navigation between queries.
  const updateSearchQuery = (values: Record<string, string | null>) =>
    updateQuery(values, !standalone);
  const setSearchType = (value: SearchType) =>
    updateSearchQuery({ type: value === "all" ? null : value });
  useEffect(() => {
    if (!isOpen || !href) return;
    const patch: Record<string, string | null> = {};
    if (params.has("q") && (params.getAll("q").length !== 1 || params.get("q") !== committedQuery || !committedQuery)) patch.q = committedQuery || null;
    if (params.has("type") && !["all", "podcasts", "episodes"].includes(rawType ?? "")) patch.type = null;
    if (Object.keys(patch).length) updateQuery(patch, true);
  }, [isOpen, href, params, committedQuery, rawType]);
  const searchInputRef = useRef<HTMLInputElement>(null);
  const sidebarRef = useRef<HTMLDivElement>(null);
  const previousFocusRef = useRef<HTMLElement | null>(null);
  const [isFocused, setIsFocused] = useState(false);
  const [expandedPodcasts, setExpandedPodcasts] = useState(false);
  const [expandedEpisodes, setExpandedEpisodes] = useState(false);
  const {
    query,
    setQuery,
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
  } = useSearchSidebar({ isOpen, type: searchType });
  useEffect(() => { if (isOpen) setQuery(committedQuery); }, [committedQuery, isOpen, setQuery]);
  const submitQuery = () => updateSearchQuery({ q: query.trim() || null });

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
    updateSearchQuery({ q: historyQuery.trim() || null });
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
      {!standalone && <div
        className={`search-workbench-backdrop fixed inset-0 z-40 ${
          isOpen ? "opacity-100" : "opacity-0 pointer-events-none"
        }`}
        onClick={handleClose}
        aria-hidden="true"
      />}

      {/* 侧边栏 */}
      <div
        ref={sidebarRef}
        role={standalone ? "main" : "dialog"}
        aria-modal={standalone ? undefined : true}
        aria-labelledby="search-workbench-title"
        style={standalone ? { width: "100%", maxWidth: "none" } : undefined}
        tabIndex={-1}
        onFocus={() => setIsFocused(true)}
        onBlur={() => setIsFocused(false)}
        className={`search-workbench fixed right-0 top-0 z-[60] flex h-full w-full flex-col ${
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
          onSubmit={submitQuery}
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
