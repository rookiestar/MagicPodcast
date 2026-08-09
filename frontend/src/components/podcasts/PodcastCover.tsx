"use client";

import { memo, useCallback, useEffect, useRef, useState } from "react";
import { IconHeadphones } from "@tabler/icons-react";
import Image from "next/image";
import PlainImage from "@/components/ui/PlainImage";
import { canUseNextImage } from "@/lib/imageOptimization";
import { getProxiedImageUrl } from "@/lib/imageProxy";
import {
  podcastCoverLoadQueue,
  type PodcastCoverLoadPriority,
} from "@/lib/podcastCoverLoadQueue";

// 默认 sizes 常量
const DEFAULT_SIZES =
  "(max-width: 640px) 50vw, (max-width: 828px) 33vw, (max-width: 1200px) 20vw, 256px";

// 基础容器样式
const BASE_CONTAINER_CLASS =
  "aspect-square bg-slate-200 relative w-full h-full overflow-hidden";

// 预加载只覆盖下一小段滚动距离，避免屏外封面占满浏览器调度预算。
const PRELOAD_MARGIN_PX = 360;
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

function getRootRect(root: Element | null) {
  return (
    root?.getBoundingClientRect() ?? {
      top: 0,
      right: window.innerWidth,
      bottom: window.innerHeight,
      left: 0,
    }
  );
}

function isWithinPreloadRange(
  element: HTMLElement,
  root: Element | null,
): boolean {
  const targetRect = element.getBoundingClientRect();
  if (targetRect.width <= 0 || targetRect.height <= 0) {
    return false;
  }
  const rootRect = getRootRect(root);

  return (
    targetRect.bottom >= rootRect.top - PRELOAD_MARGIN_PX &&
    targetRect.top <= rootRect.bottom + PRELOAD_MARGIN_PX &&
    targetRect.right >= rootRect.left &&
    targetRect.left <= rootRect.right
  );
}

function isWithinViewport(element: HTMLElement, root: Element | null) {
  const targetRect = element.getBoundingClientRect();
  if (targetRect.width <= 0 || targetRect.height <= 0) {
    return false;
  }
  const rootRect = getRootRect(root);

  return (
    targetRect.bottom > rootRect.top &&
    targetRect.top < rootRect.bottom &&
    targetRect.right > rootRect.left &&
    targetRect.left < rootRect.right
  );
}

function getBaseLoadPriority(
  priority: PodcastCoverProps["priority"],
): PodcastCoverLoadPriority {
  if (priority === "high") {
    return "high";
  }
  if (priority === "medium") {
    return "medium";
  }
  return "low";
}

function getPriorityScore(priority: PodcastCoverLoadPriority) {
  switch (priority) {
    case "high":
      return 3;
    case "medium":
      return 2;
    case "low":
      return 1;
  }
}

function getHigherPriority(
  first: PodcastCoverLoadPriority,
  second: PodcastCoverLoadPriority,
) {
  return getPriorityScore(first) >= getPriorityScore(second) ? first : second;
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
  const [imageStarted, setImageStarted] = useState(false);
  const [requestedPriority, setRequestedPriority] =
    useState<PodcastCoverLoadPriority>(() => getBaseLoadPriority(priority));
  const requestedPriorityRef = useRef(requestedPriority);
  requestedPriorityRef.current = requestedPriority;
  const containerRef = useRef<HTMLDivElement>(null);
  const retryTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const loadHandleRef = useRef<{
    updatePriority: (nextPriority: PodcastCoverLoadPriority) => void;
    release: () => void;
  } | null>(null);

  // 获取图片URL（优先使用代理URL）
  const imageUrl = coverUrl ? getProxiedImageUrl(coverUrl) || "" : "";

  // 仅调用方确认的首屏项立即挂载；其余项目统一通过实际可见性评估加载。
  const isHighPriority = priority === "high";

  // 自动为首屏图片设置高优先级（如果未显式指定）
  const resolvedFetchPriority =
    fetchPriority ??
    (isHighPriority || requestedPriority === "medium" ? "high" : "auto");
  const resolvedLoading =
    isHighPriority || requestedPriority === "medium" ? "eager" : "lazy";

  // 使用 Intersection Observer 实现提前预加载，并在真正进入视口时升级请求优先级。
  useEffect(() => {
    if (isHighPriority) {
      setRequestedPriority("high");
      setShouldLoad(true);
      return;
    }

    const container = containerRef.current;
    if (!container) return;
    const root = getNearestScrollRoot(container);
    const basePriority = getBaseLoadPriority(priority);

    const updateLoadIntent = () => {
      const isVisible = isWithinViewport(container, root);
      setRequestedPriority(
        getHigherPriority(basePriority, isVisible ? "medium" : "low"),
      );
      setShouldLoad(true);
      return isVisible;
    };

    // 数据和滚动容器首次就绪时主动评估，避免嵌套滚动区要等一次滚动事件
    // 才收到观察器回调。
    const isInitiallyVisible = isWithinPreloadRange(container, root)
      ? updateLoadIntent()
      : false;

    const observer = new IntersectionObserver(
      ([entry]) => {
        if (entry.isIntersecting && updateLoadIntent()) {
          observer.disconnect();
        }
      },
      { root, rootMargin: PRELOAD_MARGIN, threshold: [0, 0.01] },
    );

    if (!isInitiallyVisible || !isWithinViewport(container, root)) {
      observer.observe(container);
    }

    return () => observer.disconnect();
  }, [isHighPriority, priority]);

  // 切换封面时重置错误与重试状态，并取消尚未触发的重试，避免上一张的失败影响新封面。
  useEffect(() => {
    if (retryTimerRef.current) {
      clearTimeout(retryTimerRef.current);
      retryTimerRef.current = null;
    }
    setRetryCount(0);
    setImageError(false);
    setImageLoaded(false);
    setImageStarted(false);
  }, [coverUrl]);

  // 瞬时拥塞（409/429/5xx）或网络抖动时有限退避重试，超过次数再降级为占位。
  const handleError = useCallback(() => {
    if (retryCount < MAX_IMAGE_RETRIES) {
      const delay = IMAGE_RETRY_BASE_DELAY_MS * 2 ** retryCount;
      retryTimerRef.current = setTimeout(() => {
        setRetryCount((count) => count + 1);
      }, delay);
      return;
    }
    setImageError(true);
  }, [retryCount]);

  // 只允许队列授予的真实图片元素开始请求；相同 URL 的挂载共享队列状态。
  useEffect(() => {
    if (imageError || imageLoaded || !imageUrl || !shouldLoad) {
      return;
    }

    const handle = podcastCoverLoadQueue.request({
      src: imageUrl,
      priority: requestedPriorityRef.current,
      onStart: () => setImageStarted(true),
      onError: handleError,
    });
    loadHandleRef.current = handle;

    return () => {
      if (loadHandleRef.current === handle) {
        loadHandleRef.current = null;
      }
      handle.release();
    };
  }, [handleError, imageError, imageLoaded, imageUrl, shouldLoad]);

  // 预载请求进入真实视口后只升级排队优先级，不重新订阅或重启请求。
  useEffect(() => {
    loadHandleRef.current?.updatePriority(requestedPriority);
  }, [requestedPriority]);

  // 图片请求必须有界收敛：上游既不成功也不报错时，超时后进入稳定占位。
  useEffect(() => {
    if (imageError || imageLoaded || !imageUrl || !imageStarted) {
      return;
    }

    const timer = setTimeout(() => {
      loadHandleRef.current?.release();
      loadHandleRef.current = null;
      setImageError(true);
    }, COVER_LOAD_TIMEOUT_MS);
    return () => clearTimeout(timer);
  }, [imageError, imageLoaded, imageStarted, imageUrl]);

  // 卸载时清理重试定时器，避免在已卸载组件上更新状态。
  useEffect(() => {
    return () => {
      if (retryTimerRef.current) {
        clearTimeout(retryTimerRef.current);
        retryTimerRef.current = null;
      }
    };
  }, []);

  const handleLoad = useCallback(() => {
    podcastCoverLoadQueue.complete(imageUrl);
    setImageLoaded(true);
  }, [imageUrl]);

  const handleImageError = useCallback(() => {
    if (!podcastCoverLoadQueue.fail(imageUrl)) {
      handleError();
    }
  }, [handleError, imageUrl]);

  // 合并容器类名
  const containerClass = className
    ? `${BASE_CONTAINER_CLASS} ${className}`
    : BASE_CONTAINER_CLASS;

  // 如果没有封面URL或加载失败，显示占位符。
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
        {imageStarted && (
          <PlainImage
            key={`${imageUrl}:${retryCount}`}
            src={imageUrl}
            alt={title}
            className="object-cover w-full h-full"
            loading={resolvedLoading}
            fetchPriority={resolvedFetchPriority}
            onLoad={handleLoad}
            onError={handleImageError}
          />
        )}
      </div>
    );
  }

  // 使用 Next.js Image 组件。
  return (
    <div className={containerClass} ref={containerRef}>
      {imageStarted && (
        <Image
          key={`${imageUrl}:${retryCount}`}
          src={imageUrl}
          alt={title}
          fill
          sizes={sizes}
          className="object-cover"
          priority={isHighPriority}
          loading={resolvedLoading}
          fetchPriority={resolvedFetchPriority}
          onLoad={handleLoad}
          onError={handleImageError}
        />
      )}
    </div>
  );
}

// 自定义比较函数：只在关键 props 变化时才重新渲染。
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

// 使用 React.memo 包装组件。
export default memo(PodcastCover, arePropsEqual);

// 添加 displayName 用于调试。
PodcastCover.displayName = "PodcastCover";
