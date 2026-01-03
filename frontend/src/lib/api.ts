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
