"use client";

import { useEffect, useState, useRef, useCallback } from "react";
import Link from "next/link";
import { podcastApi, tagApi } from "@/lib/api";
import { stripHtml } from "@/lib/textUtils";
import { getRelativeTime, isRecentlyUpdated } from "@/lib/timeUtils";
import type { Podcast, Tag } from "@/types";
import PodcastCover from "@/components/podcasts/PodcastCover";
import PageLayout from "@/components/layout/PageLayout";
import SortDrawer from "@/components/podcasts/SortDrawer";
import { useSearch } from "@/contexts/SearchContext";

const PAGE_SIZE = 15;

type SortByType = "recent_update" | "newest_added" | "episode_count" | "title";

export default function PodcastsPage() {
  const [podcasts, setPodcasts] = useState<Podcast[]>([]);
  const [tags, setTags] = useState<Tag[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [selectedTagIds, setSelectedTagIds] = useState<number[]>([]);
  const [showAllTags, setShowAllTags] = useState(false);
  const { openSearch } = useSearch();
  const [sortBy, setSortBy] = useState<SortByType>("recent_update");

  const [listKey, setListKey] = useState(0);
  const [page, setPage] = useState(1);
  const [hasMore, setHasMore] = useState(false);
  const [totalCount, setTotalCount] = useState(0);
  const [isSortDrawerOpen, setIsSortDrawerOpen] = useState(false);

  const observerTarget = useRef<HTMLDivElement>(null);
  const tagsRefreshTimerRef = useRef<NodeJS.Timeout | null>(null);

  const fetchPodcasts = async (
    tagIds: number[] = [],
    pageNum: number = 1,
    currentSortBy: SortByType = sortBy,
  ) => {
    try {
      if (pageNum === 1) {
        setLoading(true);
      } else {
        setLoadingMore(true);
      }
      setError(null);

      const params: any = {
        page: pageNum,
        page_size: PAGE_SIZE,
        sort_by: currentSortBy,
      };

      if (tagIds.length > 0) {
        params.tag_id = tagIds;
      }

      const result = await podcastApi.list(params);

      if (pageNum === 1) {
        setPodcasts([...result.data]);
      } else {
        setPodcasts((prev) => [...prev, ...result.data]);
      }

      setHasMore(result.pagination.page < result.pagination.total_pages);
      setTotalCount(result.pagination.total);
      setPage(pageNum);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unknown error");
    } finally {
      setLoading(false);
      setLoadingMore(false);
    }
  };

  const fetchTags = async () => {
    try {
      const data = await tagApi.list();
      const tagsWithPodcasts = data.filter(
        (tag: Tag) => (tag.podcast_count || 0) > 0,
      );
      setTags(tagsWithPodcasts);
    } catch (err) {
      console.error("Failed to fetch tags:", err);
    }
  };

  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    const sortFromUrl =
      (params.get("sort_by") as SortByType) || "recent_update";
    const tagIdParams = params.getAll("tag_id");
    const tagIdsFromUrl = tagIdParams.map((id) => parseInt(id, 10));

    setSortBy(sortFromUrl);
    setSelectedTagIds(tagIdsFromUrl);

    fetchPodcasts(tagIdsFromUrl, 1, sortFromUrl);
    fetchTags();
  }, []);

  useEffect(() => {
    const handleVisibilityChange = () => {
      if (!document.hidden) {
        if (tagsRefreshTimerRef.current) {
          clearTimeout(tagsRefreshTimerRef.current);
        }

        tagsRefreshTimerRef.current = setTimeout(() => {
          fetchTags();
        }, 500);
      }
    };

    document.addEventListener("visibilitychange", handleVisibilityChange);

    return () => {
      document.removeEventListener("visibilitychange", handleVisibilityChange);
      if (tagsRefreshTimerRef.current) {
        clearTimeout(tagsRefreshTimerRef.current);
      }
    };
  }, []);

  useEffect(() => {
    if (tags.length === 0 || selectedTagIds.length === 0) {
      return;
    }

    const validTagIds = selectedTagIds.filter((id) =>
      tags.some((tag) => tag.id === id && (tag.podcast_count || 0) > 0),
    );

    if (validTagIds.length !== selectedTagIds.length) {
      const url = new URL(window.location.href);
      url.searchParams.delete("tag_id");
      validTagIds.forEach((id) =>
        url.searchParams.append("tag_id", id.toString()),
      );
      window.history.replaceState({}, "", url.toString());

      setSelectedTagIds(validTagIds);
      setPage(1);
      setPodcasts([]);
      fetchPodcasts(validTagIds, 1, sortBy);
    }
  }, [tags, selectedTagIds]);

  useEffect(() => {
    const handlePopState = () => {
      const params = new URLSearchParams(window.location.search);
      const sortFromUrl =
        (params.get("sort_by") as SortByType) || "recent_update";
      const tagIdParams = params.getAll("tag_id");
      const tagIdsFromUrl = tagIdParams.map((id) => parseInt(id, 10));

      setSortBy(sortFromUrl);
      setSelectedTagIds(tagIdsFromUrl);
      setPage(1);
      setPodcasts([]);

      fetchPodcasts(tagIdsFromUrl, 1, sortFromUrl);
    };

    window.addEventListener("popstate", handlePopState);
    return () => window.removeEventListener("popstate", handlePopState);
  }, []);

  const loadMore = useCallback(() => {
    if (!loadingMore && !loading && hasMore) {
      fetchPodcasts(selectedTagIds, page + 1, sortBy);
    }
  }, [loadingMore, loading, hasMore, page, selectedTagIds, sortBy]);

  useEffect(() => {
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries[0].isIntersecting && hasMore && !loadingMore) {
          loadMore();
        }
      },
      { rootMargin: "200px" },
    );

    const currentTarget = observerTarget.current;
    if (currentTarget) {
      observer.observe(currentTarget);
    }

    return () => {
      if (currentTarget) {
        observer.unobserve(currentTarget);
      }
    };
  }, [hasMore, loadingMore, loadMore]);

  const handleTagToggle = (tagId: number | null) => {
    let newSelected: number[];

    if (tagId === null) {
      newSelected = [];
    } else if (selectedTagIds.includes(tagId)) {
      newSelected = selectedTagIds.filter((id) => id !== tagId);
    } else {
      newSelected = [...selectedTagIds, tagId];
    }

    const url = new URL(window.location.href);
    url.searchParams.delete("tag_id");
    newSelected.forEach((id) =>
      url.searchParams.append("tag_id", id.toString()),
    );
    window.history.replaceState({}, "", url.toString());

    setSelectedTagIds(newSelected);
    setPage(1);
    setPodcasts([]);
    fetchPodcasts(newSelected, 1, sortBy);
  };

  const handleSortChange = (newSortBy: SortByType) => {
    const url = new URL(window.location.href);
    url.searchParams.set("sort_by", newSortBy);
    window.history.replaceState({}, "", url.toString());

    setSortBy(newSortBy);
    setPage(1);
    setPodcasts([]);
    fetchPodcasts(selectedTagIds, 1, newSortBy);
  };

  const sortOptions = [
    { label: "最近更新", value: "recent_update" },
    { label: "最新添加", value: "newest_added" },
    { label: "单集数量", value: "episode_count" },
    { label: "名称", value: "title" },
  ];

  // 响应式标签数量：移动端5个，桌面端8个
  const [defaultTagCount, setDefaultTagCount] = useState(5);

  useEffect(() => {
    const updateTagCount = () => {
      setDefaultTagCount(window.innerWidth < 640 ? 5 : 8);
    };

    updateTagCount();
    window.addEventListener('resize', updateTagCount);

    return () => window.removeEventListener('resize', updateTagCount);
  }, []);

  const displayTags = showAllTags ? tags : tags.slice(0, defaultTagCount);
  const hasMoreTags = tags.length > defaultTagCount;

  return (
    <PageLayout
      onSearchClick={openSearch}
      toolbar={{
        breadcrumbs: [{ label: "返回首页", href: "/" }],
        title: "我的订阅",
        description: !loading && totalCount > 0 ? `共 ${totalCount} 个节目${selectedTagIds.length > 0 ? `（已选 ${selectedTagIds.length} 个标签）` : ""}` : undefined,
        rightContent: (
          <>
            {/* 移动端：排序图标按钮 */}
            <button
              onClick={() => setIsSortDrawerOpen(true)}
              className="md:hidden w-10 h-10 flex items-center justify-center rounded-lg bg-slate-100 hover:bg-slate-200 text-slate-600 transition-colors"
              aria-label="排序"
            >
              <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M3 4h13M3 8h9m-9 4h6m4 0l4-4m0 0l4 4m-4-4h12" />
              </svg>
            </button>

            {/* 桌面端：完整排序选择器 */}
            <div className="hidden md:flex items-center gap-2">
              <span className="text-sm text-slate-600">排序：</span>
              <select
                value={sortBy}
                onChange={(e) => handleSortChange(e.target.value as SortByType)}
                className="
                  px-3 py-2 pr-8
                  border border-slate-300 rounded-lg
                  bg-white text-sm text-slate-700
                  focus:ring-2 focus:ring-violet-500 focus:border-transparent
                  transition-colors
                  appearance-none
                  cursor-pointer
                "
              >
                {sortOptions.map((option) => (
                  <option key={option.value} value={option.value}>
                    {option.label}
                  </option>
                ))}
              </select>
            </div>
          </>
        ),
      }}
    >
      {/* 移动端统计信息条 */}
      {!loading && totalCount > 0 && (
        <div className="md:hidden px-4 py-2 bg-slate-50 border-b border-slate-200">
          <p className="text-sm text-slate-600">
            共 {totalCount} 个节目
            {selectedTagIds.length > 0 && `（已选 ${selectedTagIds.length} 个标签）`}
          </p>
        </div>
      )}

      {/* 排序抽屉 */}
      <SortDrawer
        isOpen={isSortDrawerOpen}
        onClose={() => setIsSortDrawerOpen(false)}
        currentSort={sortBy}
        onSortChange={handleSortChange}
        options={sortOptions}
      />
      {/* Tag Filter */}
      {tags.length > 0 && (
        <div className="mt-4 sm:mt-6 md:mt-8 mb-3 sm:mb-4 md:mb-6">
          <div className="flex flex-wrap gap-2 sm:gap-3 items-center">
            <button
              onClick={() => handleTagToggle(null)}
              className={`min-h-[44px] px-3 py-2 rounded-lg text-sm transition-colors ${
                selectedTagIds.length === 0
                  ? "bg-slate-800 text-white"
                  : "bg-slate-100 text-slate-600 hover:bg-slate-200"
              }`}
            >
              全部
            </button>

            {displayTags.map((tag) => {
              const isSelected = selectedTagIds.includes(tag.id);
              return (
                <button
                  key={tag.id}
                  onClick={() => handleTagToggle(tag.id)}
                  className={`min-h-[44px] px-3 py-2 rounded-lg text-sm transition-colors flex items-center gap-1.5 ${
                    isSelected
                      ? "bg-slate-800 text-white"
                      : "bg-slate-100 text-slate-600 hover:bg-slate-200"
                  }`}
                  title={tag.name}
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
                onClick={() => setShowAllTags(!showAllTags)}
                className="w-11 h-11 min-w-[44px] min-h-[44px] rounded-lg flex items-center justify-center text-slate-500 hover:text-blue-600 hover:bg-blue-50 transition-all"
                title={showAllTags ? "收起" : "展开"}
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
                onClick={() => handleTagToggle(null)}
                className="min-h-[44px] px-3 py-2 rounded-lg text-sm text-slate-500 hover:text-slate-700 hover:bg-slate-100 transition-colors"
              >
                清空
              </button>
            )}
          </div>
        </div>
      )}

      {/* Loading State */}
      {loading && (
        <div className="text-center py-12">
          <div className="inline-block animate-spin rounded-full h-12 w-12 border-b-2 border-blue-600"></div>
          <p className="mt-4 text-slate-600">加载中...</p>
        </div>
      )}

      {/* Error State */}
      {error && (
        <div className="bg-red-50 border border-red-200 rounded-xl p-6 mb-6">
          <h3 className="text-red-800 font-semibold mb-2">加载失败</h3>
          <p className="text-red-600 mb-4">{error}</p>
          <button
            onClick={() => fetchPodcasts(selectedTagIds, 1)}
            className="px-4 py-2 bg-red-600 text-white rounded hover:bg-red-700"
          >
            重试
          </button>
        </div>
      )}

      {/* Podcasts List */}
      {!loading && !error && (
        <>
          <div className="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5 gap-3 md:gap-4 lg:gap-6">
            {podcasts.map((podcast, index) => {
              const params = new URLSearchParams();
              if (sortBy) {
                params.append("sort_by", sortBy);
              }
              if (selectedTagIds.length > 0) {
                params.append("tag_ids", selectedTagIds.join(","));
              }
              const queryString = params.toString();
              const detailUrl = `/podcasts/${podcast.id}${queryString ? `?${queryString}` : ""}`;

              return (
                <PodcastCard
                  key={podcast.id}
                  podcast={podcast}
                  detailUrl={detailUrl}
                  index={index}
                  priority={
                    index < 6 ? "high" : index < 15 ? "medium" : "low"
                  }
                />
              );
            })}
          </div>

          {loadingMore && (
            <div className="text-center py-8">
              <div className="inline-block animate-spin rounded-full h-8 w-8 border-b-2 border-blue-600"></div>
              <p className="mt-2 text-sm text-slate-600">加载更多...</p>
            </div>
          )}

          <div ref={observerTarget} className="h-10" />

          {!hasMore && podcasts.length > 0 && (
            <div className="text-center py-8 text-slate-500">已经到底了</div>
          )}
        </>
      )}
    </PageLayout>
  );
}

function PodcastCard({
  podcast,
  detailUrl,
  index = 0,
  priority = "medium",
}: {
  podcast: Podcast;
  detailUrl: string;
  index?: number;
  priority?: "high" | "medium" | "low";
}) {
  const displayTags = podcast.tags?.slice(0, 3) || [];
  const remainingTags = (podcast.tags?.length || 0) - 3;

  const recentlyUpdated = isRecentlyUpdated(podcast.newest_episode_date, 7);
  const relativeTime = getRelativeTime(podcast.newest_episode_date);

  return (
    <Link href={detailUrl}>
      <div className="bg-white rounded-xl shadow-md hover:shadow-lg active:scale-[0.98] active:shadow-sm transition-all duration-200 overflow-hidden cursor-pointer h-full flex flex-col touch-action-manipulation">
        <div className="relative mx-auto w-36 sm:w-44 md:w-52 lg:w-72 h-36 sm:h-44 md:h-48 lg:h-72">
          <PodcastCover
            coverUrl={podcast.cover_url}
            title={podcast.title}
            index={index}
            priority={priority}
          />

          {recentlyUpdated && (
            <div className="absolute bottom-0 right-0 m-2 z-30">
              <span className="inline-flex items-center gap-1 px-2 py-1 text-xs rounded-full bg-white text-slate-800 shadow-sm">
                <span className="w-1.5 h-1.5 rounded-full bg-green-600" />
                新更新
              </span>
            </div>
          )}
        </div>

        <div className="p-1.5 sm:p-2 md:p-4 flex-1 flex flex-col">
          <h3 className="text-sm sm:text-base md:text-lg font-semibold text-slate-900 mb-0.5 md:mb-1.5 line-clamp-2 leading-tight">
            {podcast.title}
          </h3>

          <p className="text-xs sm:text-sm text-slate-600 mb-0.5 md:mb-2">{podcast.author}</p>

          <p className="text-xs sm:text-sm text-slate-500 line-clamp-2 md:line-clamp-3 leading-snug md:leading-relaxed mb-1 md:mb-4">
            {stripHtml(podcast.description, 100)}
          </p>

          <div className="mt-auto pt-1 md:pt-3 space-y-1 md:space-y-3">
            {displayTags.length > 0 && (
              <div className="flex flex-wrap gap-1.5">
                {displayTags.map((tag) => (
                  <span
                    key={tag.id}
                    className="inline-flex items-center gap-1 px-2 py-0.5 text-xs rounded-full bg-slate-100 hover:bg-slate-200 transition-colors group relative"
                  >
                    <span
                      className="w-1.5 h-1.5 rounded-full flex-shrink-0"
                      style={{ backgroundColor: tag.color }}
                    />
                    <span className="max-w-[80px] truncate" title={tag.name}>
                      {tag.name}
                    </span>
                  </span>
                ))}
                {remainingTags > 0 && (
                  <span className="inline-flex items-center px-2 py-0.5 text-xs rounded-full bg-slate-100 text-slate-500">
                    +{remainingTags}
                  </span>
                )}
              </div>
            )}

            <div className="flex items-center justify-between text-xs sm:text-sm md:text-base text-slate-500">
              <span className="font-medium">{podcast.episode_count} 集</span>
              <span className="text-[10px] sm:text-xs md:text-sm text-slate-400">{relativeTime}</span>
            </div>
          </div>
        </div>
      </div>
    </Link>
  );
}
