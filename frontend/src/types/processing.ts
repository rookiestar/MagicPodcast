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
  transcript_sha256: string;
  notes_sha256: string;
  is_current: boolean;
  created_at: string;
}

export interface ProcessingRunDetail {
  run: ProcessingRun;
  artifact?: EpisodeArtifactSet;
  current_artifact?: EpisodeArtifactSet;
  deliveries: unknown[];
  action_suggestion?: string;
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
  kind: "transcript" | "episode_notes";
  content: string;
  sha256: string;
}

export interface ProcessingErrorDetails {
  code?: string;
  message: string;
  status?: number;
}
