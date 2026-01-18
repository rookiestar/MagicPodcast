'use client'

import { useState, useEffect, useRef, KeyboardEvent } from 'react'
import { Tag } from '@/types'
import { tagApi } from '@/lib/api'
import TagBadge from './TagBadge'

interface TagInputProps {
  selectedTags: Tag[]
  onTagsChange: (tags: Tag[]) => void
  placeholder?: string
  showSelectedTags?: boolean
}

export default function TagInput({ selectedTags, onTagsChange, placeholder = '输入标签按回车添加', showSelectedTags = true }: TagInputProps) {
  const [inputValue, setInputValue] = useState('')
  const [availableTags, setAvailableTags] = useState<Tag[]>([])
  const [showSuggestions, setShowSuggestions] = useState(false)
  const [filteredTags, setFilteredTags] = useState<Tag[]>([])
  const [loading, setLoading] = useState(false)
  const inputRef = useRef<HTMLInputElement>(null)

  // 加载所有可用标签
  useEffect(() => {
    const fetchTags = async () => {
      try {
        setLoading(true)
        const tags = await tagApi.list()
        setAvailableTags(tags)
      } catch (error) {
        console.error('Failed to fetch tags:', error)
      } finally {
        setLoading(false)
      }
    }
    fetchTags()
  }, [])

  // 过滤建议标签
  useEffect(() => {
    const selectedIds = selectedTags.map(t => t.id)

    if (inputValue.trim()) {
      // 有输入内容时，过滤匹配的标签
      const filtered = availableTags.filter(
        t => !selectedIds.includes(t.id) &&
        t.name.toLowerCase().includes(inputValue.toLowerCase())
      )
      setFilteredTags(filtered)
    } else {
      // 没有输入内容时，显示所有未选择的标签
      const filtered = availableTags.filter(
        t => !selectedIds.includes(t.id)
      )
      setFilteredTags(filtered)
    }
    // 注意：不在这里控制 showSuggestions，而是在 onFocus/onBlur 中控制
  }, [inputValue, availableTags, selectedTags])

  // 添加标签
  const addTag = async (tag: Tag) => {
    onTagsChange([...selectedTags, tag])
    setInputValue('')
    setShowSuggestions(false)
  }

  // 创建新标签
  const createTag = async (name: string) => {
    try {
      // 生成随机颜色
      const colors = ['#3B82F6', '#10B981', '#F59E0B', '#EF4444', '#8B5CF6', '#EC4899', '#6366F1']
      const randomColor = colors[Math.floor(Math.random() * colors.length)]

      const newTag = await tagApi.create({
        name: name.trim(),
        color: randomColor
      })

      onTagsChange([...selectedTags, newTag])
      setInputValue('')
      setShowSuggestions(false)
    } catch (err) {
      alert(err instanceof Error ? err.message : '创建标签失败')
    }
  }

  // 移除标签
  const removeTag = (tagId: number) => {
    onTagsChange(selectedTags.filter(t => t.id !== tagId))
  }

  // 处理键盘事件
  const handleKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Enter' && inputValue.trim()) {
      e.preventDefault()

      // 检查是否匹配已有标签
      const matchedTag = availableTags.find(
        t => t.name.toLowerCase() === inputValue.toLowerCase().trim()
      )

      const selectedIds = selectedTags.map(t => t.id)
      if (matchedTag && !selectedIds.includes(matchedTag.id)) {
        addTag(matchedTag)
      } else if (!matchedTag) {
        // 创建新标签
        createTag(inputValue)
      }
    } else if (e.key === 'Backspace' && !inputValue && selectedTags.length > 0) {
      // 删除最后一个标签
      removeTag(selectedTags[selectedTags.length - 1].id)
    } else if (e.key === 'Escape') {
      setShowSuggestions(false)
      setInputValue('')
    }
  }

  // 处理输入变化
  const handleInputChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    setInputValue(e.target.value)
  }

  // 处理失去焦点
  const handleBlur = () => {
    // 延迟关闭，以便点击建议项
    setTimeout(() => {
      setShowSuggestions(false)
    }, 200)
  }

  // 处理获得焦点
  const handleFocus = () => {
    // 显示所有可用标签
    setShowSuggestions(true)
  }

  return (
    <div className="w-full">
      {/* 已选择的标签 - 根据 showSelectedTags 控制显示 */}
      {showSelectedTags && selectedTags.length > 0 && (
        <div className="inline-flex flex-wrap items-center gap-1.5 mb-3 align-middle">
          {selectedTags.map(tag => (
            <TagBadge
              key={tag.id}
              tag={tag}
              size="md"
              variant="simple"
              removable
              onRemove={() => removeTag(tag.id)}
            />
          ))}
        </div>
      )}

      {/* 输入框 - 始终显示 */}
      <div className="relative">
        <input
          ref={inputRef}
          type="text"
          value={inputValue}
          onChange={handleInputChange}
          onKeyDown={handleKeyDown}
          onBlur={handleBlur}
          onFocus={handleFocus}
          className={`
            w-full px-4 py-2
            border border-slate-300 dark:border-slate-600
            rounded-lg
            bg-white dark:bg-slate-800
            text-sm text-slate-900 dark:text-slate-100
            placeholder:text-slate-400 dark:placeholder:text-slate-500
            focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent
            transition-colors
          `}
          placeholder={placeholder}
        />

        {/* 建议下拉列表 */}
        {showSuggestions && (filteredTags.length > 0 || inputValue.trim()) && (
          <>
            {/* 点击外部关闭 */}
            <div
              className="fixed inset-0 z-10"
              onClick={() => {
                setShowSuggestions(false)
                setInputValue('')
              }}
            />

            {/* 下拉菜单 */}
            <div className="absolute z-20 w-full mt-1 max-h-60 overflow-auto rounded-lg shadow-lg bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700">
              {filteredTags.length > 0 ? (
                <>
                  {inputValue.trim() && (
                    <div className="px-4 py-2 text-xs text-slate-500 dark:text-slate-400 border-b border-slate-200 dark:border-slate-700">
                      匹配的标签
                    </div>
                  )}
                  <div className="py-1">
                    {filteredTags.map(tag => (
                      <button
                        key={tag.id}
                        onClick={() => addTag(tag)}
                        className="w-full px-4 py-2 text-left hover:bg-slate-100 dark:hover:bg-slate-700 transition-colors focus:outline-none focus:bg-slate-100 dark:focus:bg-slate-700"
                      >
                        <div className="flex items-center gap-3">
                          <span
                            className="w-3 h-3 rounded-full flex-shrink-0"
                            style={{ backgroundColor: tag.color }}
                          />
                          <span className="text-sm font-medium text-slate-900 dark:text-slate-100">
                            {tag.name}
                          </span>
                        </div>
                      </button>
                    ))}
                  </div>
                </>
              ) : (
                inputValue.trim() && (
                  <div className="py-1">
                    <button
                      onClick={() => createTag(inputValue)}
                      className="w-full px-4 py-2 text-left hover:bg-slate-100 dark:hover:bg-slate-700 transition-colors focus:outline-none focus:bg-slate-100 dark:focus:bg-slate-700"
                    >
                      <div className="flex items-center gap-3">
                        <span className="text-sm text-blue-600 dark:text-blue-400">
                          + 创建 "{inputValue.trim()}"
                        </span>
                      </div>
                    </button>
                  </div>
                )
              )}

              {/* 当没有匹配且没有输入时显示提示 */}
              {!inputValue.trim() && filteredTags.length === 0 && availableTags.length > 0 && (
                <div className="px-4 py-3 text-sm text-slate-500 dark:text-slate-400 text-center">
                  所有标签都已选择
                </div>
              )}

              {!inputValue.trim() && availableTags.length === 0 && !loading && (
                <div className="px-4 py-3 text-sm text-slate-500 dark:text-slate-400 text-center">
                  暂无可用标签，输入名称创建新标签
                </div>
              )}
            </div>
          </>
        )}
      </div>
    </div>
  )
}
