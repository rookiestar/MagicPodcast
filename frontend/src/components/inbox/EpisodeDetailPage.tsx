"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import PageLayout from "@/components/layout/PageLayout";
import {
  consumptionApi,
  getConsumptionErrorDetails,
  requiresFocusConfirmation,
} from "@/lib/api/consumption";
import {
  episodeHref,
  attachEpisodeOrigin,
  closeTo,
  readEpisodeOrigin,
  episodeOriginIsPrevious,
  queueEpisodeReturn,
  navigate,
  normalizeEpisodeQuery,
  parseEpisodeRoute,
  singleParam,
  useLocationHref,
} from "@/lib/navigation";
import type { ConsumptionItem, ConsumptionQueue } from "@/types/consumption";
import ConsumptionDetailPanel from "./ConsumptionDetailPanel";
import FocusLimitDialog from "./FocusLimitDialog";
import styles from "./InboxPage.module.css";

export default function EpisodeDetailPage() {
  const router = useRouter();
  const href = useLocationHref();
  const route = parseEpisodeRoute(href);
  useEffect(() => attachEpisodeOrigin(route.id), [route.id]);
  useEffect(() => { if (href.startsWith("/episodes/")) normalizeEpisodeQuery(); }, [href]);
  const [item, setItem] = useState<ConsumptionItem | null>(null);
  const [error, setError] = useState("");
  const [retry, setRetry] = useState(0);
  const [busy, setBusy] = useState(false);
  const [moveError, setMoveError] = useState("");
  const [focus, setFocus] = useState<{ count: number; limit: number } | null>(
    null,
  );
  useEffect(() => {
    if (!href) return;
    if (!route.id) {
      setError("单集地址无效。");
      return;
    }
    let active = true;
    setError("");
    consumptionApi
      .getItem(route.id)
      .then((value) => {
        if (active) setItem(value);
      })
      .catch((reason: unknown) => {
        if (active)
          setError(
            getConsumptionErrorDetails(reason).status === 404
              ? "该单集不存在。"
              : "单集读取失败，请重试。",
          );
      });
    return () => {
      active = false;
    };
    // Queries only change the view; preserve unsaved inputs and loaded content.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [route.id, retry, Boolean(href)]);

  const close = () => {
    const from = singleParam(
      new URLSearchParams(window.location.search),
      "from",
    );
    const origin = readEpisodeOrigin(route.id);
    const target = origin?.href ?? (
      from === "discovery"
        ? "/discovery"
        : from === "history"
          ? "/inbox/history"
          : from === "podcast"
            ? "/podcasts"
            : from === "search"
              ? "/search"
              : "/inbox");
    // Approval is performed before Next unmounts the editor.
    if (origin && episodeOriginIsPrevious(origin)) {
      if (closeTo(target)) queueEpisodeReturn(origin);
    } else if (navigate(target, { replace: true })) {
      if (origin) queueEpisodeReturn(origin);
      router.replace(target);
    }
  };
  const move = async (
    current: ConsumptionItem,
    target: ConsumptionQueue,
    acknowledge = false,
  ) => {
    setBusy(true);
    setMoveError("");
    try {
      const next = await consumptionApi.setQueue(current.episode_id, target, {
        acknowledgeFocusLimit: acknowledge,
      });
      setItem(next);
      setFocus(null);
      return next;
    } catch (reason) {
      const details = getConsumptionErrorDetails(reason);
      if (requiresFocusConfirmation(reason))
        setFocus({
          count: details.currentCount ?? 0,
          limit: details.focusLimit ?? 7,
        });
      else setMoveError(details.message);
      return undefined;
    } finally {
      setBusy(false);
    }
  };
  return (
    <PageLayout toolbar={false} rootClassName={styles.shell} className={styles.layout} maxWidth={false}>
      <main className={styles.page}>
        <h1>单集详情</h1>
        {error ? (
          <div role="alert">
            <p>{error}</p>
            <button onClick={() => setRetry((value) => value + 1)}>重试</button>
            <button onClick={close}>返回列表</button>
          </div>
        ) : item?.episode_id !== route.id ? (
          <p role="status">正在读取单集…</p>
        ) : null}
      </main>
      {item && item.episode_id === route.id && !error && (
        <ConsumptionDetailPanel
          item={item}
          routeState={route}
          isQueueBusy={busy}
          queueMoveFailure={moveError}
          onClose={close}
          onItemChange={setItem}
          onMove={move}
          onOpenSourceEpisode={(id) => {
            navigate(episodeHref(id, { tab: "transcript" }));
          }}
        />
      )}
      {item && focus && (
        <FocusLimitDialog
          item={item}
          currentCount={focus.count}
          limit={focus.limit}
          isSaving={busy}
          onCancel={() => setFocus(null)}
          onConfirm={() => {
            void move(item, "focus", true);
          }}
        />
      )}
    </PageLayout>
  );
}
