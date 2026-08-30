"use client";

import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type KeyboardEvent,
} from "react";
import {
  IconArrowLeft,
  IconArrowRight,
  IconBookmarkPlus,
  IconCheck,
  IconCircleCheck,
  IconDots,
  IconEdit,
  IconExternalLink,
  IconRefresh,
  IconSparkles,
  IconX,
} from "@tabler/icons-react";
import { OriginalEpisodeRecovery } from "@/components/common/OriginalEpisodeRecovery";
import RichText from "@/components/RichText";
import { useOriginalEpisodeRecovery } from "@/hooks/useOriginalEpisodeRecovery";
import { episodeApi, tagApi } from "@/lib/api";
import {
  consumptionApi,
  getConsumptionErrorDetails,
} from "@/lib/api/consumption";
import { getErrorMessage } from "@/lib/errorMessage";
import {
  openOriginalEpisodeTab,
  planSafeOriginalEpisodeOpen,
} from "@/lib/originalEpisodeOpen";
import type { Tag } from "@/types";
import {
  CONSUMPTION_QUEUES,
  type ConsumptionItem,
  type ConsumptionQueue,
} from "@/types/consumption";
import {
  formatDuration,
  formatPublishedDate,
  QUEUE_PRESENTATION,
} from "./presentation";
import EpisodeCopilotPanel from "./EpisodeCopilotPanel";
import EpisodeProcessingPanel, {
  type EpisodeProcessingHeaderState,
  type EpisodeProcessingPanelHandle,
} from "./EpisodeProcessingPanel";
import styles from "./InboxPage.module.css";

interface ConsumptionDetailPanelProps {
  item: ConsumptionItem;
  isQueueBusy: boolean;
  onClose: () => void;
  onItemChange: (item: ConsumptionItem) => void;
  onMove: (
    item: ConsumptionItem,
    target: ConsumptionQueue,
  ) => Promise<ConsumptionItem | undefined>;
  onCopilotWorkspaceChange?: (isOpen: boolean) => void;
}

const DETAIL_TABS = [
  { id: "show-notes", label: "Show Notes" },
  { id: "transcript", label: "转写" },
  { id: "notes", label: "笔记" },
] as const;

type DetailTab = (typeof DETAIL_TABS)[number]["id"];

const INITIAL_PROCESSING_HEADER: EpisodeProcessingHeaderState = {
  kind: "loading",
  label: "正在读取",
  detail: "Show Notes 可继续阅读",
  primaryLabel: "读取转写状态",
  primaryDisabled: true,
  action: null,
  showTranscriptTab: false,
};

function EpisodeMetadata({
  item,
  onItemChange,
}: {
  item: ConsumptionItem;
  onItemChange: (item: ConsumptionItem) => void;
}) {
  const [notes, setNotes] = useState(item.notes ?? "");
  const [savedNotes, setSavedNotes] = useState(item.notes ?? "");
  const [tags, setTags] = useState<Tag[]>(item.tags ?? []);
  const [allTags, setAllTags] = useState<Tag[]>([]);
  const [selectedTagId, setSelectedTagId] = useState("");
  const [isLoading, setIsLoading] = useState(true);
  const [isEditingNotes, setIsEditingNotes] = useState(false);
  const [isSavingNotes, setIsSavingNotes] = useState(false);
  const [isUpdatingTags, setIsUpdatingTags] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const loadMetadata = async () => {
    setIsLoading(true);
    setError(null);
    try {
      const [nextNotes, nextTags, availableTags] = await Promise.all([
        episodeApi.getNotes(item.episode_id),
        episodeApi.getTags(item.episode_id),
        tagApi.list(),
      ]);
      setNotes(nextNotes);
      setSavedNotes(nextNotes);
      setTags(nextTags);
      setAllTags(availableTags);
      onItemChange({ ...item, notes: nextNotes, tags: nextTags });
    } catch (loadError) {
      setError(`备注与标签加载失败：${getErrorMessage(loadError)}`);
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    let active = true;
    setIsLoading(true);
    setError(null);

    void Promise.all([
      episodeApi.getNotes(item.episode_id),
      episodeApi.getTags(item.episode_id),
      tagApi.list(),
    ])
      .then(([nextNotes, nextTags, availableTags]) => {
        if (!active) return;
        setNotes(nextNotes);
        setSavedNotes(nextNotes);
        setTags(nextTags);
        setAllTags(availableTags);
        onItemChange({ ...item, notes: nextNotes, tags: nextTags });
      })
      .catch((loadError: unknown) => {
        if (!active) return;
        setError(`备注与标签加载失败：${getErrorMessage(loadError)}`);
      })
      .finally(() => {
        if (active) setIsLoading(false);
      });

    return () => {
      active = false;
    };
    // A new episode identity owns a new metadata session. Item updates from
    // queue writes should not restart an in-flight note edit.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [item.episode_id]);

  const availableTags = useMemo(() => {
    const selected = new Set(tags.map((tag) => tag.id));
    return allTags.filter((tag) => !selected.has(tag.id));
  }, [allTags, tags]);

  const saveNotes = async () => {
    if (isSavingNotes || isUpdatingTags) return;
    const previous = savedNotes;
    setIsSavingNotes(true);
    setError(null);
    try {
      await episodeApi.updateNotes(item.episode_id, notes);
      setSavedNotes(notes);
      setIsEditingNotes(false);
      onItemChange({ ...item, notes, tags });
    } catch (saveError) {
      setNotes(previous);
      setError(`备注保存失败：${getErrorMessage(saveError)}`);
    } finally {
      setIsSavingNotes(false);
    }
  };

  const addSelectedTag = async () => {
    const tagId = Number(selectedTagId);
    const tag = availableTags.find((candidate) => candidate.id === tagId);
    if (!tag || isUpdatingTags || isSavingNotes) return;

    const previous = tags;
    const next = [...tags, tag];
    setTags(next);
    setSelectedTagId("");
    setIsUpdatingTags(true);
    setError(null);
    try {
      await episodeApi.addTag(item.episode_id, tag.id);
      onItemChange({ ...item, notes: savedNotes, tags: next });
    } catch (tagError) {
      setTags(previous);
      setError(`标签更新失败：${getErrorMessage(tagError)}`);
    } finally {
      setIsUpdatingTags(false);
    }
  };

  const removeTag = async (tag: Tag) => {
    if (isUpdatingTags || isSavingNotes) return;
    const previous = tags;
    const next = tags.filter((candidate) => candidate.id !== tag.id);
    setTags(next);
    setIsUpdatingTags(true);
    setError(null);
    try {
      await episodeApi.removeTag(item.episode_id, tag.id);
      onItemChange({ ...item, notes: savedNotes, tags: next });
    } catch (tagError) {
      setTags(previous);
      setError(`标签更新失败：${getErrorMessage(tagError)}`);
    } finally {
      setIsUpdatingTags(false);
    }
  };

  return (
    <section
      className={styles.metadataSection}
      aria-label="单集笔记与标签"
      aria-busy={isLoading}
    >
      {error && (
        <div className={styles.inlineError} role="alert">
          <span>{error}</span>
          {!isLoading && error.includes("加载失败") && (
            <button
              type="button"
              className={styles.iconButton}
              onClick={() => void loadMetadata()}
              aria-label="重试加载备注与标签"
              title="重试"
            >
              <IconRefresh size={18} stroke={1.8} aria-hidden="true" />
            </button>
          )}
        </div>
      )}

      <div className={styles.metadataGrid}>
        <div className={styles.metadataBlock}>
          <div className={styles.metadataCardHeader}>
            <h3>备注</h3>
            {isLoading ? (
              <span className={styles.metadataStatus} role="status">
                同步中…
              </span>
            ) : !isEditingNotes ? (
              <button
                type="button"
                className={styles.iconButton}
                disabled={isSavingNotes || isUpdatingTags}
                onClick={() => setIsEditingNotes(true)}
                aria-label="编辑单集备注"
                title="编辑备注"
              >
                <IconEdit size={18} stroke={1.8} aria-hidden="true" />
              </button>
            ) : null}
          </div>
          {isEditingNotes ? (
            <div className={styles.metadataEditor}>
              <textarea
                className={styles.notesTextarea}
                value={notes}
                rows={5}
                disabled={isSavingNotes || isUpdatingTags}
                onChange={(event) => setNotes(event.target.value)}
                aria-label="单集备注"
                placeholder="记录你的判断、疑问或待办…"
              />
              <div className={styles.metadataActions}>
                {isSavingNotes && <span role="status">正在保存备注…</span>}
                <button
                  type="button"
                  className={styles.iconButton}
                  disabled={isSavingNotes}
                  onClick={() => {
                    setNotes(savedNotes);
                    setIsEditingNotes(false);
                  }}
                  aria-label="取消编辑单集备注"
                  title="取消"
                >
                  <IconX size={18} stroke={1.8} aria-hidden="true" />
                </button>
                <button
                  type="button"
                  className={styles.iconButtonStrong}
                  disabled={isSavingNotes || isUpdatingTags}
                  onClick={() => void saveNotes()}
                  aria-label="保存单集备注"
                  title="保存"
                >
                  <IconCheck size={18} stroke={1.9} aria-hidden="true" />
                </button>
              </div>
            </div>
          ) : (
            <p className={styles.notesReadOnly} data-empty={!savedNotes.trim()}>
              {savedNotes.trim() || "暂无备注。记录这一集值得留下的判断。"}
            </p>
          )}
        </div>

        <div className={styles.metadataBlock}>
          <div className={styles.metadataCardHeader}>
            <h3>标签</h3>
            {isUpdatingTags && (
              <span className={styles.metadataStatus} role="status">
                更新中…
              </span>
            )}
          </div>
          <div className={styles.tagList} aria-label="现有单集标签">
            {tags.length === 0 ? (
              <span className={styles.metadataEmpty}>暂无标签。</span>
            ) : (
              tags.map((tag) => (
                <span className={styles.tagChip} key={tag.id}>
                  <span
                    className={styles.tagDot}
                    style={{ backgroundColor: tag.color || "#d7681d" }}
                    aria-hidden="true"
                  />
                  {tag.name}
                  <button
                    type="button"
                    disabled={isUpdatingTags || isSavingNotes}
                    onClick={() => void removeTag(tag)}
                    aria-label={`移除标签 ${tag.name}`}
                    title="移除标签"
                  >
                    <IconX size={14} stroke={1.9} aria-hidden="true" />
                  </button>
                </span>
              ))
            )}
          </div>
          <div className={styles.tagPicker}>
            <label htmlFor={`episode-tag-picker-${item.episode_id}`}>
              添加标签
            </label>
            <div>
              <select
                id={`episode-tag-picker-${item.episode_id}`}
                value={selectedTagId}
                onChange={(event) => setSelectedTagId(event.target.value)}
                disabled={
                  isLoading ||
                  isUpdatingTags ||
                  isSavingNotes ||
                  availableTags.length === 0
                }
                aria-label="选择已有标签"
              >
                <option value="">
                  {availableTags.length > 0 ? "选择标签" : "没有可添加的标签"}
                </option>
                {availableTags.map((tag) => (
                  <option key={tag.id} value={tag.id}>
                    {tag.name}
                  </option>
                ))}
              </select>
              <button
                type="button"
                className={styles.iconButton}
                disabled={!selectedTagId || isUpdatingTags || isSavingNotes}
                onClick={() => void addSelectedTag()}
                aria-label="添加所选标签"
                title="添加标签"
              >
                <IconBookmarkPlus size={18} stroke={1.8} aria-hidden="true" />
              </button>
            </div>
          </div>
        </div>
      </div>
    </section>
  );
}

export default function ConsumptionDetailPanel({
  item,
  isQueueBusy,
  onClose,
  onItemChange,
  onMove,
  onCopilotWorkspaceChange,
}: ConsumptionDetailPanelProps) {
  const panelRef = useRef<HTMLDivElement>(null);
  const detailScrollRef = useRef<HTMLDivElement>(null);
  const closeButtonRef = useRef<HTMLButtonElement>(null);
  const copilotTriggerRef = useRef<HTMLButtonElement>(null);
  const copilotReturnRef = useRef<HTMLButtonElement>(null);
  const copilotRestoreRef = useRef<{
    detailScrollTop: number;
    focusedElement: HTMLElement | null;
  } | null>(null);
  const processingPanelRef = useRef<EpisodeProcessingPanelHandle>(null);
  const tabRefs = useRef<Record<DetailTab, HTMLButtonElement | null>>({
    "show-notes": null,
    transcript: null,
    notes: null,
  });
  const [isRefreshing, setIsRefreshing] = useState(false);
  const [isMobileViewport, setIsMobileViewport] = useState(false);
  const [isCopilotOpen, setIsCopilotOpen] = useState(false);
  const [detailError, setDetailError] = useState<string | null>(null);
  const [activeTab, setActiveTab] = useState<DetailTab>("show-notes");
  const [processingHeader, setProcessingHeader] =
    useState<EpisodeProcessingHeaderState>(INITIAL_PROCESSING_HEADER);
  const [externalState, setExternalState] = useState<
    "idle" | "saving" | "failed"
  >("idle");
  const [moveTarget, setMoveTarget] = useState<ConsumptionQueue>(
    item.queue_state === "focus" ? "someday" : "focus",
  );
  const originalPlan = useMemo(
    () => planSafeOriginalEpisodeOpen(item.original_url),
    [item.original_url],
  );
  const originalRecovery = useOriginalEpisodeRecovery();

  useEffect(() => {
    closeButtonRef.current?.focus();
    const previousOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => {
      document.body.style.overflow = previousOverflow;
    };
  }, []);

  useEffect(() => {
    const updateViewport = () => {
      setIsMobileViewport(window.innerWidth <= 900);
    };
    updateViewport();
    window.addEventListener("resize", updateViewport, { passive: true });
    return () => window.removeEventListener("resize", updateViewport);
  }, []);

  useEffect(() => {
    setIsCopilotOpen(false);
    copilotRestoreRef.current = null;
    setActiveTab("show-notes");
    setProcessingHeader(INITIAL_PROCESSING_HEADER);
  }, [item.episode_id]);

  useEffect(() => {
    let active = true;
    setIsRefreshing(true);
    setDetailError(null);
    void consumptionApi
      .getItem(item.episode_id)
      .then((canonicalItem) => {
        if (active) onItemChange(canonicalItem);
      })
      .catch((error: unknown) => {
        if (active) {
          setDetailError(
            `最新状态读取失败，当前内容仍可查看：${
              getConsumptionErrorDetails(error).message
            }`,
          );
        }
      })
      .finally(() => {
        if (active) setIsRefreshing(false);
      });
    return () => {
      active = false;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [item.episode_id]);

  const handlePanelKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    if (event.key === "Escape") {
      event.preventDefault();
      if (isCopilotOpen) {
        closeCopilot();
      } else {
        onClose();
      }
      return;
    }
    if (event.key !== "Tab" || !panelRef.current) return;

    const focusable = Array.from(
      panelRef.current.querySelectorAll<HTMLElement>(
        'button:not([disabled]), a[href], summary, select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])',
      ),
    ).filter(
      (element) =>
        !element.closest("[hidden]") &&
        (element.tagName === "SUMMARY" ||
          !element.closest("details:not([open])")),
    );
    if (focusable.length === 0) return;
    const first = focusable[0];
    const last = focusable[focusable.length - 1];
    if (event.shiftKey && document.activeElement === first) {
      event.preventDefault();
      last.focus();
    } else if (!event.shiftKey && document.activeElement === last) {
      event.preventDefault();
      first.focus();
    }
  };

  const openOriginal = async () => {
    if (!originalPlan || externalState === "saving") return;
    setExternalState("saving");
    setDetailError(null);

    const saveIntent = consumptionApi.markInProgress(item.episode_id);
    openOriginalEpisodeTab(originalPlan.openUrl);
    originalRecovery.activate(item.episode_id, originalPlan);

    try {
      const updated = await saveIntent;
      onItemChange(updated);
      setExternalState("idle");
    } catch {
      setExternalState("failed");
    }
  };

  const retryInProgress = async () => {
    setExternalState("saving");
    try {
      const updated = await consumptionApi.markInProgress(item.episode_id);
      onItemChange(updated);
      setExternalState("idle");
    } catch {
      setExternalState("failed");
    }
  };

  const moveItem = async (target: ConsumptionQueue) => {
    const updated = await onMove(item, target);
    if (updated) onItemChange(updated);
  };

  const currentQueue = item.queue_state
    ? QUEUE_PRESENTATION[item.queue_state].label
    : "未收集";
  const queueOptions = CONSUMPTION_QUEUES.filter(
    (queue) => queue !== item.queue_state,
  );
  const effectiveMoveTarget = queueOptions.includes(moveTarget)
    ? moveTarget
    : queueOptions[0];

  const selectTab = useCallback((tab: DetailTab, shouldFocus = false) => {
    setActiveTab(tab);
    if (shouldFocus) {
      window.requestAnimationFrame(() => tabRefs.current[tab]?.focus());
    }
  }, []);
  const visibleDetailTabs = useMemo(
    () =>
      DETAIL_TABS.filter(
        (tab) => tab.id !== "transcript" || processingHeader.showTranscriptTab,
      ),
    [processingHeader.showTranscriptTab],
  );

  useEffect(() => {
    if (activeTab === "transcript" && !processingHeader.showTranscriptTab) {
      selectTab("show-notes", true);
    }
  }, [activeTab, processingHeader.showTranscriptTab, selectTab]);

  const openCopilot = useCallback(() => {
    if (isCopilotOpen) return;
    copilotRestoreRef.current = {
      detailScrollTop: detailScrollRef.current?.scrollTop ?? 0,
      focusedElement:
        copilotTriggerRef.current ??
        (document.activeElement instanceof HTMLElement
          ? document.activeElement
          : null),
    };
    onCopilotWorkspaceChange?.(true);
    setIsCopilotOpen(true);
  }, [isCopilotOpen, onCopilotWorkspaceChange]);

  const closeCopilot = useCallback(() => {
    if (!isCopilotOpen) return;
    setIsCopilotOpen(false);
    onCopilotWorkspaceChange?.(false);
  }, [isCopilotOpen, onCopilotWorkspaceChange]);

  useEffect(() => {
    const snapshot = copilotRestoreRef.current;
    if (!snapshot) return;
    if (detailScrollRef.current) {
      detailScrollRef.current.scrollTop = snapshot.detailScrollTop;
    }
    if (isCopilotOpen) {
      copilotReturnRef.current?.focus({ preventScroll: true });
      return;
    }
    const restoreTarget =
      snapshot.focusedElement?.isConnected &&
      panelRef.current?.contains(snapshot.focusedElement)
        ? snapshot.focusedElement
        : (copilotTriggerRef.current ?? closeButtonRef.current);
    restoreTarget?.focus({ preventScroll: true });
    copilotRestoreRef.current = null;
  }, [isCopilotOpen]);

  const handleTabKeyDown = (
    event: KeyboardEvent<HTMLButtonElement>,
    currentTab: DetailTab,
  ) => {
    const currentIndex = visibleDetailTabs.findIndex(
      (candidate) => candidate.id === currentTab,
    );
    if (currentIndex < 0) return;
    let nextIndex = currentIndex;
    if (event.key === "ArrowRight") {
      nextIndex = (currentIndex + 1) % visibleDetailTabs.length;
    } else if (event.key === "ArrowLeft") {
      nextIndex =
        (currentIndex - 1 + visibleDetailTabs.length) %
        visibleDetailTabs.length;
    } else if (event.key === "Home") {
      nextIndex = 0;
    } else if (event.key === "End") {
      nextIndex = visibleDetailTabs.length - 1;
    } else {
      return;
    }
    event.preventDefault();
    selectTab(visibleDetailTabs[nextIndex].id, true);
  };

  return (
    <div
      className={`${styles.detailBackdrop} ${
        isCopilotOpen ? styles.detailBackdropWorkspace : ""
      }`}
      onMouseDown={(event) => {
        if (event.currentTarget === event.target) onClose();
      }}
    >
      <div
        ref={panelRef}
        className={`${styles.detailPanel} ${
          isCopilotOpen ? styles.detailPanelWorkspace : ""
        }`}
        role="dialog"
        aria-modal="true"
        aria-labelledby={
          isCopilotOpen && isMobileViewport
            ? "episode-copilot-workspace-title"
            : "consumption-detail-title"
        }
        onKeyDown={handlePanelKeyDown}
      >
        <header
          className={styles.detailHeader}
          hidden={isCopilotOpen && isMobileViewport}
        >
          <div>
            <span className={styles.detailKicker}>FOCUS DETAIL</span>
            <p>{item.podcast_title}</p>
          </div>
          <div className={styles.detailHeaderActions}>
            <button
              ref={copilotTriggerRef}
              type="button"
              className={styles.detailCopilotTrigger}
              aria-label="单集助手"
              aria-expanded={isCopilotOpen}
              aria-controls="episode-copilot-workspace"
              title="单集助手"
              hidden={isCopilotOpen}
              onClick={openCopilot}
            >
              <span className={styles.detailCopilotGlyph} aria-hidden="true">
                <span>AI</span>
                <IconSparkles size={14} stroke={1.8} />
              </span>
            </button>
            <button
              ref={closeButtonRef}
              type="button"
              className={styles.detailClose}
              onClick={onClose}
              aria-label="关闭单集明细"
              title="关闭"
            >
              <IconX size={22} stroke={1.7} aria-hidden="true" />
            </button>
          </div>
        </header>

        <div className={styles.detailWorkspace}>
          <div
            ref={detailScrollRef}
            className={styles.detailScroll}
            hidden={isCopilotOpen && isMobileViewport}
          >
            <section className={styles.detailHero}>
              <div className={styles.detailTitleBlock}>
                <div className={styles.detailMetaLine}>
                  <span className={styles.detailEpisodeNo}>
                    {item.episode_no || "EPISODE"}
                  </span>
                  <span className={styles.detailByline}>
                    {item.podcast_author || item.podcast_title}
                  </span>
                  <dl className={styles.detailFacts}>
                    <div>
                      <dt>时长</dt>
                      <dd>{formatDuration(item.duration)}</dd>
                    </div>
                    <div>
                      <dt>发布日期</dt>
                      <dd>{formatPublishedDate(item.published_date)}</dd>
                    </div>
                  </dl>
                </div>
                <h2 id="consumption-detail-title">{item.episode_title}</h2>
              </div>

              <div className={styles.detailCommandBar}>
                <div
                  className={styles.processingHeadline}
                  data-state={processingHeader.kind}
                  role="status"
                  aria-label={`转写状态：${processingHeader.label}`}
                >
                  <span className={styles.processingDot} aria-hidden="true" />
                  <span>
                    <strong>{processingHeader.label}</strong>
                    {processingHeader.detail.trim() ? (
                      <small>{processingHeader.detail}</small>
                    ) : null}
                  </span>
                </div>
                <button
                  type="button"
                  className={styles.primaryCommand}
                  disabled={processingHeader.primaryDisabled}
                  onClick={() => processingPanelRef.current?.activatePrimary()}
                >
                  {processingHeader.primaryLabel}
                </button>
                {originalPlan ? (
                  <button
                    type="button"
                    className={styles.originalLink}
                    disabled={externalState === "saving"}
                    onClick={() => void openOriginal()}
                  >
                    原节目
                    <IconExternalLink
                      size={16}
                      stroke={1.8}
                      aria-hidden="true"
                    />
                  </button>
                ) : (
                  <span className={styles.unsafeOriginal}>
                    原节目链接不可安全打开
                  </span>
                )}
                <details className={styles.secondaryActions}>
                  <summary aria-label="更多操作" title="更多操作">
                    <IconDots size={20} stroke={1.8} aria-hidden="true" />
                  </summary>
                  <div className={styles.secondaryActionsMenu}>
                    <span className={styles.secondaryContext}>
                      {currentQueue} ·{" "}
                      {item.queue_state === "done"
                        ? "已手动完成"
                        : item.in_progress_at
                          ? "进行中"
                          : "尚未开始"}
                    </span>
                    {item.queue_state !== "done" && (
                      <button
                        type="button"
                        className={styles.secondaryCommand}
                        disabled={isQueueBusy}
                        onClick={() => void moveItem("done")}
                      >
                        <IconCircleCheck
                          size={19}
                          stroke={1.8}
                          aria-hidden="true"
                        />
                        标记完成
                      </button>
                    )}
                    <div className={styles.detailMove}>
                      <select
                        aria-label="选择目标队列"
                        value={effectiveMoveTarget}
                        disabled={isQueueBusy}
                        onChange={(event) =>
                          setMoveTarget(event.target.value as ConsumptionQueue)
                        }
                      >
                        {queueOptions.map((queue) => (
                          <option key={queue} value={queue}>
                            {QUEUE_PRESENTATION[queue].label}
                          </option>
                        ))}
                      </select>
                      <button
                        type="button"
                        className={styles.iconButton}
                        disabled={isQueueBusy}
                        onClick={() => void moveItem(effectiveMoveTarget)}
                        aria-label={`移动到 ${
                          QUEUE_PRESENTATION[effectiveMoveTarget].label
                        }`}
                        title="移动"
                      >
                        <IconArrowRight
                          size={18}
                          stroke={1.8}
                          aria-hidden="true"
                        />
                      </button>
                    </div>
                    {(isQueueBusy ||
                      externalState === "saving" ||
                      isRefreshing) && (
                      <span className={styles.commandStatus} role="status">
                        {isQueueBusy
                          ? "正在保存队列…"
                          : externalState === "saving"
                            ? "正在记录进行中…"
                            : "正在同步最新状态…"}
                      </span>
                    )}
                  </div>
                </details>
              </div>
            </section>

            {(detailError || externalState === "failed") && (
              <div className={styles.detailNotice} role="alert">
                <span>
                  {externalState === "failed"
                    ? "原节目已打开，但进行中记录未保存。队列没有改变。"
                    : detailError}
                </span>
                {externalState === "failed" && (
                  <button
                    type="button"
                    className={styles.iconButton}
                    onClick={() => void retryInProgress()}
                    aria-label="重试保存进行中记录"
                    title="重试记录"
                  >
                    <IconRefresh size={18} stroke={1.8} aria-hidden="true" />
                  </button>
                )}
              </div>
            )}

            {originalRecovery.plan &&
              originalRecovery.activeKey === item.episode_id && (
                <div className={styles.recoverySlot}>
                  <OriginalEpisodeRecovery
                    copyError={originalRecovery.copyError}
                    onRetry={originalRecovery.retry}
                    onOpenApp={originalRecovery.openApp}
                    onCopy={() => void originalRecovery.copy()}
                    onDismiss={originalRecovery.dismiss}
                  />
                </div>
              )}

            <div
              className={styles.detailTabs}
              role="tablist"
              aria-label="单集详情内容"
            >
              {visibleDetailTabs.map((tab) => (
                <button
                  key={tab.id}
                  ref={(node) => {
                    tabRefs.current[tab.id] = node;
                  }}
                  id={`detail-tab-${tab.id}`}
                  type="button"
                  role="tab"
                  aria-selected={activeTab === tab.id}
                  aria-controls={`detail-panel-${tab.id}`}
                  tabIndex={activeTab === tab.id ? 0 : -1}
                  onClick={() => selectTab(tab.id)}
                  onKeyDown={(event) => handleTabKeyDown(event, tab.id)}
                >
                  {tab.label}
                </button>
              ))}
            </div>

            <section
              id="detail-panel-show-notes"
              className={styles.showNotesSection}
              role="tabpanel"
              aria-labelledby="detail-tab-show-notes"
              hidden={activeTab !== "show-notes"}
              data-copilot-source="show_notes"
              data-copilot-episode-id={item.episode_id}
            >
              {item.show_notes.trim() ? (
                <RichText
                  html={item.show_notes}
                  className={styles.showNotesRichText}
                />
              ) : (
                <p className={styles.showNotesEmpty}>该单集暂无 Show Notes。</p>
              )}
            </section>

            <div
              id="detail-panel-transcript"
              role="tabpanel"
              aria-labelledby="detail-tab-transcript"
              hidden={activeTab !== "transcript"}
            >
              <EpisodeProcessingPanel
                ref={processingPanelRef}
                item={item}
                onHeaderStateChange={setProcessingHeader}
                onViewTranscript={() => selectTab("transcript", true)}
              />
            </div>

            <div
              id="detail-panel-notes"
              role="tabpanel"
              aria-labelledby="detail-tab-notes"
              hidden={activeTab !== "notes"}
            >
              <EpisodeMetadata item={item} onItemChange={onItemChange} />
            </div>
          </div>

          <aside
            id="episode-copilot-workspace"
            className={styles.copilotWorkspace}
            aria-label={
              isMobileViewport ? "移动端单集助手" : "单集助手双栏工作台"
            }
            hidden={!isCopilotOpen}
          >
            <header className={styles.copilotWorkspaceHeader}>
              <div>
                <span className={styles.detailKicker}>EPISODE COPILOT</span>
                <h2 id="episode-copilot-workspace-title">单集助手</h2>
                <p>{item.episode_title}</p>
              </div>
              <button
                ref={copilotReturnRef}
                type="button"
                className={styles.copilotReturn}
                onClick={closeCopilot}
              >
                <IconArrowLeft size={18} stroke={1.8} aria-hidden="true" />
                {isMobileViewport ? "返回单集" : "关闭助手"}
              </button>
            </header>
            <div className={styles.copilotWorkspaceScroll}>
              <EpisodeCopilotPanel item={item} showHeading={false} />
            </div>
          </aside>
        </div>
      </div>
    </div>
  );
}
