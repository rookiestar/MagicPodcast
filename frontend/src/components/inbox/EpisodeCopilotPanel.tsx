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
import styles from "./InboxPage.module.css";

interface EpisodeCopilotPanelProps {
  item: ConsumptionItem;
  selectedProfileID?: EpisodeCopilotProfileID | null;
  onSelectedProfileIDChange?: (profileID: EpisodeCopilotProfileID) => void;
  rejectedProfileIDs?: ReadonlySet<EpisodeCopilotProfileID>;
  onRejectedProfileID?: (profileID: EpisodeCopilotProfileID) => void;
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
    dismissProfileMenu();
    dismissContextMenu();
    retryRequest.current = null;
    terminalStreamErrorHandled.current = false;
    void loadScope();
    return () => activeRequest.current?.abort();
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
    if (!normalizedQuestion || !scope || activeRequest.current) return;
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
    setStatusMessage("正在核对当前单集与公开资料…");
    setRequestError(null);
    setRequestCanRetry(false);
    setMetrics(null);
    // The activity card exists before the first backend event so the user
    // never wonders whether the click registered.
    setRun(createRunState(Date.now()));
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
  const canAsk =
    Boolean(scope) &&
    question.trim().length > 0 &&
    !isActive &&
    !isRejectedProfileSelected;

  const handleComposerKeyDown = (event: ReactKeyboardEvent) => {
    if (event.key === "Enter" && !event.shiftKey) {
      event.preventDefault();
      if (canAsk) void ask(false);
    }
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
              <textarea
                className={styles.copilotComposerInput}
                aria-label="向单集助手提问"
                value={question}
                onChange={(event) => setQuestion(event.target.value)}
                onKeyDown={handleComposerKeyDown}
                placeholder={
                  selection
                    ? "解释、核对或寻找与这段内容相关的公开资源…"
                    : "围绕当前单集提问，或先在 Show Notes / 逐字稿中划词…"
                }
                rows={2}
                maxLength={2000}
              />
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
