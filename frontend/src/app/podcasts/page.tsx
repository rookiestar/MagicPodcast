'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import { podcastApi, tagApi } from '@/lib/api'
import type { Podcast, Tag } from '@/types'

export default function PodcastsPage() {
  const [podcasts, setPodcasts] = useState<Podcast[]>([])
  const [tags, setTags] = useState<Tag[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [selectedTagIds, setSelectedTagIds] = useState<number[]>([])
  const [showAllTags, setShowAllTags] = useState(false)

  useEffect(() => {
    fetchPodcasts()
    fetchTags()
  }, [])

  const fetchPodcasts = async (tagIds: number[] = []) => {
    try {
      setLoading(true)
      setError(null)

      // 如果选择了标签，传递所有选中的标签ID（AND逻辑）
      const params = tagIds.length > 0 ? { tag_id: tagIds } : undefined
      const data = await podcastApi.list(params)
      setPodcasts(data)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unknown error')
    } finally {
      setLoading(false)
    }
  }

  const fetchTags = async () => {
    try {
      const data = await tagApi.list()
      setTags(data)
    } catch (err) {
      console.error('Failed to fetch tags:', err)
    }
  }

  const handleTagToggle = (tagId: number | null) => {
    if (tagId === null) {
      // 点击"全部"，清除所有选择
      setSelectedTagIds([])
      fetchPodcasts([])
    } else {
      // 切换标签选择状态
      if (selectedTagIds.includes(tagId)) {
        // 取消选择
        const newSelected = selectedTagIds.filter(id => id !== tagId)
        setSelectedTagIds(newSelected)
        fetchPodcasts(newSelected)
      } else {
        // 添加选择
        const newSelected = [...selectedTagIds, tagId]
        setSelectedTagIds(newSelected)
        fetchPodcasts(newSelected)
      }
    }
  }

  // 默认显示的标签数量（不含"全部"）
  const DEFAULT_TAG_COUNT = 8
  const displayTags = showAllTags ? tags : tags.slice(0, DEFAULT_TAG_COUNT)
  const hasMoreTags = tags.length > DEFAULT_TAG_COUNT

  return (
    <main className="min-h-screen bg-slate-50 dark:bg-slate-900">
      <div className="container mx-auto px-4 py-8">
        {/* Header */}
        <div className="mb-8">
          <Link
            href="/"
            className="text-blue-600 hover:text-blue-700 mb-4 inline-block"
          >
            ← 返回首页
          </Link>
          <h1 className="text-4xl font-bold text-slate-900 dark:text-slate-50 mb-2">
            我的订阅
          </h1>
          <p className="text-slate-600 dark:text-slate-400">
            管理你的播客节目
          </p>
        </div>

        {/* Tag Filter */}
        {tags.length > 0 && (
          <div className="mb-6">
            <div className="flex flex-wrap gap-2 items-center">
              <span className="text-sm text-slate-600 dark:text-slate-400">标签筛选:</span>

              {/* 全部按钮 */}
              <button
                onClick={() => handleTagToggle(null)}
                className={`px-3 py-1 rounded-full text-sm transition-colors ${
                  selectedTagIds.length === 0
                    ? 'bg-blue-600 text-white'
                    : 'bg-slate-200 dark:bg-slate-700 text-slate-700 dark:text-slate-300 hover:bg-slate-300 dark:hover:bg-slate-600'
                }`}
              >
                全部
              </button>

              {/* 标签按钮 */}
              {displayTags.map((tag) => {
                const isSelected = selectedTagIds.includes(tag.id)
                return (
                  <button
                    key={tag.id}
                    onClick={() => handleTagToggle(tag.id)}
                    className={`px-3 py-1 rounded-full text-sm transition-colors flex items-center gap-2 group relative ${
                      isSelected
                        ? 'bg-blue-600 text-white'
                        : 'bg-slate-200 dark:bg-slate-700 text-slate-700 dark:text-slate-300 hover:bg-slate-300 dark:hover:bg-slate-600'
                    }`}
                  >
                    <span
                      className="w-2 h-2 rounded-full flex-shrink-0"
                      style={{ backgroundColor: tag.color }}
                    />
                    <span className="max-w-[100px] truncate" title={tag.name}>{tag.name}</span>
                    {/* Tooltip for tag name */}
                    <div className="absolute bottom-full left-1/2 -translate-x-1/2 mb-2 px-2 py-1 bg-slate-900 dark:bg-slate-100 text-white dark:text-slate-900 text-xs rounded whitespace-nowrap opacity-0 group-hover:opacity-100 transition-opacity pointer-events-none z-20">
                      {tag.name}
                    </div>
                  </button>
                )
              })}

              {/* 展开/折叠按钮 */}
              {hasMoreTags && (
                <button
                  onClick={() => setShowAllTags(!showAllTags)}
                  className="px-3 py-1 rounded-full text-sm transition-colors text-blue-600 dark:text-blue-400 hover:bg-blue-50 dark:hover:bg-blue-900/20"
                >
                  {showAllTags ? '收起' : `展开 (+${tags.length - DEFAULT_TAG_COUNT})`}
                </button>
              )}
            </div>

            {/* 已选择的标签提示 */}
            {selectedTagIds.length > 0 && (
              <div className="mt-2 flex items-center gap-2 text-sm text-slate-600 dark:text-slate-400">
                <span>已选择 {selectedTagIds.length} 个标签</span>
                <button
                  onClick={() => handleTagToggle(null)}
                  className="text-blue-600 dark:text-blue-400 hover:underline"
                >
                  清除筛选
                </button>
              </div>
            )}
          </div>
        )}

        {/* Loading State */}
        {loading && (
          <div className="text-center py-12">
            <div className="inline-block animate-spin rounded-full h-12 w-12 border-b-2 border-blue-600"></div>
            <p className="mt-4 text-slate-600 dark:text-slate-400">加载中...</p>
          </div>
        )}

        {/* Error State */}
        {error && (
          <div className="bg-red-50 border border-red-200 rounded-lg p-6 mb-6">
            <h3 className="text-red-800 font-semibold mb-2">加载失败</h3>
            <p className="text-red-600 mb-4">{error}</p>
            <button
              onClick={fetchPodcasts}
              className="px-4 py-2 bg-red-600 text-white rounded hover:bg-red-700"
            >
              重试
            </button>
          </div>
        )}

        {/* Podcasts List */}
        {!loading && !error && (
          <>
            <div className="mb-6 text-slate-600 dark:text-slate-400">
              共 {podcasts.length} 个节目
            </div>

            <div className="grid md:grid-cols-2 lg:grid-cols-3 gap-6">
              {podcasts.map((podcast) => (
                <PodcastCard key={podcast.id} podcast={podcast} />
              ))}
            </div>
          </>
        )}
      </div>
    </main>
  )
}

function PodcastCard({ podcast }: { podcast: Podcast }) {
  // 最多显示3个标签
  const displayTags = podcast.tags?.slice(0, 3) || []
  const remainingTags = (podcast.tags?.length || 0) - 3

  return (
    <Link href={`/podcasts/${podcast.id}`}>
      <div className="bg-white dark:bg-slate-800 rounded-lg shadow-md hover:shadow-lg transition-shadow overflow-hidden cursor-pointer h-full">
        {/* Cover Image */}
        <div className="aspect-square bg-slate-200 dark:bg-slate-700 relative">
          {podcast.cover_url ? (
            <img
              src={podcast.cover_url}
              alt={podcast.title}
              className="w-full h-full object-cover"
            />
          ) : (
            <div className="w-full h-full flex items-center justify-center text-4xl">
              🎧
            </div>
          )}
        </div>

        {/* Content */}
        <div className="p-4">
          <h3 className="text-lg font-semibold text-slate-900 dark:text-slate-50 mb-2 line-clamp-2">
            {podcast.title}
          </h3>
          <p className="text-sm text-slate-600 dark:text-slate-400 mb-2">
            {podcast.author}
          </p>
          <p className="text-sm text-slate-500 dark:text-slate-500 line-clamp-2">
            {podcast.description}
          </p>

          {/* Tags - 新增 */}
          {displayTags.length > 0 && (
            <div className="mt-3 flex flex-wrap gap-1.5">
              {displayTags.map((tag) => (
                <span
                  key={tag.id}
                  className="inline-flex items-center gap-1 px-2 py-0.5 text-xs rounded-full bg-slate-100 dark:bg-slate-700 text-slate-700 dark:text-slate-300 group relative"
                >
                  <span
                    className="w-1.5 h-1.5 rounded-full flex-shrink-0"
                    style={{ backgroundColor: tag.color }}
                  />
                  <span className="max-w-[80px] truncate" title={tag.name}>
                    {tag.name}
                  </span>
                  {/* 自定义 Tooltip */}
                  <div className="absolute bottom-full left-1/2 -translate-x-1/2 mb-2 px-2 py-1 bg-slate-900 dark:bg-slate-100 text-white dark:text-slate-900 text-xs rounded whitespace-nowrap opacity-0 group-hover:opacity-100 transition-opacity pointer-events-none z-10">
                    {tag.name}
                  </div>
                </span>
              ))}
              {remainingTags > 0 && (
                <span className="inline-flex items-center px-2 py-0.5 text-xs rounded-full bg-slate-100 dark:bg-slate-700 text-slate-500 dark:text-slate-400">
                  +{remainingTags}
                </span>
              )}
            </div>
          )}

          {/* Stats */}
          <div className="mt-4 flex items-center justify-between text-sm text-slate-500 dark:text-slate-400">
            <span>{podcast.episode_count} 集</span>
            <span>
              {new Date(podcast.newest_episode_date).toLocaleDateString()}
            </span>
          </div>
        </div>
      </div>
    </Link>
  )
}
