import LoadingLayout from "@/components/layout/LoadingLayout";
import { TagSkeleton } from "@/components/ui/Skeleton";

export default function TagsLoading() {
  return (
    <LoadingLayout
      title="标签管理"
      description="加载中..."
      rightContent={
        <div className="h-10 w-24 bg-slate-200 rounded-lg animate-pulse" />
      }
    >
      <div className="py-6">
        <TagSkeleton count={16} />
      </div>
    </LoadingLayout>
  );
}
