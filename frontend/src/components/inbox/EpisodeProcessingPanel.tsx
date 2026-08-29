"use client";

import {
  forwardRef,
  useCallback,
  useEffect,
  useImperativeHandle,
  useMemo,
  useRef,
  useState,
} from "react";
import {
  IconFileText,
  IconPlayerPlay,
  IconPlayerStop,
  IconRefresh,
} from "@tabler/icons-react";
import MarkdownViewer from "@/components/workflows/MarkdownViewer";
import {
  getProcessingErrorDetails,
  processingApi,
} from "@/lib/api/processing";
import type { ConsumptionItem } from "@/types/consumption";
import type {
  ArtifactContent,
  EpisodeAudioAsset,
  KnowledgeDelivery,
  ProcessingRun,
  ProcessingRunDetail,
  ProcessingScheduleStatus,
} from "@/types/processing";
import styles from "./InboxPage.module.css";

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
  episode_notes: "Codex 生成单集纪要",
  artifact_publish: "发布本地产物",
};

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

export interface EpisodeProcessingHeaderState {
  kind: "loading" | "idle" | "active" | "completed" | "failed";
  label: string;
  detail: string;
  primaryLabel: string;
  primaryDisabled: boolean;
  action: "start" | "view" | "retry" | "details" | null;
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
  const [isLoading, setIsLoading] = useState(true);
  const [isMutating, setIsMutating] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [artifactContent, setArtifactContent] =
    useState<ArtifactContent | null>(null);
  const [audioAsset, setAudioAsset] = useState<EpisodeAudioAsset | null>(null);
  const [isReadingArtifact, setIsReadingArtifact] = useState(false);
  const [scheduleStatus, setScheduleStatus] =
    useState<ProcessingScheduleStatus | null>(null);
  const [isScheduleLoading, setIsScheduleLoading] = useState(true);
  const [scheduleError, setScheduleError] = useState<string | null>(null);
  const hasLoadedScheduleStatus = useRef(false);
  const scheduleStatusInFlight = useRef(false);
  const loadingEpisodeIDs = useRef(new Set<number>());
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
        `定时计划暂时无法读取：${
          getProcessingErrorDetails(loadError).message
        }`,
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
    setArtifactContent(null);
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

  const mutateRun = async (
    operation: () => Promise<ProcessingRun>,
  ) => {
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

  const readArtifact = async (
    artifactSetId: number,
    kind: "transcript" | "episode_notes",
  ) => {
    if (isReadingArtifact) return;
    setIsReadingArtifact(true);
    setError(null);
    try {
      setArtifactContent(
        await processingApi.getArtifactContent(artifactSetId, kind),
      );
    } catch (readError) {
      setError(`产物读取失败：${getProcessingErrorDetails(readError).message}`);
    } finally {
      setIsReadingArtifact(false);
    }
  };

  const run = detail?.run;
  const currentArtifact = detail?.current_artifact;
  const latestScheduleRun = scheduleStatus?.latest_run;
  const latestScheduleItem = latestScheduleRun?.items.find(
    (scheduleItem) => scheduleItem.episode_id === item.episode_id,
  );
  const latestScheduleItemPending =
    latestScheduleRun?.run.status === "running" &&
    latestScheduleItem?.reason === "selection_pending";
  const canStart = item.queue_state === "focus" && !run;
  const audioPreparing =
    run?.current_step === "audio_prepare" ||
    audioAsset?.status === "queued" || audioAsset?.status === "downloading";
  const canRetry =
    item.queue_state === "focus" &&
    (run?.status === "failed" || run?.status === "cancelled") &&
    !run.error_code?.toLowerCase().includes("result_unknown");
  const scheduleSummary = !scheduleStatus
    ? isScheduleLoading
      ? "正在读取…"
      : "定时状态暂时不可用"
    : scheduleStatus.enabled
      ? `已启用 · 每批 ${scheduleStatus.batch_size} 集`
      : "未启用";

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
      setError(
        `${failure.code ? `${failure.code}：` : ""}${failure.message}`,
      );
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
      };
    }
    if (audioPreparing || isActive(run)) {
      return {
        kind: "active",
        label:
          run?.status === "waiting_external"
            ? "等待妙记"
            : audioPreparing
              ? "准备音频"
              : "转写中",
        detail: "转写在后台继续，当前正文不受影响",
        primaryLabel: "转写中",
        primaryDisabled: true,
        action: null,
      };
    }
    if (run?.status === "failed" || run?.status === "cancelled") {
      return {
        kind: "failed",
        label: run.status === "cancelled" ? "已取消" : "转写失败",
        detail:
          detail?.action_suggestion ||
          run.error_message ||
          "可查看原因后安全重试",
        primaryLabel: canRetry ? "重试转写" : "查看详情",
        primaryDisabled: false,
        action: canRetry ? "retry" : "details",
      };
    }
    if (currentArtifact) {
      return {
        kind: "completed",
        label: "已完成",
        detail: "已有可阅读的转写产物",
        primaryLabel: "查看转写",
        primaryDisabled: false,
        action: "view",
      };
    }
    return {
      kind: "idle",
      label: "未转写",
      detail:
        item.queue_state === "focus"
          ? "可开始飞书妙记转写"
          : "加入 Focus 后可开始转写",
      primaryLabel:
        item.queue_state === "focus" ? "开始转写" : "加入 Focus 后转写",
      primaryDisabled: !canStart || isMutating,
      action: canStart ? "start" : null,
    };
  }, [
    audioAsset,
    audioPreparing,
    canRetry,
    canStart,
    currentArtifact,
    detail?.action_suggestion,
    isLoading,
    isMutating,
    item.queue_state,
    run,
  ]);

  useEffect(() => {
    onHeaderStateChange?.(headerState);
  }, [headerState, onHeaderStateChange]);

  useImperativeHandle(
    ref,
    () => ({
      activatePrimary: () => {
        if (headerState.primaryDisabled) return;
        if (headerState.action === "start") {
          void startProcessing();
          return;
        }
        if (headerState.action === "retry") {
          void retryProcessing();
          return;
        }
        if (
          headerState.action === "view" ||
          headerState.action === "details"
        ) {
          onViewTranscript?.();
        }
      },
    }),
    [headerState, onViewTranscript, retryProcessing, startProcessing],
  );

  return (
    <section
      className={styles.processingSection}
      aria-labelledby="processing-title"
    >
      <div className={styles.detailSectionHeading}>
        <div>
          <span className={styles.detailKicker}>FOCUS PROCESSING</span>
          <h3 id="processing-title">自动加工</h3>
        </div>
        {isLoading && <span role="status">正在读取…</span>}
      </div>

      {error && (
        <div className={styles.inlineError} role="alert">
          <span>{error}</span>
          <button
            type="button"
            className={styles.iconButton}
            disabled={isLoading}
            onClick={() => void loadLatest()}
            aria-label="重试读取加工状态"
            title="重试"
          >
            <IconRefresh size={18} stroke={1.8} aria-hidden="true" />
          </button>
        </div>
      )}

      <div className={styles.processingSummary}>
        <dl>
          <div>
            <dt>状态</dt>
            <dd>
              {run
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
                        : "尚未加工"}
            </dd>
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
            <dt>来源</dt>
            <dd>飞书妙记 · 本地 Codex Runtime</dd>
          </div>
          <div>
            <dt>最近更新</dt>
            <dd>{formatUpdatedAt(run?.updated_at || audioAsset?.updated_at)}</dd>
          </div>
          <div>
            <dt>定时计划</dt>
            <dd>{scheduleSummary}</dd>
          </div>
          <div>
            <dt>下次计划</dt>
            <dd>
              {scheduleStatus?.enabled
                ? formatUpdatedAt(scheduleStatus.next_run_at)
                : "—"}
            </dd>
          </div>
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
              cron：{scheduleStatus.cron} · 时区：{scheduleStatus.timezone} · 每批 {" "}
              {scheduleStatus.batch_size} 集
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
              {formatUpdatedAt(latestScheduleRun.run.scheduled_for)} · 已入队 {" "}
              {latestScheduleRun.run.started_count} 集 · 跳过 {" "}
              {latestScheduleRun.run.skipped_count} 集
            </span>
            {latestScheduleItem && (
              <span>
                {latestScheduleItemPending
                  ? "此集正在确认加工资格"
                  : latestScheduleItem.outcome === "started"
                  ? "此集已加入加工队列"
                  : `此集跳过：${
                      scheduleSkipLabels[latestScheduleItem.reason || ""] ||
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
            自动重试：{formatUpdatedAt(run.next_attempt_at)}（已尝试 {run.attempt_count}/
            {run.max_attempts} 次）
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
            <strong>{audioAsset.error_code || "AUDIO_PREPARATION_FAILED"}</strong>
            <span>{audioAsset.error_message}</span>
            <span>修正音频来源后可重新开始；失败文件不会保留。</span>
          </div>
        )}

        <div className={styles.processingActions}>
          {canStart && !audioPreparing && (
            <button
              type="button"
              className={styles.primaryCommand}
              disabled={isMutating}
              onClick={() => void startProcessing()}
            >
              <IconPlayerPlay size={18} stroke={1.8} aria-hidden="true" />
              开始转写
            </button>
          )}
          {isActive(run) && (
            <button
              type="button"
              className={styles.secondaryCommand}
              disabled={isMutating}
              onClick={() => void mutateRun(() => processingApi.cancel(run!.id))}
            >
              <IconPlayerStop size={18} stroke={1.8} aria-hidden="true" />
              取消
            </button>
          )}
          {canRetry && (
            <button
              type="button"
              className={styles.primaryCommand}
              disabled={isMutating}
              onClick={() => void retryProcessing()}
            >
              <IconRefresh size={18} stroke={1.8} aria-hidden="true" />
              重试转写
            </button>
          )}
          {isMutating && <span role="status">正在提交…</span>}
          {audioPreparing && <span role="status">正在下载并校验音频…</span>}
          {!run && item.queue_state !== "focus" && (
            <span className={styles.processingHint}>
              加入 Focus 后可手动加工；加入本身不会自动触发。
            </span>
          )}
        </div>

        {currentArtifact && (
          <div className={styles.processingArtifacts}>
            <div>
              <strong>
                {run?.id === currentArtifact.run_id
                  ? "当前成功产物"
                  : "上一成功版本"}
              </strong>
              <span>
                {currentArtifact.pipeline_version} ·{" "}
                {formatUpdatedAt(currentArtifact.created_at)}
              </span>
            </div>
            <button
              type="button"
              className={styles.secondaryCommand}
              disabled={isReadingArtifact}
              onClick={() =>
                void readArtifact(currentArtifact.id, "transcript")
              }
            >
              <IconFileText size={18} stroke={1.8} aria-hidden="true" />
              逐字稿
            </button>
            <button
              type="button"
              className={styles.secondaryCommand}
              disabled={isReadingArtifact}
              onClick={() =>
                void readArtifact(currentArtifact.id, "episode_notes")
              }
            >
              <IconFileText size={18} stroke={1.8} aria-hidden="true" />
              旧版纪要
            </button>
            <span className={styles.processingHint}>
              重新转写后可获得妙记纪要和同步逐字稿。
            </span>
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

      {artifactContent && (
        <div
          className={styles.processingDocument}
          data-copilot-source={
            artifactContent.kind === "transcript" ? "transcript" : undefined
          }
          data-copilot-episode-id={
            artifactContent.kind === "transcript"
              ? item.episode_id
              : undefined
          }
        >
          <div className={styles.metadataLabelRow}>
            <span>
              {artifactContent.kind === "transcript" ? "规范逐字稿" : "旧版纪要"}
            </span>
            <button
              type="button"
              className={styles.iconButton}
              onClick={() => setArtifactContent(null)}
              aria-label="关闭加工产物"
            >
              关闭
            </button>
          </div>
          <MarkdownViewer content={artifactContent.content} />
        </div>
      )}
    </section>
  );
});

export default EpisodeProcessingPanel;
