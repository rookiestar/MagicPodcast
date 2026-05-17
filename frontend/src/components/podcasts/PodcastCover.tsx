"use client";

import { useState, useEffect, useRef, memo } from "react";
import Image from "next/image";
import { getProxiedImageUrl } from "@/lib/imageProxy";

// 默认 sizes 常量
const DEFAULT_SIZES = "(max-width: 640px) 50vw, (max-width: 828px) 33vw, (max-width: 1200px) 20vw, 256px";

// 基础容器样式
const BASE_CONTAINER_CLASS = "aspect-square bg-slate-200 relative w-full h-full overflow-hidden";

// 预加载距离阈值（像素）
const PRELOAD_MARGIN = "200px";

interface PodcastCoverProps {
  coverUrl?: string;
  title: string;
  index?: number;
  priority?: "high" | "medium" | "low";
  sizes?: string;
  className?: string;
  fetchPriority?: "high" | "low" | "auto";
}

function PodcastCover({
  coverUrl,
  title,
  index = 0,
  priority = "medium",
  sizes = DEFAULT_SIZES,
  className = "",
  fetchPriority,
}: PodcastCoverProps) {
  const [imageError, setImageError] = useState(false);
  const [shouldLoad, setShouldLoad] = useState(false);
  const containerRef = useRef<HTMLDivElement>(null);

  // 获取图片URL（优先使用代理URL）
  const imageUrl = coverUrl ? getProxiedImageUrl(coverUrl) || coverUrl : "";

  // 检查是否是代理URL（代理URL使用普通img标签，避免Next.js图片优化器的HEAD请求问题）
  const isProxiedUrl = imageUrl.includes("/images/proxy");

  // 根据优先级设置加载策略
  const isHighPriority = priority === "high" || (priority !== "low" && index < 6);

  // 自动为首屏图片设置高优先级（如果未显式指定）
  const resolvedFetchPriority = fetchPriority ?? (isHighPriority ? "high" : "auto");

  // 使用 Intersection Observer 实现提前预加载
  useEffect(() => {
    // 高优先级图片立即加载
    if (isHighPriority) {
      setShouldLoad(true);
      return;
    }

    // 低优先级图片使用 Intersection Observer 提前预加载
    const container = containerRef.current;
    if (!container) return;

    const observer = new IntersectionObserver(
      ([entry]) => {
        if (entry.isIntersecting) {
          setShouldLoad(true);
          observer.disconnect();
        }
      },
      { rootMargin: PRELOAD_MARGIN } // 提前 200px 开始加载
    );

    observer.observe(container);

    return () => observer.disconnect();
  }, [isHighPriority]);

  // 合并容器类名
  const containerClass = className
    ? `${BASE_CONTAINER_CLASS} ${className}`
    : BASE_CONTAINER_CLASS;

  // 如果没有封面URL或加载失败，显示占位符
  if (!imageUrl || imageError) {
    return (
      <div className={containerClass} ref={containerRef}>
        <div className="w-full h-full flex items-center justify-center">
          <div className="text-5xl text-slate-400">🎧</div>
        </div>
      </div>
    );
  }

  // 对于代理URL，使用普通img标签
  if (isProxiedUrl) {
    return (
      <div className={containerClass} ref={containerRef}>
        {(shouldLoad || isHighPriority) && (
          <img
            src={imageUrl}
            alt={title}
            className="object-cover w-full h-full"
            loading={isHighPriority ? "eager" : "lazy"}
            fetchPriority={resolvedFetchPriority}
            onError={() => setImageError(true)}
          />
        )}
      </div>
    );
  }

  // 使用 Next.js Image 组件
  return (
    <div className={containerClass} ref={containerRef}>
      {(shouldLoad || isHighPriority) && (
        <Image
          src={imageUrl}
          alt={title}
          fill
          sizes={sizes}
          className="object-cover"
          priority={isHighPriority}
          loading={isHighPriority ? "eager" : "lazy"}
          onError={() => setImageError(true)}
        />
      )}
    </div>
  );
}

// 自定义比较函数：只在关键 props 变化时才重新渲染
function arePropsEqual(
  prevProps: Readonly<PodcastCoverProps>,
  nextProps: Readonly<PodcastCoverProps>,
) {
  return (
    prevProps.coverUrl === nextProps.coverUrl &&
    prevProps.title === nextProps.title &&
    prevProps.index === nextProps.index &&
    prevProps.priority === nextProps.priority &&
    prevProps.sizes === nextProps.sizes &&
    prevProps.className === nextProps.className &&
    prevProps.fetchPriority === nextProps.fetchPriority
  );
}

// 使用 React.memo 包装组件
export default memo(PodcastCover, arePropsEqual);

// 添加 displayName 用于调试
PodcastCover.displayName = "PodcastCover";
