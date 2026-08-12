"use client";

import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";
import useSWR from "swr";
import DiscoveryDesk from "@/components/discovery/DiscoveryDesk";
import DiscoveryFocusSummary from "@/components/discovery/DiscoveryFocusSummary";
import WorkflowReportWorkbench from "@/components/discovery/WorkflowReportWorkbench";
import { SimplePageLayout } from "@/components/layout/PageLayout";
import {
  DISCOVERY_CANDIDATES_PATH,
  fetchDiscoveryCandidateDetails,
  fetchDiscoveryCandidatesWithRetry,
  readDiscoveryCandidatesCache,
  writeDiscoveryCandidatesCache,
} from "@/lib/discoveryCandidates";
import {
  DISCOVERY_REPORTS_PATH,
  fetchHomepageReports,
} from "@/lib/discoveryReports";
import { revalidateConsumptionSummary } from "@/lib/api/consumption";
import { apiClient } from "@/lib/fetcher";
import type {
  DiscoveryConsumptionResponse,
  DiscoveryCandidate,
  HomepageReportsData,
  TriageDecisionResponse,
  TriageDecisionState,
} from "@/types/discovery";
import type { ConsumptionItem } from "@/types/consumption";

interface DiscoveryPageClientProps {
  initialCandidates?: DiscoveryCandidate[];
}

const SKELETON_ROWS = [0, 1, 2, 3, 4];
const discoveryCandidatesFetcher = () => fetchDiscoveryCandidatesWithRetry();
const discoveryReportsFetcher = () => fetchHomepageReports(30);

function legacyStateFromConsumption(
  state: DiscoveryConsumptionResponse,
): TriageDecisionState {
  if (state.queue_state) return "shortlisted";
  if (state.dismissed_at) return "discarded";
  return "pending";
}

function applyConsumptionToCandidate(
  candidate: DiscoveryCandidate,
  state: DiscoveryConsumptionResponse,
): DiscoveryCandidate {
  return {
    ...candidate,
    decision_state: legacyStateFromConsumption(state),
    decision_updated_at:
      state.queue_updated_at ??
      state.dismissed_at ??
      candidate.decision_updated_at,
    queue_state: state.queue_state,
    dismissed_at: state.dismissed_at,
    queue_updated_at: state.queue_updated_at,
    in_progress_at: state.in_progress_at,
    read_at: state.read_at,
  };
}

function applyConsumptionToReports(
  reports: HomepageReportsData | undefined,
  state: DiscoveryConsumptionResponse,
): HomepageReportsData | undefined {
  if (!reports) return reports;
  const updateGroup = (group: HomepageReportsData["today"]) =>
    group.map((report) => ({
      ...report,
      episodes: report.episodes.map((episode) =>
        episode.episode_id === state.episode_id
          ? {
              ...episode,
              decision_state: legacyStateFromConsumption(state),
              decision_updated_at:
                state.queue_updated_at ??
                state.dismissed_at ??
                episode.decision_updated_at,
              queue_state: state.queue_state,
              dismissed_at: state.dismissed_at,
              queue_updated_at: state.queue_updated_at,
              in_progress_at: state.in_progress_at,
              read_at: state.read_at,
            }
          : episode,
      ),
    }));
  return {
    ...reports,
    today: updateGroup(reports.today),
    history: reports.history ? updateGroup(reports.history) : reports.history,
  };
}

function DiscoveryPageSkeleton({
  failed,
  onRetry,
  reportContent,
  focusContent,
}: {
  failed: boolean;
  onRetry: () => void;
  reportContent?: ReactNode;
  focusContent?: ReactNode;
}) {
  return (
    <main
      className="discovery-desk discovery-unified-layout discovery-page-skeleton"
      aria-label="正在读取工作流最近更新"
      aria-busy={!failed}
    >
      <p className="sr-only" aria-live="polite">
        {failed
          ? "最近更新暂时无法读取，可以重新尝试。"
          : "正在后台读取工作流最近更新。"}
      </p>

      <aside className="discovery-sidebar" aria-label="Discovery 导航与筛选">
        <div className="discovery-workbench-copy editorial-title-group">
          <h1 className="editorial-section-title">Discovery</h1>
          <span className="discovery-source-label">最近更新 · 7 天</span>
        </div>
        <div className="discovery-status-filters" aria-hidden="true">
          {["全部", "未读", "未收集"].map((label) => (
            <span key={label}>
              {label}
              <strong>—</strong>
            </span>
          ))}
        </div>
        <section
          className="discovery-focus-rail"
          aria-label="Focus 快捷区域"
        >
          {focusContent}
        </section>
      </aside>

      <section className="discovery-stream">
        {reportContent}
        {failed && (
          <div className="discovery-load-notice is-error" role="alert">
            <span>
              <strong>最近更新暂时无法读取</strong>
              <small>连接多次未成功，页面其他功能仍可继续使用。</small>
            </span>
            <button type="button" onClick={onRetry}>
              重新尝试
            </button>
          </div>
        )}

        <section className="discovery-loading-list" aria-hidden="true">
          <div className="discovery-loading-header">
            <span className="discovery-skeleton-block discovery-skeleton-heading" />
            <span className="discovery-skeleton-block discovery-skeleton-count" />
          </div>
          <div className="discovery-loading-workspace">
            {SKELETON_ROWS.map((row) => (
              <div className="discovery-loading-row" key={row}>
                <span className="discovery-skeleton-block discovery-skeleton-index" />
                <span className="discovery-skeleton-block discovery-skeleton-cover" />
                <span className="discovery-loading-copy">
                  <span className="discovery-skeleton-block is-short" />
                  <span className="discovery-skeleton-block is-title" />
                  <span className="discovery-skeleton-block is-body" />
                </span>
              </div>
            ))}
          </div>
        </section>
      </section>

    </main>
  );
}

export default function DiscoveryPageClient({
  initialCandidates,
}: DiscoveryPageClientProps) {
  const [cachedCandidates, setCachedCandidates] = useState<
    DiscoveryCandidate[] | undefined
  >();
  const [consumptionOverrides, setConsumptionOverrides] = useState<
    Record<number, DiscoveryConsumptionResponse>
  >({});
  const decisionRequestsRef = useRef<
    Map<
      number,
      {
        state: TriageDecisionState;
        promise: Promise<TriageDecisionResponse>;
      }
    >
  >(new Map());

  const { data, error, isValidating, mutate } = useSWR<DiscoveryCandidate[]>(
    DISCOVERY_CANDIDATES_PATH,
    discoveryCandidatesFetcher,
    {
      fallbackData: initialCandidates,
      revalidateOnMount: true,
      revalidateOnFocus: false,
      revalidateOnReconnect: true,
      shouldRetryOnError: false,
      keepPreviousData: true,
    },
  );

  // Reports load independently so slow/failed reports never block 最近更新 (#90).
  // History in the first payload is metadata-only (#95).
  const {
    data: reportsData,
    error: reportsError,
    isValidating: reportsValidating,
    mutate: mutateReports,
  } = useSWR<HomepageReportsData>(
    DISCOVERY_REPORTS_PATH,
    discoveryReportsFetcher,
    {
      revalidateOnMount: true,
      revalidateOnFocus: false,
      revalidateOnReconnect: true,
      shouldRetryOnError: false,
    },
  );

  useEffect(() => {
    if (initialCandidates === undefined) {
      setCachedCandidates(readDiscoveryCandidatesCache(window.sessionStorage));
    }
  }, [initialCandidates]);

  useEffect(() => {
    if (data !== undefined) {
      writeDiscoveryCandidatesCache(window.sessionStorage, data);
    }
  }, [data]);

  const saveDecision = useCallback(
    (
      episodeID: number,
      state: TriageDecisionState,
    ): Promise<TriageDecisionResponse> => {
      const existing = decisionRequestsRef.current.get(episodeID);
      if (existing?.state === state) return existing.promise;

      const request = (async () => {
        if (existing) {
          await existing.promise.catch(() => undefined);
        }
        let response;
        if (state === "shortlisted") {
          response = await apiClient.put<{
            success: boolean;
            data: DiscoveryConsumptionResponse;
          }>(`/api/v1/consumption/episodes/${episodeID}/queue`, {
            queue_state: "inbox",
          });
        } else if (state === "discarded") {
          response = await apiClient.put<{
            success: boolean;
            data: DiscoveryConsumptionResponse;
          }>(`/api/v1/consumption/episodes/${episodeID}/dismissed`, {
            dismissed: true,
          });
        } else {
          const candidateState =
            consumptionOverrides[episodeID] ??
            data?.find((candidate) => candidate.episode_id === episodeID);
          const reportState = [
            ...(reportsData?.today ?? []),
            ...(reportsData?.history ?? []),
          ]
            .flatMap((report) => report.episodes)
            .find((episode) => episode.episode_id === episodeID);
          const restoringDismissed =
            existing?.state !== "shortlisted" &&
            Boolean(candidateState?.dismissed_at ?? reportState?.dismissed_at);
          response = restoringDismissed
            ? await apiClient.put<{
                success: boolean;
                data: DiscoveryConsumptionResponse;
              }>(`/api/v1/consumption/episodes/${episodeID}/dismissed`, {
                dismissed: false,
              })
            : await apiClient.delete<{
                success: boolean;
                data: DiscoveryConsumptionResponse;
              }>(`/api/v1/consumption/episodes/${episodeID}/queue`);
        }
        const consumption = response.data.data;
        void revalidateConsumptionSummary();
        setConsumptionOverrides((current) => ({
          ...current,
          [episodeID]: consumption,
        }));
        await mutate(
          (current) =>
            current?.map((candidate) =>
              candidate.episode_id === episodeID
                ? applyConsumptionToCandidate(candidate, consumption)
                : candidate,
            ),
          { revalidate: true },
        );
        await mutateReports(
          (current) => applyConsumptionToReports(current, consumption),
          { revalidate: true },
        );
        const decision: TriageDecisionResponse = {
          state: legacyStateFromConsumption(consumption),
          decision_updated_at:
            consumption.queue_updated_at ??
            consumption.dismissed_at ??
            new Date().toISOString(),
        };
        return decision;
      })();

      decisionRequestsRef.current.set(episodeID, { state, promise: request });
      const cleanup = () => {
        if (decisionRequestsRef.current.get(episodeID)?.promise === request) {
          decisionRequestsRef.current.delete(episodeID);
        }
      };
      void request.then(cleanup, cleanup);
      return request;
    },
    [consumptionOverrides, data, mutate, mutateReports, reportsData],
  );

  const markRead = useCallback(
    async (episodeID: number) => {
      const response = await apiClient.post<{
        success: boolean;
        data: DiscoveryConsumptionResponse;
      }>(`/api/v1/consumption/episodes/${episodeID}/read`);
      const consumption = response.data.data;
      setConsumptionOverrides((current) => ({
        ...current,
        [episodeID]: consumption,
      }));
      await mutate(
        (current) =>
          current?.map((candidate) =>
            candidate.episode_id === episodeID
              ? applyConsumptionToCandidate(candidate, consumption)
              : candidate,
          ),
        { revalidate: false },
      );
      return consumption;
    },
    [mutate],
  );

  const applyQueueChange = useCallback(
    async (item: ConsumptionItem) => {
      const consumption: DiscoveryConsumptionResponse = {
        episode_id: item.episode_id,
        queue_state: item.queue_state,
        dismissed_at: item.dismissed_at,
        queue_updated_at: item.queue_updated_at,
        in_progress_at: item.in_progress_at,
        read_at: item.read_at,
      };
      setConsumptionOverrides((current) => ({
        ...current,
        [item.episode_id]: consumption,
      }));
      await Promise.all([
        mutate(
          (current) =>
            current?.map((candidate) =>
              candidate.episode_id === item.episode_id
                ? applyConsumptionToCandidate(candidate, consumption)
                : candidate,
            ),
          { revalidate: false },
        ),
        mutateReports(
          (current) => applyConsumptionToReports(current, consumption),
          { revalidate: false },
        ),
      ]);
    },
    [mutate, mutateReports],
  );

  const displayCandidates = useMemo(() => {
    const base = data ?? cachedCandidates;
    if (!base) return undefined;
    if (Object.keys(consumptionOverrides).length === 0) return base;
    return base.map((candidate) => {
      const override = consumptionOverrides[candidate.episode_id];
      return override
        ? applyConsumptionToCandidate(candidate, override)
        : candidate;
    });
  }, [data, cachedCandidates, consumptionOverrides]);

  const decisionOverrides = useMemo(
    () =>
      Object.fromEntries(
        Object.entries(consumptionOverrides).map(([episodeID, state]) => [
          Number(episodeID),
          legacyStateFromConsumption(state),
        ]),
      ) as Record<number, TriageDecisionState>,
    [consumptionOverrides],
  );

  const hasCandidates = displayCandidates !== undefined;
  const isUsingCachedCandidates =
    data === undefined && cachedCandidates !== undefined;
  const retry = () => {
    void mutate();
  };
  const retryReports = () => {
    void mutateReports();
  };
  const refreshMessage = isValidating
    ? isUsingCachedCandidates
      ? "正在后台更新，当前显示上次加载结果…"
      : "正在后台更新最近内容…"
    : error
      ? "最近更新暂时无法刷新，当前显示上次加载结果。"
      : "";

  const todayReports = reportsData?.today ?? [];
  const historyReports = reportsData?.history ?? [];
  const reportsFailed = Boolean(reportsError && !reportsValidating);
  const reportsLoading = !reportsData && reportsValidating;
  const showReportWorkbench =
    reportsLoading ||
    reportsFailed ||
    todayReports.length > 0 ||
    historyReports.length > 0;
  const reportContent = showReportWorkbench ? (
    <WorkflowReportWorkbench
      todayReports={todayReports}
      historyReports={historyReports}
      timezone={reportsData?.timezone}
      onDecision={saveDecision}
      decisionOverrides={decisionOverrides}
      consumptionOverrides={consumptionOverrides}
      failed={reportsFailed}
      loading={reportsLoading}
      onRetry={retryReports}
    />
  ) : null;
  const focusContent = (
    <DiscoveryFocusSummary onQueueChange={applyQueueChange} />
  );
  const noticeContent = refreshMessage ? (
    <div
      className={`discovery-refresh-notice ${
        error && !isValidating ? "is-stale" : ""
      }`}
    >
      <span role="status" aria-live="polite">
        {refreshMessage}
      </span>
      {error && !isValidating && (
        <button type="button" onClick={retry}>
          重新尝试
        </button>
      )}
    </div>
  ) : null;

  return (
    <SimplePageLayout maxWidth={false} className="discovery-page-shell">
      {!hasCandidates ? (
        <DiscoveryPageSkeleton
          failed={Boolean(error && !isValidating)}
          onRetry={retry}
          reportContent={reportContent}
          focusContent={focusContent}
        />
      ) : (
        <DiscoveryDesk
          candidates={displayCandidates}
          reportContent={reportContent}
          focusContent={focusContent}
          noticeContent={noticeContent}
          onDecision={saveDecision}
          onRead={markRead}
          onLoadCandidateDetails={fetchDiscoveryCandidateDetails}
        />
      )}
    </SimplePageLayout>
  );
}
