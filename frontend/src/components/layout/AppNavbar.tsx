"use client";

import React, { useState } from "react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { CompactLogo } from "./Logo";

interface NavItem {
  label: string;
  href: string;
  icon: string;
}

const navItems: NavItem[] = [
  { label: "首页", href: "/", icon: "🏠" },
  { label: "播客", href: "/podcasts", icon: "🎙️" },
  { label: "标签", href: "/tags", icon: "🏷️" },
  { label: "工作流", href: "/workflows", icon: "⚡" },
  { label: "导入", href: "/import", icon: "📥" },
];

interface AppNavbarProps {
  onSearchClick?: () => void;
  syncStatus?: {
    isSyncing: boolean;
    lastSync?: string;
  };
}

/**
 * 全局导航栏组件
 *
 * 功能：
 * - 桌面端：顶部导航栏（Logo + 导航链接 + 搜索按钮）
 * - 移动端：隐藏（使用MobileBottomNav代替）
 * - 固定在顶部，毛玻璃效果
 */
export default function AppNavbar({
  onSearchClick,
  syncStatus,
}: AppNavbarProps) {
  const pathname = usePathname();
  const [scrolled, setScrolled] = useState(false);

  // 监听滚动，添加阴影效果
  React.useEffect(() => {
    const handleScroll = () => {
      setScrolled(window.scrollY > 10);
    };
    window.addEventListener("scroll", handleScroll);
    return () => window.removeEventListener("scroll", handleScroll);
  }, []);

  return (
    <nav
      className={`
        fixed top-0 left-0 right-0 z-50
        bg-white/95 backdrop-blur-sm
        border-b border-slate-200
        transition-shadow duration-200
        ${scrolled ? "shadow-md" : "shadow-sm"}
        hidden md:block
      `}
      style={{ height: "64px" }}
    >
      <div className="container mx-auto px-4 h-full">
        <div className="flex items-center justify-between h-full">
          {/* 左侧：Logo */}
          <Link href="/" prefetch={false} className="flex-shrink-0">
            <CompactLogo size={36} />
          </Link>

          {/* 中间：导航链接 */}
          <div className="flex items-center gap-1">
            {navItems.map((item) => {
              const isActive = pathname === item.href;
              return (
                <Link
                  key={item.href}
                  href={item.href}
                  prefetch={false}
                  className={`
                    px-4 py-2 rounded-lg text-sm font-medium
                    transition-all duration-200
                    flex items-center gap-2
                    ${
                      isActive
                        ? "bg-violet-50 text-violet-700"
                        : "text-slate-600 hover:bg-slate-100 hover:text-slate-800"
                    }
                  `}
                >
                  <span className="text-base">{item.icon}</span>
                  <span>{item.label}</span>
                </Link>
              );
            })}
          </div>

          {/* 右侧：操作区 */}
          <div className="flex items-center gap-3">
            {/* 搜索按钮 */}
            <button
              onClick={onSearchClick}
              className="
                w-10 h-10 rounded-lg
                flex items-center justify-center
                text-slate-600 hover:text-slate-800
                hover:bg-slate-100
                transition-colors
              "
              title="搜索"
            >
              <span className="text-lg">🔍</span>
            </button>

            {/* 同步状态指示器 */}
            {syncStatus && (
              <div
                className={`w-10 h-10 rounded-lg flex items-center justify-center transition-colors ${
                  syncStatus.isSyncing ? "text-blue-600" : "text-slate-400"
                }`}
                title={
                  syncStatus.isSyncing
                    ? "正在同步..."
                    : syncStatus.lastSync
                    ? `上次同步: ${syncStatus.lastSync}`
                    : "未同步"
                }
              >
                <span
                  className={`text-lg ${
                    syncStatus.isSyncing ? "animate-spin" : ""
                  }`}
                >
                  🔄
                </span>
              </div>
            )}
          </div>
        </div>
      </div>
    </nav>
  );
}
