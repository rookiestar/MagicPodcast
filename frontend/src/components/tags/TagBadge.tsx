import { memo } from "react";
import type { Tag } from "@/types";
import {
  areTagBadgePropsEqual,
  getTagBadgeSizeClass,
  getTagRemoveButtonClass,
  getTagRemoveButtonStyle,
  getTagRemoveTitle,
  shouldStopTagRemovePropagation,
  type TagBadgeSize,
  type TagBadgeVariant,
} from "@/lib/tagBadgeState";

interface TagBadgeProps {
  tag: Tag;
  onRemove?: (tagId: number) => void;
  size?: TagBadgeSize;
  removable?: boolean;
  variant?: TagBadgeVariant;
}

function TagBadge({
  tag,
  onRemove,
  size = "md",
  removable = false,
  variant = "colorful",
}: TagBadgeProps) {
  const sizeClass = getTagBadgeSizeClass(size);

  // 简洁模式：与节目列表页一致的灰色样式 + 彩色圆点
  if (variant === "simple") {
    return (
      <span
        className={`
          inline-flex items-center gap-1 rounded-full font-medium
          ${sizeClass}
          transition-colors
          bg-slate-100 hover:bg-slate-200 text-slate-600
          group relative
        `}
        title={tag.name}
      >
        <span
          className="w-1.5 h-1.5 rounded-full flex-shrink-0"
          style={{ backgroundColor: tag.color }}
        />
        <span className="max-w-[120px] truncate">{tag.name}</span>
        {removable && onRemove && (
          <TagRemoveButton
            tag={tag}
            variant="simple"
            onRemove={() => onRemove(tag.id)}
          />
        )}
      </span>
    );
  }

  // 彩色模式：原有的彩色背景样式
  return (
    <span
      className={`
        inline-flex items-center gap-1.5 rounded-full font-medium
        ${sizeClass}
        transition-all duration-200
        group relative
      `}
      style={{
        backgroundColor: `${tag.color}20`,
        color: tag.color,
        border: `1px solid ${tag.color}40`,
      }}
    >
      <span className="max-w-[120px] truncate" title={tag.name}>
        {tag.name}
      </span>
      {/* 自定义 Tooltip */}
      <div className="absolute bottom-full left-1/2 -translate-x-1/2 mb-2 px-2 py-1 bg-slate-900 dark:bg-slate-100 text-white dark:text-slate-900 text-xs rounded whitespace-nowrap opacity-0 group-hover:opacity-100 transition-opacity pointer-events-none z-10">
        {tag.name}
      </div>
      {removable && onRemove && (
        <TagRemoveButton
          tag={tag}
          variant="colorful"
          onRemove={() => onRemove(tag.id)}
        />
      )}
    </span>
  );
}

interface TagRemoveButtonProps {
  tag: Tag;
  variant: "colorful" | "simple";
  onRemove: () => void;
}

function TagRemoveButton({ tag, variant, onRemove }: TagRemoveButtonProps) {
  return (
    <button
      onClick={(event) => {
        if (shouldStopTagRemovePropagation(variant)) {
          event.stopPropagation();
        }

        onRemove();
      }}
      className={getTagRemoveButtonClass(variant)}
      style={getTagRemoveButtonStyle(tag.color, variant)}
      title={getTagRemoveTitle(tag.name)}
    >
      <svg
        className="w-4 h-4"
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
  );
}

// 使用 React.memo 包装组件
export default memo(TagBadge, areTagBadgePropsEqual);

// 添加 displayName 用于调试
TagBadge.displayName = "TagBadge";
