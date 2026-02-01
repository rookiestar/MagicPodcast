"use client";

import React from "react";
import Link from "next/link";
import { usePathname } from "next/navigation";

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
  { label: "更多", href: "/import", icon: "•••" },
];

interface MobileBottomNavProps {
  onSearchClick?: () => void;
}

/**
 * 移动端底部导航栏组件
 *
 * 功能：
 * - 仅在移动端显示（<768px）
 * - 固定在底部
 * - 包含主要导航项
 * - 搜索按钮作为独立项
 */
export default function MobileBottomNav({
  onSearchClick,
}: MobileBottomNavProps) {
  const pathname = usePathname();

  return (
    <nav
      className="
        fixed bottom-0 left-0 right-0 z-50
        bg-white/95 backdrop-blur-sm
        border-t border-slate-200
        md:hidden
        safe-area-inset-bottom
      "
      style={{ height: "60px" }}
    >
      <div className="flex items-center justify-around h-full px-2">
        {navItems.map((item) => {
          const isActive = pathname === item.href;
          return (
            <Link
              key={item.href}
              href={item.href}
              className={`
                flex flex-col items-center justify-center
                gap-0.5
                flex-1
                transition-all duration-200
                ${isActive ? "text-violet-600" : "text-slate-500"}
              `}
              style={{ minHeight: "48px" }}
            >
              <span
                className={`
                  text-xl
                  ${isActive ? "scale-110" : "scale-100"}
                  transition-transform
                `}
              >
                {item.icon}
              </span>
              <span
                className={`
                  text-[10px] font-medium
                  ${isActive ? "font-semibold" : ""}
                `}
              >
                {item.label}
              </span>
            </Link>
          );
        })}

        {/* 搜索按钮 */}
        <button
          onClick={onSearchClick}
          className="
            flex flex-col items-center justify-center
            gap-0.5
            flex-1
            text-slate-500
            transition-all duration-200
          "
          style={{ minHeight: "48px" }}
        >
          <span className="text-xl">🔍</span>
          <span className="text-[10px] font-medium">搜索</span>
        </button>
      </div>
    </nav>
  );
}
