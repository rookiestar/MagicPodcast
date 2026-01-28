'use client'

import { useState } from 'react'
import Image from 'next/image'
import { getProxiedImageUrl } from '@/lib/imageProxy'

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
  const [imageError, setImageError] = useState(false)

  // 获取图片URL（优先使用代理URL）
  const imageUrl = coverUrl ? (getProxiedImageUrl(coverUrl) || coverUrl) : ''

  // 根据优先级设置Next.js Image的priority属性
  const isHighPriority = priority === 'high' || index < 6

  // 如果没有封面URL或加载失败，显示占位符
  if (!imageUrl || imageError) {
    return (
      <div className="aspect-square bg-slate-200 w-full h-full flex items-center justify-center">
        <div className="text-5xl text-slate-400">🎧</div>
      </div>
    )
  }

  return (
    <div className="aspect-square bg-slate-200 relative w-full h-full overflow-hidden">
      {/* 使用Next.js Image组件 */}
      <Image
        src={imageUrl}
        alt={title}
        fill
        sizes="(max-width: 640px) 50vw, (max-width: 828px) 33vw, (max-width: 1200px) 20vw, 256px"
        className="object-cover"
        priority={isHighPriority}
        loading={isHighPriority ? 'eager' : 'lazy'}
        onError={() => setImageError(true)}
      />
    </div>
  )
}
