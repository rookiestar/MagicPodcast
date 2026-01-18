'use client'

import { useState, useEffect, useRef } from 'react'
import { imageLoadQueue } from '@/lib/imageLoader'
import { getProxiedImageUrl } from '@/lib/imageProxy'
import { useViewportDetectionOnce } from '@/hooks/useViewportDetection'

interface PodcastCoverProps {
  coverUrl?: string
  title: string
  index?: number
  priority?: 'high' | 'medium' | 'low'
}

export default function PodcastCover({
  coverUrl,
  title,
  index = 0,
  priority = 'medium'
}: PodcastCoverProps) {
  const [imageLoaded, setImageLoaded] = useState(false)
  const [imageError, setImageError] = useState(false)
  const imgRef = useRef<HTMLImageElement>(null)
  const taskIdRef = useRef<string>(`podcast-${title}-${Date.now()}-${Math.random()}`)

  // 视口检测：当元素进入视口时，可以提升优先级
  const { elementRef, isInView } = useViewportDetectionOnce({
    threshold: 0.1,
    rootMargin: '200px', // 提前200px触发
  })

  // 动态优先级：如果进入视口，提升为高优先级
  const effectivePriority = isInView ? 'high' : priority

  // 使用队列加载图片
  useEffect(() => {
    if (!coverUrl || !imgRef.current || imageLoaded || imageError) return

    // 获取代理后的URL
    const proxiedUrl = getProxiedImageUrl(coverUrl)

    // 根据优先级计算加载延迟
    const getLoadDelay = () => {
      switch (effectivePriority) {
        case 'high': return 0
        case 'medium': return index >= 6 ? 200 : 0
        case 'low': return index >= 15 ? 500 : 0
        default: return 0
      }
    }

    const delay = getLoadDelay()
    const taskId = taskIdRef.current

    // 延迟后加入加载队列
    const timeoutId = setTimeout(() => {
      if (imgRef.current && !imageLoaded && !imageError) {
        imageLoadQueue.add({
          id: taskId,
          src: proxiedUrl || coverUrl, // 使用代理URL或原始URL
          imgElement: imgRef.current!,
          priority: effectivePriority,
          retryCount: 0,
          onSuccess: () => {
            setImageLoaded(true)
          },
          onError: () => {
            setImageError(true)
            setImageLoaded(false)
          }
        })
      }
    }, delay)

    return () => {
      clearTimeout(timeoutId)
      imageLoadQueue.cancel(taskId)
    }
  }, [coverUrl, effectivePriority, index, imageLoaded, imageError, title, isInView])

  return (
    <div ref={elementRef} className="aspect-square bg-slate-200 relative w-full h-full overflow-hidden">
      {/* 背景层：始终显示背景色 */}
      <div className="absolute inset-0 bg-slate-200 z-0">
        {/* LQIP占位符：使用模糊的原图作为占位符 */}
        {coverUrl && !imageError && (
          <img
            src={getProxiedImageUrl(coverUrl) || coverUrl}
            alt="loading"
            className="w-full h-full object-cover blur-md opacity-40"
          />
        )}
      </div>

      {/* 加载指示器 */}
      {!imageLoaded && !imageError && coverUrl && (
        <div className="absolute inset-0 flex items-center justify-center z-10 bg-slate-200">
          <div className="w-8 h-8 border-3 border-slate-400 border-t-transparent rounded-full animate-spin" />
        </div>
      )}

      {/* 真实封面 */}
      {coverUrl ? (
        <img
          ref={imgRef}
          alt={title}
          className={`w-full h-full object-cover transition-opacity duration-300 relative z-10 ${
            imageLoaded ? 'opacity-100' : 'opacity-0'
          }`}
        />
      ) : null}

      {/* 占位符：当没有封面或图片加载失败时显示 */}
      {(!coverUrl || imageError) && (
        <div className="w-full h-full flex items-center justify-center bg-slate-200 z-20">
          <div className="text-5xl text-slate-400">🎧</div>
        </div>
      )}
    </div>
  )
}
