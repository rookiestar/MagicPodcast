"use client";

import { useEffect, useState, useRef, useCallback } from "react";
import { podcastApi } from "@/lib/api";
import { useTags } from "@/hooks/useTagSWR";
import { useUrlState } from "@/hooks/useUrlState";
import type { Podcast, Tag } from "@/types";
import ResponsivePodcastCard from "@/components/podcasts/ResponsivePodcastCard";
import { PodcastCardSkeleton } from "@/components/ui/Skeleton";
import PageLayout from "@/components/layout/PageLayout";
import SortDrawer from "@/components/podcasts/SortDrawer";
import { useSearch } from "@/contexts/SearchContext";
import { useBreakpoint, getPageSize } from "@/hooks/useBreakpoint";

type SortByType = "recent_update" | "newest_added" | "episode_count" | "title";

export default function PodcastsContent() {
  const [podcasts, setPodcasts] = useState<Podcast[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [showAllTags, setShowAllTags] = useState(false);
  const { openSearch } = useSearch();
  const { isMobile, columns } = useBreakpoint();
  const pageSize = getPageSize(columns);

  // 使用 URL 状态同步 Hook
  const [sortBy, setSortBy] = useUrlState<SortByType>("sort_by", "recent_update");
  const [selectedTagIds, setSelectedTagIds] = useUrlState<number[]>("tag_id", [], { isArray: true });

  // 使用 SWR 获取标签（自动缓存）
  const { tags: allTags } = useTags();
  // 过滤出有关联播客的标签，确保 allTags 是数组（SWR 可能返回 undefined）
  const tags = (allTags || []).filter((tag: Tag) => (tag.podcast_count || 0) > 0);

  const [page, setPage] = useState(1);
  const [hasMore, setHasMore] = useState(false);
  const [totalCount, setTotalCount] = useState(0);
  const [isSortDrawerOpen, setIsSortDrawerOpen] = useState(false);

  const observerTarget = useRef<HTMLDivElement>(null);

  const fetchPodcasts = useCallback(async (
    tagIds: number[] = [],
    pageNum: number = 1,
    currentSortBy: SortByType = "recent_update",
  ) => {
    try {
      if (pageNum !== 1) {
        setLoadingMore(true);
      } else {
        setIsLoading(true);
        setPodcasts([]);
      }
      setError(null);

      const params: Record<string, unknown> = {
        page: pageNum,
        page_size: pageSize,
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
      setLoadingMore(false);
      setIsLoading(false);
    }
  }, [pageSize]);

  // 初始化加载
  useEffect(() => {
    fetchPodcasts(selectedTagIds, 1, sortBy);
  }, [fetchPodcasts]);

  // 验证标签有效性
  useEffect(() => {
    if (tags.length === 0 || selectedTagIds.length === 0) {
      return;
    }

    const validTagIds = selectedTagIds.filter((id) =>
      tags.some((tag) => tag.id === id && (tag.podcast_count || 0) > 0),
    );

    if (validTagIds.length !== selectedTagIds.length) {
      setSelectedTagIds(validTagIds);
      setPage(1);
      setPodcasts([]);
      fetchPodcasts(validTagIds, 1, sortBy);
    }
  }, [tags, selectedTagIds, sortBy, fetchPodcasts, setSelectedTagIds]);

  const loadMore = useCallback(() => {
    if (!loadingMore && hasMore) {
      fetchPodcasts(selectedTagIds, page + 1, sortBy);
    }
  }, [loadingMore, hasMore, page, selectedTagIds, sortBy, fetchPodcasts]);

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

  const handleTagToggle = useCallback((tagId: number | null) => {
    let newSelected: number[];

    if (tagId === null) {
      newSelected = [];
    } else if (selectedTagIds.includes(tagId)) {
      newSelected = selectedTagIds.filter((id) => id !== tagId);
    } else {
      newSelected = [...selectedTagIds, tagId];
    }

    // URL 更新由 hook 自动处理
    setSelectedTagIds(newSelected);
    setPage(1);
    setPodcasts([]);
    fetchPodcasts(newSelected, 1, sortBy);
  }, [selectedTagIds, sortBy, fetchPodcasts, setSelectedTagIds]);

  const handleSortChange = useCallback((newSortBy: SortByType) => {
    // URL 更新由 hook 自动处理
    setSortBy(newSortBy);
    setPage(1);
    setPodcasts([]);
    fetchPodcasts(selectedTagIds, 1, newSortBy);
  }, [selectedTagIds, fetchPodcasts, setSortBy]);

  const sortOptions = [
    { label: "最近更新", value: "recent_update" },
    { label: "最新添加", value: "newest_added" },
    { label: "单集数量", value: "episode_count" },
    { label: "名称", value: "title" },
  ];

  // 响应式标签数量：移动端5个，桌面端8个
  const defaultTagCount = isMobile ? 5 : 8;
  const displayTags = showAllTags ? tags : tags.slice(0, defaultTagCount);
  const hasMoreTags = tags.length > defaultTagCount;

  return (
    <PageLayout
      onSearchClick={openSearch}
      toolbar={{
        breadcrumbs: [{ label: "返回首页", href: "/" }],
        title: "我的订阅",
        description: totalCount > 0 ? `共 ${totalCount} 个节目${selectedTagIds.length > 0 ? `（已选 ${selectedTagIds.length} 个标签）` : ""}` : undefined,
        rightContent: (
          <>
            {/* 移动端：排序图标按钮 */}
            <button
              onClick={() => setIsSortDrawerOpen(true)}
              className="md:hidden w-10 h-10 flex items-center justify-center rounded-lg bg-slate-100 hover:bg-slate-200 text-slate-600 active:bg-slate-300 active:scale-95 transition-all duration-200"
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
      {totalCount > 0 && (
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
              className={`min-h-[44px] px-3 py-2 rounded-lg text-sm transition-all duration-200 active:scale-95 ${
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
                  onClick={() => handleTagToggle(tag.id)}
                  className={`min-h-[44px] px-3 py-2 rounded-lg text-sm transition-all duration-200 active:scale-95 flex items-center gap-1.5 ${
                    isSelected
                      ? "bg-slate-800 text-white active:bg-slate-900"
                      : "bg-slate-100 text-slate-600 hover:bg-slate-200 active:bg-slate-300"
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
                className="w-11 h-11 min-w-[44px] min-h-[44px] rounded-lg flex items-center justify-center text-slate-500 hover:text-blue-600 hover:bg-blue-50 active:bg-blue-100 active:scale-95 transition-all duration-200"
                title={showAllTags ? "收起" : "展开"}
              >
                {showAllTags ? (
                  <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 15l7-7 7 7" />
                  </svg>
                ) : (
                  <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 9l-7 7-7-7" />
                  </svg>
                )}
              </button>
            )}

            {selectedTagIds.length > 0 && (
              <button
                onClick={() => handleTagToggle(null)}
                className="min-h-[44px] px-3 py-2 rounded-lg text-sm text-slate-500 hover:text-slate-700 hover:bg-slate-100 active:bg-slate-200 active:scale-95 transition-all duration-200"
              >
                清空
              </button>
            )}
          </div>
        </div>
      )}

      {/* Error State */}
      {error && (
        <div className="bg-red-50 border border-red-200 rounded-xl p-6 mb-6">
          <h3 className="text-red-800 font-semibold mb-2">加载失败</h3>
          <p className="text-red-600 mb-4">{error}</p>
          <button
            onClick={() => fetchPodcasts(selectedTagIds, 1, sortBy)}
            className="px-4 py-2 bg-red-600 text-white rounded hover:bg-red-700"
          >
            重试
          </button>
        </div>
      )}

      {/* Loading State - Skeleton */}
      {isLoading && !error && (
        <div className="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5 gap-3 md:gap-4 lg:gap-6">
          {Array.from({ length: 10 }).map((_, i) => (
            <PodcastCardSkeleton key={i} isMobile={isMobile} />
          ))}
        </div>
      )}

      {/* Empty State - No Results */}
      {!isLoading && !error && podcasts.length === 0 && selectedTagIds.length > 0 && (
        <div className="flex flex-col items-center justify-center py-12 px-4">
          <svg className="w-12 h-12 text-slate-300 mb-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
          </svg>
          <p className="text-slate-500 text-center">
            没有找到同时包含这些标签的节目
          </p>
          <button
            onClick={() => handleTagToggle(null)}
            className="mt-4 text-sm text-slate-600 hover:text-slate-900 underline underline-offset-2"
          >
            清空筛选
          </button>
        </div>
      )}

      {/* Empty State - No Podcasts at all */}
      {!isLoading && !error && podcasts.length === 0 && selectedTagIds.length === 0 && (
        <div className="flex flex-col items-center justify-center py-12 px-4">
          <svg className="w-12 h-12 text-slate-300 mb-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M19 11a7 7 0 01-7 7m0 0a7 7 0 01-7-7m7 7v4m0 0H8m4 0h4m-4-8a3 3 0 01-3-3V5a3 3 0 116 0v6a3 3 0 01-3 3z" />
          </svg>
          <p className="text-slate-500 text-center mb-4">
            还没有订阅任何节目
          </p>
          <div className="flex gap-3">
            <a
              href="/import"
              className="text-sm text-slate-600 hover:text-slate-900 underline underline-offset-2"
            >
              导入订阅
            </a>
          </div>
        </div>
      )}

      {/* Podcasts List */}
      {!isLoading && !error && podcasts.length > 0 && (
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
              <ResponsivePodcastCard
                key={podcast.id}
                podcast={podcast}
                detailUrl={detailUrl}
                index={index}
                priority={
                  index < 6 ? "high" : index < 15 ? "medium" : "low"
                }
                isMobile={isMobile}
              />
            );
          })}
        </div>
      )}

      {/* Loading More Indicator */}
      {!isLoading && !error && loadingMore && (
        <div className="text-center py-8">
          <div className="inline-block animate-spin rounded-full h-8 w-8 border-b-2 border-blue-600"></div>
          <p className="mt-2 text-sm text-slate-600">加载更多...</p>
        </div>
      )}

      {/* Scroll Detector - 无条件渲染以确保 IntersectionObserver 始终可用 */}
      <div ref={observerTarget} className="h-10" />

      {/* End of List Indicator */}
      {!isLoading && !error && !hasMore && podcasts.length > 0 && (
        <div className="text-center py-8 text-slate-500">已经到底了</div>
      )}
    </PageLayout>
  );
}
