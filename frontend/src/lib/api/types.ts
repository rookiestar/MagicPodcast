// 搜索参数
export interface SearchParams {
  q: string;
  type?: "all" | "podcasts" | "episodes";
  tag_id?: number | number[];
  page?: number;
  page_size?: number;
  episode_page?: number;
  episode_page_size?: number;
  include_totals?: boolean;
}
