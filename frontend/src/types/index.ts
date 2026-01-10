// Podcast 类型定义
export interface Podcast {
  id: number
  xyz_id: string
  title: string
  description: string
  author: string
  cover_url: string
  feed_url?: string
  episode_count: number
  newest_episode_date: string
  created_at: string
  added_date?: string
  is_subscribed: boolean
  is_dead: boolean
  my_rate?: number
  notes?: string
  data_source?: string

  // 🆕 PodcastIndex 新增字段（可选）
  link?: string                        // 播客网站链接
  newest_enclosure_url?: string        // 最新单集音频URL
  newest_enclosure_duration?: number   // 最新单集时长（秒）
  last_update?: string                 // Feed最后更新时间
  oldest_episode_date?: string         // 最旧单集发布日期
  popularity_score?: number            // 受欢迎程度 (0-10)
  priority?: number                    // 抓取优先级 (0-10, -1=暂停)
  update_frequency?: number            // 更新频率 (0-10)

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

// Episode 类型定义
export interface Episode {
  id: number
  guid: string
  podcast_id: number
  episode_no: string
  title: string
  medium_url: string
  show_notes: string
  published_date: string
  duration: number           // 音频时长（秒）
  link: string              // 单集网页链接
  image_url: string         // 单集封面图URL
  enclosure_type: string    // 音频MIME类型
  enclosure_length: number  // 音频文件大小（字节）
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
