/** @type {import('next').NextConfig} */
const nextConfig = {
  reactStrictMode: true,
  images: {
    // 启用现代图片格式优化
    formats: ['image/avif', 'image/webp'],

    // 响应式图片设备尺寸
    deviceSizes: [640, 750, 828, 1080, 1200, 1920],

    // 图片尺寸断点
    imageSizes: [96, 128, 256, 384, 512, 640],

    // 远程图片模式配置（Next.js 14推荐）
    // 限制：remotePatterns 最多 50 个元素
    // 策略：本地服务优先 + 少量关键域名 + 通配符兜底
    remotePatterns: [
      // ==================== 本地开发服务（必须最先匹配）====================
      {
        protocol: 'http',
        hostname: 'localhost',
        port: '8080',
        pathname: '/images/**',
      },
      {
        protocol: 'http',
        hostname: 'localhost',
        port: '3000',
        pathname: '/api/v1/images/**',
      },

      // ==================== 最常用的播客平台（HTTP必须显式支持）====================
      // 小宇宙/蜻蜓 CDN
      {
        protocol: 'http',
        hostname: '**.xmcdn.com',
      },
      {
        protocol: 'https',
        hostname: '**.xmcdn.com',
      },
      // 荔枝FM
      {
        protocol: 'http',
        hostname: '**.lizhi.fm',
      },
      {
        protocol: 'https',
        hostname: '**.lizhi.fm',
      },
      // Typlog（需要代理）
      {
        protocol: 'http',
        hostname: '**.typlog.com',
      },
      {
        protocol: 'https',
        hostname: '**.typlog.com',
      },
      // Vistopia
      {
        protocol: 'http',
        hostname: '**.vistopia.com.cn',
      },
      {
        protocol: 'https',
        hostname: '**.vistopia.com.cn',
      },
      // XYZ CDN
      {
        protocol: 'http',
        hostname: '**.xyzcdn.net',
      },
      {
        protocol: 'https',
        hostname: '**.xyzcdn.net',
      },

      // ==================== 通用通配符（兜底，允许所有其他域名）====================
      {
        protocol: 'https',
        hostname: '**',
      },
      {
        protocol: 'http',
        hostname: '**',
      },
    ],

    // 最小缓存TTL（30天，配合后端缓存策略）
    minimumCacheTTL: 60 * 60 * 24 * 30,

    // 图片优化配置
    loader: 'default',
    dangerouslyAllowSVG: true,
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
  typescript: {
    // 在build时忽略类型错误
    ignoreBuildErrors: true,
  },
  eslint: {
    // 在build时忽略ESLint错误
    ignoreDuringBuilds: true,
  },
  // API 代理配置 - 兼顾本地开发和域名访问
  async rewrites() {
    const backendUrl = process.env.BACKEND_URL || 'http://localhost:8080'
    return [
      {
        source: '/api/v1/:path*',
        destination: `${backendUrl}/api/v1/:path*`,
      },
      {
        source: '/health',
        destination: `${backendUrl}/health`,
      },
    ]
  },
}
