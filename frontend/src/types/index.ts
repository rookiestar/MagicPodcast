// Podcast 类型定义
export interface Podcast {
  id: number
  xyz_id: string
  title: string
  description: string
  author: string
  cover_url: string
  episode_count: number
  newest_episode_date: string
  created_at: string
  tags?: Tag[]
}

// API 响应类型
export interface ApiResponse<T> {
  success: boolean
  data?: T
  error?: {
    code: string
    message: string
  }
}

// Episode 类型定义（暂未使用）
export interface Episode {
  id: number
  xyz_id: string
  podcast_id: number
  episode_no: string
  title: string
  medium_url: string
  show_notes: string
  published_date: string
  my_rate: number
  notes: string
}

// Tag 类型定义（暂未使用）
export interface Tag {
  id: number
  name: string
  description: string
  color: string
}
