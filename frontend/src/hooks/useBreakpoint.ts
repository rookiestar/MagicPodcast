"use client";

import { useState, useEffect, useRef, useCallback } from "react";

const DEFAULT_VIEWPORT_WIDTH = 1280;

export function getColumnsForViewportWidth(width: number): number {
  if (width >= 1280) return 5;
  if (width >= 1024) return 4;
  if (width >= 768) return 3;
  if (width >= 640) return 2;
  return 1;
}

/**
 * 响应式断点检测 Hook
 *
 * 功能：
 * - 检测当前设备类型（移动端 < 640px）
 * - 返回当前网格列数（1-5），用于响应式布局计算
 * - 使用 requestAnimationFrame 节流优化 resize 事件
 *
 * 断点对应列数（与 Tailwind CSS 网格布局一致）：
 * - <640px: 1 列
 * - ≥640px (sm): 2 列
 * - ≥768px (md): 3 列
 * - ≥1024px (lg): 4 列
 * - ≥1280px (xl): 5 列
 *
 * 使用方式：
 * ```typescript
 * const { isMobile, columns } = useBreakpoint();
 * const pageSize = getPageSize(columns); // 获取响应式每页数量
 * ```
 */
export function useBreakpoint() {
  const [isMobile, setIsMobile] = useState(false);
  const [columns, setColumns] = useState(5); // 默认桌面端5列
  const [isReady, setIsReady] = useState(false);
  const tickingRef = useRef(false);

  const checkBreakpoint = useCallback(() => {
    const width = window.innerWidth;
    setIsMobile(width < 640);
    setColumns(getColumnsForViewportWidth(width));
    setIsReady(true);
  }, []);

  // 节流的 resize 处理器
  const handleResize = useCallback(() => {
    if (!tickingRef.current) {
      tickingRef.current = true;
      requestAnimationFrame(() => {
        checkBreakpoint();
        tickingRef.current = false;
      });
    }
  }, [checkBreakpoint]);

  useEffect(() => {
    // 初始检查
    checkBreakpoint();

    // 监听窗口大小变化（带节流）
    window.addEventListener("resize", handleResize, { passive: true });

    // 清理函数
    return () => {
      window.removeEventListener("resize", handleResize);
    };
  }, [checkBreakpoint, handleResize]);

  return { isMobile, columns, isReady };
}

/**
 * 根据列数计算每页加载数量
 * 确保每次加载都是完整的行数
 */
export function getPageSize(columns: number): number {
  switch (columns) {
    case 5:
      return 15; // 3行
    case 4:
      return 12; // 3行
    case 3:
      return 9; // 3行
    case 2:
      return 8; // 4行
    case 1:
      return 10; // 10行
    default:
      return 15;
  }
}

export function getPageSizeForViewportWidth(
  width = DEFAULT_VIEWPORT_WIDTH,
): number {
  return getPageSize(getColumnsForViewportWidth(width));
}
