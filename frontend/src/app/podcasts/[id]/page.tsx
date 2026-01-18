'use client'

import { useEffect, useState, useRef, useCallback } from 'react'
import { useParams, useSearchParams } from 'next/navigation'
import Link from 'next/link'
import { podcastApi, episodeApi } from '@/lib/api'
import type { Podcast, Tag, Episode } from '@/types'
import TagInput from '@/components/tags/TagInput'
import RichText from '@/components/RichText'
import EpisodeCard from '@/components/episodes/EpisodeCard'
import PodcastCover from '@/components/podcasts/PodcastCover'

export default function PodcastDetailPage() {
  const params = useParams()
  const searchParams = useSearchParams()
  const id = parseInt(params.id as string)
  const targetEpisodeId = searchParams.get('episode_id') // 获取目标单集 ID
  const sortBy = searchParams.get('sort_by') || '' // 获取排序方式
  const tagIds = searchParams.get('tag_ids') // 获取标签筛选（逗号分隔）
  const episodeListRef = useRef<HTMLDivElement>(null)

  // 构建返回 URL 的查询参数
  const buildBackUrl = () => {
    const params = new URLSearchParams()
    if (sortBy) {
      params.append('sort_by', sortBy)
    }
    if (tagIds) {
      // tag_ids 是逗号分隔的字符串，需要转换为多个 tag_id 参数
      tagIds.split(',').forEach(id => {
        params.append('tag_id', id)
      })
    }
    const queryString = params.toString()
    return `/podcasts${queryString ? `?${queryString}` : ''}`
  }

  const [podcast, setPodcast] = useState<Podcast | null>(null)
  const [tags, setTags] = useState<Tag[]>([])
  const [notes, setNotes] = useState('')
  const [isEditingNotes, setIsEditingNotes] = useState(false)
  const [episodes, setEpisodes] = useState<Episode[]>([])
  const [displayedEpisodes, setDisplayedEpisodes] = useState<Episode[]>([]) // 渐进式显示的单集
  const [episodesLoading, setEpisodesLoading] = useState(true)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [visibleCount, setVisibleCount] = useState(10) // 可见的单集数量
  const [isLoadingMore, setIsLoadingMore] = useState(false) // 正在加载更多

  // 数据获取函数
  const fetchPodcast = async () => {
    try {
      setLoading(true)
      setError(null)
      const data = await podcastApi.get(id)
      setPodcast(data)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unknown error')
      console.error('Failed to fetch podcast:', err)
    } finally {
      setLoading(false)
    }
  }

  const fetchTags = async () => {
    try {
      const data = await podcastApi.getTags(id)
      setTags(data)
    } catch (err) {
      console.error('Failed to fetch tags:', err)
      setTags([])
    }
  }

  const fetchNotes = async () => {
    try {
      const data = await podcastApi.getNotes(id)
      setNotes(data ?? '')
    } catch (err) {
      console.error('Failed to fetch notes:', err)
      setNotes('')
    }
  }

  const fetchEpisodes = async () => {
    try {
      setEpisodesLoading(true)
      const data = await episodeApi.listByPodcast(id)
      setEpisodes(data)

      // 立即显示前10个单集
      const initialCount = Math.min(10, data.length)
      setDisplayedEpisodes(data.slice(0, initialCount))
      setVisibleCount(initialCount)

      // 如果单集数量<=10，立即完成加载
      if (data.length <= 10) {
        setEpisodesLoading(false)
      } else {
        setEpisodesLoading(false)
      }
    } catch (err) {
      console.error('Failed to fetch episodes:', err)
      setEpisodes([])
      setDisplayedEpisodes([])
      setEpisodesLoading(false)
    }
  }

  // 加载更多单集
  const loadMoreEpisodes = useCallback(() => {
    if (isLoadingMore || displayedEpisodes.length >= episodes.length) return

    setIsLoadingMore(true)
    // 模拟异步加载，避免频繁触发
    setTimeout(() => {
      const nextCount = Math.min(visibleCount + 10, episodes.length)
      setVisibleCount(nextCount)
      setDisplayedEpisodes(episodes.slice(0, nextCount))
      setIsLoadingMore(false)
    }, 300)
  }, [isLoadingMore, displayedEpisodes, episodes, visibleCount])

  useEffect(() => {
    if (id) {
      fetchPodcast()
      fetchTags()
      fetchNotes()
      fetchEpisodes()
    }
  }, [id])

  // 滚动监听 - 自动加载更多单集
  useEffect(() => {
    const handleScroll = () => {
      // 当滚动到距离底部500px时，自动加载更多
      const scrollPosition = window.innerHeight + window.scrollY
      const threshold = document.body.offsetHeight - 500

      if (scrollPosition >= threshold && !isLoadingMore && displayedEpisodes.length < episodes.length) {
        loadMoreEpisodes()
      }
    }

    window.addEventListener('scroll', handleScroll, { passive: true })
    return () => window.removeEventListener('scroll', handleScroll)
  }, [isLoadingMore, displayedEpisodes, episodes, loadMoreEpisodes])

  // 当单集列表加载完成且有目标单集时，展开到目标单集并滚动到指定位置
  useEffect(() => {
    if (!episodesLoading && targetEpisodeId && episodes.length > 0) {
      const targetEpisodeIdNum = parseInt(targetEpisodeId)

      // 找到目标单集在列表中的索引
      const targetIndex = episodes.findIndex(ep => ep.id === targetEpisodeIdNum)

      if (targetIndex !== -1) {
        // 计算需要显示的单集数量（目标索引 + 1，且要是 10 的倍数向上取整）
        const requiredVisibleCount = Math.ceil((targetIndex + 1) / 10) * 10

        // 更新 displayedEpisodes 以包含目标单集
        if (requiredVisibleCount > visibleCount) {
          setVisibleCount(requiredVisibleCount)
          setDisplayedEpisodes(episodes.slice(0, requiredVisibleCount))
        }

        // 等待 DOM 更新后滚动
        setTimeout(() => {
          const element = document.getElementById(`episode-${targetEpisodeId}`)
          if (element) {
            element.scrollIntoView({ behavior: 'smooth', block: 'center' })
            // 添加高亮效果
            element.classList.add('ring-2', 'ring-blue-500', 'ring-offset-2')
            setTimeout(() => {
              element.classList.remove('ring-2', 'ring-blue-500', 'ring-offset-2')
            }, 2000)
          }
        }, 300) // 增加延迟确保 DOM 完全渲染
      }
    }
  }, [episodesLoading, targetEpisodeId, episodes])

  // 处理标签变化（添加、移除、批量更新）
  const handleTagsChange = async (newTags: Tag[]) => {
    // 计算差异
    const currentIds = new Set(tags.map(t => t.id))
    const newIds = new Set(newTags.map(t => t.id))

    // 找出需要添加的标签
    const toAdd = newTags.filter(t => !currentIds.has(t.id))
    // 找出需要移除的标签
    const toRemove = tags.filter(t => !newIds.has(t.id))

    try {
      // 先添加新标签
      for (const tag of toAdd) {
        await podcastApi.addTag(id, tag.id)
      }

      // 再移除旧标签
      for (const tag of toRemove) {
        await podcastApi.removeTag(id, tag.id)
      }

      // 更新本地状态
      setTags(newTags)
    } catch (err) {
      const errorMsg = err instanceof Error ? err.message : '更新标签失败'
      alert(`标签更新失败: ${errorMsg}`)
      console.error('Failed to update tags:', err)
      // 刷新标签以恢复正确状态
      await fetchTags()
    }
  }

  const handleNotesSave = async () => {
    try {
      await podcastApi.updateNotes(id, notes)
      setIsEditingNotes(false)
    } catch (err) {
      const errorMsg = err instanceof Error ? err.message : '保存备注失败'
      alert(`保存失败: ${errorMsg}`)
      console.error('Failed to save notes:', err)
    }
  }

  return (
    <main className="min-h-screen bg-slate-50">
      <div className="container mx-auto px-4 py-8">
        {/* Header */}
        <div className="mb-8">
          <Link
            href={buildBackUrl()}
            className="w-36 h-11 px-4 bg-white text-slate-800 font-medium rounded-xl border border-slate-300 hover:bg-slate-50 hover:border-slate-400 transition-colors flex items-center justify-center gap-2"
          >
            <span>←</span>
            <span>返回列表</span>
          </Link>
        </div>

        {/* Loading State */}
        {loading && (
          <div className="text-center py-12">
            <div className="inline-block animate-spin rounded-full h-12 w-12 border-b-2 border-blue-600"></div>
            <p className="mt-4 text-slate-600">加载中...</p>
          </div>
        )}

        {/* Error State */}
        {error && (
          <div className="bg-red-50 border border-red-200 rounded-lg p-6">
            <h3 className="text-red-800 font-semibold mb-2">加载失败</h3>
            <p className="text-red-600">{error}</p>
          </div>
        )}

        {/* Podcast Detail */}
        {!loading && !error && podcast && (
          <div className="bg-white rounded-lg shadow-lg overflow-hidden">
            {/* Cover */}
            <div className="md:flex">
              <div className="md:w-1/3 p-6">
                <div className="aspect-square w-full rounded-lg overflow-hidden">
                  <PodcastCover
                    coverUrl={podcast.cover_url}
                    title={podcast.title}
                    priority="high"
                  />
                </div>
              </div>

              {/* Info */}
              <div className="md:w-2/3 p-8">
                <h1 className="text-3xl font-bold text-slate-900 mb-4">
                  {podcast.title}
                </h1>

                <div className="space-y-4 text-slate-600">
                  {/* 主播信息、单集数、最新更新、播放按钮 - 合并为同一行 */}
                  <div className="flex flex-wrap gap-6 items-center">
                    <div>
                      <span className="font-semibold text-slate-900">
                        主播：
                      </span>
                      {podcast.author}
                    </div>
                    <div>
                      <span className="font-semibold text-slate-900">
                        单集数：
                      </span>
                      {podcast.episode_count || 0}
                    </div>
                    <div>
                      <span className="font-semibold text-slate-900">
                        最新更新：
                      </span>
                      {(() => {
                        try {
                          const date = podcast.newest_episode_date ? new Date(podcast.newest_episode_date) : null
                          return date && !isNaN(date.getTime())
                            ? date.toLocaleString('zh-CN', {
                                year: 'numeric',
                                month: '2-digit',
                                day: '2-digit',
                                hour: '2-digit',
                                minute: '2-digit',
                                second: '2-digit',
                                hour12: false
                              })
                            : '未知'
                        } catch {
                          return '未知'
                        }
                      })()}
                    </div>
                    {/* 🆕 最新单集播放按钮 */}
                    {podcast.newest_enclosure_url && (
                      <button
                        onClick={() => window.open(podcast.newest_enclosure_url, '_blank')}
                        className="ml-2 px-2.5 py-1.5 bg-slate-700 hover:bg-slate-800 text-white text-sm rounded-lg transition-colors inline-flex items-center gap-1.5"
                        title="播放最新一集"
                      >
                        <svg className="w-4 h-4" fill="currentColor" viewBox="0 0 24 24">
                          <path d="M8 5v14l11-7z" />
                        </svg>
                        {podcast.newest_enclosure_duration && (
                          <span className="text-xs opacity-80">
                            {Math.floor(podcast.newest_enclosure_duration / 60)}分{podcast.newest_enclosure_duration % 60}秒
                          </span>
                        )}
                      </button>
                    )}
                  </div>

                  {/* 🆕 播客官网链接 */}
                  {podcast.link && (
                    <div>
                      <span className="font-semibold text-slate-900">
                        官网：
                      </span>
                      <a
                        href={podcast.link}
                        target="_blank"
                        rel="noopener noreferrer"
                        className="text-blue-600 hover:text-blue-700 dark:text-blue-400 dark:hover:text-blue-300 ml-2"
                      >
                        访问网站 →
                      </a>
                    </div>
                  )}

                  {/* 🆕 热门标签 */}
                  {podcast.popularity_score && podcast.popularity_score >= 7 && (
                    <div>
                      <span className="inline-flex items-center px-3 py-1 rounded-full text-sm font-medium bg-orange-100 text-orange-800 dark:bg-orange-900 dark:text-orange-200">
                        🔥 热门播客 (热度: {podcast.popularity_score}/10)
                      </span>
                    </div>
                  )}

                  <div>
                    <span className="font-semibold text-slate-900">
                      简介：
                    </span>
                    <div className="mt-1 text-sm">
                      <RichText html={podcast.description || '暂无简介'} />
                    </div>
                  </div>

                  {/* 标签管理 */}
                  <div>
                    <div className="inline-flex flex-wrap items-center gap-2">
                      <span className="font-semibold text-slate-900">
                        标签：
                      </span>
                      {/* 已选标签 - 紧跟"标签："后面 */}
                      {tags.length > 0 && (
                        <div className="inline-flex flex-wrap items-center gap-1.5">
                          {tags.map(tag => (
                            <span
                              key={tag.id}
                              className="inline-flex items-center gap-1 rounded-full font-medium text-sm px-3 py-1 bg-slate-100 hover:bg-slate-200 text-slate-600 transition-colors"
                            >
                              <span
                                className="w-1.5 h-1.5 rounded-full flex-shrink-0"
                                style={{ backgroundColor: tag.color }}
                              />
                              <span className="max-w-[120px] truncate" title={tag.name}>
                                {tag.name}
                              </span>
                            </span>
                          ))}
                        </div>
                      )}
                    </div>
                    {/* 标签输入框 - 换行显示 */}
                    <div className="mt-3">
                      <TagInput
                        selectedTags={tags}
                        onTagsChange={handleTagsChange}
                        placeholder="点击输入框从列表选择，或输入新标签名按回车添加"
                        showSelectedTags={false}
                      />
                    </div>
                  </div>

                  {/* 备注编辑 */}
                  <div>
                    <div className="flex items-center justify-between mb-2">
                      <span className="font-semibold text-slate-900">
                        备注：
                      </span>
                      {!isEditingNotes && (
                        <button
                          onClick={() => setIsEditingNotes(true)}
                          className="text-sm text-blue-600 hover:text-blue-700 dark:text-blue-400 dark:hover:text-blue-300"
                        >
                          编辑
                        </button>
                      )}
                    </div>
                    {isEditingNotes ? (
                      <div className="space-y-2">
                        <textarea
                          value={notes}
                          onChange={(e) => setNotes(e.target.value)}
                          className="w-full px-3 py-2 border border-slate-300 rounded-lg bg-white text-sm text-slate-900 focus:ring-2 focus:ring-blue-500 focus:border-transparent"
                          rows={4}
                          placeholder="添加备注..."
                        />
                        <div className="flex gap-2">
                          <button
                            onClick={handleNotesSave}
                            className="px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 transition-colors"
                          >
                            保存
                          </button>
                          <button
                            onClick={() => {
                              setIsEditingNotes(false)
                              fetchNotes() // 恢复原始内容
                            }}
                            className="px-4 py-2 bg-slate-200 text-slate-700 dark:text-slate-300 rounded-lg hover:bg-slate-300 dark:hover:bg-slate-600 transition-colors"
                          >
                            取消
                          </button>
                        </div>
                      </div>
                    ) : (
                      <p className="text-sm bg-slate-50/50 p-3 rounded-lg">
                        {notes || <span className="text-slate-400 dark:text-slate-500">暂无备注</span>}
                      </p>
                    )}
                  </div>
                </div>
              </div>
            </div>
          </div>
        )}

        {/* Episodes List - 新增section */}
        {!loading && !error && podcast && (
          <div className="mt-8" ref={episodeListRef}>
            <h2 className="text-2xl font-bold text-slate-900 mb-6">
              单集列表 ({episodes.length} 集)
            </h2>

            {/* 初始加载状态 - 显示骨架屏 */}
            {episodesLoading && displayedEpisodes.length === 0 ? (
              <div className="space-y-4">
                <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
                  {[1, 2, 3, 4].map((i) => (
                    <div key={i} className="bg-white rounded-lg shadow-sm p-6 animate-pulse">
                      <div className="flex items-start gap-3">
                        <div className="w-16 h-16 bg-slate-200 rounded-lg"></div>
                        <div className="flex-1 space-y-2">
                          <div className="h-4 bg-slate-200 rounded w-3/4"></div>
                          <div className="h-3 bg-slate-200 rounded w-1/2"></div>
                        </div>
                      </div>
                    </div>
                  ))}
                </div>
                <p className="text-center text-sm text-slate-600 mt-6">
                  正在加载单集列表...
                </p>
              </div>
            ) : episodes.length === 0 ? (
              <div className="bg-white rounded-lg p-12 text-center shadow-sm">
                <div className="text-6xl mb-4">📭</div>
                <p className="text-slate-600 text-lg">暂无单集</p>
                <p className="text-slate-5000 text-sm mt-2">
                  点击下方按钮同步单集数据
                </p>
              </div>
            ) : (
              <>
                <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
                  {displayedEpisodes.map((episode, index) => (
                    <div key={episode.id} id={`episode-${episode.id}`} className="transition-all duration-200">
                      <EpisodeCard
                        episode={episode}
                        podcastCover={podcast.cover_url}
                        index={index}
                        priority={index < 3 ? 'high' : index < 10 ? 'medium' : 'low'}
                      />
                    </div>
                  ))}
                </div>

                {/* 加载更多按钮 */}
                {displayedEpisodes.length < episodes.length && !isLoadingMore && (
                  <div className="text-center mt-8">
                    <button
                      onClick={loadMoreEpisodes}
                      className="px-6 py-3 bg-white text-slate-800 font-medium rounded-xl border border-slate-300 hover:bg-slate-50 hover:border-slate-400 transition-colors"
                    >
                      加载更多 ({displayedEpisodes.length}/{episodes.length})
                    </button>
                  </div>
                )}

                {/* 加载更多提示 */}
                {isLoadingMore && displayedEpisodes.length < episodes.length && (
                  <div className="text-center mt-8">
                    <p className="text-sm text-slate-600 flex items-center justify-center gap-2">
                      <span className="inline-block animate-spin rounded-full h-4 w-4 border-b-2 border-blue-600"></span>
                      正在加载更多单集...
                    </p>
                  </div>
                )}
              </>
            )}
          </div>
        )}
      </div>
    </main>
  )
}
