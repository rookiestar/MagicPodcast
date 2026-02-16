"use client";

import React from "react";

// 工作流状态类型
export type WorkflowStatus = "enabled" | "disabled";

// 任务状态类型
export type JobStatus = "pending" | "running" | "completed" | "failed" | "cancelled";

// 所有支持的状态类型
export type StatusType = WorkflowStatus | JobStatus;

interface StatusBadgeProps {
  status: StatusType;
  size?: "sm" | "md";
  compact?: boolean;
  className?: string;
}

// 状态配置
const statusConfig: Record<StatusType, { text: string; className: string; dotClass?: string }> = {
  enabled: {
    text: "启用中",
    className: "bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200",
    dotClass: "bg-green-500",
  },
  disabled: {
    text: "已禁用",
    className: "bg-gray-100 text-gray-800 dark:bg-gray-800 dark:text-gray-300",
    dotClass: "bg-gray-400",
  },
  pending: {
    text: "等待中",
    className: "bg-gray-100 text-gray-800 dark:bg-gray-800 dark:text-gray-300",
  },
  running: {
    text: "执行中",
    className: "bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200",
  },
  completed: {
    text: "已完成",
    className: "bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200",
  },
  failed: {
    text: "失败",
    className: "bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-200",
  },
  cancelled: {
    text: "已取消",
    className: "bg-yellow-100 text-yellow-800 dark:bg-yellow-900 dark:text-yellow-200",
  },
};

/**
 * 状态徽章组件
 * 用于显示工作流状态或任务状态
 */
export function StatusBadge({ status, size = "md", compact = false, className = "" }: StatusBadgeProps) {
  const config = statusConfig[status];

  // 紧凑模式：仅显示彩色圆点
  if (compact && config.dotClass) {
    return (
      <span
        className={`w-3 h-3 rounded-full flex-shrink-0 ${config.dotClass} ${className}`}
        title={config.text}
      />
    );
  }

  // 尺寸样式
  const sizeClasses = size === "sm"
    ? "px-2.5 py-0.5 text-xs"
    : "px-3 py-1 text-sm";

  return (
    <span
      className={`inline-flex items-center rounded-full font-medium ${sizeClasses} ${config.className} ${className} flex-shrink-0`}
    >
      {config.text}
    </span>
  );
}

/**
 * 工作流状态徽章
 * 便捷组件，用于显示工作流的启用/禁用状态
 */
export function WorkflowStatusBadge({
  isEnabled,
  size = "md",
  compact = false,
}: {
  isEnabled: boolean;
  size?: "sm" | "md";
  compact?: boolean;
}) {
  return (
    <StatusBadge
      status={isEnabled ? "enabled" : "disabled"}
      size={size}
      compact={compact}
    />
  );
}

/**
 * 任务状态徽章
 * 便捷组件，用于显示任务的执行状态
 */
export function JobStatusBadge({
  status,
  size = "sm",
}: {
  status: string;
  size?: "sm" | "md";
}) {
  // 验证状态值，无效值默认为 pending
  const validStatus: JobStatus = ["pending", "running", "completed", "failed", "cancelled"].includes(status)
    ? (status as JobStatus)
    : "pending";

  return <StatusBadge status={validStatus} size={size} />;
}

export default StatusBadge;
