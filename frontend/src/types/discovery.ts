export interface DiscoveryCandidate {
  episode_id: number;
  podcast_id: number;
  podcast_title: string;
  podcast_author: string;
  podcast_cover_url: string;
  episode_title: string;
  episode_no: string;
  duration: number;
  candidate_time: string;
  time_basis: "fetched_at" | "created_at";
  source: "最近更新";
  excerpt?: string;
  metadata_only?: boolean;
  show_notes?: string;
  show_notes_status: "available" | "missing";
  original_url: string;
  image_url: string;
  decision_state: TriageDecisionState;
  decision_updated_at?: string;
  queue_state?: QueueState | null;
  dismissed_at?: string;
  queue_updated_at?: string;
  in_progress_at?: string;
  read_at?: string;
  pre_reads?: DiscoveryPreRead[];
}

export type TriageDecisionState = "pending" | "shortlisted" | "discarded";
export type QueueState = "inbox" | "focus" | "someday" | "done";

export type DiscoveryPreReadKind =
  "summary" | "viewpoints" | "relevant" | "challenge";

export type DiscoveryPreReadStatus =
  "available" | "pending" | "insufficient" | "failed" | "missing";

export interface DiscoveryPreReadSource {
  kind: string;
  label: string;
  url?: string;
}

export interface DiscoveryPreRead {
  kind: DiscoveryPreReadKind;
  label: string;
  status: DiscoveryPreReadStatus;
  content: string;
  relation_strength?: "明确相关" | "弱相关";
  sources: DiscoveryPreReadSource[];
  generated_at: string;
  version: string;
}

export interface TriageDecisionResponse {
  state: TriageDecisionState;
  decision_updated_at: string;
}

export interface DiscoveryConsumptionResponse {
  episode_id: number;
  queue_state: QueueState | null;
  dismissed_at?: string;
  queue_updated_at?: string;
  in_progress_at?: string;
  read_at?: string;
}

export type HomepageReportType = "daily" | "weekly";

export interface HomepageReportEpisode {
  episode_id: number;
  order: number;
  podcast_id: number;
  podcast_title: string;
  podcast_cover_url?: string;
  episode_title: string;
  episode_no?: string;
  duration?: number;
  published_date?: string;
  image_url?: string;
  link?: string;
  /** Report-authored rationale; never ordinary Show Notes (#93). */
  recommendation?: string;
  /** Program context (e.g. Show Notes excerpt). */
  context?: string;
  /** Legacy alias; prefer context. */
  excerpt?: string;
  decision_state: TriageDecisionState;
  decision_updated_at?: string;
  queue_state?: QueueState | null;
  dismissed_at?: string;
  queue_updated_at?: string;
  in_progress_at?: string;
  read_at?: string;
}

export interface HomepageReport {
  id: number;
  job_id: number;
  workflow_id: number;
  workflow_name: string;
  report_type: HomepageReportType | string;
  title: string;
  theme?: string;
  content?: string;
  summary?: string;
  completed_at: string;
  generated_at: string;
  episode_count: number;
  episodes: HomepageReportEpisode[];
  /** History list rows omit full Markdown (#95). */
  metadata_only?: boolean;
}

export interface HomepageReportsData {
  date: string;
  timezone: string;
  today: HomepageReport[];
  history?: HomepageReport[];
}
