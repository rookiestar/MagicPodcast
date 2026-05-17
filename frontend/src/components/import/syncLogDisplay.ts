import type { LogEntry } from "@/lib/syncLogState";

export function getLogLabel(type: LogEntry["type"]) {
  switch (type) {
    case "success":
      return "成功";
    case "error":
      return "失败";
    case "progress":
      return "进度";
    case "summary":
      return "汇总";
    case "complete":
      return "完成";
    case "skip_paid":
    case "skip_cert":
    case "skip_not_found":
    case "skip_access_denied":
    case "skip_geo_blocked":
    case "skip_duplicate":
    case "skip_invalid":
    case "skip_other":
      return "跳过";
    case "skip_no_update":
      return "无更新";
    default:
      return "信息";
  }
}

export function getLogBadgeClass(type: LogEntry["type"]) {
  switch (type) {
    case "success":
      return "bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-300";
    case "error":
      return "bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300";
    case "progress":
      return "bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300";
    case "summary":
      return "bg-violet-100 text-violet-700 dark:bg-violet-900/30 dark:text-violet-300";
    case "complete":
      return "bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300";
    default:
      return "bg-slate-100 text-slate-600 dark:bg-slate-800 dark:text-slate-300";
  }
}

export function getLogRowClass(type: LogEntry["type"]) {
  switch (type) {
    case "success":
      return "border-green-100 bg-green-50/70 dark:border-green-900/40 dark:bg-green-950/20";
    case "error":
      return "border-red-100 bg-red-50/80 dark:border-red-900/40 dark:bg-red-950/25";
    case "progress":
      return "border-blue-100 bg-blue-50/70 dark:border-blue-900/40 dark:bg-blue-950/20";
    case "summary":
      return "border-violet-100 bg-violet-50/70 dark:border-violet-900/40 dark:bg-violet-950/20";
    default:
      return "border-slate-100 bg-white dark:border-slate-800 dark:bg-slate-900";
  }
}

export function getSkipLogTypeLabel(type: LogEntry["type"]) {
  switch (type) {
    case "skip_paid":
      return "付费播客";
    case "skip_cert":
      return "证书过期";
    case "skip_not_found":
      return "不存在";
    case "skip_no_update":
      return "无更新";
    case "skip_access_denied":
      return "访问拒绝";
    case "skip_geo_blocked":
      return "地区限制";
    case "skip_duplicate":
      return "重复";
    case "skip_invalid":
      return "格式无效";
    case "skip_other":
      return "其他";
    default:
      return "";
  }
}
