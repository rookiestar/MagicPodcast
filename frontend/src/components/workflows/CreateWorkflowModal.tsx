'use client'

import { useState } from 'react'
import { workflowApi, podcastApi } from '@/lib/api'
import type { WorkflowRequest, WorkflowScopeType, ScopeConfig, RulesConfig, Podcast } from '@/types'

type Step = 1 | 2 | 3

interface CreateWorkflowModalProps {
  isOpen: boolean
  onClose: () => void
  onSuccess: () => void
}

// 预设的Cron表达式
const CRON_PRESETS = [
  { label: '每天凌晨2点', value: '0 2 * * *' },
  { label: '每天早上8点', value: '0 8 * * *' },
  { label: '每天晚上8点', value: '0 20 * * *' },
  { label: '每周日凌晨2点', value: '0 2 * * 0' },
  { label: '每周一早上6点', value: '0 6 * * 1' },
]

export default function CreateWorkflowModal({ isOpen, onClose, onSuccess }: CreateWorkflowModalProps) {
  const [step, setStep] = useState<Step>(1)
  const [loading, setLoading] = useState(false)

  // Step 1: 基本信息
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [schedule, setSchedule] = useState('0 2 * * *')
  const [customCron, setCustomCron] = useState('')

  // Step 2: 范围配置
  const [scopeType, setScopeType] = useState<WorkflowScopeType>('all_subscribed')
  const [selectedPodcastIds, setSelectedPodcastIds] = useState<number[]>([])
  const [customUrls, setCustomUrls] = useState<string[]>([])
  const [newCustomUrl, setNewCustomUrl] = useState('')
  const [podcasts, setPodcasts] = useState<Podcast[]>([])
  const [podcastSearch, setPodcastSearch] = useState('')
  const [candidatePodcastIds, setCandidatePodcastIds] = useState<number[]>([]) // 备选列表中的节目ID
  const [isLoadingPodcasts, setIsLoadingPodcasts] = useState(false) // 加载状态

  // Step 3: 规则配置
  const [timeRange, setTimeRange] = useState(0)
  const [minDuration, setMinDuration] = useState(0)
  const [maxResults, setMaxResults] = useState(0)
  const [keywords, setKeywords] = useState('')
  const [excludeWords, setExcludeWords] = useState('')
  const [isEnabled, setIsEnabled] = useState(true)

  // 加载播客列表 - 支持分页加载所有节目
  const loadPodcasts = async () => {
    try {
      setIsLoadingPodcasts(true)
      console.log('[CreateWorkflowModal] Loading all podcasts...')
      let allPodcasts: Podcast[] = []
      let page = 1
      let hasMore = true
      const maxPages = 10 // 最多加载10页（1000个节目），避免超时

      // 分页加载所有节目
      while (hasMore && page <= maxPages) {
        console.log(`[CreateWorkflowModal] Loading page ${page}...`)
        const response = await podcastApi.list({ page, page_size: 100 })
        const newPodcasts = response.data || []

        allPodcasts = [...allPodcasts, ...newPodcasts]
        console.log(`[CreateWorkflowModal] Loaded page ${page}:`, newPodcasts.length, 'items, total:', allPodcasts.length)

        // 如果返回的数量少于 page_size，说明已经是最后一页
        if (newPodcasts.length < 100) {
          hasMore = false
        } else {
          page++
        }
      }

      if (page > maxPages && hasMore) {
        console.warn('[CreateWorkflowModal] Reached max pages limit, some podcasts may not be loaded')
      }

      console.log('[CreateWorkflowModal] Total podcasts loaded:', allPodcasts.length)
      setPodcasts(allPodcasts)
    } catch (err) {
      console.error('[CreateWorkflowModal] Failed to load podcasts:', err)
      alert('加载节目失败: ' + (err instanceof Error ? err.message : '未知错误'))
    } finally {
      setIsLoadingPodcasts(false)
    }
  }

  // 处理自定义URL添加
  const handleAddCustomUrl = () => {
    if (newCustomUrl.trim() && !customUrls.includes(newCustomUrl.trim())) {
      setCustomUrls([...customUrls, newCustomUrl.trim()])
      setNewCustomUrl('')
    }
  }

  // 添加节目到备选列表
  const handleAddToCandidate = (podcastId: number) => {
    if (!candidatePodcastIds.includes(podcastId)) {
      setCandidatePodcastIds([...candidatePodcastIds, podcastId])
    }
  }

  // 从备选列表移除节目
  const handleRemoveFromCandidate = (podcastId: number) => {
    setCandidatePodcastIds(candidatePodcastIds.filter(id => id !== podcastId))
  }

  // 批量添加搜索结果到备选列表
  const handleAddAllFiltered = () => {
    const filteredIds = filteredPodcasts.map(p => p.id)
    const newCandidateIds = [...new Set([...candidatePodcastIds, ...filteredIds])]
    setCandidatePodcastIds(newCandidateIds)
  }

  // 处理自定义URL删除
  const handleRemoveCustomUrl = (url: string) => {
    setCustomUrls(customUrls.filter(u => u !== url))
  }

  // 验证当前步骤
  const validateStep = (): boolean => {
    if (step === 1) {
      if (!name.trim()) {
        alert('请输入工作流名称')
        return false
      }
      if (!schedule.trim()) {
        alert('请选择或输入定时规则')
        return false
      }
    }
    if (step === 2) {
      if (scopeType === 'specific_podcasts' && candidatePodcastIds.length === 0) {
        alert('请至少添加一个节目到备选列表')
        return false
      }
      if (scopeType === 'custom_sources' && customUrls.length === 0) {
        alert('请至少添加一个RSS源')
        return false
      }
    }
    return true
  }

  // 处理下一步
  const handleNext = async () => {
    if (!validateStep()) return

    if (step === 2 && scopeType === 'specific_podcasts' && podcasts.length === 0) {
      await loadPodcasts()
    }

    if (step < 3) {
      setStep((step + 1) as Step)
    } else {
      await handleSubmit()
    }
  }

  // 处理上一步
  const handlePrev = () => {
    if (step > 1) {
      setStep((step - 1) as Step)
    }
  }

  // 提交创建
  const handleSubmit = async () => {
    try {
      setLoading(true)

      const scopeConfig: ScopeConfig = {}
      if (scopeType === 'specific_podcasts') {
        scopeConfig.podcast_ids = candidatePodcastIds
      } else if (scopeType === 'custom_sources') {
        scopeConfig.custom_urls = customUrls
      }

      const rulesConfig: RulesConfig = {
        time_range: timeRange || undefined,
        min_duration: minDuration || undefined,
        max_results: maxResults || undefined,
        keywords: keywords.trim() || undefined,
        exclude_words: excludeWords.trim() || undefined,
      }

      const data: WorkflowRequest = {
        name: name.trim(),
        description: description.trim(),
        schedule: customCron.trim() || schedule,
        scope_type: scopeType,
        scope_config: scopeConfig,
        rules_config: rulesConfig,
        is_enabled: isEnabled,
      }

      console.log('[CreateWorkflowModal] Submitting workflow:', data)

      await workflowApi.create(data)
      onSuccess()
      handleClose()
    } catch (err) {
      console.error('[CreateWorkflowModal] Create failed:', err)
      const errorMessage = err instanceof Error ? err.message : 'Unknown error'
      alert(`创建失败: ${errorMessage}`)
    } finally {
      setLoading(false)
    }
  }

  // 关闭Modal
  const handleClose = () => {
    setStep(1)
    setName('')
    setDescription('')
    setSchedule('0 2 * * *')
    setCustomCron('')
    setScopeType('all_subscribed')
    setSelectedPodcastIds([])
    setCandidatePodcastIds([])
    setCustomUrls([])
    setNewCustomUrl('')
    setPodcasts([])
    setPodcastSearch('')
    setTimeRange(0)
    setMinDuration(0)
    setMaxResults(0)
    setKeywords('')
    setExcludeWords('')
    setIsEnabled(true)
    onClose()
  }

  // 过滤播客列表
  const filteredPodcasts = podcasts.filter(p => {
    if (!podcastSearch.trim()) return true

    const searchLower = podcastSearch.toLowerCase().trim()
    const title = (p.title || '').toLowerCase()
    const author = (p.author || '').toLowerCase()

    const titleMatch = title.includes(searchLower)
    const authorMatch = author.includes(searchLower)

    return titleMatch || authorMatch
  })

  if (!isOpen) return null

  return (
    <div className="fixed inset-0 bg-black/50 z-50 flex items-center justify-center p-4">
      <div className="bg-white dark:bg-slate-800 rounded-lg shadow-2xl w-full max-w-3xl max-h-[90vh] overflow-hidden flex flex-col">
        {/* Header */}
        <div className="border-b border-slate-200 dark:border-slate-700 p-6">
          <div className="flex items-center justify-between">
            <h2 className="text-2xl font-bold text-slate-900 dark:text-slate-50">
              创建工作流 ({step}/3)
            </h2>
            <button
              onClick={handleClose}
              className="text-slate-400 hover:text-slate-600 dark:hover:text-slate-200 text-2xl"
            >
              ×
            </button>
          </div>
          {/* Progress Bar */}
          <div className="mt-4 flex gap-2">
            <div className={`flex-1 h-1 rounded ${step >= 1 ? 'bg-blue-600' : 'bg-slate-200 dark:bg-slate-700'}`} />
            <div className={`flex-1 h-1 rounded ${step >= 2 ? 'bg-blue-600' : 'bg-slate-200 dark:bg-slate-700'}`} />
            <div className={`flex-1 h-1 rounded ${step >= 3 ? 'bg-blue-600' : 'bg-slate-200 dark:bg-slate-700'}`} />
          </div>
        </div>

        {/* Content */}
        <div className="flex-1 overflow-y-auto p-6">
          {step === 1 && (
            <div className="space-y-6">
              <div>
                <label className="block text-sm font-medium text-slate-700 dark:text-slate-300 mb-2">
                  工作流名称 <span className="text-red-500">*</span>
                </label>
                <input
                  type="text"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder="例如: 每日科技播客抓取"
                  className="w-full px-4 py-2 border border-slate-300 dark:border-slate-600 rounded-lg bg-white dark:bg-slate-700 text-slate-900 dark:text-slate-100 focus:ring-2 focus:ring-blue-500 focus:border-transparent"
                />
              </div>

              <div>
                <label className="block text-sm font-medium text-slate-700 dark:text-slate-300 mb-2">
                  描述
                </label>
                <textarea
                  value={description}
                  onChange={(e) => setDescription(e.target.value)}
                  placeholder="简要描述这个工作流的用途..."
                  rows={3}
                  className="w-full px-4 py-2 border border-slate-300 dark:border-slate-600 rounded-lg bg-white dark:bg-slate-700 text-slate-900 dark:text-slate-100 focus:ring-2 focus:ring-blue-500 focus:border-transparent"
                />
              </div>

              <div>
                <label className="block text-sm font-medium text-slate-700 dark:text-slate-300 mb-2">
                  定时规则 <span className="text-red-500">*</span>
                </label>
                <div className="space-y-3">
                  <div className="grid grid-cols-2 gap-2">
                    {CRON_PRESETS.map((preset) => (
                      <button
                        key={preset.value}
                        type="button"
                        onClick={() => {
                          setSchedule(preset.value)
                          setCustomCron('')
                        }}
                        className={`px-4 py-2 rounded-lg text-left transition-colors ${
                          schedule === preset.value && !customCron
                            ? 'bg-blue-600 text-white'
                            : 'bg-slate-100 dark:bg-slate-700 text-slate-700 dark:text-slate-300 hover:bg-slate-200 dark:hover:bg-slate-600'
                        }`}
                      >
                        {preset.label}
                      </button>
                    ))}
                  </div>
                  <div className="flex gap-2">
                    <input
                      type="text"
                      value={customCron}
                      onChange={(e) => {
                        setCustomCron(e.target.value)
                        if (e.target.value) setSchedule('')
                      }}
                      placeholder="自定义Cron表达式，如: 0 */6 * * * (每6小时)"
                      className="flex-1 px-4 py-2 border border-slate-300 dark:border-slate-600 rounded-lg bg-white dark:bg-slate-700 text-slate-900 dark:text-slate-100 focus:ring-2 focus:ring-blue-500 focus:border-transparent"
                    />
                  </div>
                  <p className="text-xs text-slate-500 dark:text-slate-400">
                    Cron格式: 分 时 日 月 周
                  </p>
                </div>
              </div>
            </div>
          )}

          {step === 2 && (
            <div className="space-y-6">
              <div>
                <label className="block text-sm font-medium text-slate-700 dark:text-slate-300 mb-3">
                  选择要处理的节目范围 <span className="text-red-500">*</span>
                </label>
                <div className="space-y-3">
                  <label className="flex items-start gap-3 p-3 border border-slate-200 dark:border-slate-700 rounded-lg hover:bg-slate-50 dark:hover:bg-slate-900 cursor-pointer">
                    <input
                      type="radio"
                      name="scopeType"
                      checked={scopeType === 'all_subscribed'}
                      onChange={() => setScopeType('all_subscribed')}
                      className="mt-1"
                    />
                    <div>
                      <div className="font-medium text-slate-900 dark:text-slate-50">全部已订阅节目</div>
                      <div className="text-sm text-slate-600 dark:text-slate-400">处理所有订阅的播客节目</div>
                    </div>
                  </label>

                  <label className="flex items-start gap-3 p-3 border border-slate-200 dark:border-slate-700 rounded-lg hover:bg-slate-50 dark:hover:bg-slate-900 cursor-pointer">
                    <input
                      type="radio"
                      name="scopeType"
                      checked={scopeType === 'specific_podcasts'}
                      onChange={() => {
                        setScopeType('specific_podcasts')
                        if (podcasts.length === 0) loadPodcasts()
                      }}
                      className="mt-1"
                    />
                    <div className="flex-1">
                      <div className="font-medium text-slate-900 dark:text-slate-50">指定节目</div>
                      <div className="text-sm text-slate-600 dark:text-slate-400 mb-2">从订阅中选择特定节目</div>
                      {scopeType === 'specific_podcasts' && (
                        <div className="space-y-3">
                          {/* 加载指示器 */}
                          {isLoadingPodcasts && (
                            <div className="flex items-center gap-2 text-sm text-blue-600 dark:text-blue-400">
                              <div className="animate-spin rounded-full h-4 w-4 border-b-2 border-blue-600"></div>
                              <span>正在加载节目数据...</span>
                            </div>
                          )}

                          {/* 搜索框 */}
                          <div className="relative">
                            <input
                              type="text"
                              value={podcastSearch}
                              onChange={(e) => setPodcastSearch(e.target.value)}
                              placeholder="搜索节目名称或作者..."
                              disabled={isLoadingPodcasts}
                              className="w-full px-3 py-2 pr-8 border border-slate-300 dark:border-slate-600 rounded bg-white dark:bg-slate-700 text-sm disabled:opacity-50"
                            />
                            {podcastSearch && !isLoadingPodcasts && (
                              <button
                                onClick={() => setPodcastSearch('')}
                                className="absolute right-2 top-1/2 -translate-y-1/2 text-slate-400 hover:text-slate-600"
                              >
                                ✕
                              </button>
                            )}
                          </div>

                          {/* 三栏布局 - 固定高度 */}
                          <div className="grid grid-cols-12 gap-3">
                            {/* 左侧：搜索结果列表 */}
                            <div className="col-span-5">
                              <div className="text-xs font-medium text-slate-700 dark:text-slate-300 mb-1">
                                搜索结果 ({filteredPodcasts.length})
                              </div>
                              <div className="h-80 overflow-y-auto border border-slate-200 dark:border-slate-700 rounded-lg p-2 bg-white dark:bg-slate-800">
                                {isLoadingPodcasts ? (
                                  <div className="text-center text-slate-500 dark:text-slate-400 py-4 text-xs">
                                    加载中...
                                  </div>
                                ) : filteredPodcasts.length === 0 ? (
                                  <div className="text-center text-slate-500 dark:text-slate-400 py-4 text-xs">
                                    {podcasts.length === 0 ? (
                                      <button
                                        onClick={() => loadPodcasts()}
                                        className="text-blue-600 dark:text-blue-400 hover:underline"
                                      >
                                        点击加载节目
                                      </button>
                                    ) : podcastSearch ? (
                                      `没有找到匹配"${podcastSearch}"的节目`
                                    ) : (
                                      '显示所有 ' + podcasts.length + ' 个节目'
                                    )}
                                  </div>
                                ) : (
                                  filteredPodcasts.slice(0, 50).map((podcast) => (
                                    <div
                                      key={podcast.id}
                                      className="flex items-center justify-between p-2 hover:bg-slate-100 dark:hover:bg-slate-700 rounded cursor-pointer group mb-1"
                                    >
                                      <div className="flex-1 min-w-0 pr-2">
                                        <div className="text-xs font-medium text-slate-900 dark:text-slate-50 truncate">
                                          {podcast.title}
                                        </div>
                                        {podcast.author && (
                                          <div className="text-xs text-slate-500 dark:text-slate-400 truncate">
                                            {podcast.author}
                                          </div>
                                        )}
                                      </div>
                                      <button
                                        onClick={() => handleAddToCandidate(podcast.id)}
                                        disabled={candidatePodcastIds.includes(podcast.id)}
                                        className="w-7 h-7 flex items-center justify-center text-sm bg-slate-100 dark:bg-slate-700 text-slate-600 dark:text-slate-300 rounded hover:bg-slate-200 dark:hover:bg-slate-600 disabled:bg-slate-50 dark:disabled:bg-slate-800 disabled:text-slate-300 dark:disabled:text-slate-600 disabled:cursor-not-allowed border border-slate-200 dark:border-slate-600"
                                        title={candidatePodcastIds.includes(podcast.id) ? '已添加' : '添加'}
                                      >
                                        {candidatePodcastIds.includes(podcast.id) ? '✓' : '>'}
                                      </button>
                                    </div>
                                  ))
                                )}
                                {filteredPodcasts.length > 50 && (
                                  <div className="text-center text-xs text-slate-400 dark:text-slate-500 py-2">
                                    仅显示前 50 个结果
                                  </div>
                                )}
                              </div>
                            </div>

                            {/* 中间：批量操作按钮 */}
                            <div className="col-span-2 flex flex-col justify-center gap-3">
                              {filteredPodcasts.length > 0 && !isLoadingPodcasts && (
                                <button
                                  onClick={handleAddAllFiltered}
                                  className="w-9 h-9 flex items-center justify-center text-lg bg-slate-100 dark:bg-slate-700 text-slate-600 dark:text-slate-300 rounded hover:bg-slate-200 dark:hover:bg-slate-600 mx-auto border border-slate-200 dark:border-slate-600"
                                  title="添加所有搜索结果"
                                >
                                  ≫
                                </button>
                              )}
                              {candidatePodcastIds.length > 0 && (
                                <button
                                  onClick={() => setCandidatePodcastIds([])}
                                  className="w-9 h-9 flex items-center justify-center text-sm bg-slate-100 dark:bg-slate-700 text-slate-600 dark:text-slate-300 rounded hover:bg-slate-200 dark:hover:bg-slate-600 mx-auto border border-slate-200 dark:border-slate-600"
                                  title="清空备选列表"
                                >
                                  ✕
                                </button>
                              )}
                            </div>

                            {/* 右侧：备选列表 */}
                            <div className="col-span-5">
                              <div className="text-xs font-medium text-slate-700 dark:text-slate-300 mb-1">
                                已选 ({candidatePodcastIds.length})
                              </div>
                              <div className="h-80 overflow-y-auto border border-slate-200 dark:border-slate-700 rounded-lg p-2 bg-slate-50 dark:bg-slate-800/50">
                                {candidatePodcastIds.length === 0 ? (
                                  <div className="text-center text-slate-400 dark:text-slate-500 py-4 text-xs">
                                    空列表
                                  </div>
                                ) : (
                                  candidatePodcastIds.map((id) => {
                                    const podcast = podcasts.find(p => p.id === id)
                                    if (!podcast) {
                                      console.warn('[CreateWorkflowModal] Podcast not found for ID:', id)
                                      return null
                                    }
                                    return (
                                      <div
                                        key={podcast.id}
                                        className="flex items-center justify-between p-2 bg-white dark:bg-slate-800 hover:bg-slate-100 dark:hover:bg-slate-700 rounded mb-1 border border-slate-200 dark:border-slate-600"
                                      >
                                        <div className="flex-1 min-w-0 pr-2">
                                          <div className="text-xs font-medium text-slate-900 dark:text-slate-50 truncate">
                                            {podcast.title}
                                          </div>
                                          {podcast.author && (
                                            <div className="text-xs text-slate-500 dark:text-slate-400 truncate">
                                              {podcast.author}
                                            </div>
                                          )}
                                        </div>
                                        <button
                                          onClick={() => handleRemoveFromCandidate(podcast.id)}
                                          className="w-7 h-7 flex items-center justify-center text-sm bg-slate-100 dark:bg-slate-700 text-slate-600 dark:text-slate-300 rounded hover:bg-slate-200 dark:hover:bg-slate-600 border border-slate-200 dark:border-slate-600"
                                          title="移除"
                                        >
                                          {'<'}
                                        </button>
                                      </div>
                                    )
                                  })
                                )}
                              </div>
                            </div>
                          </div>
                        </div>
                      )}
                    </div>
                  </label>

                  <label className="flex items-start gap-3 p-3 border border-slate-200 dark:border-slate-700 rounded-lg hover:bg-slate-50 dark:hover:bg-slate-900 cursor-pointer">
                    <input
                      type="radio"
                      name="scopeType"
                      checked={scopeType === 'custom_sources'}
                      onChange={() => setScopeType('custom_sources')}
                      className="mt-1"
                    />
                    <div className="flex-1">
                      <div className="font-medium text-slate-900 dark:text-slate-50">自定义RSS源</div>
                      <div className="text-sm text-slate-600 dark:text-slate-400 mb-2">添加自定义RSS源URL</div>
                      {scopeType === 'custom_sources' && (
                        <div className="space-y-2">
                          <div className="flex gap-2">
                            <input
                              type="url"
                              value={newCustomUrl}
                              onChange={(e) => setNewCustomUrl(e.target.value)}
                              onKeyPress={(e) => e.key === 'Enter' && handleAddCustomUrl()}
                              placeholder="输入RSS URL，按回车添加"
                              className="flex-1 px-3 py-2 border border-slate-300 dark:border-slate-600 rounded bg-white dark:bg-slate-700 text-sm"
                            />
                            <button
                              type="button"
                              onClick={handleAddCustomUrl}
                              className="px-4 py-2 bg-blue-600 text-white rounded hover:bg-blue-700 text-sm"
                            >
                              添加
                            </button>
                          </div>
                          {customUrls.length > 0 && (
                            <div className="space-y-1">
                              {customUrls.map((url) => (
                                <div key={url} className="flex items-center gap-2 p-2 bg-slate-50 dark:bg-slate-900 rounded">
                                  <span className="text-xs flex-1 truncate">{url}</span>
                                  <button
                                    type="button"
                                    onClick={() => handleRemoveCustomUrl(url)}
                                    className="text-red-600 hover:text-red-700 text-sm"
                                  >
                                    删除
                                  </button>
                                </div>
                              ))}
                            </div>
                          )}
                        </div>
                      )}
                    </div>
                  </label>
                </div>
              </div>
            </div>
          )}

          {step === 3 && (
            <div className="space-y-6">
              <div>
                <label className="block text-sm font-medium text-slate-700 dark:text-slate-300 mb-4">
                  抓取规则配置 (可选)
                </label>
                <div className="space-y-4">
                  <div>
                    <label className="block text-sm text-slate-600 dark:text-slate-400 mb-1">
                      时间范围 (天)
                    </label>
                    <input
                      type="number"
                      min={0}
                      value={timeRange || ''}
                      onChange={(e) => setTimeRange(parseInt(e.target.value) || 0)}
                      placeholder="0"
                      className="w-full px-4 py-2 border border-slate-300 dark:border-slate-600 rounded-lg bg-white dark:bg-slate-700 text-slate-900 dark:text-slate-100"
                    />
                    <p className="text-xs text-slate-500 dark:text-slate-400 mt-1">只抓取指定天数内发布的单集，0表示不限制</p>
                  </div>

                  <div>
                    <label className="block text-sm text-slate-600 dark:text-slate-400 mb-1">
                      最小时长 (秒)
                    </label>
                    <input
                      type="number"
                      min={0}
                      value={minDuration || ''}
                      onChange={(e) => setMinDuration(parseInt(e.target.value) || 0)}
                      placeholder="0"
                      className="w-full px-4 py-2 border border-slate-300 dark:border-slate-600 rounded-lg bg-white dark:bg-slate-700 text-slate-900 dark:text-slate-100"
                    />
                    <p className="text-xs text-slate-500 dark:text-slate-400 mt-1">只抓取超过此时长的单集，0表示不限制</p>
                  </div>

                  <div>
                    <label className="block text-sm text-slate-600 dark:text-slate-400 mb-1">
                      最大结果数
                    </label>
                    <input
                      type="number"
                      min={0}
                      value={maxResults || ''}
                      onChange={(e) => setMaxResults(parseInt(e.target.value) || 0)}
                      placeholder="0"
                      className="w-full px-4 py-2 border border-slate-300 dark:border-slate-600 rounded-lg bg-white dark:bg-slate-700 text-slate-900 dark:text-slate-100"
                    />
                    <p className="text-xs text-slate-500 dark:text-slate-400 mt-1">每个节目最多抓取的单集数量，0表示不限制</p>
                  </div>

                  <div>
                    <label className="block text-sm text-slate-600 dark:text-slate-400 mb-1">
                      关键词过滤
                    </label>
                    <input
                      type="text"
                      value={keywords}
                      onChange={(e) => setKeywords(e.target.value)}
                      placeholder="例如: 技术,AI,机器学习"
                      className="w-full px-4 py-2 border border-slate-300 dark:border-slate-600 rounded-lg bg-white dark:bg-slate-700 text-slate-900 dark:text-slate-100"
                    />
                    <p className="text-xs text-slate-500 dark:text-slate-400 mt-1">只抓取标题或简介中包含这些关键词的单集，逗号分隔</p>
                  </div>

                  <div>
                    <label className="block text-sm text-slate-600 dark:text-slate-400 mb-1">
                      排除词
                    </label>
                    <input
                      type="text"
                      value={excludeWords}
                      onChange={(e) => setExcludeWords(e.target.value)}
                      placeholder="例如: 广告,推广"
                      className="w-full px-4 py-2 border border-slate-300 dark:border-slate-600 rounded-lg bg-white dark:bg-slate-700 text-slate-900 dark:text-slate-100"
                    />
                    <p className="text-xs text-slate-500 dark:text-slate-400 mt-1">排除标题或简介中包含这些词的单集，逗号分隔</p>
                  </div>
                </div>
              </div>

              <div className="flex items-center gap-2">
                <input
                  type="checkbox"
                  id="isEnabled"
                  checked={isEnabled}
                  onChange={(e) => setIsEnabled(e.target.checked)}
                  className="rounded"
                />
                <label htmlFor="isEnabled" className="text-sm font-medium text-slate-700 dark:text-slate-300">
                  创建后立即启用此工作流
                </label>
              </div>
            </div>
          )}
        </div>

        {/* Footer */}
        <div className="border-t border-slate-200 dark:border-slate-700 p-6 flex justify-between">
          <button
            onClick={handleClose}
            className="px-6 py-2 text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:hover:text-slate-200"
          >
            取消
          </button>
          <div className="flex gap-3">
            {step > 1 && (
              <button
                onClick={handlePrev}
                className="px-6 py-2 border border-slate-300 dark:border-slate-600 rounded-lg hover:bg-slate-50 dark:hover:bg-slate-700 transition-colors"
              >
                上一步
              </button>
            )}
            <button
              onClick={handleNext}
              disabled={loading}
              className="px-6 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
            >
              {loading ? '处理中...' : step === 3 ? '创建' : '下一步'}
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}
