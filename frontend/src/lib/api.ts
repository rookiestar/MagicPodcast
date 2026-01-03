import axios from 'axios'
import type { ApiResponse, Podcast } from '@/types'

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
  list: async (): Promise<Podcast[]> => {
    const response = await api.get<ApiResponse<Podcast[]>>('/api/v1/podcasts')
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
}

// 健康检查
export const healthApi = {
  check: async (): Promise<{ status: string; database: string }> => {
    const response = await api.get('/health')
    return response.data
  },
}
