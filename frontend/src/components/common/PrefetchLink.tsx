"use client";

import { useCallback } from "react";
import Link from "next/link";
import { prefetchPodcastData, prefetchWorkflowData } from "@/lib/prefetch";

type PrefetchType = "podcast" | "workflow";

interface PrefetchLinkProps {
  href: string;
  children: React.ReactNode;
  prefetchId?: number;
  prefetchType?: PrefetchType;
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
  className,
  onClick,
  title,
}: PrefetchLinkProps) {
  const handleMouseEnter = useCallback(() => {
    if (!prefetchId) return;

    // 延迟 100ms 预取，避免快速划过时触发不必要的请求
    const timer = setTimeout(() => {
      if (prefetchType === "podcast") {
        prefetchPodcastData(prefetchId);
      } else if (prefetchType === "workflow") {
        prefetchWorkflowData(prefetchId);
      }
    }, 100);

    // 清理函数 - 组件卸载时清理定时器
    return () => clearTimeout(timer);
  }, [prefetchId, prefetchType]);

  return (
    <Link
      href={href}
      className={className}
      onClick={onClick}
      onMouseEnter={handleMouseEnter}
      title={title}
    >
      {children}
    </Link>
  );
}
