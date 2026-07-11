export const LOG_CONFIG = {
  // 日志保留
  MAX_LOGS: 1000, // 最大日志条数
  LOG_STORAGE_KEY: "syncLogs",
  // 日志级别
  LEVEL: process.env.NODE_ENV === "development" ? "debug" : "info",
} as const;

export const STORAGE_KEYS = {
  SYNC_LOGS: "syncLogs",
  SYNC_LOG_MODE: "syncLogMode",
  SYNCING: "syncing",
  IMPORTING: "importing",
  AUTO_SCROLL: "autoScroll",
} as const;
