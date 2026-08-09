"use client";

import { useCallback, useEffect, useRef } from "react";
import Link from "next/link";
import { prefetchPodcastData, prefetchWorkflowData } from "@/lib/prefetch";

type PrefetchType = "podcast" | "workflow";

interface PrefetchLinkProps {
  href: string;
  children: React.ReactNode;
  prefetchId?: number;
  prefetchType?: PrefetchType;
  prefetch?: boolean;
  isScrolling?: boolean;
  className?: string;
  onClick?: (e: React.MouseEvent) => void;
  title?: string;
}

/**
 * 带预取功能的 Link 组件
 * 当用户 hover 时，提前加载数据到 SWR 缓存
 * 这样点击时数据已经缓存，实现即时导航
 */
export default function PrefetchLink({
  href,
  children,
  prefetchId,
  prefetchType = "podcast",
  prefetch = false,
  isScrolling = false,
  className,
  onClick,
  title,
}: PrefetchLinkProps) {
  const prefetchTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const isScrollingRef = useRef(isScrolling);
  isScrollingRef.current = isScrolling;

  const clearPrefetchTimer = useCallback(() => {
    if (prefetchTimerRef.current) {
      clearTimeout(prefetchTimerRef.current);
      prefetchTimerRef.current = null;
    }
  }, []);

  const runPrefetch = useCallback(() => {
    if (!prefetchId || isScrollingRef.current) return;

    if (prefetchType === "podcast") {
      prefetchPodcastData(prefetchId);
    } else if (prefetchType === "workflow") {
      prefetchWorkflowData(prefetchId);
    }
  }, [prefetchId, prefetchType]);

  const handleMouseEnter = useCallback(() => {
    if (!prefetchId || isScrollingRef.current) return;
    clearPrefetchTimer();
    // 延迟 100ms 预取，避免快速划过时触发不必要的请求
    prefetchTimerRef.current = setTimeout(() => {
      prefetchTimerRef.current = null;
      runPrefetch();
    }, 100);
  }, [clearPrefetchTimer, prefetchId, runPrefetch]);

  const handleFocus = useCallback(() => {
    handleMouseEnter();
  }, [handleMouseEnter]);

  useEffect(() => clearPrefetchTimer, [clearPrefetchTimer]);

  return (
    <Link
      href={href}
      prefetch={prefetch}
      className={className}
      onClick={onClick}
      onMouseEnter={handleMouseEnter}
      onMouseLeave={clearPrefetchTimer}
      onFocus={handleFocus}
      onBlur={clearPrefetchTimer}
      title={title}
    >
      {children}
    </Link>
  );
}
