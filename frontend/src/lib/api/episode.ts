import { api } from './client'
import type { ApiResponse, Episode, Tag } from '@/types'

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
