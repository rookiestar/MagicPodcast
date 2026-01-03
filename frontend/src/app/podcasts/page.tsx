'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import { podcastApi } from '@/lib/api'
import type { Podcast } from '@/types'

export default function PodcastsPage() {
  const [podcasts, setPodcasts] = useState<Podcast[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    fetchPodcasts()
  }, [])

  const fetchPodcasts = async () => {
    try {
      setLoading(true)
      setError(null)
      const data = await podcastApi.list()
      setPodcasts(data)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unknown error')
    } finally {
      setLoading(false)
    }
  }

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
            管理你的播客节目（当前显示假数据）
          </p>
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
