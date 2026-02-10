"use client";

import { useEffect } from "react";

interface SortOption {
  label: string;
  value: string;
}

interface SortDrawerProps {
  isOpen: boolean;
  onClose: () => void;
  currentSort: string;
  onSortChange: (value: string) => void;
  options: SortOption[];
}

/**
 * 移动端排序抽屉组件
 * 底部滑入式选择器，符合iOS交互习惯
 */
export default function SortDrawer({
  isOpen,
  onClose,
  currentSort,
  onSortChange,
  options,
}: SortDrawerProps) {
  // 防止背景滚动
  useEffect(() => {
    if (isOpen) {
      document.body.style.overflow = "hidden";
    } else {
      document.body.style.overflow = "";
    }

    return () => {
      document.body.style.overflow = "";
    };
  }, [isOpen]);

  const handleSortChange = (value: string) => {
    onSortChange(value);
    onClose();
  };

  return (
    <>
      {/* 遮罩 */}
      {isOpen && (
        <div
          className="fixed inset-0 bg-black/50 z-50 transition-opacity"
          onClick={onClose}
          aria-hidden="true"
        />
      )}

      {/* 底部抽屉 */}
      <div
        className={`fixed bottom-0 left-0 right-0 bg-white rounded-t-2xl z-50 transform transition-transform duration-300 ease-in-out ${
          isOpen ? "translate-y-0" : "translate-y-full"
        }`}
        style={{ maxHeight: "70vh" }}
      >
        <div className="p-4">
          {/* 标题栏 */}
          <div className="flex items-center justify-between mb-4">
            <h3 className="text-lg font-semibold text-slate-800">选择排序方式</h3>
            <button
              onClick={onClose}
              className="w-8 h-8 flex items-center justify-center rounded-full hover:bg-slate-100 transition-colors"
              aria-label="关闭"
            >
              <svg
                className="w-5 h-5 text-slate-600"
                fill="none"
                stroke="currentColor"
                viewBox="0 0 24 24"
              >
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  strokeWidth={2}
                  d="M6 18L18 6M6 6l12 12"
                />
              </svg>
            </button>
          </div>

          {/* 排序选项 */}
          <div className="space-y-2">
            {options.map((option) => {
              const isSelected = currentSort === option.value;
              return (
                <button
                  key={option.value}
                  onClick={() => handleSortChange(option.value)}
                  className={`w-full text-left px-4 py-3 rounded-lg flex items-center justify-between transition-colors ${
                    isSelected
                      ? "bg-violet-100 text-violet-700"
                      : "bg-slate-50 text-slate-700 hover:bg-slate-100"
                  }`}
                >
                  <span className="font-medium">{option.label}</span>
                  {isSelected && (
                    <svg
                      className="w-5 h-5"
                      fill="currentColor"
                      viewBox="0 0 24 24"
                    >
                      <path d="M9 16.17L4.83 12l-1.42 1.41L9 19 21 7l-1.41-1.41z" />
                    </svg>
                  )}
                </button>
              );
            })}
          </div>

          {/* 取消按钮 */}
          <button
            onClick={onClose}
            className="w-full mt-4 py-3 text-slate-600 font-medium hover:bg-slate-50 rounded-lg transition-colors"
          >
            取消
          </button>
        </div>
      </div>
    </>
  );
}
