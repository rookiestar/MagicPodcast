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

// 分页响应类型
export interface PaginatedResponse<T> {
  success: boolean
  data?: T
  pagination?: {
    page: number
    page_size: number
    total: number
    total_pages: number
  }
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

// 搜索相关类型定义
export interface MatchedField {
  field: string
  score: number
  snippet: string
}

export interface PodcastSearchResult {
  id: number
  title: string
  author: string
  description: string
  cover_url: string
  episode_count: number
  newest_episode_date: string
  relevance_score: number
  matched_fields?: MatchedField[]  // 改为可选
  tags?: Tag[]
}

export interface EpisodeSearchResult {
  id: number
  podcast_id: number
  podcast_title: string
  podcast_cover_url: string
  title: string
  show_notes: string
  published_date: string | null
  duration: number
  relevance_score: number
  matched_fields?: MatchedField[]  // 改为可选
}

export interface SearchPagination {
  page: number
  page_size: number
  total: number
  total_pages: number
}

export interface SearchResponse {
  podcasts: PodcastSearchResult[]
  episodes: EpisodeSearchResult[]
}

export interface SearchData {
  podcasts: PodcastSearchResult[]
  episodes: EpisodeSearchResult[]
  pagination: {
    podcasts: SearchPagination
    episodes: SearchPagination
  }
}

// Workflow 相关类型定义
export type WorkflowScopeType = 'specific_podcasts' | 'all_subscribed' | 'custom_sources'

export type JobStatus = 'pending' | 'running' | 'completed' | 'failed' | 'cancelled'

export type ExecutionStatus = 'pending' | 'running' | 'success' | 'failed' | 'skipped'

export interface ScopeConfig {
  podcast_ids?: number[]     // 指定节目的ID列表
  custom_urls?: string[]     // 自定义RSS源URL列表
}

export interface RulesConfig {
  time_range?: number        // 时间范围（天），0表示不限制
  min_duration?: number      // 最小时长（秒），0表示不限制
  max_results?: number       // 最大结果数，0表示不限制
  keywords?: string          // 关键词过滤（逗号分隔）
  exclude_words?: string     // 排除词（逗号分隔）
}

export interface Workflow {
  id: number
  name: string
  description: string
  schedule: string           // cron表达式
  scope_type: WorkflowScopeType
  scope_config: ScopeConfig
  rules_config: RulesConfig
  is_enabled: boolean
  created_at: string
  updated_at: string
  last_job?: Job
  stats?: WorkflowStats
}

export interface WorkflowStats {
  total_jobs: number
  success_jobs: number
  failed_jobs: number
  success_rate: number
  total_episodes: number
  last_execution?: string
  next_execution?: string
}

export interface Job {
  id: number
  workflow_id: number
  status: JobStatus
  start_time?: string
  end_time?: string
  podcasts_processed: number
  episodes_found: number
  episodes_created: number
  error_count: number
  triggered_by: string       // 'cron' | 'manual'
  created_at: string
  duration?: number          // 执行时长（毫秒）
  executions?: JobExecution[]
}

export interface JobExecution {
  id: number
  job_id: number
  podcast_id?: number
  podcast_title?: string
  podcast_feed_url?: string
  status: ExecutionStatus
  episodes_found: number
  episodes_created: number
  error_message?: string
  log_info?: string
  processing_time: number    // 毫秒
  created_at: string
}

export interface WorkflowRequest {
  name: string
  description?: string
  schedule: string
  scope_type: WorkflowScopeType
  scope_config: ScopeConfig
  rules_config: RulesConfig
  is_enabled: boolean
}

export interface WorkflowsResponse {
  workflows: Workflow[]
  pagination: {
    page: number
    page_size: number
    total: number
    total_pages: number
  }
}

export interface JobsResponse {
  jobs: Job[]
  pagination: {
    page: number
    page_size: number
    total: number
    total_pages: number
  }
}
