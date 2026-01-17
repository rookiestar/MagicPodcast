'use client'

import { useSearchParams } from 'next/navigation'
import Link from 'next/link'

export default function Home() {
  const searchParams = useSearchParams()
  const sortBy = searchParams.get('sort_by') || ''

  return (
    <main className="min-h-screen bg-slate-50">
      <div className="container mx-auto px-4 py-20">
        {/* Hero Section */}
        <div className="text-center mb-16">
          {/* Static Icon */}
          <div className="mb-8">
            <div className="text-7xl">🎙️</div>
          </div>

          {/* Title */}
          <h1 className="text-6xl md:text-7xl font-bold mb-4" style={{ letterSpacing: '-0.02em' }}>
            <span className="bg-gradient-to-r from-violet-600 to-indigo-600 bg-clip-text text-transparent">
              Magic
            </span>
            <span className="text-slate-800 mx-2">Podcast</span>
          </h1>

          {/* Subtitle */}
          <p className="text-xl text-slate-600 mb-12 max-w-2xl mx-auto">
            个人播库管理与自动化处理工具
          </p>

          {/* Action Buttons - Light Colors */}
          <div className="flex gap-3 justify-center flex-wrap max-w-3xl mx-auto">
            <Link
              href={`/podcasts${sortBy ? `?sort_by=${sortBy}` : ''}`}
              className="px-6 py-3 bg-white text-slate-800 font-medium rounded-lg border border-slate-300 hover:bg-slate-50 hover:border-slate-400 transition-colors"
            >
              🎧 查看播客列表
            </Link>

            <Link
              href="/tags"
              className="px-6 py-3 bg-white text-slate-800 font-medium rounded-lg border border-slate-300 hover:bg-slate-50 hover:border-slate-400 transition-colors"
            >
              🏷️ 标签管理
            </Link>

            <Link
              href="/workflows"
              className="px-6 py-3 bg-white text-slate-800 font-medium rounded-lg border border-slate-300 hover:bg-slate-50 hover:border-slate-400 transition-colors"
            >
              ⚙️ 工作流管理
            </Link>

            <Link
              href="/import"
              className="px-6 py-3 bg-white text-slate-800 font-medium rounded-lg border border-slate-300 hover:bg-slate-50 hover:border-slate-400 transition-colors"
            >
              📥 导入/同步
            </Link>
          </div>
        </div>

        {/* Feature Cards */}
        <div className="grid md:grid-cols-3 gap-6 max-w-5xl mx-auto">
          <Link
            href={`/podcasts${sortBy ? `?sort_by=${sortBy}` : ''}`}
            className="group"
          >
            <FeatureCard
              emoji="🎙️"
              title="我的订阅管理"
              description="同步小宇宙平台的订阅节目和单集数据"
            />
          </Link>

          <Link href="/tags" className="group">
            <FeatureCard
              emoji="🏷️"
              title="本地标签与备注"
              description="为节目/单集添加自定义标签和备注"
            />
          </Link>

          <Link href="/workflows" className="group">
            <FeatureCard
              emoji="⚙️"
              title="自动化工作流"
              description="基于规则自动抓取播客信息并生成报告"
            />
          </Link>
        </div>

        {/* Footer */}
        <div className="mt-20 text-center">
          <p className="text-slate-400 text-sm">
            项目处于开发阶段 • 阶段 1 实施中
          </p>
        </div>
      </div>
    </main>
  )
}

function FeatureCard({
  emoji,
  title,
  description
}: {
  emoji: string
  title: string
  description: string
}) {
  return (
    <div className="bg-white rounded-xl shadow-sm hover:shadow-md transition-shadow p-8 border border-slate-200">
      <div className="text-5xl mb-4">{emoji}</div>
      <h3 className="text-xl font-semibold text-slate-800 mb-2">
        {title}
      </h3>
      <p className="text-slate-600 text-sm leading-relaxed">
        {description}
      </p>
    </div>
  )
}
