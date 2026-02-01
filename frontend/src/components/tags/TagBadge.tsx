import { Tag } from "@/types";

interface TagBadgeProps {
  tag: Tag;
  onRemove?: (tagId: number) => void;
  size?: "sm" | "md" | "lg";
  removable?: boolean;
  variant?: "colorful" | "simple";
}

export default function TagBadge({
  tag,
  onRemove,
  size = "md",
  removable = false,
  variant = "colorful",
}: TagBadgeProps) {
  const sizeClasses = {
    sm: "text-xs px-2 py-0.5",
    md: "text-sm px-3 py-1",
    lg: "text-base px-4 py-1.5",
  };

  // 简洁模式：与节目列表页一致的灰色样式 + 彩色圆点
  if (variant === "simple") {
    return (
      <span
        className={`
          inline-flex items-center gap-1 rounded-full font-medium
          ${sizeClasses[size]}
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
          <button
            onClick={(e) => {
              e.stopPropagation();
              onRemove(tag.id);
            }}
            className="ml-1 hover:bg-slate-300 rounded-full p-0.5 transition-colors focus:outline-none"
            title={`移除 "${tag.name}" 标签`}
          >
            <svg
              className="w-3 h-3"
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
        )}
      </span>
    );
  }

  // 彩色模式：原有的彩色背景样式
  return (
    <span
      className={`
        inline-flex items-center gap-1.5 rounded-full font-medium
        ${sizeClasses[size]}
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
        <button
          onClick={() => onRemove(tag.id)}
          className="
            hover:bg-white/50 rounded-full p-0.5
            transition-colors duration-150
            focus:outline-none focus:ring-2 focus:ring-offset-1 focus:ring-current
          "
          style={{ color: tag.color }}
          title={`移除 "${tag.name}" 标签`}
        >
          <svg
            className="w-3 h-3"
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
      )}
    </span>
  );
}
