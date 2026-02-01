'use client'

import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Badge } from '@/components/ui/badge'
import { useWorkflowForm } from '@/hooks/useWorkflowForm'
import type { Workflow } from '@/types'

interface WorkflowFormModalProps {
  isOpen: boolean
  onClose: () => void
  onSuccess: () => void
  workflow?: Workflow | null
}

export default function WorkflowFormModal({ isOpen, onClose, onSuccess, workflow }: WorkflowFormModalProps) {
  const {
    // 状态
    step,
    loading,
    formData,

    // Step 1
    cronError,
    cronPresets,

    // Step 2
    podcasts,
    podcastSearch,
    candidatePodcastIds,
    isLoadingPodcasts,
    displayedCount,
    tags,
    tagSearch,
    isTagFilterExpanded,
    isLoadingTags,
    newCustomUrl,

    // 操作
    nextStep,
    prevStep,
    updateField,
    setPodcastSearch,
    setTagSearch,
    setIsTagFilterExpanded,
    setNewCustomUrl,
    loadMorePodcasts,
    addCustomUrl,
    removeCustomUrl,

    // 提交
    submit,
  } = useWorkflowForm({ workflow, isOpen })

  // 处理提交
  const handleSubmit = async () => {
    const success = await submit()
    if (success) {
      onSuccess()
      onClose()
    }
  }

  return (
    <Dialog open={isOpen} onOpenChange={onClose}>
      <DialogContent className="max-w-4xl max-h-[90vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>
            {workflow ? '编辑工作流' : '创建工作流'}
          </DialogTitle>
        </DialogHeader>

        {/* 步骤指示器 */}
        <div className="flex items-center justify-center space-x-4 mb-6">
          {[1, 2, 3, 4].map((s) => (
            <div key={s} className="flex items-center">
              <div className={`flex items-center justify-center w-8 h-8 rounded-full ${
                step === s ? 'bg-violet-600 text-white' : 'bg-gray-200 text-gray-600'
              }`}>
                {s}
              </div>
              {s < 4 && (
                <div className={`w-16 h-1 mx-2 ${
                  step > s ? 'bg-violet-600' : 'bg-gray-200'
                }`} />
              )}
            </div>
          ))}
        </div>

        {/* Step 1: 基本信息 */}
        {step === 1 && (
          <div className="space-y-4">
            <h2 className="text-xl font-semibold">基本信息</h2>

            <div>
              <Label htmlFor="name">工作流名称 *</Label>
              <Input
                id="name"
                value={formData.name}
                onChange={(e) => updateField('name', e.target.value)}
                placeholder="例如：科技播客每日更新"
              />
            </div>

            <div>
              <Label htmlFor="description">描述</Label>
              <Textarea
                id="description"
                value={formData.description}
                onChange={(e) => updateField('description', e.target.value)}
                placeholder="简要描述这个工作流的用途"
                rows={3}
              />
            </div>

            <div>
              <Label>执行计划</Label>
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <Label htmlFor="preset">预设</Label>
                  <Select
                    value={formData.customCron ? 'custom' : formData.schedule}
                    onValueChange={(value) => {
                      if (value === 'custom') {
                        updateField('customCron', formData.schedule)
                      } else {
                        updateField('schedule', value)
                        updateField('customCron', '')
                      }
                    }}
                  >
                    <SelectTrigger id="preset">
                      <SelectValue placeholder="选择预设" />
                    </SelectTrigger>
                    <SelectContent>
                      {cronPresets.map((preset) => (
                        <SelectItem key={preset.value} value={preset.value}>
                          {preset.label}
                        </SelectItem>
                      ))}
                      <SelectItem value="custom">自定义</SelectItem>
                    </SelectContent>
                  </Select>
                </div>

                <div>
                  <Label htmlFor="cron">Cron表达式</Label>
                  <Input
                    id="cron"
                    value={formData.customCron || formData.schedule}
                    onChange={(e) => {
                      if (cronPresets.some(p => p.value === e.target.value)) {
                        updateField('schedule', e.target.value)
                        updateField('customCron', '')
                      } else {
                        updateField('customCron', e.target.value)
                      }
                    }}
                    placeholder="0 0 2 * * *"
                  />
                  {cronError && (
                    <p className="text-sm text-red-600 mt-1">{cronError}</p>
                  )}
                </div>
              </div>
            </div>
          </div>
        )}

        {/* Step 2: 范围配置 */}
        {step === 2 && (
          <div className="space-y-4">
            <h2 className="text-xl font-semibold">范围配置</h2>

            <div>
              <Label htmlFor="scopeType">抓取范围</Label>
              <Select
                value={formData.scopeType}
                onValueChange={(value: any) => updateField('scopeType', value)}
              >
                <SelectTrigger id="scopeType">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all_subscribed">所有订阅节目</SelectItem>
                  <SelectItem value="selected">指定节目</SelectItem>
                  <SelectItem value="by_tags">按标签筛选</SelectItem>
                  <SelectItem value="custom_urls">自定义RSS地址</SelectItem>
                </SelectContent>
              </Select>
            </div>

            {/* 指定节目 */}
            {formData.scopeType === 'selected' && (
              <div>
                <Label htmlFor="podcastSearch">搜索节目</Label>
                <Input
                  id="podcastSearch"
                  value={podcastSearch}
                  onChange={(e) => setPodcastSearch(e.target.value)}
                  placeholder="输入节目名称搜索"
                />
                <div className="mt-2 max-h-60 overflow-y-auto border rounded">
                  {isLoadingPodcasts ? (
                    <div className="p-4 text-center">加载中...</div>
                  ) : (
                    podcasts.slice(0, displayedCount).map((podcast) => (
                      <div
                        key={podcast.id}
                        className="flex items-center justify-between p-3 hover:bg-gray-50 cursor-pointer border-b"
                        onClick={() => {
                          const newIds = formData.selectedPodcastIds.includes(podcast.id)
                            ? formData.selectedPodcastIds.filter(id => id !== podcast.id)
                            : [...formData.selectedPodcastIds, podcast.id]
                          updateField('selectedPodcastIds', newIds)
                        }}
                      >
                        <div className="flex-1">
                          <div className="font-medium">{podcast.title}</div>
                          <div className="text-sm text-gray-600">{podcast.author}</div>
                        </div>
                        <input
                          type="checkbox"
                          checked={formData.selectedPodcastIds.includes(podcast.id)}
                          onChange={() => {}}
                          className="w-4 h-4"
                        />
                      </div>
                    ))
                  )}
                </div>
                <div className="mt-2 text-sm text-gray-600">
                  已选择 {formData.selectedPodcastIds.length} 个节目
                </div>
              </div>
            )}

            {/* 自定义RSS */}
            {formData.scopeType === 'custom_urls' && (
              <div>
                <Label>自定义RSS地址</Label>
                <div className="flex gap-2">
                  <Input
                    value={newCustomUrl}
                    onChange={(e) => setNewCustomUrl(e.target.value)}
                    placeholder="https://example.com/feed.xml"
                  />
                  <Button onClick={addCustomUrl}>添加</Button>
                </div>
                <div className="mt-2 space-y-2">
                  {formData.customUrls.map((url, index) => (
                    <div key={index} className="flex items-center justify-between p-2 bg-gray-50 rounded">
                      <span className="text-sm flex-1 truncate">{url}</span>
                      <Button
                        variant="destructive"
                        size="sm"
                        onClick={() => removeCustomUrl(index)}
                      >
                        删除
                      </Button>
                    </div>
                  ))}
                </div>
              </div>
            )}
          </div>
        )}

        {/* Step 3: 规则配置 */}
        {step === 3 && (
          <div className="space-y-4">
            <h2 className="text-xl font-semibold">规则配置</h2>

            <div>
              <Label htmlFor="timeRange">时间范围（天）</Label>
              <Input
                id="timeRange"
                type="number"
                value={formData.timeRange || ''}
                onChange={(e) => updateField('timeRange', parseInt(e.target.value) || 0)}
                placeholder="0表示不限制"
              />
            </div>

            <div>
              <Label htmlFor="minDuration">最小时长（分钟）</Label>
              <Input
                id="minDuration"
                type="number"
                value={formData.minDuration || ''}
                onChange={(e) => updateField('minDuration', parseInt(e.target.value) || 0)}
                placeholder="0表示不限制"
              />
            </div>

            <div>
              <Label htmlFor="maxResults">最大结果数</Label>
              <Input
                id="maxResults"
                type="number"
                value={formData.maxResults || ''}
                onChange={(e) => updateField('maxResults', parseInt(e.target.value) || 0)}
                placeholder="0表示不限制"
              />
            </div>

            <div>
              <Label htmlFor="keywords">关键词（逗号分隔）</Label>
              <Input
                id="keywords"
                value={formData.keywords}
                onChange={(e) => updateField('keywords', e.target.value)}
                placeholder="科技,AI,创新"
              />
            </div>

            <div>
              <Label htmlFor="excludeWords">排除词（逗号分隔）</Label>
              <Input
                id="excludeWords"
                value={formData.excludeWords}
                onChange={(e) => updateField('excludeWords', e.target.value)}
                placeholder="广告,预告"
              />
            </div>

            {/* LLM配置 */}
            <div className="border-t pt-4">
              <h3 className="text-lg font-semibold mb-2">LLM智能摘要（可选）</h3>

              <div className="flex items-center space-x-2 mb-4">
                <input
                  type="checkbox"
                  id="llmEnabled"
                  checked={formData.llmEnabled}
                  onChange={(e) => updateField('llmEnabled', e.target.checked)}
                />
                <Label htmlFor="llmEnabled">启用LLM智能摘要</Label>
              </div>

              {formData.llmEnabled && (
                <div className="space-y-4">
                  <div>
                    <Label htmlFor="llmMaxEpisodes">最大处理集数</Label>
                    <Input
                      id="llmMaxEpisodes"
                      type="number"
                      value={formData.llmMaxEpisodes}
                      onChange={(e) => updateField('llmMaxEpisodes', parseInt(e.target.value) || 20)}
                    />
                  </div>

                  <div>
                    <Label htmlFor="llmModel">模型名称</Label>
                    <Input
                      id="llmModel"
                      value={formData.llmModel}
                      onChange={(e) => updateField('llmModel', e.target.value)}
                      placeholder="gpt-4"
                    />
                  </div>
                </div>
              )}
            </div>
          </div>
        )}

        {/* Step 4: 确认 */}
        {step === 4 && (
          <div className="space-y-4">
            <h2 className="text-xl font-semibold">确认信息</h2>

            <div className="bg-gray-50 p-4 rounded space-y-2">
              <div><strong>名称：</strong>{formData.name}</div>
              <div><strong>描述：</strong>{formData.description || '无'}</div>
              <div><strong>执行计划：</strong>{formData.customCron || formData.schedule}</div>
              <div><strong>范围：</strong>{
                formData.scopeType === 'all_subscribed' ? '所有订阅节目' :
                formData.scopeType === 'selected' ? `指定节目 (${formData.selectedPodcastIds.length}个)` :
                formData.scopeType === 'by_tags' ? `按标签筛选 (${formData.selectedTagIds.length}个)` :
                `自定义RSS (${formData.customUrls.length}个)`
              }</div>
              <div><strong>时间范围：</strong>{formData.timeRange ? `${formData.timeRange}天` : '不限制'}</div>
              <div><strong>关键词：</strong>{formData.keywords || '无'}</div>
              <div><strong>LLM摘要：</strong>{formData.llmEnabled ? '启用' : '禁用'}</div>
            </div>
          </div>
        )}

        {/* 导航按钮 */}
        <div className="flex justify-between mt-6">
          <Button
            variant="outline"
            onClick={prevStep}
            disabled={step === 1}
          >
            上一步
          </Button>

          {step < 4 ? (
            <Button onClick={nextStep}>
              下一步
            </Button>
          ) : (
            <Button onClick={handleSubmit} disabled={loading}>
              {loading ? '提交中...' : '提交'}
            </Button>
          )}
        </div>
      </DialogContent>
    </Dialog>
  )
}
