"use client";

import useSWR from "swr";
import TodayShortlist from "@/components/discovery/TodayShortlist";
import { SimplePageLayout } from "@/components/layout/PageLayout";
import { apiClient, fetcher } from "@/lib/fetcher";
import type {
  TodayShortlistData,
  TriageDecisionResponse,
  TriageDecisionState,
} from "@/types/discovery";

const emptyToday: TodayShortlistData = {
  date: "—",
  timezone: "—",
  candidates: [],
};

export default function TodayShortlistPage() {
  const { data, error, isLoading, mutate } = useSWR<TodayShortlistData>(
    "/api/v1/discovery/shortlist/today",
    fetcher,
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

  return (
    <SimplePageLayout maxWidth={false} className="discovery-page-shell">
      {isLoading ? (
        <div className="discovery-page-state" aria-live="polite">
          正在读取今日备选…
        </div>
      ) : (
        <TodayShortlist
          data={data ?? emptyToday}
          error={error ? "今日备选暂时读取失败" : undefined}
          onDecision={saveDecision}
        />
      )}
    </SimplePageLayout>
  );
}
