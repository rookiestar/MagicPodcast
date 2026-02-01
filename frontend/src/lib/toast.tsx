"use client";

import React, { createContext, useContext, useState, useCallback } from "react";

export type ToastType = "success" | "error" | "info" | "warning";

export interface Toast {
  id: number;
  message: string;
  type: ToastType;
}

interface ToastContextType {
  toasts: Toast[];
  showToast: (message: string, type: ToastType) => void;
  removeToast: (id: number) => void;
}

const ToastContext = createContext<ToastContextType | undefined>(undefined);

export function ToastProvider({ children }: { children: React.ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([]);

  const showToast = useCallback((message: string, type: ToastType) => {
    const id = Date.now();
    setToasts((prev) => [...prev, { id, message, type }]);

    // 自动移除
    setTimeout(() => {
      setToasts((prev) => prev.filter((t) => t.id !== id));
    }, 3000);
  }, []);

  const removeToast = useCallback((id: number) => {
    setToasts((prev) => prev.filter((t) => t.id !== id));
  }, []);

  const contextValue = { toasts, showToast, removeToast };

  // 设置全局上下文供 errorHandler 使用
  React.useEffect(() => {
    setGlobalToastContext(contextValue);
  }, [contextValue]);

  return (
    <ToastContext.Provider value={contextValue}>
      {children}
      <ToastContainer />
    </ToastContext.Provider>
  );
}

function ToastContainer() {
  const context = useContext(ToastContext);
  if (!context) return null;

  const { toasts, removeToast } = context;

  const getToastStyles = (type: ToastType) => {
    const baseStyles =
      "px-4 py-3 rounded-lg shadow-lg flex items-center gap-2 min-w-[300px] max-w-md animate-in slide-in-from-top";

    const typeStyles = {
      success:
        "bg-green-50 text-green-800 border border-green-200 dark:bg-green-900/20 dark:text-green-200",
      error:
        "bg-red-50 text-red-800 border border-red-200 dark:bg-red-900/20 dark:text-red-200",
      info: "bg-blue-50 text-blue-800 border border-blue-200 dark:bg-blue-900/20 dark:text-blue-200",
      warning:
        "bg-yellow-50 text-yellow-800 border border-yellow-200 dark:bg-yellow-900/20 dark:text-yellow-200",
    };

    return `${baseStyles} ${typeStyles[type]}`;
  };

  const getIcon = (type: ToastType) => {
    const icons = {
      success: "✓",
      error: "✕",
      info: "ℹ",
      warning: "⚠",
    };
    return icons[type];
  };

  return (
    <div className="fixed top-4 right-4 z-50 flex flex-col gap-2 pointer-events-none">
      {toasts.map((toast) => (
        <div
          key={toast.id}
          className={getToastStyles(toast.type) + " pointer-events-auto"}
        >
          <span className="text-lg font-bold">{getIcon(toast.type)}</span>
          <span className="flex-1">{toast.message}</span>
          <button
            onClick={() => removeToast(toast.id)}
            className="ml-2 text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200"
            aria-label="Close"
          >
            ✕
          </button>
        </div>
      ))}
    </div>
  );
}

// Hook for using toast
export function useToast() {
  const context = useContext(ToastContext);
  if (!context) {
    throw new Error("useToast must be used within ToastProvider");
  }

  return {
    toast: {
      success: (message: string) => context.showToast(message, "success"),
      error: (message: string) => context.showToast(message, "error"),
      info: (message: string) => context.showToast(message, "info"),
      warning: (message: string) => context.showToast(message, "warning"),
    },
  };
}

// 便捷的导出（用于 errorHandler 等非组件文件）
let globalToastContext: ToastContextType | null = null;

export const toast = {
  success: (message: string) => {
    if (globalToastContext) {
      globalToastContext.showToast(message, "success");
    } else {
      console.warn("[Toast] ToastProvider not found, message:", message);
    }
  },
  error: (message: string) => {
    if (globalToastContext) {
      globalToastContext.showToast(message, "error");
    } else {
      console.warn("[Toast] ToastProvider not found, message:", message);
    }
  },
  info: (message: string) => {
    if (globalToastContext) {
      globalToastContext.showToast(message, "info");
    } else {
      console.warn("[Toast] ToastProvider not found, message:", message);
    }
  },
  warning: (message: string) => {
    if (globalToastContext) {
      globalToastContext.showToast(message, "warning");
    } else {
      console.warn("[Toast] ToastProvider not found, message:", message);
    }
  },
};

// 设置全局上下文（由 ToastProvider 内部调用）
export function setGlobalToastContext(context: ToastContextType) {
  globalToastContext = context;
}
