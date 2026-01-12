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

  const handleToggle = async (id: number) => {
    try {
      await workflowApi.toggle(id)
      await fetchWorkflows()
    } catch (err) {
      alert(`切换失败: ${err instanceof Error ? err.message : 'Unknown error'}`)
      console.error('Failed to toggle workflow:', err)
    }
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
      <span className="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200">
        启用中
      </span>
    ) : (
      <span className="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-gray-100 text-gray-800 dark:bg-gray-800 dark:text-gray-300">
        已禁用
      </span>
    )
  }

  const getScopeTypeLabel = (scopeType: string) => {
    switch (scopeType) {
      case 'specific_podcasts':
        return '指定节目'
      case 'all_subscribed':
        return '全部订阅'
      case 'custom_sources':
        return '自定义源'
      default:
        return scopeType
    }
  }

  return (
    <main className="min-h-screen bg-slate-50 dark:bg-slate-900">
      <div className="container mx-auto px-4 py-8">
        {/* Header */}
        <div className="mb-8">
          <div className="flex items-center justify-between mb-4">
            <Link
              href="/"
              className="text-blue-600 hover:text-blue-700 dark:text-blue-400 dark:hover:text-blue-300"
            >
              ← 返回首页
            </Link>
          </div>
          <div className="flex items-center justify-between">
            <div>
              <h1 className="text-3xl font-bold text-slate-900 dark:text-slate-50">
                工作流管理
              </h1>
              <p className="mt-2 text-slate-600 dark:text-slate-400">
                管理和监控自动化单集抓取任务
              </p>
            </div>
            <button
              onClick={() => setShowCreateModal(true)}
              className="px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 transition-colors"
            >
              + 创建工作流
            </button>
          </div>
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

        {/* Empty State */}
        {!loading && !error && workflows.length === 0 && (
          <div className="bg-white dark:bg-slate-800 rounded-lg p-12 text-center shadow-sm">
            <div className="text-6xl mb-4">⚙️</div>
            <p className="text-slate-600 dark:text-slate-400 text-lg">暂无工作流</p>
            <p className="text-slate-500 dark:text-slate-500 text-sm mt-2">
              点击上方按钮创建你的第一个工作流
            </p>
          </div>
        )}

        {/* Workflows List */}
        {!loading && !error && workflows.length > 0 && (
          <div className="space-y-4">
            {workflows.map((workflow) => (
              <div
                key={workflow.id}
                className="bg-white dark:bg-slate-800 rounded-lg shadow-sm hover:shadow-md transition-shadow p-6"
              >
                <div className="flex items-start justify-between">
                  <div className="flex-1">
                    <div className="flex items-center gap-3 mb-2">
                      <h3 className="text-xl font-semibold text-slate-900 dark:text-slate-50">
                        {workflow.name}
                      </h3>
                      {getStatusBadge(workflow.is_enabled)}
                    </div>

                    {workflow.description && (
                      <p className="text-slate-600 dark:text-slate-400 mb-3">
                        {workflow.description}
                      </p>
                    )}

                    <div className="flex flex-wrap gap-4 text-sm text-slate-600 dark:text-slate-400">
                      <div className="flex items-center gap-1">
                        <span className="font-medium">范围:</span>
                        <span>{getScopeTypeLabel(workflow.scope_type)}</span>
                      </div>

                      <div className="flex items-center gap-1">
                        <span className="font-medium">定时:</span>
                        <code className="px-2 py-0.5 bg-slate-100 dark:bg-slate-700 rounded text-xs">
                          {workflow.schedule}
                        </code>
                      </div>

                      {workflow.stats && (
                        <>
                          <div className="flex items-center gap-1">
                            <span className="font-medium">执行次数:</span>
                            <span>{workflow.stats.total_jobs}</span>
                          </div>

                          <div className="flex items-center gap-1">
                            <span className="font-medium">成功率:</span>
                            <span className={workflow.stats.success_rate >= 80 ? 'text-green-600' : 'text-yellow-600'}>
                              {workflow.stats.success_rate.toFixed(1)}%
                            </span>
                          </div>

                          {workflow.stats.last_execution && (
                            <div className="flex items-center gap-1">
                              <span className="font-medium">最后执行:</span>
                              <span>
                                {new Date(workflow.stats.last_execution).toLocaleString('zh-CN')}
                              </span>
                            </div>
                          )}
                        </>
                      )}
                    </div>
                  </div>

                  {/* Actions */}
                  <div className="flex items-center gap-2 ml-4">
                    <Link
                      href={`/workflows/${workflow.id}`}
                      className="px-3 py-1.5 text-sm text-blue-600 hover:text-blue-700 dark:text-blue-400 dark:hover:text-blue-300 border border-blue-600 dark:border-blue-400 rounded hover:bg-blue-50 dark:hover:bg-blue-900/20 transition-colors"
                    >
                      查看详情
                    </Link>
                    <button
                      onClick={() => handleToggle(workflow.id)}
                      className={`px-3 py-1.5 text-sm rounded transition-colors ${
                        workflow.is_enabled
                          ? 'text-yellow-600 border border-yellow-600 hover:bg-yellow-50 dark:text-yellow-400 dark:border-yellow-400 dark:hover:bg-yellow-900/20'
                          : 'text-green-600 border border-green-600 hover:bg-green-50 dark:text-green-400 dark:border-green-400 dark:hover:bg-green-900/20'
                      }`}
                    >
                      {workflow.is_enabled ? '禁用' : '启用'}
                    </button>
                    <button
                      onClick={() => handleDelete(workflow.id)}
                      className="px-3 py-1.5 text-sm text-red-600 border border-red-600 hover:bg-red-50 dark:text-red-400 dark:border-red-400 dark:hover:bg-red-900/20 rounded transition-colors"
                    >
                      删除
                    </button>
                  </div>
                </div>
              </div>
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
