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

    // 允许的图片域名（包括播客封面和图片代理）
    domains: [
      'localhost',
      // 小宇宙相关域名
      'i.typlog.com',
      'typlog.com',
      'image.xyzcdn.net',
      'bts-image.xyzcdn.net',
      // 中文播客平台
      'fdfs.xmcdn.com',
      'cdn.lizhi.fm',
      'cdn.vistopia.com.cn',
      'cdn5.vistopia.com.cn',
      'radio-res.cgtn.com',
      // 国际播客托管平台
      'storage.buzzsprout.com',
      'content.production.cdn.art19.com',
      'img.transistorcdn.com',
      'megaphone.imgix.net',
      'media.redcircle.com',
      'media24.fireside.fm',
      'media.wavpub.com',
      'media.smfm2016.com',
      'pan.icu',
      's.anyway.red',
      'cdn.justinbot.com',
      // 独立播客域名
      'lexfridman.com',
      'crazy.capital',
      'host.podapi.xyz',
      'assets.pippa.io',
      // CDN域名
      'd3t3ozftmdmh3i.cloudfront.net',
      // Apple Podcasts (iTunes) 域名
      'is1-ssl.mzstatic.com',
      'is2-ssl.mzstatic.com',
      'is3-ssl.mzstatic.com',
      'is4-ssl.mzstatic.com',
      'is5-ssl.mzstatic.com',
      'a1.mzstatic.com',
      'a2.mzstatic.com',
      'a3.mzstatic.com',
      'a4.mzstatic.com',
      'a5.mzstatic.com',
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
