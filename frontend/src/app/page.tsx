import Link from 'next/link'

export default function Home() {
  return (
    <main className="min-h-screen bg-gradient-to-b from-slate-50 to-slate-100 dark:from-slate-900 dark:to-slate-800">
      <div className="container mx-auto px-4 py-16">
        <div className="text-center mb-16">
          <h1 className="text-5xl font-bold text-slate-900 dark:text-slate-50 mb-4">
            🎧 MagicPodcast
          </h1>
          <p className="text-xl text-slate-600 dark:text-slate-400 mb-8">
            个人播库管理与自动化处理工具
          </p>
          <div className="flex gap-4 justify-center">
            <Link
              href="/podcasts"
              className="px-6 py-3 bg-blue-600 text-white rounded-lg hover:bg-blue-700 transition-colors"
            >
              查看播客列表
            </Link>
            <Link
              href="/tags"
              className="px-6 py-3 bg-purple-600 text-white rounded-lg hover:bg-purple-700 transition-colors"
            >
              标签管理
            </Link>
            <Link
              href="/import"
              className="px-6 py-3 bg-green-600 text-white rounded-lg hover:bg-green-700 transition-colors"
            >
              导入/同步
            </Link>
            <a
              href="http://localhost:8080/health"
              target="_blank"
              rel="noopener noreferrer"
              className="px-6 py-3 bg-slate-600 text-white rounded-lg hover:bg-slate-700 transition-colors"
            >
              API 健康检查
            </a>
          </div>
        </div>

        <div className="grid md:grid-cols-3 gap-8 max-w-5xl mx-auto">
          <FeatureCard
            emoji="🎧"
            title="我的订阅管理"
            description="同步小宇宙平台的订阅节目和单集数据"
          />
          <FeatureCard
            emoji="🏷️"
            title="本地标签与备注"
            description="为节目/单集添加自定义标签和备注"
          />
          <FeatureCard
            emoji="⚙️"
            title="自动化工作流"
            description="基于规则自动抓取播客信息并生成报告"
          />
        </div>

        <div className="mt-16 text-center text-slate-500 dark:text-slate-400">
          <p>项目处于开发阶段 • 阶段 1 实施中</p>
        </div>
      </div>
    </main>
  )
}

function FeatureCard({
  emoji,
  title,
  description,
}: {
  emoji: string
  title: string
  description: string
}) {
  return (
    <div className="bg-white dark:bg-slate-800 rounded-lg shadow-lg p-6 hover:shadow-xl transition-shadow">
      <div className="text-4xl mb-4">{emoji}</div>
      <h3 className="text-xl font-semibold text-slate-900 dark:text-slate-50 mb-2">
        {title}
      </h3>
      <p className="text-slate-600 dark:text-slate-400">{description}</p>
    </div>
  )
}
