/**
 * 统一的配置管理
 * 集中管理所有硬编码的配置值
 */

export const API_CONFIG = {
  // API超时配置
  TIMEOUTS: {
    DEFAULT: 30000, // 30秒
    SYNC: 30 * 60 * 1000, // 30分钟（元数据同步）
    IMPORT: 10 * 60 * 1000, // 10分钟（OPML导入）
    EPISODE_SYNC: 60 * 60 * 1000, // 60分钟（单集同步）
  },
  // 重试配置
  RETRY: {
    MAX_ATTEMPTS: 3,
    DELAY: 1000, // 1秒
  },
} as const;

export const UI_CONFIG = {
  // 分页配置
  PAGINATION: {
    DEFAULT_PAGE_SIZE: 20,
    MAX_PAGE_SIZE: 100,
  },
  // 搜索配置
  SEARCH: {
    MIN_QUERY_LENGTH: 2,
    DEBOUNCE_MS: 300,
  },
  // 工作流配置
  WORKFLOW: {
    MAX_PODCASTS: 50, // 工作流最多支持的播客数
    MAX_KEYWORDS: 10, // 最多关键词数
  },
  // 自动滚动配置
  AUTO_SCROLL: {
    THRESHOLD: 100, // 距离底部100px时触发
  },
} as const;

export const CRON_PRESETS = [
  { label: "每天凌晨2点", value: "0 2 * * *" },
  { label: "每天早上8点", value: "0 8 * * *" },
  { label: "每天晚上8点", value: "0 20 * * *" },
  { label: "每周日凌晨2点", value: "0 2 * * 0" },
  { label: "每周一早上6点", value: "0 6 * * 1" },
] as const;

export const LOG_CONFIG = {
  // 日志保留
  MAX_LOGS: 1000, // 最大日志条数
  LOG_STORAGE_KEY: "syncLogs",
  // 日志级别
  LEVEL: process.env.NODE_ENV === "development" ? "debug" : "info",
} as const;

export const STORAGE_KEYS = {
  SYNC_LOGS: "syncLogs",
  SYNCING: "syncing",
  IMPORTING: "importing",
  AUTO_SCROLL: "autoScroll",
} as const;
