'use client'

import { useEffect, useState, useRef, useCallback } from 'react'
import Link from 'next/link'
import { podcastApi, tagApi } from '@/lib/api'
import { stripHtml } from '@/lib/textUtils'
import { getRelativeTime, isRecentlyUpdated } from '@/lib/timeUtils'
import type { Podcast, Tag } from '@/types'
import SearchSidebar from '@/components/SearchSidebar'
import PodcastCover from '@/components/podcasts/PodcastCover'

const PAGE_SIZE = 15 // 默认每页15个（5行×3列）

type SortByType = 'recent_update' | 'newest_added' | 'episode_count' | 'title'

export default function PodcastsPage() {
  const [podcasts, setPodcasts] = useState<Podcast[]>([])
  const [tags, setTags] = useState<Tag[]>([])
  const [loading, setLoading] = useState(true)
  const [loadingMore, setLoadingMore] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [selectedTagIds, setSelectedTagIds] = useState<number[]>([])
  const [showAllTags, setShowAllTags] = useState(false)
  const [searchOpen, setSearchOpen] = useState(false)
  const [sortBy, setSortBy] = useState<SortByType>('recent_update')

  // 添加一个key来强制刷新列表渲染
  const [listKey, setListKey] = useState(0)

  // 分页状态
  const [page, setPage] = useState(1)
  const [hasMore, setHasMore] = useState(false)
  const [totalCount, setTotalCount] = useState(0)

  // 用于无限滚动的 ref
  const observerTarget = useRef<HTMLDivElement>(null)

  // 数据获取函数
  const fetchPodcasts = async (tagIds: number[] = [], pageNum: number = 1, currentSortBy: SortByType = sortBy) => {
    try {
      if (pageNum === 1) {
        setLoading(true)
      } else {
        setLoadingMore(true)
      }
      setError(null)

      // 构建查询参数
      const params: any = {
        page: pageNum,
        page_size: PAGE_SIZE,
        sort_by: currentSortBy
      }

      // 如果选择了标签，传递所有选中的标签ID（AND逻辑）
      if (tagIds.length > 0) {
        params.tag_id = tagIds
      }

      console.log('[fetchPodcasts] Fetching with params:', params)
      const result = await podcastApi.list(params)
      console.log('[fetchPodcasts] Got result:', result.data.length, 'podcasts')

      // 打印前3个节目的标题和数量，验证排序
      if (result.data && result.data.length > 0) {
        console.log('[fetchPodcasts] First 3 podcasts:')
        result.data.slice(0, 3).forEach((p, i) => {
          console.log(`  ${i+1}. ${p.title} - ${p.episode_count}集`)
        })
      }

      if (pageNum === 1) {
        // 强制设置新数组，触发重新渲染
        setPodcasts([...result.data])
        console.log('[fetchPodcasts] Updated state with new data')
      } else {
        setPodcasts(prev => [...prev, ...result.data])
      }

      setHasMore(result.pagination.page < result.pagination.total_pages)
      setTotalCount(result.pagination.total)
      setPage(pageNum)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unknown error')
    } finally {
      setLoading(false)
      setLoadingMore(false)
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

  // 初始化时从 URL 读取排序并加载数据
  useEffect(() => {
    // 从 URL 读取排序方式
    const params = new URLSearchParams(window.location.search)
    const sortFromUrl = (params.get('sort_by') as SortByType) || 'recent_update'
    console.log('[Init] Loading with sortBy from URL:', sortFromUrl)
    setSortBy(sortFromUrl)
    fetchPodcasts([], 1, sortFromUrl)
    fetchTags()
  }, []) // 只在挂载时执行一次

  // 监听 sortBy 变化，重新获取数据
  useEffect(() => {
    if (sortBy) {
      console.log('[sortBy changed] Refetching with sortBy:', sortBy)
      setPodcasts([]) // 清空现有数据
      setPage(1)
      fetchPodcasts(selectedTagIds, 1, sortBy)
    }
  }, [sortBy]) // 当 sortBy 变化时重新执行

  // 加载更多（用于无限滚动）
  const loadMore = useCallback(() => {
    if (!loadingMore && !loading && hasMore) {
      fetchPodcasts(selectedTagIds, page + 1, sortBy)
    }
  }, [loadingMore, loading, hasMore, page, selectedTagIds, sortBy])

  // 设置无限滚动观察器
  useEffect(() => {
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries[0].isIntersecting && hasMore && !loadingMore) {
          loadMore()
        }
      },
      { rootMargin: '200px' } // 提前200px开始加载
    )

    const currentTarget = observerTarget.current
    if (currentTarget) {
      observer.observe(currentTarget)
    }

    return () => {
      if (currentTarget) {
        observer.unobserve(currentTarget)
      }
    }
  }, [hasMore, loadingMore, loadMore])

  const handleTagToggle = (tagId: number | null) => {
    if (tagId === null) {
      // 点击"全部"，清除所有选择，重新加载第一页
      setSelectedTagIds([])
      setPage(1)
      fetchPodcasts([], 1, sortBy)
    } else {
      // 切换标签选择状态
      if (selectedTagIds.includes(tagId)) {
        // 取消选择
        const newSelected = selectedTagIds.filter(id => id !== tagId)
        setSelectedTagIds(newSelected)
        setPage(1)
        fetchPodcasts(newSelected, 1, sortBy)
      } else {
        // 添加选择
        const newSelected = [...selectedTagIds, tagId]
        setSelectedTagIds(newSelected)
        setPage(1)
        fetchPodcasts(newSelected, 1, sortBy)
      }
    }
  }

  // 处理排序方式变更
  const handleSortChange = (newSortBy: SortByType) => {
    console.log('[handleSortChange] Changing sort from', sortBy, 'to', newSortBy)

    // 更新 URL 参数
    const url = new URL(window.location.href)
    url.searchParams.set('sort_by', newSortBy)
    window.history.replaceState({}, '', url.toString())

    // 更新状态（useEffect 会自动重新获取数据）
    setSortBy(newSortBy)
  }

  // 默认显示的标签数量（不含"全部"）
  const DEFAULT_TAG_COUNT = 8
  const displayTags = showAllTags ? tags : tags.slice(0, DEFAULT_TAG_COUNT)
  const hasMoreTags = tags.length > DEFAULT_TAG_COUNT

  return (
    <main className="min-h-screen bg-slate-50">
      <div className="container mx-auto px-4 py-8">
        {/* Header */}
        <div className="mb-8">
          <div className="mb-8">
            <div className="flex items-center justify-between mb-6">
              {/* 返回首页按钮 - 固定宽度与排序框一致 */}
              <Link
                href={`/?sort_by=${sortBy}`}
                className="w-36 h-11 px-4 bg-white text-slate-800 font-medium rounded-xl border border-slate-300 hover:bg-slate-50 hover:border-slate-400 transition-colors flex items-center justify-center gap-2"
              >
                <span>←</span>
                <span>返回首页</span>
              </Link>

              {/* 右侧按钮组 */}
              <div className="flex items-center gap-3">
                {/* 搜索按钮 - 与排序按钮大小完全一致 */}
                <button
                  onClick={() => setSearchOpen(true)}
                  className="w-36 h-11 border border-slate-300 rounded-xl bg-white text-slate-400 text-sm font-medium hover:bg-slate-50 hover:border-slate-400 transition-colors relative"
                >
                  <span className="absolute left-0 top-1/2 -translate-y-1/2 pl-3 text-slate-400 text-lg pointer-events-none">🔍</span>
                </button>

                {/* 排序选择器 - 白底边框样式 + icon */}
                <div className="relative w-36">
                  <span className="absolute left-0 top-1/2 -translate-y-1/2 pl-3 text-slate-400 text-lg z-10 pointer-events-none">🔽</span>
                  <select
                    value={sortBy}
                    onChange={(e) => handleSortChange(e.target.value as SortByType)}
                    className="w-full h-11 pl-10 pr-4 py-2.5 border border-slate-300 rounded-xl bg-white text-slate-400 text-sm text-center focus:ring-2 focus:ring-violet-500 focus:border-transparent transition-colors appearance-none cursor-pointer"
                  >
                    <option value="recent_update">最近更新</option>
                    <option value="newest_added">最新添加</option>
                    <option value="episode_count">单集数量</option>
                    <option value="title">名称</option>
                  </select>
                </div>
              </div>
            </div>

            {/* 标题和描述 - 优化字号 */}
            <div className="mb-4">
              <h1 className="text-4xl md:text-5xl font-semibold text-slate-800 mb-2">
                我的订阅
              </h1>
              <p className="text-base text-slate-600 max-w-2xl">
                管理你的播客节目
              </p>
            </div>
          </div>
        </div>

        {/* Tag Filter */}
        {tags.length > 0 && (
          <div className="mb-6">
            {/* 标签按钮 */}
            <div className="flex flex-wrap gap-2">
              {/* 全部按钮 */}
              <button
                onClick={() => handleTagToggle(null)}
                className={`px-3 py-1.5 rounded-lg text-sm transition-colors ${
                  selectedTagIds.length === 0
                    ? 'bg-slate-800 text-white'
                    : 'bg-slate-100 text-slate-600 hover:bg-slate-200'
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
                    className={`px-3 py-1.5 rounded-lg text-sm transition-colors flex items-center gap-1.5 ${
                      isSelected
                        ? 'bg-slate-800 text-white'
                        : 'bg-slate-100 text-slate-600 hover:bg-slate-200'
                    }`}
                    title={tag.name}
                  >
                    <span
                      className="w-1.5 h-1.5 rounded-full flex-shrink-0"
                      style={{
                        backgroundColor: isSelected ? '#ffffff' : tag.color
                      }}
                    />
                    <span className="max-w-[100px] truncate">{tag.name}</span>
                  </button>
                )
              })}

              {/* 展开/折叠按钮 */}
              {hasMoreTags && (
                <button
                  onClick={() => setShowAllTags(!showAllTags)}
                  className="px-3 py-1.5 rounded-lg text-sm text-slate-500 hover:text-slate-700 hover:bg-slate-50 transition-colors"
                >
                  {showAllTags ? '收起' : `+${tags.length - DEFAULT_TAG_COUNT}`}
                </button>
              )}
            </div>
          </div>
        )}

        {/* Loading State */}
        {loading && (
          <div className="text-center py-12">
            <div className="inline-block animate-spin rounded-full h-12 w-12 border-b-2 border-blue-600"></div>
            <p className="mt-4 text-slate-600">加载中...</p>
          </div>
        )}

        {/* Error State */}
        {error && (
          <div className="bg-red-50 border border-red-200 rounded-xl p-6 mb-6">
            <h3 className="text-red-800 font-semibold mb-2">加载失败</h3>
            <p className="text-red-600 mb-4">{error}</p>
            <button
              onClick={() => fetchPodcasts(selectedTagIds, 1)}
              className="px-4 py-2 bg-red-600 text-white rounded hover:bg-red-700"
            >
              重试
            </button>
          </div>
        )}

        {/* Podcasts List */}
        {!loading && !error && (
          <>
            <div className="mb-6 text-slate-600">
              共 {totalCount} 个节目
            </div>

            <div className="grid grid-cols-5 gap-6">
              {podcasts.map((podcast, index) => (
                <PodcastCard
                  key={podcast.id}
                  podcast={podcast}
                  sortBy={sortBy}
                  index={index}
                  priority={index < 6 ? 'high' : index < 15 ? 'medium' : 'low'}
                />
              ))}
            </div>

            {/* Loading More Indicator */}
            {loadingMore && (
              <div className="text-center py-8">
                <div className="inline-block animate-spin rounded-full h-8 w-8 border-b-2 border-blue-600"></div>
                <p className="mt-2 text-sm text-slate-600">加载更多...</p>
              </div>
            )}

            {/* Intersection Observer Target */}
            <div ref={observerTarget} className="h-10" />

            {/* No More Data Indicator */}
            {!hasMore && podcasts.length > 0 && (
              <div className="text-center py-8 text-slate-500">
                已经到底了
              </div>
            )}
          </>
        )}
      </div>

      {/* 搜索侧边栏 */}
      <SearchSidebar isOpen={searchOpen} onClose={() => setSearchOpen(false)} />
    </main>
  )
}

function PodcastCard({
  podcast,
  sortBy,
  index = 0,
  priority = 'medium'
}: {
  podcast: Podcast
  sortBy: string
  index?: number
  priority?: 'high' | 'medium' | 'low'
}) {
  // 最多显示3个标签
  const displayTags = podcast.tags?.slice(0, 3) || []
  const remainingTags = (podcast.tags?.length || 0) - 3

  // 判断是否最近更新（7天内有新内容）
  const recentlyUpdated = isRecentlyUpdated(podcast.newest_episode_date, 7)
  const relativeTime = getRelativeTime(podcast.newest_episode_date)

  return (
    <Link href={`/podcasts/${podcast.id}${sortBy ? `?sort_by=${sortBy}` : ''}`}>
      <div className="bg-white rounded-xl shadow-md hover:shadow-lg transition-shadow overflow-hidden cursor-pointer h-full flex flex-col">
        {/* Cover Image - 使用 PodcastCover 组件 */}
        <div className="relative">
          <PodcastCover
            coverUrl={podcast.cover_url}
            title={podcast.title}
            index={index}
            priority={priority}
          />

          {/* 新更新标识 - 右下角 */}
          {recentlyUpdated && (
            <div className="absolute bottom-0 right-0 m-2 z-30">
              <span className="inline-flex items-center gap-1 px-2 py-1 text-xs rounded-full bg-white text-slate-800">
                <span className="w-1.5 h-1.5 rounded-full bg-green-600" />
                新更新
              </span>
            </div>
          )}
        </div>

        {/* Content */}
        <div className="p-4 flex-1 flex flex-col">
          <h3 className="text-lg font-semibold text-slate-900 mb-2 line-clamp-2">
            {podcast.title}
          </h3>
          <p className="text-sm text-slate-600 mb-2">
            {podcast.author}
          </p>
          <p className="text-sm text-slate-400 line-clamp-2">
            {stripHtml(podcast.description, 80)}
          </p>

          {/* Tags - 新增 */}
          {displayTags.length > 0 && (
            <div className="mt-3 flex flex-wrap gap-1.5">
              {displayTags.map((tag) => (
                <span
                  key={tag.id}
                  className="inline-flex items-center gap-1 px-2 py-0.5 text-xs rounded-full bg-slate-100 group relative"
                >
                  <span
                    className="w-1.5 h-1.5 rounded-full flex-shrink-0"
                    style={{ backgroundColor: tag.color }}
                  />
                  <span className="max-w-[80px] truncate" title={tag.name}>
                    {tag.name}
                  </span>
                  {/* 自定义 Tooltip */}
                  <div className="absolute bottom-full left-1/2 -translate-x-1/2 mb-2 px-2 py-1 bg-slate-900bg-slate-100 text-white text-xs rounded whitespace-nowrap opacity-0 group-hover:opacity-100 transition-opacity pointer-events-none z-10">
                    {tag.name}
                  </div>
                </span>
              ))}
              {remainingTags > 0 && (
                <span className="inline-flex items-center px-2 py-0.5 text-xs rounded-full bg-slate-100 text-slate-500">
                  +{remainingTags}
                </span>
              )}
            </div>
          )}

          {/* Stats - 吸底 */}
          <div className="mt-auto pt-4">
            <div className="flex items-center justify-between text-sm text-slate-500">
              <span>{podcast.episode_count} 集</span>
              <span className="text-xs">{relativeTime}</span>
            </div>
          </div>
        </div>
      </div>
    </Link>
  )
}
