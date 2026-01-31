'use client'

import { useSearchParams } from 'next/navigation'
import Link from 'next/link'
import { Suspense } from 'react'

function HomeContent() {
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
          <p className="text-xl text-slate-600 mb-16 max-w-2xl mx-auto">
            个人播客管理与自动化处理工具
          </p>
        </div>

        {/* Feature Cards */}
        <div className="grid md:grid-cols-2 gap-6 max-w-4xl mx-auto">
          <Link
            href={`/podcasts${sortBy ? `?sort_by=${sortBy}` : ''}`}
            className="group"
          >
            <FeatureCard
              emoji="🎧"
              title="我的订阅管理"
              description="同步小宇宙平台的订阅节目和单集数据"
            />
          </Link>

          <Link href="/import" className="group">
            <FeatureCard
              emoji="📥"
              title="导入/同步"
              description="导入OPML文件或同步小宇宙订阅数据"
            />
          </Link>

          <Link href="/tags" className="group">
            <FeatureCard
              emoji="🏷️"
              title="标签与备注管理"
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
      </div>
    </main>
  )
}

export default function Home() {
  return (
    <Suspense fallback={<div className="min-h-screen bg-slate-50 flex items-center justify-center">
      <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-blue-600"></div>
    </div>}>
      <HomeContent />
    </Suspense>
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
