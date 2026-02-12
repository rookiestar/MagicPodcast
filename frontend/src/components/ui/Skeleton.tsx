"use client";

// ============ 基础骨架屏 ============

interface SkeletonProps {
  className?: string;
  variant?: "text" | "circular" | "rectangular" | "rounded";
  width?: string | number;
  height?: string | number;
}

export function Skeleton({
  className = "",
  variant = "text",
  width,
  height,
}: SkeletonProps) {
  const variantClasses: Record<string, string> = {
    text: "rounded h-4",
    circular: "rounded-full",
    rectangular: "",
    rounded: "rounded-lg",
  };

  const style: React.CSSProperties = {};
  if (width) style.width = typeof width === "number" ? `${width}px` : width;
  if (height) style.height = typeof height === "number" ? `${height}px` : height;

  return (
    <div
      className={`bg-slate-200 dark:bg-slate-700 animate-pulse ${variantClasses[variant]} ${className}`}
      style={style}
    />
  );
}

// ============ 播客卡片骨架屏 ============

interface PodcastCardSkeletonProps {
  isMobile?: boolean;
}

export function PodcastCardSkeleton({ isMobile = false }: PodcastCardSkeletonProps) {
  if (isMobile) {
    return (
      <div className="flex flex-row gap-3 p-3 bg-white dark:bg-slate-800 rounded-xl shadow-md">
        <Skeleton variant="rounded" width={64} height={64} />
        <div className="flex-1 space-y-2">
          <Skeleton variant="text" className="w-3/4 h-4" />
          <Skeleton variant="text" className="w-1/2 h-3" />
          <Skeleton variant="text" className="w-full h-3" />
        </div>
      </div>
    );
  }

  return (
    <div className="flex flex-col h-full bg-white dark:bg-slate-800 rounded-xl shadow-md overflow-hidden">
      {/* 封面 */}
      <Skeleton variant="rectangular" className="w-full pt-[100%]" />

      {/* 内容区 */}
      <div className="flex-1 flex flex-col gap-2 p-4">
        <Skeleton variant="text" className="w-3/4 h-5" />
        <Skeleton variant="text" className="w-1/2 h-3" />
        <Skeleton variant="text" className="w-full h-3" />
        <Skeleton variant="text" className="w-2/3 h-3" />

        {/* 标签占位 */}
        <div className="flex gap-1.5 mt-auto pt-2">
          <Skeleton variant="rounded" className="w-14 h-5" />
          <Skeleton variant="rounded" className="w-16 h-5" />
        </div>
      </div>
    </div>
  );
}

// ============ 播客详情页骨架屏 ============

interface PodcastDetailSkeletonProps {
  isMobile?: boolean;
}

export function PodcastDetailSkeleton({ isMobile = false }: PodcastDetailSkeletonProps) {
  if (isMobile) {
    return (
      <div className="bg-white dark:bg-slate-800 rounded-lg shadow-lg p-4 space-y-4">
        {/* 头部：封面+标题 */}
        <div className="flex gap-4">
          <Skeleton variant="rounded" width={96} height={96} />
          <div className="flex-1 space-y-2">
            <Skeleton variant="text" className="w-3/4 h-6" />
            <Skeleton variant="text" className="w-1/2 h-4" />
            <Skeleton variant="text" className="w-1/3 h-3" />
          </div>
        </div>

        {/* 描述 */}
        <div className="space-y-2">
          <Skeleton variant="text" className="w-full h-3" />
          <Skeleton variant="text" className="w-full h-3" />
          <Skeleton variant="text" className="w-2/3 h-3" />
        </div>

        {/* 标签 */}
        <div className="flex gap-2">
          <Skeleton variant="rounded" className="w-16 h-6" />
          <Skeleton variant="rounded" className="w-20 h-6" />
          <Skeleton variant="rounded" className="w-14 h-6" />
        </div>
      </div>
    );
  }

  return (
    <div className="bg-white dark:bg-slate-800 rounded-lg shadow-lg overflow-hidden">
      <div className="md:flex">
        {/* 左侧：封面 */}
        <div className="md:w-1/3 p-6 flex justify-center">
          <Skeleton variant="rounded" className="aspect-square w-full max-w-[240px]" />
        </div>

        {/* 右侧：详情 */}
        <div className="md:w-2/3 p-6 space-y-4">
          <Skeleton variant="text" className="w-1/2 h-8" />
          <Skeleton variant="text" className="w-1/3 h-4" />

          <div className="space-y-2 pt-2">
            <Skeleton variant="text" className="w-full h-3" />
            <Skeleton variant="text" className="w-full h-3" />
            <Skeleton variant="text" className="w-3/4 h-3" />
          </div>

          {/* 元数据 */}
          <div className="grid grid-cols-2 gap-4 pt-4">
            <Skeleton variant="text" className="w-24 h-4" />
            <Skeleton variant="text" className="w-24 h-4" />
            <Skeleton variant="text" className="w-24 h-4" />
            <Skeleton variant="text" className="w-24 h-4" />
          </div>

          {/* 标签 */}
          <div className="flex gap-2 pt-4">
            <Skeleton variant="rounded" className="w-20 h-7" />
            <Skeleton variant="rounded" className="w-24 h-7" />
            <Skeleton variant="rounded" className="w-16 h-7" />
          </div>
        </div>
      </div>
    </div>
  );
}

// ============ 单集卡片骨架屏 ============

export function EpisodeCardSkeleton() {
  return (
    <div className="bg-white dark:bg-slate-800 rounded-lg shadow-sm p-4 animate-pulse">
      <div className="flex items-start gap-3">
        <Skeleton variant="rounded" width={56} height={56} />
        <div className="flex-1 space-y-2">
          <Skeleton variant="text" className="w-3/4 h-4" />
          <Skeleton variant="text" className="w-1/2 h-3" />
          <Skeleton variant="text" className="w-full h-3" />
          <Skeleton variant="text" className="w-2/3 h-3" />
        </div>
      </div>
    </div>
  );
}

// ============ 标签骨架屏 ============

interface TagSkeletonProps {
  count?: number;
}

export function TagSkeleton({ count = 8 }: TagSkeletonProps) {
  return (
    <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6 xl:grid-cols-8 gap-3">
      {Array.from({ length: count }).map((_, i) => (
        <Skeleton
          key={i}
          variant="rounded"
          className="h-16 w-full"
        />
      ))}
    </div>
  );
}

// ============ 工作流卡片骨架屏 ============

export function WorkflowCardSkeleton() {
  return (
    <div className="bg-white dark:bg-slate-800 rounded-lg shadow-sm p-5 space-y-3 animate-pulse">
      <div className="flex items-center gap-2">
        <Skeleton variant="text" className="w-1/3 h-6" />
        <Skeleton variant="rounded" className="w-16 h-6" />
      </div>
      <Skeleton variant="text" className="w-full h-4" />
      <Skeleton variant="text" className="w-2/3 h-4" />
      <div className="flex gap-4 pt-2">
        <Skeleton variant="text" className="w-20 h-3" />
        <Skeleton variant="text" className="w-20 h-3" />
        <Skeleton variant="text" className="w-20 h-3" />
      </div>
    </div>
  );
}

// ============ 工作流详情页骨架屏 ============

export function WorkflowDetailSkeleton() {
  return (
    <div className="py-6 space-y-6">
      {/* Tabs骨架 */}
      <div className="bg-white dark:bg-slate-800 rounded-lg shadow-lg p-4">
        <div className="flex gap-6">
          <Skeleton variant="text" className="w-20 h-6" />
          <Skeleton variant="text" className="w-24 h-6" />
        </div>
      </div>

      {/* 内容区骨架 */}
      <div className="bg-white dark:bg-slate-800 rounded-lg shadow-lg p-6 space-y-6">
        {/* 配置详情标题 */}
        <Skeleton variant="text" className="w-32 h-6" />

        {/* 调度配置 */}
        <div className="space-y-3">
          <Skeleton variant="text" className="w-24 h-5" />
          <div className="bg-slate-50 dark:bg-slate-900 rounded-lg p-5">
            <div className="grid md:grid-cols-3 gap-6">
              <div className="space-y-2">
                <Skeleton variant="text" className="w-16 h-3" />
                <Skeleton variant="rounded" className="w-full h-10" />
              </div>
              <div className="space-y-2">
                <Skeleton variant="text" className="w-16 h-3" />
                <Skeleton variant="text" className="w-32 h-5" />
              </div>
              <div className="space-y-2">
                <Skeleton variant="text" className="w-16 h-3" />
                <Skeleton variant="text" className="w-32 h-5" />
              </div>
            </div>
          </div>
        </div>

        {/* 抓取与筛选配置 */}
        <div className="space-y-3">
          <Skeleton variant="text" className="w-28 h-5" />
          <div className="bg-slate-50 dark:bg-slate-900 rounded-lg p-5 space-y-4">
            <div className="flex items-center gap-3">
              <Skeleton variant="text" className="w-20 h-4" />
              <Skeleton variant="text" className="w-32 h-5" />
            </div>
            <div className="grid md:grid-cols-2 lg:grid-cols-4 gap-4">
              <Skeleton variant="rounded" className="h-24" />
              <Skeleton variant="rounded" className="h-24" />
              <Skeleton variant="rounded" className="h-24" />
              <Skeleton variant="rounded" className="h-24" />
            </div>
          </div>
        </div>

        {/* 统计数据 */}
        <div className="space-y-3">
          <Skeleton variant="text" className="w-20 h-5" />
          <div className="grid grid-cols-2 gap-4">
            <Skeleton variant="rounded" className="h-20" />
            <Skeleton variant="rounded" className="h-20" />
          </div>
        </div>
      </div>
    </div>
  );
}
