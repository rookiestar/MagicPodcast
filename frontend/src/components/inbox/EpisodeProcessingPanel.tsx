"use client";

import {
  forwardRef,
  type KeyboardEvent,
  useCallback,
  useEffect,
  useImperativeHandle,
  useMemo,
  useRef,
  useState,
} from "react";
import { IconDownload, IconPlayerStop, IconRefresh } from "@tabler/icons-react";
import MarkdownViewer from "@/components/workflows/MarkdownViewer";
import { getProcessingErrorDetails, processingApi } from "@/lib/api/processing";
import type { ConsumptionItem } from "@/types/consumption";
import type {
  ArtifactContent,
  ArtifactContentKind,
  AudioRecoverySummary,
  EpisodeAudioAsset,
  KnowledgeDelivery,
  ProcessingRun,
  ProcessingRunDetail,
  ProcessingScheduleStatus,
} from "@/types/processing";
import styles from "./InboxPage.module.css";
import MinutesSummaryView from "./MinutesSummaryView";
import TranscriptAudioPlayer, {
  DEFAULT_TRANSCRIPT_PLAYBACK_RATE,
  type TranscriptPlaybackRate,
} from "./TranscriptAudioPlayer";

type ArtifactTab = "summary" | "minutes" | "transcript";

type ArtifactContentSlot = "minutes" | "transcript";

const artifactTabs: ReadonlyArray<{ id: ArtifactTab; label: string }> = [
  { id: "summary", label: "总结" },
  { id: "minutes", label: "纪要" },
  { id: "transcript", label: "逐字稿" },
];

interface LoadedArtifactContent {
  artifactSetId: number;
  content: ArtifactContent;
}

interface ArtifactContents {
  minutes: LoadedArtifactContent | null;
  transcript: LoadedArtifactContent | null;
}

const emptyArtifactContents: ArtifactContents = {
  minutes: null,
  transcript: null,
};

function artifactContentSlot(tab: ArtifactTab): ArtifactContentSlot {
  return tab === "transcript" ? "transcript" : "minutes";
}

function artifactTabLabel(tab: ArtifactTab) {
  switch (tab) {
    case "summary":
      return "总结";
    case "minutes":
      return "纪要";
    default:
      return "逐字稿";
  }
}

const legacyProcessingPipelineVersion = "focus-processing-v1";
const unresolvedExternalResultCodes = new Set([
  "lark_result_unknown",
  "lark_drive_result_unknown",
  "lark_minutes_result_unknown",
  "external_result_unknown",
  "cancelled_external_result_unknown",
]);

const statusLabels: Record<ProcessingRun["status"], string> = {
  queued: "等待加工",
  running: "加工中",
  waiting_external: "等待飞书转写",
  completed: "已完成",
  failed: "加工失败",
  cancelled: "已取消",
};

const stepLabels: Record<string, string> = {
  audio_prepare: "准备与校验音频",
  transcription: "飞书妙记转写",
  minutes_enrichment: "等待飞书智能纪要",
  episode_notes: "Codex 生成单集纪要",
  artifact_publish: "发布本地产物",
};

const minutesResyncErrorCodes = new Set([
  "minutes_enrichment_timeout",
  "minutes_template_unrecognized",
  "minutes_section_unparsed",
  "minutes_whiteboard_unavailable",
  "minutes_image_unavailable",
  "minutes_note_unreadable",
  "minutes_enrichment_snapshot_write_failed",
  "stored_enrichment_unavailable",
]);

const deliveryStatusLabels: Record<KnowledgeDelivery["status"], string> = {
  pending: "包已生成 / 待人工导入",
  delivering: "交付中",
  delivered: "已交付",
  failed: "交付失败",
  cancelled: "已取消",
};

const scheduleStatusLabels: Record<string, string> = {
  running: "正在选择候选",
  completed: "已完成",
  failed: "本次失败",
};

const scheduleSkipLabels: Record<string, string> = {
  active_run: "已有活动运行",
  current_artifact: "已有当前版本产物",
  not_in_focus: "已移出 Focus",
  audio_not_ready: "没有可用音频",
  episode_not_found: "单集不存在",
  previous_terminal_run: "已有同版本失败或已取消运行，需人工重试",
  start_failed: "启动失败",
  batch_limit: "本批已达上限",
  selection_interrupted: "本次选择未完成",
  selection_interrupted_by_restart: "服务重启前未完成选择",
};

function isActive(run?: ProcessingRun) {
  return (
    run?.status === "queued" ||
    run?.status === "running" ||
    run?.status === "waiting_external"
  );
}

function formatUpdatedAt(value?: string) {
  if (!value) return "—";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "—";
  return date.toLocaleString("zh-CN", { hour12: false });
}

function audioRecoveryState(
  recovery: AudioRecoverySummary | undefined,
): "available" | "queued" | "downloading" | "failed" | "unavailable" {
  if (!recovery?.recoverable) return "unavailable";
  switch (recovery.status) {
    case "queued":
      return "queued";
    case "downloading":
      return "downloading";
    case "failed":
      return "failed";
    default:
      return "available";
  }
}

function audioRecoveryLabel(recovery: AudioRecoverySummary) {
  if (!recovery.recoverable) return "音频暂不可恢复";
  switch (recovery.status) {
    case "queued":
      return "恢复已排队";
    case "downloading":
      return "正在恢复音频";
    case "failed":
      return "音频恢复失败";
    case "completed":
      return "正在确认恢复结果";
    default:
      return "可以恢复音频";
  }
}

function audioRecoveryDetail(recovery: AudioRecoverySummary) {
  if (recovery.error_message) return recovery.error_message;
  switch (recovery.status) {
    case "queued":
      return "已记录恢复请求，后台会继续处理。";
    case "downloading":
      return "后台正在恢复本地播放缓存，逐字稿保持可读。";
    case "completed":
      return "正在重新读取音频可用性，请稍候。";
    default:
      return "来源：受保护的飞书 Drive 原始音频，仅用于恢复本地播放缓存。";
  }
}

export interface EpisodeProcessingHeaderState {
  kind: "loading" | "idle" | "active" | "completed" | "failed";
  label: string;
  detail: string;
  primaryLabel: string;
  primaryDisabled: boolean;
  action: "start" | "reprocess" | "view" | "retry" | "details" | null;
  showTranscriptTab: boolean;
}

export interface EpisodeProcessingPanelHandle {
  activatePrimary: () => void;
}

interface EpisodeProcessingPanelProps {
  item: ConsumptionItem;
  onHeaderStateChange?: (state: EpisodeProcessingHeaderState) => void;
  onViewTranscript?: () => void;
}

const EpisodeProcessingPanel = forwardRef<
  EpisodeProcessingPanelHandle,
  EpisodeProcessingPanelProps
>(function EpisodeProcessingPanel(
  { item, onHeaderStateChange, onViewTranscript },
  ref,
) {
  const [detail, setDetail] = useState<ProcessingRunDetail | null>(null);
  const [hasProcessingHistory, setHasProcessingHistory] = useState(false);
  const [isLoading, setIsLoading] = useState(true);
  const [isMutating, setIsMutating] = useState(false);
  const [isRecoveringAudio, setIsRecoveringAudio] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [artifactReadFailure, setArtifactReadFailure] = useState<
    ArtifactContentKind | null
  >(null);
  const [artifactContents, setArtifactContents] =
    useState<ArtifactContents>(emptyArtifactContents);
  const [activeArtifactTab, setActiveArtifactTab] =
    useState<ArtifactTab>("summary");
  const [transcriptPlaybackRate, setTranscriptPlaybackRate] =
    useState<TranscriptPlaybackRate>(DEFAULT_TRANSCRIPT_PLAYBACK_RATE);

  const [audioAsset, setAudioAsset] = useState<EpisodeAudioAsset | null>(null);
  const [isReadingArtifact, setIsReadingArtifact] = useState(false);
  const [scheduleStatus, setScheduleStatus] =
    useState<ProcessingScheduleStatus | null>(null);
  const [isScheduleLoading, setIsScheduleLoading] = useState(true);
  const [scheduleError, setScheduleError] = useState<string | null>(null);
  const hasLoadedScheduleStatus = useRef(false);
  const scheduleStatusInFlight = useRef(false);
  const loadingEpisodeIDs = useRef(new Set<number>());
  const artifactReadSequence = useRef(0);
  const artifactReadInFlight = useRef<{
    artifactSetId: number;
    kind: ArtifactContentKind;
  } | null>(null);
  const artifactTabRefs = useRef<Record<ArtifactTab, HTMLButtonElement | null>>(
    {
      summary: null,
      minutes: null,
      transcript: null,
    },
  );
  const artifactTabWasUserSelected = useRef(false);
  const activeEpisodeID = useRef(item.episode_id);
  activeEpisodeID.current = item.episode_id;

  const loadScheduleStatus = useCallback(async () => {
    if (scheduleStatusInFlight.current) return;
    scheduleStatusInFlight.current = true;
    const isInitialLoad = !hasLoadedScheduleStatus.current;
    if (isInitialLoad) {
      setIsScheduleLoading(true);
    }
    try {
      setScheduleStatus(await processingApi.getScheduleStatus());
      hasLoadedScheduleStatus.current = true;
      setScheduleError(null);
    } catch (loadError) {
      setScheduleError(
        `定时计划暂时无法读取：${getProcessingErrorDetails(loadError).message}`,
      );
    } finally {
      scheduleStatusInFlight.current = false;
      if (isInitialLoad) {
        setIsScheduleLoading(false);
      }
    }
  }, []);

  const loadLatest = useCallback(async () => {
    const episodeID = item.episode_id;
    if (loadingEpisodeIDs.current.has(episodeID)) return;
    loadingEpisodeIDs.current.add(episodeID);
    const isCurrentEpisode = () => activeEpisodeID.current === episodeID;
    void loadScheduleStatus();
    try {
      const runs = await processingApi.listEpisodeRuns(episodeID);
      if (!isCurrentEpisode()) return;
      if (runs.length === 0) {
        setDetail(null);
        try {
          const latestAudio = await processingApi.getLatestAudio(episodeID);
          if (isCurrentEpisode()) {
            setAudioAsset(latestAudio);
            setHasProcessingHistory(true);
          }
        } catch (audioError) {
          if (getProcessingErrorDetails(audioError).status === 404) {
            if (isCurrentEpisode()) {
              setAudioAsset(null);
              setHasProcessingHistory(false);
            }
          } else {
            throw audioError;
          }
        }
        return;
      }
      setHasProcessingHistory(true);
      const nextDetail = await processingApi.getRun(runs[0].id);
      if (!isCurrentEpisode()) return;
      setDetail(nextDetail);
      if (nextDetail.run.current_step === "audio_prepare") {
        try {
          const latestAudio = await processingApi.getLatestAudio(episodeID);
          if (isCurrentEpisode()) {
            setAudioAsset(latestAudio);
          }
        } catch (audioError) {
          if (getProcessingErrorDetails(audioError).status === 404) {
            if (isCurrentEpisode()) {
              setAudioAsset(null);
            }
          } else {
            throw audioError;
          }
        }
      } else {
        setAudioAsset(null);
      }
    } catch (loadError) {
      if (isCurrentEpisode()) {
        setHasProcessingHistory(true);
        throw loadError;
      }
    } finally {
      loadingEpisodeIDs.current.delete(episodeID);
    }
  }, [item.episode_id, loadScheduleStatus]);

  useEffect(() => {
    let active = true;
    setDetail(null);
    setHasProcessingHistory(false);
    setAudioAsset(null);
    setArtifactReadFailure(null);
    setArtifactContents(emptyArtifactContents);
    setActiveArtifactTab("summary");
    artifactTabWasUserSelected.current = false;
    setTranscriptPlaybackRate(DEFAULT_TRANSCRIPT_PLAYBACK_RATE);
    artifactReadSequence.current += 1;
    artifactReadInFlight.current = null;
    setIsLoading(true);
    setError(null);
    void loadLatest()
      .catch((loadError: unknown) => {
        if (active) {
          setError(
            `加工状态读取失败，单集内容不受影响：${
              getProcessingErrorDetails(loadError).message
            }`,
          );
        }
      })
      .finally(() => {
        if (active) setIsLoading(false);
      });
    return () => {
      active = false;
    };
  }, [loadLatest]);

  useEffect(() => {
    if (!isActive(detail?.run)) return;
    const timer = window.setInterval(() => {
      void loadLatest().catch((pollError: unknown) => {
        setError(
          `加工状态暂时无法刷新：${
            getProcessingErrorDetails(pollError).message
          }`,
        );
      });
    }, 4000);
    return () => window.clearInterval(timer);
  }, [detail?.run, loadLatest]);

  useEffect(() => {
    if (isActive(detail?.run)) return;
    if (
      audioAsset?.status !== "queued" &&
      audioAsset?.status !== "downloading"
    ) {
      return;
    }
    const timer = window.setInterval(() => {
      void loadLatest().catch((pollError: unknown) => {
        setError(
          `音频准备状态暂时无法刷新：${
            getProcessingErrorDetails(pollError).message
          }`,
        );
      });
    }, 4000);
    return () => window.clearInterval(timer);
  }, [audioAsset?.status, detail?.run, loadLatest]);

  const mutateRun = async (operation: () => Promise<ProcessingRun>) => {
    if (isMutating) return;
    setIsMutating(true);
    setError(null);
    try {
      const result = await operation();
      setDetail(await processingApi.getRun(result.id));
    } catch (operationError) {
      const failure = getProcessingErrorDetails(operationError);
      setError(`${failure.code ? `${failure.code}：` : ""}${failure.message}`);
    } finally {
      setIsMutating(false);
    }
  };

  const startProcessing = useCallback(async () => {
    if (isMutating) return;
    setHasProcessingHistory(true);
    setIsMutating(true);
    setError(null);
    try {
      const result = await processingApi.start(item.episode_id);
      if (result.run) {
        setAudioAsset(result.audio_asset ?? null);
        setDetail(await processingApi.getRun(result.run.id));
      } else if (result.preparing_audio && result.audio_asset) {
        setDetail(null);
        setAudioAsset(result.audio_asset);
      } else {
        throw new Error("加工请求未返回可追踪状态");
      }
    } catch (operationError) {
      const failure = getProcessingErrorDetails(operationError);
      setError(`${failure.code ? `${failure.code}：` : ""}${failure.message}`);
    } finally {
      setIsMutating(false);
    }
  }, [isMutating, item.episode_id]);

  const readArtifact = useCallback(
    async (artifactSetId: number, kind: ArtifactContentKind) => {
      if (
        artifactReadInFlight.current?.artifactSetId === artifactSetId &&
        artifactReadInFlight.current.kind === kind
      ) {
        return;
      }
      const sequence = artifactReadSequence.current + 1;
      artifactReadSequence.current = sequence;
      artifactReadInFlight.current = { artifactSetId, kind };
      setIsReadingArtifact(true);
      setError(null);
      try {
        const content = await processingApi.getArtifactContent(
          artifactSetId,
          kind,
        );
        if (artifactReadSequence.current !== sequence) return;
        setArtifactReadFailure(null);
        const slot = artifactContentSlot(
          content.kind === "transcript" ? "transcript" : "minutes",
        );
        setArtifactContents((current) => ({
          ...current,
          [slot]: {
            artifactSetId,
            content,
          },
        }));
      } catch (readError) {
        if (artifactReadSequence.current !== sequence) return;
        setArtifactReadFailure(kind);
        setError(
          `产物读取失败：${getProcessingErrorDetails(readError).message}`,
        );
      } finally {
        if (artifactReadSequence.current === sequence) {
          setIsReadingArtifact(false);
        }
        if (
          artifactReadInFlight.current?.artifactSetId === artifactSetId &&
          artifactReadInFlight.current.kind === kind
        ) {
          artifactReadInFlight.current = null;
        }
      }
    },
    [],
  );

  const run = detail?.run;
  const currentArtifact = detail?.current_artifact;
  const requestAudioRecovery = useCallback(async () => {
    if (isRecoveringAudio || !currentArtifact) return;
    setIsRecoveringAudio(true);
    setError(null);
    try {
      const result = await processingApi.recoverAudio(currentArtifact.id);
      setArtifactContents((current) => {
        const transcript = current.transcript;
        if (!transcript || transcript.artifactSetId !== currentArtifact.id) {
          return current;
        }
        return {
          ...current,
          transcript: {
            ...transcript,
            content: {
              ...transcript.content,
              audio_recovery: result.audio_recovery,
            },
          },
        };
      });
      // The POST only acknowledges durable state. media_available becomes
      // true only after this read observes the backend's verified result.
      await readArtifact(currentArtifact.id, "transcript");
    } catch (recoveryError) {
      const failure = getProcessingErrorDetails(recoveryError);
      setError(`${failure.code ? `${failure.code}：` : ""}${failure.message}`);
    } finally {
      setIsRecoveringAudio(false);
    }
  }, [currentArtifact, isRecoveringAudio, readArtifact]);

  const summaryKind: ArtifactContentKind | null = currentArtifact?.capabilities
    .minutes_summary
    ? "minutes_summary"
    : currentArtifact?.capabilities.legacy_episode_notes
      ? "episode_notes"
      : null;
  const transcriptAvailable = currentArtifact?.capabilities.transcript === true;
  const minutesArtifactContent = artifactContents.minutes;
  const minutesContentMatchesCurrent = Boolean(
    currentArtifact &&
      minutesArtifactContent?.artifactSetId === currentArtifact.id &&
      minutesArtifactContent.content.kind === summaryKind,
  );
  const visualSummaryAvailable = Boolean(
    minutesContentMatchesCurrent &&
      (minutesArtifactContent?.content.whiteboard ||
        minutesArtifactContent?.content.visual_items?.some(
          (item) => item.type === "whiteboard",
        )),
  );
  const summaryContentSettled =
    summaryKind !== "minutes_summary" ||
    minutesContentMatchesCurrent ||
    artifactReadFailure === summaryKind;
  const requestedArtifactKind: ArtifactContentKind | null = !currentArtifact
    ? null
    : activeArtifactTab === "summary" || activeArtifactTab === "minutes"
      ? summaryKind
      : transcriptAvailable
        ? "transcript"
        : null;
  const selectedArtifactContent =
    artifactContents[artifactContentSlot(activeArtifactTab)];
  const artifactContentMatchesSelection =
    currentArtifact !== undefined &&
    requestedArtifactKind !== null &&
    selectedArtifactContent?.artifactSetId === currentArtifact.id &&
    selectedArtifactContent.content.kind === requestedArtifactKind;
  const latestScheduleRun = scheduleStatus?.latest_run;
  const latestScheduleItem = latestScheduleRun?.items.find(
    (scheduleItem) => scheduleItem.episode_id === item.episode_id,
  );
  const latestScheduleItemPending =
    latestScheduleRun?.run.status === "running" &&
    latestScheduleItem?.reason === "selection_pending";
  const canStart = item.queue_state === "focus" && !run;
  const externalResultUnknown =
    detail?.external_result_unresolved ??
    (typeof run?.error_code === "string" &&
      unresolvedExternalResultCodes.has(run.error_code.trim().toLowerCase()));
  const canReprocessLegacy =
    item.queue_state === "focus" &&
    run?.pipeline_version === legacyProcessingPipelineVersion &&
    (run.status === "completed" ||
      run.status === "failed" ||
      run.status === "cancelled") &&
    !externalResultUnknown;
  const audioPreparing =
    run?.current_step === "audio_prepare" ||
    audioAsset?.status === "queued" ||
    audioAsset?.status === "downloading";
  const canRetry =
    item.queue_state === "focus" &&
    (run?.status === "failed" || run?.status === "cancelled") &&
    !canReprocessLegacy &&
    !externalResultUnknown;
  const scheduleSummary = !scheduleStatus
    ? isScheduleLoading
      ? "正在读取…"
      : "定时状态暂时不可用"
    : scheduleStatus.enabled
      ? `已启用 · 每批 ${scheduleStatus.batch_size} 集`
      : "未启用";
  const processingStatusLabel =
    isLoading && !run && !audioAsset
      ? "正在读取"
      : run
        ? run.current_step === "audio_prepare"
          ? audioAsset?.status === "downloading"
            ? "正在准备音频"
            : "等待准备音频"
          : statusLabels[run.status]
        : audioAsset?.status === "queued"
          ? "等待准备音频"
          : audioAsset?.status === "downloading"
            ? "正在准备音频"
            : audioAsset?.status === "ready"
              ? "音频已就绪"
              : audioAsset?.status === "failed"
                ? "音频准备失败"
                : "尚未加工";
  const showTranscriptTab =
    hasProcessingHistory ||
    Boolean(run) ||
    Boolean(audioAsset) ||
    Boolean(currentArtifact);
  const showArtifactSkeleton =
    !selectedArtifactContent &&
    ((isLoading && !currentArtifact) ||
      (Boolean(currentArtifact) && isReadingArtifact));
  const artifactReadFailed =
    typeof error === "string" && error.startsWith("产物读取失败");
  const canRetryArtifactRead =
    artifactReadFailed &&
    currentArtifact !== undefined &&
    requestedArtifactKind !== null;

  const transcriptRecovery =
    selectedArtifactContent?.content.kind === "transcript"
      ? selectedArtifactContent.content.audio_recovery
      : undefined;
  const transcriptMediaAvailable =
    selectedArtifactContent?.content.kind === "transcript" &&
    selectedArtifactContent.content.media_available;
  const audioRecoveryStateValue = audioRecoveryState(transcriptRecovery);
  const audioRecoveryInFlight =
    transcriptRecovery?.status === "queued" ||
    transcriptRecovery?.status === "downloading";
  const recoveryArtifactId = currentArtifact?.id;
  const canRequestAudioRecovery =
    !transcriptMediaAvailable &&
    transcriptRecovery?.recoverable === true &&
    !audioRecoveryInFlight &&
    transcriptRecovery?.status !== "completed" &&
    (transcriptRecovery?.status !== "failed" || transcriptRecovery.can_retry) &&
    !isRecoveringAudio;

  useEffect(() => {
    if (
      !recoveryArtifactId ||
      (transcriptRecovery?.status !== "queued" &&
        transcriptRecovery?.status !== "downloading")
    ) {
      return;
    }
    const timer = window.setInterval(() => {
      void readArtifact(recoveryArtifactId, "transcript");
    }, 4000);
    return () => window.clearInterval(timer);
  }, [readArtifact, recoveryArtifactId, transcriptRecovery?.status]);

  const retryCurrentRead = useCallback(async () => {
    if (canRetryArtifactRead) {
      await readArtifact(currentArtifact.id, requestedArtifactKind);
      return;
    }
    setIsLoading(true);
    setError(null);
    try {
      await loadLatest();
      setError(null);
    } catch (loadError) {
      setError(
        `加工状态读取失败，单集内容不受影响：${
          getProcessingErrorDetails(loadError).message
        }`,
      );
    } finally {
      setIsLoading(false);
    }
  }, [
    canRetryArtifactRead,
    currentArtifact,
    loadLatest,
    readArtifact,
    requestedArtifactKind,
  ]);

  const isArtifactTabAvailable = useCallback(
    (tab: ArtifactTab) =>
      tab === "summary"
        ? visualSummaryAvailable
        : tab === "minutes"
          ? Boolean(summaryKind)
          : transcriptAvailable,
    [summaryKind, transcriptAvailable, visualSummaryAvailable],
  );

  const handleArtifactTabKeyDown = (
    event: KeyboardEvent<HTMLButtonElement>,
    currentTab: ArtifactTab,
  ) => {
    const enabledTabs = artifactTabs.filter((tab) =>
      isArtifactTabAvailable(tab.id),
    );
    if (enabledTabs.length === 0) return;
    const currentIndex = enabledTabs.findIndex((tab) => tab.id === currentTab);
    let nextIndex = currentIndex < 0 ? 0 : currentIndex;
    if (event.key === "ArrowRight" || event.key === "ArrowDown") {
      nextIndex = (nextIndex + 1) % enabledTabs.length;
    } else if (event.key === "ArrowLeft" || event.key === "ArrowUp") {
      nextIndex = (nextIndex - 1 + enabledTabs.length) % enabledTabs.length;
    } else if (event.key === "Home") {
      nextIndex = 0;
    } else if (event.key === "End") {
      nextIndex = enabledTabs.length - 1;
    } else {
      return;
    }
    event.preventDefault();
    const nextTab = enabledTabs[nextIndex].id;
    artifactTabWasUserSelected.current = true;
    setActiveArtifactTab(nextTab);
    artifactTabRefs.current[nextTab]?.focus();
  };

  useEffect(() => {
    if (!currentArtifact) {
      setArtifactContents(emptyArtifactContents);
      return;
    }
    if (!requestedArtifactKind) {
      const fallback: ArtifactTab | null = summaryKind
        ? "minutes"
        : transcriptAvailable
          ? "transcript"
          : null;
      if (fallback && fallback !== activeArtifactTab) {
        setActiveArtifactTab(fallback);
      } else {
        setArtifactContents(emptyArtifactContents);
      }
      return;
    }
    if (artifactContentMatchesSelection) {
      return;
    }
    void readArtifact(currentArtifact.id, requestedArtifactKind);
  }, [
    activeArtifactTab,
    artifactContentMatchesSelection,
    currentArtifact,
    readArtifact,
    requestedArtifactKind,
    summaryKind,
    transcriptAvailable,
  ]);

  const isMinutesResyncFailure = Boolean(
    run?.status === "failed" &&
    minutesResyncErrorCodes.has(run.error_code ?? ""),
  );

  const retryProcessing = useCallback(async () => {
    if (isMutating || !run) return;
    setIsMutating(true);
    setError(null);
    try {
      const result = await processingApi.retry(run.id);
      if (!result.run) {
        throw new Error("重试请求未返回加工运行");
      }
      setAudioAsset(result.audio_asset ?? null);
      setDetail(await processingApi.getRun(result.run.id));
    } catch (retryError) {
      const failure = getProcessingErrorDetails(retryError);
      setError(`${failure.code ? `${failure.code}：` : ""}${failure.message}`);
    } finally {
      setIsMutating(false);
    }
  }, [isMutating, run]);

  const headerState = useMemo<EpisodeProcessingHeaderState>(() => {
    if (isLoading && !run && !audioAsset) {
      return {
        kind: "loading",
        label: "正在读取",
        detail: "Show Notes 可继续阅读",
        primaryLabel: "读取转写状态",
        primaryDisabled: true,
        action: null,
        showTranscriptTab,
      };
    }
    if (audioPreparing || isActive(run)) {
      return {
        kind: "active",
        label:
          run?.current_step === "minutes_enrichment"
            ? "等待智能纪要"
            : run?.status === "waiting_external"
              ? "等待妙记"
              : audioPreparing
                ? "准备音频"
                : "转写中",
        detail: "转写在后台继续，当前正文不受影响",
        primaryLabel: "转写中",
        primaryDisabled: true,
        action: null,
        showTranscriptTab,
      };
    }
    if (run?.status === "failed" || run?.status === "cancelled") {
      return {
        kind: "failed",
        label:
          run.status === "cancelled"
            ? "已取消"
            : isMinutesResyncFailure
              ? "智能纪要同步失败"
              : "转写失败",
        detail:
          detail?.action_suggestion ||
          run.error_message ||
          "可查看原因后安全重试",
        primaryLabel: canReprocessLegacy
          ? "重新转写"
          : canRetry
            ? minutesResyncErrorCodes.has(run.error_code ?? "")
              ? "重新同步"
              : "重试转写"
            : "查看详情",
        primaryDisabled: isMutating,
        action: canReprocessLegacy
          ? "reprocess"
          : canRetry
            ? "retry"
            : "details",
        showTranscriptTab,
      };
    }
    if (currentArtifact) {
      return {
        kind: "completed",
        label: "已完成",
        detail: canReprocessLegacy
          ? "当前为旧版产物，可升级为妙记纪要与同步逐字稿"
          : "已有可阅读的转写产物",
        primaryLabel: canReprocessLegacy ? "重新转写" : "查看转写",
        primaryDisabled: isMutating,
        action: canReprocessLegacy ? "reprocess" : "view",
        showTranscriptTab,
      };
    }
    return {
      kind: "idle",
      label: "未转写",
      detail: item.queue_state === "focus" ? "" : "加入 Focus 后可开始转写",
      primaryLabel:
        item.queue_state === "focus" ? "开始转写" : "加入 Focus 后转写",
      primaryDisabled: !canStart || isMutating,
      action: canStart ? "start" : null,
      showTranscriptTab,
    };
  }, [
    audioAsset,
    audioPreparing,
    canReprocessLegacy,
    canRetry,
    canStart,
    currentArtifact,
    detail?.action_suggestion,
    isLoading,
    isMutating,
    item.queue_state,
    isMinutesResyncFailure,
    run,
    showTranscriptTab,
  ]);

  useEffect(() => {
    onHeaderStateChange?.(headerState);
  }, [headerState, onHeaderStateChange]);

  useImperativeHandle(
    ref,
    () => ({
      activatePrimary: () => {
        if (headerState.primaryDisabled) return;
        if (
          headerState.action === "start" ||
          headerState.action === "reprocess"
        ) {
          void startProcessing();
          return;
        }
        if (headerState.action === "retry") {
          void retryProcessing();
          return;
        }
        if (headerState.action === "view" || headerState.action === "details") {
          onViewTranscript?.();
        }
      },
    }),
    [headerState, onViewTranscript, retryProcessing, startProcessing],
  );

  const availableArtifactTabs = useMemo(
    () => artifactTabs.filter((tab) => isArtifactTabAvailable(tab.id)),
    [isArtifactTabAvailable],
  );
  const defaultArtifactTab: ArtifactTab | null = summaryKind
    ? "minutes"
    : transcriptAvailable
      ? "transcript"
      : null;
  const renderedArtifactTab = availableArtifactTabs.some(
    (tab) => tab.id === activeArtifactTab,
  )
    ? activeArtifactTab
    : defaultArtifactTab;

  useEffect(() => {
    if (!currentArtifact || !summaryContentSettled || !renderedArtifactTab) {
      return;
    }
    if (
      visualSummaryAvailable &&
      !artifactTabWasUserSelected.current &&
      activeArtifactTab === "minutes"
    ) {
      setActiveArtifactTab("summary");
      return;
    }
    if (renderedArtifactTab !== activeArtifactTab) {
      setActiveArtifactTab(renderedArtifactTab);
    }
  }, [
    activeArtifactTab,
    currentArtifact,
    renderedArtifactTab,
    summaryContentSettled,
    visualSummaryAvailable,
  ]);

  const showRunDetails = Boolean(
    run ||
    audioAsset ||
    detail?.deliveries.length ||
    scheduleStatus?.enabled ||
    latestScheduleRun ||
    scheduleError,
  );
  const processingStateKind =
    error && !run && !audioAsset
      ? "failed"
      : audioPreparing || isActive(run) || isMutating
        ? "active"
        : run?.status === "failed" || run?.status === "cancelled"
          ? "failed"
          : run?.status === "completed"
            ? "completed"
            : "idle";
  const processingStateTitle = isMutating
    ? run?.status === "failed" || run?.status === "cancelled"
      ? isMinutesResyncFailure
        ? "正在重新同步智能纪要"
        : "正在重试转写"
      : "正在发起转写"
    : isLoading
      ? "正在读取转写内容"
      : error && !run && !audioAsset
        ? "转写信息暂时不可用"
        : run?.current_step === "minutes_enrichment"
          ? "等待飞书智能纪要"
          : run?.status === "waiting_external"
            ? "飞书妙记转写中"
            : audioPreparing
              ? "正在准备音频"
              : isActive(run)
                ? "转写进行中"
                : run?.status === "failed"
                  ? isMinutesResyncFailure
                    ? "智能纪要同步失败"
                    : "转写失败"
                  : run?.status === "cancelled"
                    ? "转写已取消"
                    : run?.status === "completed"
                      ? "转写已完成"
                      : audioAsset?.status === "failed"
                        ? "音频准备失败"
                        : audioAsset?.status === "ready"
                          ? "音频已就绪"
                          : "暂无转写记录";
  const processingStateDescription = isMutating
    ? isMinutesResyncFailure
      ? "正在续取同一条妙记，不会重新上传音频或创建妙记。"
      : "正在创建飞书妙记任务…"
    : isLoading
      ? "正在同步最近一次运行。"
      : error && !run && !audioAsset
        ? "请重试读取，Show Notes 与笔记不受影响。"
        : run?.current_step === "minutes_enrichment"
          ? "核心转写已就绪，正在只读等待飞书智能纪要完整。"
          : run?.status === "waiting_external"
            ? "飞书妙记正在生成纪要与逐字稿。"
            : audioPreparing
              ? "音频就绪后会自动提交飞书妙记。"
              : isActive(run)
                ? "任务在后台继续，可随时返回查看。"
                : run?.status === "failed" || run?.status === "cancelled"
                  ? detail?.action_suggestion ||
                    run.error_message ||
                    "可从页面顶部重新发起。"
                  : run?.status === "completed"
                    ? "暂未发现可阅读的转写产物。"
                    : audioAsset?.status === "failed"
                      ? audioAsset.error_message || "请检查音频来源后重试。"
                      : audioAsset?.status === "ready"
                        ? "正在等待创建转写任务。"
                        : "开始转写后，纪要与逐字稿会显示在这里。";

  const artifactPanelContent = (
    <>
      {currentArtifact && (
        <div className={styles.processingArtifactMeta}>
          <span>
            {run?.id === currentArtifact.run_id ? "当前版本" : "上一成功版本"}
          </span>
          <time dateTime={currentArtifact.created_at}>
            更新于 {formatUpdatedAt(currentArtifact.created_at)}
          </time>
          {currentArtifact.capabilities.legacy_episode_notes && (
            <span className={styles.processingHint}>
              这是旧版纪要；重新转写后可获得妙记纪要和同步逐字稿。
            </span>
          )}
        </div>
      )}

      {currentArtifact &&
        !artifactContentMatchesSelection &&
        isReadingArtifact &&
        selectedArtifactContent && (
          <div className={styles.processingHint} role="status">
            正在读取{artifactTabLabel(renderedArtifactTab ?? activeArtifactTab)}
            ，暂时显示上一成功内容…
          </div>
        )}

      {showArtifactSkeleton && (
        <div className={styles.processingDocumentSkeleton} role="status">
          <span>
            {isLoading && !currentArtifact
              ? "正在读取转写状态…"
              : `正在读取${artifactTabLabel(
                  renderedArtifactTab ?? activeArtifactTab,
                )}…`}
          </span>
          <div aria-hidden="true">
            <i />
            <i />
            <i />
          </div>
        </div>
      )}

      {currentArtifact &&
        !selectedArtifactContent &&
        !showArtifactSkeleton &&
        artifactReadFailed && (
          <div
            className={`${styles.processingEmpty} ${styles.processingEmptyFailure}`}
            role="status"
          >
            暂时无法读取
            {artifactTabLabel(renderedArtifactTab ?? activeArtifactTab)}，请重试。
          </div>
        )}

      {currentArtifact && selectedArtifactContent && (
        <div
          className={`${styles.processingDocument} ${
            selectedArtifactContent.content.kind === "transcript" &&
            selectedArtifactContent.content.segments?.length
              ? styles.processingTranscriptDocument
              : styles.processingMinutesDocument
          }`}
          data-copilot-source={
            selectedArtifactContent.content.kind === "transcript"
              ? "transcript"
              : undefined
          }
          data-copilot-episode-id={
            selectedArtifactContent.content.kind === "transcript"
              ? item.episode_id
              : undefined
          }
        >
          <div className={styles.metadataLabelRow}>
            <span>
              {selectedArtifactContent.content.kind === "transcript"
                ? `逐字稿${
                    selectedArtifactContent.content.segments?.length
                      ? ` · ${selectedArtifactContent.content.segments.length} 段`
                      : ""
                  }`
                : selectedArtifactContent.content.kind === "minutes_summary"
                  ? "飞书智能纪要"
                  : "旧版纪要"}
            </span>
            {selectedArtifactContent.content.kind === "transcript" && (
              <span>
                {selectedArtifactContent.content.media_available
                  ? "音频可用"
                  : "音频不可用"}
              </span>
            )}
          </div>
          {selectedArtifactContent.content.kind === "transcript" &&
            transcriptRecovery &&
            !transcriptMediaAvailable &&
            (transcriptRecovery.recoverable ||
              Boolean(transcriptRecovery.error_message)) && (
              <div
                className={styles.audioRecovery}
                data-state={audioRecoveryStateValue}
                role={
                  audioRecoveryInFlight || isRecoveringAudio
                    ? "status"
                    : transcriptRecovery.error_message
                      ? "alert"
                      : undefined
                }
                aria-live="polite"
              >
                <div className={styles.audioRecoveryCopy}>
                  <strong>{audioRecoveryLabel(transcriptRecovery)}</strong>
                  <span>{audioRecoveryDetail(transcriptRecovery)}</span>
                  {transcriptRecovery.recoverable &&
                    !transcriptRecovery.error_message && (
                      <small>
                        远端只用于修复本地音频，不会改变逐字稿或重新加工。
                      </small>
                    )}
                </div>
                {transcriptRecovery.recoverable &&
                  transcriptRecovery.status !== "completed" &&
                  (transcriptRecovery.status !== "failed" ||
                    transcriptRecovery.can_retry) && (
                    <button
                      type="button"
                      className={styles.secondaryCommand}
                      disabled={
                        !canRequestAudioRecovery || audioRecoveryInFlight
                      }
                      onClick={() => void requestAudioRecovery()}
                    >
                      {transcriptRecovery.status === "failed" ? (
                        <IconRefresh
                          size={17}
                          stroke={1.8}
                          aria-hidden="true"
                        />
                      ) : (
                        <IconDownload
                          size={17}
                          stroke={1.8}
                          aria-hidden="true"
                        />
                      )}
                      {isRecoveringAudio
                        ? "正在提交…"
                        : transcriptRecovery.status === "queued"
                          ? "已排队"
                          : transcriptRecovery.status === "downloading"
                            ? "恢复中"
                            : transcriptRecovery.status === "failed"
                              ? "重试恢复"
                              : "恢复音频"}
                    </button>
                  )}
              </div>
            )}
          {selectedArtifactContent.content.kind === "transcript" &&
          selectedArtifactContent.content.segments?.length ? (
            <TranscriptAudioPlayer
              artifactSetId={selectedArtifactContent.artifactSetId}
              segments={selectedArtifactContent.content.segments}
              mediaAvailable={selectedArtifactContent.content.media_available}
              playbackRate={transcriptPlaybackRate}
              onPlaybackRateChange={setTranscriptPlaybackRate}
              chapters={selectedArtifactContent.content.chapters}
            />
          ) : selectedArtifactContent.content.kind === "minutes_summary" ? (
            <MinutesSummaryView
              artifactSetId={selectedArtifactContent.artifactSetId}
              content={selectedArtifactContent.content.content}
              keywords={selectedArtifactContent.content.keywords}
              decisions={selectedArtifactContent.content.decisions}
              quotes={selectedArtifactContent.content.quotes}
              links={selectedArtifactContent.content.links}
              whiteboard={selectedArtifactContent.content.whiteboard}
              visualItems={selectedArtifactContent.content.visual_items}
              inlineImages={selectedArtifactContent.content.inline_images}
              mode={renderedArtifactTab === "summary" ? "visual" : "minutes"}
            />
          ) : (
            <MarkdownViewer content={selectedArtifactContent.content.content} />
          )}
        </div>
      )}
    </>
  );

  return (
    <section className={styles.processingSection} aria-label="转写内容">
      {error && (
        <div className={styles.inlineError} role="alert">
          <span>{error}</span>
          <button
            type="button"
            className={styles.iconButton}
            disabled={isLoading || isReadingArtifact}
            onClick={() => void retryCurrentRead()}
            aria-label={
              canRetryArtifactRead
                ? `重试读取${
                    artifactTabLabel(renderedArtifactTab ?? activeArtifactTab)
                  }`
                : "重试读取加工状态"
            }
            title="重试"
          >
            <IconRefresh size={18} stroke={1.8} aria-hidden="true" />
          </button>
        </div>
      )}

      {currentArtifact && isActive(run) && (
        <div className={styles.processingRunNotice} role="status">
          <span className={styles.processingStateDot} aria-hidden="true" />
          <span>
            <strong>{processingStateTitle}</strong>
            <small>
              {run.current_step === "minutes_enrichment"
                ? "正在等待飞书智能纪要完整后发布，当前仍可阅读上一成功版本。"
                : "当前仍可阅读上一成功版本。"}
            </small>
          </span>
          <button
            type="button"
            className={styles.secondaryCommand}
            disabled={isMutating}
            onClick={() => void mutateRun(() => processingApi.cancel(run.id))}
          >
            <IconPlayerStop size={18} stroke={1.8} aria-hidden="true" />
            取消
          </button>
        </div>
      )}

      {currentArtifact &&
        (run?.status === "failed" || run?.status === "cancelled") && (
          <div className={styles.processingRunNotice} role="status">
            <span className={styles.processingStateDot} aria-hidden="true" />
            <span>
              <strong>
                {isMutating && isMinutesResyncFailure
                  ? "正在重新同步智能纪要"
                  : run.status === "cancelled"
                    ? "转写已取消"
                    : isMinutesResyncFailure
                      ? "智能纪要同步失败"
                      : "转写失败"}
              </strong>
              <small>
                {isMutating && isMinutesResyncFailure
                  ? "正在续取同一条妙记，不会重新上传音频或创建妙记。"
                  : detail?.action_suggestion || "上一成功版本仍可阅读。"}
              </small>
            </span>
            {canRetry && minutesResyncErrorCodes.has(run.error_code ?? "") && (
              <button
                type="button"
                className={styles.secondaryCommand}
                disabled={isMutating}
                onClick={() => void retryProcessing()}
              >
                <IconRefresh size={18} stroke={1.8} aria-hidden="true" />
                {isMutating ? "正在同步…" : "重新同步"}
              </button>
            )}
          </div>
        )}

      {!currentArtifact && (
        <div
          className={styles.processingStateCard}
          data-state={processingStateKind}
          role={isLoading || isActive(run) || isMutating ? "status" : undefined}
        >
          <span className={styles.processingStateDot} aria-hidden="true" />
          <div className={styles.processingStateCopy}>
            <strong>{processingStateTitle}</strong>
            <p>{processingStateDescription}</p>
          </div>
          {isActive(run) && (
            <button
              type="button"
              className={styles.secondaryCommand}
              disabled={isMutating}
              onClick={() =>
                void mutateRun(() => processingApi.cancel(run!.id))
              }
            >
              <IconPlayerStop size={18} stroke={1.8} aria-hidden="true" />
              取消
            </button>
          )}
        </div>
      )}

      {currentArtifact && (
        <div className={styles.processingSummary}>
          <div
            className={styles.processingArtifactTabs}
            role="tablist"
            aria-label="转写产物"
          >
            {availableArtifactTabs.map((tab) => {
              const selected = renderedArtifactTab === tab.id;
              return (
                <button
                  key={tab.id}
                  ref={(node) => {
                    artifactTabRefs.current[tab.id] = node;
                  }}
                  id={`processing-artifact-tab-${tab.id}`}
                  type="button"
                  role="tab"
                  aria-selected={selected}
                  aria-controls={`processing-artifact-panel-${tab.id}`}
                  tabIndex={selected ? 0 : -1}
                  onClick={() => {
                    artifactTabWasUserSelected.current = true;
                    setActiveArtifactTab(tab.id);
                  }}
                  onKeyDown={(event) => handleArtifactTabKeyDown(event, tab.id)}
                >
                  {tab.label}
                </button>
              );
            })}
          </div>

          {availableArtifactTabs.map((tab) => (
            <div
              key={tab.id}
              id={`processing-artifact-panel-${tab.id}`}
              className={styles.processingArtifactPanel}
              role="tabpanel"
              aria-labelledby={`processing-artifact-tab-${tab.id}`}
              aria-busy={renderedArtifactTab === tab.id && showArtifactSkeleton}
              tabIndex={0}
              hidden={renderedArtifactTab !== tab.id}
            >
              {renderedArtifactTab === tab.id && artifactPanelContent}
            </div>
          ))}
        </div>
      )}

      {showRunDetails && (
        <details className={styles.processingDiagnostics}>
          <summary>
            <span>运行详情</span>
            <span>{processingStatusLabel}</span>
          </summary>
          <div className={styles.processingDiagnosticsBody}>
            <dl>
              <div>
                <dt>状态</dt>
                <dd>{processingStatusLabel}</dd>
              </div>
              <div>
                <dt>当前步骤</dt>
                <dd>
                  {run?.current_step
                    ? stepLabels[run.current_step] || run.current_step
                    : "—"}
                </dd>
              </div>
              <div>
                <dt>更新时间</dt>
                <dd>
                  {formatUpdatedAt(run?.updated_at || audioAsset?.updated_at)}
                </dd>
              </div>
              {(scheduleStatus?.enabled ||
                scheduleError ||
                latestScheduleRun) && (
                <div>
                  <dt>定时计划</dt>
                  <dd>{scheduleSummary}</dd>
                </div>
              )}
              {scheduleStatus?.enabled && (
                <div>
                  <dt>下次计划</dt>
                  <dd>{formatUpdatedAt(scheduleStatus.next_run_at)}</dd>
                </div>
              )}
            </dl>

            {scheduleError && (
              <div className={styles.processingHint} role="status">
                {scheduleError}
              </div>
            )}
            {scheduleStatus?.enabled && (
              <div className={styles.processingHint}>
                <strong>定时配置</strong>
                <span>
                  cron：{scheduleStatus.cron} · 时区：{scheduleStatus.timezone}{" "}
                  · 每批 {scheduleStatus.batch_size} 集
                </span>
                <span>修改主机配置并重启服务后生效。</span>
              </div>
            )}
            {latestScheduleRun && (
              <div className={styles.processingHint}>
                <strong>
                  最近定时：
                  {scheduleStatusLabels[latestScheduleRun.run.status] ||
                    latestScheduleRun.run.status}
                </strong>
                <span>
                  {formatUpdatedAt(latestScheduleRun.run.scheduled_for)} ·
                  已入队 {latestScheduleRun.run.started_count} 集 · 跳过{" "}
                  {latestScheduleRun.run.skipped_count} 集
                </span>
                {latestScheduleItem && (
                  <span>
                    {latestScheduleItemPending
                      ? "此集正在确认加工资格"
                      : latestScheduleItem.outcome === "started"
                        ? "此集已加入加工队列"
                        : `此集跳过：${
                            scheduleSkipLabels[
                              latestScheduleItem.reason || ""
                            ] ||
                            latestScheduleItem.reason ||
                            "未满足条件"
                          }`}
                  </span>
                )}
                {latestScheduleRun.run.error_message && (
                  <span>{latestScheduleRun.run.error_message}</span>
                )}
              </div>
            )}

            {isActive(run) && run.next_attempt_at && (
              <div className={styles.processingHint} role="status">
                自动重试：{formatUpdatedAt(run.next_attempt_at)}（已尝试{" "}
                {run.attempt_count}/{run.max_attempts} 次）
              </div>
            )}

            {run?.error_message && (
              <div
                className={
                  run.status === "cancelled"
                    ? styles.processingCancellation
                    : styles.processingFailure
                }
                role={run.status === "cancelled" ? "status" : undefined}
              >
                <strong>{run.error_code || "PROCESSING_FAILED"}</strong>
                <span>{run.error_message}</span>
                {detail?.action_suggestion && (
                  <span>{detail.action_suggestion}</span>
                )}
              </div>
            )}
            {!run && audioAsset?.error_message && (
              <div className={styles.processingFailure}>
                <strong>
                  {audioAsset.error_code || "AUDIO_PREPARATION_FAILED"}
                </strong>
                <span>{audioAsset.error_message}</span>
                <span>修正音频来源后可重新开始；失败文件不会保留。</span>
              </div>
            )}

            {detail && detail.deliveries.length > 0 && (
              <div className={styles.processingArtifacts}>
                <div>
                  <strong>知识交付</strong>
                  {detail.deliveries.map((delivery) => (
                    <span key={delivery.id}>
                      {delivery.target} · {delivery.destination} ·{" "}
                      {deliveryStatusLabels[delivery.status] || delivery.status}
                    </span>
                  ))}
                </div>
                {detail.deliveries.some(
                  (delivery) => delivery.status === "pending",
                ) && (
                  <span className={styles.processingHint}>
                    本地包已保存，可按说明人工导入。
                  </span>
                )}
              </div>
            )}
          </div>
        </details>
      )}
    </section>
  );
});

export default EpisodeProcessingPanel;
