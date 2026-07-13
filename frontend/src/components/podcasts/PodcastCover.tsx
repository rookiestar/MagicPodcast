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

// 预加载距离阈值（像素）。扩大到 ~800px（约 2 行卡片高度），使图片在
// 进入视口前有足够时间完成网络请求和渲染，避免快速滚动时出现灰底占位。
const PRELOAD_MARGIN = "800px";

// 瞬时拥塞（409/429/5xx）或网络抖动下的有限退避重试：<img> 的 onError 拿不到
// 状态码，统一按失败重试，超过次数再降级为占位，避免单次瞬时失败造成永久占位。
const MAX_IMAGE_RETRIES = 2;
const IMAGE_RETRY_BASE_DELAY_MS = 400;

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
  const [retryCount, setRetryCount] = useState(0);
  const [shouldLoad, setShouldLoad] = useState(false);
  const containerRef = useRef<HTMLDivElement>(null);
  const retryTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // 获取图片URL（优先使用代理URL）
  const baseImageUrl = coverUrl ? getProxiedImageUrl(coverUrl) || coverUrl : "";

  // 检查是否是代理URL（代理URL使用普通img标签，避免Next.js图片优化器的HEAD请求问题）
  const isProxiedUrl = baseImageUrl.includes("/images/proxy");

  // 重试时附加缓存破坏后缀，强制浏览器发起新请求而非复用失败响应；仅对代理
  // URL 生效，避免改写相对路径或 data URI。
  const imageUrl =
    retryCount > 0 && isProxiedUrl
      ? `${baseImageUrl}&_retry=${retryCount}`
      : baseImageUrl;

  // 根据优先级设置加载策略。扩大高优先级范围到前 18 个（桌面端约 2-3 屏可视区域），
  // 配合后端 imageOperation.MaxConcurrent=12 的并发能力，让首屏及附近图片立即加载。
  const isHighPriority = priority === "high" || (priority !== "low" && index < 18);

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
      { rootMargin: PRELOAD_MARGIN }
    );

    observer.observe(container);

    return () => observer.disconnect();
  }, [isHighPriority]);

  // 切换封面时重置错误与重试状态，并取消尚未触发的重试，避免上一张的失败影响新封面
  useEffect(() => {
    if (retryTimerRef.current) {
      clearTimeout(retryTimerRef.current);
      retryTimerRef.current = null;
    }
    setRetryCount(0);
    setImageError(false);
  }, [coverUrl]);

  // 卸载时清理重试定时器，避免在已卸载组件上更新状态
  useEffect(() => {
    return () => {
      if (retryTimerRef.current) {
        clearTimeout(retryTimerRef.current);
        retryTimerRef.current = null;
      }
    };
  }, []);

  // 合并容器类名
  const containerClass = className
    ? `${BASE_CONTAINER_CLASS} ${className}`
    : BASE_CONTAINER_CLASS;

  // 瞬时拥塞（409/429/5xx）或网络抖动时有限退避重试，超过次数再降级为占位。
  // <img> 的 onError 不区分状态码，统一按失败重试；每次重试换新的缓存破坏后缀。
  const handleError = () => {
    if (retryCount < MAX_IMAGE_RETRIES) {
      const delay = IMAGE_RETRY_BASE_DELAY_MS * 2 ** retryCount;
      retryTimerRef.current = setTimeout(() => {
        setRetryCount((count) => count + 1);
      }, delay);
      return;
    }
    setImageError(true);
  };

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
            onError={handleError}
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
          onError={handleError}
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
