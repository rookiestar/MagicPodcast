"use client";

import { useId, useState } from "react";
import { IconPlus, IconTargetArrow, IconX } from "@tabler/icons-react";
import useSWR from "swr";
import {
  consumptionApi,
  getConsumptionErrorDetails,
  requiresFocusConfirmation,
} from "@/lib/api/consumption";
import type {
  ConsumptionItem,
  ConsumptionQueuePayload,
  ConsumptionSummary,
} from "@/types/consumption";

const FOCUS_KEY = "/api/v1/consumption/queues/focus";
const INBOX_KEY = "/api/v1/consumption/queues/inbox";

interface DiscoveryFocusSummaryProps {
  onQueueChange?: (item: ConsumptionItem) => void | Promise<void>;
}

export default function DiscoveryFocusSummary({
  onQueueChange,
}: DiscoveryFocusSummaryProps) {
  const titleId = useId();
  const [selectorOpen, setSelectorOpen] = useState(false);
  const [savingEpisodeId, setSavingEpisodeId] = useState<number | null>(null);
  const [pendingConfirmation, setPendingConfirmation] =
    useState<ConsumptionItem | null>(null);
  const [errorMessage, setErrorMessage] = useState("");

  const {
    data: summary,
    error: summaryError,
    isLoading: summaryLoading,
    mutate: mutateSummary,
  } = useSWR<ConsumptionSummary>(
    "/api/v1/consumption/summary",
    consumptionApi.getSummary,
    {
      revalidateOnFocus: false,
      shouldRetryOnError: false,
    },
  );
  const {
    data: focus,
    error: focusError,
    mutate: mutateFocus,
  } = useSWR<ConsumptionQueuePayload>(
    FOCUS_KEY,
    () => consumptionApi.listQueue("focus"),
    {
      revalidateOnFocus: false,
      shouldRetryOnError: false,
    },
  );
  const {
    data: inbox,
    error: inboxError,
    isLoading: inboxLoading,
    mutate: mutateInbox,
  } = useSWR<ConsumptionQueuePayload>(
    selectorOpen ? INBOX_KEY : null,
    () => consumptionApi.listQueue("inbox"),
    {
      revalidateOnFocus: false,
      shouldRetryOnError: false,
    },
  );

  const focusCount = summary?.counts.focus ?? focus?.items.length ?? 0;
  const focusLimit = summary?.focus_limit ?? 7;
  const focusItems = focus?.items ?? [];
  const visibleFocusItems = focusItems.slice(0, 3);

  const refreshQueues = async (item: ConsumptionItem) => {
    await Promise.all([
      mutateSummary(),
      mutateFocus(),
      mutateInbox(),
      onQueueChange?.(item),
    ]);
  };

  const addToFocus = async (
    item: ConsumptionItem,
    acknowledgeFocusLimit: boolean,
  ) => {
    setErrorMessage("");
    setSavingEpisodeId(item.episode_id);
    try {
      const updated = await consumptionApi.setQueue(item.episode_id, "focus", {
        acknowledgeFocusLimit,
      });
      await refreshQueues(updated);
      setPendingConfirmation(null);
      setSelectorOpen(false);
    } catch (error) {
      if (!acknowledgeFocusLimit && requiresFocusConfirmation(error)) {
        setPendingConfirmation(item);
      } else {
        setErrorMessage(getConsumptionErrorDetails(error).message);
      }
    } finally {
      setSavingEpisodeId(null);
    }
  };

  const requestAdd = (item: ConsumptionItem) => {
    if (focusCount >= focusLimit) {
      setPendingConfirmation(item);
      return;
    }
    void addToFocus(item, false);
  };

  return (
    <>
      <section
        className="discovery-focus-summary"
        aria-label="Focus 快捷摘要"
        aria-busy={summaryLoading}
      >
        <div className="discovery-focus-heading">
          <IconTargetArrow aria-hidden="true" stroke={1.8} />
          <span>
            <strong>Focus</strong>
            <small>
              {focusCount} / {focusLimit}
            </small>
          </span>
        </div>
        <div className="discovery-focus-items">
          {visibleFocusItems.length > 0 ? (
            visibleFocusItems.map((item) => (
              <span key={item.episode_id} title={item.episode_title}>
                {item.episode_title}
              </span>
            ))
          ) : (
            <span>{summaryLoading ? "正在读取…" : "尚未投入内容"}</span>
          )}
          {focusItems.length > visibleFocusItems.length && (
            <span>另有 {focusItems.length - visibleFocusItems.length} 项</span>
          )}
        </div>
        <button
          type="button"
          className="discovery-focus-add"
          aria-label="从 Inbox 添加到 Focus"
          title="从 Inbox 添加到 Focus"
          onClick={() => {
            setErrorMessage("");
            setSelectorOpen(true);
          }}
        >
          <IconPlus aria-hidden="true" stroke={1.8} />
        </button>
        {(summaryError || focusError) && (
          <p className="discovery-focus-error" role="status">
            Focus 摘要暂时无法读取
          </p>
        )}
      </section>

      {selectorOpen && (
        <div className="discovery-focus-selector-overlay">
          <button
            type="button"
            className="discovery-focus-selector-backdrop"
            aria-label="关闭 Focus 添加"
            onClick={() => setSelectorOpen(false)}
          />
          <section
            className="discovery-focus-selector"
            role="dialog"
            aria-modal="true"
            aria-labelledby={titleId}
          >
            <header>
              <div>
                <h2 id={titleId}>从 Inbox 添加</h2>
                <p>仅显示已收集、尚未投入的条目。</p>
              </div>
              <button
                type="button"
                aria-label="关闭"
                onClick={() => setSelectorOpen(false)}
              >
                <IconX aria-hidden="true" />
              </button>
            </header>

            {errorMessage && (
              <p className="discovery-focus-selector-error" role="alert">
                {errorMessage}
              </p>
            )}
            {inboxError ? (
              <div className="discovery-focus-selector-state">
                <p>Inbox 暂时无法读取。</p>
                <button type="button" onClick={() => void mutateInbox()}>
                  重新尝试
                </button>
              </div>
            ) : inboxLoading ? (
              <p className="discovery-focus-selector-state" role="status">
                正在读取 Inbox…
              </p>
            ) : (inbox?.items.length ?? 0) === 0 ? (
              <p className="discovery-focus-selector-state">
                Inbox 中暂无可添加内容。
              </p>
            ) : (
              <ul className="discovery-focus-selector-list">
                {inbox?.items.map((item) => (
                  <li key={item.episode_id}>
                    <span>
                      <small>{item.podcast_title}</small>
                      <strong>{item.episode_title}</strong>
                    </span>
                    <button
                      type="button"
                      aria-label={`将 ${item.episode_title} 添加到 Focus`}
                      title="添加到 Focus"
                      disabled={savingEpisodeId !== null}
                      onClick={() => requestAdd(item)}
                    >
                      <IconPlus aria-hidden="true" />
                    </button>
                  </li>
                ))}
              </ul>
            )}

            {pendingConfirmation && (
              <div
                className="discovery-focus-confirmation"
                role="alertdialog"
                aria-label="确认超过 Focus 软上限"
              >
                <p>Focus 已有 {focusCount} 项。仍加入不会自动移出任何内容。</p>
                <div>
                  <button
                    type="button"
                    onClick={() => setPendingConfirmation(null)}
                  >
                    先不加入
                  </button>
                  <button
                    type="button"
                    disabled={savingEpisodeId !== null}
                    onClick={() => void addToFocus(pendingConfirmation, true)}
                  >
                    仍加入 Focus
                  </button>
                </div>
              </div>
            )}
          </section>
        </div>
      )}
    </>
  );
}
