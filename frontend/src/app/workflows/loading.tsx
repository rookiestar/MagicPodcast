import LoadingLayout from "@/components/layout/LoadingLayout";
import { WorkflowCardSkeleton } from "@/components/ui/Skeleton";

export default function WorkflowsLoading() {
  return (
    <LoadingLayout
      title="自动化工作流"
      description="加载中..."
      rightContent={
        <div className="h-10 w-28 bg-slate-200 rounded-lg animate-pulse" />
      }
    >
      <div className="py-6">
        <div className="grid gap-4">
          {Array.from({ length: 5 }).map((_, i) => (
            <WorkflowCardSkeleton key={i} />
          ))}
        </div>
      </div>
    </LoadingLayout>
  );
}
