'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import { tagApi } from '@/lib/api'
import type { Tag } from '@/types'

export default function TagsPage() {
  const [tags, setTags] = useState<Tag[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [showCreateModal, setShowCreateModal] = useState(false)
  const [newTagName, setNewTagName] = useState('')
  const [newTagColor, setNewTagColor] = useState('#3B82F6')
  const [selectedTags, setSelectedTags] = useState<Set<number>>(new Set())
  const [isSelectMode, setIsSelectMode] = useState(false)

  useEffect(() => {
    fetchTags()
  }, [])

  const fetchTags = async () => {
    try {
      setLoading(true)
      setError(null)
      const data = await tagApi.list()
      setTags(data)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unknown error')
    } finally {
      setLoading(false)
    }
  }

  const handleCreateTag = async (e: React.FormEvent) => {
    e.preventDefault()
    try {
      await tagApi.create({
        name: newTagName,
        color: newTagColor,
      })
      setShowCreateModal(false)
      setNewTagName('')
      setNewTagColor('#3B82F6')
      fetchTags()
    } catch (err) {
      alert(err instanceof Error ? err.message : '创建失败')
    }
  }

  const handleDeleteTag = async (id: number, name: string) => {
    if (!confirm(`确定要删除标签"${name}"吗？`)) return

    try {
      await tagApi.delete(id)
      fetchTags()
    } catch (err) {
      alert(err instanceof Error ? err.message : '删除失败')
    }
  }

  const handleToggleSelect = (tagId: number) => {
    const newSelected = new Set(selectedTags)
    if (newSelected.has(tagId)) {
      newSelected.delete(tagId)
    } else {
      newSelected.add(tagId)
    }
    setSelectedTags(newSelected)
  }

  const handleSelectAll = () => {
    if (selectedTags.size === tags.length) {
      setSelectedTags(new Set())
    } else {
      setSelectedTags(new Set(tags.map(tag => tag.id)))
    }
  }

  const handleBatchDelete = async () => {
    if (selectedTags.size === 0) return

    if (!confirm(`确定要删除选中的 ${selectedTags.size} 个标签吗？`)) return

    try {
      // 逐个删除
      for (const tagId of selectedTags) {
        await tagApi.delete(tagId)
      }
      setSelectedTags(new Set())
      setIsSelectMode(false)
      fetchTags()
    } catch (err) {
      alert(err instanceof Error ? err.message : '批量删除失败')
    }
  }

  return (
    <main className="min-h-screen bg-slate-50">
      <div className="container mx-auto px-4 py-8">
        {/* Header */}
        <div className="mb-8">
          <div className="mb-8">
            <div className="flex items-center justify-between mb-8">
              {/* 返回首页按钮 */}
              <Link
                href="/"
                className="w-36 h-11 px-4 bg-white text-slate-800 font-medium rounded-xl border border-slate-300 hover:bg-slate-50 hover:border-slate-400 transition-colors flex items-center justify-center gap-2"
              >
                <span>←</span>
                <span>返回首页</span>
              </Link>

              {/* 右侧按钮组 */}
              <div className="flex items-center gap-2">
                {isSelectMode ? (
                  <>
                    {/* 取消选择 */}
                    <button
                      onClick={() => {
                        setIsSelectMode(false)
                        setSelectedTags(new Set())
                      }}
                      className="w-24 h-11 bg-white text-slate-700 rounded-xl border border-slate-300 hover:bg-slate-50 transition-colors"
                    >
                      取消
                    </button>
                    {/* 全选按钮 */}
                    <button
                      onClick={handleSelectAll}
                      className="w-24 h-11 bg-white text-slate-700 rounded-xl border border-slate-300 hover:bg-slate-50 transition-colors"
                    >
                      {selectedTags.size === tags.length ? '取消全选' : '全选'}
                    </button>
                    {/* 批量删除按钮 */}
                    <button
                      onClick={handleBatchDelete}
                      disabled={selectedTags.size === 0}
                      className="w-28 h-11 bg-red-600 text-white rounded-xl hover:bg-red-700 transition-colors disabled:bg-red-300 disabled:cursor-not-allowed"
                    >
                      删除 ({selectedTags.size})
                    </button>
                  </>
                ) : (
                  <>
                    {/* 多选模式按钮 */}
                    <button
                      onClick={() => setIsSelectMode(true)}
                      className="w-24 h-11 bg-white text-slate-700 rounded-xl border border-slate-300 hover:bg-slate-50 transition-colors"
                    >
                      多选
                    </button>
                    {/* 新建标签按钮 */}
                    <button
                      onClick={() => setShowCreateModal(true)}
                      className="w-28 h-11 border-2 border-blue-600 rounded-xl bg-blue-600 text-white text-sm font-medium hover:bg-blue-700 hover:border-blue-700 transition-colors"
                    >
                      + 新建标签
                    </button>
                  </>
                )}
              </div>
            </div>

            {/* 标题和描述 */}
            <div className="mb-4">
              <h1 className="text-4xl md:text-5xl font-semibold text-slate-800 mb-2">
                标签管理
              </h1>
              <p className="text-base text-slate-600 max-w-2xl">
                管理你的播客标签
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
          <div className="bg-red-50 border border-red-200 rounded-lg p-6 mb-6">
            <h3 className="text-red-800 font-semibold mb-2">加载失败</h3>
            <p className="text-red-600 mb-4">{error}</p>
            <button
              onClick={fetchTags}
              className="px-4 py-2 bg-red-600 text-white rounded hover:bg-red-700"
            >
              重试
            </button>
          </div>
        )}

        {/* Tags List */}
        {!loading && !error && (
          <>
            <div className="mb-6 text-slate-600">
              共 {tags.length} 个标签
            </div>

            {tags.length === 0 ? (
              <div className="bg-white rounded-lg p-12 text-center">
                <p className="text-slate-600 mb-4">
                  还没有创建任何标签
                </p>
                <button
                  onClick={() => setShowCreateModal(true)}
                  className="px-6 py-3 bg-blue-600 text-white rounded-lg hover:bg-blue-700"
                >
                  创建第一个标签
                </button>
              </div>
            ) : (
              <div className="grid grid-cols-2 sm:grid-cols-4 md:grid-cols-6 lg:grid-cols-8 gap-3">
                {tags.map((tag) => (
                  <TagCard
                    key={tag.id}
                    tag={tag}
                    isSelectMode={isSelectMode}
                    isSelected={selectedTags.has(tag.id)}
                    onToggleSelect={() => handleToggleSelect(tag.id)}
                  />
                ))}
              </div>
            )}
          </>
        )}

        {/* Create Tag Modal */}
        {showCreateModal && (
          <div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50">
            <div className="bg-white rounded-lg p-6 w-full max-w-md">
              <h2 className="text-2xl font-bold text-slate-900 mb-4">
                新建标签
              </h2>
              <form onSubmit={handleCreateTag} className="space-y-4">
                <div>
                  <label className="block text-sm font-semibold text-slate-900 mb-2">
                    标签名称 *
                  </label>
                  <input
                    type="text"
                    value={newTagName}
                    onChange={(e) => setNewTagName(e.target.value)}
                    className="w-full px-3 py-2 border border-slate-300 rounded-lg bg-white text-slate-900 focus:ring-2 focus:ring-blue-500 focus:border-transparent"
                    required
                  />
                </div>
                <div>
                  <label className="block text-sm font-semibold text-slate-900 mb-2">
                    颜色
                  </label>
                  <div className="flex gap-2">
                    <input
                      type="color"
                      value={newTagColor}
                      onChange={(e) => setNewTagColor(e.target.value)}
                      className="h-10 w-20 rounded cursor-pointer"
                    />
                    <input
                      type="text"
                      value={newTagColor}
                      onChange={(e) => setNewTagColor(e.target.value)}
                      className="flex-1 px-3 py-2 border border-slate-300 rounded-lg bg-white text-slate-900"
                      placeholder="#3B82F6"
                    />
                  </div>
                </div>
                <div className="flex gap-2 pt-4">
                  <button
                    type="submit"
                    className="flex-1 px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 transition-colors"
                  >
                    创建
                  </button>
                  <button
                    type="button"
                    onClick={() => setShowCreateModal(false)}
                    className="flex-1 px-4 py-2 bg-slate-200 text-slate-700 rounded-lg hover:bg-slate-300 transition-colors"
                  >
                    取消
                  </button>
                </div>
              </form>
            </div>
          </div>
        )}
      </div>
    </main>
  )
}

function TagCard({
  tag,
  isSelectMode,
  isSelected,
  onToggleSelect
}: {
  tag: Tag
  isSelectMode: boolean
  isSelected: boolean
  onToggleSelect: () => void
}) {
  return (
    <div
      className={`
        bg-white rounded-lg shadow-sm px-3 py-2 h-12 flex items-center gap-2
        hover:shadow-md transition-shadow cursor-pointer
        ${isSelected ? 'ring-2 ring-blue-500' : ''}
      `}
      onClick={isSelectMode ? onToggleSelect : undefined}
    >
      {/* 多选模式复选框 */}
      {isSelectMode && (
        <div className="flex-shrink-0">
          <input
            type="checkbox"
            checked={isSelected}
            onChange={onToggleSelect}
            className="w-4 h-4 rounded border-slate-300 text-blue-600 focus:ring-blue-500"
            onClick={(e) => e.stopPropagation()}
          />
        </div>
      )}

      {/* 颜色圆点 */}
      <div
        className="w-3 h-3 rounded-full flex-shrink-0"
        style={{ backgroundColor: tag.color || '#ccc' }}
      />

      {/* 标签名称 */}
      <h3 className="text-sm font-normal text-slate-900 truncate flex-1">
        {tag.name}
      </h3>
    </div>
  )
}
