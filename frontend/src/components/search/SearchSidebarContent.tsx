"use client";

import {
  IconAlertTriangle,
  IconClock,
  IconHeadphones,
  IconRadio,
  IconSearch,
} from "@tabler/icons-react";
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
    <div className="search-workbench-content">
      {panelState === "history" && (
        <div className="search-history">
          <div className="search-section-heading">
            <h3>
              <IconClock aria-hidden="true" stroke={1.8} />
              最近搜索
            </h3>
            <button
              onClick={onClearHistory}
              className="search-clear-history"
            >
              清空
            </button>
          </div>
          <div className="search-history-list">
            {searchHistory.map((historyQuery, index) => (
              <button
                key={index}
                onClick={() => onHistoryClick(historyQuery)}
              >
                <IconSearch aria-hidden="true" stroke={1.6} />
                {historyQuery}
              </button>
            ))}
          </div>
        </div>
      )}

      {panelState === "prompt" && (
        <div className="search-workbench-state">
          <IconSearch aria-hidden="true" stroke={1.25} />
          <p>输入关键词开始搜索</p>
          <small>支持节目标题、作者、简介和单集内容</small>
        </div>
      )}

      {panelState === "error" && (
        <div className="search-workbench-state is-error">
          <IconAlertTriangle aria-hidden="true" stroke={1.4} />
          <p>搜索失败</p>
          <small>{searchError}</small>
        </div>
      )}

      {panelState === "empty" && (
        <div className="search-workbench-state">
          <IconSearch aria-hidden="true" stroke={1.25} />
          <p>未找到相关结果</p>
          <small>试试其他关键词</small>
        </div>
      )}

      {panelState === "loading" && (
        <div className="search-workbench-loading" role="status">
          <span aria-hidden="true" />
          正在搜索
        </div>
      )}

      {panelState === "results" && (
        <div className="search-results">
          {showPodcasts && (
            <>
              {shouldShowSearchSectionHeading(
                searchType,
                results.episodes.length,
              ) && (
                <h3 className="search-results-heading">
                  <IconRadio aria-hidden="true" stroke={1.7} />
                  节目 <span>{results.podcasts.length}</span>
                </h3>
              )}
              <div className="search-podcast-results">
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
                  className="search-expand-results"
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
                  <div className="search-results-divider" />
                  <h3 className="search-results-heading">
                    <IconHeadphones aria-hidden="true" stroke={1.7} />
                    单集 <span>{results.episodes.length}</span>
                  </h3>
                </>
              )}
              <div className="search-episode-results">
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
                  className="search-expand-results"
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
