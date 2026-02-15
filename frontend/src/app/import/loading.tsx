import LoadingLayout from "@/components/layout/LoadingLayout";

export default function ImportLoading() {
  return (
    <LoadingLayout
      title="导入/同步"
      description="加载中..."
    >
      <div className="py-8">
        <div className="max-w-2xl mx-auto space-y-6">
          {/* Upload area skeleton */}
          <div className="bg-white rounded-xl shadow-sm p-8">
            <div className="border-2 border-dashed border-slate-300 rounded-lg p-12 text-center">
              <div className="h-12 w-12 bg-slate-200 rounded-full mx-auto mb-4 animate-pulse" />
              <div className="h-5 w-48 bg-slate-200 rounded mx-auto mb-2 animate-pulse" />
              <div className="h-4 w-32 bg-slate-200 rounded mx-auto animate-pulse" />
            </div>
          </div>

          {/* Sync options skeleton */}
          <div className="bg-white rounded-xl shadow-sm p-6 space-y-4">
            <div className="h-6 w-32 bg-slate-200 rounded animate-pulse" />
            <div className="space-y-3">
              {Array.from({ length: 3 }).map((_, i) => (
                <div key={i} className="flex items-center gap-3">
                  <div className="h-5 w-5 bg-slate-200 rounded animate-pulse" />
                  <div className="h-4 w-40 bg-slate-200 rounded animate-pulse" />
                </div>
              ))}
            </div>
            <div className="h-10 w-32 bg-slate-200 rounded-lg animate-pulse" />
          </div>
        </div>
      </div>
    </LoadingLayout>
  );
}
