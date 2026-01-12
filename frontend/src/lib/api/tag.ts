import { api } from './client'
import type { ApiResponse, Tag } from '@/types'

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
