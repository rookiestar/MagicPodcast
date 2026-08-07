"use client";

import { useState, useEffect, useRef, memo } from "react";
import { IconHeadphones } from "@tabler/icons-react";
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
const PRELOAD_MARGIN_PX = 800;
const PRELOAD_MARGIN = `${PRELOAD_MARGIN_PX}px`;

// 瞬时拥塞（409/429/5xx）或网络抖动下的有限退避重试：<img> 的 onError 拿不到
// 状态码，统一按失败重试，超过次数再降级为占位，避免单次瞬时失败造成永久占位。
const MAX_IMAGE_RETRIES = 2;
const IMAGE_RETRY_BASE_DELAY_MS = 400;
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

function getNearestScrollRoot(element: HTMLElement): Element | null {
  let parent = element.parentElement;
  while (parent) {
    const style = window.getComputedStyle(parent);
    if (/(auto|scroll|overlay)/.test(`${style.overflow} ${style.overflowY}`)) {
      return parent;
    }
    parent = parent.parentElement;
  }
  return null;
}

function isWithinPreloadRange(
  element: HTMLElement,
  root: Element | null,
): boolean {
  const targetRect = element.getBoundingClientRect();
  if (targetRect.width <= 0 || targetRect.height <= 0) {
    return false;
  }
  const rootRect = root?.getBoundingClientRect() ?? {
    top: 0,
    right: window.innerWidth,
    bottom: window.innerHeight,
    left: 0,
  };

  return (
    targetRect.bottom >= rootRect.top - PRELOAD_MARGIN_PX &&
    targetRect.top <= rootRect.bottom + PRELOAD_MARGIN_PX &&
    targetRect.right >= rootRect.left &&
    targetRect.left <= rootRect.right
  );
}

function PodcastCover({
  coverUrl,
  title,
  priority = "medium",
  sizes = DEFAULT_SIZES,
  className = "",
  fetchPriority,
}: PodcastCoverProps) {
  const [imageError, setImageError] = useState(false);
  const [imageLoaded, setImageLoaded] = useState(false);
  const [retryCount, setRetryCount] = useState(0);
  const [shouldLoad, setShouldLoad] = useState(false);
  const containerRef = useRef<HTMLDivElement>(null);
  const retryTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // 获取图片URL（优先使用代理URL）
  const imageUrl = coverUrl ? getProxiedImageUrl(coverUrl) || "" : "";

  // 仅调用方确认的首屏项立即挂载；其余项目统一通过实际可见性评估加载。
  const isHighPriority = priority === "high";

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
    const root = getNearestScrollRoot(container);

    // 数据和滚动容器首次就绪时主动评估，避免嵌套滚动区要等一次滚动事件
    // 才收到观察器回调。
    if (isWithinPreloadRange(container, root)) {
      setShouldLoad(true);
      return;
    }

    const observer = new IntersectionObserver(
      ([entry]) => {
        if (entry.isIntersecting) {
          setShouldLoad(true);
          observer.disconnect();
        }
      },
      { root, rootMargin: PRELOAD_MARGIN },
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
    setImageLoaded(false);
  }, [coverUrl]);

  // 图片请求必须有界收敛：上游既不成功也不报错时，超时后进入稳定占位，
  // 避免灰色骨架无限停留。失败重试仍由 handleError 使用更短的退避预算控制。
  useEffect(() => {
    if (imageError || imageLoaded || !imageUrl || !(shouldLoad || isHighPriority)) {
      return;
    }

    const timer = setTimeout(() => {
      setImageError(true);
    }, COVER_LOAD_TIMEOUT_MS);
    return () => clearTimeout(timer);
  }, [imageError, imageLoaded, imageUrl, isHighPriority, shouldLoad]);

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
  // <img> 的 onError 不区分状态码，统一按失败重试；通过 key 重挂载图片节点，
  // 但保持规范 URL 不变，避免为同一派生图制造新的浏览器/CDN 缓存项。
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
        <div
          className="w-full h-full flex items-center justify-center"
          role="img"
          aria-label={`${title}封面暂不可用`}
        >
          <IconHeadphones
            className="h-12 w-12 text-slate-400"
            aria-hidden="true"
            stroke={1.2}
          />
        </div>
      </div>
    );
  }

  if (!canUseNextImage(imageUrl)) {
    return (
      <div className={containerClass} ref={containerRef}>
        {(shouldLoad || isHighPriority) && (
          <PlainImage
            key={`${imageUrl}:${retryCount}`}
            src={imageUrl}
            alt={title}
            className="object-cover w-full h-full"
            loading={isHighPriority ? "eager" : "lazy"}
            fetchPriority={resolvedFetchPriority}
            onLoad={() => setImageLoaded(true)}
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
          key={`${imageUrl}:${retryCount}`}
          src={imageUrl}
          alt={title}
          fill
          sizes={sizes}
          className="object-cover"
          priority={isHighPriority}
          loading={isHighPriority ? "eager" : "lazy"}
          onLoad={() => setImageLoaded(true)}
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
