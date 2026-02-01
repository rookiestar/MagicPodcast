// API 相关类型定义

export interface ApiResponse<T = any> {
  success: boolean
  data: T
  error?: {
    message: string
    code?: string
  }
}

// 分页信息
export interface Pagination {
  page: number
  page_size: number
  total: number
  total_pages: number
}

// 搜索参数
export interface SearchParams {
  q: string
  type?: 'all' | 'podcasts' | 'episodes'
  tag_id?: number | number[]
  page?: number
  page_size?: number
  episode_page?: number
  episode_page_size?: number
}

// SSE进度回调
export type SSEProgressCallback = (
  type: string,
  message: string,
  current?: number,
  total?: number,
  data?: any
) => void
