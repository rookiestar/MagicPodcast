// 播客单集清单：外部发现资料，独立于个人播客库。

export interface CollectionPreviewItem {
  position: number;
  external_episode_id: string;
  episode_title: string;
  podcast_title: string;
  podcast_author: string;
  recommendation: string;
  duration: number;
  published_at: string | null;
  episode_url: string;
  pay_type: string;
  is_private_media: boolean;
}

export interface CollectionPreview {
  preview_id: string;
  platform: string;
  external_id: string;
  title: string;
  description: string;
  author: string;
  source_url: string;
  total_known: boolean;
  read_count: number;
  items: CollectionPreviewItem[];
}

export interface CollectionImportResult {
  duplicate: boolean;
  collection_id: number;
}

export interface CollectionSummary {
  id: number;
  title: string;
  description: string;
  author: string;
  platform: string;
  external_id: string;
  source_url: string;
  total_known: boolean;
  item_count: number;
  adopted_count: number;
  created_at: string;
  last_refreshed_at: string | null;
}

export interface CollectionItemDetail {
  id: number;
  position: number;
  external_episode_id: string;
  external_podcast_id: string;
  podcast_title: string;
  podcast_author: string;
  podcast_cover_url: string;
  episode_title: string;
  recommendation: string;
  shownotes: string;
  duration: number;
  published_at: string | null;
  image_url: string;
  episode_url: string;
  pay_type: string;
  is_private_media: boolean;
  adopted_episode_id: number | null;
  adopted_episode_title: string;
  adopted_episode_queue: string | null;
}

export interface CollectionDetail {
  id: number;
  title: string;
  description: string;
  author: string;
  platform: string;
  external_id: string;
  source_url: string;
  total_known: boolean;
  revision: number;
  item_count: number;
  adopted_count: number;
  created_at: string;
  last_refreshed_at: string | null;
  items: CollectionItemDetail[];
}

export type CollectionAdoptedFilter = "all" | "unadopted" | "adopted";

export interface CollectionApiError {
  code?: string;
  message?: string;
}
