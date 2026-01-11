'use client'

import { useState, useEffect, useRef } from 'react'
import Link from 'next/link'
import { searchApi } from '@/lib/api'
import type { PodcastSearchResult, EpisodeSearchResult } from '@/types'

interface SearchSidebarProps {
  isOpen: boolean
  onClose: () => void
}

// 搜索历史管理
const MAX_SEARCH_HISTORY = 6
const STORAGE_KEY = 'podcast_search_history'

const getSearchHistory = (): string[] => {
  if (typeof window === 'undefined') return []
  try {
    const stored = localStorage.getItem(STORAGE_KEY)
    return stored ? JSON.parse(stored) : []
  } catch {
    return []
  }
}

const saveSearchHistory = (history: string[]) => {
  if (typeof window === 'undefined') return
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(history))
  } catch (error) {
    console.error('Failed to save search history:', error)
  }
}

const addToSearchHistory = (query: string) => {
  if (!query.trim()) return
  const history = getSearchHistory()
  // 移除重复项
  const filtered = history.filter(q => q !== query)
  // 添加到开头
  const newHistory = [query, ...filtered].slice(0, MAX_SEARCH_HISTORY)
  saveSearchHistory(newHistory)
  return newHistory
}

const clearSearchHistory = () => {
  saveSearchHistory([])
}

export default function SearchSidebar({ isOpen, onClose }: SearchSidebarProps) {
  const [query, setQuery] = useState('')
  const [searchType, setSearchType] = useState<'all' | 'podcasts' | 'episodes'>('all')
  const [results, setResults] = useState<{
    podcasts: PodcastSearchResult[]
    episodes: EpisodeSearchResult[]
  }>({ podcasts: [], episodes: [] })
  const [loading, setLoading] = useState(false)
  const searchInputRef = useRef<HTMLInputElement>(null)

  // 存储完整的搜索结果（用于切换筛选器时显示）
  const [allResults, setAllResults] = useState<{
    podcasts: PodcastSearchResult[]
    episodes: EpisodeSearchResult[]
  }>({ podcasts: [], episodes: [] })

  // 搜索历史
  const [searchHistory, setSearchHistory] = useState<string[]>([])

  // 自动聚焦
  useEffect(() => {
    if (isOpen && searchInputRef.current) {
      searchInputRef.current.focus()
    }
  }, [isOpen])

  // 重置状态当关闭时
  useEffect(() => {
    if (!isOpen) {
      setQuery('')
      setResults({ podcasts: [], episodes: [] })
      setAllResults({ podcasts: [], episodes: [] })
      setSearchType('all')
    }
  }, [isOpen])

  // 加载搜索历史
  useEffect(() => {
    setSearchHistory(getSearchHistory())
  }, [isOpen])

  // 防抖搜索 - 只在 query 变化时触发
  useEffect(() => {
    const timer = setTimeout(() => {
      if (query.trim().length >= 2) {
        performSearch()
      } else {
        setResults({ podcasts: [], episodes: [] })
        setAllResults({ podcasts: [], episodes: [] })
      }
    }, 500)

    return () => clearTimeout(timer)
  }, [query]) // 移除 searchType 依赖

  // 当 searchType 变化时，更新显示结果（不重新搜索）
  useEffect(() => {
    if (allResults.podcasts.length > 0 || allResults.episodes.length > 0) {
      if (searchType === 'all') {
        setResults(allResults)
      } else if (searchType === 'podcasts') {
        setResults({ podcasts: allResults.podcasts, episodes: [] })
      } else if (searchType === 'episodes') {
        setResults({ podcasts: [], episodes: allResults.episodes })
      }
    }
  }, [searchType, allResults])

  const performSearch = async () => {
    setLoading(true)
    try {
      // 始终搜索全部类型，获取完整结果
      const response = await searchApi.search({
        q: query,
        type: 'all', // 始终搜索全部
        page: 1,
        page_size: 100,
        episode_page: 1,
        episode_page_size: 100,
      })
      // 确保 matched_fields 始终被初始化
      const processedData = {
        podcasts: response.data.podcasts.map(p => ({
          ...p,
          matched_fields: p.matched_fields || []
        })),
        episodes: response.data.episodes.map(e => ({
          ...e,
          matched_fields: e.matched_fields || []
        })),
        pagination: response.data.pagination
      }
      // 存储完整结果
      setAllResults(processedData)
      // 根据 searchType 设置显示结果
      if (searchType === 'all') {
        setResults(processedData)
      } else if (searchType === 'podcasts') {
        setResults({ podcasts: processedData.podcasts, episodes: [] })
      } else if (searchType === 'episodes') {
        setResults({ podcasts: [], episodes: processedData.episodes })
      }

      // 添加到搜索历史
      const newHistory = addToSearchHistory(query)
      setSearchHistory(newHistory)
    } catch (error) {
      console.error('Search failed:', error)
      setResults({ podcasts: [], episodes: [] })
      setAllResults({ podcasts: [], episodes: [] })
    } finally {
      setLoading(false)
    }
  }

  // 添加 useCallback 避免依赖问题
  // eslint-disable-next-line react-hooks/exhaustive-deps

  const handleClose = () => {
    onClose()
  }

  const handleHistoryClick = (historyQuery: string) => {
    setQuery(historyQuery)
    // 自动触发搜索（通过 useEffect 的防抖）
  }

  const handleClearHistory = () => {
    clearSearchHistory()
    setSearchHistory([])
  }

  const highlightKeyword = (text: string, keyword: string) => {
    if (!keyword) return text
    const regex = new RegExp(`(${keyword.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')})`, 'gi')
    return text.replace(regex, '<mark class="bg-yellow-200 dark:bg-yellow-800 rounded px-0.5">$1</mark>')
  }

  if (!isOpen) return null

  const hasResults = results.podcasts.length > 0 || results.episodes.length > 0
  const showPodcasts = searchType === 'all' || searchType === 'podcasts'
  const showEpisodes = searchType === 'all' || searchType === 'episodes'
  const showHistory = query.length < 2 && searchHistory.length > 0 && !loading && !hasResults

  return (
    <>
      {/* 遮罩层 */}
      <div
        className="fixed inset-0 bg-black/50 z-40"
        onClick={handleClose}
      />

      {/* 侧边栏 */}
      <div className="fixed right-0 top-0 h-full w-full max-w-2xl bg-white dark:bg-slate-800 shadow-2xl z-50 flex flex-col">
        {/* 头部 */}
        <div className="border-b border-slate-200 dark:border-slate-700 p-4">
          <div className="flex items-center gap-3">
            <div className="flex-1 relative">
              <span className="absolute left-3 top-1/2 -translate-y-1/2 text-slate-400">🔍</span>
              <input
                ref={searchInputRef}
                type="text"
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                placeholder="搜索节目、单集..."
                className="w-full pl-10 pr-4 py-2 bg-slate-100 dark:bg-slate-700 rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500 dark:text-slate-100"
              />
            </div>
            <button
              onClick={handleClose}
              className="p-2 hover:bg-slate-100 dark:hover:bg-slate-700 rounded-lg text-xl"
              title="关闭"
            >
              ✕
            </button>
          </div>

          {/* 搜索类型切换 */}
          <div className="flex gap-2 mt-3">
            <button
              onClick={() => setSearchType('all')}
              className={`px-3 py-1 rounded-full text-sm transition-colors ${
                searchType === 'all'
                  ? 'bg-blue-600 text-white'
                  : 'bg-slate-200 dark:bg-slate-700 text-slate-700 dark:text-slate-300'
              }`}
            >
              全部 {(allResults.podcasts.length + allResults.episodes.length > 0) && `(${allResults.podcasts.length + allResults.episodes.length})`}
            </button>
            <button
              onClick={() => setSearchType('podcasts')}
              className={`px-3 py-1 rounded-full text-sm transition-colors ${
                searchType === 'podcasts'
                  ? 'bg-blue-600 text-white'
                  : 'bg-slate-200 dark:bg-slate-700 text-slate-700 dark:text-slate-300'
              }`}
            >
              节目 {allResults.podcasts.length > 0 && `(${allResults.podcasts.length})`}
            </button>
            <button
              onClick={() => setSearchType('episodes')}
              className={`px-3 py-1 rounded-full text-sm transition-colors ${
                searchType === 'episodes'
                  ? 'bg-blue-600 text-white'
                  : 'bg-slate-200 dark:bg-slate-700 text-slate-700 dark:text-slate-300'
              }`}
            >
              单集 {allResults.episodes.length > 0 && `(${allResults.episodes.length})`}
            </button>
          </div>
        </div>

        {/* 结果区域 */}
        <div className="flex-1 overflow-y-auto">
          {/* 搜索历史 */}
          {showHistory && (
            <div className="p-4">
              <div className="flex items-center justify-between mb-3">
                <h3 className="text-sm font-semibold text-slate-700 dark:text-slate-300">
                  🕐 最近搜索
                </h3>
                <button
                  onClick={handleClearHistory}
                  className="text-xs text-slate-500 hover:text-slate-700 dark:text-slate-400 dark:hover:text-slate-200 transition-colors"
                >
                  清空
                </button>
              </div>
              <div className="space-y-2">
                {searchHistory.map((historyQuery, index) => (
                  <button
                    key={index}
                    onClick={() => handleHistoryClick(historyQuery)}
                    className="w-full text-left px-3 py-2 bg-slate-50 dark:bg-slate-900 hover:bg-slate-100 dark:hover:bg-slate-800 rounded-lg transition-colors text-slate-700 dark:text-slate-300 text-sm"
                  >
                    📌 {historyQuery}
                  </button>
                ))}
              </div>
            </div>
          )}

          {!loading && query.length < 2 && !showHistory && (
            <div className="flex flex-col items-center justify-center h-full text-slate-500 dark:text-slate-400">
              <span className="text-6xl mb-4">🔍</span>
              <p className="text-lg">输入关键词开始搜索</p>
              <p className="text-sm mt-2">支持搜索节目标题、作者、简介和单集内容</p>
            </div>
          )}

          {!loading && query.length >= 2 && !hasResults && (
            <div className="flex flex-col items-center justify-center h-full text-slate-500 dark:text-slate-400">
              <p className="text-lg">未找到相关结果</p>
              <p className="text-sm mt-2">试试其他关键词</p>
            </div>
          )}

          {loading && (
            <div className="flex items-center justify-center py-12">
              <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-blue-600"></div>
            </div>
          )}

          {!loading && hasResults && (
            <div className="p-4">
              {/* 播客结果 */}
              {showPodcasts && results.podcasts.length > 0 && (
                <>
                  {searchType === 'all' && results.episodes.length > 0 && (
                    <h3 className="text-lg font-semibold text-slate-900 dark:text-slate-50 mb-3">
                      📻 节目 ({results.podcasts.length})
                    </h3>
                  )}
                  <div className="space-y-3">
                  {results.podcasts.map((podcast) => (
                    <Link
                      key={podcast.id}
                      href={`/podcasts/${podcast.id}`}
                      onClick={handleClose}
                      className="block p-4 bg-slate-50 dark:bg-slate-900 rounded-lg hover:bg-slate-100 dark:hover:bg-slate-800 transition-colors"
                    >
                      <div className="flex gap-4">
                        <img
                          src={podcast.cover_url || '/placeholder.png'}
                          alt={podcast.title}
                          className="w-20 h-20 rounded-lg object-cover flex-shrink-0"
                        />
                        <div className="flex-1 min-w-0">
                          <h3
                            className="font-semibold text-slate-900 dark:text-slate-50 mb-1"
                            dangerouslySetInnerHTML={{
                              __html: highlightKeyword(podcast.title, query)
                            }}
                          />
                          <p className="text-sm text-slate-600 dark:text-slate-400 mb-2">
                            {podcast.author} · {podcast.episode_count} 集
                          </p>
                          {(() => {
                            // 优先显示 description 的 snippet，其次显示 author，最后显示 title
                            const descField = podcast.matched_fields?.find(f => f.field === 'description')
                            const authorField = podcast.matched_fields?.find(f => f.field === 'author')
                            const titleField = podcast.matched_fields?.find(f => f.field === 'title')
                            const snippetToShow = descField?.snippet || authorField?.snippet || titleField?.snippet || podcast.description

                            return snippetToShow ? (
                              <p
                                className="text-sm text-slate-500 dark:text-slate-500 line-clamp-2"
                                dangerouslySetInnerHTML={{
                                  __html: highlightKeyword(snippetToShow, query)
                                }}
                              />
                            ) : null
                          })()}
                        </div>
                      </div>
                    </Link>
                  ))}
                  </div>
                </>
              )}

              {/* 单集结果 */}
              {showEpisodes && results.episodes.length > 0 && (
                <>
                  {searchType === 'all' && results.podcasts.length > 0 && (
                    <>
                      <div className="my-6 border-t border-slate-200 dark:border-slate-700"></div>
                      <h3 className="text-lg font-semibold text-slate-900 dark:text-slate-50 mb-3">
                        🎧 单集 ({results.episodes.length})
                      </h3>
                    </>
                  )}
                  <div className="space-y-3">
                  {results.episodes.map((episode) => (
                    <Link
                      key={episode.id}
                      href={`/podcasts/${episode.podcast_id}?episode_id=${episode.id}`}
                      onClick={handleClose}
                      className="block p-4 bg-slate-50 dark:bg-slate-900 rounded-lg hover:bg-slate-100 dark:hover:bg-slate-800 transition-colors"
                    >
                      <h3
                        className="font-semibold text-slate-900 dark:text-slate-50 mb-1"
                        dangerouslySetInnerHTML={{
                          __html: highlightKeyword(episode.title, query)
                        }}
                      />
                      <p className="text-sm text-slate-600 dark:text-slate-400 mb-2">
                        {episode.podcast_title}
                        {episode.published_date && ` · ${new Date(episode.published_date).toLocaleDateString()}`}
                      </p>
                      {(() => {
                        // 优先显示 show_notes 的 snippet，如果没有才显示 title 的 snippet
                        const showNotesField = episode.matched_fields?.find(f => f.field === 'show_notes')
                        const titleField = episode.matched_fields?.find(f => f.field === 'title')
                        const snippetToShow = showNotesField?.snippet || titleField?.snippet || episode.show_notes

                        return snippetToShow ? (
                          <p
                            className="text-sm text-slate-500 dark:text-slate-500 line-clamp-2"
                            dangerouslySetInnerHTML={{
                              __html: highlightKeyword(snippetToShow, query)
                            }}
                          />
                        ) : null
                      })()}
                    </Link>
                  ))}
                  </div>
                </>
              )}
            </div>
          )}
        </div>
      </div>
    </>
  )
}
