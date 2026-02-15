import LoadingLayout from "@/components/layout/LoadingLayout";
import { WorkflowDetailSkeleton } from "@/components/ui/Skeleton";

export default function WorkflowDetailLoading() {
  return (
    <LoadingLayout
      showBack
      title="加载中..."
      rightContent={
        <div className="flex gap-2">
          <div className="h-10 w-20 bg-slate-200 rounded-lg animate-pulse" />
          <div className="h-10 w-24 bg-slate-200 rounded-lg animate-pulse" />
        </div>
      }
    >
      <WorkflowDetailSkeleton />
    </LoadingLayout>
  );
}
