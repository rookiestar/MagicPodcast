export type EpisodeCopilotSelectionSource = "show_notes" | "transcript";

export interface EpisodeCopilotProfile {
  id: string;
  model: string;
  effort: string;
  service_tier?: string;
  is_default: boolean;
}

export interface EpisodeCopilotContextScope {
  episode_id: number;
  show_notes_available: boolean;
  transcript_available: boolean;
  private_note_available: boolean;
  profiles: EpisodeCopilotProfile[];
  default_profile_id: string;
}

export interface EpisodeCopilotQuestion {
  question: string;
  selection: string;
  selection_source: EpisodeCopilotSelectionSource | "";
  include_private_note: boolean;
  profile_id: string;
}

export type EpisodeCopilotEventType =
  | "context"
  | "status"
  | "answer_delta"
  | "error"
  | "complete";

export interface EpisodeCopilotStreamEvent {
  type: EpisodeCopilotEventType;
  stage?: string;
  message?: string;
  code?: string;
  retryable?: boolean;
  transcript_used: boolean;
  private_note_included: boolean;
  profile_id?: string;
  first_content_ms?: number;
  total_ms?: number;
}
