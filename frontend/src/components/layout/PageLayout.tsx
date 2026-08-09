"use client";

import React from "react";
import AppNavbar from "./AppNavbar";
import MobileBottomNav from "./MobileBottomNav";
import PageToolbar, { PageToolbarProps } from "./PageToolbar";
import SearchSidebar from "@/components/SearchSidebar";
import { useSearch } from "@/contexts/SearchContext";

interface PageLayoutProps {
  children: React.ReactNode;
  /** 页面工具栏配置，传false则不显示工具栏 */
  toolbar?: PageToolbarProps | false;
  /** 是否显示全局导航栏（默认true） */
  showNavbar?: boolean;
  /** 是否显示底部导航栏（移动端，默认true） */
  showBottomNav?: boolean;
  /** 同步状态 */
  syncStatus?: {
    isSyncing: boolean;
    lastSync?: string;
  };
  /** 搜索回调 */
  onSearchClick?: () => void;
  /** 额外的CSS类名 */
  className?: string;
  /** 页面最外层的CSS类名 */
  rootClassName?: string;
  /** 最大宽度样式（默认container mx-auto） */
  maxWidth?: boolean;
}

/**
 * 页面布局容器组件
 *
 * 功能：
 * - 统一的页面布局结构
 * - 自动处理顶部空间（为固定导航栏留白）
 * - 集成全局导航栏、页面工具栏
 * - 全局搜索侧边栏
 * - 响应式布局
 *
 * 布局结构：
 * - 桌面端：导航栏(64px) + 工具栏(56px) + 内容
 * - 移动端：内容 + 底部导航栏(60px)
 */
export default function PageLayout({
  children,
  toolbar,
  showNavbar = true,
  showBottomNav = true,
  syncStatus,
  onSearchClick,
  className = "",
  rootClassName = "",
  maxWidth = true,
}: PageLayoutProps) {
  const { isSearchOpen, openSearch, closeSearch } = useSearch();

  // 默认搜索行为：打开全局搜索侧边栏
  const handleSearchClick = () => {
    if (onSearchClick) {
      onSearchClick();
    } else {
      // 默认行为：打开全局搜索侧边栏
      openSearch();
    }
  };

  // 计算顶部的padding
  const paddingTop = showNavbar ? "64px" : "0px";
  // 移动端底部padding（为底部导航栏留空间）
  const paddingBottom = showBottomNav ? "60px" : "0px";
  const defaultRootBackground = rootClassName ? "" : "bg-slate-50";

  return (
    <div className={`min-h-screen ${defaultRootBackground} ${rootClassName}`}>
      {/* 全局导航栏（桌面端） */}
      {showNavbar && (
        <AppNavbar
          onSearchClick={handleSearchClick}
          syncStatus={syncStatus}
        />
      )}

      {/* 主内容区域 */}
      <div
        className={className}
        style={{
          paddingTop,
          paddingBottom: paddingBottom,
        }}
      >
        {/* 页面工具栏 */}
        {toolbar && <PageToolbar {...toolbar} />}

        {/* 内容区域 */}
        <div className={maxWidth ? "container mx-auto px-4" : ""}>
          {children}
        </div>
      </div>

      {/* 底部导航栏（移动端） */}
      {showBottomNav && (
        <MobileBottomNav onSearchClick={handleSearchClick} />
      )}

      {/* 全局搜索侧边栏 */}
      <SearchSidebar isOpen={isSearchOpen} onClose={closeSearch} />
    </div>
  );
}

/**
 * 简化版布局：不显示工具栏
 */
export function SimplePageLayout(props: Omit<PageLayoutProps, "toolbar">) {
  return <PageLayout {...props} toolbar={false} />;
}
