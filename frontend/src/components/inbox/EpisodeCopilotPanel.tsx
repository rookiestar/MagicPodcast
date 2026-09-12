"use client";

import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type KeyboardEvent as ReactKeyboardEvent,
} from "react";
import {
  IconChevronDown,
  IconCopy,
  IconPlayerStop,
  IconPlus,
  IconRefresh,
  IconScale,
  IconSend,
  IconShieldCheck,
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
  EpisodeCopilotProfileID,
  EpisodeCopilotQuestion,
  EpisodeCopilotSelectionSource,
  EpisodeCopilotStreamEvent,
  EpisodePersonCandidate,
} from "@/types/episodeCopilot";
import EpisodeCopilotActivityCard from "./EpisodeCopilotActivityCard";
import {
  applyAnswerDelta,
  applyStreamEvent,
  cancelRun,
  createRunState,
  type CopilotRunState,
} from "./episodeCopilotRun";
import {
  EpisodeCopilotContextMenu,
  EpisodeCopilotProfileMenu,
  orderedProfiles,
  profileDisplayName,
} from "./EpisodeCopilotMenus";
import { useMenuPopover } from "./useMenuPopover";
import {
  filterPeople,
  jumpToLibrarySource,
  mentionDraft,
  parseLibrarySources,
  replaceMention,
  roleLabel,
} from "./episodeCopilotMention";
import styles from "./InboxPage.module.css";

interface EpisodeCopilotPanelProps {
  item: ConsumptionItem;
  selectedProfileID?: EpisodeCopilotProfileID | null;
  onSelectedProfileIDChange?: (profileID: EpisodeCopilotProfileID) => void;
  rejectedProfileIDs?: ReadonlySet<EpisodeCopilotProfileID>;
  onRejectedProfileID?: (profileID: EpisodeCopilotProfileID) => void;
  onManagePeople?: () => void;
  onOpenSourceEpisode?: (episodeId: number) => void | Promise<void>;
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
// Fixed local product copy; never a per-episode model call.
const quickQuestions = [
  "总结这期节目的核心观点",
  "解释这期内容的关键转折",
  "列出提到的工具与人物",
] as const;

function selectionLabel(source: EpisodeCopilotSelectionSource) {
  return source === "transcript" ? "逐字稿" : "Show Notes";
}

function addRejectedProfileID(
  current: ReadonlySet<EpisodeCopilotProfileID>,
  profileID: EpisodeCopilotProfileID,
) {
  if (current.has(profileID)) return current;
  return new Set([...current, profileID]);
}

function resolveProfileID(scope: EpisodeCopilotContextScope) {
  return scope.default_profile_id;
}

function welcomeContextMessage(scope: EpisodeCopilotContextScope) {
  if (scope.show_notes_available && scope.transcript_available) {
    return "我已读取本集 Show Notes 和逐字稿，围绕这一集提问即可。";
  }
  if (scope.show_notes_available) {
    return "我已读取本集 Show Notes，围绕这一集提问即可。";
  }
  if (scope.transcript_available) {
    return "我已读取本集逐字稿，围绕这一集提问即可。";
  }
  return "当前单集暂无可用 Show Notes 或逐字稿，仍可围绕标题与公开资料提问。";
}

export default function EpisodeCopilotPanel({
  item,
  selectedProfileID: controlledProfileID,
  onSelectedProfileIDChange,
  rejectedProfileIDs: controlledRejectedProfileIDs,
  onRejectedProfileID,
  onOpenSourceEpisode,
  onManagePeople,
}: EpisodeCopilotPanelProps) {
  const [scope, setScope] = useState<EpisodeCopilotContextScope | null>(null);
  const [scopeError, setScopeError] = useState<string | null>(null);
  const [isLoadingScope, setIsLoadingScope] = useState(true);
  const [question, setQuestion] = useState("");
  // The submitted question becomes the user message in the conversation;
  // an empty value keeps the welcome + quick questions view visible.
  const [askedQuestion, setAskedQuestion] = useState("");
  const [selection, setSelection] = useState<CapturedSelection | null>(null);
  const [includePrivateNote, setIncludePrivateNote] = useState(false);
  // null keeps the scope's balanced default; a page refresh resets to null.
  const [localSelectedProfileID, setLocalSelectedProfileID] =
    useState<EpisodeCopilotProfileID | null>(null);
  const [phase, setPhase] = useState<RequestPhase>("idle");
  const [statusMessage, setStatusMessage] = useState("");
  const [answer, setAnswer] = useState("");
  const [requestError, setRequestError] = useState<string | null>(null);
  const [requestCanRetry, setRequestCanRetry] = useState(false);
  const [localRejectedProfileIDs, setLocalRejectedProfileIDs] = useState<
    ReadonlySet<EpisodeCopilotProfileID>
  >(new Set());
  const [isSlow, setIsSlow] = useState(false);
  const [metrics, setMetrics] = useState<{
    firstContentMS: number;
    totalMS: number;
    profileID: string;
  } | null>(null);
  // One activity-card run per question; created on submit before any
  // backend event so the request is immediately visible.
  const [run, setRun] = useState<CopilotRunState | null>(null);
  const [nowTick, setNowTick] = useState(() => Date.now());
  const [submissionAnnouncement, setSubmissionAnnouncement] = useState("");
  const [copyState, setCopyState] = useState<"idle" | "copied" | "failed">(
    "idle",
  );
  const [targetPerson, setTargetPerson] =
    useState<EpisodePersonCandidate | null>(null);
  const targetPersonRef = useRef(targetPerson);
 targetPersonRef.current = targetPerson;
 const [mentionOpen, setMentionOpen] = useState(false);
  const [mentionIndex, setMentionIndex] = useState(0);
 const [personSelectionInvalid, setPersonSelectionInvalid] = useState(false);
  const profileMenu = useMenuPopover();
  const contextMenu = useMenuPopover();
  const dismissProfileMenu = profileMenu.dismissMenu;
  const dismissContextMenu = contextMenu.dismissMenu;
  const activeRequest = useRef<AbortController | null>(null);
  const retryRequest = useRef<EpisodeCopilotQuestion | null>(null);
  const terminalStreamErrorHandled = useRef(false);
  const copyTimer = useRef<number | null>(null);
  const conversationRef = useRef<HTMLDivElement | null>(null);
  const stickToBottom = useRef(true);
  const selectedProfileID = controlledProfileID ?? localSelectedProfileID;
  const rejectedProfileIDs =
    controlledRejectedProfileIDs ?? localRejectedProfileIDs;

  const selectProfile = (profileID: EpisodeCopilotProfileID) => {
    setLocalSelectedProfileID(profileID);
    onSelectedProfileIDChange?.(profileID);
  };

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
    let alive = true;
    const refresh = async (event: Event) => {
      if (event instanceof CustomEvent && event.detail !== item.episode_id) return;
      try {
        const next = await episodeCopilotApi.getContext(item.episode_id);
        if (!alive) return;
        setScope(next);
        const selected = targetPersonRef.current;
        if (selected) {
          const person = next.people?.find(p => p.id===selected.id && p.status==="confirmed");
          setTargetPerson(person ?? null);
          if (!person) setPersonSelectionInvalid(true);
        }
      } catch { if(alive) setScopeError("人物资料刷新失败，请重试。"); }
    };
    window.addEventListener("episode-people-changed",refresh);
    window.addEventListener("focus",refresh);
    return () => {alive=false; window.removeEventListener("episode-people-changed",refresh); window.removeEventListener("focus",refresh);};
  }, [item.episode_id]);

  useEffect(() => {
    activeRequest.current?.abort();
    activeRequest.current = null;
    setScope(null);
    setQuestion("");
    setAskedQuestion("");
    setSelection(null);
    setIncludePrivateNote(false);
    setPhase("idle");
    setStatusMessage("");
    setAnswer("");
    setRequestError(null);
    setRequestCanRetry(false);
    setMetrics(null);
    setIsSlow(false);
    setRun(null);
    setSubmissionAnnouncement("");
    setCopyState("idle");
 setPersonSelectionInvalid(false);
    setTargetPerson(null);
    setMentionOpen(false);
    dismissProfileMenu();
    dismissContextMenu();
    retryRequest.current = null;
    terminalStreamErrorHandled.current = false;
    void loadScope();
    return () => { activeRequest.current?.abort();  };
  }, [dismissContextMenu, dismissProfileMenu, loadScope]);

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

  useEffect(
    () => () => {
      if (copyTimer.current !== null) {
        window.clearTimeout(copyTimer.current);
      }
    },
    [],
  );

  // Keep the newest message visible while the reader stays near the bottom;
  // scrolling up to re-read keeps the position stable.
  const handleConversationScroll = () => {
    const element = conversationRef.current;
    if (!element) return;
    stickToBottom.current =
      element.scrollHeight - element.scrollTop - element.clientHeight < 80;
  };

  useEffect(() => {
    const element = conversationRef.current;
    if (element && stickToBottom.current) {
      element.scrollTop = element.scrollHeight;
    }
  }, [answer, askedQuestion, run, statusMessage, phase]);

  const handleEvent = (
    event: EpisodeCopilotStreamEvent,
    replaceAnswer: { current: boolean },
    requestProfileID: EpisodeCopilotProfileID,
  ) => {
    if (event.type === "context" || event.type === "status") {
      setStatusMessage(event.message || "正在处理…");
      setRun((current) =>
        current ? applyStreamEvent(current, event, Date.now()) : current,
      );
      return;
    }
    if (event.type === "answer_delta") {
      setPhase("streaming");
      setIsSlow(false);
      setStatusMessage("正在继续生成回答与来源…");
      setRun((current) =>
        current ? applyAnswerDelta(current, Date.now()) : current,
      );
      if (replaceAnswer.current) {
        replaceAnswer.current = false;
        setAnswer(event.message || "");
      } else {
        setAnswer((current) => current + (event.message || ""));
      }
      return;
    }
    if (event.type === "error") {
      terminalStreamErrorHandled.current = true;
      setPhase("failed");
      setIsSlow(false);
      setStatusMessage("");
      setRequestCanRetry(event.retryable === true);
      setRun((current) =>
        current ? applyStreamEvent(current, event, Date.now()) : current,
      );
      if (event.code === "profile_unavailable") {
        const rejectedProfileID = event.profile_id ?? requestProfileID;
        if (onRejectedProfileID) {
          onRejectedProfileID(rejectedProfileID);
        } else {
          setLocalRejectedProfileIDs((current) =>
            addRejectedProfileID(current, rejectedProfileID),
          );
        }
      }
      setRequestError(event.message || "助手回答失败，请重试");
      return;
    }
    if (event.type === "complete") {
      setPhase("completed");
      setStatusMessage("回答完成");
      setSelection(null);
      setRun((current) =>
        current ? applyStreamEvent(current, event, Date.now()) : current,
      );
      setMetrics({
        firstContentMS: event.first_content_ms ?? 0,
        totalMS: event.total_ms ?? 0,
        profileID: event.profile_id ?? "",
      });
    }
  };

  const ask = async (
    preserveAnswer: boolean,
    requestToRetry?: EpisodeCopilotQuestion,
    explicitQuestion?: string,
  ) => {
    const normalizedQuestion =
      requestToRetry?.question ?? explicitQuestion ?? question.trim();
    if (!normalizedQuestion || !scope || activeRequest.current || personSelectionInvalid || (targetPerson && targetPerson.status !== "confirmed")) return;
    if (requestToRetry?.target_person_id && !scope.people?.some((person) => person.id === requestToRetry.target_person_id && person.status === "confirmed")) return;
    const requestProfileID =
      requestToRetry?.profile_id ??
      selectedProfileID ??
      resolveProfileID(scope);
    if (rejectedProfileIDs.has(requestProfileID)) return;
    profileMenu.dismissMenu();
    contextMenu.dismissMenu();
    terminalStreamErrorHandled.current = false;
    const controller = new AbortController();
    activeRequest.current = controller;
    const request: EpisodeCopilotQuestion = requestToRetry ?? {
      question: normalizedQuestion,
      selection: selection?.text ?? "",
      selection_source: selection?.source ?? "",
      include_private_note:
        includePrivateNote && scope.private_note_available,
      profile_id: requestProfileID,
      ...(targetPerson ? { target_person_id: targetPerson.id } : {}),
    };
    if (requestToRetry) {
      selectProfile(request.profile_id);
    }
    if (!requestToRetry) {
      retryRequest.current = request;
      setAskedQuestion(normalizedQuestion);
      setSubmissionAnnouncement(`已发送问题：${normalizedQuestion}`);
      setIncludePrivateNote(false);
      setCopyState("idle");
    }
    const replaceAnswer = { current: preserveAnswer };
    if (!preserveAnswer) setAnswer("");
    setPhase("waiting");
    setStatusMessage(
      targetPerson
        ? scope.index_ready === false
          ? "正在准备人物发言索引，不会把它显示为无资料…"
          : "正在检索该人物的库内发言…"
        : "正在核对当前单集与公开资料…",
    );
    setRequestError(null);
    setRequestCanRetry(false);
    setMetrics(null);
    // The activity card exists before the first backend event so the user
    // never wonders whether the click registered.
    setRun(createRunState(Date.now(), Boolean(request.target_person_id)));
    setNowTick(Date.now());
    stickToBottom.current = true;
    try {
      await episodeCopilotApi.ask(
        item.episode_id,
        request,
        (event) => handleEvent(event, replaceAnswer, request.profile_id),
        controller.signal,
      );
    } catch (error) {
      if (isEpisodeCopilotCancellation(error)) {
        setPhase("cancelled");
        setStatusMessage("已取消；问题、选区和已有答案已保留。");
        setRun((current) =>
          current ? cancelRun(current, Date.now()) : current,
        );
      } else if (!terminalStreamErrorHandled.current) {
        setPhase("failed");
        setIsSlow(false);
        setStatusMessage("");
        const code = (error as { code?: string } | null)?.code ?? null;
        const retryable = code !== "profile_unavailable";
        setRequestCanRetry(retryable);
        setRun((current) =>
          current
            ? applyStreamEvent(
                current,
                {
                  type: "error",
                  message: getErrorMessage(error),
                  code,
                  retryable,
                  transcript_used: false,
                  private_note_included: false,
                },
                Date.now(),
              )
            : current,
        );
        if (code === "profile_unavailable") {
          if (onRejectedProfileID) {
            onRejectedProfileID(request.profile_id);
          } else {
            setLocalRejectedProfileIDs((current) =>
              addRejectedProfileID(current, request.profile_id),
            );
          }
        }
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

  const submitQuickQuestion = (quickQuestion: string) => {
    if (activeRequest.current) return;
    setQuestion(quickQuestion);
    void ask(false, undefined, quickQuestion);
  };

  const copyAnswer = async () => {
    if (copyTimer.current !== null) {
      window.clearTimeout(copyTimer.current);
    }
    try {
      await navigator.clipboard.writeText(answer);
      setCopyState("copied");
    } catch {
      setCopyState("failed");
    }
    copyTimer.current = window.setTimeout(() => {
      copyTimer.current = null;
      setCopyState("idle");
    }, 1600);
  };

  const isActive = phase === "waiting" || phase === "streaming";

  // Elapsed clocks tick once per second while a run is active so wait and
  // stage times stay honest without waiting for new events.
  const runIsActive = run !== null && run.finish === null && isActive;
  useEffect(() => {
    if (!runIsActive) return;
    setNowTick(Date.now());
    const ticker = window.setInterval(() => setNowTick(Date.now()), 1000);
    return () => window.clearInterval(ticker);
  }, [runIsActive]);

  const effectiveSelectedProfileID =
    selectedProfileID ?? (scope ? resolveProfileID(scope) : null);
  const isRejectedProfileSelected =
    effectiveSelectedProfileID !== null &&
    rejectedProfileIDs.has(effectiveSelectedProfileID);
  const pendingTarget =
    targetPerson !== null && targetPerson.status !== "confirmed";
  const canAsk =
    Boolean(scope) &&
    question.trim().length > 0 &&
    !isActive &&
    !isRejectedProfileSelected &&
    !pendingTarget && !personSelectionInvalid;

  const people = scope?.people ?? [];
  const activeMention = mentionDraft(
    question,
    question.length,
  );
  const mentionCandidates = mentionOpen
    ? filterPeople(people, activeMention?.query ?? "")
    : [];

  const handleComposerKeyDown = (event: ReactKeyboardEvent) => {
    const composing =
      event.nativeEvent.isComposing || event.key === "Process" || event.keyCode === 229;
    if (composing) return;
    if (mentionOpen && mentionCandidates.length && (event.key === "ArrowDown" || event.key === "ArrowUp")) {
      event.preventDefault();
      setMentionIndex((index) => (index + (event.key === "ArrowDown" ? 1 : -1) + mentionCandidates.length) % mentionCandidates.length);
      return;
    }
    if (event.key === "Escape") {
      setMentionOpen(false);
      return;
    }
    if (event.key === "Enter" && !event.shiftKey) {
      event.preventDefault();
      if (mentionOpen && mentionCandidates[mentionIndex % mentionCandidates.length]) {
        selectPerson(mentionCandidates[mentionIndex % mentionCandidates.length]);
        return;
      }
      if (canAsk) void ask(false);
    }
  };

  const selectPerson = (person: EpisodePersonCandidate) => {
    const draft = mentionDraft(question, question.length);
    if (draft) {
      setQuestion(replaceMention(question, draft, question.length));
    }
    setTargetPerson(person);
 setPersonSelectionInvalid(false);
    setMentionOpen(false);

  };

  const showRetry =
    (phase === "failed" && requestCanRetry) || phase === "cancelled";
  const answerComplete = phase === "completed";

  return (
    <section className={styles.copilotSection} aria-label="单集助手">
      {isLoadingScope && (
        <div className={styles.copilotPending} role="status">
          <IconSparkles size={18} stroke={1.6} aria-hidden="true" />
          正在核对可用上下文…
        </div>
      )}
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
          <span className={styles.srOnly} role="status">
            {submissionAnnouncement}
          </span>

          <div
            ref={conversationRef}
            className={styles.copilotConversation}
            onScroll={handleConversationScroll}
          >
            {!askedQuestion && (
              <div className={styles.copilotWelcome}>
                <span className={styles.copilotWelcomeMark} aria-hidden="true">
                  <IconSparkles size={17} stroke={1.7} />
                </span>
                <p className={styles.copilotWelcomeTitle}>
                  你好，我是这一集的单集助手。
                </p>
                <p className={styles.copilotWelcomeBody}>
                  {welcomeContextMessage(scope)}
                </p>
                <div
                  className={styles.copilotSuggestions}
                  role="group"
                  aria-label="快捷问题"
                >
                  {quickQuestions.map((quickQuestion) => (
                    <button
                      key={quickQuestion}
                      type="button"
                      className={styles.copilotSuggestion}
                      onClick={() => submitQuickQuestion(quickQuestion)}
                    >
                      {quickQuestion}
                    </button>
                  ))}
                </div>
              </div>
            )}

            {askedQuestion && (
              <div className={styles.copilotUserMessage}>
                <p>{askedQuestion}</p>
              </div>
            )}

            {run && (
              <EpisodeCopilotActivityCard
                run={run}
                now={nowTick}
                isSlow={isSlow}
                profileName={profileDisplayName(
                  retryRequest.current?.profile_id ?? "",
                )}
              />
            )}

            {(statusMessage || isSlow) && (
              <span className={styles.copilotStatus} role="status">
                {statusMessage || "响应较慢；单集仍可阅读，可随时取消。"}
              </span>
            )}

            {requestError && (
              <div className={styles.inlineError} role="alert">
                <span>{requestError}</span>
              </div>
            )}

            {showRetry && (
              <div className={styles.copilotTimelineActions}>
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
              </div>
            )}

            {answer && (
              <div className={styles.copilotAnswer}>
                <MarkdownViewer content={answer} density="reading" />
                {parseLibrarySources(answer).length > 0 ? (
                  <div className={styles.copilotSourceLinks}>
                    {parseLibrarySources(answer).map((source) => (
                      <button
                        key={`${source.episodeId}-${source.fragmentOrder}`}
                        type="button"
                        className={styles.copilotSourceLink}
                        onClick={() =>
                          void (async () => {
                            if (source.sourceVersion) {
                              const current = await episodeCopilotApi.getPeople(source.episodeId);
                              if (current.source_version !== source.sourceVersion) throw new Error("source changed");
                            }
                            await jumpToLibrarySource(source, { openEpisode: onOpenSourceEpisode });
                          })().catch(() => setScopeError("来源已更新或暂时无法定位，请重新提问核对。"))
                        }
                      >
                        打开来源 · {source.title || `单集 ${source.episodeId}`} · {source.date} · 片段{" "}
                        {source.fragmentOrder}
                      </button>
                    ))}
                  </div>
                ) : null}
                {answerComplete && (
                  <div className={styles.copilotAnswerActions}>
                    <button
                      type="button"
                      className={styles.copilotAnswerAction}
                      onClick={() => void copyAnswer()}
                    >
                      <IconCopy size={15} stroke={1.7} aria-hidden="true" />
                      {copyState === "copied"
                        ? "已复制"
                        : copyState === "failed"
                          ? "复制失败"
                          : "复制"}
                    </button>
                    <button
                      type="button"
                      className={styles.copilotAnswerAction}
                      onClick={() =>
                        void ask(true, retryRequest.current ?? undefined)
                      }
                    >
                      <IconRefresh size={15} stroke={1.7} aria-hidden="true" />
                      重新生成
                    </button>
                  </div>
                )}
                {metrics && (
                  <span className={styles.copilotMetrics}>
                    首字 {metrics.firstContentMS}ms · 完成 {metrics.totalMS}ms
                    {metrics.profileID &&
                      ` · ${profileDisplayName(metrics.profileID)}`}
                  </span>
                )}
              </div>
            )}
          </div>

          <div className={styles.copilotComposerShell}>
            {isRejectedProfileSelected && effectiveSelectedProfileID && (
              <p className={styles.copilotNotice} role="status">
                当前选择的{profileDisplayName(effectiveSelectedProfileID)}档位已确认不可用；请切换其他档位后再提问。
              </p>
            )}
            {!scope.transcript_available && scope.show_notes_available && (
              <p className={styles.copilotNotice}>
                当前无成功逐字稿，将明确降级为 Show Notes。
              </p>
            )}
            {!scope.transcript_available && !scope.show_notes_available && (
              <p className={styles.copilotNotice}>
                当前无可用 Show Notes 或逐字稿；回答将仅参考单集信息与公开资料。
              </p>
            )}

            {selection && (
              <div className={styles.copilotAttachment}>
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
                  <IconX size={15} stroke={1.8} aria-hidden="true" />
                </button>
              </div>
            )}

            <div className={styles.copilotComposer}>
              {targetPerson ? (
                <div className={styles.copilotPersonChip} data-testid="copilot-person-chip">
                  <span>
                    {targetPerson.display_name}
                    {targetPerson.aliases[0] ? ` / ${targetPerson.aliases[0]}` : ""}
                    · {roleLabel(targetPerson.role)}
                    {targetPerson.status === "pending" ? " · 身份待确认" : ""}
                  </span>
                  <span className={styles.copilotPersonChipActions}>
                    <button
                      type="button"
                      className={styles.iconButton}
                      aria-label="清除人物选择"
                      onClick={() => {
                        setTargetPerson(null);
                        setPersonSelectionInvalid(false);
                      }}
                    >
                      <IconX size={15} stroke={1.8} aria-hidden="true" />
                    </button>
                  </span>
                </div>
              ) : null}
              {pendingTarget ? (
                <p className={styles.copilotNotice} role="status">
                  待确认人物不能提交模拟回答，请到逐字稿确认人物。普通问答仍可用。
                </p>
              ) : null}
              {scope.index_ready === false ? (
                <p className={styles.copilotNotice} role="status">
                  {scope.preparation_state === "outdated" ? "人物资料需要重新识别。原文和普通问答仍可使用。" : "人物资料尚未准备。可以先阅读原文或进行普通问答。"}
                </p>
              ) : null}
              {scope.transcript_available && <button type="button" className={styles.copilotCorrectionToggle} onClick={onManagePeople}>到逐字稿确认人物</button>}
              {personSelectionInvalid ? <p role="alert">原选人物已失效，问题已保留。请重新选择人物，或<button type="button" onClick={() => setPersonSelectionInvalid(false)}>改为普通问答</button>。</p> : null}
              <textarea
                className={styles.copilotComposerInput}
                aria-label="向单集助手提问"
                aria-controls={mentionOpen ? "copilot-people-list" : undefined}
                aria-activedescendant={mentionOpen && mentionCandidates.length ? `copilot-person-option-${mentionCandidates[mentionIndex % mentionCandidates.length].id}` : undefined}
                value={question}
                onChange={(event) => {
                  const value = event.target.value;
                  setQuestion(value);
                  const draft = mentionDraft(
                    value,
                    event.target.selectionStart ?? value.length,
                  );
                  setMentionOpen(Boolean(draft) && !targetPerson);
                  setMentionIndex(0);
                }}
                onKeyDown={handleComposerKeyDown}
                placeholder={
                  targetPerson
                    ? `向${targetPerson.display_name}提问，回答会标明 AI 模拟…`
                    : selection
                      ? "解释、核对或寻找与这段内容相关的公开资源…"
                    : "输入 @ 选择本集主播或嘉宾，或围绕当前单集提问…"
                }
                rows={2}
                maxLength={2000}
              />
              {mentionOpen && mentionCandidates.length > 0 ? (
                <ul
                  className={styles.copilotMentionList}
                  role="listbox"
                  aria-label="选择本集人物"
                  id="copilot-people-list"
                >
                  {mentionCandidates.map((person,index) => (
                    <li key={person.id}>
                      <button
                        type="button"
                        role="option"
                        id={`copilot-person-option-${person.id}`}
                        aria-selected={index === mentionIndex % mentionCandidates.length}
                        aria-label={`选择${person.display_name}`}
                        aria-describedby={`copilot-person-description-${person.id}`}
                        onClick={() => selectPerson(person)}
                      >
                        <strong>{person.display_name}</strong>
                        {person.aliases.length ? `（${person.aliases.join("、")}）` : ""}
                        · <span id={`copilot-person-description-${person.id}`}>
                          {roleLabel(person.role)}
                          {person.identity_note ? ` · ${person.identity_note}` : ""}
                          {person.status === "pending" ? " · 身份待确认" : ""}
                        </span>
                      </button>
                    </li>
                  ))}
                </ul>
              ) : null}
              <div className={styles.copilotComposerControls}>
                <button
                  ref={contextMenu.triggerRef}
                  type="button"
                  className={styles.copilotContextButton}
                  aria-label="查看本次回答上下文"
                  aria-expanded={contextMenu.open}
                  aria-controls={
                    contextMenu.open ? contextMenu.menuId : undefined
                  }
                  onClick={() => {
                    profileMenu.dismissMenu();
                    contextMenu.toggleMenu();
                  }}
                >
                  <IconPlus size={19} stroke={1.7} aria-hidden="true" />
                </button>
                <button
                  ref={profileMenu.triggerRef}
                  type="button"
                  className={styles.copilotProfileTrigger}
                  data-testid="copilot-profiles"
                  aria-label={`回答档位：${profileDisplayName(
                    effectiveSelectedProfileID ?? "",
                  )}`}
                  aria-haspopup="menu"
                  aria-expanded={profileMenu.open}
                  aria-controls={
                    profileMenu.open ? profileMenu.menuId : undefined
                  }
                  disabled={isActive}
                  onClick={() => {
                    contextMenu.dismissMenu();
                    profileMenu.toggleMenu();
                  }}
                >
                  <IconScale size={15} stroke={1.8} aria-hidden="true" />
                  {profileDisplayName(effectiveSelectedProfileID ?? "")}
                  <IconChevronDown size={13} stroke={1.8} aria-hidden="true" />
                </button>
                {isActive ? (
                  <button
                    type="button"
                    className={styles.copilotSendButton}
                    aria-label="取消"
                    title="取消"
                    onClick={() => activeRequest.current?.abort()}
                  >
                    <IconPlayerStop
                      size={19}
                      stroke={1.8}
                      aria-hidden="true"
                    />
                  </button>
                ) : (
                  <button
                    type="button"
                    className={styles.copilotSendButton}
                    aria-label="提问"
                    title="提问"
                    disabled={!canAsk}
                    onClick={() => void ask(false)}
                  >
                    <IconSend size={19} stroke={1.8} aria-hidden="true" />
                  </button>
                )}
              </div>

              {profileMenu.open && scope.profiles?.length ? (
                <EpisodeCopilotProfileMenu
                  id={profileMenu.menuId}
                  menuRef={profileMenu.menuRef}
                  profiles={orderedProfiles(scope)}
                  selectedID={effectiveSelectedProfileID ?? "balanced"}
                  rejectedIDs={rejectedProfileIDs}
                  onSelect={(profileID) => {
                    selectProfile(profileID);
                    profileMenu.closeMenu();
                  }}
                  onKeyDown={profileMenu.handleMenuKeyDown}
                />
              ) : null}
              {contextMenu.open && (
                <EpisodeCopilotContextMenu
                  id={contextMenu.menuId}
                  menuRef={contextMenu.menuRef}
                  scope={scope}
                  includePrivateNote={includePrivateNote}
                  isActive={isActive}
                  onTogglePrivateNote={(include) =>
                    setIncludePrivateNote(include)
                  }
                  onKeyDown={contextMenu.handleMenuKeyDown}
                />
              )}
            </div>

            <p className={styles.copilotReadonlyNote}>
              <IconShieldCheck size={14} stroke={1.7} aria-hidden="true" />
              只读回答 · 不会修改你的内容
            </p>
          </div>
        </>
      )}
    </section>
  );
}
