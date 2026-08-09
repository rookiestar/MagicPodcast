import type { SyncStats } from "@/lib/syncLogState";

function StatCard({
  value,
  label,
  className = "text-slate-900 dark:text-slate-50",
}: {
  value: number;
  label: string;
  className?: string;
}) {
  return (
    <div className="rounded-lg bg-slate-50 p-4 dark:bg-slate-900">
      <p className={`text-2xl font-mono font-semibold ${className}`}>{value}</p>
      <p className="text-sm text-slate-600 dark:text-slate-400">{label}</p>
    </div>
  );
}

function SkipBreakdown({ stats }: { stats: SyncStats }) {
  if (stats.skips <= 0) return null;

  return (
    <div className="mt-3 flex flex-wrap gap-2 text-xs text-slate-600 dark:text-slate-400">
      <span className="rounded-full bg-slate-100 px-2 py-1 dark:bg-slate-800">
        付费 {stats.skipPaid}
      </span>
      <span className="rounded-full bg-slate-100 px-2 py-1 dark:bg-slate-800">
        证书 {stats.skipCert}
      </span>
      <span className="rounded-full bg-slate-100 px-2 py-1 dark:bg-slate-800">
        不存在 {stats.skipNotFound}
      </span>
      <span className="rounded-full bg-slate-100 px-2 py-1 dark:bg-slate-800">
        访问拒绝 {stats.skipAccess}
      </span>
      <span className="rounded-full bg-slate-100 px-2 py-1 dark:bg-slate-800">
        地区限制 {stats.skipGeo}
      </span>
      <span className="rounded-full bg-slate-100 px-2 py-1 dark:bg-slate-800">
        其他 {stats.skipOther}
      </span>
    </div>
  );
}

export default function SyncLogStats({ stats }: { stats: SyncStats }) {
  return (
    <div className="mb-4">
      {stats.fromSummary && stats.duration && (
        <div className="mb-3 text-xs font-medium text-blue-600 dark:text-blue-400">
          总耗时: {stats.duration}
        </div>
      )}

      <div className="grid grid-cols-2 gap-4 md:grid-cols-5">
        <StatCard value={stats.total} label="总计" />
        <StatCard
          value={stats.success}
          label="成功"
          className="text-green-600 dark:text-green-400"
        />
        <StatCard
          value={stats.errors}
          label="失败"
          className="text-red-600 dark:text-red-400"
        />
        <StatCard
          value={stats.skips}
          label="跳过"
          className="text-slate-600 dark:text-slate-400"
        />
        <StatCard
          value={stats.skipNoUpdate}
          label="无更新"
          className="text-amber-600 dark:text-amber-400"
        />
      </div>

      <SkipBreakdown stats={stats} />
    </div>
  );
}
