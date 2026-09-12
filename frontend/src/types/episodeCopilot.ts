export type EpisodeCopilotSelectionSource = "show_notes" | "transcript";
export type EpisodeCopilotProfileID = "quick" | "balanced" | "deep";

export interface EpisodeCopilotProfile {
  id: EpisodeCopilotProfileID;
  model: string;
  effort: string;
  service_tier?: string;
  /** Stable human speed tier name, e.g. Fast or Standard. */
  service_tier_name: string;
  is_default: boolean;
}

export type EpisodePersonStatus = "confirmed" | "pending";
export type EpisodePersonRole = "host" | "guest" | "unknown";

export interface EpisodePersonCandidate {
 role_user_confirmed?: boolean;
  id: number;
  display_name: string;
  aliases: string[];
  identity_note: string;
  role: EpisodePersonRole;
  status: EpisodePersonStatus;
  status_reason: string;
  evidence_kind?: string;
  evidence_locator?: string;
}

export interface EpisodeCopilotContextScope {
 preparation_state?: "required" | "outdated" | "ready" | "no_transcript";
 excluded_people?: EpisodePersonCandidate[];
  episode_id: number;
  show_notes_available: boolean;
  transcript_available: boolean;
  private_note_available: boolean;
  profiles: EpisodeCopilotProfile[];
  default_profile_id: EpisodeCopilotProfileID;
  people?: EpisodePersonCandidate[];
  index_ready?: boolean;
  index_status?: string;
}

export interface EpisodeCopilotQuestion {
  question: string;
  selection: string;
  selection_source: EpisodeCopilotSelectionSource | "";
  include_private_note: boolean;
  profile_id: EpisodeCopilotProfileID;
  target_person_id?: number;
}

export type EpisodeCopilotEventType =
  | "context"
  | "status"
  | "answer_delta"
  | "error"
  | "complete";

/**
 * User-visible question stages, in fixed presentation order. Stage IDs are
 * part of the SSE contract; the panel renders its own labels.
 */
export type EpisodeCopilotStage =
  | "read_context"
  | "library_search"
  | "research_runtime"
  | "public_research"
  | "source_validation"
  | "answer_runtime"
  | "compose_answer"
  | "citation_validation";

/** Provider-neutral activity category from the Runtime Host. */
export type EpisodeCopilotActivityCategory =
  | "stage"
  | "web_search"
  | "reasoning"
  | "plan"
  | "agent_message"
  | "turn"
  | "item";

export type EpisodeCopilotActivityState =
  | "started"
  | "updated"
  | "completed"
  | "failed";

/**
 * One sanitized execution activity. Activities update in place by stable
 * `id`; display text is bounded plain text and metadata only carries public
 * facts such as candidate domains.
 */
export interface EpisodeCopilotActivity {
  id: string;
  ordinal: number;
  stage: EpisodeCopilotStage;
  category: EpisodeCopilotActivityCategory;
  state: EpisodeCopilotActivityState;
  text?: string;
  observed_at: string;
  elapsed_ms?: number;
  metadata?: Record<string, string>;
}

/** Per-stage durations (ms) reported once on complete; 0 = stage not run. */
export interface EpisodeCopilotStageTimings {
  research_runtime_ready_ms?: number;
  public_research_ms?: number;
  source_validation_ms?: number;
  answer_runtime_ready_ms?: number;
  citation_validation_ms?: number;
}

export interface EpisodeAttributionView {
  id: number;
  person_id?: number | null;
  display_name?: string;
  source_kind: string;
  source_version: string;
  fragment_order: number;
  speaker_label: string;
  start_ms: number;
  text: string;
  status: string;
  evidence_kind?: string;
  evidence_locator?: string;
  user_confirmed?: boolean;
}

export interface EpisodePeoplePayload {
  revision?: number;
  draft?: PersonReviewDraft;
 preparation_state?: EpisodeCopilotContextScope["preparation_state"];
 excluded_people?: EpisodePersonCandidate[];
  episode_id: number;
  source_version: string;
  index_ready: boolean;
  people: EpisodePersonCandidate[];
  attributions: EpisodeAttributionView[];
}

export interface EpisodeCopilotStreamEvent {
  type: EpisodeCopilotEventType;
  stage?: string;
  message?: string;
  code?: string;
  retryable?: boolean;
  transcript_used: boolean;
  private_note_included: boolean;
  profile_id?: EpisodeCopilotProfileID;
  first_content_ms?: number;
  total_ms?: number;
  activity?: EpisodeCopilotActivity;
  stage_timings?: EpisodeCopilotStageTimings;
}

export interface PersonReviewMatch {
 applied?: boolean;
 role_edited?: boolean;
 key: string; person_id?: number; display_name: string; role: string;
 speaker_label: string; orders: number[]; selected: boolean;
 evidence_locator?: string; uncertain: boolean;
}
export interface PersonReviewDraft {
 id: number; revision: number; source_version: string;
 matches: PersonReviewMatch[]; outdated: boolean;
}
