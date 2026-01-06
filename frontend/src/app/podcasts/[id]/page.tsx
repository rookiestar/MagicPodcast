'use client'

import { useEffect, useState } from 'react'
import { useParams } from 'next/navigation'
import Link from 'next/link'
import { podcastApi, episodeApi } from '@/lib/api'
import type { Podcast, Tag, Episode } from '@/types'
import TagInput from '@/components/tags/TagInput'
import RichText from '@/components/RichText'

export default function PodcastDetailPage() {
  const params = useParams()
  const id = parseInt(params.id as string)

  const [podcast, setPodcast] = useState<Podcast | null>(null)
  const [tags, setTags] = useState<Tag[]>([])
  const [notes, setNotes] = useState('')
  const [isEditingNotes, setIsEditingNotes] = useState(false)
  const [episodes, setEpisodes] = useState<Episode[]>([])
  const [episodesLoading, setEpisodesLoading] = useState(true)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (id) {
      fetchPodcast()
      fetchTags()
      fetchNotes()
      fetchEpisodes()
    }
  }, [id])

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
    } catch (err) {
      console.error('Failed to fetch episodes:', err)
      setEpisodes([])
    } finally {
      setEpisodesLoading(false)
    }
  }

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
    <main className="min-h-screen bg-slate-50 dark:bg-slate-900">
      <div className="container mx-auto px-4 py-8">
        {/* Header */}
        <div className="mb-8">
          <Link
            href="/podcasts"
            className="text-blue-600 hover:text-blue-700 mb-4 inline-block"
          >
            ← 返回列表
          </Link>
        </div>

        {/* Loading State */}
        {loading && (
          <div className="text-center py-12">
            <div className="inline-block animate-spin rounded-full h-12 w-12 border-b-2 border-blue-600"></div>
            <p className="mt-4 text-slate-600 dark:text-slate-400">加载中...</p>
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
          <div className="bg-white dark:bg-slate-800 rounded-lg shadow-lg overflow-hidden">
            {/* Cover */}
            <div className="md:flex">
              <div className="md:w-1/3">
                <div className="aspect-square md:aspect-auto md:h-full bg-slate-200 dark:bg-slate-700 relative">
                  {podcast.cover_url ? (
                    <img
                      src={podcast.cover_url}
                      alt={podcast.title}
                      className="w-full h-full object-cover"
                    />
                  ) : (
                    <div className="w-full h-full flex items-center justify-center text-8xl">
                      🎧
                    </div>
                  )}
                </div>
              </div>

              {/* Info */}
              <div className="md:w-2/3 p-8">
                <h1 className="text-3xl font-bold text-slate-900 dark:text-slate-50 mb-4">
                  {podcast.title}
                </h1>

                <div className="space-y-4 text-slate-600 dark:text-slate-400">
                  {/* 主播信息 */}
                  <div>
                    <span className="font-semibold text-slate-900 dark:text-slate-50">
                      主播：
                    </span>
                    {podcast.author}
                  </div>

                  {/* 🆕 播客官网链接 */}
                  {podcast.link && (
                    <div>
                      <span className="font-semibold text-slate-900 dark:text-slate-50">
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

                  {/* 🆕 单集数和最新更新 */}
                  <div className="flex gap-6">
                    <div>
                      <span className="font-semibold text-slate-900 dark:text-slate-50">
                        单集数：
                      </span>
                      {podcast.episode_count || 0}
                    </div>
                    <div>
                      <span className="font-semibold text-slate-900 dark:text-slate-50">
                        最新更新：
                      </span>
                      {podcast.newest_episode_date
                        ? new Date(podcast.newest_episode_date).toLocaleString('zh-CN', {
                            year: 'numeric',
                            month: '2-digit',
                            day: '2-digit',
                            hour: '2-digit',
                            minute: '2-digit',
                            second: '2-digit',
                            hour12: false
                          })
                        : '未知'}
                    </div>
                  </div>

                  {/* 🆕 最新单集播放按钮 */}
                  {podcast.newest_enclosure_url && (
                    <div>
                      <span className="font-semibold text-slate-900 dark:text-slate-50">
                        最新一集：
                      </span>
                      <button
                        onClick={() => window.open(podcast.newest_enclosure_url, '_blank')}
                        className="ml-2 px-4 py-2 bg-green-600 text-white text-sm rounded-lg hover:bg-green-700 transition-colors inline-flex items-center gap-2"
                      >
                        ▶️ 播放
                        {podcast.newest_enclosure_duration && (
                          <span className="text-xs opacity-80">
                            ({Math.floor(podcast.newest_enclosure_duration / 60)}分{podcast.newest_enclosure_duration % 60}秒)
                          </span>
                        )}
                      </button>
                    </div>
                  )}

                  <div>
                    <span className="font-semibold text-slate-900 dark:text-slate-50">
                      简介：
                    </span>
                    <div className="mt-1">
                      <RichText html={podcast.description || '暂无简介'} />
                    </div>
                  </div>

                  {/* 标签管理 */}
                  <div>
                    <span className="font-semibold text-slate-900 dark:text-slate-50 block mb-2">
                      标签：
                    </span>
                    <TagInput
                      selectedTags={tags}
                      onTagsChange={handleTagsChange}
                      placeholder="输入标签名按回车添加"
                    />
                  </div>

                  {/* 备注编辑 */}
                  <div>
                    <div className="flex items-center justify-between mb-2">
                      <span className="font-semibold text-slate-900 dark:text-slate-50">
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
                          className="w-full px-3 py-2 border border-slate-300 dark:border-slate-600 rounded-lg bg-white dark:bg-slate-700 text-slate-900 dark:text-slate-100 focus:ring-2 focus:ring-blue-500 focus:border-transparent"
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
                            className="px-4 py-2 bg-slate-200 dark:bg-slate-700 text-slate-700 dark:text-slate-300 rounded-lg hover:bg-slate-300 dark:hover:bg-slate-600 transition-colors"
                          >
                            取消
                          </button>
                        </div>
                      </div>
                    ) : (
                      <p className="text-slate-700 dark:text-slate-300 bg-slate-50 dark:bg-slate-900/50 p-3 rounded-lg">
                        {notes || '暂无备注'}
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
          <div className="mt-8">
            <h2 className="text-2xl font-bold text-slate-900 dark:text-slate-50 mb-4">
              单集列表 ({podcast.episode_count} 集)
            </h2>

            {episodesLoading ? (
              <div className="text-center py-8">
                <div className="inline-block animate-spin rounded-full h-8 w-8 border-b-2 border-blue-600"></div>
                <p className="mt-2 text-sm text-slate-600 dark:text-slate-400">加载中...</p>
              </div>
            ) : episodes.length === 0 ? (
              <div className="bg-white dark:bg-slate-800 rounded-lg p-8 text-center">
                <p className="text-slate-600 dark:text-slate-400">暂无单集</p>
              </div>
            ) : (
              <div className="bg-white dark:bg-slate-800 rounded-lg shadow overflow-hidden">
                <div className="divide-y divide-slate-200 dark:divide-slate-700">
                  {episodes.map((episode) => (
                    <div key={episode.id} className="p-4 hover:bg-slate-50 dark:hover:bg-slate-700/50 transition-colors">
                      <div className="flex items-start justify-between gap-4">
                        <div className="flex-1 min-w-0">
                          <h3 className="text-lg font-semibold text-slate-900 dark:text-slate-50 mb-1 truncate">
                            {episode.title}
                          </h3>
                          <p className="text-sm text-slate-500 dark:text-slate-400 mb-2">
                            {episode.episode_no} · {new Date(episode.published_date).toLocaleDateString()}
                          </p>
                          {episode.show_notes && (
                            <div className="text-sm text-slate-600 dark:text-slate-400">
                              <RichText
                                html={episode.show_notes}
                                className="line-clamp-2"
                              />
                            </div>
                          )}
                        </div>
                        {episode.medium_url && (
                          <button
                            onClick={() => window.open(episode.medium_url, '_blank')}
                            className="flex-shrink-0 px-4 py-2 bg-blue-600 text-white text-sm rounded-lg hover:bg-blue-700 transition-colors"
                          >
                            播放
                          </button>
                        )}
                      </div>
                    </div>
                  ))}
                </div>
              </div>
            )}
          </div>
        )}
      </div>
    </main>
  )
}
