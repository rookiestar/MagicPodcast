/** @type {import('next').NextConfig} */
const imageOptimizerPath = process.env.NEXT_PUBLIC_IMAGE_OPTIMIZER_PATH || '/_next/image'
const nextDistDir = process.env.MAGICPODCAST_NEXT_DIST_DIR || '.next'

const nextConfig = {
  reactStrictMode: true,
  // 发布流程把新构建放在独立目录，验证通过后才切换为 .next。
  distDir: nextDistDir,
  // 生产构建时移除 console 语句（保留 console.error 和 console.warn）
  compiler: {
    removeConsole: {
      exclude: ['error', 'warn'],
    },
  },
  images: {
    // 启用现代图片格式优化
    formats: ['image/avif', 'image/webp'],

    // 响应式图片设备尺寸
    deviceSizes: [640, 750, 828, 1080, 1200, 1920],

    // 图片尺寸断点
    imageSizes: [96, 128, 256, 384, 512, 640],

    // 远程图片统一经后端白名单代理，不允许 Next 优化器直接访问任意域名。
    remotePatterns: [],

    // 最小缓存TTL（30天，配合后端缓存策略）
    minimumCacheTTL: 60 * 60 * 24 * 30,

    // 图片优化配置
    path: imageOptimizerPath,
    loader: 'default',
    dangerouslyAllowSVG: false,
    contentDispositionType: 'attachment',
    contentSecurityPolicy: "default-src 'self'; script-src 'none'; sandbox;",
  },
  // 增加超时时间以支持长时间运行的SSE连接
  experimental: {
    serverActions: {
      allowedOrigins: ['localhost:3000'],
    },
  },
}

// 导出配置
module.exports = {
  ...nextConfig,
  // API 代理配置 - 兼顾本地开发和域名访问
  async rewrites() {
    const backendUrl = process.env.BACKEND_URL || 'http://127.0.0.1:8080'
    return [
      {
        source: '/api/v1/:path*',
        destination: `${backendUrl}/api/v1/:path*`,
      },
      {
        source: '/images/:path*',
        destination: `${backendUrl}/images/:path*`,
      },
      {
        source: '/health',
        destination: `${backendUrl}/health`,
      },
    ]
  },
}
