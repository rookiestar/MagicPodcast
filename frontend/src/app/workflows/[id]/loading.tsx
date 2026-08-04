import LoadingLayout from "@/components/layout/LoadingLayout";
import { WorkflowDetailSkeleton } from "@/components/ui/Skeleton";

export default function WorkflowDetailLoading() {
  return (
    <LoadingLayout
      tone="editorial"
      showBack
      title="加载中..."
      rightContent={
        <div className="flex gap-2 animate-pulse">
          <div className="editorial-loading-block h-10 w-20" />
          <div className="editorial-loading-block h-10 w-24" />
        </div>
      }
    >
      <WorkflowDetailSkeleton />
    </LoadingLayout>
  );
}
