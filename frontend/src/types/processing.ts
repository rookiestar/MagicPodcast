export type ProcessingRunStatus =
  | "queued"
  | "running"
  | "waiting_external"
  | "completed"
  | "failed"
  | "cancelled";

export interface ProcessingRun {
  id: number;
  episode_id: number;
  pipeline_version: string;
  trigger_source: "manual" | "scheduled";
  status: ProcessingRunStatus;
  current_step: string;
  previous_run_id?: number;
  attempt_count: number;
  max_attempts: number;
  next_attempt_at?: string;
  started_at?: string;
  finished_at?: string;
  cancelled_at?: string;
  error_code?: string;
  error_message?: string;
  error_retryable: boolean;
  created_at: string;
  updated_at: string;
}

export interface EpisodeArtifactSet {
  id: number;
  run_id: number;
  episode_id: number;
  pipeline_version: string;
  manifest_path: string;
  manifest_sha256: string;
  minutes_summary_sha256?: string;
  transcript_sha256: string;
  notes_sha256: string;
  transcript_timeline_sha256?: string;
  capabilities: EpisodeArtifactCapabilities;
  is_current: boolean;
  created_at: string;
}

export interface EpisodeArtifactCapabilities {
  minutes_summary: boolean;
  transcript: boolean;
  structured_timeline: boolean;
  matching_audio: boolean;
  legacy_episode_notes: boolean;
}

export interface ProcessingRunDetail {
  run: ProcessingRun;
  artifact?: EpisodeArtifactSet;
  current_artifact?: EpisodeArtifactSet;
  deliveries: KnowledgeDelivery[];
  action_suggestion?: string;
}

export type KnowledgeDeliveryStatus =
  "pending" | "delivering" | "delivered" | "failed" | "cancelled";

export interface KnowledgeDelivery {
  id: number;
  artifact_set_id: number;
  target: string;
  destination: string;
  adapter_version: string;
  status: KnowledgeDeliveryStatus;
  attempt_count: number;
  remote_ref?: string;
  public_url?: string;
  error_code?: string;
  error_message?: string;
  error_retryable: boolean;
  delivered_at?: string;
  created_at: string;
  updated_at: string;
}

export interface ProcessingStartResult {
  run?: ProcessingRun;
  reused_active: boolean;
  reused_successful: boolean;
  audio_asset?: EpisodeAudioAsset;
  preparing_audio: boolean;
}

export interface EpisodeAudioAsset {
  id: number;
  episode_id: number;
  status: "queued" | "downloading" | "ready" | "failed";
  sha256?: string;
  size_bytes: number;
  duration_seconds: number;
  media_type?: string;
  extension?: string;
  error_code?: string;
  error_message?: string;
  queued_at: string;
  downloading_at?: string;
  ready_at?: string;
  failed_at?: string;
  created_at: string;
  updated_at: string;
}

export interface ArtifactContent {
  kind: ArtifactContentKind;
  content: string;
  sha256: string;
  segments?: TranscriptSegment[];
  timeline_sha256?: string;
  media_available: boolean;
}

export type ArtifactContentKind =
  "minutes_summary" | "transcript" | "episode_notes";

export interface TranscriptSegment {
  order: number;
  speaker: string;
  start_ms: number;
  text: string;
}

export interface ProcessingErrorDetails {
  code?: string;
  message: string;
  status?: number;
}

export type ProcessingScheduleRunStatus = "running" | "completed" | "failed";

export interface ProcessingScheduleItem {
  id: number;
  schedule_run_id: number;
  episode_id: number;
  queue_position: number;
  outcome: "started" | "skipped";
  reason?: string;
  processing_run_id?: number;
  created_at: string;
  updated_at: string;
}

export interface ProcessingScheduleRun {
  id: number;
  scheduled_for: string;
  cron_expression: string;
  timezone: string;
  batch_size: number;
  status: ProcessingScheduleRunStatus;
  candidate_count: number;
  started_count: number;
  skipped_count: number;
  error_code?: string;
  error_message?: string;
  finished_at?: string;
  created_at: string;
  updated_at: string;
}

export interface ProcessingScheduleRunDetail {
  run: ProcessingScheduleRun;
  items: ProcessingScheduleItem[];
}

export interface ProcessingScheduleStatus {
  enabled: boolean;
  cron: string;
  timezone: string;
  batch_size: number;
  next_run_at?: string;
  latest_run?: ProcessingScheduleRunDetail;
}
