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
          onClick={() => isSelected ? onRemove(podcast.id) : onAdd(podcast.id)}
          disabled={isSelected}
          className="w-7 h-7 flex items-center justify-center text-sm bg-slate-100 dark:bg-slate-700 text-slate-600 dark:text-slate-300 rounded hover:bg-slate-200 dark:hover:bg-slate-600 disabled:bg-slate-50 dark:disabled:bg-slate-800 disabled:text-slate-300 dark:disabled:text-slate-600 disabled:cursor-not-allowed border border-slate-200 dark:border-slate-600 flex-shrink-0"
          title={isSelected ? '已添加' : '添加'}
        >
          {isSelected ? '✓' : '>'}
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
