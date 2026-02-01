/**
 * 时间相关工具函数
 */

/**
 * 获取相对时间字符串
 * @param dateString ISO日期字符串
 * @returns 相对时间字符串（如"今天"、"昨天"、"3天前"）
 */
export function getRelativeTime(dateString: string): string {
  const now = new Date();
  const past = new Date(dateString);
  const diffMs = now.getTime() - past.getTime();
  const diffDays = Math.floor(diffMs / (1000 * 60 * 60 * 24));
  const diffHours = Math.floor(diffMs / (1000 * 60 * 60));
  const diffMinutes = Math.floor(diffMs / (1000 * 60));

  // 小于1小时
  if (diffMinutes < 60) {
    if (diffMinutes < 1) return "刚刚";
    return `${diffMinutes}分钟前`;
  }

  // 小于24小时
  if (diffHours < 24) {
    return `${diffHours}小时前`;
  }

  // 今天
  if (diffDays === 0) {
    return "今天";
  }

  // 昨天
  if (diffDays === 1) {
    return "昨天";
  }

  // 小于7天
  if (diffDays < 7) {
    return `${diffDays}天前`;
  }

  // 小于30天，显示周数
  if (diffDays < 30) {
    const weeks = Math.floor(diffDays / 7);
    return `${weeks}周前`;
  }

  // 小于365天，显示月数
  if (diffDays < 365) {
    const months = Math.floor(diffDays / 30);
    return `${months}个月前`;
  }

  // 超过1年，显示年份
  const years = Math.floor(diffDays / 365);
  return `${years}年前`;
}

/**
 * 判断是否是最近更新（7天内有新内容）
 * @param dateString ISO日期字符串
 * @param days 最近天数，默认7天
 * @returns 是否是最近更新
 */
export function isRecentlyUpdated(
  dateString: string,
  days: number = 7,
): boolean {
  const now = new Date();
  const past = new Date(dateString);
  const diffMs = now.getTime() - past.getTime();
  const diffDays = diffMs / (1000 * 60 * 60 * 24);

  return diffDays <= days;
}

/**
 * 格式化日期为本地字符串
 * @param dateString ISO日期字符串
 * @returns 格式化后的日期字符串
 */
export function formatDate(dateString: string): string {
  const date = new Date(dateString);
  return date.toLocaleDateString("zh-CN", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  });
}

/**
 * 格式化日期时间为本地字符串
 * @param dateString ISO日期字符串
 * @returns 格式化后的日期时间字符串
 */
export function formatDateTime(dateString: string): string {
  const date = new Date(dateString);
  return date.toLocaleString("zh-CN", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  });
}
