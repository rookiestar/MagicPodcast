"use client";

import { useRef, useMemo, memo, useEffect } from "react";
import { useWindowVirtualizer } from "@tanstack/react-virtual";
import ResponsivePodcastCard from "@/components/podcasts/ResponsivePodcastCard";
import {
  getPodcastGridEstimateRowHeight,
  getPodcastGridRowGap,
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

// 单行渲染组件
const PodcastRow = memo(function PodcastRow({
  rowPodcasts,
  startIndex,
  columns,
  sortBy,
  selectedTagIds,
  isMobile,
  listStateKey,
}: {
  rowPodcasts: Podcast[];
  startIndex: number;
  columns: number;
  sortBy: string;
  selectedTagIds: number[];
  isMobile: boolean;
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
            onNavigate={() => {
              savePodcastListScrollSnapshot({
                stateKey: listStateKey,
                scrollY: window.scrollY,
                podcastIndex: index,
              });
            }}
            priority={index < 6 ? "high" : index < 15 ? "medium" : "low"}
            isMobile={isMobile}
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

  // 计算行数
  const rowCount = useMemo(
    () => Math.ceil(podcasts.length / columns),
    [podcasts.length, columns],
  );

  const estimateRowHeight = getPodcastGridEstimateRowHeight(isMobile);
  const rowGap = getPodcastGridRowGap(isMobile);

  // 使用 window 虚拟化（基于页面滚动）
  const rowVirtualizer = useWindowVirtualizer({
    count: rowCount,
    estimateSize: () => estimateRowHeight,
    overscan: 5, // 预渲染 5 行
    scrollMargin: listRef.current?.offsetTop ?? 0,
  });

  // 获取可见的虚拟行
  const virtualRows = rowVirtualizer.getVirtualItems();

  // 检测是否需要加载更多
  useEffect(() => {
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
  }, [virtualRows, rowCount, hasMore, isLoading, onLoadMore]);

  if (podcasts.length === 0) {
    return null;
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
                listStateKey={listStateKey}
              />
            </div>
          );
        })}
      </div>
    </div>
  );
}
