/**
 * 图片代理工具函数
 * 用于代理无法直接访问的图片（如 i.typlog.com）
 */

import { getSafeImageSource } from "./imageSourcePolicy";

/**
 * 获取代理后的图片URL
 * @param originalUrl 原始图片URL
 * @returns 代理后的图片URL
 */
export function getProxiedImageUrl(
  originalUrl: string | undefined | null,
): string | undefined {
  return getSafeImageSource(originalUrl);
}

/**
 * 获取有效的封面URL（优先使用自定义封面）
 * @param customCoverUrl 自定义封面URL（用户设置，不会被同步覆盖）
 * @param originalCoverUrl 原始封面URL（来自RSS/PodcastIndex）
 * @returns 有效的封面URL（优先返回自定义封面）
 */
export function getEffectiveCoverUrl(
  customCoverUrl: string | undefined | null,
  originalCoverUrl: string | undefined | null,
): string | undefined {
  // 优先使用自定义封面
  if (customCoverUrl) {
    return customCoverUrl;
  }
  // 否则使用原始封面
  return originalCoverUrl || undefined;
}
