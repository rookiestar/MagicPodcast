import type { ReportStats } from "@/lib/reportStats";

export default function ReportStatsLine({
  stats,
}: {
  stats?: ReportStats | null;
}) {
  if (!stats?.line) {
    return null;
  }
  return (
    <p className="report-stats-line" data-testid="report-stats-line">
      {stats.line}
    </p>
  );
}
