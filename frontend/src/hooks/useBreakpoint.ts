"use client";

import { useState, useEffect, useRef, useCallback } from "react";

/**
 * 移动端/桌面端设备检测 Hook
 *
 * 功能：
 * - 检测当前设备类型（移动端 < 640px）
 * - 提供统一的设备类型状态给整个应用使用
 * - 避免每个组件重复检测
 * - 使用节流优化 resize 事件
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
 * - ✅ 100ms 节流避免频繁触发
 */
export function useBreakpoint() {
  const [isMobile, setIsMobile] = useState(false);
  const tickingRef = useRef(false);

  const checkMobile = useCallback(() => {
    setIsMobile(window.innerWidth < 640);
  }, []);

  // 节流的 resize 处理器
  const handleResize = useCallback(() => {
    if (!tickingRef.current) {
      tickingRef.current = true;
      requestAnimationFrame(() => {
        checkMobile();
        tickingRef.current = false;
      });
    }
  }, [checkMobile]);

  useEffect(() => {
    // 初始检查
    checkMobile();

    // 监听窗口大小变化（带节流）
    window.addEventListener("resize", handleResize, { passive: true });

    // 清理函数
    return () => {
      window.removeEventListener("resize", handleResize);
    };
  }, [checkMobile, handleResize]);

  return { isMobile };
}
