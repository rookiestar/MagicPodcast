'use client'

import { useEffect, useState } from 'react'

export type ToastType = 'success' | 'error' | 'info' | 'warning'

export interface Toast {
  id: number
  message: string
  type: ToastType
  duration?: number
}

let toastId = 0
const listeners: ((toast: Toast) => void)[] = []

// 显示toast
export function showToast(message: string, type: ToastType = 'info', duration: number = 3000) {
  const id = ++toastId
  const toast: Toast = { id, message, type, duration }

  // 通知所有监听器
  listeners.forEach(listener => listener(toast))

  // 自动移除
  if (duration > 0) {
    setTimeout(() => {
      removeToast(id)
    }, duration)
  }

  return id
}

// 移除toast
function removeToast(id: number) {
  // 这里我们用一个特殊的事件来通知移除
  listeners.forEach(listener => {
    listener({ id, message: '', type: 'info', duration: 0 } as Toast)
  })
}

// 便捷方法
export const toast = {
  success: (message: string, duration?: number) => showToast(message, 'success', duration),
  error: (message: string, duration?: number) => showToast(message, 'error', duration),
  info: (message: string, duration?: number) => showToast(message, 'info', duration),
  warning: (message: string, duration?: number) => showToast(message, 'warning', duration),
}

// Toast容器组件
export function ToastContainer() {
  const [toasts, setToasts] = useState<Toast[]>([])

  useEffect(() => {
    // 添加监听器
    const listener = (toast: Toast) => {
      if (toast.message === '') {
        // 移除toast
        setToasts(prev => prev.filter(t => t.id !== toast.id))
      } else {
        // 添加新toast
        setToasts(prev => [...prev, toast])
      }
    }

    listeners.push(listener)

    return () => {
      const index = listeners.indexOf(listener)
      if (index > -1) {
        listeners.splice(index, 1)
      }
    }
  }, [])

  const getToastStyles = (type: ToastType) => {
    const baseStyles = 'px-4 py-3 rounded-lg shadow-lg flex items-center gap-2 min-w-[300px] max-w-md animate-in slide-in-from-top'

    const typeStyles = {
      success: 'bg-green-50 text-green-800 border border-green-200',
      error: 'bg-red-50 text-red-800 border border-red-200',
      info: 'bg-blue-50 text-blue-800 border border-blue-200',
      warning: 'bg-yellow-50 text-yellow-800 border border-yellow-200',
    }

    return `${baseStyles} ${typeStyles[type]}`
  }

  const getIcon = (type: ToastType) => {
    const icons = {
      success: '✓',
      error: '✕',
      info: 'ℹ',
      warning: '⚠',
    }
    return icons[type]
  }

  return (
    <div className="fixed top-4 right-4 z-50 flex flex-col gap-2">
      {toasts.map(toast => (
        <div key={toast.id} className={getToastStyles(toast.type)}>
          <span className="text-lg font-bold">{getIcon(toast.type)}</span>
          <span className="flex-1">{toast.message}</span>
          <button
            onClick={() => removeToast(toast.id)}
            className="ml-2 text-gray-500 hover:text-gray-700"
          >
            ✕
          </button>
        </div>
      ))}
    </div>
  )
}
