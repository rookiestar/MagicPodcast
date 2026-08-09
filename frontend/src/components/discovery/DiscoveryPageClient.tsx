"use client";

import { useEffect, useState } from "react";
import useSWR from "swr";
import DiscoveryDesk from "@/components/discovery/DiscoveryDesk";
import { SimplePageLayout } from "@/components/layout/PageLayout";
import {
  DISCOVERY_CANDIDATES_PATH,
  fetchDiscoveryCandidatesWithRetry,
  readDiscoveryCandidatesCache,
  writeDiscoveryCandidatesCache,
} from "@/lib/discoveryCandidates";
import { apiClient } from "@/lib/fetcher";
import type {
  DiscoveryCandidate,
  TriageDecisionResponse,
  TriageDecisionState,
} from "@/types/discovery";

interface DiscoveryPageClientProps {
  initialCandidates?: DiscoveryCandidate[];
}

const SKELETON_ROWS = [0, 1, 2, 3, 4];
const discoveryCandidatesFetcher = () =>
  fetchDiscoveryCandidatesWithRetry();

function DiscoveryPageSkeleton({
  failed,
  onRetry,
}: {
  failed: boolean;
  onRetry: () => void;
}) {
  return (
    <main
      className="discovery-desk discovery-page-skeleton"
      aria-label="正在读取个人库最近更新"
      aria-busy={!failed}
    >
      <p className="sr-only" aria-live="polite">
        {failed
          ? "最近更新暂时无法读取，可以重新尝试。"
          : "正在后台读取个人库最近更新。"}
      </p>

      <section className="discovery-loading-header" aria-hidden="true">
        <span className="discovery-skeleton-block discovery-skeleton-heading" />
        <span className="discovery-skeleton-block discovery-skeleton-count" />
      </section>

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

      <div className="discovery-loading-workspace" aria-hidden="true">
        <section className="discovery-loading-list">
          <div className="discovery-skeleton-block discovery-skeleton-section" />
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
        </section>
        <aside className="discovery-loading-preview">
          <span className="discovery-skeleton-block discovery-skeleton-section" />
          <div className="discovery-loading-preview-title">
            <span className="discovery-skeleton-block discovery-skeleton-cover" />
            <span className="discovery-loading-copy">
              <span className="discovery-skeleton-block is-short" />
              <span className="discovery-skeleton-block is-title" />
              <span className="discovery-skeleton-block is-short" />
            </span>
          </div>
          <span className="discovery-skeleton-block is-body" />
          <span className="discovery-skeleton-block is-body" />
          <span className="discovery-skeleton-block is-body is-narrow" />
        </aside>
      </div>
    </main>
  );
}

export default function DiscoveryPageClient({
  initialCandidates,
}: DiscoveryPageClientProps) {
  const [cachedCandidates, setCachedCandidates] = useState<
    DiscoveryCandidate[] | undefined
  >();
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

  const displayCandidates = data ?? cachedCandidates;
  const hasCandidates = displayCandidates !== undefined;
  const isUsingCachedCandidates =
    data === undefined && cachedCandidates !== undefined;
  const retry = () => {
    void mutate();
  };
  const refreshMessage = isValidating
    ? isUsingCachedCandidates
      ? "正在后台更新，当前显示上次加载结果…"
      : "正在后台更新最近内容…"
    : error
      ? "最近更新暂时无法刷新，当前显示上次加载结果。"
      : "";

  return (
    <SimplePageLayout maxWidth={false} className="discovery-page-shell">
      {!hasCandidates ? (
        <DiscoveryPageSkeleton
          failed={Boolean(error && !isValidating)}
          onRetry={retry}
        />
      ) : (
        <>
          {refreshMessage && (
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
          )}
          <DiscoveryDesk
            candidates={displayCandidates}
            onDecision={saveDecision}
          />
        </>
      )}
    </SimplePageLayout>
  );
}
