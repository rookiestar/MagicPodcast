import { positiveID } from "@/lib/navigationParams";
import { PODCAST_SORT_OPTIONS } from "@/lib/podcastListState";
import PodcastsContent from "./PodcastsContent";
import { resolveApiBaseUrl } from "@/lib/apiBaseUrl";
import { buildPodcastListPath } from "@/lib/podcastApiPaths";
import {
  parsePodcastListApiPayload,
  type PodcastListApiPayload,
  type PodcastListPage,
} from "@/lib/podcastListState";
import type { Podcast } from "@/types";

const INITIAL_PODCAST_PAGE_SIZE = 10;
const INITIAL_FETCH_TIMEOUT_MS = 2_500;

async function loadInitialPodcastPage(path: string): Promise<
  PodcastListPage<Podcast> | undefined
> {
  try {
    const response = await fetch(
      `${resolveApiBaseUrl(false)}${path}`,
      {
        cache: "no-store",
        headers: { Accept: "application/json" },
        signal: AbortSignal.timeout(INITIAL_FETCH_TIMEOUT_MS),
      },
    );
    if (!response.ok) {
      return undefined;
    }

    const payload =
      (await response.json()) as PodcastListApiPayload<Podcast>;
    return parsePodcastListApiPayload(payload);
  } catch {
    return undefined;
  }
}

export default async function PodcastsPage({ searchParams }: {
  searchParams?: Promise<Record<string, string | string[] | undefined>>;
} = {}) {
  const params = await searchParams ?? {};
  const sort = typeof params.sort_by === "string" && PODCAST_SORT_OPTIONS.some((option) => option.value === params.sort_by) ? params.sort_by : "recent_update";
  const rawTags = Array.isArray(params.tag_id) ? params.tag_id : params.tag_id ? [params.tag_id] : [];
  const tags = [...new Set(rawTags.map(positiveID).filter((id): id is number => id !== null))].sort((a,b) => a-b);
  const query = new URLSearchParams();
  for (const [key, value] of Object.entries(params)) {
    for (const item of Array.isArray(value) ? value : value === undefined ? [] : [value]) query.append(key, item);
  }
  const path = buildPodcastListPath({ page: 1, page_size: INITIAL_PODCAST_PAGE_SIZE, sort_by: sort, view: "summary", tag_id: tags });
  const initialPage = await loadInitialPodcastPage(path);
  return <PodcastsContent initialPage={initialPage} initialHref={`/podcasts${query.size ? `?${query}` : ""}`} initialScope={`${sort}:${tags.join(",")}`} />;
}
