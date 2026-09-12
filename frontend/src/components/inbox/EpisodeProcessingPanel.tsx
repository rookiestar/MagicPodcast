"use client";

import {
  type KeyboardEvent,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { IconDownload, IconPlayerStop, IconRefresh } from "@tabler/icons-react";
import MarkdownViewer from "@/components/workflows/MarkdownViewer";
import { getProcessingErrorDetails, processingApi } from "@/lib/api/processing";
import { type EpisodeRoute, updateQuery } from "@/lib/navigation";
import type { ConsumptionItem } from "@/types/consumption";
import type {
  ArtifactContent,
  ArtifactContentKind,
  AudioRecoverySummary,
  EpisodeAudioAsset,
  EpisodeArtifactSet,
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

type ArtifactTabState = "ready" | "updating" | "pending" | "failed";

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

// The transcript entry itself is fixed (#314): the outer tab only needs the
// phase for its restrained marker and the accessible status name, while the
// readable copy and every start/retry action live in this panel body.
export interface EpisodeProcessingHeaderState {
  kind: "loading" | "idle" | "active" | "completed" | "failed";
  label: string;
}

interface EpisodeProcessingPanelProps {
  routeState?: EpisodeRoute;
  routeArtifact?: ArtifactTab;
  onRouteArtifactChange?: (tab: ArtifactTab, replace?: boolean) => void;
  item: ConsumptionItem;
  onHeaderStateChange?: (state: EpisodeProcessingHeaderState) => void;
}

function EpisodeProcessingPanel({
  item,
  onHeaderStateChange,
  routeArtifact,
  routeState,
  onRouteArtifactChange,
}: EpisodeProcessingPanelProps) {
  const [detail, setDetail] = useState<ProcessingRunDetail | null>(null);
  const [latestLoading, setIsLoading] = useState(true);
  const [sourceVersion, setSourceVersion] = useState<{ key: string; artifact?: EpisodeArtifactSet; error?: string } | null>(null);
  const [sourceRetry, setSourceRetry] = useState(0);
  const referenceKey = `${item.episode_id}:${routeState?.sourceID}`;
  const knownCurrent = detail?.current_artifact;
  const knownSource = knownCurrent?.id === routeState?.sourceID && knownCurrent?.episode_id === item.episode_id ? knownCurrent : undefined;
  const resolvedSource = sourceVersion?.key === referenceKey ? sourceVersion : knownSource ? {key:referenceKey,artifact:knownSource} : null;
  const isLoading = routeState?.hasReference ? !resolvedSource : latestLoading;
  useEffect(() => {
    if (!routeState?.hasReference || !routeState.sourceID || routeState.referenceInvalid) return;
    if (knownSource) { setSourceVersion({key:referenceKey,artifact:knownSource}); return; }
    let active = true;
    setSourceVersion(null);
    processingApi.getEpisodeArtifact(item.episode_id, routeState.sourceID).then((artifact) => {
      if (active) setSourceVersion({ key: referenceKey, artifact });
    }).catch((error: unknown) => {
      if (active) setSourceVersion({ key: referenceKey, error: getProcessingErrorDetails(error).status === 404 ? "原引用不可定位：此单集没有该版本。" : "引用版本读取失败，请重试。" });
    });
    return () => { active = false; };
  }, [item.episode_id, routeState?.hasReference, routeState?.sourceID, routeState?.referenceInvalid, referenceKey, sourceRetry, knownSource]);
  const [isMutating, setIsMutating] = useState(false);
  const [isRecoveringAudio, setIsRecoveringAudio] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [artifactReadFailures, setArtifactReadFailures] = useState<
    Set<ArtifactContentKind>
  >(() => new Set());
  const [artifactContents, setArtifactContents] = useState<ArtifactContents>(
    emptyArtifactContents,
  );
  const [localArtifactTab, setLocalArtifactTab] =
    useState<ArtifactTab>("minutes");
  const activeArtifactTab = routeArtifact ?? localArtifactTab;
  const setActiveArtifactTab = (tab: ArtifactTab) => {
    setLocalArtifactTab(tab);
  };
  const [summaryAbsentNotice, setSummaryAbsentNotice] = useState(false);
  const [artifactStateAnnouncement, setArtifactStateAnnouncement] =
    useState("");
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
  const announcedArtifactState = useRef("");
  const trackedArtifactID = useRef<number | null>(null);
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
        return;
      }
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
        throw loadError;
      }
    } finally {
      loadingEpisodeIDs.current.delete(episodeID);
    }
  }, [item.episode_id, loadScheduleStatus]);

  useEffect(() => {
    let active = true;
    setDetail(null);
    setAudioAsset(null);
    setArtifactReadFailures(new Set());
    setArtifactContents(emptyArtifactContents);
    setActiveArtifactTab("minutes");
    setSummaryAbsentNotice(false);
    setArtifactStateAnnouncement("");
    announcedArtifactState.current = "";
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
        setArtifactReadFailures((current) => {
          if (!current.has(kind)) return current;
          const next = new Set(current);
          next.delete(kind);
          return next;
        });
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
        setArtifactReadFailures((current) => {
          const next = new Set(current);
          next.add(kind);
          return next;
        });
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
  const currentArtifact = routeState?.hasReference ? resolvedSource?.artifact : detail?.current_artifact;
  const historicalSource = Boolean(routeState?.hasReference && currentArtifact && (!currentArtifact.is_current || (detail?.current_artifact && detail.current_artifact.id !== currentArtifact.id)));

  useEffect(() => {
    const artifactID = currentArtifact?.id ?? null;
    if (trackedArtifactID.current === artifactID) return;
    trackedArtifactID.current = artifactID;
    artifactReadSequence.current += 1;
    artifactReadInFlight.current = null;
    setIsReadingArtifact(false);
    setArtifactReadFailures(new Set());
    setArtifactStateAnnouncement("");
    announcedArtifactState.current = "";
  }, [currentArtifact?.id]);

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
  const audioPreparing =
    run?.current_step === "audio_prepare" ||
    audioAsset?.status === "queued" ||
    audioAsset?.status === "downloading";
  const runActive = isActive(run);
  const runTerminalUnsuccessful =
    run?.status === "failed" || run?.status === "cancelled";
  const processingUnderway = runActive || audioPreparing;
  const minutesArtifactContent = artifactContents.minutes;
  const transcriptArtifactContent = artifactContents.transcript;
  const minutesContentMatchesCurrent = Boolean(
    currentArtifact &&
      minutesArtifactContent?.artifactSetId === currentArtifact.id &&
      minutesArtifactContent.content.kind === summaryKind,
  );
  const transcriptContentMatchesCurrent = Boolean(
    currentArtifact &&
      transcriptArtifactContent?.artifactSetId === currentArtifact.id &&
      transcriptArtifactContent.content.kind === "transcript",
  );
  const minutesVisualAvailable = Boolean(
    minutesArtifactContent?.content.whiteboard ||
      minutesArtifactContent?.content.visual_items?.some(
        (visualItem) => visualItem.type === "whiteboard",
      ),
  );
  const visualSummaryAvailable =
    minutesContentMatchesCurrent && minutesVisualAvailable;
  const nativeOutputsExpected = Boolean(
    (run && run.pipeline_version !== legacyProcessingPipelineVersion) ||
      (!run && audioPreparing),
  );
  const summaryExpected = Boolean(
    (currentArtifact && summaryKind === "minutes_summary") ||
      nativeOutputsExpected,
  );
  const summaryAbsent =
    minutesContentMatchesCurrent &&
    summaryKind === "minutes_summary" &&
    !minutesVisualAvailable;
  const requestedArtifactKind: ArtifactContentKind | null = !currentArtifact
    ? null
    : activeArtifactTab === "summary"
      ? summaryKind === "minutes_summary"
        ? "minutes_summary"
        : null
      : activeArtifactTab === "minutes"
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
  const summaryVisible = summaryExpected && !summaryAbsent;
  const coreProductAwaitingVersion =
    processingUnderway || (runTerminalUnsuccessful && !currentArtifact);
  const minutesVisible = Boolean(summaryKind) || coreProductAwaitingVersion;
  const transcriptVisible =
    transcriptAvailable || coreProductAwaitingVersion;
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

  const isArtifactTabVisible = useCallback(
    (tab: ArtifactTab) =>
      tab === "summary"
        ? summaryVisible
        : tab === "minutes"
          ? minutesVisible
          : transcriptVisible,
    [minutesVisible, summaryVisible, transcriptVisible],
  );

  const artifactTabState = (tab: ArtifactTab): ArtifactTabState => {
    if (tab === "transcript") {
      if (artifactReadFailures.has("transcript")) return "failed";
      if (transcriptContentMatchesCurrent) {
        return runActive ? "updating" : "ready";
      }
      if (runActive && transcriptAvailable) return "updating";
    } else if (tab === "summary") {
      if (artifactReadFailures.has("minutes_summary")) return "failed";
      if (visualSummaryAvailable) return runActive ? "updating" : "ready";
      if (
        runActive &&
        currentArtifact?.capabilities.minutes_summary === true
      ) {
        return "updating";
      }
    } else if (
      summaryKind !== null &&
      artifactReadFailures.has(summaryKind)
    ) {
      return "failed";
    } else if (minutesContentMatchesCurrent) {
      return runActive ? "updating" : "ready";
    } else if (runActive && summaryKind !== null) {
      return "updating";
    }
    if (currentArtifact && isReadingArtifact) return "pending";
    if (runTerminalUnsuccessful) return "failed";
    if (processingUnderway) return "pending";
    return "ready";
  };

  const artifactTabStateDescription = (tab: ArtifactTab) => {
    switch (artifactTabState(tab)) {
      case "updating":
        return "正在生成新版，当前展示上一成功版本";
      case "pending":
        return currentArtifact && isReadingArtifact ? "正在读取" : "生成中";
      case "failed":
        if (
          tab === "transcript"
            ? artifactReadFailures.has("transcript")
            : artifactReadFailures.has("minutes_summary")
        ) {
          return "读取失败";
        }
        return run?.status === "cancelled" ? "已取消" : "生成失败";
      default:
        return "已可用";
    }
  };

  const handleArtifactTabKeyDown = (
    event: KeyboardEvent<HTMLButtonElement>,
    currentTab: ArtifactTab,
  ) => {
    const enabledTabs = artifactTabs.filter((tab) =>
      isArtifactTabVisible(tab.id),
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
    onRouteArtifactChange?.(nextTab);
    artifactTabRefs.current[nextTab]?.focus();
  };

  useEffect(() => {
    if (!currentArtifact || !requestedArtifactKind) return;
    if (artifactContentMatchesSelection) return;
    void readArtifact(currentArtifact.id, requestedArtifactKind);
  }, [
    activeArtifactTab,
    artifactContentMatchesSelection,
    currentArtifact,
    readArtifact,
    requestedArtifactKind,
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
      return { kind: "loading", label: "正在读取" };
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
      };
    }
    if (currentArtifact) {
      return { kind: "completed", label: "转写就绪" };
    }
    return { kind: "idle", label: "未转写" };
  }, [
    audioAsset,
    audioPreparing,
    currentArtifact,
    isLoading,
    isMinutesResyncFailure,
    run,
  ]);

  useEffect(() => {
    onHeaderStateChange?.(headerState);
  }, [headerState, onHeaderStateChange]);

  const visibleArtifactTabs = useMemo(
    () => artifactTabs.filter((tab) => isArtifactTabVisible(tab.id)),
    [isArtifactTabVisible],
  );
  const defaultArtifactTab: ArtifactTab | null = minutesVisible
    ? "minutes"
    : transcriptVisible
      ? "transcript"
      : summaryVisible
        ? "summary"
        : null;
  const renderedArtifactTab =
    routeArtifact ??
    (visibleArtifactTabs.some((tab) => tab.id === activeArtifactTab)
      ? activeArtifactTab
      : defaultArtifactTab);

  useEffect(() => {
    if (routeArtifact || !renderedArtifactTab) return;
    if (renderedArtifactTab !== activeArtifactTab) {
      if (
        artifactTabWasUserSelected.current &&
        activeArtifactTab === "summary"
      ) {
        setSummaryAbsentNotice(true);
      }
      setActiveArtifactTab(renderedArtifactTab);
      return;
    }
    if (!visualSummaryAvailable) return;
    setSummaryAbsentNotice(false);
    if (
      !artifactTabWasUserSelected.current &&
      activeArtifactTab === "minutes"
    ) {
      setActiveArtifactTab("summary");
    }
  }, [
    activeArtifactTab,
    renderedArtifactTab,
    visualSummaryAvailable,
    routeArtifact,
  ]);

  useEffect(() => {
    if (!isLoading && !routeArtifact && renderedArtifactTab && onRouteArtifactChange) {
      // Wait for the first minutes read before canonicalizing an unspecified
      // artifact. A visual summary starts from the minutes tab, then the
      // default-tab effect promotes it to summary; writing `minutes` here
      // first would pin the route and prevent that promotion.
      if (
        renderedArtifactTab === "minutes" &&
        summaryKind === "minutes_summary" &&
        (!minutesContentMatchesCurrent ||
          (visualSummaryAvailable && !artifactTabWasUserSelected.current))
      ) {
        return;
      }
      onRouteArtifactChange(renderedArtifactTab, true);
    }
  }, [
    isLoading,
    routeArtifact,
    renderedArtifactTab,
    onRouteArtifactChange,
    summaryKind,
    minutesContentMatchesCurrent,
    visualSummaryAvailable,
  ]);

  const renderedArtifactStateKey = renderedArtifactTab
    ? `${renderedArtifactTab}:${artifactTabStateDescription(renderedArtifactTab)}`
    : "";
  useEffect(() => {
    if (!renderedArtifactStateKey) return;
    const previous = announcedArtifactState.current;
    if (previous === renderedArtifactStateKey) return;
    announcedArtifactState.current = renderedArtifactStateKey;
    if (!previous) return;
    const [previousTab, previousDescription] = previous.split(":");
    const [nextTab, nextDescription] = renderedArtifactStateKey.split(":");
    if (previousTab !== nextTab) return;
    if (nextDescription === "已可用") {
      if (previousDescription === "正在生成新版，当前展示上一成功版本") {
        setArtifactStateAnnouncement(
          `${artifactTabLabel(previousTab as ArtifactTab)}，新版已就绪`,
        );
      } else if (
        previousDescription === "生成中" ||
        previousDescription === "正在读取"
      ) {
        setArtifactStateAnnouncement(
          `${artifactTabLabel(previousTab as ArtifactTab)}，已可用`,
        );
      }
      return;
    }
    if (
      nextDescription === "读取失败" ||
      nextDescription === "生成失败" ||
      nextDescription === "已取消"
    ) {
      setArtifactStateAnnouncement(
        `${artifactTabLabel(previousTab as ArtifactTab)}，${nextDescription}`,
      );
    }
  }, [renderedArtifactStateKey]);

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
        : run?.status === "failed"
          ? isMinutesResyncFailure
            ? "智能纪要同步失败"
            : "转写失败"
          : run?.status === "cancelled"
            ? "转写已取消"
            : run?.current_step === "minutes_enrichment"
              ? "等待飞书智能纪要"
              : run?.status === "waiting_external"
                ? "飞书妙记转写中"
                : audioPreparing
                  ? "正在准备音频"
                  : isActive(run)
                    ? "转写进行中"
                    : run?.status === "completed"
                      ? "转写已完成"
                      : audioAsset?.status === "failed"
                        ? "音频准备失败"
                        : audioAsset?.status === "ready"
                          ? "音频已就绪"
                          : canStart
                            ? "把这期节目，变成可回看的文字"
                            : "加入 Focus 后可开始转写";
  const processingStateDescription = isMutating
    ? isMinutesResyncFailure
      ? "正在续取同一条妙记，不会重新上传音频或创建妙记。"
      : "正在创建飞书妙记任务…"
    : isLoading
      ? "正在同步最近一次运行。"
      : error && !run && !audioAsset
        ? "请重试读取，Show Notes 与笔记不受影响。"
        : run?.status === "failed" || run?.status === "cancelled"
          ? detail?.action_suggestion || run.error_message || "可重新发起转写。"
          : run?.current_step === "minutes_enrichment"
            ? "核心转写已就绪，正在只读等待飞书智能纪要完整。"
            : run?.status === "waiting_external"
              ? "飞书妙记正在生成纪要与逐字稿。"
              : audioPreparing
                ? "音频就绪后会自动提交飞书妙记。"
                : isActive(run)
                  ? "任务在后台继续，可随时返回查看。"
                  : run?.status === "completed"
                    ? "暂未发现可阅读的转写产物。"
                    : audioAsset?.status === "failed"
                      ? audioAsset.error_message || "请检查音频来源后重试。"
                      : audioAsset?.status === "ready"
                        ? "正在等待创建转写任务。"
                        : canStart
                          ? "生成总结、纪要与逐字稿"
                          : "开始转写后，纪要与逐字稿会显示在这里。";

  // The start command lives in the body (#314) and stays quiet while the
  // processing status itself is unreadable or still loading.
  const canStartFromCard =
    canStart && !isLoading && !(error && !run && !audioAsset);
  const cardRetryCommand = canReprocessLegacy
    ? { label: "重新转写", run: () => startProcessing() }
    : canRetry
      ? {
          label: minutesResyncErrorCodes.has(run?.error_code ?? "")
            ? "重新同步"
            : "重试转写",
          run: () => retryProcessing(),
        }
      : null;

  const processingStateCard = (
    <div
      className={styles.processingStateCard}
      data-state={processingStateKind}
      role={isLoading || isActive(run) || isMutating ? "status" : undefined}
    >
      <span className={styles.processingStateDot} aria-hidden="true" />
      <div className={styles.processingStateCopy}>
        <strong>{processingStateTitle}</strong>
        <p>{processingStateDescription}</p>
        {(audioPreparing || isActive(run)) && (
          <p className={styles.processingStateHint}>
            你可以继续阅读 Show Notes。
          </p>
        )}
      </div>
      {!currentArtifact && canStartFromCard && (
        <button
          type="button"
          className={styles.primaryCommand}
          disabled={isMutating}
          onClick={() => void startProcessing()}
        >
          开始转写
        </button>
      )}
      {!currentArtifact && cardRetryCommand && (
        <button
          type="button"
          className={styles.primaryCommand}
          disabled={isMutating}
          onClick={() => void cardRetryCommand.run()}
        >
          <IconRefresh size={18} stroke={1.8} aria-hidden="true" />
          {cardRetryCommand.label}
        </button>
      )}
      {run && isActive(run) && (
        <button
          type="button"
          className={styles.secondaryCommand}
          disabled={isMutating}
          onClick={() => void mutateRun(() => processingApi.cancel(run.id))}
        >
          <IconPlayerStop size={18} stroke={1.8} aria-hidden="true" />
          取消
        </button>
      )}
    </div>
  );

  const artifactWaitingCopy = (tab: ArtifactTab) => {
    const label = artifactTabLabel(tab);
    if (runTerminalUnsuccessful) {
      return run?.status === "cancelled"
        ? `转写已取消，${label}未生成。`
        : `${label}未能随本次转写生成${canRetry ? "，可在上方重试" : ""}。`;
    }
    return `正在生成新版${label}，尚无可读内容。`;
  };

  const artifactPanelContent = (
    <>
      {currentArtifact &&
        summaryAbsentNotice &&
        renderedArtifactTab === "minutes" && (
          <div className={styles.processingHint} role="status">
            本版本没有受管画板或图片，视觉总结不存在，已切换到纪要。
          </div>
        )}

      {!currentArtifact && processingStateCard}

      {currentArtifact &&
        selectedArtifactContent &&
        (runActive ||
          (!artifactContentMatchesSelection && isReadingArtifact)) && (
          <div className={styles.processingHint} role="status">
            {runActive
              ? `正在生成新版${artifactTabLabel(
                  renderedArtifactTab ?? activeArtifactTab,
                )}，当前展示上一成功版本。`
              : `正在读取${artifactTabLabel(
                  renderedArtifactTab ?? activeArtifactTab,
                )}，暂时显示上一成功内容…`}
          </div>
        )}

      {currentArtifact &&
        renderedArtifactTab &&
        !selectedArtifactContent &&
        !showArtifactSkeleton &&
        !artifactReadFailed &&
        artifactTabState(renderedArtifactTab) !== "ready" && (
          <div
            className={`${styles.processingEmpty} ${
              artifactTabState(renderedArtifactTab) === "failed"
                ? styles.processingEmptyFailure
                : ""
            }`}
            role="status"
          >
            {artifactWaitingCopy(renderedArtifactTab)}
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

      {currentArtifact && selectedArtifactContent && (!routeState?.hasReference || artifactContentMatchesSelection) && (
        <div
          className={styles.processingDocument}
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
          {/* Normal completion stays quiet: the tab carries the product
              identity and the working player carries audio availability. */}
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
              episodeId={item.episode_id}
              artifactSetId={selectedArtifactContent.artifactSetId}
              routeState={routeState}
              readOnlyPeople={historicalSource}
              segments={selectedArtifactContent.content.segments}
              mediaAvailable={selectedArtifactContent.content.media_available}
              audioDurationSeconds={
                selectedArtifactContent.content.audio_duration_seconds
              }
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

  const referenceMissingTranscript = Boolean(currentArtifact && !currentArtifact.capabilities.transcript);
  if (routeState?.hasReference && (routeState.referenceInvalid || resolvedSource?.error || !currentArtifact || referenceMissingTranscript)) {
    return <section className={styles.processingSection} aria-label="转写内容">
      <p role={routeState.referenceInvalid || resolvedSource?.error || referenceMissingTranscript ? "alert" : "status"}>{routeState.referenceInvalid ? "原引用不可定位：来源版本或片段参数无效。" : referenceMissingTranscript ? "原引用不可定位：该版本没有逐字稿。" : resolvedSource?.error ?? "正在读取引用版本…"}</p>
      {resolvedSource?.error && <button onClick={() => setSourceRetry((value) => value + 1)}>重试读取引用</button>}
      <button onClick={() => updateQuery({ source: null, fragment: null, t: null })}>打开当前版本</button>
    </section>;
  }

  return (
    <section className={styles.processingSection} aria-label="转写内容">
      {historicalSource && <p role="status">正在查看引用的历史转写；人物管理只适用于当前版本。<button onClick={() => updateQuery({ source: null, fragment: null, t: null })}>打开当前版本</button></p>}
      {routeState?.peopleOpen && !transcriptAvailable && !isLoading && <p role="status">当前没有可供人物核对的逐字稿；打开链接不会自动转写。</p>}
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
                ? `重试读取${artifactTabLabel(
                    renderedArtifactTab ?? activeArtifactTab,
                  )}`
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
            {canRetry && !minutesResyncErrorCodes.has(run.error_code ?? "") && (
              <button
                type="button"
                className={styles.secondaryCommand}
                disabled={isMutating}
                onClick={() => void retryProcessing()}
              >
                <IconRefresh size={18} stroke={1.8} aria-hidden="true" />
                {isMutating ? "正在重试…" : "重试转写"}
              </button>
            )}
            {canReprocessLegacy && (
              <button
                type="button"
                className={styles.secondaryCommand}
                disabled={isMutating}
                onClick={() => void startProcessing()}
              >
                <IconRefresh size={18} stroke={1.8} aria-hidden="true" />
                {isMutating ? "正在发起…" : "重新转写"}
              </button>
            )}
          </div>
        )}

      {!currentArtifact && visibleArtifactTabs.length === 0 && (
        <>{processingStateCard}</>
      )}

      {routeArtifact &&
        !isLoading &&
        !visibleArtifactTabs.some((tab) => tab.id === routeArtifact) && (
          <p role="status">请求的{artifactTabLabel(routeArtifact)}尚不可用。</p>
        )}
      {(currentArtifact || visibleArtifactTabs.length > 0) && (
        <div className={styles.processingSummary}>
          <div className={styles.processingArtifactHeader}>
            {currentArtifact && (
              <div className={styles.processingArtifactMeta}>
                {summaryKind && (
                  <span>
                    {summaryKind === "minutes_summary"
                      ? "飞书智能纪要"
                      : "旧版纪要"}
                  </span>
                )}
                <span>
                  {historicalSource ? "引用的历史版本" : run?.id === currentArtifact.run_id
                    ? "当前版本"
                    : "上一成功版本"}
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
            {visibleArtifactTabs.length > 0 && (
              <div
                className={styles.processingArtifactTabs}
                role="tablist"
                aria-label="转写产物"
              >
                {visibleArtifactTabs.map((tab) => {
                  const selected = renderedArtifactTab === tab.id;
                  const state = artifactTabState(tab.id);
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
                      aria-describedby={`processing-artifact-tab-state-${tab.id}`}
                      tabIndex={selected ? 0 : -1}
                      onClick={() => {
                        artifactTabWasUserSelected.current = true;
                        setActiveArtifactTab(tab.id);
                        onRouteArtifactChange?.(tab.id);
                      }}
                      onKeyDown={(event) =>
                        handleArtifactTabKeyDown(event, tab.id)
                      }
                    >
                      {state !== "ready" && (
                        <span
                          className={styles.processingArtifactTabDot}
                          data-state={
                            state === "failed" ? "failed" : "working"
                          }
                          aria-hidden="true"
                        />
                      )}
                      {tab.label}
                    </button>
                  );
                })}
              </div>
            )}
          </div>

          <div className="sr-only">
            {visibleArtifactTabs.map((tab) => (
              <span key={tab.id} id={`processing-artifact-tab-state-${tab.id}`}>
                {`${tab.label}，${artifactTabStateDescription(tab.id)}`}
              </span>
            ))}
          </div>
          <span className="sr-only" role="status" aria-label="转写产物状态">
            {artifactStateAnnouncement}
          </span>

          {visibleArtifactTabs.map((tab) => (
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

            {/* Completed runs keep their low-frequency maintenance command in
                the run details; the reading flow stays quiet (#314). Without a
                stored artifact the state card already carries the command. */}
            {run?.status === "completed" && canReprocessLegacy && currentArtifact && (
              <div className={styles.processingMaintenance}>
                <button
                  type="button"
                  className={styles.secondaryCommand}
                  disabled={isMutating}
                  onClick={() => void startProcessing()}
                >
                  <IconRefresh size={18} stroke={1.8} aria-hidden="true" />
                  {isMutating ? "正在发起…" : "重新转写"}
                </button>
              </div>
            )}

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
}

export default EpisodeProcessingPanel;
