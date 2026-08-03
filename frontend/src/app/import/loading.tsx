import LoadingLayout from "@/components/layout/LoadingLayout";

export default function ImportLoading() {
  return (
    <LoadingLayout
      tone="editorial"
      title="导入/同步"
      description="加载中..."
    >
      <div className="py-8">
        <div className="max-w-2xl mx-auto space-y-6">
          {/* Upload area skeleton */}
          <div className="editorial-loading-block p-8">
            <div
              className="border-2 border-dashed p-12 text-center"
              style={{ borderColor: "#c7bbab" }}
            >
              <div className="editorial-loading-block h-12 w-12 rounded-full mx-auto mb-4 animate-pulse" />
              <div className="editorial-loading-block h-5 w-48 mx-auto mb-2 animate-pulse" />
              <div className="editorial-loading-block h-4 w-32 mx-auto animate-pulse" />
            </div>
          </div>

          {/* Sync options skeleton */}
          <div className="editorial-loading-block p-6 space-y-4">
            <div className="editorial-loading-block h-6 w-32 animate-pulse" />
            <div className="space-y-3">
              {Array.from({ length: 3 }).map((_, i) => (
                <div key={i} className="flex items-center gap-3">
                  <div className="editorial-loading-block h-5 w-5 animate-pulse" />
                  <div className="editorial-loading-block h-4 w-40 animate-pulse" />
                </div>
              ))}
            </div>
            <div className="editorial-loading-block h-10 w-32 animate-pulse" />
          </div>
        </div>
      </div>
    </LoadingLayout>
  );
}
