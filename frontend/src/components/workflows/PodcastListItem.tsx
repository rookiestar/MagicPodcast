import React, { memo } from 'react'
import PodcastCover from '@/components/podcasts/PodcastCover'
import type { Podcast } from '@/types'

interface PodcastListItemProps {
  podcast: Podcast
  isSelected: boolean
  onAdd: (id: number) => void
  onRemove: (id: number) => void
  index: number
}

export const PodcastListItem = memo<PodcastListItemProps>(
  ({ podcast, isSelected, onAdd, onRemove, index }) => {
    return (
      <div className="flex items-center justify-between p-2 hover:bg-slate-100 dark:hover:bg-slate-700 rounded cursor-pointer group mb-1">
        <div className="flex items-center gap-3 flex-1 min-w-0">
          {/* 封面 */}
          <div className="w-10 h-10 flex-shrink-0 rounded overflow-hidden shadow-sm">
            <PodcastCover
              coverUrl={podcast.cover_url}
              title={podcast.title}
              index={index}
              priority="low"
            />
          </div>

          {/* 文本 */}
          <div className="flex-1 min-w-0 pr-2">
            <div className="text-xs font-medium text-slate-900 dark:text-slate-50 truncate">
              {podcast.title}
            </div>
            {podcast.author && (
              <div className="text-xs text-slate-500 dark:text-slate-400 truncate">
                {podcast.author}
              </div>
            )}
          </div>
        </div>

        <button
          onClick={(e) => {
            e.stopPropagation()
            isSelected ? onRemove(podcast.id) : onAdd(podcast.id)
          }}
          className={`
            w-7 h-7 flex items-center justify-center text-sm rounded flex-shrink-0 border transition-all
            ${isSelected
              ? 'bg-red-100 dark:bg-red-900/30 text-red-600 dark:text-red-400 border-red-200 dark:border-red-800 hover:bg-red-200 dark:hover:bg-red-900/50'
              : 'bg-blue-100 dark:bg-blue-900/30 text-blue-600 dark:text-blue-400 border-blue-200 dark:border-blue-800 hover:bg-blue-200 dark:hover:bg-blue-900/50'
            }
          `}
          title={isSelected ? '移除' : '添加'}
        >
          {isSelected ? '✕' : '✓'}
        </button>
      </div>
    )
  },
  (prevProps, nextProps) => {
    // 自定义比较函数：仅在关键属性变化时重渲染
    return (
      prevProps.podcast.id === nextProps.podcast.id &&
      prevProps.isSelected === nextProps.isSelected &&
      prevProps.podcast.title === nextProps.podcast.title
    )
  }
)

PodcastListItem.displayName = 'PodcastListItem'
