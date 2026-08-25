"use client";

import { useCallback, useEffect, useState } from "react";
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

export default function EpisodeProcessingPanel({
  item,
}: {
  item: ConsumptionItem;
}) {
  const [detail, setDetail] = useState<ProcessingRunDetail | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [isMutating, setIsMutating] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [artifactContent, setArtifactContent] =
    useState<ArtifactContent | null>(null);
  const [audioAsset, setAudioAsset] = useState<EpisodeAudioAsset | null>(null);
  const [isReadingArtifact, setIsReadingArtifact] = useState(false);

  const loadLatest = useCallback(async () => {
    const runs = await processingApi.listEpisodeRuns(item.episode_id);
    if (runs.length === 0) {
      setDetail(null);
      try {
        setAudioAsset(await processingApi.getLatestAudio(item.episode_id));
      } catch (audioError) {
        if (getProcessingErrorDetails(audioError).status === 404) {
          setAudioAsset(null);
        } else {
          throw audioError;
        }
      }
      return;
    }
    const nextDetail = await processingApi.getRun(runs[0].id);
    setDetail(nextDetail);
    if (nextDetail.run.current_step === "audio_prepare") {
      try {
        setAudioAsset(await processingApi.getLatestAudio(item.episode_id));
      } catch (audioError) {
        if (getProcessingErrorDetails(audioError).status === 404) {
          setAudioAsset(null);
        } else {
          throw audioError;
        }
      }
    } else {
      setAudioAsset(null);
    }
  }, [item.episode_id]);

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

  const startProcessing = async () => {
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
  };

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
  const canStart = item.queue_state === "focus" && !run;
  const audioPreparing =
    run?.current_step === "audio_prepare" ||
    audioAsset?.status === "queued" || audioAsset?.status === "downloading";
  const canRetry =
    item.queue_state === "focus" &&
    (run?.status === "failed" || run?.status === "cancelled") &&
    !run.error_code?.toLowerCase().includes("result_unknown");

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
        </dl>

        {run?.error_message && (
          <div className={styles.processingFailure}>
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
              开始加工
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
              onClick={() =>
                void (async () => {
                  if (isMutating) return;
                  setIsMutating(true);
                  setError(null);
                  try {
                    const result = await processingApi.retry(run!.id);
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
                })()
              }
            >
              <IconRefresh size={18} stroke={1.8} aria-hidden="true" />
              从检查点重试
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
              单集纪要
            </button>
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
        <div className={styles.processingDocument}>
          <div className={styles.metadataLabelRow}>
            <span>
              {artifactContent.kind === "transcript" ? "规范逐字稿" : "单集纪要"}
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
}
