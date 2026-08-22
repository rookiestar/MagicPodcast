import type { Tag } from "@/types";

export const CONSUMPTION_QUEUES = [
  "inbox",
  "focus",
  "someday",
  "done",
] as const;

export type ConsumptionQueue = (typeof CONSUMPTION_QUEUES)[number];
export type ConsumptionAttention = "" | "stale" | "review";

export interface ConsumptionItem {
  episode_id: number;
  podcast_id: number;
  podcast_title: string;
  podcast_author: string;
  podcast_cover_url: string;
  episode_title: string;
  episode_no: string;
  duration: number;
  published_date: string;
  show_notes: string;
  original_url: string;
  image_url: string;
  notes: string;
  tags: Tag[];
  queue_state: ConsumptionQueue | null;
  dismissed_at?: string;
  queue_updated_at?: string;
  in_progress_at?: string;
  read_at?: string;
  activity_at?: string;
  attention?: ConsumptionAttention;
}

export interface ConsumptionSummary {
  counts: Record<ConsumptionQueue, number>;
  focus_limit: number;
  focus_over_limit: boolean;
}

export interface ConsumptionQueuePayload {
  queue_state: ConsumptionQueue;
  revision: number;
  items: ConsumptionItem[];
}

export interface ConsumptionQueuePlacementRequest {
  queue_state: ConsumptionQueue;
  before_episode_id: number | null;
  expected_revisions: Partial<Record<ConsumptionQueue, number>>;
  acknowledge_focus_limit?: boolean;
}

export interface ConsumptionQueuePlacementResult {
  queues: Partial<Record<ConsumptionQueue, ConsumptionQueuePayload>>;
}

export interface ConsumptionErrorDetails {
  code?: string;
  message: string;
  status?: number;
  currentCount?: number;
  focusLimit?: number;
}
