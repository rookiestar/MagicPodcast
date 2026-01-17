'use client'

import { useState, useEffect } from 'react'
import { Tag } from '@/types'
import { tagApi } from '@/lib/api'
import TagBadge from './TagBadge'

interface TagSelectorProps {
  selectedTags: Tag[]
  onTagAdd: (tag: Tag) => void
  onTagRemove: (tagId: number) => void
  excludedTagIds?: number[]
}

export default function TagSelector({
  selectedTags,
  onTagAdd,
  onTagRemove,
  excludedTagIds = []
}: TagSelectorProps) {
  const [availableTags, setAvailableTags] = useState<Tag[]>([])
  const [isOpen, setIsOpen] = useState(false)
  const [loading, setLoading] = useState(false)

  // 加载所有可用标签
  useEffect(() => {
    const fetchTags = async () => {
      try {
        setLoading(true)
        const tags = await tagApi.list()
        // 过滤掉已选择的标签和排除的标签
        const selectedIds = selectedTags.map(t => t.id)
        const filtered = tags.filter(
          t => !selectedIds.includes(t.id) && !excludedTagIds.includes(t.id)
        )
        setAvailableTags(filtered)
      } catch (error) {
        console.error('Failed to fetch tags:', error)
      } finally {
        setLoading(false)
      }
    }
    fetchTags()
  }, [selectedTags, excludedTagIds])

  const handleTagClick = (tag: Tag) => {
    onTagAdd(tag)
    setIsOpen(false)
  }

  return (
    <div className="relative">
      {/* 已选择的标签 */}
      {selectedTags.length > 0 && (
        <div className="flex flex-wrap gap-2 mb-3">
          {selectedTags.map(tag => (
            <TagBadge
              key={tag.id}
              tag={tag}
              onRemove={onTagRemove}
              removable
            />
          ))}
        </div>
      )}

      {/* 添加标签按钮 */}
      <div className="relative">
        <button
          onClick={() => setIsOpen(!isOpen)}
          className="
            inline-flex items-center gap-2 px-4 py-2
            bg-white dark:bg-slate-800
            border-2 border-dashed border-slate-300 dark:border-slate-600
            rounded-lg text-slate-600 dark:text-slate-400
            hover:border-blue-500 hover:text-blue-600
            dark:hover:border-blue-400 dark:hover:text-blue-300
            transition-colors duration-200
            focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2
          "
        >
          <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 4v16m8-8H4" />
          </svg>
          <span>添加标签</span>
        </button>

        {/* 下拉菜单 */}
        {isOpen && (
          <>
            {/* 点击外部关闭 */}
            <div
              className="fixed inset-0 z-10"
              onClick={() => setIsOpen(false)}
            />

            {/* 菜单内容 */}
            <div className="absolute z-20 mt-2 w-64 max-h-80 overflow-auto rounded-lg shadow-lg bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700">
              {loading ? (
                <div className="px-4 py-8 text-center text-slate-500 dark:text-slate-400">
                  <div className="inline-block animate-spin rounded-full h-6 w-6 border-b-2 border-blue-600"></div>
                  <p className="mt-2 text-sm">加载中...</p>
                </div>
              ) : availableTags.length === 0 ? (
                <div className="px-4 py-8 text-center text-slate-500 dark:text-slate-400">
                  <p className="text-sm">暂无可用标签</p>
                </div>
              ) : (
                <div className="py-1">
                  {availableTags.map(tag => (
                    <button
                      key={tag.id}
                      onClick={() => handleTagClick(tag)}
                      className="
                        w-full px-4 py-3 text-left
                        hover:bg-slate-100 dark:hover:bg-slate-700
                        transition-colors duration-150
                        focus:outline-none focus:bg-slate-100 dark:focus:bg-slate-700
                      "
                    >
                      <div className="flex items-center gap-3">
                        <span
                          className="w-3 h-3 rounded-full"
                          style={{ backgroundColor: tag.color }}
                        />
                        <div className="flex-1 min-w-0">
                          <p className="text-sm font-medium text-slate-900 dark:text-slate-100 truncate">
                            {tag.name}
                          </p>
                        </div>
                      </div>
                    </button>
                  ))}
                </div>
              )}
            </div>
          </>
        )}
      </div>
    </div>
  )
}
