import { SWRConfiguration } from "swr";

// 全局 SWR 配置
export const swrConfig: SWRConfiguration = {
  revalidateOnFocus: false, // 窗口聚焦时不自动重新验证
  revalidateOnReconnect: true, // 网络重连时重新验证
  shouldRetryOnError: false, // 错误时不自动重试
  dedupingInterval: 5000, // 5秒内相同请求去重
  errorRetryCount: 0, // 错误重试次数
};

// 不同数据类型的缓存策略
export const cacheStrategies = {
  // 播客列表 - 短缓存，频繁更新
  podcasts: {
    dedupingInterval: 10000, // 10秒去重
    revalidateOnFocus: true, // 窗口聚焦时刷新
  },
  // 播客详情 - 中等缓存
  podcastDetail: {
    dedupingInterval: 30000, // 30秒去重
    revalidateOnFocus: false,
  },
  // 标签列表 - 长缓存，不常变化
  tags: {
    dedupingInterval: 60000, // 60秒去重
    revalidateOnFocus: false,
  },
  // 工作流列表 - 中等缓存
  workflows: {
    dedupingInterval: 30000, // 30秒去重
    revalidateOnFocus: false,
  },
  // 单集列表 - 中等缓存
  episodes: {
    dedupingInterval: 30000,
    revalidateOnFocus: false,
  },
};
