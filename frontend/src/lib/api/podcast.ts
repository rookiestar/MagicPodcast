import { api } from './client'
import type { ApiResponse, Podcast, Tag } from '@/types'

export const podcastApi = {
  // 获取播客列表
  list: async (params?: {
    tag_id?: number | number[]
    page?: number
    page_size?: number
  }): Promise<{ data: Podcast[]; pagination: { page: number; page_size: number; total: number; total_pages: number } }> => {
    const queryParams = new URLSearchParams()

    if (params?.tag_id) {
      // 支持多个tag_id（数组）
      if (Array.isArray(params.tag_id)) {
        params.tag_id.forEach(id => queryParams.append('tag_id', id.toString()))
      } else {
        queryParams.append('tag_id', params.tag_id.toString())
      }
    }

    // 添加分页参数
    if (params?.page) queryParams.append('page', params.page.toString())
    if (params?.page_size) queryParams.append('page_size', params.page_size.toString())

    const url = queryParams.toString()
      ? `/api/v1/podcasts?${queryParams.toString()}`
      : '/api/v1/podcasts'

    console.log('[podcastApi.list] Requesting:', url)

    const response = await api.get<{
      success: boolean
      data: Podcast[]
      pagination: { page: number; page_size: number; total: number; total_pages: number }
      error?: { message: string }
    }>(url)

    console.log('[podcastApi.list] Response:', response.data)

    if (response.data.success && response.data.data) {
      return {
        data: response.data.data,
        pagination: response.data.pagination || {
          page: 1,
          page_size: 15,
          total: response.data.data.length,
          total_pages: 1
        }
      }
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
