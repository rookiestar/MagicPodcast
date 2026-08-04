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
  /** 视觉基调：slate（默认，保持现有播客加载态）或 editorial（暖纸编辑感，匹配 editorial-page-shell） */
  tone?: "slate" | "editorial";
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
  tone = "slate",
}: LoadingLayoutProps) {
  if (tone === "editorial") {
    return <EditorialLoadingLayout title={title} description={description} rightContent={rightContent} showBack={showBack}>{children}</EditorialLoadingLayout>;
  }

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

/**
 * 编辑感加载骨架：与 editorial-page-shell 视觉一致。
 */
function EditorialLoadingLayout({
  children,
  title,
  description,
  rightContent,
  showBack,
}: Omit<LoadingLayoutProps, "tone">) {
  return (
    <div className="editorial-loading min-h-screen">
      {/* 顶部导航栏骨架 */}
      <header className="editorial-loading-bar is-navbar fixed top-0 left-0 right-0 h-16 z-50 animate-pulse">
        <div className="container mx-auto h-full px-4 flex items-center justify-between">
          <div className="flex items-center gap-6">
            <div className="editorial-loading-block h-6 w-32" />
            <div className="hidden md:flex items-center gap-4">
              <div className="editorial-loading-block h-5 w-16" />
              <div className="editorial-loading-block h-5 w-16" />
              <div className="editorial-loading-block h-5 w-20" />
            </div>
          </div>
          <div className="flex items-center gap-3">
            <div className="editorial-loading-block h-9 w-48" />
            <div className="editorial-loading-block h-9 w-9" />
          </div>
        </div>
      </header>

      {/* 主内容区域 */}
      <div style={{ paddingTop: "64px", paddingBottom: "60px" }}>
        {/* 页面工具栏骨架 */}
        {(title || showBack) && (
          <div className="editorial-loading-bar animate-pulse">
            <div className="container mx-auto px-4 py-4">
              <div className="editorial-loading-toolbar-row flex flex-wrap items-center justify-between gap-x-4 gap-y-3">
                <div className="editorial-loading-toolbar-main flex items-center gap-4 min-w-0">
                  {showBack && (
                    <div className="editorial-loading-block h-5 w-20" />
                  )}
                  <div className="space-y-2">
                    {title && (
                      <div className="editorial-loading-block h-6 w-36" />
                    )}
                    {description && (
                      <div className="editorial-loading-block h-4 w-48" />
                    )}
                  </div>
                </div>
                {rightContent && (
                  <div className="editorial-loading-toolbar-actions max-w-full">
                    {rightContent}
                  </div>
                )}
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
      <nav className="editorial-loading-bar is-navbar md:hidden fixed bottom-0 left-0 right-0 h-[60px] animate-pulse">
        <div className="h-full flex items-center justify-around px-4">
          {Array.from({ length: 4 }).map((_, i) => (
            <div key={i} className="flex flex-col items-center gap-1">
              <div className="editorial-loading-block h-6 w-6" />
              <div className="editorial-loading-block h-3 w-8" />
            </div>
          ))}
        </div>
      </nav>
    </div>
  );
}
