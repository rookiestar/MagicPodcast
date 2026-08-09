"use client";

import { useRef, useMemo, memo, useEffect, useState } from "react";
import { useWindowVirtualizer } from "@tanstack/react-virtual";
import ResponsivePodcastCard from "@/components/podcasts/ResponsivePodcastCard";
import {
  getPodcastGridCoverPriority,
  getPodcastGridEstimateRowHeight,
  getPodcastGridRowGap,
  getPodcastGridOverscan,
  getLastVisiblePodcastRowIndex,
  shouldLoadMorePodcastRows,
} from "@/lib/podcastGridVirtualization";
import { savePodcastListScrollSnapshot } from "@/lib/podcastListScrollState";
import type { Podcast } from "@/types";

interface VirtualPodcastGridProps {
  podcasts: Podcast[];
  columns: number;
  isMobile: boolean;
  listStateKey: string;
  sortBy: string;
  selectedTagIds: number[];
  onLoadMore?: () => void;
  hasMore?: boolean;
  isLoading?: boolean;
}

const SERVER_FALLBACK_ITEM_LIMIT = 15;

function getPodcastDetailUrl(
  podcastId: number,
  sortBy: string,
  selectedTagIds: number[],
) {
  const params = new URLSearchParams();
  if (sortBy) {
    params.append("sort_by", sortBy);
  }
  if (selectedTagIds.length > 0) {
    params.append("tag_ids", selectedTagIds.join(","));
  }
  const queryString = params.toString();
  return `/podcasts/${podcastId}${queryString ? `?${queryString}` : ""}`;
}

// 单行渲染组件
const PodcastRow = memo(function PodcastRow({
  rowPodcasts,
  startIndex,
  columns,
  sortBy,
  selectedTagIds,
  isMobile,
  isScrolling,
  listStateKey,
}: {
  rowPodcasts: Podcast[];
  startIndex: number;
  columns: number;
  sortBy: string;
  selectedTagIds: number[];
  isMobile: boolean;
  isScrolling: boolean;
  listStateKey: string;
}) {
  // 过滤掉可能的 undefined 元素
  const validPodcasts = rowPodcasts.filter(
    (p): p is Podcast => p != null && p.id != null,
  );

  return (
    <div
      className="grid gap-3 md:gap-4 lg:gap-6"
      style={{
        gridTemplateColumns: `repeat(${columns}, minmax(0, 1fr))`,
      }}
    >
      {validPodcasts.map((podcast, colIndex) => {
        const index = startIndex + colIndex;
        const detailUrl = getPodcastDetailUrl(
          podcast.id,
          sortBy,
          selectedTagIds,
        );

        return (
          <ResponsivePodcastCard
            key={podcast.id}
            podcast={podcast}
            detailUrl={detailUrl}
            index={index}
            onNavigate={() => {
              savePodcastListScrollSnapshot({
                stateKey: listStateKey,
                scrollY: window.scrollY,
                podcastIndex: index,
              });
            }}
            priority={getPodcastGridCoverPriority(index, columns, isMobile)}
            isMobile={isMobile}
            isScrolling={isScrolling}
          />
        );
      })}
    </div>
  );
});

export default function VirtualPodcastGrid({
  podcasts,
  columns,
  isMobile,
  listStateKey,
  sortBy,
  selectedTagIds,
  onLoadMore,
  hasMore = false,
  isLoading = false,
}: VirtualPodcastGridProps) {
  const listRef = useRef<HTMLDivElement>(null);
  const lastLoadMoreRowCountRef = useRef<number | null>(null);
  const scrollingStopTimerRef = useRef<ReturnType<typeof setTimeout> | null>(
    null,
  );
  const [hasUserScrolled, setHasUserScrolled] = useState(false);
  const [isScrolling, setIsScrolling] = useState(false);

  // 计算行数
  const rowCount = useMemo(
    () => Math.ceil(podcasts.length / columns),
    [podcasts.length, columns],
  );

  const estimateRowHeight = getPodcastGridEstimateRowHeight(isMobile);
  const rowGap = getPodcastGridRowGap(isMobile);

  // 使用 window 虚拟化（基于页面滚动）
  // 只保留约 2 个桌面行 / 4 个移动行的 overscan，避免屏外封面和详情预取
  // 抢占当前屏资源；已成功封面由共享队列和浏览器缓存复用。
  const rowVirtualizer = useWindowVirtualizer({
    count: rowCount,
    estimateSize: () => estimateRowHeight,
    overscan: getPodcastGridOverscan(isMobile),
    scrollMargin: listRef.current?.offsetTop ?? 0,
  });

  // 获取可见的虚拟行
  const virtualRows = rowVirtualizer.getVirtualItems();

  useEffect(() => {
    const handleScroll = () => {
      if (window.scrollY > 0) {
        setHasUserScrolled(true);
      }

      setIsScrolling(true);
      if (scrollingStopTimerRef.current) {
        clearTimeout(scrollingStopTimerRef.current);
      }
      scrollingStopTimerRef.current = setTimeout(() => {
        scrollingStopTimerRef.current = null;
        setIsScrolling(false);
      }, 120);
    };

    window.addEventListener("scroll", handleScroll, { passive: true });
    if (window.scrollY > 0) {
      setHasUserScrolled(true);
    }

    return () => {
      window.removeEventListener("scroll", handleScroll);
      if (scrollingStopTimerRef.current) {
        clearTimeout(scrollingStopTimerRef.current);
        scrollingStopTimerRef.current = null;
      }
    };
  }, []);

  // 检测是否需要加载更多
  useEffect(() => {
    if (!hasUserScrolled) {
      return;
    }

    const lastVisibleRowIndex = getLastVisiblePodcastRowIndex(
      virtualRows,
      window.scrollY + window.innerHeight,
    );
    const shouldLoadMore = shouldLoadMorePodcastRows({
      lastVisibleRowIndex,
      rowCount,
      hasMore,
      isLoading,
    });

    if (!shouldLoadMore) {
      if (!hasMore) {
        lastLoadMoreRowCountRef.current = null;
      }
      return;
    }

    if (onLoadMore && lastLoadMoreRowCountRef.current !== rowCount) {
      lastLoadMoreRowCountRef.current = rowCount;
      onLoadMore();
    }
  }, [hasUserScrolled, virtualRows, rowCount, hasMore, isLoading, onLoadMore]);

  if (podcasts.length === 0) {
    return null;
  }

  // Window virtualization has no measured rows during server rendering and
  // the first client render. Keep the first batch as real responsive cards so
  // useful content does not wait for hydration; virtualization replaces this
  // bounded fallback as soon as measurements are available.
  if (virtualRows.length === 0) {
    return (
      <div
        ref={listRef}
        className="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5 gap-3 md:gap-4 lg:gap-6"
      >
        {podcasts
          .slice(0, SERVER_FALLBACK_ITEM_LIMIT)
          .map((podcast, index) => (
            <ResponsivePodcastCard
              key={podcast.id}
              podcast={podcast}
              detailUrl={getPodcastDetailUrl(
                podcast.id,
                sortBy,
                selectedTagIds,
              )}
              index={index}
              onNavigate={() => {
                savePodcastListScrollSnapshot({
                  stateKey: listStateKey,
                  scrollY: window.scrollY,
                  podcastIndex: index,
                });
              }}
              priority={getPodcastGridCoverPriority(
                index,
                columns,
                isMobile,
              )}
              isMobile={isMobile}
              isScrolling={isScrolling}
            />
          ))}
      </div>
    );
  }

  return (
    <div ref={listRef} style={{ position: "relative" }}>
      <div
        style={{
          height: rowVirtualizer.getTotalSize(),
          width: "100%",
          position: "relative",
        }}
      >
        {virtualRows.map((virtualRow) => {
          const startIndex = virtualRow.index * columns;
          const rowPodcasts = podcasts.slice(
            virtualRow.index * columns,
            (virtualRow.index + 1) * columns
          );

          return (
            <div
              key={virtualRow.key}
              data-index={virtualRow.index}
              ref={rowVirtualizer.measureElement}
              style={{
                position: "absolute",
                top: 0,
                left: 0,
                width: "100%",
                paddingBottom: rowGap,
                transform: `translateY(${virtualRow.start - rowVirtualizer.options.scrollMargin}px)`,
              }}
            >
              <PodcastRow
                rowPodcasts={rowPodcasts}
                startIndex={startIndex}
                columns={columns}
                sortBy={sortBy}
                selectedTagIds={selectedTagIds}
                isMobile={isMobile}
                isScrolling={isScrolling}
                listStateKey={listStateKey}
              />
            </div>
          );
        })}
      </div>
    </div>
  );
}
