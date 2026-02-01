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
    remotePatterns: [
      // 本地开发
      {
        protocol: 'http',
        hostname: 'localhost',
        port: '3000',
        pathname: '/api/v1/images/**',
      },
      // 小宇宙相关域名
      {
        protocol: 'https',
        hostname: '**.typlog.com',
      },
      {
        protocol: 'https',
        hostname: '**.xyzcdn.net',
      },
      // 中文播客平台
      {
        protocol: 'https',
        hostname: '**.xmcdn.com',
      },
      {
        protocol: 'https',
        hostname: '**.lizhi.fm',
      },
      {
        protocol: 'https',
        hostname: '**.vistopia.com.cn',
      },
      {
        protocol: 'https',
        hostname: 'radio-res.cgtn.com',
      },
      // 国际播客托管平台
      {
        protocol: 'https',
        hostname: '**.buzzsprout.com',
      },
      {
        protocol: 'https',
        hostname: '**.art19.com',
      },
      {
        protocol: 'https',
        hostname: '**.transistorcdn.com',
      },
      {
        protocol: 'https',
        hostname: '**.imgix.net',
      },
      {
        protocol: 'https',
        hostname: '**.redcircle.com',
      },
      {
        protocol: 'https',
        hostname: '**.fireside.fm',
      },
      {
        protocol: 'https',
        hostname: '**.wavpub.com',
      },
      {
        protocol: 'https',
        hostname: '**.smfm2016.com',
      },
      {
        protocol: 'https',
        hostname: 'pan.icu',
      },
      {
        protocol: 'https',
        hostname: 's.anyway.red',
      },
      {
        protocol: 'https',
        hostname: '**.justinbot.com',
      },
      // 独立播客域名
      {
        protocol: 'https',
        hostname: 'lexfridman.com',
      },
      {
        protocol: 'https',
        hostname: 'crazy.capital',
      },
      {
        protocol: 'https',
        hostname: '**.podapi.xyz',
      },
      {
        protocol: 'https',
        hostname: '**.pippa.io',
      },
      // CDN域名
      {
        protocol: 'https',
        hostname: '**.cloudfront.net',
      },
      // Apple Podcasts (iTunes) 域名
      {
        protocol: 'https',
        hostname: '**.mzstatic.com',
      },
      // 其他常见播客图片域名
      {
        protocol: 'https',
        hostname: '**',
        pathname: '/**//**.{jpg,jpeg,png,gif,webp,avif,svg}',
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

// 添加API代理重写规则
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
  async rewrites() {
    return [
      {
        source: '/api/v1/:path*',
        destination: 'http://localhost:8080/api/v1/:path*',
      },
    ]
  },
}
