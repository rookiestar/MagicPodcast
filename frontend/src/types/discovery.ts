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
  time_basis: "published_date" | "updated_date";
  source: "最近更新";
  show_notes: string;
  show_notes_status: "available" | "missing";
  original_url: string;
  image_url: string;
  decision_state: TriageDecisionState;
  decision_updated_at?: string;
  pre_reads: DiscoveryPreRead[];
}

export type TriageDecisionState = "pending" | "shortlisted" | "discarded";

export type DiscoveryPreReadKind =
  | "summary"
  | "viewpoints"
  | "relevant"
  | "challenge";

export type DiscoveryPreReadStatus =
  | "available"
  | "pending"
  | "insufficient"
  | "failed"
  | "missing";

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

export interface TodayShortlistData {
  date: string;
  timezone: string;
  candidates: DiscoveryCandidate[];
}
