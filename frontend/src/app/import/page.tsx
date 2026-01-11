'use client'

import { useState, useEffect, useRef, useMemo } from 'react'
import Link from 'next/link'
import { syncApi } from '@/lib/api'

interface LogEntry {
  id: string
  type: 'info' | 'success' | 'error' | 'progress' | 'summary' |
       'skip_paid' | 'skip_cert' | 'skip_not_found' | 'skip_no_update' |
       'skip_access_denied' | 'skip_geo_blocked' |
       'skip_duplicate' | 'skip_invalid' | 'skip_other'
  message: string
  timestamp: string
  current?: number
  total?: number
  reason?: string // 跳过原因
  data?: any // summary数据
}

type TabType = 'import' | 'sync'

export default function ImportPage() {
  const [activeTab, setActiveTab] = useState<TabType>('import')

  // 导入OPML状态
  const [file, setFile] = useState<File | null>(null)
  const [importing, setImporting] = useState(false)

  // 同步元数据状态
  const [syncing, setSyncing] = useState(false)

  // 共享的日志和UI状态
  const [logs, setLogs] = useState<LogEntry[]>([])
  const [filter, setFilter] = useState<'all' | 'errors' | 'success' | 'skips'>('all')
  const [autoScroll, setAutoScroll] = useState(true)

  const logContainerRef = useRef<HTMLDivElement>(null)
  const logEndRef = useRef<HTMLDivElement>(null)

  // 自动滚动到底部
  useEffect(() => {
    if (!autoScroll) return

    requestAnimationFrame(() => {
      if (logEndRef.current && autoScroll) {
        logEndRef.current.scrollIntoView({ behavior: 'auto', block: 'end' })
      }
    })
  }, [logs, autoScroll])

  // 从localStorage恢复状态（仅执行一次）
  useEffect(() => {
    const savedLogs = localStorage.getItem('syncLogs')
    const savedSyncing = localStorage.getItem('syncing')
    const savedImporting = localStorage.getItem('importing')

    const restoredLogs: LogEntry[] = []

    if (savedLogs) {
      try {
        const parsedLogs = JSON.parse(savedLogs)
        restoredLogs.push(...parsedLogs)
      } catch (e) {
        console.error('Failed to parse saved logs:', e)
      }
    }

    // 如果之前在同步中，添加提示
    if (savedSyncing === 'true') {
      restoredLogs.push({
        id: Date.now().toString(),
        type: 'info',
        message: '⚠️ 页面已刷新，上次同步状态已丢失',
        timestamp: new Date().toLocaleTimeString()
      })
    }

    // 如果之前在导入中，添加提示
    if (savedImporting === 'true') {
      restoredLogs.push({
        id: Date.now().toString(),
        type: 'info',
        message: '⚠️ 页面已刷新，导入需要重新开始',
        timestamp: new Date().toLocaleTimeString()
      })
    }

    if (restoredLogs.length > 0) {
      setLogs(restoredLogs)
    }
  }, [])

  // 保存状态到localStorage
  useEffect(() => {
    localStorage.setItem('syncLogs', JSON.stringify(logs))
  }, [logs])

  useEffect(() => {
    localStorage.setItem('syncing', syncing.toString())
  }, [syncing])

  useEffect(() => {
    localStorage.setItem('importing', importing.toString())
  }, [importing])

  // 监听滚动事件
  useEffect(() => {
    const container = logContainerRef.current
    if (!container) return

    const handleScroll = () => {
      if (autoScroll) {
        setAutoScroll(false)
      }
    }

    container.addEventListener('scroll', handleScroll, { passive: true })
    return () => container.removeEventListener('scroll', handleScroll)
  }, [autoScroll])

  // 恢复自动滚动
  const handleResumeAutoScroll = () => {
    setAutoScroll(true)
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

  // 统计信息 - 优先使用summary消息，否则统计日志
  const stats = useMemo(() => {
    // 查找最新的summary消息
    const summaryLog = logs.filter(l => l.type === 'summary').pop()

    if (summaryLog?.data) {
      // 使用后端发送的准确统计数据
      return {
        total: summaryLog.data.total_podcasts || 0,
        success: summaryLog.data.success_podcasts || 0,
        errors: summaryLog.data.failed_podcasts || 0,
        skips: summaryLog.data.skipped_podcasts || 0,
        skipPaid: 0,
        skipCert: 0,
        skipNotFound: 0,
        skipAccess: 0,
        skipGeo: 0,
        skipOther: 0,
        skipNoUpdate: summaryLog.data.no_update_podcasts || 0,
        duration: summaryLog.data.duration || '',
        fromSummary: true, // 标记数据来自summary
      }
    }

    // 降级：如果没有summary，统计日志（旧逻辑）
    const successLogs = logs.filter(l =>
      l.type === 'success' &&
      (l.message.includes('成功导入:') || l.message.includes('成功同步:')) &&
      !l.message.includes('完成')
    )

    return {
      total: logs.filter(l =>
        (l.type === 'success' && (l.message.includes('成功导入:') || l.message.includes('成功同步:')) && !l.message.includes('完成')) ||
        l.type === 'error' ||
        l.type.startsWith('skip_')
      ).length,
      errors: logs.filter(l => l.type === 'error').length,
      success: successLogs.length,
      skips: logs.filter(l => l.type.startsWith('skip_')).length,
      skipPaid: logs.filter(l => l.type === 'skip_paid').length,
      skipCert: logs.filter(l => l.type === 'skip_cert').length,
      skipNotFound: logs.filter(l => l.type === 'skip_not_found').length,
      skipAccess: logs.filter(l => l.type === 'skip_access_denied').length,
      skipGeo: logs.filter(l => l.type === 'skip_geo_blocked').length,
      skipOther: logs.filter(l => l.type === 'skip_other' || l.type === 'skip_duplicate' || l.type === 'skip_invalid').length,
      skipNoUpdate: logs.filter(l => l.type === 'skip_no_update').length,
      fromSummary: false,
    }
  }, [logs])

  const addLog = (type: 'info' | 'success' | 'error' | 'progress' | LogEntry['type'], message: string, current?: number, total?: number, data?: any) => {
    // 明确创建log对象，确保data字段被包含
    const newLog: LogEntry = {
      id: Date.now() + Math.random().toString(),
      type: type as LogEntry['type'],
      message: message,
      timestamp: new Date().toLocaleTimeString(),
      ...(current !== undefined && { current }),
      ...(total !== undefined && { total }),
      ...(data !== undefined && { data }),
    }

    setLogs(prev => [...prev, newLog])
  }

  const handleFileChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const selectedFile = e.target.files?.[0]
    if (selectedFile) {
      const validTypes = ['application/xml', 'text/xml', 'text/opml', 'text/x-opml']
      const fileExt = selectedFile.name.split('.').pop()?.toLowerCase()

      if (!validTypes.includes(selectedFile.type) && !['opml', 'xml'].includes(fileExt || '')) {
        alert('请选择OPML或XML文件')
        return
      }

      setFile(selectedFile)
      setLogs([])
    }
  }

  // 导入OPML（智能模式：本地匹配+在线同步）
  const handleImport = async () => {
    if (!file) {
      alert('请先选择OPML文件')
      return
    }

    setImporting(true)
    setLogs([])

    addLog('info', '开始导入OPML（智能模式：本地匹配+在线同步）...')

    let receivedSummary = false

    try {
      await syncApi.importOPMLSSE(file, (type, message, current, total) => {
        addLog(type as any, message, current, total)
        // 标记是否收到了summary消息
        if (type === 'summary' || type === 'complete') {
          receivedSummary = true
        }
      })

      // 如果导入成功完成但没有收到summary消息，添加一个默认的完成消息
      if (!receivedSummary) {
        console.log('[Import] 导入完成但未收到summary消息，添加默认完成消息')
        addLog('success', '✅ 导入完成！所有播客已自动同步')
      }
    } catch (error: any) {
      console.error('导入失败:', error)

      if (error.message?.includes('超时')) {
        addLog('error', '导入超时：可能是网络较慢或文件太大')
        addLog('info', '提示：您可以重新导入，系统会自动跳过已导入的播客')
      } else if (error.message?.includes('Network') || error.message?.includes('fetch')) {
        addLog('error', '网络连接错误：' + (error.message || '未知错误'))
        addLog('info', '提示：请检查网络连接后重试')
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

  // 同步元数据
  const handleSync = async () => {
    setSyncing(true)
    setLogs([])  // 清空日志

    addLog('info', '开始同步所有播客的元数据...')

    let receivedSummary = false

    try {
      await syncApi.syncPodcastsMetadataSSE((type, message, current, total, data) => {
        addLog(type as any, message, current, total, data)
        // 标记是否收到了summary消息
        if (type === 'summary' || type === 'complete') {
          receivedSummary = true
        }
      })

      // 如果同步成功完成但没有收到summary消息，添加一个默认的完成消息
      if (!receivedSummary) {
        console.log('[Sync] 同步完成但未收到summary消息，添加默认完成消息')
        addLog('success', '✅ 同步已完成！')
      }
    } catch (error: any) {
      console.error('同步失败:', error)
      addLog('error', '同步失败：' + (error.message || '未知错误'))
    } finally {
      setSyncing(false)
    }
  }

  const getLogIcon = (type: LogEntry['type']) => {
    switch (type) {
      case 'success': return '✅'
      case 'error': return '❌'
      case 'progress': return '⏳'
      case 'summary': return '📊'
      case 'skip_paid': return '💰'
      case 'skip_cert': return '🔐'
      case 'skip_not_found': return '🔍'
      case 'skip_no_update': return '✓'
      case 'skip_access_denied': return '🚫'
      case 'skip_geo_blocked': return '🌍'
      case 'skip_duplicate': return '🔄'
      case 'skip_invalid': return '📄'
      case 'skip_other': return '⏭️'
      default: return 'ℹ️'
    }
  }

  const getLogColor = (type: LogEntry['type']) => {
    switch (type) {
      case 'success': return 'text-green-700'
      case 'error': return 'text-red-700'
      case 'progress': return 'text-blue-700'
      case 'summary': return 'text-purple-700'
      case 'skip_paid': return 'text-yellow-700'
      case 'skip_cert': return 'text-orange-700'
      case 'skip_not_found': return 'text-gray-600'
      case 'skip_no_update': return 'text-gray-500'
      case 'skip_access_denied': return 'text-red-600'
      case 'skip_geo_blocked': return 'text-purple-700'
      case 'skip_duplicate': return 'text-cyan-700'
      case 'skip_invalid': return 'text-indigo-700'
      case 'skip_other': return 'text-gray-500'
      default: return 'text-gray-700'
    }
  }

  const getLogTypeLabel = (type: LogEntry['type']) => {
    switch (type) {
      case 'skip_paid': return '付费播客'
      case 'skip_cert': return '证书过期'
      case 'skip_not_found': return '不存在'
      case 'skip_no_update': return '无更新'
      case 'skip_access_denied': return '访问拒绝'
      case 'skip_geo_blocked': return '地区限制'
      case 'skip_duplicate': return '重复'
      case 'skip_invalid': return '格式无效'
      case 'skip_other': return '其他'
      default: return ''
    }
  }

  return (
    <div className="min-h-screen bg-gray-50 py-12 px-4 sm:px-6 lg:px-8">
      <div className="max-w-4xl mx-auto">
        <div className="bg-white shadow rounded-lg">
          {/* Header */}
          <div className="px-6 py-4 border-b border-gray-200">
            <div className="flex items-center justify-between mb-4">
              <h1 className="text-2xl font-bold text-gray-900">导入/同步</h1>
              <Link
                href="/"
                className="text-sm text-blue-600 hover:text-blue-800"
              >
                ← 返回首页
              </Link>
            </div>

            {/* Tabs */}
            <div className="flex gap-2">
              <button
                onClick={() => setActiveTab('import')}
                disabled={importing || syncing}
                className={`px-4 py-2 rounded-lg font-medium transition-colors ${
                  activeTab === 'import'
                    ? 'bg-green-600 text-white'
                    : 'bg-gray-200 text-gray-700 hover:bg-gray-300'
                } ${importing || syncing ? 'opacity-50 cursor-not-allowed' : ''}`}
              >
                📁 导入OPML
              </button>
              <button
                onClick={() => setActiveTab('sync')}
                disabled={importing || syncing}
                className={`px-4 py-2 rounded-lg font-medium transition-colors ${
                  activeTab === 'sync'
                    ? 'bg-blue-600 text-white'
                    : 'bg-gray-200 text-gray-700 hover:bg-gray-300'
                } ${importing || syncing ? 'opacity-50 cursor-not-allowed' : ''}`}
              >
                🔄 同步元数据
              </button>
            </div>
          </div>

          {/* Content */}
          <div className="px-6 py-6">
            {/* Import Tab */}
            {activeTab === 'import' && (
              <>
                {/* 说明部分 */}
                <div className="mb-6 p-4 bg-blue-50 border border-blue-200 rounded-md">
                  <h3 className="text-sm font-medium text-blue-900 mb-2">
                    关于导入OPML
                  </h3>
                  <ul className="text-xs text-blue-800 space-y-1 list-disc list-inside">
                    <li>仅从本地PodcastIndex数据库匹配播客信息（快速）</li>
                    <li>导入完成后可选择是否在线同步最新元数据</li>
                    <li>支持从小宇宙、Apple Podcasts等应用导出的OPML文件</li>
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
                <div className="mb-6">
                  <button
                    onClick={handleImport}
                    disabled={!file || importing}
                    className={`px-4 py-2 border border-transparent rounded-md shadow-sm text-sm font-medium text-white
                      ${!file || importing
                        ? 'bg-gray-300 cursor-not-allowed'
                        : 'bg-green-600 hover:bg-green-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-green-500'
                      }`}
                  >
                    {importing ? '导入中...' : '开始导入'}
                  </button>
                </div>
              </>
            )}

            {/* Sync Tab */}
            {activeTab === 'sync' && (
              <>
                {/* 说明部分 */}
                <div className="mb-6 p-4 bg-blue-50 border border-blue-200 rounded-md">
                  <h3 className="text-sm font-medium text-blue-900 mb-2">
                    关于同步元数据
                  </h3>
                  <ul className="text-xs text-blue-800 space-y-1 list-disc list-inside">
                    <li>从在线RSS feed更新所有播客的最新元数据</li>
                    <li>包括单集数量、最新发布时间、播客描述等信息</li>
                    <li>可能需要较长时间，取决于播客数量和网络状况</li>
                  </ul>
                </div>

                {/* 同步按钮 */}
                <div className="mb-6">
                  <button
                    onClick={handleSync}
                    disabled={syncing}
                    className={`px-4 py-2 border border-transparent rounded-md shadow-sm text-sm font-medium text-white
                      ${syncing
                        ? 'bg-gray-300 cursor-not-allowed'
                        : 'bg-blue-600 hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-blue-500'
                      }`}
                  >
                    {syncing ? '同步中...' : '开始同步'}
                  </button>
                </div>
              </>
            )}

            {/* 实时日志 */}
            {logs.length > 0 && (
              <div className="border border-gray-300 rounded-lg p-4 bg-gray-50">
                <div className="flex items-center justify-between mb-3">
                  <div className="flex items-center space-x-3">
                    <h3 className="text-sm font-medium text-gray-900">
                      {activeTab === 'import' ? '导入日志' : '同步日志'}
                      {(importing || syncing) && (
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
                        onClick={() => setFilter('errors')}
                        className={`text-xs px-3 py-1 rounded border transition-colors min-w-[100px] ${
                          filter === 'errors'
                            ? 'bg-red-50 border-red-500 text-red-700'
                            : 'bg-white border-gray-300 text-gray-700 hover:bg-gray-50'
                        }`}
                      >
                        失败 ({stats.errors})
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
                    {!importing && !syncing && (
                      <button
                        onClick={() => {
                          setLogs([])
                          setFilter('all')
                          setAutoScroll(true)
                          localStorage.removeItem('syncLogs')
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
                  <div className="mb-3 text-xs text-gray-600 flex items-center flex-wrap gap-x-3 gap-y-1">
                    {stats.fromSummary && stats.duration && (
                      <span className="text-blue-600 font-medium">⏱️ {stats.duration}</span>
                    )}
                    <span className="text-gray-500">总计: {stats.total}</span>
                    {stats.success > 0 && <span className="text-green-600">✅ {stats.success} 个成功</span>}
                    {stats.errors > 0 && <span className="text-red-600">❌ {stats.errors} 个错误</span>}
                    {stats.skips > 0 && (
                      <span className="text-gray-600">
                        ⏭️ {stats.skips} 个跳过
                        {!stats.fromSummary && (
                          <span className="ml-1 text-gray-500">
                            (💰{stats.skipPaid} 🔐{stats.skipCert} 🔍{stats.skipNotFound} 🚫{stats.skipAccess} 🌍{stats.skipGeo} ⏭️{stats.skipOther})
                          </span>
                        )}
                      </span>
                    )}
                    {stats.skipNoUpdate > 0 && (
                      <span className="text-gray-500">✓ {stats.skipNoUpdate} 个无更新</span>
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
                        {log.type === 'summary' && log.data ? (
                          // summary类型：显示详细的统计信息
                          <span className="font-semibold">
                            📊 同步完成！
                            <br />
                            总计: {log.data.total_podcasts} |
                            成功: {log.data.success_podcasts} |
                            失败: {log.data.failed_podcasts} |
                            跳过: {log.data.skipped_podcasts}
                            {log.data.no_update_podcasts > 0 && (
                            <span> (无更新: {log.data.no_update_podcasts})</span>
                            )}
                            {log.data.duration && <span> | 耗时: {log.data.duration}</span>}
                          </span>
                        ) : (
                          // 其他类型：显示原始消息
                          log.message
                        )}
                      </span>
                    </div>
                  ))}
                  <div ref={logEndRef} />
                </div>

                {/* 自动滚动提示 */}
                {(importing || syncing) && autoScroll && (
                  <div className="mt-2 text-xs text-gray-500 text-center flex items-center justify-center">
                    <span className="inline-block animate-pulse mr-1">⬇</span>
                    正在处理中，自动滚动...
                  </div>
                )}
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}
