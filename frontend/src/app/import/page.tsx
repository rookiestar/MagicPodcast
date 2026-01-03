'use client'

import { useState, useEffect, useRef } from 'react'
import { syncApi } from '@/lib/api'

interface LogEntry {
  id: string
  type: 'info' | 'success' | 'error' | 'progress' |
       'skip_paid' | 'skip_cert' | 'skip_not_found' |
       'skip_access_denied' | 'skip_geo_blocked' |
       'skip_duplicate' | 'skip_invalid' | 'skip_other'
  message: string
  timestamp: string
  current?: number
  total?: number
  reason?: string // 跳过原因
}

export default function ImportPage() {
  const [file, setFile] = useState<File | null>(null)
  const [importing, setImporting] = useState(false)
  const [logs, setLogs] = useState<LogEntry[]>([])
  const [filter, setFilter] = useState<'all' | 'errors' | 'success' | 'skips'>('all')
  const [autoScroll, setAutoScroll] = useState(true)
  const [debugInfo, setDebugInfo] = useState<string[]>([])  // 调试信息
  const [result, setResult] = useState<{
    success: boolean
    message: string
    total_podcasts: number
    success_count: number
    failed_count: number
    errors?: string[]
  } | null>(null)
  const logContainerRef = useRef<HTMLDivElement>(null)
  const logEndRef = useRef<HTMLDivElement>(null)

  // 自动滚动到底部（只在添加新日志时触发）
  useEffect(() => {
    // 如果自动滚动被禁用，则完全不滚动
    if (!autoScroll) {
      return
    }

    // 使用requestAnimationFrame确保DOM更新后再滚动
    requestAnimationFrame(() => {
      if (logEndRef.current && autoScroll) {
        logEndRef.current.scrollIntoView({ behavior: 'auto', block: 'end' })
      }
    })
  }, [logs, autoScroll])

  // 监听滚动事件 - 任何手动滚动都禁用自动滚动
  useEffect(() => {
    const container = logContainerRef.current
    if (!container) return

    let isAutoScrolling = false // 标记是否是自动滚动

    const handleScroll = () => {
      // 如果是自动滚动触发的，忽略
      if (isAutoScrolling) {
        return
      }

      // 任何手动滚动都禁用自动滚动
      if (autoScroll) {
        setAutoScroll(false)
      }
    }

    // 监听滚动开始，区分自动滚动和手动滚动
    const detectScrollStart = () => {
      if (!autoScroll) return

      // 延迟检查，如果是自动滚动，scrollTop会很快变化
      setTimeout(() => {
        const { scrollTop, scrollHeight, clientHeight } = container
        const isAtBottom = scrollHeight - scrollTop <= clientHeight + 50

        // 如果不在底部，说明是用户手动滚动
        if (!isAtBottom) {
          setAutoScroll(false)
        }
      }, 100)
    }

    container.addEventListener('scroll', handleScroll, { passive: true })

    return () => {
      container.removeEventListener('scroll', handleScroll)
    }
  }, [autoScroll])

  // 恢复自动滚动
  const handleResumeAutoScroll = () => {
    setAutoScroll(true)
    // 立即滚动到底部
    requestAnimationFrame(() => {
      if (logEndRef.current) {
        logEndRef.current.scrollIntoView({ behavior: 'smooth', block: 'end' })
      }
    })
  }

  // 过滤后的日志
  const filteredLogs = logs.filter(log => {
    if (filter === 'all') return true
    if (filter === 'errors') return log.type === 'error'
    if (filter === 'success') return log.type === 'success'
    if (filter === 'skips') return log.type.startsWith('skip_')
    return true
  })

  // 统计信息 - 始终基于所有日志
  const stats = {
    total: logs.filter(l =>
      l.type === 'success' ||
      l.type === 'error' ||
      l.type.startsWith('skip_')
    ).length,  // 只统计有意义的日志（成功、错误、跳过）
    errors: logs.filter(l => l.type === 'error').length,
    success: logs.filter(l => l.type === 'success').length,
    skips: logs.filter(l => l.type.startsWith('skip_')).length,
    skipPaid: logs.filter(l => l.type === 'skip_paid').length,
    skipCert: logs.filter(l => l.type === 'skip_cert').length,
    skipNotFound: logs.filter(l => l.type === 'skip_not_found').length,
    skipAccess: logs.filter(l => l.type === 'skip_access_denied').length,
    skipGeo: logs.filter(l => l.type === 'skip_geo_blocked').length,
    skipOther: logs.filter(l => l.type === 'skip_other' || l.type === 'skip_duplicate' || l.type === 'skip_invalid').length,
  }

  const addLog = (type: 'info' | 'success' | 'error' | 'progress', message: string, current?: number, total?: number) => {
    const newLog: LogEntry = {
      id: Date.now() + Math.random().toString(),
      type,
      message,
      timestamp: new Date().toLocaleTimeString(),
      current,
      total,
    }
    setLogs(prev => [...prev, newLog])
  }

  const handleFileChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const selectedFile = e.target.files?.[0]
    if (selectedFile) {
      // 验证文件类型
      const validTypes = ['application/xml', 'text/xml', 'text/opml', 'text/x-opml']
      const fileExt = selectedFile.name.split('.').pop()?.toLowerCase()

      if (!validTypes.includes(selectedFile.type) && !['opml', 'xml'].includes(fileExt || '')) {
        alert('请选择OPML或XML文件')
        return
      }

      setFile(selectedFile)
      setResult(null)
      setLogs([])
    }
  }

  const handleImport = async () => {
    if (!file) {
      alert('请先选择OPML文件')
      return
    }

    setImporting(true)
    setResult(null)
    setLogs([])
    setDebugInfo([])

    const addDebug = (msg: string) => {
      const timestamp = new Date().toLocaleTimeString()
      setDebugInfo(prev => [...prev, `[${timestamp}] ${msg}`])
      console.log('[Debug]', msg)
    }

    addLog('info', '开始导入...')
    addDebug(`开始导入: ${file.name} (${(file.size / 1024).toFixed(2)} KB)`)

    try {
      await syncApi.importOPMLSSE(file, (type, message, current, total) => {
        addLog(type as any, message, current, total)

        // 每10条消息记录一次调试信息
        if (logs.length % 10 === 0) {
          addDebug(`已处理 ${logs.length} 条日志，最新类型: ${type}`)
        }
      })

      addLog('success', '导入完成！')
      addDebug('导入成功完成')
    } catch (error: any) {
      console.error('导入失败:', error)
      addDebug(`捕获错误: ${error.message}`)
      addDebug(`错误名称: ${error.name}`)
      addDebug(`错误堆栈: ${error.stack}`)

      // 区分不同类型的错误
      if (error.message?.includes('超时')) {
        addLog('error', '导入超时：可能是网络较慢或文件太大')
        addLog('info', '提示：您可以重新导入，系统会自动跳过已导入的播客')
      } else if (error.message?.includes('Network') || error.message?.includes('fetch')) {
        addLog('error', '网络连接错误：' + (error.message || '未知错误'))
        addLog('info', '提示：请检查网络连接后重试')
        addDebug('网络错误详情：' + JSON.stringify(error))
      } else if (error.message?.includes('abort') || error.message?.includes('取消')) {
        addLog('error', '导入被取消')
      } else {
        addLog('error', '导入失败：' + (error.message || '未知错误'))
        addLog('info', '提示：部分播客可能已成功导入，您可以查看播客列表')
      }
    } finally {
      setImporting(false)
    }
  }

  const getLogIcon = (type: LogEntry['type']) => {
    switch (type) {
      case 'success':
        return '✅'
      case 'error':
        return '❌'
      case 'progress':
        return '⏳'
      case 'skip_paid':
        return '💰'
      case 'skip_cert':
        return '🔐'
      case 'skip_not_found':
        return '🔍'
      case 'skip_access_denied':
        return '🚫'
      case 'skip_geo_blocked':
        return '🌍'
      case 'skip_duplicate':
        return '🔄'
      case 'skip_invalid':
        return '📄'
      case 'skip_other':
        return '⏭️'
      default:
        return 'ℹ️'
    }
  }

  const getLogColor = (type: LogEntry['type']) => {
    switch (type) {
      case 'success':
        return 'text-green-700'
      case 'error':
        return 'text-red-700'
      case 'progress':
        return 'text-blue-700'
      case 'skip_paid':
        return 'text-yellow-700'
      case 'skip_cert':
        return 'text-orange-700'
      case 'skip_not_found':
        return 'text-gray-600'
      case 'skip_access_denied':
        return 'text-red-600'
      case 'skip_geo_blocked':
        return 'text-purple-700'
      case 'skip_duplicate':
        return 'text-cyan-700'
      case 'skip_invalid':
        return 'text-indigo-700'
      case 'skip_other':
        return 'text-gray-500'
      default:
        return 'text-gray-700'
    }
  }

  const getLogTypeLabel = (type: LogEntry['type']) => {
    switch (type) {
      case 'skip_paid':
        return '付费播客'
      case 'skip_cert':
        return '证书过期'
      case 'skip_not_found':
        return '不存在'
      case 'skip_access_denied':
        return '访问拒绝'
      case 'skip_geo_blocked':
        return '地区限制'
      case 'skip_duplicate':
        return '重复'
      case 'skip_invalid':
        return '格式无效'
      case 'skip_other':
        return '其他'
      default:
        return ''
    }
  }

  return (
    <div className="min-h-screen bg-gray-50 py-12 px-4 sm:px-6 lg:px-8">
      <div className="max-w-4xl mx-auto">
        <div className="bg-white shadow rounded-lg">
          <div className="px-6 py-4 border-b border-gray-200">
            <h1 className="text-2xl font-bold text-gray-900">导入OPML文件</h1>
            <p className="mt-1 text-sm text-gray-500">
              从其他播客应用导出您的订阅列表
            </p>
          </div>

          <div className="px-6 py-6">
            {/* 说明部分 */}
            <div className="mb-6 p-4 bg-blue-50 border border-blue-200 rounded-md">
              <h3 className="text-sm font-medium text-blue-900 mb-2">
                如何导出OPML文件？
              </h3>
              <ul className="text-xs text-blue-800 space-y-1 list-disc list-inside">
                <li><strong>Apple Podcasts</strong>: 文件 → 库 → 导出播放列表 → 选择OPML格式</li>
                <li><strong>Overcast</strong>: 设置 → 导出数据 → OPML</li>
                <li><strong>Pocket Casts</strong>: 个人资料 → 设置 → 导出播客</li>
                <li><strong>其他应用</strong>: 查找"导出OPML"或"导出订阅"选项</li>
              </ul>
            </div>

            {/* 文件上传 */}
            <div className="mb-6">
              <label className="block text-sm font-medium text-gray-700 mb-2">
                选择OPML文件
              </label>
              <input
                type="file"
                accept=".opml,.xml"
                onChange={handleFileChange}
                disabled={importing}
                className="block w-full text-sm text-gray-500
                  file:mr-4 file:py-2 file:px-4
                  file:rounded-md file:border-0
                  file:text-sm file:font-semibold
                  file:bg-blue-50 file:text-blue-700
                  hover:file:bg-blue-100
                  disabled:file:bg-gray-100 disabled:file:text-gray-400
                "
              />
              {file && (
                <p className="mt-2 text-sm text-gray-600">
                  已选择: {file.name} ({(file.size / 1024).toFixed(2)} KB)
                </p>
              )}
            </div>

            {/* 导入按钮 */}
            <div className="flex justify-between items-center mb-6">
              <button
                onClick={handleImport}
                disabled={!file || importing}
                className={`px-4 py-2 border border-transparent rounded-md shadow-sm text-sm font-medium text-white
                  ${!file || importing
                    ? 'bg-gray-300 cursor-not-allowed'
                    : 'bg-blue-600 hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-blue-500'
                  }`}
              >
                {importing ? '导入中...' : '开始导入'}
              </button>

              {/* 清空调试信息按钮 */}
              {debugInfo.length > 0 && !importing && (
                <button
                  onClick={() => setDebugInfo([])}
                  className="text-xs text-gray-500 hover:text-gray-700"
                >
                  清空调试信息
                </button>
              )}
            </div>

            {/* 调试信息显示 */}
            {debugInfo.length > 0 && (
              <div className="mb-4 p-3 bg-yellow-50 border border-yellow-200 rounded">
                <h4 className="text-xs font-medium text-yellow-900 mb-2">调试信息</h4>
                <div className="text-xs font-mono text-yellow-800 space-y-1 max-h-40 overflow-y-auto">
                  {debugInfo.slice(-10).map((info, index) => (
                    <div key={index}>{info}</div>
                  ))}
                  {debugInfo.length > 10 && (
                    <div className="text-gray-500">... ({debugInfo.length - 10} 条更多信息)</div>
                  )}
                </div>
              </div>
            )}

            {/* 实时日志 */}
            {logs.length > 0 && (
              <div className="border border-gray-300 rounded-lg p-4 bg-gray-50">
                <div className="flex items-center justify-between mb-3">
                  <div className="flex items-center space-x-3">
                    <h3 className="text-sm font-medium text-gray-900">
                      导入日志
                      {importing && (
                        <span className="ml-2 inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-blue-100 text-blue-800">
                          进行中
                        </span>
                      )}
                    </h3>

                    {/* 过滤器按钮组 */}
                    <div className="flex items-center space-x-1">
                      <button
                        onClick={() => setFilter('all')}
                        className={`text-xs px-3 py-1 rounded border transition-colors min-w-[100px] ${
                          filter === 'all'
                            ? 'bg-blue-50 border-blue-500 text-blue-700'
                            : 'bg-white border-gray-300 text-gray-700 hover:bg-gray-50'
                        }`}
                      >
                        全部 ({stats.total})
                      </button>
                      <button
                        onClick={() => setFilter('errors')}
                        className={`text-xs px-3 py-1 rounded border transition-colors min-w-[100px] ${
                          filter === 'errors'
                            ? 'bg-red-50 border-red-500 text-red-700'
                            : 'bg-white border-gray-300 text-gray-700 hover:bg-gray-50'
                        }`}
                      >
                        错误 ({stats.errors})
                      </button>
                      <button
                        onClick={() => setFilter('success')}
                        className={`text-xs px-3 py-1 rounded border transition-colors min-w-[100px] ${
                          filter === 'success'
                            ? 'bg-green-50 border-green-500 text-green-700'
                            : 'bg-white border-gray-300 text-gray-700 hover:bg-gray-50'
                        }`}
                      >
                        成功 ({stats.success})
                      </button>
                      <button
                        onClick={() => setFilter('skips')}
                        className={`text-xs px-3 py-1 rounded border transition-colors min-w-[100px] ${
                          filter === 'skips'
                            ? 'bg-gray-50 border-gray-500 text-gray-700'
                            : 'bg-white border-gray-300 text-gray-700 hover:bg-gray-50'
                        }`}
                      >
                        跳过 ({stats.skips})
                      </button>
                    </div>

                    {/* 自动滚动指示器 */}
                    {!autoScroll && (
                      <button
                        onClick={handleResumeAutoScroll}
                        className="text-xs text-blue-600 hover:text-blue-800"
                        title="恢复自动滚动"
                      >
                        ↺ 恢复自动滚动
                      </button>
                    )}
                  </div>

                  <div className="flex items-center space-x-2">
                    {!importing && (
                      <button
                        onClick={() => {
                          setLogs([])
                          setFilter('all')
                          setAutoScroll(true)
                        }}
                        className="text-xs text-gray-500 hover:text-gray-700"
                      >
                        清空日志
                      </button>
                    )}
                  </div>
                </div>

                {/* 统计信息 */}
                {(stats.errors > 0 || stats.success > 0 || stats.skips > 0) && (
                  <div className="mb-3 text-xs text-gray-600">
                    {stats.success > 0 && <span className="text-green-600 mr-3">✅ {stats.success} 个成功</span>}
                    {stats.errors > 0 && <span className="text-red-600 mr-3">❌ {stats.errors} 个错误</span>}
                    {stats.skips > 0 && (
                      <span className="text-gray-600">
                        ⏭️ {stats.skips} 个跳过
                        {stats.skips > 0 && (
                          <span className="ml-2 text-gray-500">
                            (💰{stats.skipPaid} 🔐{stats.skipCert} 🔍{stats.skipNotFound} 🚫{stats.skipAccess} 🌍{stats.skipGeo} ⏭️{stats.skipOther})
                          </span>
                        )}
                      </span>
                    )}
                  </div>
                )}

                <div
                  ref={logContainerRef}
                  className="space-y-1 max-h-96 overflow-y-auto"
                >
                  {filteredLogs.map((log) => (
                    <div
                      key={log.id}
                      className={`text-xs ${getLogColor(log.type)} font-mono`}
                    >
                      <span className="text-gray-400">[{log.timestamp}]</span>{' '}
                      <span>{getLogIcon(log.type)}</span>{' '}
                      <span>
                        {log.type === 'progress' && log.current !== undefined && log.total !== undefined
                          ? `[${log.current}/${log.total}] `
                          : ''
                        }
                        {log.type.startsWith('skip_') && (
                          <span className="font-semibold">[{getLogTypeLabel(log.type)}] </span>
                        )}
                        {log.message}
                      </span>
                    </div>
                  ))}
                  <div ref={logEndRef} />
                </div>

                {/* 自动滚动提示 */}
                {importing && autoScroll && (
                  <div className="mt-2 text-xs text-gray-500 text-center flex items-center justify-center">
                    <span className="inline-block animate-pulse mr-1">⬇</span>
                    正在处理中，自动滚动...
                  </div>
                )}
              </div>
            )}
          </div>
        </div>

        {/* 返回按钮 */}
        <div className="mt-4 text-center">
          <a
            href="/"
            className="text-sm text-blue-600 hover:text-blue-800"
          >
            ← 返回首页
          </a>
        </div>
      </div>
    </div>
  )
}
