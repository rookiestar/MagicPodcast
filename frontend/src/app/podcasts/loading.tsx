import LoadingLayout from "@/components/layout/LoadingLayout";
import { PodcastCardSkeleton } from "@/components/ui/Skeleton";

export default function PodcastsLoading() {
  return (
    <LoadingLayout
      tone="editorial"
      title="我的订阅"
      description="加载中..."
      rightContent={
        <div className="animate-pulse">
          {/* 移动端排序按钮骨架 */}
          <div className="md:hidden w-10 h-10 bg-slate-200 rounded-lg" />
          {/* 桌面端排序选择器骨架 */}
          <div className="hidden md:flex items-center gap-2">
            <div className="h-4 w-10 bg-slate-200 rounded" />
            <div className="h-10 w-28 bg-slate-200 rounded-lg" />
          </div>
        </div>
      }
    >
      {/* Tag filter skeleton */}
      <div className="mt-4 sm:mt-6 md:mt-8 mb-3 sm:mb-4 md:mb-6 animate-pulse">
        <div className="flex flex-wrap gap-2 sm:gap-3 items-center">
          {Array.from({ length: 6 }).map((_, i) => (
            <div key={i} className="h-11 min-w-[60px] px-4 bg-slate-200 rounded-lg" />
          ))}
        </div>
      </div>

      {/* Grid skeleton - 与实际页面完全一致的网格布局 */}
      <div className="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5 gap-3 md:gap-4 lg:gap-6">
        {Array.from({ length: 10 }).map((_, i) => (
          <PodcastCardSkeleton key={i} />
        ))}
      </div>
    </LoadingLayout>
  );
}
