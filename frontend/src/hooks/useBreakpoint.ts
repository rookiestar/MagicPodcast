"use client";

import { useState, useEffect } from "react";

/**
 * 移动端/桌面端设备检测 Hook
 *
 * 功能：
 * - 检测当前设备类型（移动端 < 640px）
 * - 提供统一的设备类型状态给整个应用使用
 * - 避免每个组件重复检测
 *
 * 使用方式：
 * ```typescript
 * const { isMobile } = useBreakpoint();
 * if (isMobile) { // 移动端逻辑 }
 * ```
 *
 * 优势：
 * - ✅ 只有一个resize事件监听器（而不是50个）
 * - ✅ 页面统一管理设备状态
 * - ✅ 组件变成纯展示型，性能提升显著
 */
export function useBreakpoint() {
  const [isMobile, setIsMobile] = useState(false);

  useEffect(() => {
    const checkMobile = () => {
      setIsMobile(window.innerWidth < 640);
    };

    // 初始检查
    checkMobile();

    // 监听窗口大小变化
    window.addEventListener("resize", checkMobile);

    // 清理函数
    return () => {
      window.removeEventListener("resize", checkMobile);
    };
  }, []);

  return { isMobile };
}
