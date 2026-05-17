"use client";

import React from "react";
import Link from "next/link";

/**
 * 面包屑项
 */
interface BreadcrumbItem {
  label: string;
  href?: string;
}

/**
 * 工具栏右侧操作按钮
 */
interface ActionButton {
  label: string;
  icon?: string;
  onClick?: () => void;
  href?: string;
  variant?: "primary" | "secondary" | "ghost";
  disabled?: boolean;
}

/**
 * 页面工具栏属性
 */
export interface PageToolbarProps {
  /** 面包屑导航 */
  breadcrumbs?: BreadcrumbItem[];
  /** 页面标题 */
  title?: React.ReactNode;
  /** 页面描述 */
  description?: string;
  /** 右侧操作按钮 */
  actions?: ActionButton[];
  /** 自定义左侧内容（替代面包屑和标题） */
  leftContent?: React.ReactNode;
  /** 自定义右侧内容（替代操作按钮） */
  rightContent?: React.ReactNode;
  /** 是否固定在顶部 */
  sticky?: boolean;
  /** 额外的CSS类名 */
  className?: string;
}

/**
 * 页面工具栏组件
 *
 * 功能：
 * - 显示面包屑导航、标题、描述
 * - 右侧操作按钮区
 * - 支持固定模式（sticky）或普通模式
 * - 支持完全自定义左右两侧内容
 */
export default function PageToolbar({
  breadcrumbs,
  title,
  description,
  actions,
  leftContent,
  rightContent,
  sticky = true,
  className = "",
}: PageToolbarProps) {
  const toolbarStyles = sticky
    ? "sticky top-16 md:top-16 z-40 bg-white border-b border-slate-200"
    : "bg-transparent";

  return (
    <div
      className={`
        ${toolbarStyles}
        transition-all duration-200
        ${className}
      `}
    >
      <div className="container mx-auto px-4">
        {/* 移动端：极简布局 */}
        <div className="md:hidden py-3">
          <div className="flex items-center justify-between gap-3">
            {/* 左侧：返回按钮 + 标题 */}
            <div className="flex items-center gap-3 flex-1 min-w-0">
              {breadcrumbs && breadcrumbs.length > 0 && breadcrumbs[0] && (
                <Link
                  href={breadcrumbs[0].href}
                  prefetch={false}
                  className="flex-shrink-0 text-2xl text-slate-600 hover:text-slate-800 active:text-slate-900 active:scale-95 transition-all duration-200"
                  aria-label="返回"
                >
                  ←
                </Link>
              )}
              {title && (
                <h1 className="text-base font-semibold text-slate-800 truncate">
                  {title}
                </h1>
              )}
            </div>

            {/* 右侧：自定义内容或操作按钮 */}
            <div className="flex items-center gap-2 flex-shrink-0">
              {rightContent ||
                (actions &&
                  actions.map((action, index) => {
                    const ButtonComponent = action.href ? Link : "button";
                    return (
                      <ButtonComponent
                        key={index}
                        href={action.href}
                        {...(ButtonComponent !== "button" && { prefetch: false })}
                        onClick={action.onClick}
                        disabled={action.disabled}
                        className="w-10 h-10 flex items-center justify-center rounded-lg bg-slate-100 hover:bg-slate-200 text-slate-600 transition-colors"
                        {...(ButtonComponent === "button" && { type: "button" })}
                      >
                        {action.icon && <span>{action.icon}</span>}
                      </ButtonComponent>
                    );
                  }))}
            </div>
          </div>
        </div>

        {/* 桌面端：完整布局 */}
        <div className="hidden md:flex items-center justify-between gap-4 py-3">
          {/* 左侧：面包屑 + 标题 + 描述 */}
          <div className="flex-1 min-w-0">
            {leftContent || (
              <div className="flex items-center gap-6">
                {/* 面包屑导航 */}
                {breadcrumbs && breadcrumbs.length > 0 && (
                  <div className="flex items-center gap-2 text-sm">
                    {breadcrumbs.map((item, index) => (
                      <React.Fragment key={index}>
                        {index > 0 && (
                          <span className="text-slate-400">/</span>
                        )}
                        {item.href ? (
                          <Link
                            href={item.href}
                            prefetch={false}
                            className="
                              text-slate-600 hover:text-slate-800
                              transition-colors
                              cursor-pointer
                              px-2 py-3 -mx-2 -my-3
                              min-h-[44px]
                              inline-flex items-center
                              active:scale-95
                            "
                          >
                            {item.label}
                          </Link>
                        ) : (
                          <span className="text-slate-800 font-medium">
                            {item.label}
                          </span>
                        )}
                      </React.Fragment>
                    ))}
                  </div>
                )}

                {/* 标题和描述 */}
                {(title || description) && (
                  <div className="flex flex-col">
                    {title && (
                      <h1 className="text-xl font-semibold text-slate-800">
                        {title}
                      </h1>
                    )}
                    {description && (
                      <p className="text-sm text-slate-600">{description}</p>
                    )}
                  </div>
                )}
              </div>
            )}
          </div>

          {/* 右侧：操作按钮 */}
          <div className="flex items-center gap-2 flex-shrink-0">
            {rightContent ||
              (actions &&
                actions.map((action, index) => {
                  const ButtonComponent = action.href ? Link : "button";
                  const baseStyles =
                    "px-4 py-2 rounded-lg text-sm font-medium transition-all duration-200 flex items-center gap-2";
                  const variantStyles = {
                    primary:
                      "bg-gradient-to-r from-violet-600 to-indigo-600 text-white hover:from-violet-700 hover:to-indigo-700 shadow-sm",
                    secondary:
                      "bg-white text-slate-700 border border-slate-300 hover:bg-slate-50 hover:border-slate-400",
                    ghost: "text-slate-600 hover:bg-slate-100",
                  };

                  return (
                    <ButtonComponent
                      key={index}
                      href={action.href}
                      {...(ButtonComponent !== "button" && { prefetch: false })}
                      onClick={action.onClick}
                      disabled={action.disabled}
                      className={`${baseStyles} ${variantStyles[action.variant || "secondary"]} ${
                        action.disabled ? "opacity-50 cursor-not-allowed" : ""
                      }`}
                      {...(ButtonComponent === "button" && { type: "button" })}
                    >
                      {action.icon && <span>{action.icon}</span>}
                      <span>{action.label}</span>
                    </ButtonComponent>
                  );
                }))}
          </div>
        </div>
      </div>
    </div>
  );
}

/**
 * 预设的播客列表页工具栏
 */
export function PodcastListToolbar({
  onSortChange,
  sortBy,
  sortOptions,
}: {
  onSortChange: (value: string) => void;
  sortBy: string;
  sortOptions: { label: string; value: string }[];
}) {
  return (
    <div className="sticky top-16 md:top-16 z-40 bg-white border-b border-slate-200">
      <div className="container mx-auto px-4 py-3">
        <div className="flex items-center justify-between gap-4">
          {/* 左侧：面包屑 */}
          <Link
            href="/"
            prefetch={false}
            className="text-sm text-slate-600 hover:text-slate-800 active:text-slate-900 hover:bg-slate-100 active:bg-slate-200 active:scale-95 transition-all duration-200 px-2 py-3 -mx-2 -my-3 rounded-lg inline-flex items-center gap-1"
          >
            <span>←</span>
            <span>返回首页</span>
          </Link>

          {/* 右侧：排序选择器 */}
          <div className="flex items-center gap-2">
            <span className="text-sm text-slate-600">排序：</span>
            <select
              value={sortBy}
              onChange={(e) => onSortChange(e.target.value)}
              className="
                px-3 py-2 pr-8
                border border-slate-300 rounded-lg
                bg-white text-sm text-slate-700
                focus:ring-2 focus:ring-violet-500 focus:border-transparent
                transition-colors
                appearance-none
                cursor-pointer
              "
            >
              {sortOptions.map((option) => (
                <option key={option.value} value={option.value}>
                  {option.label}
                </option>
              ))}
            </select>
          </div>
        </div>
      </div>
    </div>
  );
}
