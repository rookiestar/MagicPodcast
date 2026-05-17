"use client";

import { SearchEpisodeResultCard } from "@/components/search/SearchEpisodeResultCard";
import { SearchPodcastResultCard } from "@/components/search/SearchPodcastResultCard";
import {
  getSearchExpandButtonLabel,
  getVisibleSearchResults,
  shouldShowEpisodeSearchResults,
  shouldShowPodcastSearchResults,
  shouldShowSearchExpandButton,
  shouldShowSearchSectionHeading,
} from "@/lib/searchResultDisplay";
import type {
  SearchResultsData,
  SearchSidebarPanelState,
  SearchType,
} from "@/lib/searchSidebarState";

const PODCASTS_PER_PAGE = 10;
const EPISODES_PER_PAGE = 10;

interface SearchSidebarContentProps {
  panelState: SearchSidebarPanelState;
  query: string;
  searchType: SearchType;
  results: Omit<SearchResultsData, "pagination">;
  searchError: string | null;
  searchHistory: string[];
  expandedPodcasts: boolean;
  expandedEpisodes: boolean;
  onHistoryClick: (historyQuery: string) => void;
  onClearHistory: () => void;
  onTogglePodcasts: () => void;
  onToggleEpisodes: () => void;
}

export function SearchSidebarContent({
  panelState,
  query,
  searchType,
  results,
  searchError,
  searchHistory,
  expandedPodcasts,
  expandedEpisodes,
  onHistoryClick,
  onClearHistory,
  onTogglePodcasts,
  onToggleEpisodes,
}: SearchSidebarContentProps) {
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

  return (
    <div className="flex-1 overflow-y-auto">
      {panelState === "history" && (
        <div className="p-4">
          <div className="flex items-center justify-between mb-3">
            <h3 className="text-sm font-semibold text-slate-700 dark:text-slate-300">
              🕐 最近搜索
            </h3>
            <button
              onClick={onClearHistory}
              className="text-xs text-slate-500 hover:text-slate-700 dark:text-slate-400 dark:hover:text-slate-200 transition-colors"
            >
              清空
            </button>
          </div>
          <div className="space-y-2">
            {searchHistory.map((historyQuery, index) => (
              <button
                key={index}
                onClick={() => onHistoryClick(historyQuery)}
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
          <p className="text-sm mt-2">支持搜索节目标题、作者、简介和单集内容</p>
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
                  <SearchPodcastResultCard
                    key={podcast.id}
                    podcast={podcast}
                    index={index}
                    query={query}
                  />
                ))}
              </div>

              {shouldShowSearchExpandButton(
                results.podcasts.length,
                PODCASTS_PER_PAGE,
              ) && (
                <button
                  onClick={onTogglePodcasts}
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
                  <SearchEpisodeResultCard
                    key={episode.id}
                    episode={episode}
                    query={query}
                  />
                ))}
              </div>

              {shouldShowSearchExpandButton(
                results.episodes.length,
                EPISODES_PER_PAGE,
              ) && (
                <button
                  onClick={onToggleEpisodes}
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
  );
}
