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
  generated_at: string
  format: string
  file_size: number
  // LLM相关字段
  llm_summary?: string
  llm_model_used?: string
  llm_tokens_used?: number
  llm_error?: string
}

// 格式化token数量
const formatTokenCount = (tokens: number): string => {
  if (tokens === 0) return "0";
  if (tokens < 1000) return tokens.toString();
  if (tokens < 1000000) return `${(tokens / 1000).toFixed(1)}K`;
  return `${(tokens / 1000000).toFixed(1)}M`;
};

export default function ReportModal({ isOpen, onClose, jobId, jobStatus }: ReportModalProps) {
  const [report, setReport] = useState<Report | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [regenerating, setRegenerating] = useState(false)

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

  const regenerateLLMSummary = async () => {
    if (!report || regenerating) return

    try {
      setRegenerating(true)
      const response = await api.post<{ success: boolean; data: Report; message: string }>(
        `/api/v1/jobs/${jobId}/regenerate-llm`
      )

      if (response.data.success) {
        setReport(response.data.data)
        // Show success message
        alert('AI摘要重新生成成功！')
      }
    } catch (err) {
      const errorMsg = err instanceof Error ? err.message : 'Unknown error'
      alert(`重新生成失败: ${errorMsg}`)
      console.error('Failed to regenerate LLM summary:', err)
    } finally {
      setRegenerating(false)
    }
  }

  if (!isOpen) return null

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/50">
      <div className="bg-white dark:bg-slate-800 rounded-lg shadow-xl max-w-4xl w-full max-h-[90vh] overflow-hidden flex flex-col">
        {/* Header */}
        <div className="flex items-center justify-between p-6 border-b border-slate-200 dark:border-slate-700">
          <div>
            <h2 className="text-xl font-bold text-slate-900 dark:text-slate-50">
              执行报告
            </h2>
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
                <div className="mb-4 bg-amber-50 dark:bg-amber-900/20 border border-amber-200 dark:border-amber-800 rounded-lg p-4">
                  <div className="flex items-start gap-3">
                    <svg className="w-5 h-5 text-amber-600 dark:text-amber-400 flex-shrink-0 mt-0.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 9v2m0 4h.01M15 19a9 9 0 11-18 0 9 9 0 0118 0z" />
                    </svg>
                    <div className="flex-1">
                      <h4 className="text-amber-800 dark:text-amber-200 font-semibold mb-1">AI摘要生成失败</h4>
                      <p className="text-sm text-amber-700 dark:text-amber-300 font-mono">{report.llm_error}</p>
                      <p className="text-xs text-amber-600 dark:text-amber-400 mt-1">
                        报告内容已生成，但AI智能摘要未能成功生成。这可能是由于LLM服务不可用或配置错误导致。
                      </p>
                      <button
                        onClick={regenerateLLMSummary}
                        disabled={regenerating}
                        className="mt-3 px-4 py-2 bg-white dark:bg-slate-700 border border-slate-300 dark:border-slate-600 hover:bg-slate-50 dark:hover:bg-slate-800 disabled:opacity-50 disabled:cursor-not-allowed text-slate-700 dark:text-slate-300 text-sm rounded transition-all flex items-center gap-2"
                      >
                        {regenerating ? (
                          <>
                            <div className="inline-block animate-spin rounded-full h-4 w-4 border-b-2 border-slate-400"></div>
                            <span>重新生成中...</span>
                          </>
                        ) : (
                          <>
                            <svg className="w-4 h-4 text-slate-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H14m0 0L9 14m0 0l-2-2m0 0L9 18m0 0a8.001 8.001 0 01-15.356-2m15.357 2H15" />
                            </svg>
                            <span>重新生成摘要</span>
                          </>
                        )}
                      </button>
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
            <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-sm text-slate-600 dark:text-slate-400">
              <div className="flex items-center gap-1">
                <span className="font-medium">{report.podcasts_count}</span> 个节目 •
                <span className="font-medium">{report.episodes_count}</span> 个单集 •
                <span className="font-medium">{(report.file_size / 1024).toFixed(1)} KB</span>
              </div>
              {/* LLM统计信息 */}
              {report.llm_tokens_used && report.llm_model_used && (
                <div className="flex items-center gap-2 px-3 py-1.5 bg-purple-100 dark:bg-purple-900/30 rounded-lg">
                  <span className="text-purple-800 dark:text-purple-300">🤖 AI: {formatTokenCount(report.llm_tokens_used)} ({report.llm_model_used})</span>
                </div>
              )}
              {report.llm_error && (
                <div className="flex items-center gap-2 px-3 py-1.5 bg-red-100 dark:bg-red-900/20 rounded-lg">
                  <span className="text-red-800 dark:text-red-300">⚠️ AI摘要失败</span>
                </div>
              )}
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
              className="px-4 py-2 bg-white dark:bg-slate-700 hover:bg-slate-50 dark:hover:bg-slate-600 text-slate-700 dark:text-slate-300 border border-slate-300 dark:border-slate-600 rounded-lg text-sm font-medium transition-colors"
            >
              关闭
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}
