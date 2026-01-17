'use client'

import { useEffect, useState } from 'react'
import { useParams, useRouter } from 'next/navigation'
import Link from 'next/link'
import { workflowApi, podcastApi } from '@/lib/api'
import { schedulerApi } from '@/lib/api/scheduler'
import type { Workflow, Job, Podcast } from '@/types'
import WorkflowFormModal from '@/components/workflows/WorkflowFormModal'

type TabType = 'overview' | 'jobs' | 'config'

export default function WorkflowDetailPage() {
  const params = useParams()
  const router = useRouter()
  const id = parseInt(params.id as string)

  const [workflow, setWorkflow] = useState<Workflow | null>(null)
  const [jobs, setJobs] = useState<Job[]>([])
  const [podcasts, setPodcasts] = useState<Podcast[]>([])
  const [activeTab, setActiveTab] = useState<TabType>('overview')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [showEditModal, setShowEditModal] = useState(false)

  // Job详情展开状态
  const [selectedJobId, setSelectedJobId] = useState<number | null>(null)
  const [jobDetails, setJobDetails] = useState<Record<number, Job>>({})
  const [loadingJobId, setLoadingJobId] = useState<number | null>(null)

  useEffect(() => {
    if (id) {
      fetchWorkflow()
      fetchJobs()
    }
  }, [id])

  const fetchWorkflow = async () => {
    try {
      setLoading(true)
      setError(null)
      const data = await workflowApi.get(id)
      setWorkflow(data)

      // 如果是指定节目类型，获取播客列表
      if (data.scope_type === 'specific_podcasts' && data.scope_config?.podcast_ids && data.scope_config.podcast_ids.length > 0) {
        fetchPodcasts(data.scope_config.podcast_ids)
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unknown error')
      console.error('Failed to fetch workflow:', err)
    } finally {
      setLoading(false)
    }
  }

  const fetchPodcasts = async (podcastIds: number[]) => {
    try {
      // 获取所有播客（分页加载）
      const allPodcasts: Podcast[] = []
      let page = 1
      let hasMore = true

      while (hasMore) {
        const response = await podcastApi.list({ page, page_size: 100 })
        const filtered = response.data.filter(p => podcastIds.includes(p.id))
        allPodcasts.push(...filtered)

        // 如果已经获取到所有需要的播客，或者没有更多数据了
        if (allPodcasts.length >= podcastIds.length || response.data.length < 100) {
          hasMore = false
        } else {
          page++
        }
      }

      setPodcasts(allPodcasts)
    } catch (err) {
      console.error('Failed to fetch podcasts:', err)
    }
  }

  const fetchJobs = async () => {
    try {
      const response = await workflowApi.listJobs(id, { page: 1, page_size: 20 })
      setJobs(response.jobs)
    } catch (err) {
      console.error('Failed to fetch jobs:', err)
    }
  }

  const fetchJobDetail = async (jobId: number) => {
    // 如果已经缓存，直接切换展开状态
    if (jobDetails[jobId]) {
      setSelectedJobId(selectedJobId === jobId ? null : jobId)
      return
    }

    // 如果是同一个Job且正在加载，不重复请求
    if (loadingJobId === jobId) {
      return
    }

    try {
      setLoadingJobId(jobId)
      const detail = await workflowApi.getJob(jobId)
      setJobDetails(prev => ({ ...prev, [jobId]: detail }))
      setSelectedJobId(jobId)
    } catch (err) {
      console.error('Failed to fetch job detail:', err)
      alert('获取详情失败')
    } finally {
      setLoadingJobId(null)
    }
  }

  const handleToggle = async () => {
    if (!workflow) return
    try {
      const updated = await workflowApi.toggle(id)
      setWorkflow(updated)
    } catch (err) {
      alert(`操作失败: ${err instanceof Error ? err.message : 'Unknown error'}`)
    }
  }

  const handleTrigger = async () => {
    if (!workflow) return
    if (!confirm('确定要立即执行此工作流吗?')) return

    try {
      await workflowApi.trigger(id)
      alert('工作流已触发')
      fetchWorkflow()
      fetchJobs()
    } catch (err) {
      alert(`触发失败: ${err instanceof Error ? err.message : 'Unknown error'}`)
    }
  }

  const handleDelete = async () => {
    if (!workflow) return
    if (!confirm('确定要删除这个工作流吗？此操作不可恢复。')) return

    try {
      await workflowApi.delete(id)
      router.push('/workflows')
    } catch (err) {
      alert(`删除失败: ${err instanceof Error ? err.message : 'Unknown error'}`)
    }
  }

  const getStatusBadge = (isEnabled: boolean) => {
    return isEnabled ? (
      <span className="inline-flex items-center px-3 py-1 rounded-full text-sm font-medium bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200">
        ● 启用中
      </span>
    ) : (
      <span className="inline-flex items-center px-3 py-1 rounded-full text-sm font-medium bg-gray-100 text-gray-800 dark:bg-gray-800 dark:text-gray-300">
        ○ 已禁用
      </span>
    )
  }

  const getJobStatusBadge = (status: string) => {
    const statusMap: Record<string, { text: string; className: string }> = {
      pending: { text: '等待中', className: 'bg-gray-100 text-gray-800 dark:bg-gray-800 dark:text-gray-300' },
      running: { text: '执行中', className: 'bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200' },
      completed: { text: '已完成', className: 'bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200' },
      failed: { text: '失败', className: 'bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-200' },
      cancelled: { text: '已取消', className: 'bg-yellow-100 text-yellow-800 dark:bg-yellow-900 dark:text-yellow-200' },
    }
    const statusInfo = statusMap[status] || statusMap.pending
    return (
      <span className={`inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium ${statusInfo.className}`}>
        {statusInfo.text}
      </span>
    )
  }

  if (loading) {
    return (
      <main className="min-h-screen bg-slate-50 dark:bg-slate-900">
        <div className="container mx-auto px-4 py-8">
          <div className="text-center py-12">
            <div className="inline-block animate-spin rounded-full h-12 w-12 border-b-2 border-blue-600"></div>
            <p className="mt-4 text-slate-600 dark:text-slate-400">加载中...</p>
          </div>
        </div>
      </main>
    )
  }

  if (error || !workflow) {
    return (
      <main className="min-h-screen bg-slate-50 dark:bg-slate-900">
        <div className="container mx-auto px-4 py-8">
          <div className="bg-red-50 border border-red-200 rounded-lg p-6">
            <h3 className="text-red-800 font-semibold mb-2">加载失败</h3>
            <p className="text-red-600">{error || '工作流不存在'}</p>
            <Link
              href="/workflows"
              className="mt-4 inline-block text-blue-600 hover:text-blue-700"
            >
              ← 返回列表
            </Link>
          </div>
        </div>
      </main>
    )
  }

  return (
    <main className="min-h-screen bg-slate-50 dark:bg-slate-900">
      <div className="container mx-auto px-4 py-8">
        {/* Header */}
        <div className="mb-8">
          <div className="mb-4">
            <Link
              href="/workflows"
              className="w-36 h-11 px-4 bg-white dark:bg-slate-700 text-slate-800 dark:text-slate-200 font-medium rounded-xl border border-slate-300 dark:border-slate-600 hover:bg-slate-50 dark:hover:bg-slate-600 hover:border-slate-400 dark:hover:border-slate-500 transition-colors flex items-center justify-center gap-2"
            >
              <span>←</span>
              <span>返回列表</span>
            </Link>
          </div>

          <div className="bg-white dark:bg-slate-800 rounded-lg shadow-lg p-6">
            <div className="flex items-start justify-between mb-4">
              <div className="flex-1">
                <div className="flex items-center gap-3 mb-2">
                  <h1 className="text-3xl font-bold text-slate-900 dark:text-slate-50">
                    {workflow.id}: {workflow.name}
                  </h1>
                  {getStatusBadge(workflow.is_enabled)}
                </div>
                {workflow.description && (
                  <p className="text-slate-600 dark:text-slate-400 mt-2">
                    {workflow.description}
                  </p>
                )}
              </div>
              <div className="flex items-center gap-2">
                <button
                  onClick={handleTrigger}
                  className="px-4 py-2 bg-slate-100 dark:bg-slate-700 text-slate-700 dark:text-slate-300 rounded-lg hover:bg-slate-200 dark:hover:bg-slate-600 transition-colors text-sm flex items-center gap-2"
                >
                  <svg className="w-4 h-4 text-blue-600 dark:text-blue-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2.5} d="M13 10V3L4 14h7v7l9-11h-7z" />
                  </svg>
                  手动执行
                </button>
                <button
                  onClick={handleToggle}
                  className={`px-4 py-2 rounded-lg transition-colors text-sm flex items-center gap-2 ${
                    workflow.is_enabled
                      ? 'bg-slate-100 dark:bg-slate-700 text-slate-700 dark:text-slate-300 hover:bg-slate-200 dark:hover:bg-slate-600'
                      : 'bg-slate-100 dark:bg-slate-700 text-slate-700 dark:text-slate-300 hover:bg-slate-200 dark:hover:bg-slate-600'
                  }`}
                >
                  {workflow.is_enabled ? (
                    <>
                      <svg className="w-4 h-4 text-amber-600 dark:text-amber-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2.5} d="M10 9v6m4-6v6m7-3a9 9 0 11-18 0 9 9 0 0118 0z" />
                      </svg>
                      停用
                    </>
                  ) : (
                    <>
                      <svg className="w-4 h-4 text-green-600 dark:text-green-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2.5} d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
                      </svg>
                      启用
                    </>
                  )}
                </button>
                <button
                  onClick={() => setShowEditModal(true)}
                  className="px-4 py-2 bg-slate-100 dark:bg-slate-700 text-slate-700 dark:text-slate-300 rounded-lg hover:bg-slate-200 dark:hover:bg-slate-600 transition-colors text-sm flex items-center gap-2"
                >
                  <svg className="w-4 h-4 text-slate-800 dark:text-slate-200" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2.5} d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2h2.828l8.586-8.586z" />
                  </svg>
                  编辑
                </button>
                <button
                  onClick={handleDelete}
                  className="px-4 py-2 bg-slate-100 dark:bg-slate-700 text-red-600 dark:text-red-400 rounded-lg hover:bg-red-50 dark:hover:bg-red-900/20 transition-colors text-sm flex items-center gap-2"
                >
                  <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2.5} d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
                  </svg>
                  删除
                </button>
              </div>
            </div>

            {/* Tabs */}
            <div className="border-t border-slate-200 dark:border-slate-700 pt-4">
              <div className="flex gap-6">
                <button
                  onClick={() => setActiveTab('overview')}
                  className={`pb-2 border-b-2 transition-colors ${
                    activeTab === 'overview'
                      ? 'border-blue-600 text-blue-600 dark:text-blue-400'
                      : 'border-transparent text-slate-600 hover:text-slate-900 dark:text-slate-400 dark:hover:text-slate-200'
                  }`}
                >
                  📊 概览
                </button>
                <button
                  onClick={() => setActiveTab('jobs')}
                  className={`pb-2 border-b-2 transition-colors ${
                    activeTab === 'jobs'
                      ? 'border-blue-600 text-blue-600 dark:text-blue-400'
                      : 'border-transparent text-slate-600 hover:text-slate-900 dark:text-slate-400 dark:hover:text-slate-200'
                  }`}
                >
                  📜 执行历史
                </button>
                <button
                  onClick={() => setActiveTab('config')}
                  className={`pb-2 border-b-2 transition-colors ${
                    activeTab === 'config'
                      ? 'border-blue-600 text-blue-600 dark:text-blue-400'
                      : 'border-transparent text-slate-600 hover:text-slate-900 dark:text-slate-400 dark:hover:text-slate-200'
                  }`}
                >
                  ⚙️ 配置
                </button>
              </div>
            </div>
          </div>
        </div>

        {/* Tab Content */}
        <div className="bg-white dark:bg-slate-800 rounded-lg shadow-lg p-6">
          {activeTab === 'overview' && (
            <div>
              <h2 className="text-xl font-semibold text-slate-900 dark:text-slate-50 mb-4">调度信息</h2>
              <div className="bg-slate-50 dark:bg-slate-900 rounded-lg p-5">
                <div className="grid md:grid-cols-2 gap-8">
                  {/* 左列: 抓取范围 */}
                  <div>
                    <p className="text-sm text-slate-600 dark:text-slate-400 mb-2">抓取范围</p>
                    <p className="text-slate-900 dark:text-slate-50 mb-3">
                      {workflow.scope_type === 'all_subscribed' && '全部订阅'}
                      {workflow.scope_type === 'specific_podcasts' && `指定节目 (${podcasts.length}个)`}
                      {workflow.scope_type === 'custom_sources' && '自定义源'}
                    </p>
                    {workflow.scope_type === 'specific_podcasts' && podcasts.length > 0 && (
                      <div className="flex flex-wrap gap-2">
                        {podcasts.map((podcast) => (
                          <Link
                            key={podcast.id}
                            href={`/podcasts/${podcast.id}`}
                            className="text-xs px-2 py-1 bg-blue-50 dark:bg-blue-900/30 text-blue-700 dark:text-blue-300 rounded hover:bg-blue-100 dark:hover:bg-blue-900/50 transition-colors"
                          >
                            {podcast.title}
                          </Link>
                        ))}
                      </div>
                    )}
                  </div>

                  {/* 右列: 定时规则和执行时间 */}
                  <div className="space-y-4">
                    {/* 定时规则 */}
                    <div>
                      <p className="text-sm text-slate-600 dark:text-slate-400 mb-2">定时规则</p>
                      <code className="px-3 py-1.5 bg-slate-100 dark:bg-slate-700 rounded text-sm font-mono">
                        {workflow.schedule}
                      </code>
                    </div>

                    {/* 上次执行 */}
                    <div>
                      <p className="text-sm text-slate-600 dark:text-slate-400 mb-1">上次执行</p>
                      {workflow.stats?.last_execution ? (
                        <span className="text-slate-900 dark:text-slate-50">
                          {new Date(workflow.stats.last_execution).toLocaleString('zh-CN')}
                        </span>
                      ) : (
                        <span className="text-sm text-slate-500 dark:text-slate-400">暂无记录</span>
                      )}
                    </div>

                    {/* 下次执行 */}
                    <div>
                      <p className="text-sm text-slate-600 dark:text-slate-400 mb-1">下次执行</p>
                      {workflow.stats?.next_execution ? (
                        <span className="text-blue-600 dark:text-blue-400 font-medium">
                          {new Date(workflow.stats.next_execution).toLocaleString('zh-CN')}
                        </span>
                      ) : (
                        <span className="text-sm text-slate-500 dark:text-slate-400">
                          {workflow.is_enabled ? '等待调度...' : '工作流已禁用'}
                        </span>
                      )}
                    </div>
                  </div>
                </div>
              </div>

              {/* 配置详情 */}
              <div className="mt-6">
                <h3 className="text-lg font-semibold text-slate-900 dark:text-slate-50 mb-3">配置详情</h3>
                <div className="grid md:grid-cols-2 gap-6">
                  {/* 范围配置 */}
                  <div>
                    <h4 className="text-sm font-medium text-slate-700 dark:text-slate-300 mb-3">范围配置</h4>
                    {workflow.scope_type === 'specific_podcasts' && podcasts.length > 0 ? (
                      <div className="bg-slate-50 dark:bg-slate-900 rounded-lg p-4">
                        <div className="flex flex-wrap gap-2">
                          {podcasts.map((podcast) => (
                            <Link
                              key={podcast.id}
                              href={`/podcasts/${podcast.id}`}
                              className="text-sm px-3 py-2 bg-white dark:bg-slate-800 text-slate-900 dark:text-slate-50 rounded-lg border border-slate-200 dark:border-slate-700 hover:border-blue-400 dark:hover:border-blue-500 hover:shadow-sm transition-all"
                            >
                              <div className="flex items-center gap-2">
                                {podcast.cover_url && (
                                  <img
                                    src={podcast.cover_url}
                                    alt={podcast.title}
                                    className="w-8 h-8 rounded object-cover"
                                  />
                                )}
                                <span className="font-medium">{podcast.title}</span>
                              </div>
                            </Link>
                          ))}
                        </div>
                      </div>
                    ) : (
                      <pre className="bg-slate-100 dark:bg-slate-900 rounded-lg p-4 text-xs overflow-x-auto">
                        {JSON.stringify(workflow.scope_config, null, 2)}
                      </pre>
                    )}
                  </div>

                  {/* 规则配置 */}
                  <div>
                    <h4 className="text-sm font-medium text-slate-700 dark:text-slate-300 mb-3">规则配置</h4>
                    <div className="bg-slate-50 dark:bg-slate-900 rounded-lg p-4">
                      {workflow.rules_config?.time_range && workflow.rules_config.time_range > 0 && (
                        <div className="mb-2">
                          <span className="text-slate-600 dark:text-slate-400">时间范围：</span>
                          <span className="ml-2 font-medium text-slate-900 dark:text-slate-50">
                            最近 {workflow.rules_config.time_range} 天
                          </span>
                        </div>
                      )}
                      {workflow.rules_config?.min_duration && workflow.rules_config.min_duration > 0 && (
                        <div className="mb-2">
                          <span className="text-slate-600 dark:text-slate-400">最小时长：</span>
                          <span className="ml-2 font-medium text-slate-900 dark:text-slate-50">
                            {Math.floor(workflow.rules_config.min_duration / 60)} 分钟
                          </span>
                        </div>
                      )}
                      {workflow.rules_config?.max_results && workflow.rules_config.max_results > 0 && (
                        <div className="mb-2">
                          <span className="text-slate-600 dark:text-slate-400">最大结果数：</span>
                          <span className="ml-2 font-medium text-slate-900 dark:text-slate-50">
                            {workflow.rules_config.max_results} 个
                          </span>
                        </div>
                      )}
                      {workflow.rules_config?.keywords && (
                        <div className="mb-2">
                          <span className="text-slate-600 dark:text-slate-400">关键词：</span>
                          <span className="ml-2 font-medium text-slate-900 dark:text-slate-50">
                            {workflow.rules_config.keywords}
                          </span>
                        </div>
                      )}
                      {workflow.rules_config?.exclude_words && (
                        <div>
                          <span className="text-slate-600 dark:text-slate-400">排除词：</span>
                          <span className="ml-2 font-medium text-slate-900 dark:text-slate-50">
                            {workflow.rules_config.exclude_words}
                          </span>
                        </div>
                      )}
                      {(!workflow.rules_config?.time_range || workflow.rules_config.time_range === 0) &&
                       !workflow.rules_config?.min_duration &&
                       !workflow.rules_config?.max_results &&
                       !workflow.rules_config?.keywords &&
                       !workflow.rules_config?.exclude_words && (
                        <p className="text-slate-500 dark:text-slate-400 text-sm">无特殊规则</p>
                      )}
                    </div>
                  </div>
                </div>
              </div>

              {workflow.stats && (
                <div className="mt-6">
                  <h3 className="text-lg font-semibold text-slate-900 dark:text-slate-50 mb-3">统计数据</h3>
                  <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
                    <div className="bg-slate-50 dark:bg-slate-900 rounded-lg p-4">
                      <p className="text-2xl font-bold text-slate-900 dark:text-slate-50">
                        {workflow.stats.total_jobs}
                      </p>
                      <p className="text-sm text-slate-600 dark:text-slate-400">执行次数</p>
                    </div>
                    <div className="bg-slate-50 dark:bg-slate-900 rounded-lg p-4">
                      <p className="text-2xl font-bold text-green-600">
                        {workflow.stats.success_rate.toFixed(1)}%
                      </p>
                      <p className="text-sm text-slate-600 dark:text-slate-400">成功率</p>
                    </div>
                    <div className="bg-slate-50 dark:bg-slate-900 rounded-lg p-4">
                      <p className="text-2xl font-bold text-slate-900 dark:text-slate-50">
                        {workflow.stats.total_episodes}
                      </p>
                      <p className="text-sm text-slate-600 dark:text-slate-400">创建单集</p>
                    </div>
                    <div className="bg-slate-50 dark:bg-slate-900 rounded-lg p-4">
                      <p className="text-2xl font-bold text-slate-900 dark:text-slate-50">
                        {workflow.stats.success_jobs}
                      </p>
                      <p className="text-sm text-slate-600 dark:text-slate-400">成功次数</p>
                    </div>
                  </div>
                </div>
              )}

              {workflow.last_job && (
                <div className="mt-6">
                  <h3 className="text-lg font-semibold text-slate-900 dark:text-slate-50 mb-3">最近执行</h3>
                  <div className="bg-slate-50 dark:bg-slate-900 rounded-lg p-4">
                    <div className="flex items-center justify-between mb-2">
                      <div className="flex items-center gap-3">
                        {getJobStatusBadge(workflow.last_job.status)}
                        <span className="text-sm text-slate-600 dark:text-slate-400">
                          {new Date(workflow.last_job.created_at).toLocaleString('zh-CN')}
                        </span>
                      </div>
                      {workflow.last_job.duration && (
                        <span className="text-sm text-slate-600 dark:text-slate-400">
                          耗时 {Math.floor(workflow.last_job.duration / 1000)}秒
                        </span>
                      )}
                    </div>
                    <div className="grid grid-cols-3 gap-4 text-sm">
                      <div>
                        <span className="text-slate-600 dark:text-slate-400">处理节目:</span>
                        <span className="ml-2 font-medium text-slate-900 dark:text-slate-50">
                          {workflow.last_job.podcasts_processed}
                        </span>
                      </div>
                      <div>
                        <span className="text-slate-600 dark:text-slate-400">发现单集:</span>
                        <span className="ml-2 font-medium text-slate-900 dark:text-slate-50">
                          {workflow.last_job.episodes_found}
                        </span>
                      </div>
                      <div>
                        <span className="text-slate-600 dark:text-slate-400">创建单集:</span>
                        <span className="ml-2 font-medium text-slate-900 dark:text-slate-50">
                          {workflow.last_job.episodes_created}
                        </span>
                      </div>
                    </div>
                  </div>
                </div>
              )}
            </div>
          )}

          {activeTab === 'jobs' && (
            <div>
              <h2 className="text-xl font-semibold text-slate-900 dark:text-slate-50 mb-4">执行历史</h2>
              {jobs.length === 0 ? (
                <div className="text-center py-8 text-slate-500 dark:text-slate-400">
                  暂无执行记录
                </div>
              ) : (
                <div className="space-y-3">
                  {jobs.map((job) => (
                    <div
                      key={job.id}
                      className="border border-slate-200 dark:border-slate-700 rounded-lg overflow-hidden"
                    >
                      {/* Job摘要卡片 - 可点击展开/收起 */}
                      <div
                        onClick={() => fetchJobDetail(job.id)}
                        className="p-4 hover:bg-slate-50 dark:hover:bg-slate-900 transition-colors cursor-pointer"
                      >
                        <div className="flex items-start justify-between mb-2">
                          <div className="flex items-center gap-3">
                            {getJobStatusBadge(job.status)}
                            <span className="text-sm text-slate-600 dark:text-slate-400">
                              {new Date(job.created_at).toLocaleString('zh-CN')}
                            </span>
                            <span className="text-xs px-2 py-1 bg-slate-100 dark:bg-slate-700 rounded">
                              {job.triggered_by === 'cron' ? '定时' : '手动'}
                            </span>
                          </div>
                          <div className="flex items-center gap-2">
                            {job.duration && (
                              <span className="text-sm text-slate-600 dark:text-slate-400">
                                {Math.floor(job.duration / 1000)}s
                              </span>
                            )}
                            <span className="text-slate-400 text-sm">
                              {selectedJobId === job.id ? '▲ 收起' : '▼ 展开'}
                            </span>
                          </div>
                        </div>

                        <div className="grid grid-cols-4 gap-4 text-sm">
                          <div>
                            <span className="text-slate-600 dark:text-slate-400">处理节目:</span>
                            <span className="ml-2 font-medium text-slate-900 dark:text-slate-50">
                              {job.podcasts_processed}
                            </span>
                          </div>
                          <div>
                            <span className="text-slate-600 dark:text-slate-400">发现单集:</span>
                            <span className="ml-2 font-medium text-slate-900 dark:text-slate-50">
                              {job.episodes_found}
                            </span>
                          </div>
                          <div>
                            <span className="text-slate-600 dark:text-slate-400">创建单集:</span>
                            <span className="ml-2 font-medium text-slate-900 dark:text-slate-50">
                              {job.episodes_created}
                            </span>
                          </div>
                          <div>
                            <span className="text-slate-600 dark:text-slate-400">错误数:</span>
                            <span className={`ml-2 font-medium ${
                              job.error_count > 0 ? 'text-red-600' : 'text-slate-900 dark:text-slate-50'
                            }`}>
                              {job.error_count}
                            </span>
                          </div>
                        </div>
                      </div>

                      {/* 展开的详细执行记录 */}
                      {selectedJobId === job.id && (
                        <div className="border-t border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-900/50">
                          {loadingJobId === job.id ? (
                            <div className="p-4 text-center text-sm text-slate-500 dark:text-slate-400">
                              加载中...
                            </div>
                          ) : jobDetails[job.id]?.executions ? (
                            <div className="p-4">
                              <h4 className="text-sm font-medium text-slate-700 dark:text-slate-300 mb-3">
                                详细执行记录
                              </h4>
                              <div className="space-y-2">
                                {jobDetails[job.id]?.executions?.map((exec) => (
                                  <div
                                    key={exec.id}
                                    className="bg-white dark:bg-slate-800 rounded-lg p-3 border border-slate-200 dark:border-slate-700"
                                  >
                                    <div className="flex items-start justify-between mb-2">
                                      <div className="flex-1">
                                        <div className="flex items-center gap-2 mb-1">
                                          {exec.status === 'success' && (
                                            <span className="text-green-600 dark:text-green-400">✓</span>
                                          )}
                                          {exec.status === 'failed' && (
                                            <span className="text-red-600 dark:text-red-400">✗</span>
                                          )}
                                          {exec.status === 'skipped' && (
                                            <span className="text-yellow-600 dark:text-yellow-400">○</span>
                                          )}
                                          <span className="font-medium text-slate-900 dark:text-slate-50">
                                            {exec.podcast_title}
                                          </span>
                                        </div>
                                        <a
                                          href={exec.podcast_feed_url}
                                          target="_blank"
                                          rel="noopener noreferrer"
                                          className="text-xs text-blue-600 hover:text-blue-700 dark:text-blue-400 dark:hover:text-blue-300 truncate max-w-md block"
                                        >
                                          {exec.podcast_feed_url}
                                        </a>
                                      </div>
                                      <span className="text-xs text-slate-500 dark:text-slate-400">
                                        {exec.processing_time}ms
                                      </span>
                                    </div>

                                    <div className="grid grid-cols-3 gap-4 text-xs">
                                      <div>
                                        <span className="text-slate-600 dark:text-slate-400">状态:</span>
                                        <span className="ml-1 font-medium text-slate-900 dark:text-slate-50">
                                          {exec.status === 'success' && '成功'}
                                          {exec.status === 'failed' && '失败'}
                                          {exec.status === 'skipped' && '跳过'}
                                          {exec.status === 'running' && '执行中'}
                                        </span>
                                      </div>
                                      <div>
                                        <span className="text-slate-600 dark:text-slate-400">发现:</span>
                                        <span className="ml-1 font-medium text-slate-900 dark:text-slate-50">
                                          {exec.episodes_found}
                                        </span>
                                      </div>
                                      <div>
                                        <span className="text-slate-600 dark:text-slate-400">新建:</span>
                                        <span className="ml-1 font-medium text-slate-900 dark:text-slate-50">
                                          {exec.episodes_created}
                                        </span>
                                      </div>
                                    </div>

                                    {exec.error_message && (
                                      <div className="mt-2 text-xs text-red-600 bg-red-50 dark:bg-red-900/20 rounded p-2">
                                        错误: {exec.error_message}
                                      </div>
                                    )}
                                  </div>
                                ))}
                              </div>

                              {jobDetails[job.id]?.executions?.length === 0 && (
                                <div className="text-center py-4 text-sm text-slate-500 dark:text-slate-400">
                                  暂无详细执行记录
                                </div>
                              )}
                            </div>
                          ) : (
                            <div className="p-4 text-center text-sm text-slate-500 dark:text-slate-400">
                              点击获取详细记录
                            </div>
                          )}
                        </div>
                      )}
                    </div>
                  ))}
                </div>
              )}
            </div>
          )}

          {activeTab === 'config' && (
            <div>
              <h2 className="text-xl font-semibold text-slate-900 dark:text-slate-50 mb-4">配置预览</h2>
              <div className="space-y-4">
                <div>
                  <h3 className="text-sm font-medium text-slate-700 dark:text-slate-300 mb-2">范围配置</h3>
                  {workflow.scope_type === 'specific_podcasts' && podcasts.length > 0 ? (
                    <div className="bg-slate-50 dark:bg-slate-900 rounded-lg p-4">
                      <div className="flex flex-wrap gap-2">
                        {podcasts.map((podcast) => (
                          <Link
                            key={podcast.id}
                            href={`/podcasts/${podcast.id}`}
                            className="text-sm px-3 py-2 bg-white dark:bg-slate-800 text-slate-900 dark:text-slate-50 rounded-lg border border-slate-200 dark:border-slate-700 hover:border-blue-400 dark:hover:border-blue-500 hover:shadow-sm transition-all"
                          >
                            <div className="flex items-center gap-2">
                              {podcast.cover_url && (
                                <img
                                  src={podcast.cover_url}
                                  alt={podcast.title}
                                  className="w-8 h-8 rounded object-cover"
                                />
                              )}
                              <span className="font-medium">{podcast.title}</span>
                            </div>
                          </Link>
                        ))}
                      </div>
                    </div>
                  ) : (
                    <pre className="bg-slate-100 dark:bg-slate-900 rounded-lg p-4 text-sm overflow-x-auto">
                      {JSON.stringify(workflow.scope_config, null, 2)}
                    </pre>
                  )}
                </div>
                <div>
                  <h3 className="text-sm font-medium text-slate-700 dark:text-slate-300 mb-2">规则配置</h3>
                  <div className="bg-slate-50 dark:bg-slate-900 rounded-lg p-4">
                    {workflow.rules_config?.time_range && workflow.rules_config.time_range > 0 && (
                      <div className="mb-2">
                        <span className="text-slate-600 dark:text-slate-400">时间范围：</span>
                        <span className="ml-2 font-medium text-slate-900 dark:text-slate-50">
                          最近 {workflow.rules_config.time_range} 天
                        </span>
                      </div>
                    )}
                    {workflow.rules_config?.min_duration && workflow.rules_config.min_duration > 0 && (
                      <div className="mb-2">
                        <span className="text-slate-600 dark:text-slate-400">最小时长：</span>
                        <span className="ml-2 font-medium text-slate-900 dark:text-slate-50">
                          {Math.floor(workflow.rules_config.min_duration / 60)} 分钟
                        </span>
                      </div>
                    )}
                    {workflow.rules_config?.max_results && workflow.rules_config.max_results > 0 && (
                      <div className="mb-2">
                        <span className="text-slate-600 dark:text-slate-400">最大结果数：</span>
                        <span className="ml-2 font-medium text-slate-900 dark:text-slate-50">
                          {workflow.rules_config.max_results} 个
                        </span>
                      </div>
                    )}
                    {workflow.rules_config?.keywords && (
                      <div className="mb-2">
                        <span className="text-slate-600 dark:text-slate-400">关键词：</span>
                        <span className="ml-2 font-medium text-slate-900 dark:text-slate-50">
                          {workflow.rules_config.keywords}
                        </span>
                      </div>
                    )}
                    {workflow.rules_config?.exclude_words && (
                      <div>
                        <span className="text-slate-600 dark:text-slate-400">排除词：</span>
                        <span className="ml-2 font-medium text-slate-900 dark:text-slate-50">
                          {workflow.rules_config.exclude_words}
                        </span>
                      </div>
                    )}
                    {(!workflow.rules_config?.time_range || workflow.rules_config.time_range === 0) &&
                     !workflow.rules_config?.min_duration &&
                     !workflow.rules_config?.max_results &&
                     !workflow.rules_config?.keywords &&
                     !workflow.rules_config?.exclude_words && (
                      <p className="text-slate-500 dark:text-slate-400 text-sm">无特殊规则</p>
                    )}
                  </div>
                </div>
              </div>
            </div>
          )}
        </div>
      </div>

      {/* Edit Workflow Modal */}
      {workflow && (
        <WorkflowFormModal
          isOpen={showEditModal}
          onClose={() => setShowEditModal(false)}
          onSuccess={async () => {
            // 如果修改了schedule或is_enabled，需要重载调度器
            try {
              await schedulerApi.reload()
            } catch (err) {
              console.error('Failed to reload scheduler:', err)
            }
            fetchWorkflow()
            setShowEditModal(false)
          }}
          workflow={workflow}
        />
      )}
    </main>
  )
}
