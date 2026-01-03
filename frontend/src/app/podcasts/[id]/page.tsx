'use client'

import { useEffect, useState } from 'react'
import { useParams } from 'next/navigation'
import Link from 'next/link'
import { podcastApi } from '@/lib/api'
import type { Podcast } from '@/types'

export default function PodcastDetailPage() {
  const params = useParams()
  const id = parseInt(params.id as string)

  const [podcast, setPodcast] = useState<Podcast | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (id) {
      fetchPodcast()
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
                  <div>
                    <span className="font-semibold text-slate-900 dark:text-slate-50">
                      主播：
                    </span>
                    {podcast.author}
                  </div>

                  <div>
                    <span className="font-semibold text-slate-900 dark:text-slate-50">
                      简介：
                    </span>
                    <p className="mt-1">{podcast.description}</p>
                  </div>

                  <div className="flex gap-6">
                    <div>
                      <span className="font-semibold text-slate-900 dark:text-slate-50">
                        单集数：
                      </span>
                      {podcast.episode_count}
                    </div>
                    <div>
                      <span className="font-semibold text-slate-900 dark:text-slate-50">
                        最新更新：
                      </span>
                      {new Date(podcast.newest_episode_date).toLocaleDateString()}
                    </div>
                  </div>

                  <div className="pt-4 border-t border-slate-200 dark:border-slate-700">
                    <div className="text-sm text-slate-500 dark:text-slate-500">
                      <div>ID: {podcast.id}</div>
                      <div>XYZ ID: {podcast.xyz_id}</div>
                      <div>
                        添加时间: {new Date(podcast.created_at).toLocaleString()}
                      </div>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </div>
        )}
      </div>
    </main>
  )
}
