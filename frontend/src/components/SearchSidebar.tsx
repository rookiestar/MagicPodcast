"use client";

import { useState, useEffect, useRef } from "react";
import PodcastCover from "@/components/podcasts/PodcastCover";
import { useSearchSidebar } from "@/hooks/useSearchSidebar";
import {
  buildEpisodeSearchResultHref,
  buildPodcastSearchResultHref,
  getEpisodeSearchSnippet,
  getPodcastSearchSnippet,
  getSearchExpandButtonLabel,
  getSearchResultsCount,
  getSearchTextHighlightParts,
  getVisibleSearchResults,
  shouldShowEpisodeSearchResults,
  shouldShowPodcastSearchResults,
  shouldShowSearchExpandButton,
  shouldShowSearchSectionHeading,
} from "@/lib/searchResultDisplay";
import { getSearchSidebarPanelState } from "@/lib/searchSidebarState";

interface SearchSidebarProps {
  isOpen: boolean;
  onClose: () => void;
}

function renderHighlightedText(text: string, keyword: string) {
  return getSearchTextHighlightParts(text, keyword).map((part, index) => {
    if (!part.highlighted) return part.text;

    return (
      <mark
        key={`${part.text}-${index}`}
        className="bg-yellow-200 dark:bg-yellow-800 rounded px-0.5"
      >
        {part.text}
      </mark>
    );
  });
}

export default function SearchSidebar({ isOpen, onClose }: SearchSidebarProps) {
  const searchInputRef = useRef<HTMLInputElement>(null);
  const sidebarRef = useRef<HTMLDivElement>(null);
  const [isFocused, setIsFocused] = useState(false);
  const [expandedPodcasts, setExpandedPodcasts] = useState(false);
  const [expandedEpisodes, setExpandedEpisodes] = useState(false);
  const PODCASTS_PER_PAGE = 10;
  const EPISODES_PER_PAGE = 10;
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

  const resultCount = getSearchResultsCount({
    podcastCount: allResults.podcasts.length,
    episodeCount: allResults.episodes.length,
  });
  const showPodcasts = shouldShowPodcastSearchResults(
    searchType,
    results.podcasts.length,
  );
  const showEpisodes = shouldShowEpisodeSearchResults(
    searchType,
    results.episodes.length,
  );
  const visiblePodcasts = getVisibleSearchResults(
    results.podcasts,
    expandedPodcasts,
    PODCASTS_PER_PAGE,
  );
  const visibleEpisodes = getVisibleSearchResults(
    results.episodes,
    expandedEpisodes,
    EPISODES_PER_PAGE,
  );
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
        {/* 头部 */}
        <div className="border-b border-slate-200 dark:border-slate-700 p-4">
          <div className="flex items-center gap-3">
            <div className="flex-1 relative">
              <span className="absolute left-3 top-1/2 -translate-y-1/2 text-slate-400">
                🔍
              </span>
              <input
                ref={searchInputRef}
                type="text"
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                placeholder="搜索节目、单集..."
                className="w-full pl-10 pr-4 py-2 bg-slate-100 dark:bg-slate-700 rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500 dark:text-slate-100"
              />
            </div>
            <button
              onClick={handleClose}
              className="p-2 hover:bg-slate-100 dark:hover:bg-slate-700 rounded-lg text-2xl leading-none text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:hover:text-slate-200 transition-colors"
              aria-label="关闭搜索"
              title="关闭"
            >
              ×
            </button>
          </div>

          {/* 搜索类型切换 */}
          <div className="flex gap-2 mt-3">
            <button
              onClick={() => setSearchType("all")}
              className={`px-3 py-1 rounded-full text-sm transition-colors ${
                searchType === "all"
                  ? "bg-blue-600 text-white"
                  : "bg-slate-200 dark:bg-slate-700 text-slate-700 dark:text-slate-300"
              }`}
            >
              全部{" "}
              {resultCount > 0 && `(${resultCount})`}
            </button>
            <button
              onClick={() => setSearchType("podcasts")}
              className={`px-3 py-1 rounded-full text-sm transition-colors ${
                searchType === "podcasts"
                  ? "bg-blue-600 text-white"
                  : "bg-slate-200 dark:bg-slate-700 text-slate-700 dark:text-slate-300"
              }`}
            >
              节目{" "}
              {allResults.podcasts.length > 0 &&
                `(${allResults.podcasts.length})`}
            </button>
            <button
              onClick={() => setSearchType("episodes")}
              className={`px-3 py-1 rounded-full text-sm transition-colors ${
                searchType === "episodes"
                  ? "bg-blue-600 text-white"
                  : "bg-slate-200 dark:bg-slate-700 text-slate-700 dark:text-slate-300"
              }`}
            >
              单集{" "}
              {allResults.episodes.length > 0 &&
                `(${allResults.episodes.length})`}
            </button>
          </div>
        </div>

        {/* 结果区域 */}
        <div className="flex-1 overflow-y-auto">
          {/* 搜索历史 */}
          {panelState === "history" && (
            <div className="p-4">
              <div className="flex items-center justify-between mb-3">
                <h3 className="text-sm font-semibold text-slate-700 dark:text-slate-300">
                  🕐 最近搜索
                </h3>
                <button
                  onClick={handleClearHistory}
                  className="text-xs text-slate-500 hover:text-slate-700 dark:text-slate-400 dark:hover:text-slate-200 transition-colors"
                >
                  清空
                </button>
              </div>
              <div className="space-y-2">
                {searchHistory.map((historyQuery, index) => (
                  <button
                    key={index}
                    onClick={() => handleHistoryClick(historyQuery)}
                    className="w-full text-left px-3 py-2 bg-slate-50 dark:bg-slate-900 hover:bg-slate-100 dark:hover:bg-slate-800 rounded-lg transition-colors text-slate-700 dark:text-slate-300 text-sm"
                  >
                    📌 {historyQuery}
                  </button>
                ))}
              </div>
            </div>
          )}

          {panelState === "prompt" && (
            <div className="flex flex-col items-center justify-center h-full text-slate-500 dark:text-slate-400">
              <span className="text-6xl mb-4">🔍</span>
              <p className="text-lg">输入关键词开始搜索</p>
              <p className="text-sm mt-2">
                支持搜索节目标题、作者、简介和单集内容
              </p>
            </div>
          )}

          {panelState === "error" && (
            <div className="flex flex-col items-center justify-center h-full text-slate-500 dark:text-slate-400">
              <p className="text-lg">搜索失败</p>
              <p className="text-sm mt-2">{searchError}</p>
            </div>
          )}

          {panelState === "empty" && (
            <div className="flex flex-col items-center justify-center h-full text-slate-500 dark:text-slate-400">
              <p className="text-lg">未找到相关结果</p>
              <p className="text-sm mt-2">试试其他关键词</p>
            </div>
          )}

          {panelState === "loading" && (
            <div className="flex items-center justify-center py-12">
              <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-blue-600"></div>
            </div>
          )}

          {panelState === "results" && (
            <div className="p-4">
              {/* 播客结果 */}
              {showPodcasts && (
                <>
                  {shouldShowSearchSectionHeading(
                    searchType,
                    results.episodes.length,
                  ) && (
                    <h3 className="text-lg font-semibold text-slate-900 dark:text-slate-50 mb-3">
                      📻 节目 ({results.podcasts.length})
                    </h3>
                  )}
                  <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                    {visiblePodcasts.map((podcast, index) => (
                      <a
                        key={podcast.id}
                        href={buildPodcastSearchResultHref(podcast.id)}
                        target="_blank"
                        rel="noopener noreferrer"
                        className="block p-4 bg-white dark:bg-slate-800 rounded-lg hover:shadow-md transition-all border border-slate-200 dark:border-slate-700"
                      >
                        <div className="flex gap-4">
                          <div className="w-20 h-20 flex-shrink-0">
                            <PodcastCover
                              coverUrl={podcast.cover_url}
                              title={podcast.title}
                              index={index}
                              priority={
                                index < 3
                                  ? "high"
                                  : index < 10
                                    ? "medium"
                                    : "low"
                              }
                            />
                          </div>
                          <div className="flex-1 min-w-0">
                            <h3
                              className="font-semibold text-slate-900 dark:text-slate-50 mb-1 line-clamp-1"
                            >
                              {renderHighlightedText(podcast.title, query)}
                            </h3>
                            <p className="text-sm text-slate-600 dark:text-slate-400 mb-2">
                              {podcast.author} · {podcast.episode_count} 集
                            </p>
                            {(() => {
                              const snippetToShow =
                                getPodcastSearchSnippet(podcast);

                              return snippetToShow ? (
                                <p
                                  className="text-sm text-slate-500 dark:text-slate-500 line-clamp-2"
                                >
                                  {renderHighlightedText(snippetToShow, query)}
                                </p>
                              ) : null;
                            })()}
                          </div>
                        </div>
                      </a>
                    ))}
                  </div>

                  {/* 展开按钮 */}
                  {shouldShowSearchExpandButton(
                    results.podcasts.length,
                    PODCASTS_PER_PAGE,
                  ) && (
                    <button
                      onClick={() => setExpandedPodcasts(!expandedPodcasts)}
                      className="w-full mt-3 py-2 text-sm text-blue-600 dark:text-blue-400 hover:text-blue-700 dark:hover:text-blue-300 transition-colors"
                    >
                      {getSearchExpandButtonLabel(
                        expandedPodcasts,
                        results.podcasts.length,
                        "节目",
                      )}
                    </button>
                  )}
                </>
              )}

              {/* 单集结果 */}
              {showEpisodes && (
                <>
                  {shouldShowSearchSectionHeading(
                    searchType,
                    results.podcasts.length,
                  ) && (
                    <>
                      <div className="my-6 border-t border-slate-200 dark:border-slate-700"></div>
                      <h3 className="text-lg font-semibold text-slate-900 dark:text-slate-50 mb-3">
                        🎧 单集 ({results.episodes.length})
                      </h3>
                    </>
                  )}
                  <div className="space-y-3">
                    {visibleEpisodes.map((episode) => (
                      <a
                        key={episode.id}
                        href={buildEpisodeSearchResultHref(
                          episode.podcast_id,
                          episode.id,
                        )}
                        target="_blank"
                        rel="noopener noreferrer"
                        className="block p-4 bg-slate-50 dark:bg-slate-900 rounded-lg hover:bg-slate-100 dark:hover:bg-slate-800 transition-colors"
                      >
                        <h3
                          className="font-semibold text-slate-900 dark:text-slate-50 mb-1"
                        >
                          {renderHighlightedText(episode.title, query)}
                        </h3>
                        <p className="text-sm text-slate-600 dark:text-slate-400 mb-2">
                          {episode.podcast_title}
                          {episode.published_date &&
                            ` · ${new Date(episode.published_date).toLocaleDateString()}`}
                        </p>
                        {(() => {
                          const snippetToShow =
                            getEpisodeSearchSnippet(episode);

                          return snippetToShow ? (
                            <p
                              className="text-sm text-slate-500 dark:text-slate-500 line-clamp-2"
                            >
                              {renderHighlightedText(snippetToShow, query)}
                            </p>
                          ) : null;
                        })()}
                      </a>
                    ))}
                  </div>

                  {/* 展开按钮 */}
                  {shouldShowSearchExpandButton(
                    results.episodes.length,
                    EPISODES_PER_PAGE,
                  ) && (
                    <button
                      onClick={() => setExpandedEpisodes(!expandedEpisodes)}
                      className="w-full mt-3 py-2 text-sm text-blue-600 dark:text-blue-400 hover:text-blue-700 dark:hover:text-blue-300 transition-colors"
                    >
                      {getSearchExpandButtonLabel(
                        expandedEpisodes,
                        results.episodes.length,
                        "单集",
                      )}
                    </button>
                  )}
                </>
              )}
            </div>
          )}
        </div>
      </div>
    </>
  );
}
