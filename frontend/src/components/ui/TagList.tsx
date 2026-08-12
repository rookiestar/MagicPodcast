"use client";

import type { Tag } from "@/types";

interface TagListProps {
  tags: Tag[];
  maxDisplay?: number;
  maxNameWidth?: string;
  className?: string;
}

/**
 * 可复用的标签列表组件
 * 显示标签列表，支持限制显示数量和剩余标签计数
 */
export default function TagList({
  tags,
  maxDisplay = 3,
  maxNameWidth = "60px",
  className = "",
}: TagListProps) {
  if (!tags || tags.length === 0) {
    return null;
  }

  const displayedTags = tags.slice(0, maxDisplay);
  const remainingTags = tags.length - maxDisplay;

  return (
    <div className={`flex flex-wrap gap-1.5 ${className}`}>
      {displayedTags.map((tag) => (
        <span
          key={tag.id}
          className="inline-flex items-center gap-1 px-2 py-0.5 text-xs rounded-full bg-transparent hover:bg-black/5"
          title={tag.name}
        >
          <span
            className="w-1.5 h-1.5 rounded-full flex-shrink-0"
            style={{ backgroundColor: tag.color }}
          />
          <span
            className="truncate"
            style={{ maxWidth: maxNameWidth }}
          >
            {tag.name}
          </span>
        </span>
      ))}
      {remainingTags > 0 && (
        <span className="inline-flex items-center px-2 py-0.5 text-xs rounded-full bg-transparent text-slate-500 hover:bg-black/5">
          +{remainingTags}
        </span>
      )}
    </div>
  );
}
