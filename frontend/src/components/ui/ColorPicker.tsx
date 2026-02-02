"use client";

import { useState, useRef, useEffect } from "react";

interface ColorPickerProps {
  value: string;
  onChange: (color: string) => void;
  disabled?: boolean;
}

// 32色精选色板（每行 8 个，共 4 行）- 优化浅色在白色背景的可读性
const PRESET_COLORS = [
  // Row 1: 暖色系 - 红橙渐变
  "#F87171", "#EF4444", "#DC2626", "#B91C1C",
  "#FB923C", "#F97316", "#EA580C", "#C2410C",
  // Row 2: 黄绿渐变
  "#FACC15", "#EAB308", "#CA8A04", "#A16207",
  "#4ADE80", "#22C55E", "#16A34A", "#15803D",
  // Row 3: 青蓝渐变
  "#22D3EE", "#06B6D4", "#0891B2", "#0E7490",
  "#60A5FA", "#3B82F6", "#2563EB", "#1D4ED8",
  // Row 4: 紫粉灰渐变
  "#C084FC", "#A78BFA", "#7C3AED", "#6D28D9",
  "#F472B6", "#EC4899", "#DB2777", "#BE185D",
];

export default function ColorPicker({
  value,
  onChange,
  disabled = false,
}: ColorPickerProps) {
  const [focusedIndex, setFocusedIndex] = useState<number>(-1);
  const containerRef = useRef<HTMLDivElement>(null);

  // 展开所有颜色，用于键盘导航
  const focusedColor = focusedIndex >= 0 ? PRESET_COLORS[focusedIndex] : null;
  const colorsPerRow = 8;

  // 键盘导航
  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (disabled) return;

    switch (e.key) {
      case "ArrowRight":
        e.preventDefault();
        setFocusedIndex((prev) =>
          prev < PRESET_COLORS.length - 1 ? prev + 1 : prev
        );
        break;
      case "ArrowLeft":
        e.preventDefault();
        setFocusedIndex((prev) => (prev > 0 ? prev - 1 : 0));
        break;
      case "ArrowDown":
        e.preventDefault();
        setFocusedIndex((prev) =>
          prev < PRESET_COLORS.length - colorsPerRow ? prev + colorsPerRow : prev
        );
        break;
      case "ArrowUp":
        e.preventDefault();
        setFocusedIndex((prev) => (prev >= colorsPerRow ? prev - colorsPerRow : prev));
        break;
      case "Enter":
      case " ":
        e.preventDefault();
        if (focusedColor) {
          onChange(focusedColor);
        }
        break;
      case "Escape":
        e.preventDefault();
        setFocusedIndex(-1);
        break;
    }
  };

  // 点击颜色块
  const handleColorClick = (color: string) => {
    if (!disabled) {
      onChange(color);
    }
  };

  return (
    <div
      ref={containerRef}
      className={`space-y-3 ${disabled ? "opacity-50 pointer-events-none" : ""}`}
      onKeyDown={handleKeyDown}
      tabIndex={disabled ? -1 : 0}
      role="listbox"
      aria-label="选择颜色"
    >
      {/* 预设颜色 - 8列网格 */}
      <div className="grid grid-cols-8 gap-2">
        {PRESET_COLORS.map((color, index) => {
          const isSelected = value === color;
          const isFocused = focusedColor === color;

          return (
            <button
              key={color}
              type="button"
              onClick={() => handleColorClick(color)}
              onFocus={() => setFocusedIndex(index)}
              className={`
                w-8 h-8 rounded-lg transition-all duration-150
                ${isSelected ? "ring-2 ring-offset-2 ring-blue-500 scale-110" : ""}
                ${isFocused ? "ring-2 ring-offset-2 ring-gray-400 scale-105" : ""}
                hover:scale-105 focus:outline-none
              `}
              style={{ backgroundColor: color }}
              aria-label={`${color}`}
              aria-selected={isSelected}
              role="option"
              tabIndex={-1}
            />
          );
        })}
      </div>

      {/* 自定义颜色 */}
      <div className="flex items-center gap-2 pt-2 border-t border-slate-200 dark:border-slate-700">
        <label className="text-sm text-slate-600 dark:text-slate-400">
          自定义颜色：
        </label>
        <input
          type="color"
          value={value}
          onChange={(e) => onChange(e.target.value)}
          disabled={disabled}
          className="w-8 h-8 rounded cursor-pointer border-0 p-0"
          aria-label="自定义颜色"
        />
        <span className="text-xs text-slate-500 dark:text-slate-500 font-mono">
          {value}
        </span>
      </div>
    </div>
  );
}
