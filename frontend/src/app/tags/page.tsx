'use client'

import { useEffect, useState, useMemo } from 'react'
import Link from 'next/link'
import { tagApi } from '@/lib/api'
import type { Tag } from '@/types'

type SortMode = 'popularity' | 'alphabetical'

export default function TagsPage() {
  const [tags, setTags] = useState<Tag[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [showCreateModal, setShowCreateModal] = useState(false)
  const [newTagName, setNewTagName] = useState('')
  const [newTagColor, setNewTagColor] = useState('#3B82F6')
  const [selectedTags, setSelectedTags] = useState<Set<number>>(new Set())
  const [isSelectMode, setIsSelectMode] = useState(false)
  const [sortMode, setSortMode] = useState<SortMode>('popularity')

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

  // 获取中文拼音首字母
  const getChineseInitial = (char: string): string => {
    const zh = "阿八查哒  发噶哈i 喀拉拿那哦p七r仨他哇v呀z".split("")
    const en = "ABCDEFGHJKLMNOPQRSTWXYZ".split("")

    // 简化的汉字到拼音首字母映射
    const pinyinMap: Record<string, string> = {
      '两': 'L', '性': 'X', '休': 'X', '体': 'T', '育': 'Y', '儿': 'E', '童': 'T',
      '与': 'Y', '家': 'J', '庭': 'T', '自': 'Z', '然': 'R', '科': 'K', '学': 'X',
      '历': 'L', '史': 'S', '名': 'M', '胜': 'S', '与': 'Y', '旅': 'L', '行': 'X',
      '地': 'D', '方': 'F', '文': 'W', '化': 'H', '哲': 'Z', '学': 'X', '心': 'X',
      '理': 'L', '学': 'X', '健': 'J', '康': 'K', '与': 'Y', '健': 'J', '身': 'S',
      '商': 'S', '务': 'W', '业': 'Y', '新': 'X', '闻': 'W', '评': 'P', '论': 'L',
      '实': 'S', '用': 'Y', '知': 'Z', '识': 'S', '创': 'C', '业': 'Y', '剧': 'J',
      '情': 'Q', '动': 'D', '画': 'H', '漫': 'M', '漫': 'M', '画': 'H', '医': 'Y',
      '学': 'X', '心': 'X', '理': 'L', '健': 'J', '康': 'K', '与': 'Y', '健': 'J',
      '身': 'S', '教': 'J', '育': 'Y', '佛': 'F', '教': 'J', '图': 'T', '书': 'S',
      '新': 'X', '闻': 'W', '娱': 'Y', '乐': 'L', '政': 'Z', '府': 'F', '管': 'G',
      '理': 'L', '营': 'Y', '销': 'X', '财': 'C', '经': 'J', '济': 'J', '每': 'M',
      '日': 'R', '新': 'X', '闻': 'W', '汽': 'Q', '车': 'C', '游': 'Y', '戏': 'X',
      '泳': 'Y', '灵': 'L', '修': 'X', '宗': 'Z', '教': 'J', '爱': 'A', '好': 'H',
      '宠': 'C', '物': 'W', '园': 'Y', '艺': 'Y', '林': 'L', '家': 'J', '居': 'J',
      '园': 'Y', '小': 'X', '说': 'S', '幽': 'Y', '默': 'M', '对': 'D', '谈': 'T',
      '影': 'Y', '评': 'P', '电': 'D', '影': 'Y', '史': 'S', '专': 'Z', '访': 'F',
      '视': 'S', '电': 'D', '视': 'S', '与': 'Y', '电': 'D', '影': 'Y', '评': 'P',
      '论': 'L', '社': 'S', '会': 'H', '化': 'H', '社': 'S', '会': 'H', '科': 'K',
      '学': 'X', '自': 'Z', '然': 'R', '科': 'K', '学': 'X', '科': 'K', '幻': 'H',
      '小': 'X', '说': 'S', '科': 'K', '技': 'J', '科': 'K', '技': 'J', '新': 'X',
      '闻': 'W', '管': 'G', '理': 'L', '篮': 'L', '球': 'Q', '跑': 'P', '步': 'B',
      '职': 'Z', '业': 'Y', '育': 'Y', '儿': 'E', '童': 'T', '育': 'Y', '脱': 'T',
      '口': 'K', '秀': 'X', '自': 'Z', '我': 'W', '完': 'W', '善': 'S', '自': 'Z',
      '然': 'R', '艺': 'Y', '术': 'S', '设': 'S', '计': 'J', '视': 'S', '觉': 'J',
      '艺': 'Y', '术': 'S', '表': 'B', '演': 'Y', '艺': 'Y', '术': 'S', '语': 'Y',
      '言': 'Y', '学': 'X', '习': 'X', '课': 'K', '程': 'C', '足': 'Z', '球': 'Q',
      '野': 'Y', '外': 'W', '非': 'F', '营': 'Y', '利': 'L', '组': 'Z', '织': 'Z',
      '音': 'Y', '乐': 'Y', '音': 'Y', '乐': 'L', '史': 'S', '音': 'Y', '乐': 'Y',
      '赏': 'S', '析': 'X', '饮': 'Y', '食': 'S', '文': 'W', '化': 'H', '航': 'H',
      '空': 'K', '美': 'M', '术': 'S', '节': 'J', '目': 'M', '漫': 'M', '谈': 'T',
      '营': 'Y', '销': 'X', '传': 'C', '媒': 'M', '素': 'S', '养': 'Y', '职': 'Z',
      '场': 'C', '筛': 'S', '选': 'X', '炼': 'L', '工': 'G', '作': 'Z', '流': 'L',
      '罪': 'Z', '纪': 'J', '实': 'S', '录': 'L', '犯': 'F', '罪': 'Z', '电': 'D',
      '子': 'Z', '游': 'Y', '戏': 'X', '生': 'S', '命': 'M', '科': 'K', '学': 'X',
      '飞': 'F', '行': 'X', '物': 'W', '理': 'L', '化': 'H', '学': 'X', '笑': 'X',
      '话': 'H', '新': 'X', '闻': 'W', '联': 'L', '播': 'B', '客': 'K'
    }

    // 查询映射表
    if (pinyinMap[char]) {
      return pinyinMap[char]
    }

    // 根据Unicode范围判断
    const code = char.charCodeAt(0)
    if (code >= 0x4E00 && code <= 0x9FA5) {
      // 汉字范围，使用默认映射
      return 'Z' // 未知汉字归到Z
    }

    if (/[a-zA-Z]/.test(char)) {
      return char.toUpperCase()
    }

    return '#'
  }

  // 按字母分组标签
  const groupedTags = useMemo(() => {
    if (sortMode === 'popularity') {
      return null
    }

    // 使用 Intl.Collator 进行中文拼音排序
    const collator = new Intl.Collator('zh-CN', { sensitivity: 'base' })
    const sorted = [...tags].sort((a, b) => collator.compare(a.name, b.name))

    // 按首字母分组
    const groups: Record<string, Tag[]> = {}
    sorted.forEach(tag => {
      // 获取拼音首字母
      const firstChar = tag.name.charAt(0)
      const letter = getChineseInitial(firstChar)

      if (!groups[letter]) {
        groups[letter] = []
      }
      groups[letter].push(tag)
    })

    return groups
  }, [tags, sortMode])

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

            {/* 排序模式切换 */}
            <div className="flex items-center gap-2 mb-4">
              <span className="text-sm text-slate-600">排序方式:</span>
              <button
                onClick={() => setSortMode('popularity')}
                className={
                  "px-3 py-1.5 rounded-lg text-sm transition-colors " +
                  (sortMode === 'popularity'
                    ? 'bg-slate-800 text-white'
                    : 'bg-slate-100 text-slate-600 hover:bg-slate-200')
                }
              >
                热度
              </button>
              <button
                onClick={() => setSortMode('alphabetical')}
                className={
                  "px-3 py-1.5 rounded-lg text-sm transition-colors " +
                  (sortMode === 'alphabetical'
                    ? 'bg-slate-800 text-white'
                    : 'bg-slate-100 text-slate-600 hover:bg-slate-200')
                }
              >
                字母
              </button>
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
            ) : sortMode === 'alphabetical' && groupedTags ? (
              // 字母序分组显示
              <div className="space-y-6">
                {Object.keys(groupedTags)
                  .sort()
                  .map((letter) => (
                    <div key={letter}>
                      <h3 className="text-lg font-semibold text-slate-800 mb-3 sticky top-0 bg-slate-50 py-2 border-b border-slate-200">
                        {letter}
                      </h3>
                      <div className="grid grid-cols-2 sm:grid-cols-4 md:grid-cols-6 lg:grid-cols-8 gap-3">
                        {groupedTags[letter].map((tag) => (
                          <TagCard
                            key={tag.id}
                            tag={tag}
                            isSelectMode={isSelectMode}
                            isSelected={selectedTags.has(tag.id)}
                            onToggleSelect={() => handleToggleSelect(tag.id)}
                          />
                        ))}
                      </div>
                    </div>
                  ))}
              </div>
            ) : (
              // 热度模式显示
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
  // 根据节目数量计算视觉强度
  const count = tag.podcast_count || 0
  let intensityClass = ""
  let countColorClass = "text-slate-400"

  if (count >= 50) {
    intensityClass = "ring-2 ring-blue-300 bg-blue-100"
    countColorClass = "text-blue-700 font-bold"
  } else if (count >= 30) {
    intensityClass = "ring-1 ring-blue-200 bg-blue-50"
    countColorClass = "text-blue-600 font-semibold"
  } else if (count >= 10) {
    intensityClass = "ring-1 ring-slate-200 bg-slate-100"
    countColorClass = "text-slate-600 font-medium"
  }

  const cardClass = [
    "rounded-lg shadow-sm px-3 py-2 h-12 flex items-center gap-2",
    "hover:shadow-md transition-all cursor-pointer relative",
    isSelected ? "ring-2 ring-blue-500" : "",
    intensityClass
  ].filter(Boolean).join(" ")

  return (
    <div
      className={cardClass}
      onClick={isSelectMode ? onToggleSelect : undefined}
    >
      {/* 多选模式复选框 */}
      {isSelectMode && (
        <div className="absolute top-2 right-2">
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

      {/* 标签名称和数量 */}
      <h3 className="text-sm font-normal text-slate-900 truncate flex-1">
        {tag.name}
        {tag.podcast_count !== undefined && (
          <span className={countColorClass + " ml-1"}>({tag.podcast_count})</span>
        )}
      </h3>
    </div>
  )
}
