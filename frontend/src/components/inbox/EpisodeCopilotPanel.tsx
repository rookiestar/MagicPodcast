"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import {
  IconPlayerStop,
  IconRefresh,
  IconSend,
  IconSparkles,
  IconX,
} from "@tabler/icons-react";
import MarkdownViewer from "@/components/workflows/MarkdownViewer";
import {
  episodeCopilotApi,
  isEpisodeCopilotCancellation,
} from "@/lib/api/episodeCopilot";
import { getErrorMessage } from "@/lib/errorMessage";
import type { ConsumptionItem } from "@/types/consumption";
import type {
  EpisodeCopilotContextScope,
  EpisodeCopilotQuestion,
  EpisodeCopilotSelectionSource,
  EpisodeCopilotStreamEvent,
} from "@/types/episodeCopilot";
import styles from "./InboxPage.module.css";

interface EpisodeCopilotPanelProps {
  item: ConsumptionItem;
  showHeading?: boolean;
}

interface CapturedSelection {
  text: string;
  source: EpisodeCopilotSelectionSource;
}

type RequestPhase =
  | "idle"
  | "waiting"
  | "streaming"
  | "completed"
  | "cancelled"
  | "failed";

const slowResponseThresholdMS = 2500;
const maxSelectionCharacters = 12_000;

function selectionLabel(source: EpisodeCopilotSelectionSource) {
  return source === "transcript" ? "逐字稿" : "Show Notes";
}

export default function EpisodeCopilotPanel({
  item,
  showHeading = true,
}: EpisodeCopilotPanelProps) {
  const [scope, setScope] = useState<EpisodeCopilotContextScope | null>(null);
  const [scopeError, setScopeError] = useState<string | null>(null);
  const [isLoadingScope, setIsLoadingScope] = useState(true);
  const [question, setQuestion] = useState("");
  const [selection, setSelection] = useState<CapturedSelection | null>(null);
  const [includePrivateNote, setIncludePrivateNote] = useState(false);
  const [phase, setPhase] = useState<RequestPhase>("idle");
  const [statusMessage, setStatusMessage] = useState("");
  const [answer, setAnswer] = useState("");
  const [requestError, setRequestError] = useState<string | null>(null);
  const [isSlow, setIsSlow] = useState(false);
  const [metrics, setMetrics] = useState<{
    firstContentMS: number;
    totalMS: number;
  } | null>(null);
  const activeRequest = useRef<AbortController | null>(null);
  const retryRequest = useRef<EpisodeCopilotQuestion | null>(null);

  const loadScope = useCallback(async () => {
    setIsLoadingScope(true);
    setScopeError(null);
    try {
      const nextScope = await episodeCopilotApi.getContext(item.episode_id);
      setScope(nextScope);
      if (!nextScope.private_note_available) setIncludePrivateNote(false);
    } catch (error) {
      setScope(null);
      setScopeError(
        `助手上下文暂时不可用，单集阅读不受影响：${getErrorMessage(error)}`,
      );
    } finally {
      setIsLoadingScope(false);
    }
  }, [item.episode_id]);

  useEffect(() => {
    activeRequest.current?.abort();
    activeRequest.current = null;
    setScope(null);
    setQuestion("");
    setSelection(null);
    setIncludePrivateNote(false);
    setPhase("idle");
    setStatusMessage("");
    setAnswer("");
    setRequestError(null);
    setMetrics(null);
    setIsSlow(false);
    retryRequest.current = null;
    void loadScope();
    return () => activeRequest.current?.abort();
  }, [loadScope]);

  useEffect(() => {
    const captureSelection = () => {
      const browserSelection = window.getSelection();
      if (
        !browserSelection ||
        browserSelection.isCollapsed ||
        browserSelection.rangeCount === 0
      ) {
        return;
      }
      const text = Array.from(browserSelection.toString().trim())
        .slice(0, maxSelectionCharacters)
        .join("");
      if (!text) return;
      const range = browserSelection.getRangeAt(0);
      const common = range.commonAncestorContainer;
      const element =
        common instanceof Element ? common : common.parentElement;
      const sourceElement = element?.closest<HTMLElement>(
        "[data-copilot-source]",
      );
      if (
        !sourceElement ||
        sourceElement.dataset.copilotEpisodeId !== String(item.episode_id)
      ) {
        return;
      }
      const source = sourceElement.dataset
        .copilotSource as EpisodeCopilotSelectionSource;
      if (source !== "show_notes" && source !== "transcript") return;
      setSelection({ text, source });
    };
    document.addEventListener("selectionchange", captureSelection);
    return () =>
      document.removeEventListener("selectionchange", captureSelection);
  }, [item.episode_id]);

  useEffect(() => {
    if (phase !== "waiting") {
      setIsSlow(false);
      return;
    }
    const timer = window.setTimeout(
      () => setIsSlow(true),
      slowResponseThresholdMS,
    );
    return () => window.clearTimeout(timer);
  }, [phase]);

  const handleEvent = (
    event: EpisodeCopilotStreamEvent,
    replaceAnswer: { current: boolean },
  ) => {
    if (event.type === "context" || event.type === "status") {
      setStatusMessage(event.message || "正在处理…");
      return;
    }
    if (event.type === "answer_delta") {
      setPhase("streaming");
      setIsSlow(false);
      setStatusMessage("正在继续生成回答与来源…");
      if (replaceAnswer.current) {
        replaceAnswer.current = false;
        setAnswer(event.message || "");
      } else {
        setAnswer((current) => current + (event.message || ""));
      }
      return;
    }
    if (event.type === "error") {
      setPhase("failed");
      setIsSlow(false);
      setStatusMessage("");
      setRequestError(event.message || "助手回答失败，请重试");
      return;
    }
    if (event.type === "complete") {
      setPhase("completed");
      setStatusMessage("回答完成");
      setSelection(null);
      setMetrics({
        firstContentMS: event.first_content_ms ?? 0,
        totalMS: event.total_ms ?? 0,
      });
    }
  };

  const ask = async (
    preserveAnswer: boolean,
    requestToRetry?: EpisodeCopilotQuestion,
  ) => {
    const normalizedQuestion =
      requestToRetry?.question ?? question.trim();
    if (!normalizedQuestion || !scope || activeRequest.current) return;
    const controller = new AbortController();
    activeRequest.current = controller;
    const request: EpisodeCopilotQuestion =
      requestToRetry ?? {
        question: normalizedQuestion,
        selection: selection?.text ?? "",
        selection_source: selection?.source ?? "",
        include_private_note:
          includePrivateNote && scope.private_note_available,
      };
    if (!requestToRetry) {
      retryRequest.current = request;
      setIncludePrivateNote(false);
    }
    const replaceAnswer = { current: preserveAnswer };
    if (!preserveAnswer) setAnswer("");
    setPhase("waiting");
    setStatusMessage("正在核对当前单集与公开资料…");
    setRequestError(null);
    setMetrics(null);
    try {
      await episodeCopilotApi.ask(
        item.episode_id,
        request,
        (event) => handleEvent(event, replaceAnswer),
        controller.signal,
      );
    } catch (error) {
      if (isEpisodeCopilotCancellation(error)) {
        setPhase("cancelled");
        setStatusMessage("已取消；问题、选区和已有答案已保留。");
      } else {
        setPhase("failed");
        setIsSlow(false);
        setStatusMessage("");
        setRequestError(
          `${getErrorMessage(error)}；问题、选区和已有答案已保留。`,
        );
      }
    } finally {
      if (activeRequest.current === controller) {
        activeRequest.current = null;
      }
    }
  };

  const isActive = phase === "waiting" || phase === "streaming";
  const canAsk =
    Boolean(scope) && question.trim().length > 0 && !isActive;

  return (
    <section
      className={styles.copilotSection}
      aria-labelledby={showHeading ? "episode-copilot-title" : undefined}
    >
      {showHeading && (
        <div className={styles.detailSectionHeading}>
          <div>
            <span className={styles.detailKicker}>EPISODE COPILOT</span>
            <h3 id="episode-copilot-title">单集助手</h3>
          </div>
          <IconSparkles size={22} stroke={1.6} aria-hidden="true" />
        </div>
      )}

      <p className={styles.copilotDataFlow}>
        问题与当前单集上下文会交给 Mac mini 上的本地 Codex Runtime。助手只读，不会修改备注、队列、产物或知识中心。
      </p>

      {isLoadingScope && <span role="status">正在核对可用上下文…</span>}
      {scopeError && (
        <div className={styles.inlineError} role="alert">
          <span>{scopeError}</span>
          <button
            type="button"
            className={styles.iconButton}
            onClick={() => void loadScope()}
            aria-label="重试读取助手上下文"
          >
            <IconRefresh size={18} stroke={1.8} aria-hidden="true" />
          </button>
        </div>
      )}

      {scope && (
        <>
          <div className={styles.copilotScope} aria-label="助手可用上下文">
            <span data-available={scope.show_notes_available}>
              Show Notes
            </span>
            <span data-available={scope.transcript_available}>
              {scope.transcript_available ? "逐字稿可用" : "无逐字稿"}
            </span>
            <span data-available={scope.private_note_available}>
              {scope.private_note_available ? "私有备注可选" : "无私有备注"}
            </span>
          </div>
          {!scope.transcript_available && (
            <p className={styles.copilotDegraded}>
              当前无成功逐字稿，将明确降级为 Show Notes。
            </p>
          )}

          {selection && (
            <div className={styles.copilotSelection}>
              <div>
                <strong>已选 {selectionLabel(selection.source)}</strong>
                <span>{selection.text}</span>
              </div>
              <button
                type="button"
                className={styles.iconButton}
                onClick={() => setSelection(null)}
                aria-label="清除助手选区"
              >
                <IconX size={17} stroke={1.8} aria-hidden="true" />
              </button>
            </div>
          )}

          <label className={styles.copilotComposer}>
            <span>向单集助手提问</span>
            <textarea
              aria-label="向单集助手提问"
              value={question}
              onChange={(event) => setQuestion(event.target.value)}
              placeholder={
                selection
                  ? "解释、核对或寻找与这段内容相关的公开资源…"
                  : "围绕当前单集提问，或先在 Show Notes / 逐字稿中划词…"
              }
              rows={3}
              maxLength={2000}
            />
          </label>

          {scope.private_note_available && (
            <label className={styles.copilotPrivateNote}>
              <input
                type="checkbox"
                checked={includePrivateNote}
                disabled={isActive}
                onChange={(event) =>
                  setIncludePrivateNote(event.target.checked)
                }
              />
              <span>
                本次包含我的私有备注
                <small>仅进入关闭全部工具的最终回答，不进入公开搜索。</small>
              </span>
            </label>
          )}

          <div className={styles.copilotActions}>
            {!isActive && (
              <button
                type="button"
                className={styles.primaryCommand}
                disabled={!canAsk}
                onClick={() => void ask(false)}
              >
                <IconSend size={18} stroke={1.8} aria-hidden="true" />
                提问
              </button>
            )}
            {isActive && (
              <button
                type="button"
                className={styles.secondaryCommand}
                onClick={() => activeRequest.current?.abort()}
              >
                <IconPlayerStop size={18} stroke={1.8} aria-hidden="true" />
                取消
              </button>
            )}
            {(phase === "failed" || phase === "cancelled") && (
              <button
                type="button"
                className={styles.secondaryCommand}
                disabled={!retryRequest.current}
                onClick={() =>
                  void ask(true, retryRequest.current ?? undefined)
                }
              >
                <IconRefresh size={18} stroke={1.8} aria-hidden="true" />
                重试
              </button>
            )}
            {(statusMessage || isSlow) && (
              <span className={styles.copilotStatus} role="status">
                {isSlow
                  ? "响应较慢；单集仍可阅读，可随时取消。"
                  : statusMessage}
              </span>
            )}
          </div>

          {requestError && (
            <div className={styles.inlineError} role="alert">
              <span>{requestError}</span>
            </div>
          )}

          {answer && (
            <div className={styles.copilotAnswer} aria-live="polite">
              <MarkdownViewer content={answer} density="reading" />
              {metrics && (
                <span className={styles.copilotMetrics}>
                  首字 {metrics.firstContentMS}ms · 完成 {metrics.totalMS}ms
                </span>
              )}
            </div>
          )}
        </>
      )}
    </section>
  );
}
