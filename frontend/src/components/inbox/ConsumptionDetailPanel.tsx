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
  IconBookmarkPlus,
  IconCheck,
  IconCircleCheck,
  IconChevronDown,
  IconEdit,
  IconExternalLink,
  IconRefresh,
  IconSparkles,
  IconX,
} from "@tabler/icons-react";
import { OriginalEpisodeRecovery } from "@/components/common/OriginalEpisodeRecovery";
import { ShowNotesDocumentView } from "@/components/common/ShowNotesDocumentView";
import { useOriginalEpisodeRecovery } from "@/hooks/useOriginalEpisodeRecovery";
import { episodeApi, tagApi } from "@/lib/api";
import {
  consumptionApi,
  getConsumptionErrorDetails,
} from "@/lib/api/consumption";
import { getErrorMessage } from "@/lib/errorMessage";
import { acquireDocumentScrollLock } from "@/lib/documentScrollLock";
import {
  openOriginalEpisodeTab,
  originalEpisodeAccessText,
  planOriginalEpisodeAccess,
} from "@/lib/originalEpisodeOpen";
import { createEpisodeShowNotesStore } from "@/lib/episodeShowNotesStore";
import type { Tag } from "@/types";
import type { EpisodeCopilotProfileID } from "@/types/episodeCopilot";
import {
  CONSUMPTION_QUEUES,
  type ConsumptionItem,
  type ConsumptionQueue,
} from "@/types/consumption";
import type { ShowNotesDocument } from "@/types/showNotes";
import {
  formatDuration,
  formatPublishedDate,
  QUEUE_PRESENTATION,
} from "./presentation";
import EpisodeCopilotPanel from "./EpisodeCopilotPanel";
import EpisodeProcessingPanel, {
  type EpisodeProcessingHeaderState,
} from "./EpisodeProcessingPanel";
import styles from "./InboxPage.module.css";
import { useMenuPopover } from "./useMenuPopover";

interface ConsumptionDetailPanelProps {
  item: ConsumptionItem;
  isQueueBusy: boolean;
  queueMoveFailure?: string;
  onClose: () => void;
  onItemChange: (item: ConsumptionItem) => void;
  onMove: (
    item: ConsumptionItem,
    target: ConsumptionQueue,
  ) => Promise<ConsumptionItem | undefined>;
  onRetryQueueMove?: () => void;
  onCopilotWorkspaceChange?: (isOpen: boolean) => void;
  selectedCopilotProfileID?: EpisodeCopilotProfileID | null;
  onSelectedCopilotProfileIDChange?: (
    profileID: EpisodeCopilotProfileID,
  ) => void;
  rejectedCopilotProfileIDs?: ReadonlySet<EpisodeCopilotProfileID>;
  onRejectedCopilotProfileID?: (profileID: EpisodeCopilotProfileID) => void;
  onOpenSourceEpisode?: (episodeId: number) => void | Promise<void>;
}

const DETAIL_TABS = [
  { id: "show-notes", label: "Show Notes" },
  { id: "transcript", label: "转写" },
  { id: "notes", label: "笔记" },
] as const;

type DetailTab = (typeof DETAIL_TABS)[number]["id"];

type ShowNotesLoadState =
  | { episodeId: number; status: "loading" }
  | { episodeId: number; status: "success"; document: ShowNotesDocument }
  | { episodeId: number; status: "error"; message: string };

const INITIAL_PROCESSING_HEADER: EpisodeProcessingHeaderState = {
  kind: "loading",
  label: "正在读取",
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

// Replaces the former three-dot menu: the current action queue or status is
// the trigger's own label, and one click on a popup target runs the existing
// queue write. Done remains a separate completion state rather than a fourth
// action queue.
function QueueSwitchMenu({
  item,
  disabled,
  onMove,
}: {
  item: ConsumptionItem;
  disabled: boolean;
  onMove: (
    target: ConsumptionQueue,
  ) => Promise<ConsumptionItem | undefined>;
}) {
  const {
    open,
    menuId,
    triggerRef,
    menuRef,
    closeMenu,
    toggleMenu,
    handleMenuKeyDown,
  } = useMenuPopover();
  const [displayQueue, setDisplayQueue] = useState(item.queue_state);
  const [pendingTarget, setPendingTarget] =
    useState<ConsumptionQueue | null>(null);
  const busy = disabled || pendingTarget !== null;

  useEffect(() => {
    if (!busy) setDisplayQueue(item.queue_state);
  }, [busy, item.queue_state]);

  const currentQueue = displayQueue;
  const isDone = currentQueue === "done";
  const isActionQueue = currentQueue !== null && !isDone;
  const currentLabel = currentQueue
    ? QUEUE_PRESENTATION[currentQueue].label
    : "未收集";
  const triggerContext = isActionQueue ? "当前队列" : "当前状态";
  const showInlineBusy = busy && !open;
  // From Done, every action queue is a reprocess target that keeps the
  // completion record. Inside the action queues, Done is a separate command.
  const targetQueues = CONSUMPTION_QUEUES.filter(
    (queue) => queue !== currentQueue && queue !== "done",
  );

  const move = async (target: ConsumptionQueue) => {
    if (busy) return;
    setPendingTarget(target);
    const updated = await onMove(target);
    if (updated) {
      setDisplayQueue(updated.queue_state);
    }
    closeMenu();
    setPendingTarget(null);
  };

  return (
    <div className={styles.queueMenu}>
      <button
        ref={triggerRef}
        type="button"
        data-queue-switch-trigger
        className={styles.queueMenuTrigger}
        aria-label={
          showInlineBusy
            ? `${triggerContext} ${currentLabel}，正在保存队列`
            : `${triggerContext} ${currentLabel}，打开切换菜单`
        }
        aria-haspopup="menu"
        aria-expanded={open}
        aria-controls={open ? menuId : undefined}
        aria-disabled={busy}
        onClick={() => {
          if (!busy) toggleMenu();
        }}
      >
        {showInlineBusy ? `${currentLabel} · 保存中…` : currentLabel}
        {!showInlineBusy && (
          <IconChevronDown size={15} stroke={1.8} aria-hidden="true" />
        )}
      </button>
      {showInlineBusy && (
        <span
          className={styles.srOnly}
          role="status"
          aria-label="队列保存状态"
        >
          正在保存队列
        </span>
      )}
      {open && (
        <div
          ref={menuRef}
          id={menuId}
          className={styles.queueMenuPopup}
          role="menu"
          aria-label="切换至"
          onKeyDown={handleMenuKeyDown}
        >
          <span className={styles.queueMenuTitle} aria-hidden="true">
            切换至
          </span>
          {targetQueues.map((queue) => (
            <button
              key={queue}
              type="button"
              role="menuitem"
              disabled={busy}
              onClick={() => void move(queue)}
            >
              {QUEUE_PRESENTATION[queue].label}
            </button>
          ))}
          {!isDone && (
            <>
              <div className={styles.queueMenuSeparator} role="separator" />
              <button
                type="button"
                role="menuitem"
                disabled={busy}
                onClick={() => void move("done")}
              >
                <IconCircleCheck size={17} stroke={1.8} aria-hidden="true" />
                标记完成
              </button>
            </>
          )}
          {busy && (
            <span className={styles.commandStatus} role="status">
              正在保存队列…
            </span>
          )}
        </div>
      )}
    </div>
  );
}

export default function ConsumptionDetailPanel({
  item,
  isQueueBusy,
  queueMoveFailure,
  onClose,
  onItemChange,
  onMove,
  onRetryQueueMove,
  onCopilotWorkspaceChange,
  selectedCopilotProfileID,
  onSelectedCopilotProfileIDChange,
  rejectedCopilotProfileIDs,
  onRejectedCopilotProfileID,
  onOpenSourceEpisode,
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
  const detailTabScrollTopRef = useRef<Partial<Record<DetailTab, number>>>({});
  const showNotesRequestSequence = useRef(0);
  const tabRefs = useRef<Record<DetailTab, HTMLButtonElement | null>>({
    "show-notes": null,
    transcript: null,
    notes: null,
  });
  const [isMobileViewport, setIsMobileViewport] = useState(false);
  const [isCopilotOpen, setIsCopilotOpen] = useState(false);
  const [detailError, setDetailError] = useState<string | null>(null);
  const [activeTab, setActiveTab] = useState<DetailTab>("show-notes");
  // One-shot “转写已完成” notice: fires only when this detail session observed
  // the run go from in-progress to completed — never on first load, polling,
  // or tab switches (#314).
  const [completionNotice, setCompletionNotice] = useState(false);
  const processingTurnedActiveRef = useRef(false);
  const [loadedShowNotes, setLoadedShowNotes] = useState<ShowNotesLoadState>({
    episodeId: item.episode_id,
    status: "loading",
  });
  const [processingHeader, setProcessingHeader] =
    useState<EpisodeProcessingHeaderState>(INITIAL_PROCESSING_HEADER);
  const [externalState, setExternalState] = useState<
    "idle" | "saving" | "failed"
  >("idle");
  const originalAccess = useMemo(
    () => planOriginalEpisodeAccess(item.original_url),
    [item.original_url],
  );
  const originalRecovery = useOriginalEpisodeRecovery();
  const showNotesStore = useMemo(
    () => createEpisodeShowNotesStore(episodeApi.getShowNotes),
    [],
  );

  const loadShowNotes = useCallback(async () => {
    const episodeId = item.episode_id;
    const requestSequence = ++showNotesRequestSequence.current;
    setLoadedShowNotes({ episodeId, status: "loading" });
    try {
      const document = await showNotesStore.load(episodeId);
      if (showNotesRequestSequence.current === requestSequence) {
        setLoadedShowNotes({
          episodeId,
          status: "success",
          document,
        });
      }
    } catch (error) {
      if (showNotesRequestSequence.current === requestSequence) {
        setLoadedShowNotes({
          episodeId,
          status: "error",
          message: getErrorMessage(error),
        });
      }
    }
  }, [item.episode_id, showNotesStore]);

  useEffect(() => {
    closeButtonRef.current?.focus();
    return acquireDocumentScrollLock();
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
    detailTabScrollTopRef.current = {};
    setActiveTab("show-notes");
    setProcessingHeader(INITIAL_PROCESSING_HEADER);
    setCompletionNotice(false);
    processingTurnedActiveRef.current = false;
  }, [item.episode_id]);

  useEffect(() => {
    void loadShowNotes();
    return () => {
      showNotesRequestSequence.current += 1;
    };
  }, [loadShowNotes]);

  useEffect(() => {
    let active = true;
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
    if (originalAccess.state !== "openable" || externalState === "saving")
      return;
    setExternalState("saving");
    setDetailError(null);

    const saveIntent = consumptionApi.markInProgress(item.episode_id);
    openOriginalEpisodeTab(originalAccess.openUrl);
    originalRecovery.activate(item.episode_id, originalAccess.plan);

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
    return updated;
  };

  const retryQueueMove = () => {
    panelRef.current
      ?.querySelector<HTMLButtonElement>("[data-queue-switch-trigger]")
      ?.focus({ preventScroll: true });
    onRetryQueueMove?.();
  };

  // One shared control: it sits in the merged nav row on desktop and inside
  // the hero meta area on mobile, so only one focusable copy ever exists
  // (#314).
  const originalEpisodeControl =
    originalAccess.state === "openable" ? (
      <button
        type="button"
        className={styles.originalLink}
        disabled={externalState === "saving"}
        onClick={() => void openOriginal()}
      >
        原节目
        <IconExternalLink size={16} stroke={1.8} aria-hidden="true" />
      </button>
    ) : (
      <span
        className={styles.unsafeOriginal}
        data-original-access={originalAccess.state}
      >
        {originalEpisodeAccessText(originalAccess)}
      </span>
    );

  const queueSwitchMenu = (
    <QueueSwitchMenu item={item} disabled={isQueueBusy} onMove={moveItem} />
  );

  const showNotesState =
    loadedShowNotes.episodeId === item.episode_id
      ? loadedShowNotes
      : ({ episodeId: item.episode_id, status: "loading" } as const);

  const selectTab = useCallback((tab: DetailTab, shouldFocus = false) => {
    const currentScrollTop = detailScrollRef.current?.scrollTop;
    if (currentScrollTop !== undefined) {
      detailTabScrollTopRef.current[activeTab] = currentScrollTop;
    }
    const savedScrollTop = detailTabScrollTopRef.current[tab];
    setActiveTab(tab);
    if (savedScrollTop !== undefined || shouldFocus) {
      window.requestAnimationFrame(() => {
        const detailScroll = detailScrollRef.current;
        if (detailScroll && savedScrollTop !== undefined) {
          detailScroll.scrollTop = savedScrollTop;
        }
        if (shouldFocus) tabRefs.current[tab]?.focus();
      });
    }
  }, [activeTab]);

  // The transcript entry is fixed, so completion is announced once per session
  // instead of steering the active tab.
  useEffect(() => {
    if (processingHeader.kind === "active") {
      processingTurnedActiveRef.current = true;
    } else if (
      processingHeader.kind === "completed" &&
      processingTurnedActiveRef.current
    ) {
      processingTurnedActiveRef.current = false;
      setCompletionNotice(true);
    }
  }, [processingHeader.kind]);

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
    const currentIndex = DETAIL_TABS.findIndex(
      (candidate) => candidate.id === currentTab,
    );
    if (currentIndex < 0) return;
    let nextIndex = currentIndex;
    if (event.key === "ArrowRight") {
      nextIndex = (currentIndex + 1) % DETAIL_TABS.length;
    } else if (event.key === "ArrowLeft") {
      nextIndex = (currentIndex - 1 + DETAIL_TABS.length) % DETAIL_TABS.length;
    } else if (event.key === "Home") {
      nextIndex = 0;
    } else if (event.key === "End") {
      nextIndex = DETAIL_TABS.length - 1;
    } else {
      return;
    }
    event.preventDefault();
    selectTab(DETAIL_TABS[nextIndex].id, true);
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
            data-detail-scroll=""
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

              {isMobileViewport && (
                <div className={styles.detailHeroActions}>
                  {originalEpisodeControl}
                  {queueSwitchMenu}
                </div>
              )}
            </section>

            {queueMoveFailure && (
              <div className={styles.detailNotice} role="alert">
                <span>{queueMoveFailure}</span>
                {onRetryQueueMove && (
                  <button
                    type="button"
                    className={styles.iconButton}
                    onClick={retryQueueMove}
                    aria-label={`重试移动 ${item.episode_title}`}
                    title="重试移动"
                  >
                    <IconRefresh size={18} stroke={1.8} aria-hidden="true" />
                  </button>
                )}
              </div>
            )}

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

            {completionNotice && (
              <div className={styles.completionNotice} role="status">
                <span>转写已完成</span>
                <button
                  type="button"
                  className={styles.iconButton}
                  onClick={() => setCompletionNotice(false)}
                  aria-label="关闭转写完成提示"
                  title="关闭提示"
                >
                  <IconX size={18} stroke={1.8} aria-hidden="true" />
                </button>
              </div>
            )}

            <div className={styles.detailNavBar}>
              <div
                className={styles.detailTabs}
                role="tablist"
                aria-label="单集详情内容"
              >
                {DETAIL_TABS.map((tab) => {
                  const transcriptActive =
                    tab.id === "transcript" &&
                    processingHeader.kind === "active";
                  const transcriptFailed =
                    tab.id === "transcript" &&
                    processingHeader.kind === "failed";
                  const transcriptMarked =
                    transcriptActive || transcriptFailed;
                  return (
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
                      aria-label={
                        transcriptMarked
                          ? `转写，${processingHeader.label}`
                          : undefined
                      }
                      tabIndex={activeTab === tab.id ? 0 : -1}
                      onClick={() => selectTab(tab.id)}
                      onKeyDown={(event) => handleTabKeyDown(event, tab.id)}
                    >
                      {transcriptMarked && (
                        <span
                          className={styles.processingArtifactTabDot}
                          data-state={transcriptFailed ? "failed" : "working"}
                          aria-hidden="true"
                        />
                      )}
                      {tab.label}
                    </button>
                  );
                })}
              </div>

              {!isMobileViewport && (
                <div className={styles.detailNavActions}>
                  {originalEpisodeControl}
                  {queueSwitchMenu}
                </div>
              )}
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
              {showNotesState.status === "loading" && (
                <p className={styles.showNotesEmpty} role="status">
                  正在读取 Show Notes…
                </p>
              )}
              {showNotesState.status === "error" && (
                <div className={styles.showNotesState} role="alert">
                  <p className={styles.showNotesEmpty}>
                    Show Notes 读取失败：{showNotesState.message}
                  </p>
                  <button
                    type="button"
                    className={styles.iconButton}
                    onClick={() => void loadShowNotes()}
                    aria-label="重试读取 Show Notes"
                    title="重试读取"
                  >
                    <IconRefresh size={18} stroke={1.8} aria-hidden="true" />
                  </button>
                </div>
              )}
              {showNotesState.status === "success" && (
                <ShowNotesDocumentView
                  document={showNotesState.document}
                  className={styles.showNotesRichText}
                  emptyFallback={
                    <p className={styles.showNotesEmpty}>
                      该单集暂无 Show Notes。
                    </p>
                  }
                />
              )}
            </section>

            <div
              id="detail-panel-transcript"
              role="tabpanel"
              aria-labelledby="detail-tab-transcript"
              hidden={activeTab !== "transcript"}
            >
              <EpisodeProcessingPanel
                item={item}
                onHeaderStateChange={setProcessingHeader}
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
              <EpisodeCopilotPanel
                item={item}
                selectedProfileID={selectedCopilotProfileID}
                onSelectedProfileIDChange={onSelectedCopilotProfileIDChange}
                rejectedProfileIDs={rejectedCopilotProfileIDs}
                onRejectedProfileID={onRejectedCopilotProfileID}
                onOpenSourceEpisode={onOpenSourceEpisode}
              />
            </div>
          </aside>
        </div>
      </div>
    </div>
  );
}
