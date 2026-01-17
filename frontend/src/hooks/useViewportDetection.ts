import { useEffect, useRef, useState } from 'react'

/**
 * 视口检测 Hook
 * 检测元素是否进入视口，可用于动态提升图片加载优先级
 */
export function useViewportDetection(
  options: {
    threshold?: number // 触发阈值（0-1），默认0.1表示10%可见时触发
    rootMargin?: string // 根边距，默认'100px'（提前100px触发）
  } = {}
) {
  const elementRef = useRef<HTMLDivElement>(null)
  const [isInView, setIsInView] = useState(false)

  const { threshold = 0.1, rootMargin = '100px' } = options

  useEffect(() => {
    const element = elementRef.current
    if (!element) return

    // 创建 Intersection Observer
    const observer = new IntersectionObserver(
      (entries) => {
        entries.forEach((entry) => {
          if (entry.isIntersecting) {
            setIsInView(true)
            // 元素进入视口后，可以选择停止观察
            // observer.unobserve(element)
          } else {
            setIsInView(false)
          }
        })
      },
      {
        threshold,
        rootMargin,
      }
    )

    // 开始观察
    observer.observe(element)

    // 清理
    return () => {
      observer.unobserve(element)
    }
  }, [threshold, rootMargin])

  return { elementRef, isInView }
}

/**
 * 视口检测 Hook（仅触发一次）
 * 元素首次进入视口后不再改变状态
 */
export function useViewportDetectionOnce(options: {
  threshold?: number
  rootMargin?: string
} = {}) {
  const elementRef = useRef<HTMLDivElement>(null)
  const [isInView, setIsInView] = useState(false)
  const hasTriggeredRef = useRef(false)

  const { threshold = 0.1, rootMargin = '100px' } = options

  useEffect(() => {
    const element = elementRef.current
    if (!element || hasTriggeredRef.current) return

    // 创建 Intersection Observer
    const observer = new IntersectionObserver(
      (entries) => {
        entries.forEach((entry) => {
          if (entry.isIntersecting && !hasTriggeredRef.current) {
            setIsInView(true)
            hasTriggeredRef.current = true
            // 元素进入视口后停止观察
            observer.unobserve(element)
          }
        })
      },
      {
        threshold,
        rootMargin,
      }
    )

    // 开始观察
    observer.observe(element)

    // 清理
    return () => {
      observer.unobserve(element)
    }
  }, [threshold, rootMargin])

  return { elementRef, isInView }
}
