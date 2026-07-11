import { useState, useEffect, useRef } from "react";

interface InfiniteScrollTriggerOptions extends IntersectionObserverInit {
  enabled?: boolean;
}

const parseRootMarginPixels = (rootMargin: string) => {
  const firstValue = rootMargin.trim().split(/\s+/)[0];
  if (!firstValue.endsWith("px")) {
    return 0;
  }

  const parsed = Number.parseFloat(firstValue);
  return Number.isFinite(parsed) ? parsed : 0;
};

const getViewportBounds = (root: Element | Document | null) => {
  if (root && "getBoundingClientRect" in root) {
    const rect = root.getBoundingClientRect();
    return { top: rect.top, bottom: rect.bottom };
  }

  const viewportHeight =
    window.innerHeight || document.documentElement.clientHeight;
  return { top: 0, bottom: viewportHeight };
};

const getScrollTarget = (root: Element | Document | null) => {
  if (!root) {
    return window;
  }

  return root;
};

const isElementNearViewport = (
  element: HTMLElement,
  root: Element | Document | null,
  rootMargin: string,
) => {
  const rect = element.getBoundingClientRect();
  const bounds = getViewportBounds(root);
  const margin = parseRootMarginPixels(rootMargin);

  return rect.top <= bounds.bottom + margin && rect.bottom >= bounds.top - margin;
};

/**
 * 使用Intersection Observer实现无限滚动
 *
 * @param callback - 当触发滚动时调用的回调
 * @param options - Intersection Observer选项
 * @returns { ref, observer }
 *
 * @example
 * const { ref } = useInfiniteScrollTrigger(() => {
 *   loadMore()
 * })
 *
 * return <div ref={ref}>Loading more...</div>
 */
export function useInfiniteScrollTrigger(
  callback: () => void,
  options: InfiniteScrollTriggerOptions = {},
) {
  const [element, setElement] = useState<HTMLElement | null>(null);
  const observer = useRef<IntersectionObserver | null>(null);
  const callbackRef = useRef(callback);
  const hasTriggeredRef = useRef(false);
  const {
    enabled = true,
    root = null,
    rootMargin = "200px",
    threshold = 0.1,
  } = options;

  useEffect(() => {
    callbackRef.current = callback;
  }, [callback]);

  useEffect(() => {
    if (!element || !enabled) {
      return;
    }

    hasTriggeredRef.current = false;
    const triggerOnce = () => {
      if (hasTriggeredRef.current) {
        return;
      }
      hasTriggeredRef.current = true;
      callbackRef.current();
    };

    if (typeof IntersectionObserver === "undefined") {
      const scrollTarget = getScrollTarget(root);
      const handleScroll = () => {
        if (isElementNearViewport(element, root, rootMargin)) {
          triggerOnce();
        }
      };

      handleScroll();
      scrollTarget.addEventListener("scroll", handleScroll, { passive: true });
      window.addEventListener("resize", handleScroll);

      return () => {
        scrollTarget.removeEventListener("scroll", handleScroll);
        window.removeEventListener("resize", handleScroll);
      };
    }

    // 创建Intersection Observer
    observer.current = new IntersectionObserver(
      (entries) => {
        const [entry] = entries;
        if (entry.isIntersecting) {
          triggerOnce();
        }
      },
      {
        root,
        rootMargin,
        threshold,
      },
    );

    // 开始观察
    observer.current.observe(element);

    // 清理
    return () => {
      if (observer.current) {
        observer.current.disconnect();
      }
    };
  }, [element, enabled, root, rootMargin, threshold]);

  return {
    ref: setElement,
  };
}
