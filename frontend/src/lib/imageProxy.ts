/**
 * 图片代理工具函数
 * 用于代理无法直接访问的图片（如 i.typlog.com）
 */

const IMAGE_PROXY_ENABLED = true; // 是否启用图片代理
// 在浏览器环境中使用相对路径（支持 tunnel/代理访问）
const IMAGE_PROXY_BASE_URL = typeof window !== "undefined"
  ? (process.env.NEXT_PUBLIC_API_URL || "")  // 浏览器：相对路径或自定义 URL
  : (process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080");  // SSR：需要完整 URL

/**
 * 获取代理后的图片URL
 * @param originalUrl 原始图片URL
 * @returns 代理后的图片URL
 */
export function getProxiedImageUrl(
  originalUrl: string | undefined | null,
): string | undefined {
  // 如果没有原始URL，返回undefined
  if (!originalUrl) {
    return undefined;
  }

  // 如果未启用代理，直接返回原始URL
  if (!IMAGE_PROXY_ENABLED) {
    return originalUrl;
  }

  // 检查是否需要代理的域名
  const needsProxy = SHOULD_PROXY_DOMAINS.some((domain) =>
    originalUrl.includes(domain),
  );

  // 如果不需要代理，直接返回原始URL
  if (!needsProxy) {
    return originalUrl;
  }

  // 构建代理URL
  const proxyUrl = `${IMAGE_PROXY_BASE_URL}/images/proxy?url=${encodeURIComponent(originalUrl)}`;

  return proxyUrl;
}

// 需要代理的域名列表
// 策略：只代理确认有访问问题的域名
const SHOULD_PROXY_DOMAINS = [
  "typlog.com",                    // 墙内访问不稳定
  "i.typlog.com",                  // typlog 图片 CDN
  "wavpub.com",                    // 连接重置问题
  "d3t3ozftmdmh3i.cloudfront.net", // CloudFront CDN 403 问题
];

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
