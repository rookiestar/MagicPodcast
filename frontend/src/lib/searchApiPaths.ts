import type { SearchParams } from "@/lib/api/types";

const SEARCH_BASE_PATH = "/api/v1/search";

export function buildSearchPath(params: SearchParams) {
  const queryParams = new URLSearchParams();

  queryParams.append("q", params.q);

  if (params.type) queryParams.append("type", params.type);

  if (params.tag_id) {
    const tagIds = Array.isArray(params.tag_id) ? params.tag_id : [params.tag_id];
    tagIds.forEach((id) => queryParams.append("tag_id", id.toString()));
  }

  if (params.page) queryParams.append("page", params.page.toString());
  if (params.page_size) {
    queryParams.append("page_size", params.page_size.toString());
  }
  if (params.episode_page) {
    queryParams.append("episode_page", params.episode_page.toString());
  }
  if (params.episode_page_size) {
    queryParams.append(
      "episode_page_size",
      params.episode_page_size.toString(),
    );
  }

  return `${SEARCH_BASE_PATH}?${queryParams.toString()}`;
}
