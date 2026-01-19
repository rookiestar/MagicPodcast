import type { Metadata } from 'next'
import './globals.css'
import { ToastProvider, setGlobalToastContext } from '@/lib/toast'

export const metadata: Metadata = {
  title: 'MagicPodcast - 个人播库管理',
  description: '个人播库管理与自动化处理工具',
}

export default function RootLayout({
  children,
}: {
  children: React.ReactNode
}) {
  return (
    <html lang="zh-CN">
      <body>
        <ToastProvider>
          {children}
        </ToastProvider>
      </body>
    </html>
  )
}
