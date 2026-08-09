"use client";

import useSWR from "swr";
import DiscoveryDesk from "@/components/discovery/DiscoveryDesk";
import { SimplePageLayout } from "@/components/layout/PageLayout";
import { apiClient, fetcher } from "@/lib/fetcher";
import type {
  DiscoveryCandidate,
  TriageDecisionResponse,
  TriageDecisionState,
} from "@/types/discovery";

const DISCOVERY_CANDIDATES_PATH =
  "/api/v1/discovery/candidates?limit=30";

interface DiscoveryPageClientProps {
  initialCandidates?: DiscoveryCandidate[];
}

export default function DiscoveryPageClient({
  initialCandidates,
}: DiscoveryPageClientProps) {
  const { data, error, isLoading, mutate } = useSWR<DiscoveryCandidate[]>(
    DISCOVERY_CANDIDATES_PATH,
    fetcher,
    {
      fallbackData: initialCandidates,
      revalidateOnMount: true,
    },
  );

  const saveDecision = async (
    episodeID: number,
    state: TriageDecisionState,
  ) => {
    const response = await apiClient.put<{
      success: boolean;
      data: TriageDecisionResponse;
    }>(`/api/v1/discovery/candidates/${episodeID}/decision`, { state });
    await mutate();
    return response.data.data;
  };

  const hasCandidates = data !== undefined;

  return (
    <SimplePageLayout maxWidth={false} className="discovery-page-shell">
      {!hasCandidates && isLoading ? (
        <div className="discovery-page-state" aria-live="polite">
          正在读取个人播客库…
        </div>
      ) : !hasCandidates && error ? (
        <div className="discovery-page-state discovery-page-error" role="alert">
          <strong>暂时无法读取最近更新</strong>
          <span>播客库、搜索与其他功能仍可继续使用。</span>
        </div>
      ) : (
        <DiscoveryDesk candidates={data ?? []} onDecision={saveDecision} />
      )}
    </SimplePageLayout>
  );
}
