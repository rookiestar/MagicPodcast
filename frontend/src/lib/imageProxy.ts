/**
 * 图片代理工具函数
 * 用于代理无法直接访问的图片（如 i.typlog.com）
 */

const IMAGE_PROXY_ENABLED = true; // 是否启用图片代理
const IMAGE_PROXY_BASE_URL =
  process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080";

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
const SHOULD_PROXY_DOMAINS = ["i.typlog.com", "typlog.com"];

/**
 * 预加载图片（使用代理）
 * @param url 图片URL
 */
export function preloadImage(url: string): Promise<void> {
  return new Promise((resolve, reject) => {
    const img = new Image();
    const proxiedUrl = getProxiedImageUrl(url) || url;

    img.onload = () => resolve();
    img.onerror = () => reject(new Error(`Failed to load image: ${url}`));

    img.src = proxiedUrl;
  });
}

/**
 * 批量预加载图片
 * @param urls 图片URL数组
 */
export async function preloadImages(urls: string[]): Promise<void[]> {
  const promises = urls
    .filter((url) => url) // 过滤掉空URL
    .map((url) => preloadImage(url));

  return Promise.allSettled(promises).then((results) => {
    results.forEach((result, index) => {
      if (result.status === "rejected") {
        console.warn(`[preloadImages] Failed to preload: ${urls[index]}`);
      }
    });
    return [];
  });
}
