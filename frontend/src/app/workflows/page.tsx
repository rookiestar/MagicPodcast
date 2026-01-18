'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import { workflowApi } from '@/lib/api'
import type { Workflow } from '@/types'
import WorkflowFormModal from '@/components/workflows/WorkflowFormModal'

export default function WorkflowsPage() {
  const [workflows, setWorkflows] = useState<Workflow[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [showCreateModal, setShowCreateModal] = useState(false)

  useEffect(() => {
    fetchWorkflows()
  }, [])

  const fetchWorkflows = async () => {
    try {
      setLoading(true)
      setError(null)
      const response = await workflowApi.list()
      setWorkflows(response.workflows)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unknown error')
      console.error('Failed to fetch workflows:', err)
    } finally {
      setLoading(false)
    }
  }

  const handleToggle = async (id: number, e: React.MouseEvent) => {
    e.preventDefault()
    try {
      await workflowApi.toggle(id)
      await fetchWorkflows()
    } catch (err) {
      alert(`切换失败: ${err instanceof Error ? err.message : 'Unknown error'}`)
      console.error('Failed to toggle workflow:', err)
    }
  }

  const handleTrigger = async (id: number, e: React.MouseEvent) => {
    e.preventDefault()
    if (!confirm('确定要立即执行此工作流吗?')) return

    try {
      await workflowApi.trigger(id)
      alert('工作流已触发')
      fetchWorkflows()
    } catch (err) {
      alert(`执行失败: ${err instanceof Error ? err.message : 'Unknown error'}`)
      console.error('Failed to trigger workflow:', err)
    }
  }

  const handleEdit = (id: number, e: React.MouseEvent) => {
    e.preventDefault()
    window.location.href = `/workflows/${id}`
  }

  const handleDelete = async (id: number) => {
    if (!confirm('确定要删除这个工作流吗？')) return

    try {
      await workflowApi.delete(id)
      await fetchWorkflows()
    } catch (err) {
      alert(`删除失败: ${err instanceof Error ? err.message : 'Unknown error'}`)
      console.error('Failed to delete workflow:', err)
    }
  }

  const getStatusBadge = (status: boolean) => {
    return status ? (
      <span className="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-green-100 text-green-800">
        启用中
      </span>
    ) : (
      <span className="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-gray-100 text-gray-800">
        已禁用
      </span>
    )
  }

  const getScopeTypeLabel = (workflow: Workflow) => {
    let label = ''
    switch (workflow.scope_type) {
      case 'specific_podcasts':
        label = '指定节目'
        break
      case 'all_subscribed':
        label = '全部订阅'
        break
      case 'custom_sources':
        label = '自定义源'
        break
      default:
        label = workflow.scope_type
    }

    // 如果有统计信息且有节目数，添加节目数
    if (workflow.stats && workflow.stats.podcast_count !== undefined && workflow.stats.podcast_count > 0) {
      label += `（${workflow.stats.podcast_count}）`
    }

    return label
  }

  const formatTimeRange = (timeRange?: number) => {
    if (!timeRange || timeRange === 0) return '不限制'
    return `最近${timeRange}天`
  }

  const formatDateTime = (dateStr?: string) => {
    if (!dateStr) return '-'
    const date = new Date(dateStr)
    return date.toLocaleString('zh-CN', {
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit'
    })
  }

  return (
    <main className="min-h-screen bg-slate-50">
      <div className="container mx-auto px-4 py-8">
        {/* Header */}
        <div className="mb-8">
          <div className="mb-8">
            <div className="flex items-center justify-between mb-6">
              {/* 返回首页按钮 */}
              <Link
                href="/"
                className="w-36 h-11 px-4 bg-white text-slate-800 font-medium rounded-xl border border-slate-300 hover:bg-slate-50 hover:border-slate-400 transition-colors flex items-center justify-center gap-2"
              >
                <span>←</span>
                <span>返回首页</span>
              </Link>

              {/* 右侧按钮组 */}
              <div className="flex items-center gap-3">
                {/* 创建工作流按钮 - 突出显示 */}
                <button
                  onClick={() => setShowCreateModal(true)}
                  className="w-36 h-11 border-2 border-blue-600 rounded-xl bg-blue-600 text-white text-sm font-medium hover:bg-blue-700 hover:border-blue-700 transition-colors relative"
                >
                  <span className="absolute left-0 top-1/2 -translate-y-1/2 pl-3 text-white text-lg pointer-events-none">+</span>
                  <span className="w-full text-center">创建工作流</span>
                </button>
              </div>
            </div>

            {/* 标题和描述 */}
            <div className="mb-4">
              <h1 className="text-4xl md:text-5xl font-semibold text-slate-800 mb-2">
                工作流管理
              </h1>
              <p className="text-base text-slate-600 max-w-2xl">
                管理和监控自动化单集抓取任务
              </p>
            </div>
          </div>
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

        {/* Empty State */}
        {!loading && !error && workflows.length === 0 && (
          <div className="bg-white rounded-lg p-12 text-center shadow-sm">
            <div className="text-6xl mb-4">⚙️</div>
            <p className="text-slate-600 text-lg">暂无工作流</p>
            <p className="text-slate-5000 text-sm mt-2">
              点击上方按钮创建你的第一个工作流
            </p>
          </div>
        )}

        {/* Workflows List */}
        {!loading && !error && workflows.length > 0 && (
          <div className="space-y-4">
            {workflows.map((workflow) => (
              <Link
                key={workflow.id}
                href={`/workflows/${workflow.id}`}
                className="block bg-white rounded-lg shadow-sm hover:shadow-md transition-shadow p-8"
              >
                <div className="flex items-start justify-between">
                  <div className="flex-1">
                    <div className="flex items-center gap-3 mb-3">
                      <h3 className="text-2xl font-semibold text-slate-900">
                        {workflow.id}: {workflow.name}
                      </h3>
                      {getStatusBadge(workflow.is_enabled)}
                    </div>

                    {workflow.description && (
                      <p className="text-slate-600 text-base mb-6">
                        {workflow.description}
                      </p>
                    )}

                    <div className="flex flex-wrap gap-x-8 gap-y-4 text-base text-slate-600 mt-6">
                      <div className="flex items-center gap-2">
                        <span className="font-medium">范围:</span>
                        <span>{getScopeTypeLabel(workflow)}</span>
                      </div>

                      <div className="flex items-center gap-2">
                        <span className="font-medium">时间范围:</span>
                        <span>{formatTimeRange(workflow.rules_config?.time_range)}</span>
                      </div>

                      <div className="flex items-center gap-2">
                        <span className="font-medium">定时:</span>
                        <code className="px-2 py-0.5 bg-slate-100 rounded text-xs">
                          {workflow.schedule}
                        </code>
                      </div>

                      {workflow.stats && (
                        <>
                          <div className="flex items-center gap-2">
                            <span className="font-medium">创建单集:</span>
                            <span className="text-blue-600 font-medium">{workflow.stats.total_episodes}</span>
                          </div>

                          <div className="flex items-center gap-2">
                            <span className="font-medium">上次执行:</span>
                            <span>{formatDateTime(workflow.stats.last_execution)}</span>
                          </div>

                          <div className="flex items-center gap-2">
                            <span className="font-medium">下次执行:</span>
                            <span>{formatDateTime(workflow.stats.next_execution)}</span>
                          </div>

                          <div className="flex items-center gap-2">
                            <span className="font-medium">执行次数:</span>
                            <span>{workflow.stats.total_jobs}</span>
                          </div>

                          <div className="flex items-center gap-2">
                            <span className="font-medium">成功率:</span>
                            <span className={workflow.stats.success_rate >= 80 ? 'text-green-600' : 'text-yellow-600'}>
                              {workflow.stats.success_rate.toFixed(1)}%
                            </span>
                          </div>
                        </>
                      )}
                    </div>
                  </div>

                  {/* Actions */}
                  <div className="flex items-center gap-2 ml-4">
                    {/* 手动执行 */}
                    <button
                      onClick={(e) => handleTrigger(workflow.id, e)}
                      className="p-2.5 text-blue-600 dark:text-blue-400 border border-slate-200 dark:border-slate-600 rounded-lg hover:bg-slate-50 dark:hover:bg-slate-700 hover:border-blue-300 dark:hover:border-blue-500 transition-all"
                      title="手动执行"
                    >
                      <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2.5} d="M13 10V3L4 14h7v7l9-11h-7z" />
                      </svg>
                    </button>

                    {/* 启用/停用 */}
                    <button
                      onClick={(e) => handleToggle(workflow.id, e)}
                      className={`p-2.5 border border-slate-200 dark:border-slate-600 rounded-lg hover:bg-slate-50 dark:hover:bg-slate-700 hover:border-slate-300 dark:hover:border-slate-500 transition-all ${
                        workflow.is_enabled ? 'text-amber-600 dark:text-amber-400' : 'text-green-600 dark:text-green-400'
                      }`}
                      title={workflow.is_enabled ? '停用' : '启用'}
                    >
                      {workflow.is_enabled ? (
                        <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2.5} d="M10 9v6m4-6v6m7-3a9 9 0 11-18 0 9 9 0 0118 0z" />
                        </svg>
                      ) : (
                        <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2.5} d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
                        </svg>
                      )}
                    </button>

                    {/* 编辑 */}
                    <button
                      onClick={(e) => handleEdit(workflow.id, e)}
                      className="p-2.5 text-slate-800 dark:text-slate-200 border border-slate-200 dark:border-slate-600 rounded-lg hover:bg-slate-50 dark:hover:bg-slate-700 hover:border-slate-300 dark:hover:border-slate-500 transition-all"
                      title="编辑"
                    >
                      <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2.5} d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2h2.828l8.586-8.586z" />
                      </svg>
                    </button>

                    {/* 删除 */}
                    <button
                      onClick={(e) => {
                        e.preventDefault()
                        handleDelete(workflow.id)
                      }}
                      className="p-2.5 text-red-600 dark:text-red-400 border border-slate-200 dark:border-slate-600 rounded-lg hover:bg-red-50 dark:hover:bg-red-900/20 hover:border-red-300 dark:hover:border-red-500 transition-all"
                      title="删除"
                    >
                      <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2.5} d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
                      </svg>
                    </button>
                  </div>
                </div>
              </Link>
            ))}
          </div>
        )}
      </div>

      {/* Create Workflow Modal */}
      <WorkflowFormModal
        isOpen={showCreateModal}
        onClose={() => setShowCreateModal(false)}
        onSuccess={() => {
          fetchWorkflows()
          setShowCreateModal(false)
        }}
      />
    </main>
  )
}
