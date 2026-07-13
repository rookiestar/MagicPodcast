"use client";

import { useState, useEffect, useRef, memo } from "react";
import Image from "next/image";
import PlainImage from "@/components/ui/PlainImage";
import { canUseNextImage } from "@/lib/imageOptimization";
import { getProxiedImageUrl } from "@/lib/imageProxy";

// 默认 sizes 常量
const DEFAULT_SIZES = "(max-width: 640px) 50vw, (max-width: 828px) 33vw, (max-width: 1200px) 20vw, 256px";

// 基础容器样式
const BASE_CONTAINER_CLASS = "aspect-square bg-slate-200 relative w-full h-full overflow-hidden";

// 预加载距离阈值（像素）
const PRELOAD_MARGIN = "200px";

// 封面加载的有界上限：超过该时长仍未完成加载（无 onLoad/onError）时收敛到稳定占位，
// 避免慢速或挂起的图片请求让封面永久停留在灰色块。
const COVER_LOAD_TIMEOUT_MS = 15_000;

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
  const [imageLoaded, setImageLoaded] = useState(false);
  const [shouldLoad, setShouldLoad] = useState(false);
  const containerRef = useRef<HTMLDivElement>(null);

  // 获取图片URL：统一走安全代理，未在白名单内的远端来源返回 undefined，
  // 由占位符承接，不再回退到原始 URL 以避免浏览器直连绕过代理与私网阻断。
  const imageUrl = coverUrl ? getProxiedImageUrl(coverUrl) : "";

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

  // 有界加载收敛：图片开始加载后，若在上限内既未完成也未报错，则收敛到稳定占位。
  useEffect(() => {
    if (imageError || imageLoaded || !imageUrl) {
      return;
    }
    // 仅当图片确实开始加载（首屏高优先级或已进入预加载）时启动计时。
    if (!(shouldLoad || isHighPriority)) {
      return;
    }
    const timer = setTimeout(() => {
      setImageError(true);
    }, COVER_LOAD_TIMEOUT_MS);
    return () => clearTimeout(timer);
  }, [imageError, imageLoaded, imageUrl, shouldLoad, isHighPriority]);

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

  if (!canUseNextImage(imageUrl)) {
    return (
      <div className={containerClass} ref={containerRef}>
        {(shouldLoad || isHighPriority) && (
          <PlainImage
            src={imageUrl}
            alt={title}
            className="object-cover w-full h-full"
            loading={isHighPriority ? "eager" : "lazy"}
            fetchPriority={resolvedFetchPriority}
            onLoad={() => setImageLoaded(true)}
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
          unoptimized={isProxiedUrl}
          className="object-cover"
          priority={isHighPriority}
          loading={isHighPriority ? "eager" : "lazy"}
          onLoad={() => setImageLoaded(true)}
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
