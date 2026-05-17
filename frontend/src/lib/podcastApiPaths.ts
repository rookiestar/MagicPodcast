export interface PodcastListPathParams {
  page?: number;
  page_size?: number;
  sort_by?: string;
  tag_id?: number | number[];
  search?: string;
}

const PODCASTS_BASE_PATH = "/api/v1/podcasts";

export function buildPodcastListPath(params: PodcastListPathParams = {}) {
  const queryParams = new URLSearchParams();

  if (params.page) queryParams.set("page", params.page.toString());
  if (params.page_size) {
    queryParams.set("page_size", params.page_size.toString());
  }
  if (params.sort_by) queryParams.set("sort_by", params.sort_by);
  if (params.search) queryParams.set("search", params.search);
  if (params.tag_id) {
    const tagIds = Array.isArray(params.tag_id) ? params.tag_id : [params.tag_id];
    tagIds.forEach((id) => queryParams.append("tag_id", id.toString()));
  }

  const query = queryParams.toString();
  return query ? `${PODCASTS_BASE_PATH}?${query}` : PODCASTS_BASE_PATH;
}

export function buildPodcastDetailPath(id: number) {
  return `${PODCASTS_BASE_PATH}/${id}`;
}

export function buildPodcastBatchPath() {
  return `${PODCASTS_BASE_PATH}/batch`;
}

export function buildPodcastNotesPath(id: number) {
  return `${buildPodcastDetailPath(id)}/notes`;
}

export function buildPodcastTagsPath(id: number) {
  return `${buildPodcastDetailPath(id)}/tags`;
}

export function buildPodcastTagPath(id: number, tagId: number) {
  return `${buildPodcastTagsPath(id)}/${tagId}`;
}

export function buildPodcastCustomCoverPath(id: number) {
  return `${buildPodcastDetailPath(id)}/custom-cover`;
}

export function buildPodcastEpisodesPath(
  id: number,
  page: number,
  pageSize: number,
) {
  const queryParams = new URLSearchParams({
    page: page.toString(),
    page_size: pageSize.toString(),
  });

  return `${buildPodcastDetailPath(id)}/episodes?${queryParams.toString()}`;
}

export function buildPodcastEpisodesCollectionPath(id: number) {
  return `${buildPodcastDetailPath(id)}/episodes`;
}
