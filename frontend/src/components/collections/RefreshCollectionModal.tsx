"use client";

import { useEffect, useRef, useState } from "react";
import { IconX } from "@tabler/icons-react";
import {
  applyCollectionRefresh,
  collectionErrorMessage,
  collectionErrorCode,
  refreshCollectionPreview,
  type CollectionApplyRefreshResult,
  type CollectionRefreshPreview,
} from "@/lib/collections";

interface RefreshCollectionModalProps {
  isOpen: boolean;
  collectionID: number;
  onClose: () => void;
  onApplied: (result: CollectionApplyRefreshResult) => void;
}

/**
 * 刷新清单弹窗：读取差异 → 展示新增/移出/重排/推荐语变化 → 确认应用。
 * 失败保留既有清单；过期预览明确要求重新刷新。
 */
export default function RefreshCollectionModal({
  isOpen,
  collectionID,
  onClose,
  onApplied,
}: RefreshCollectionModalProps) {
  const [preview, setPreview] = useState<CollectionRefreshPreview | null>(null);
  const [loading, setLoading] = useState(false);
  const [applying, setApplying] = useState(false);
  const [error, setError] = useState("");
  const [attempt, setAttempt] = useState(0);
  const abortRef = useRef<AbortController | null>(null);
  const dialogRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!isOpen) return;
    const previous =
      document.activeElement instanceof HTMLElement
        ? document.activeElement
        : null;
    dialogRef.current?.querySelector<HTMLElement>("button")?.focus();
    return () => previous?.focus();
  }, [isOpen]);

  useEffect(() => {
    if (!isOpen) return;
    let cancelled = false;
    const controller = new AbortController();
    abortRef.current = controller;
    setLoading(true);
    setError("");
    setPreview(null);
    refreshCollectionPreview(collectionID, controller.signal)
      .then((result) => {
        if (!cancelled) setPreview(result);
      })
      .catch((caught) => {
        if (!cancelled)
          setError(collectionErrorMessage(caught, "刷新失败，可稍后重试。"));
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
      controller.abort();
    };
  }, [isOpen, collectionID, attempt]);

  if (!isOpen) return null;

  const close = () => {
    if (applying) return;
    abortRef.current?.abort();
    onClose();
  };

  const apply = async () => {
    if (!preview) return;
    setApplying(true);
    setError("");
    try {
      const result = await applyCollectionRefresh(
        collectionID,
        preview.preview_id,
        preview.base_revision,
      );
      onClose();
      onApplied(result);
    } catch (caught) {
      const code = collectionErrorCode(caught);
      if (code === "PREVIEW_EXPIRED") {
        setError("刷新预览已过期，请重新刷新。");
        setPreview(null);
      } else if (code === "REVISION_CONFLICT") {
        setError("清单已被其他页面更新，请重新刷新后再应用。");
        setPreview(null);
      } else {
        setError(collectionErrorMessage(caught, "应用刷新失败，可稍后重试。"));
      }
    } finally {
      setApplying(false);
    }
  };

  const handleKeyDown = (event: React.KeyboardEvent<HTMLDivElement>) => {
    if (event.key === "Escape" && !applying) {
      event.preventDefault();
      event.stopPropagation();
      close();
      return;
    }
    if (event.key === "Tab") {
      const controls = Array.from(
        event.currentTarget.querySelectorAll<HTMLElement>(
          'button:not([disabled]), [tabindex="0"]',
        ),
      ).filter((element) => element.getClientRects().length);
      const first = controls[0];
      const last = controls.at(-1);
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last?.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first?.focus();
      }
    }
  };

  return (
    <div className="editorial-modal-backdrop fixed inset-0 z-[60] flex items-center justify-center p-4">
      <div
        className="editorial-modal shadow-2xl w-full max-w-lg overflow-hidden flex flex-col max-h-[calc(100vh-2rem)]"
        ref={dialogRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby="refresh-collection-title"
        onKeyDown={handleKeyDown}
      >
        <div className="editorial-modal-header">
          <div className="editorial-modal-heading">
            <span className="editorial-modal-kicker">播客清单</span>
            <small>刷新</small>
          </div>
          <button
            type="button"
            onClick={close}
            className="editorial-modal-close"
            aria-label="关闭"
            disabled={applying}
          >
            <IconX aria-hidden="true" stroke={1.8} />
          </button>
        </div>

        <h2 id="refresh-collection-title" className="editorial-modal-title">
          刷新清单
        </h2>

        <div className="editorial-modal-body overflow-y-auto">
          {loading && (
            <p
              className="collection-form-hint"
              role="status"
              aria-live="polite"
            >
              正在读取源清单的最新内容…
            </p>
          )}
          {error && (
            <p className="collection-form-error" role="alert">
              {error}
            </p>
          )}
          {error && !loading && !preview && (
            <button
              type="button"
              className="collection-btn-secondary"
              onClick={() => setAttempt((value) => value + 1)}
            >
              重新刷新
            </button>
          )}
          {preview && (
            <div className="flex flex-col gap-3">
              {preview.changes.added_count === 0 &&
              preview.changes.removed_count === 0 &&
              preview.changes.reordered_count === 0 &&
              preview.changes.recommendation_changed_count === 0 &&
              !preview.changes.metadata_changed_count &&
              !preview.changes.collection_changed ? (
                <p role="status">没有变化，确认后记录本次检查时间。</p>
              ) : (
                <>
                  <ul className="collection-refresh-summary">
                    <li>新增 {preview.changes.added_count} 条</li>
                    <li>移出 {preview.changes.removed_count} 条</li>
                    <li>顺序调整 {preview.changes.reordered_count} 条</li>
                    <li>
                      推荐语变化 {preview.changes.recommendation_changed_count}{" "}
                      条
                    </li>
                    {!!preview.changes.metadata_changed_count && (
                      <li>
                        单集信息变化 {preview.changes.metadata_changed_count} 条
                      </li>
                    )}
                    {preview.changes.collection_changed && (
                      <li>
                        清单信息更新：{preview.title} · 作者：
                        {preview.author || "未提供"}
                      </li>
                    )}
                  </ul>
                  {!!preview.items?.length && (
                    <ol
                      className="collection-preview-items"
                      aria-label="刷新后的清单顺序和推荐语"
                    >
                      {preview.items.map((item) => (
                        <li
                          key={item.external_episode_id}
                          className="collection-preview-item"
                        >
                          <span className="collection-preview-index">
                            {item.position + 1}
                          </span>
                          <span className="collection-preview-copy">
                            <strong>{item.episode_title}</strong>
                            <small>
                              {item.podcast_title}
                              {item.recommendation
                                ? ` · ${item.recommendation}`
                                : ""}
                            </small>
                          </span>
                        </li>
                      ))}
                    </ol>
                  )}
                  {preview.removed.length > 0 && (
                    <div>
                      <p className="collection-form-hint">
                        被移出的条目将退出本清单；其中已收录单集及其队列、笔记与来源记录全部保留。
                      </p>
                      <ul className="collection-preview-items">
                        {preview.removed.map((item) => (
                          <li
                            key={item.external_episode_id}
                            className="collection-preview-item"
                          >
                            <span className="collection-preview-index">
                              {item.position + 1}
                            </span>
                            <span className="collection-preview-copy">
                              <strong>{item.episode_title}</strong>
                              <small>
                                {item.podcast_title}
                                {item.adopted ? " · 已收录，将保留" : ""}
                              </small>
                            </span>
                          </li>
                        ))}
                      </ul>
                    </div>
                  )}
                  <p className="collection-form-hint">
                    将应用你正在预览的这份差异；新增条目保持未收录，不会自动进入
                    Inbox。
                  </p>
                </>
              )}
            </div>
          )}
        </div>

        <div className="editorial-modal-footer">
          <button
            type="button"
            className="collection-btn-secondary"
            onClick={close}
            disabled={applying}
          >
            取消
          </button>
          <button
            type="button"
            className="collection-btn-primary"
            onClick={() => void apply()}
            disabled={loading || !preview || applying}
          >
            {applying ? "正在应用…" : "应用刷新"}
          </button>
        </div>
      </div>
    </div>
  );
}
