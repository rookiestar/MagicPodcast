const PODCAST_LIST_SCROLL_STORAGE_KEY = "magicpodcast:podcast-list-scroll";
const DEFAULT_SCROLL_RESTORE_MAX_AGE_MS = 30 * 60 * 1000;

export interface PodcastListStateKeyInput {
  sortBy?: string | null;
  selectedTagIds?: number[];
}

export interface PodcastListScrollSnapshot {
  stateKey: string;
  scrollY: number;
  podcastIndex: number;
  savedAt: number;
}

export type PodcastListScrollRestoreAction = "load-more" | "restore";

function getSessionStorage() {
  if (typeof window === "undefined") {
    return null;
  }

  try {
    return window.sessionStorage;
  } catch {
    return null;
  }
}

export function getPodcastListStateKey({
  sortBy,
  selectedTagIds = [],
}: PodcastListStateKeyInput) {
  const params = new URLSearchParams();

  if (sortBy) {
    params.set("sort_by", sortBy);
  }

  const uniqueTagIds = Array.from(new Set(selectedTagIds))
    .filter((id) => Number.isInteger(id) && id > 0)
    .sort((a, b) => a - b);

  uniqueTagIds.forEach((id) => params.append("tag_id", String(id)));

  const queryString = params.toString();
  return `/podcasts${queryString ? `?${queryString}` : ""}`;
}

export function savePodcastListScrollSnapshot(
  snapshot: Omit<PodcastListScrollSnapshot, "savedAt"> & {
    savedAt?: number;
  },
) {
  const storage = getSessionStorage();
  if (!storage) {
    return;
  }

  storage.setItem(
    PODCAST_LIST_SCROLL_STORAGE_KEY,
    JSON.stringify({
      ...snapshot,
      savedAt: snapshot.savedAt ?? Date.now(),
    }),
  );
}

function parsePodcastListScrollSnapshot(
  value: string | null,
): PodcastListScrollSnapshot | null {
  if (!value) {
    return null;
  }

  try {
    const parsed = JSON.parse(value) as Partial<PodcastListScrollSnapshot>;
    if (
      typeof parsed.stateKey !== "string" ||
      typeof parsed.scrollY !== "number" ||
      typeof parsed.podcastIndex !== "number" ||
      typeof parsed.savedAt !== "number" ||
      !Number.isFinite(parsed.scrollY) ||
      !Number.isInteger(parsed.podcastIndex) ||
      parsed.scrollY < 0 ||
      parsed.podcastIndex < 0
    ) {
      return null;
    }

    return {
      stateKey: parsed.stateKey,
      scrollY: parsed.scrollY,
      podcastIndex: parsed.podcastIndex,
      savedAt: parsed.savedAt,
    };
  } catch {
    return null;
  }
}

export function readPodcastListScrollSnapshot(
  stateKey: string,
  {
    now = Date.now(),
    maxAgeMs = DEFAULT_SCROLL_RESTORE_MAX_AGE_MS,
  }: {
    now?: number;
    maxAgeMs?: number;
  } = {},
) {
  const storage = getSessionStorage();
  const snapshot = parsePodcastListScrollSnapshot(
    storage?.getItem(PODCAST_LIST_SCROLL_STORAGE_KEY) ?? null,
  );

  if (!snapshot || snapshot.stateKey !== stateKey) {
    return null;
  }

  if (now - snapshot.savedAt > maxAgeMs) {
    storage?.removeItem(PODCAST_LIST_SCROLL_STORAGE_KEY);
    return null;
  }

  return snapshot;
}

export function clearPodcastListScrollSnapshot(stateKey?: string) {
  const storage = getSessionStorage();
  if (!storage) {
    return;
  }

  if (!stateKey) {
    storage.removeItem(PODCAST_LIST_SCROLL_STORAGE_KEY);
    return;
  }

  const snapshot = parsePodcastListScrollSnapshot(
    storage.getItem(PODCAST_LIST_SCROLL_STORAGE_KEY),
  );
  if (snapshot?.stateKey === stateKey) {
    storage.removeItem(PODCAST_LIST_SCROLL_STORAGE_KEY);
  }
}

export function getPodcastListScrollRestoreAction({
  snapshot,
  loadedCount,
  hasMore,
  isLoadingMore,
}: {
  snapshot: PodcastListScrollSnapshot | null;
  loadedCount: number;
  hasMore: boolean;
  isLoadingMore: boolean;
}): PodcastListScrollRestoreAction | null {
  if (!snapshot) {
    return null;
  }

  if (loadedCount <= snapshot.podcastIndex && hasMore) {
    return isLoadingMore ? null : "load-more";
  }

  return "restore";
}

export function restorePodcastListScroll(snapshot: PodcastListScrollSnapshot) {
  if (typeof window === "undefined") {
    return;
  }

  window.requestAnimationFrame(() => {
    window.scrollTo(0, snapshot.scrollY);
  });
}
