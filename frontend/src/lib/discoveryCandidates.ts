import type { DiscoveryCandidate } from "@/types/discovery";

export const DISCOVERY_CANDIDATES_PATH =
  "/api/v1/discovery/candidates?limit=1000&view=summary";
export const DISCOVERY_CANDIDATES_CACHE_TTL_MS = 30 * 60 * 1000;

const DISCOVERY_FETCH_TIMEOUT_MS = 8000;
const DISCOVERY_RETRY_DELAYS_MS = [1000, 2000] as const;
const DISCOVERY_CANDIDATES_CACHE_KEY = "magicpodcast:discovery-candidates:v2";
const DISCOVERY_DETAIL_CACHE_TTL_MS = 5 * 60 * 1000;
const DISCOVERY_DETAIL_CACHE_MAX_ITEMS = 50;
const discoveryCandidateDetails = new Map<
  number,
  { savedAt: number; data: DiscoveryCandidate }
>();
const discoveryCandidateDetailRequests = new Map<
  number,
  Promise<DiscoveryCandidate>
>();

interface DiscoveryCandidatesResponse {
  success: boolean;
  data?: DiscoveryCandidate[];
}

interface DiscoveryCandidatesCache {
  savedAt: number;
  data: DiscoveryCandidate[];
}

interface DiscoveryCandidatesStorage {
  getItem(key: string): string | null;
  setItem(key: string, value: string): void;
  removeItem(key: string): void;
}

class DiscoveryCandidatesRequestError extends Error {
  constructor(
    message: string,
    readonly retryable: boolean,
  ) {
    super(message);
    this.name = "DiscoveryCandidatesRequestError";
  }
}

function isRetryableStatus(status: number) {
  return status === 408 || status === 425 || status === 429 || status >= 500;
}

function wait(delayMs: number) {
  if (delayMs <= 0) return Promise.resolve();
  return new Promise<void>((resolve) => {
    setTimeout(resolve, delayMs);
  });
}

function isDiscoveryCandidate(value: unknown): value is DiscoveryCandidate {
  if (!value || typeof value !== "object") return false;
  const candidate = value as Partial<DiscoveryCandidate>;
  return (
    typeof candidate.episode_id === "number" &&
    typeof candidate.podcast_id === "number" &&
    typeof candidate.episode_title === "string" &&
    typeof candidate.podcast_title === "string"
  );
}

function isDiscoveryCandidateDetail(
  value: unknown,
): value is DiscoveryCandidate {
  if (!isDiscoveryCandidate(value)) return false;
  const hasValidShowNotes =
    (value.show_notes_status === "available" &&
      typeof value.show_notes === "string") ||
    (value.show_notes_status === "missing" &&
      (value.show_notes === undefined || typeof value.show_notes === "string"));
  return (
    hasValidShowNotes &&
    Array.isArray(value.pre_reads)
  );
}

export function clearDiscoveryCandidateDetailsCache() {
  discoveryCandidateDetails.clear();
  discoveryCandidateDetailRequests.clear();
}

export function fetchDiscoveryCandidateDetails(
  episodeID: number,
  request: typeof fetch = fetch,
): Promise<DiscoveryCandidate> {
  const cached = discoveryCandidateDetails.get(episodeID);
  if (cached && Date.now() - cached.savedAt <= DISCOVERY_DETAIL_CACHE_TTL_MS) {
    return Promise.resolve(cached.data);
  }
  discoveryCandidateDetails.delete(episodeID);

  const existing = discoveryCandidateDetailRequests.get(episodeID);
  if (existing) return existing;

  const detailRequest = (async () => {
    const response = await request(
      `/api/v1/discovery/candidates/${episodeID}`,
      {
        cache: "no-store",
        headers: { Accept: "application/json" },
        signal: AbortSignal.timeout(DISCOVERY_FETCH_TIMEOUT_MS),
      },
    );
    if (!response.ok) {
      throw new DiscoveryCandidatesRequestError(
        `Discovery candidate request failed with HTTP ${response.status}`,
        isRetryableStatus(response.status),
      );
    }

    const payload = (await response.json()) as {
      success: boolean;
      data?: DiscoveryCandidate;
    };
    if (!payload.success || !isDiscoveryCandidateDetail(payload.data)) {
      throw new DiscoveryCandidatesRequestError(
        "Discovery candidate response is invalid",
        true,
      );
    }

    if (discoveryCandidateDetails.size >= DISCOVERY_DETAIL_CACHE_MAX_ITEMS) {
      const oldestEpisodeID = discoveryCandidateDetails.keys().next().value;
      if (oldestEpisodeID !== undefined) {
        discoveryCandidateDetails.delete(oldestEpisodeID);
      }
    }
    discoveryCandidateDetails.set(episodeID, {
      savedAt: Date.now(),
      data: payload.data,
    });
    return payload.data;
  })();

  discoveryCandidateDetailRequests.set(episodeID, detailRequest);
  const cleanup = () => {
    if (discoveryCandidateDetailRequests.get(episodeID) === detailRequest) {
      discoveryCandidateDetailRequests.delete(episodeID);
    }
  };
  void detailRequest.then(cleanup, cleanup);
  return detailRequest;
}

export function readDiscoveryCandidatesCache(
  storage: DiscoveryCandidatesStorage,
  now = Date.now(),
): DiscoveryCandidate[] | undefined {
  try {
    const raw = storage.getItem(DISCOVERY_CANDIDATES_CACHE_KEY);
    if (!raw) return undefined;

    const cached = JSON.parse(raw) as Partial<DiscoveryCandidatesCache>;
    if (
      typeof cached.savedAt !== "number" ||
      now - cached.savedAt > DISCOVERY_CANDIDATES_CACHE_TTL_MS ||
      !Array.isArray(cached.data) ||
      cached.data.length === 0 ||
      !cached.data.every(isDiscoveryCandidate)
    ) {
      return undefined;
    }

    return cached.data;
  } catch {
    return undefined;
  }
}

export function writeDiscoveryCandidatesCache(
  storage: DiscoveryCandidatesStorage,
  candidates: DiscoveryCandidate[],
  now = Date.now(),
) {
  try {
    if (candidates.length === 0) {
      storage.removeItem(DISCOVERY_CANDIDATES_CACHE_KEY);
      return;
    }

    const cached: DiscoveryCandidatesCache = {
      savedAt: now,
      data: candidates,
    };
    storage.setItem(DISCOVERY_CANDIDATES_CACHE_KEY, JSON.stringify(cached));
  } catch {
    // Session storage can be unavailable or full; loading continues normally.
  }
}

export async function fetchDiscoveryCandidatesWithRetry(
  request: typeof fetch = fetch,
  retryDelaysMs: readonly number[] = DISCOVERY_RETRY_DELAYS_MS,
): Promise<DiscoveryCandidate[]> {
  let lastError: unknown;

  for (let attempt = 0; attempt <= retryDelaysMs.length; attempt += 1) {
    try {
      const response = await request(DISCOVERY_CANDIDATES_PATH, {
        cache: "no-store",
        headers: { Accept: "application/json" },
        signal: AbortSignal.timeout(DISCOVERY_FETCH_TIMEOUT_MS),
      });

      if (!response.ok) {
        throw new DiscoveryCandidatesRequestError(
          `Discovery candidates request failed with HTTP ${response.status}`,
          isRetryableStatus(response.status),
        );
      }

      const payload = (await response.json()) as DiscoveryCandidatesResponse;
      if (!payload.success || !Array.isArray(payload.data)) {
        throw new DiscoveryCandidatesRequestError(
          "Discovery candidates response is invalid",
          true,
        );
      }

      return payload.data;
    } catch (error) {
      lastError = error;
      const retryable =
        !(error instanceof DiscoveryCandidatesRequestError) || error.retryable;
      const hasRetry = attempt < retryDelaysMs.length;
      if (!retryable || !hasRetry) break;
      await wait(retryDelaysMs[attempt]);
    }
  }

  throw lastError instanceof Error
    ? lastError
    : new Error("Discovery candidates request failed");
}
