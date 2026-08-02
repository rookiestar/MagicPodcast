"use client";

import { IconCheck, IconX } from "@tabler/icons-react";
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

  if (!isOpen) {
    return null;
  }

  return (
    <>
      <div
        className="podcast-sort-backdrop"
        onClick={onClose}
        aria-hidden="true"
      />

      <div
        className="podcast-sort-drawer"
        role="dialog"
        aria-modal="true"
        aria-labelledby="podcast-sort-drawer-title"
      >
        <div className="podcast-sort-drawer-inner">
          <div className="podcast-sort-drawer-heading">
            <h3
              id="podcast-sort-drawer-title"
            >
              选择排序方式
            </h3>
            <button
              onClick={onClose}
              className="podcast-sort-drawer-close"
              aria-label="关闭"
            >
              <IconX aria-hidden="true" stroke={1.8} />
            </button>
          </div>

          <div className="podcast-sort-options">
            {options.map((option) => {
              const isSelected = currentSort === option.value;
              return (
                <button
                  key={option.value}
                  onClick={() => handleSortChange(option.value)}
                  aria-pressed={isSelected}
                  className={isSelected ? "is-selected" : ""}
                >
                  <span>{option.label}</span>
                  {isSelected && (
                    <IconCheck aria-hidden="true" stroke={2} />
                  )}
                </button>
              );
            })}
          </div>

          <button
            onClick={onClose}
            className="podcast-sort-cancel"
          >
            取消
          </button>
        </div>
      </div>
    </>
  );
}
