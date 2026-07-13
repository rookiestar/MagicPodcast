import { PodcastCardSkeleton } from "@/components/ui/Skeleton";

interface MobilePodcastListSummaryProps {
  totalCount: number;
  selectedTagCount: number;
}

export function MobilePodcastListSummary({
  totalCount,
  selectedTagCount,
}: MobilePodcastListSummaryProps) {
  if (totalCount <= 0) {
    return null;
  }

  return (
    <div className="md:hidden px-4 py-2 bg-slate-50 border-b border-slate-200">
      <p className="text-sm text-slate-600">
        共 {totalCount} 个节目
        {selectedTagCount > 0 && `（已选 ${selectedTagCount} 个标签）`}
      </p>
    </div>
  );
}

interface PodcastListErrorStateProps {
  message: string;
  onRetry: () => void;
}

export function PodcastListErrorState({
  message,
  onRetry,
}: PodcastListErrorStateProps) {
  return (
    <div className="bg-red-50 border border-red-200 rounded-lg p-6 mb-6">
      <h3 className="text-red-800 font-semibold mb-2">加载失败</h3>
      <p className="text-red-600 mb-4">{message}</p>
      <button
        onClick={onRetry}
        className="px-4 py-2 bg-red-600 text-white rounded-lg hover:bg-red-700 transition-colors focus:outline-none focus:ring-2 focus:ring-red-500 focus:ring-offset-2"
      >
        重试
      </button>
    </div>
  );
}

interface PodcastListLoadingGridProps {
  isMobile: boolean;
}

export function PodcastListLoadingGrid({
  isMobile,
}: PodcastListLoadingGridProps) {
  return (
    <div className="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5 gap-3 md:gap-4 lg:gap-6">
      {Array.from({ length: 10 }).map((_, index) => (
        <PodcastCardSkeleton key={index} isMobile={isMobile} />
      ))}
    </div>
  );
}

interface PodcastListEmptyFilterStateProps {
  onClearFilters: () => void;
}

export function PodcastListEmptyFilterState({
  onClearFilters,
}: PodcastListEmptyFilterStateProps) {
  return (
    <div className="flex flex-col items-center justify-center py-12 px-4">
      <svg
        className="w-12 h-12 text-slate-300 mb-4"
        fill="none"
        stroke="currentColor"
        viewBox="0 0 24 24"
      >
        <path
          strokeLinecap="round"
          strokeLinejoin="round"
          strokeWidth={1.5}
          d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z"
        />
      </svg>
      <p className="text-slate-500 text-center">
        没有找到同时包含这些标签的节目
      </p>
      <button
        onClick={onClearFilters}
        className="mt-4 text-sm text-slate-600 hover:text-slate-900 underline underline-offset-2 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 rounded"
      >
        清空筛选
      </button>
    </div>
  );
}

export function PodcastListEmptyLibraryState() {
  return (
    <div className="flex flex-col items-center justify-center py-12 px-4">
      <svg
        className="w-12 h-12 text-slate-300 mb-4"
        fill="none"
        stroke="currentColor"
        viewBox="0 0 24 24"
      >
        <path
          strokeLinecap="round"
          strokeLinejoin="round"
          strokeWidth={1.5}
          d="M19 11a7 7 0 01-7 7m0 0a7 7 0 01-7-7m7 7v4m0 0H8m4 0h4m-4-8a3 3 0 01-3-3V5a3 3 0 116 0v6a3 3 0 01-3 3z"
        />
      </svg>
      <p className="text-slate-500 text-center mb-4">还没有订阅任何节目</p>
      <div className="flex gap-3">
        <a
          href="/import"
          className="text-sm text-slate-600 hover:text-slate-900 underline underline-offset-2 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 rounded"
        >
          导入订阅
        </a>
      </div>
    </div>
  );
}

interface PodcastListFooterProps {
  hasPodcasts: boolean;
  hasMore: boolean;
  isLoadingMore: boolean;
}

export function PodcastListFooter({
  hasPodcasts,
  hasMore,
  isLoadingMore,
}: PodcastListFooterProps) {
  if (!hasPodcasts) {
    return null;
  }

  if (isLoadingMore) {
    return (
      <div className="text-center py-8">
        <div className="inline-block animate-spin rounded-full h-8 w-8 border-b-2 border-blue-600" />
        <p className="mt-2 text-sm text-slate-600">加载更多...</p>
      </div>
    );
  }

  if (!hasMore) {
    return <div className="text-center py-8 text-slate-500">已经到底了</div>;
  }

  return null;
}

interface PodcastListPaginationErrorFooterProps {
  message: string;
  onRetry: () => void;
}

// 分页失败页脚：已加载节目仍保留在网格中，仅在该页失败时给出可重试的内联提示，
// 避免分页失败把整列表替换为错误态而丢失已加载内容。
export function PodcastListPaginationErrorFooter({
  message,
  onRetry,
}: PodcastListPaginationErrorFooterProps) {
  return (
    <div
      data-testid="pagination-error"
      className="text-center py-6 px-4"
      role="alert"
      aria-live="polite"
    >
      <p className="text-sm text-red-600 mb-3">
        {message || "分页加载失败，请重试"}
      </p>
      <button
        onClick={onRetry}
        className="px-4 py-2 bg-red-600 text-white rounded-lg hover:bg-red-700 transition-colors focus:outline-none focus:ring-2 focus:ring-red-500 focus:ring-offset-2"
      >
        重试
      </button>
    </div>
  );
}
