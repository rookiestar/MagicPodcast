import DiscoveryPageClient from "@/components/discovery/DiscoveryPageClient";
import { resolveApiBaseUrl } from "@/lib/apiBaseUrl";
import type { DiscoveryCandidate } from "@/types/discovery";

const INITIAL_CANDIDATE_LIMIT = 5;
const INITIAL_CANDIDATES_PATH =
  `/api/v1/discovery/candidates?limit=${INITIAL_CANDIDATE_LIMIT}`;
const INITIAL_FETCH_TIMEOUT_MS = 2500;

interface DiscoveryCandidatesResponse {
  success: boolean;
  data?: DiscoveryCandidate[];
}

async function loadInitialCandidates(): Promise<
  DiscoveryCandidate[] | undefined
> {
  try {
    const response = await fetch(
      `${resolveApiBaseUrl(false)}${INITIAL_CANDIDATES_PATH}`,
      {
        cache: "no-store",
        headers: { Accept: "application/json" },
        signal: AbortSignal.timeout(INITIAL_FETCH_TIMEOUT_MS),
      },
    );
    if (!response.ok) {
      return undefined;
    }

    const payload = (await response.json()) as DiscoveryCandidatesResponse;
    if (!payload.success || !Array.isArray(payload.data)) {
      return undefined;
    }
    return payload.data;
  } catch {
    return undefined;
  }
}

export default async function DiscoveryPage() {
  const initialCandidates = await loadInitialCandidates();
  return <DiscoveryPageClient initialCandidates={initialCandidates} />;
}
