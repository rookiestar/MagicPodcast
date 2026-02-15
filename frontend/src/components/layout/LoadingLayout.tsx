"use client";

import React from "react";

interface LoadingLayoutProps {
  children: React.ReactNode;
  /** 工具栏标题 */
  title?: string;
  /** 工具栏描述 */
  description?: string;
  /** 工具栏右侧内容骨架 */
  rightContent?: React.ReactNode;
  /** 是否显示返回链接 */
  showBack?: boolean;
}

/**
 * 加载状态布局组件
 * 提供与 PageLayout 一致的骨架屏结构
 */
export default function LoadingLayout({
  children,
  title,
  description,
  rightContent,
  showBack = false,
}: LoadingLayoutProps) {
  return (
    <div className="min-h-screen bg-slate-50">
      {/* 顶部导航栏骨架 */}
      <header className="fixed top-0 left-0 right-0 h-16 bg-white border-b border-slate-200 z-50 animate-pulse">
        <div className="container mx-auto h-full px-4 flex items-center justify-between">
          <div className="flex items-center gap-6">
            {/* Logo */}
            <div className="h-6 w-32 bg-slate-200 rounded" />
            {/* 导航链接 */}
            <div className="hidden md:flex items-center gap-4">
              <div className="h-5 w-16 bg-slate-200 rounded" />
              <div className="h-5 w-16 bg-slate-200 rounded" />
              <div className="h-5 w-20 bg-slate-200 rounded" />
            </div>
          </div>
          {/* 搜索和操作按钮 */}
          <div className="flex items-center gap-3">
            <div className="h-9 w-48 bg-slate-200 rounded-lg" />
            <div className="h-9 w-9 bg-slate-200 rounded-full" />
          </div>
        </div>
      </header>

      {/* 主内容区域 */}
      <div style={{ paddingTop: "64px", paddingBottom: "60px" }}>
        {/* 页面工具栏骨架 */}
        {(title || showBack) && (
          <div className="bg-white border-b border-slate-200 animate-pulse">
            <div className="container mx-auto px-4 py-4">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-4">
                  {showBack && (
                    <div className="h-6 w-20 bg-slate-200 rounded" />
                  )}
                  <div className="space-y-2">
                    {title && (
                      <div className="h-6 w-32 bg-slate-200 rounded" />
                    )}
                    {description && (
                      <div className="h-4 w-48 bg-slate-200 rounded" />
                    )}
                  </div>
                </div>
                {/* rightContent 由调用方控制动画 */}
              </div>
            </div>
          </div>
        )}

        {/* 内容区域 */}
        <div className="container mx-auto px-4">
          {children}
        </div>
      </div>

      {/* 底部导航栏骨架（移动端） */}
      <nav className="md:hidden fixed bottom-0 left-0 right-0 h-[60px] bg-white border-t border-slate-200 animate-pulse">
        <div className="h-full flex items-center justify-around px-4">
          {Array.from({ length: 4 }).map((_, i) => (
            <div key={i} className="flex flex-col items-center gap-1">
              <div className="h-6 w-6 bg-slate-200 rounded" />
              <div className="h-3 w-8 bg-slate-200 rounded" />
            </div>
          ))}
        </div>
      </nav>
    </div>
  );
}
