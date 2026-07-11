import LoadingLayout from "@/components/layout/LoadingLayout";
import { PodcastDetailSkeleton } from "@/components/ui/Skeleton";

export default function PodcastDetailLoading() {
  return (
    <LoadingLayout
      showBack
      title="加载中..."
      rightContent={
        <div className="h-10 w-28 bg-slate-200 rounded-lg animate-pulse" />
      }
    >
      <div className="py-6">
        {/* Detail skeleton */}
        <PodcastDetailSkeleton />

        {/* Episodes section - 与 page.tsx 骨架屏保持一致 */}
        <div className="mt-8">
          {/* 标题骨架屏 */}
          <div className="mb-6 h-8 w-40 bg-slate-200 rounded animate-pulse"></div>
          {/* Episode cards */}
          <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
            {Array.from({ length: 4 }).map((_, i) => (
              <div
                key={i}
                className="bg-white rounded-lg shadow-sm p-6 animate-pulse"
              >
                <div className="flex items-start gap-3">
                  <div className="w-16 h-16 bg-slate-200 rounded-lg"></div>
                  <div className="flex-1 space-y-2">
                    <div className="h-4 bg-slate-200 rounded w-3/4"></div>
                    <div className="h-3 bg-slate-200 rounded w-1/2"></div>
                  </div>
                </div>
              </div>
            ))}
          </div>
        </div>
      </div>
    </LoadingLayout>
  );
}
