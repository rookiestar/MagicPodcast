import type { Tag } from "@/types";
import type { ShowNotesDocument } from "@/types/showNotes";

export type { ShowNotesDocument } from "@/types/showNotes";

export const CONSUMPTION_QUEUES = [
  "inbox",
  "focus",
  "someday",
  "done",
] as const;

export type ConsumptionQueue = (typeof CONSUMPTION_QUEUES)[number];
export type ConsumptionAttention = "" | "stale" | "review";
export type CompletionHistoryStatus =
  | ConsumptionQueue
  | "dismissed"
  | "unassigned";

export interface CompletionUndo {
  token: string;
  expires_at: string;
}

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
  show_notes_document: ShowNotesDocument;
  original_url: string;
  image_url: string;
  notes: string;
  tags: Tag[];
  queue_state: ConsumptionQueue | null;
  dismissed_at?: string;
  queue_updated_at?: string;
  completed_at?: string;
  in_progress_at?: string;
  read_at?: string;
  activity_at?: string;
  attention?: ConsumptionAttention;
  completion_undo?: CompletionUndo;
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
  has_more: boolean;
}

export interface ConsumptionQueuePlacementRequest {
  queue_state: ConsumptionQueue;
  before_episode_id: number | null;
  expected_revisions: Partial<Record<ConsumptionQueue, number>>;
  acknowledge_focus_limit?: boolean;
}

export interface ConsumptionQueuePlacementResult {
  queues: Partial<Record<ConsumptionQueue, ConsumptionQueuePayload>>;
  completion_undo?: CompletionUndo;
}

export interface CompletionHistoryItem {
  episode_id: number;
  podcast_id: number;
  podcast_title: string;
  podcast_cover_url: string;
  episode_title: string;
  episode_no: string;
  image_url: string;
  completed_at: string;
  current_status: CompletionHistoryStatus;
}

export interface CompletionHistoryPayload {
  items: CompletionHistoryItem[];
  total_count: number;
  match_count: number;
  has_more: boolean;
  next_cursor?: string;
  search_query: string;
}

export interface ConsumptionErrorDetails {
  code?: string;
  message: string;
  status?: number;
  currentCount?: number;
  focusLimit?: number;
}
