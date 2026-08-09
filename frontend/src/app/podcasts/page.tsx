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
const INITIAL_PODCASTS_PATH = buildPodcastListPath({
  page: 1,
  page_size: INITIAL_PODCAST_PAGE_SIZE,
  sort_by: "recent_update",
  view: "summary",
});

async function loadInitialPodcastPage(): Promise<
  PodcastListPage<Podcast> | undefined
> {
  try {
    const response = await fetch(
      `${resolveApiBaseUrl(false)}${INITIAL_PODCASTS_PATH}`,
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

export default async function PodcastsPage() {
  const initialPage = await loadInitialPodcastPage();
  return <PodcastsContent initialPage={initialPage} />;
}
