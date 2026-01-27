'use client'

import { useState, useEffect } from 'react'
import { api } from '@/lib/api/client'
import MarkdownViewer from './MarkdownViewer'

interface ReportModalProps {
  isOpen: boolean
  onClose: () => void
  jobId: number
  jobStatus: string
}

interface Report {
  id: number
  job_id: number
  title: string
  content: string
  summary: string
  episodes_count: number
  podcasts_count: number
  matched_count: number        // 新增：匹配的单集数
  time_range_start: string     // 新增：时间范围起始
  time_range_end: string       // 新增：时间范围结束
  time_range_mode: string      // 新增：触发模式（daily | manual）
  generated_at: string
  format: string
  file_size: number

  // LLM相关字段
  llm_summary?: string         // LLM生成的摘要
  llm_model_used?: string      // 使用的模型名称
  llm_tokens_used?: number     // 使用的token数量
  llm_error?: string           // LLM错误信息（如果生成失败）
}

export default function ReportModal({ isOpen, onClose, jobId, jobStatus }: ReportModalProps) {
  const [report, setReport] = useState<Report | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (isOpen && jobId) {
      fetchReport()
    }
  }, [isOpen, jobId])

  const fetchReport = async () => {
    try {
      setLoading(true)
      setError(null)
      const response = await api.get<{ success: boolean; data: Report }>(`/api/v1/jobs/${jobId}/report`)
      setReport(response.data.data)
    } catch (err) {
      const errorMsg = err instanceof Error ? err.message : 'Unknown error'
      setError(errorMsg)
      console.error('Failed to fetch report:', err)
    } finally {
      setLoading(false)
    }
  }

  if (!isOpen) return null

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/50">
      <div className="bg-white dark:bg-slate-800 rounded-lg shadow-xl max-w-4xl w-full max-h-[90vh] overflow-hidden flex flex-col">
        {/* Header */}
        <div className="flex items-center justify-between p-6 border-b border-slate-200 dark:border-slate-700">
          <div className="flex-1">
            <div className="flex items-center gap-2">
              <h2 className="text-xl font-bold text-slate-900 dark:text-slate-50">
                执行报告
              </h2>
              {report?.llm_summary && (
                <span className="px-2 py-1 bg-purple-100 dark:bg-purple-900/30 text-purple-700 dark:text-purple-300 text-xs rounded-full">
                  🤖 AI增强
                </span>
              )}
            </div>
            {report && (
              <p className="text-sm text-slate-600 dark:text-slate-400 mt-1">
                {report.summary} • {new Date(report.generated_at).toLocaleString('zh-CN')}
              </p>
            )}
          </div>
          <button
            onClick={onClose}
            className="p-2 hover:bg-slate-100 dark:hover:bg-slate-700 rounded-lg transition-colors"
            aria-label="关闭"
          >
            <svg className="w-5 h-5 text-slate-600 dark:text-slate-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>

        {/* Content */}
        <div className="flex-1 overflow-y-auto p-6">
          {loading ? (
            <div className="flex items-center justify-center py-12">
              <div className="text-center">
                <div className="inline-block animate-spin rounded-full h-8 w-8 border-b-2 border-blue-600 mb-3"></div>
                <p className="text-sm text-slate-600 dark:text-slate-400">
                  {jobStatus === 'running' ? '任务执行中，报告生成中...' : '加载报告中...'}
                </p>
              </div>
            </div>
          ) : error ? (
            <div className="bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg p-6">
              <div className="flex items-start gap-3">
                <svg className="w-5 h-5 text-red-600 dark:text-red-400 mt-0.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
                </svg>
                <div className="flex-1">
                  <h3 className="text-red-800 dark:text-red-200 font-semibold mb-1">报告加载失败</h3>
                  <p className="text-sm text-red-600 dark:text-red-400 mb-2">{error}</p>
                  <p className="text-xs text-red-500 dark:text-red-500">
                    可能原因：报告尚未生成、任务执行中，或报告生成失败
                  </p>
                  <button
                    onClick={fetchReport}
                    className="mt-3 px-3 py-1.5 bg-red-600 hover:bg-red-700 text-white text-sm rounded-md transition-colors"
                  >
                    重试
                  </button>
                </div>
              </div>
            </div>
          ) : report ? (
            <>
              {/* LLM错误提示 */}
              {report.llm_error && (
                <div className="mb-4 bg-yellow-50 dark:bg-yellow-900/20 border border-yellow-200 dark:border-yellow-800 rounded-lg p-4">
                  <div className="flex items-start gap-3">
                    <svg className="w-5 h-5 text-yellow-600 dark:text-yellow-400 mt-0.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
                    </svg>
                    <div className="flex-1">
                      <h3 className="text-yellow-800 dark:text-yellow-200 font-semibold text-sm mb-1">AI摘要生成失败</h3>
                      <p className="text-xs text-yellow-700 dark:text-yellow-300">{report.llm_error}</p>
                    </div>
                  </div>
                </div>
              )}

              <MarkdownViewer content={report.content} />
            </>
          ) : null}
        </div>

        {/* Footer */}
        <div className="flex items-center justify-between p-4 border-t border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-900">
          {report && (
            <div className="text-sm text-slate-600 dark:text-slate-400">
              <div className="flex items-center gap-2 mb-1">
                <span className="font-medium">{report.podcasts_count}</span> 个节目 •
                <span className="font-medium">{report.matched_count}</span> 个匹配单集 •
                <span className="font-medium">{(report.file_size / 1024).toFixed(1)} KB</span>
                {report.llm_tokens_used && (
                  <>
                    • <span className="font-medium text-purple-600 dark:text-purple-400">{report.llm_tokens_used}</span> tokens
                  </>
                )}
              </div>
              <div className="flex items-center gap-2 text-xs">
                <span className={`px-1.5 py-0.5 rounded ${
                  report.time_range_mode === 'daily'
                    ? 'bg-blue-100 dark:bg-blue-900/30 text-blue-700 dark:text-blue-300'
                    : 'bg-green-100 dark:bg-green-900/30 text-green-700 dark:text-green-300'
                }`}>
                  {report.time_range_mode === 'daily' ? '自动定时' : '手动触发'}
                </span>
                {report.llm_model_used && (
                  <span className="px-1.5 py-0.5 bg-purple-100 dark:bg-purple-900/30 text-purple-700 dark:text-purple-300 rounded">
                    {report.llm_model_used}
                  </span>
                )}
                <span className="text-slate-500 dark:text-slate-500">
                  {new Date(report.time_range_start).toLocaleString('zh-CN')} → {new Date(report.time_range_end).toLocaleString('zh-CN')}
                </span>
              </div>
            </div>
          )}
          <div className="flex gap-2">
            {report && (
              <button
                onClick={() => {
                  const blob = new Blob([report.content], { type: 'text/markdown' })
                  const url = URL.createObjectURL(blob)
                  const a = document.createElement('a')
                  a.href = url
                  a.download = `report-${report.job_id}-${new Date(report.generated_at).toISOString().slice(0, 10)}.md`
                  document.body.appendChild(a)
                  a.click()
                  document.body.removeChild(a)
                  URL.revokeObjectURL(url)
                }}
                className="px-4 py-2 bg-blue-600 hover:bg-blue-700 text-white rounded-lg text-sm font-medium transition-colors flex items-center gap-2"
              >
                <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4" />
                </svg>
                下载 Markdown
              </button>
            )}
            <button
              onClick={onClose}
              className="px-4 py-2 bg-slate-200 dark:bg-slate-700 hover:bg-slate-300 dark:hover:bg-slate-600 text-slate-700 dark:text-slate-300 rounded-lg text-sm font-medium transition-colors"
            >
              关闭
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}
