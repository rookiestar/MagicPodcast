import LoadingLayout from "@/components/layout/LoadingLayout";
import { PodcastDetailSkeleton, EpisodeCardSkeleton } from "@/components/ui/Skeleton";

export default function PodcastDetailLoading() {
  return (
    <LoadingLayout
      showBack
      title="加载中..."
      rightContent={
        <div className="h-10 w-28 bg-slate-200 rounded-lg animate-pulse" />
      }
    >
      <div className="py-6 space-y-6">
        {/* Detail skeleton */}
        <PodcastDetailSkeleton />

        {/* Episodes section */}
        <div className="space-y-4">
          {/* Section header skeleton */}
          <div className="flex items-center justify-between animate-pulse">
            <div className="h-6 w-20 bg-slate-200 rounded" />
            <div className="h-4 w-16 bg-slate-200 rounded" />
          </div>
          {/* Episode cards - each has its own animate-pulse */}
          <div className="grid gap-4">
            {Array.from({ length: 5 }).map((_, i) => (
              <EpisodeCardSkeleton key={i} />
            ))}
          </div>
        </div>
      </div>
    </LoadingLayout>
  );
}
