'use client'

import { useState } from 'react'
import type { Episode } from '@/types'
import RichText from '@/components/RichText'

interface EpisodeCardProps {
  episode: Episode
  podcastCover?: string
}

export default function EpisodeCard({ episode, podcastCover }: EpisodeCardProps) {
  const [isExpanded, setIsExpanded] = useState(false)

  // 格式化时长
  const formatDuration = (seconds: number) => {
    if (!seconds || seconds <= 0) return null
    const hours = Math.floor(seconds / 3600)
    const minutes = Math.floor((seconds % 3600) / 60)
    const secs = seconds % 60

    const parts = []
    if (hours > 0) {
      parts.push(`${hours}小时`)
    }
    if (minutes > 0) {
      parts.push(`${minutes}分`)
    }
    if (secs > 0 || parts.length === 0) {
      parts.push(`${secs}秒`)
    }

    return parts.join('')
  }

  // 格式化日期
  const formatDate = (dateString: string) => {
    try {
      const date = new Date(dateString)
      return date.toLocaleDateString('zh-CN', {
        year: 'numeric',
        month: '2-digit',
        day: '2-digit'
      })
    } catch {
      return dateString
    }
  }

  // 格式化文件大小
  const formatFileSize = (bytes: number) => {
    if (!bytes || bytes <= 0) return null
    const mb = bytes / (1024 * 1024)
    return `${mb.toFixed(1)} MB`
  }

  const coverImage = episode.image_url || podcastCover

  return (
    <div className="group relative bg-white dark:bg-slate-800 rounded-xl shadow-sm hover:shadow-xl transition-all duration-300 overflow-hidden border border-slate-200 dark:border-slate-700">
      {/* Content */}
      <div className="p-4">
        {/* Title with Thumbnail */}
        <div className="flex items-start gap-3 mb-3">
          {/* Thumbnail */}
          <div className="flex-shrink-0 w-16 h-16 rounded-lg overflow-hidden bg-slate-200 dark:bg-slate-700 relative">
            {coverImage ? (
              <img
                src={coverImage}
                alt={episode.title}
                className="w-full h-full object-cover"
              />
            ) : (
              <div className="w-full h-full flex items-center justify-center text-2xl">
                🎧
              </div>
            )}
          </div>

          {/* Title and Info */}
          <div className="flex-1 min-w-0">
            <div className="flex items-start justify-between gap-2">
              {episode.link ? (
                <a
                  href={episode.link}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="flex-1 font-semibold text-slate-900 dark:text-slate-50 text-base line-clamp-2 leading-snug hover:text-blue-600 dark:hover:text-blue-400 transition-colors"
                >
                  {episode.title}
                </a>
              ) : (
                <span className="flex-1 font-semibold text-slate-900 dark:text-slate-50 text-base line-clamp-2 leading-snug">
                  {episode.title}
                </span>
              )}

              {/* Play Button Icon */}
              {episode.medium_url && (
                <button
                  onClick={(e) => {
                    e.preventDefault()
                    e.stopPropagation()
                    window.open(episode.medium_url, '_blank')
                  }}
                  className="flex-shrink-0 w-8 h-8 flex items-center justify-center rounded-full bg-blue-600 hover:bg-blue-700 text-white transition-all duration-200 hover:scale-110"
                  aria-label="播放"
                >
                  <svg className="w-4 h-4 ml-0.5" fill="currentColor" viewBox="0 0 24 24">
                    <path d="M8 5v14l11-7z" />
                  </svg>
                </button>
              )}
            </div>
          </div>
        </div>

        {/* Meta Info */}
        <div className="flex items-center gap-3 text-xs text-slate-500 dark:text-slate-400 mb-3">
          {episode.episode_no && (
            <span className="px-2 py-0.5 bg-blue-100 dark:bg-blue-900/30 text-blue-700 dark:text-blue-300 rounded-md font-medium">
              {episode.episode_no}
            </span>
          )}
          <span>{formatDate(episode.published_date)}</span>
          {episode.duration > 0 && (
            <>
              <span>•</span>
              <span>{formatDuration(episode.duration)}</span>
            </>
          )}
          {episode.enclosure_length > 0 && (
            <>
              <span>•</span>
              <span>{formatFileSize(episode.enclosure_length)}</span>
            </>
          )}
        </div>

        {/* Show Notes - Collapsible */}
        {episode.show_notes && (
          <div
            className={`text-sm text-slate-600 dark:text-slate-400 transition-all duration-300 ${
              isExpanded ? 'max-h-96 overflow-y-auto' : 'max-h-12 overflow-hidden'
            }`}
            onMouseEnter={() => !isExpanded && setIsExpanded(true)}
            onMouseLeave={() => isExpanded && setIsExpanded(false)}
          >
            <div className="relative">
              <RichText
                html={episode.show_notes}
                className="prose prose-sm dark:prose-invert max-w-none"
              />
              {!isExpanded && (
                <div className="absolute bottom-0 left-0 right-0 h-8 bg-gradient-to-t from-white dark:from-slate-800 to-transparent" />
              )}
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
