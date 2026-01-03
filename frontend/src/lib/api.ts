import axios from 'axios'
import type { ApiResponse, Podcast, Tag, Episode } from '@/types'

const API_URL = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080'

// 创建 axios 实例
const api = axios.create({
  baseURL: API_URL,
  timeout: 10000,
  headers: {
    'Content-Type': 'application/json',
  },
})

// API 方法
export const podcastApi = {
  // 获取播客列表
  list: async (params?: { tag_id?: number | number[] }): Promise<Podcast[]> => {
    const queryParams = new URLSearchParams()

    if (params?.tag_id) {
      // 支持多个tag_id（数组）
      if (Array.isArray(params.tag_id)) {
        params.tag_id.forEach(id => queryParams.append('tag_id', id.toString()))
      } else {
        queryParams.append('tag_id', params.tag_id.toString())
      }
    }

    const url = queryParams.toString()
      ? `/api/v1/podcasts?${queryParams.toString()}`
      : '/api/v1/podcasts'

    const response = await api.get<ApiResponse<Podcast[]>>(url)
    if (response.data.success && response.data.data) {
      return response.data.data
    }
    throw new Error(response.data.error?.message || 'Failed to fetch podcasts')
  },

  // 获取单个播客详情
  get: async (id: number): Promise<Podcast> => {
    const response = await api.get<ApiResponse<Podcast>>(`/api/v1/podcasts/${id}`)
    if (response.data.success && response.data.data) {
      return response.data.data
    }
    throw new Error(response.data.error?.message || 'Failed to fetch podcast')
  },

  // 获取播客备注
  getNotes: async (id: number): Promise<string> => {
    const response = await api.get<ApiResponse<{ id: number; notes: string }>>(`/api/v1/podcasts/${id}/notes`)
    if (response.data.success && response.data.data) {
      return response.data.data.notes
    }
    throw new Error(response.data.error?.message || 'Failed to fetch notes')
  },

  // 更新播客备注
  updateNotes: async (id: number, notes: string): Promise<void> => {
    const response = await api.put<ApiResponse<{ id: number; notes: string }>>(`/api/v1/podcasts/${id}/notes`, { notes })
    if (!response.data.success) {
      throw new Error(response.data.error?.message || 'Failed to update notes')
    }
  },

  // 获取播客的所有标签
  getTags: async (id: number): Promise<Tag[]> => {
    const response = await api.get<ApiResponse<Tag[]>>(`/api/v1/podcasts/${id}/tags`)
    if (response.data.success && response.data.data) {
      return response.data.data
    }
    throw new Error(response.data.error?.message || 'Failed to fetch tags')
  },

  // 为播客添加标签
  addTag: async (id: number, tagId: number): Promise<void> => {
    const response = await api.post<ApiResponse<any>>(`/api/v1/podcasts/${id}/tags`, { tag_id: tagId })
    if (!response.data.success) {
      throw new Error(response.data.error?.message || 'Failed to add tag')
    }
  },

  // 移除播客标签
  removeTag: async (id: number, tagId: number): Promise<void> => {
    const response = await api.delete<ApiResponse<any>>(`/api/v1/podcasts/${id}/tags/${tagId}`)
    if (!response.data.success) {
      throw new Error(response.data.error?.message || 'Failed to remove tag')
    }
  },
}

export const episodeApi = {
  // 获取播客的单集列表
  listByPodcast: async (podcastId: number): Promise<Episode[]> => {
    const response = await api.get<ApiResponse<Episode[]>>(`/api/v1/podcasts/${podcastId}/episodes`)
    if (response.data.success && response.data.data) {
      return response.data.data
    }
    throw new Error(response.data.error?.message || 'Failed to fetch episodes')
  },

  // 获取单集备注
  getNotes: async (id: number): Promise<string> => {
    const response = await api.get<ApiResponse<{ id: number; notes: string }>>(`/api/v1/episodes/${id}/notes`)
    if (response.data.success && response.data.data) {
      return response.data.data.notes
    }
    throw new Error(response.data.error?.message || 'Failed to fetch notes')
  },

  // 更新单集备注
  updateNotes: async (id: number, notes: string): Promise<void> => {
    const response = await api.put<ApiResponse<{ id: number; notes: string }>>(`/api/v1/episodes/${id}/notes`, { notes })
    if (!response.data.success) {
      throw new Error(response.data.error?.message || 'Failed to update notes')
    }
  },

  // 获取单集的所有标签
  getTags: async (id: number): Promise<Tag[]> => {
    const response = await api.get<ApiResponse<Tag[]>>(`/api/v1/episodes/${id}/tags`)
    if (response.data.success && response.data.data) {
      return response.data.data
    }
    throw new Error(response.data.error?.message || 'Failed to fetch tags')
  },

  // 为单集添加标签
  addTag: async (id: number, tagId: number): Promise<void> => {
    const response = await api.post<ApiResponse<any>>(`/api/v1/episodes/${id}/tags`, { tag_id: tagId })
    if (!response.data.success) {
      throw new Error(response.data.error?.message || 'Failed to add tag')
    }
  },

  // 移除单集标签
  removeTag: async (id: number, tagId: number): Promise<void> => {
    const response = await api.delete<ApiResponse<any>>(`/api/v1/episodes/${id}/tags/${tagId}`)
    if (!response.data.success) {
      throw new Error(response.data.error?.message || 'Failed to remove tag')
    }
  },
}

export const tagApi = {
  // 获取所有标签
  list: async (): Promise<Tag[]> => {
    const response = await api.get<ApiResponse<Tag[]>>('/api/v1/tags')
    if (response.data.success && response.data.data) {
      return response.data.data
    }
    throw new Error(response.data.error?.message || 'Failed to fetch tags')
  },

  // 获取单个标签详情
  get: async (id: number): Promise<Tag> => {
    const response = await api.get<ApiResponse<Tag>>(`/api/v1/tags/${id}`)
    if (response.data.success && response.data.data) {
      return response.data.data
    }
    throw new Error(response.data.error?.message || 'Failed to fetch tag')
  },

  // 创建标签
  create: async (data: { name: string; description?: string; color?: string }): Promise<Tag> => {
    const response = await api.post<ApiResponse<Tag>>('/api/v1/tags', data)
    if (response.data.success && response.data.data) {
      return response.data.data
    }
    throw new Error(response.data.error?.message || 'Failed to create tag')
  },

  // 更新标签
  update: async (id: number, data: { description?: string; color?: string }): Promise<Tag> => {
    const response = await api.put<ApiResponse<Tag>>(`/api/v1/tags/${id}`, data)
    if (response.data.success && response.data.data) {
      return response.data.data
    }
    throw new Error(response.data.error?.message || 'Failed to update tag')
  },

  // 删除标签
  delete: async (id: number): Promise<void> => {
    const response = await api.delete<ApiResponse<any>>(`/api/v1/tags/${id}`)
    if (!response.data.success) {
      throw new Error(response.data.error?.message || 'Failed to delete tag')
    }
  },
}

// 健康检查
export const healthApi = {
  check: async (): Promise<{ status: string; database: string }> => {
    const response = await api.get('/health')
    return response.data
  },
}

// 同步API
export const syncApi = {
  // 导入OPML文件（使用SSE流式响应）
  importOPMLSSE: async (
    file: File,
    onProgress: (type: string, message: string, current?: number, total?: number) => void
  ): Promise<void> => {
    return new Promise((resolve, reject) => {
      const formData = new FormData()
      formData.append('opml_file', file)

      console.log('[Import] 开始导入，文件:', file.name, '大小:', file.size)

      // 使用AbortController设置超时
      const controller = new AbortController()
      const timeoutId = setTimeout(() => {
        console.error('[Import] 导入超时（10分钟）')
        controller.abort()
        reject(new Error('导入超时（10分钟），可能是网络较慢或文件太大'))
      }, 10 * 60 * 1000) // 10分钟超时

      const startTime = Date.now()
      let messageCount = 0

      // 使用fetch来获取stream
      fetch(`${API_URL}/api/v1/sync/import-sse`, {
        method: 'POST',
        body: formData,
        headers: {},
        signal: controller.signal,
      })
        .then(response => {
          clearTimeout(timeoutId)
          const elapsedTime = Date.now() - startTime
          console.log('[Import] 收到响应，状态:', response.status, '耗时:', elapsedTime + 'ms')

          if (!response.ok) {
            throw new Error(`HTTP error! status: ${response.status}`)
          }

          const reader = response.body?.getReader()
          const decoder = new TextDecoder()

          if (!reader) {
            throw new Error('Response body is null')
          }

          let buffer = '' // 缓冲区，用于处理被截断的消息

          // 读取流
          function readStream() {
            reader.read().then(({ done, value }) => {
              if (done) {
                const totalTime = Date.now() - startTime
                console.log('[Import] 流结束，总耗时:', totalTime + 'ms', '消息数:', messageCount)
                resolve()
                return
              }

              try {
                // 解码数据并追加到缓冲区
                buffer += decoder.decode(value, { stream: true })

                // 按行分割，但保留最后一个可能不完整的行
                const lines = buffer.split('\n')
                buffer = lines.pop() || '' // 保留最后一个可能不完整的行

                for (const line of lines) {
                  const trimmedLine = line.trim()

                  // 跳过空行
                  if (!trimmedLine) {
                    continue
                  }

                  // 跳过SSE注释（用于keepalive）
                  if (trimmedLine.startsWith(':')) {
                    console.log('[Import] 收到ping:', trimmedLine)
                    continue
                  }

                  // 处理data消息
                  if (trimmedLine.startsWith('data: ')) {
                    try {
                      const data = JSON.parse(trimmedLine.slice(6))
                      const { type, message, current, total } = data
                      messageCount++

                      if (messageCount % 50 === 0) {
                        console.log('[Import] 已处理', messageCount, '条消息，最新:', type, message)
                      }

                      onProgress(type, message, current, total)

                      // 如果是complete消息，结束流
                      if (type === 'complete') {
                        const totalTime = Date.now() - startTime
                        console.log('[Import] 收到complete消息，总耗时:', totalTime + 'ms')
                        resolve()
                        reader.cancel()
                        return
                      }
                    } catch (e) {
                      console.error('[Import] 解析SSE消息失败:', e, trimmedLine)
                      // 继续处理下一条消息，不中断整个流程
                    }
                  }
                }

                // 继续读取
                readStream()
              } catch (error) {
                console.error('[Import] 流处理错误:', error)
                reject(error)
              }
            }).catch(error => {
              const totalTime = Date.now() - startTime
              console.error('[Import] 读取错误:', error, '耗时:', totalTime + 'ms', '消息数:', messageCount)

              if (error.name === 'AbortError') {
                reject(new Error('导入被取消'))
              } else {
                reject(error)
              }
            })
          }

          readStream()
        })
        .catch(error => {
          clearTimeout(timeoutId)
          const totalTime = Date.now() - startTime
          console.error('[Import] Fetch错误:', error, '耗时:', totalTime + 'ms', '消息数:', messageCount)

          if (error.name === 'AbortError') {
            reject(new Error('导入超时被取消'))
          } else {
            reject(error)
          }
        })
    })
  },

  // 导入OPML文件（旧版本，用于回退）
  importOPML: async (file: File): Promise<{
    success: boolean
    message: string
    total_podcasts: number
    success_count: number
    failed_count: number
    errors?: string[]
  }> => {
    const formData = new FormData()
    formData.append('opml_file', file)

    const response = await api.post('/api/v1/sync/import', formData, {
      headers: {
        'Content-Type': 'multipart/form-data',
      },
      timeout: 300000, // 5分钟超时，用于处理大量feed导入
    })

    return response.data
  },

  // 同步所有订阅
  syncSubscriptions: async (): Promise<{
    success: boolean
    message: string
    total_podcasts: number
    success_count: number
    failed_count: number
    new_episodes: number
    errors?: string[]
  }> => {
    const response = await api.post('/api/v1/sync/subscriptions', {}, {
      timeout: 300000, // 5分钟超时，用于处理大量订阅同步
    })
    return response.data
  },

  // 获取同步状态
  getStatus: async (): Promise<{
    success: boolean
    total_podcasts: number
    podcast_sources: Record<string, number>
    last_sync_time: string | null
  }> => {
    const response = await api.get('/api/v1/sync/status')
    return response.data
  },
}
