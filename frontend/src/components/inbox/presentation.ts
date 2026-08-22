import type { ConsumptionQueue } from "@/types/consumption";

export interface QueuePresentation {
  label: string;
  policy: string;
  empty: string;
}

export const QUEUE_PRESENTATION: Record<ConsumptionQueue, QueuePresentation> = {
  inbox: {
    label: "Inbox",
    policy: "先收进来，再决定投入或稍后。",
    empty: "没有待决定的单集。",
  },
  focus: {
    label: "Focus",
    policy: "短承诺队列，建议保持在 7 项以内。",
    empty: "近期还没有明确投入的单集。",
  },
  someday: {
    label: "Someday",
    policy: "保留价值，但暂不占用近期注意力。",
    empty: "没有留待以后处理的单集。",
  },
  done: {
    label: "Done",
    policy: "只记录你明确确认完成的内容。",
    empty: "还没有手动完成的单集。",
  },
};

export function formatDuration(duration: number) {
  if (!Number.isFinite(duration) || duration <= 0) {
    return "时长未知";
  }
  const totalMinutes = Math.max(1, Math.round(duration / 60));
  const hours = Math.floor(totalMinutes / 60);
  const minutes = totalMinutes % 60;
  if (hours === 0) return `${minutes} 分钟`;
  if (minutes === 0) return `${hours} 小时`;
  return `${hours} 小时 ${minutes} 分`;
}

export function formatPublishedDate(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "日期未知";
  return new Intl.DateTimeFormat("zh-CN", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  }).format(date);
}

export function formatActivityDate(value?: string) {
  if (!value) return "";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "";
  return new Intl.DateTimeFormat("zh-CN", {
    month: "numeric",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  }).format(date);
}
